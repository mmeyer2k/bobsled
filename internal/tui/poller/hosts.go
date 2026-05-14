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
	args := append(mux.Args(target), target,
		`systemctl --user list-units 'bobsled@*' --all --no-legend --plain --no-pager 2>/dev/null; `+
			`echo '---STATE---'; cat state.yaml 2>/dev/null`)
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
	parts := strings.SplitN(stdout.String(), "---STATE---", 2)
	parseUnits(parts[0], st)
	if len(parts) == 2 {
		parseState(parts[1], st)
	}
	return st, nil
}

func parseUnits(s string, st *HostState) {
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || !strings.HasPrefix(f[0], "bobsled@") {
			continue
		}
		var n int
		_, _ = fmt.Sscanf(f[0], "bobsled@%d.service", &n)
		ss := st.Slots[n]
		ss.N = n
		ss.UnitState = f[2] // active / activating / failed / inactive
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
// ctx done.
func HostsPoller(ctx context.Context, mux *SSHMux, targets []string, interval time.Duration, emit chan<- HostsMsg) {
	for _, t := range targets {
		go hostLoop(ctx, mux, t, interval, emit)
	}
	<-ctx.Done()
}

func hostLoop(ctx context.Context, mux *SSHMux, target string, interval time.Duration, emit chan<- HostsMsg) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		st, err := ProbeHost(ctx, mux, target)
		select {
		case <-ctx.Done():
			return
		case emit <- HostsMsg{Host: target, State: st, Err: err}:
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
