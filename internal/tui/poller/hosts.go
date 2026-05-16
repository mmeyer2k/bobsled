// internal/tui/poller/hosts.go
package poller

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/m-meyer2k/bobsled/internal/state"
	"gopkg.in/yaml.v3"
)

type SlotState struct {
	N         int
	UnitState string
	Enabled   bool
	Repo      string
	Container string
	StartedAt time.Time
}

type HostState struct {
	Name       string
	Slots      map[int]SlotState
	Capacity   int
	Reachable  bool
	LastError  string
	LastUpdate time.Time
}

// ProbeHost runs one combined SSH command to fetch units + state.yaml in a
// single round-trip. Non-zero ssh exits set Reachable=false and stash the
// error in LastError; the function returns an error only on truly unexpected
// local failures (e.g. exec failed locally).
func ProbeHost(ctx context.Context, mux *SSHMux, target string) (*HostState, error) {
	// `list-unit-files` only returns the template (`bobsled@.service`), not
	// the per-instance units. The enabled instances exist as symlinks in
	// `default.target.wants/`; listing that dir is the reliable way to know
	// which slots are currently enabled.
	script := `systemctl --user list-units 'bobsled@*' --all --no-legend --plain --no-pager 2>/dev/null; ` +
		`echo '---ENABLED---'; ` +
		`ls -1 $HOME/.config/systemd/user/default.target.wants/ 2>/dev/null | grep '^bobsled@' || true; ` +
		`echo '---STATE---'; cat state.yaml 2>/dev/null`
	args := append(mux.Args(target), target, script)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	st := &HostState{
		Name:       target,
		Slots:      map[int]SlotState{},
		LastUpdate: time.Now(),
	}
	if err != nil {
		st.Reachable = false
		st.LastError = fmt.Sprintf("%v: %s", err, strings.TrimSpace(stderr.String()))
		return st, nil
	}
	st.Reachable = true

	out := stdout.String()
	unitsPart, rest, _ := strings.Cut(out, "---ENABLED---")
	enabledPart, statePart, _ := strings.Cut(rest, "---STATE---")
	parseUnits(unitsPart, st)
	parseEnabled(enabledPart, st)
	parseState(statePart, st)
	return st, nil
}

func parseUnits(s string, st *HostState) {
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || !strings.HasPrefix(f[0], "bobsled@") {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(f[0], "bobsled@%d.service", &n); err != nil || n <= 0 {
			continue // skip the template `bobsled@.service` entry
		}
		ss := st.Slots[n]
		ss.N = n
		ss.UnitState = f[2] // active / activating / failed / inactive
		st.Slots[n] = ss
	}
}

// parseEnabled reads the output of `ls .../default.target.wants/` — one
// `bobsled@N.service` symlink per enabled slot.
func parseEnabled(s string, st *HostState) {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "bobsled@") {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(line, "bobsled@%d.service", &n); err != nil || n <= 0 {
			continue
		}
		ss := st.Slots[n]
		ss.N = n
		ss.Enabled = true
		st.Slots[n] = ss
	}
}

func parseState(s string, st *HostState) {
	var parsed state.State
	if err := yaml.Unmarshal([]byte(s), &parsed); err != nil || parsed.Instances == nil {
		return
	}
	for n, inst := range parsed.Instances {
		ss := st.Slots[n]
		ss.N = n
		ss.Repo = inst.Repo
		st.Slots[n] = ss
	}
}

type HostsMsg struct {
	Host  string
	State *HostState
	Err   error
}

// HostsPoller probes each target on an interval and sends results to emit.
// One goroutine per target so a slow host doesn't block the others. Stops on
// ctx done. hosts maps inventory short name → SSH target.
//
// `setInterval` lets callers retune the polling cadence at runtime. Send a
// new Duration on the channel and every per-host loop will `ticker.Reset()`
// on its next iteration. Pass nil to opt out — the loops will tick at the
// initial `interval` forever. Buffered size 1 is enough; later sends will
// overwrite a pending un-consumed update via the fan-out broker.
func HostsPoller(ctx context.Context, mux *SSHMux, hosts map[string]string, interval time.Duration, setInterval <-chan time.Duration, emit chan<- HostsMsg) {
	// One per-host control channel so the fan-out broker can deliver the new
	// interval to every loop. Buffered 1 with overwrite-on-full semantics —
	// only the latest value matters, an unread one is stale anyway.
	perLoop := make([]chan time.Duration, 0, len(hosts))
	for name, target := range hosts {
		ch := make(chan time.Duration, 1)
		perLoop = append(perLoop, ch)
		go hostLoop(ctx, mux, name, target, interval, ch, emit)
	}
	if setInterval != nil {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case d := <-setInterval:
					for _, ch := range perLoop {
						// Drain a stale value if present, then write. Keeps
						// channel buffer == 1 with "last write wins" semantics.
						select {
						case <-ch:
						default:
						}
						select {
						case ch <- d:
						default:
						}
					}
				}
			}
		}()
	}
	<-ctx.Done()
}

func hostLoop(ctx context.Context, mux *SSHMux, name, target string, interval time.Duration, setInterval <-chan time.Duration, emit chan<- HostsMsg) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		st, err := ProbeHost(ctx, mux, target)
		select {
		case <-ctx.Done():
			return
		case emit <- HostsMsg{Host: name, State: st, Err: err}:
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case d := <-setInterval:
			// Reset the ticker and proceed immediately to the next probe —
			// pressing R should feel responsive, not wait out the old period.
			tick.Reset(d)
		}
	}
}
