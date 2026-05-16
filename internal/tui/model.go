// internal/tui/model.go
package tui

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/tui/poller"
)

// parsePendingKey reverses the "host:slot" encoding used by Model.Pending.
func parsePendingKey(k string) (host string, slot int, ok bool) {
	i := strings.LastIndex(k, ":")
	if i < 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(k[i+1:])
	if err != nil {
		return "", 0, false
	}
	return k[:i], n, true
}

const (
	hostsInterval   = 2 * time.Second
	runnersInterval = 3 * time.Second
	runsInterval    = 15 * time.Second
)

// forceRedrawMsg is a no-op message used to trigger a Bubbletea re-render
// immediately after a huh form closes (huh emits tea.Quit on completion which
// we must discard, leaving the renderer stale until the next update).
type forceRedrawMsg struct{}

func forceRedrawCmd() tea.Cmd {
	return func() tea.Msg { return forceRedrawMsg{} }
}

type Model struct {
	Inv           *inventory.Inventory
	Client        *ghapp.Client
	Mux           *poller.SSHMux
	Hosts         map[string]*poller.HostState
	Runners       map[string]*poller.RepoRunners
	Runs          map[string]*poller.RepoRuns
	Errs          map[string]string // source label → last error
	InventoryPath string

	Cursor     Cursor
	Expanded   map[string]bool
	Form         *FormWithResult
	formOnSubmit func(result interface{}) tea.Cmd
	pickerHost   string
	// Pending overlays a transient state label on a slot row while a
	// long-running action is in flight. Key: "<host>:<slot>". Value: label
	// e.g. "deleting". Cleared either by the next poll (when the slot row
	// vanishes) or by onActionResult when the underlying command finishes.
	Pending map[string]string
	// PendingPools synthesizes a phantom repo+slot row during pool creation
	// so the user sees something happening between form submit and the first
	// poll that picks up the new pool. Key: "<host>|<repo>" (pipe separator
	// chosen because GitHub repos can contain colons in the future API but
	// never pipes). Value: label (currently always "creating"). Cleared in
	// hostsTickMsg when the host's state reports a slot for the repo, and in
	// onActionResult on failure (since no poll will ever reflect a failed add).
	PendingPools map[string]string
	StatusLog    *ringBuffer
	Flash     *flash
	Paused    bool
	Width     int
	Height    int
}

// New builds a fresh Model from the inventory + ghapp client + inventory path.
// All hosts start expanded.
func New(inv *inventory.Inventory, c *ghapp.Client, inventoryPath string) Model {
	expanded := make(map[string]bool, len(inv.Hosts))
	for name := range inv.Hosts {
		expanded[name] = true
	}
	return Model{
		Inv:           inv,
		Client:        c,
		Mux:           poller.NewSSHMux(),
		Hosts:         map[string]*poller.HostState{},
		Runners:       map[string]*poller.RepoRunners{},
		Runs:          map[string]*poller.RepoRuns{},
		Errs:          map[string]string{},
		Expanded:      expanded,
		Pending:       map[string]string{},
		PendingPools:  map[string]string{},
		StatusLog:     newRingBuffer(5),
		InventoryPath: inventoryPath,
	}
}

// openForm activates a huh form overlay. It stores the form + callback,
// then fires the form's Init() so huh dispatches its first internal cmds.
func (m Model) openForm(fwr FormWithResult, onSubmit func(result interface{}) tea.Cmd) (Model, tea.Cmd) {
	m.Form = &fwr
	m.formOnSubmit = onSubmit
	return m, m.Form.Form.Init()
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		startHostsPoller(m),
		startRunnersPoller(m),
		startRunsPoller(m),
	)
}

// ===== Poller cmd plumbing =====

func startHostsPoller(m Model) tea.Cmd {
	targets := make(map[string]string, len(m.Inv.Hosts))
	for name, h := range m.Inv.Hosts {
		targets[name] = h.SSH
	}
	ch := make(chan poller.HostsMsg, 32)
	go poller.HostsPoller(programCtx(), m.Mux, targets, hostsInterval, ch)
	return waitForHostsMsg(ch)
}

func startRunnersPoller(m Model) tea.Cmd {
	if m.Client == nil {
		return nil
	}
	repos := poolRepos(m.Inv)
	ch := make(chan poller.RunnersMsg, 32)
	go poller.RunnersPoller(programCtx(), m.Client, repos, runnersInterval, ch)
	return waitForRunnersMsg(ch)
}

func startRunsPoller(m Model) tea.Cmd {
	if m.Client == nil {
		return nil
	}
	repos := poolRepos(m.Inv)
	ch := make(chan poller.RunsMsg, 32)
	go poller.RunsPoller(programCtx(), m.Client, repos, runsInterval, ch)
	return waitForRunsMsg(ch)
}

func waitForHostsMsg(ch chan poller.HostsMsg) tea.Cmd {
	return func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return nil
		}
		return hostsTickMsg{M: m, Ch: ch}
	}
}
func waitForRunnersMsg(ch chan poller.RunnersMsg) tea.Cmd {
	return func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return nil
		}
		return runnersTickMsg{M: m, Ch: ch}
	}
}
func waitForRunsMsg(ch chan poller.RunsMsg) tea.Cmd {
	return func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return nil
		}
		return runsTickMsg{M: m, Ch: ch}
	}
}

type hostsTickMsg struct {
	M  poller.HostsMsg
	Ch chan poller.HostsMsg
}
type runnersTickMsg struct {
	M  poller.RunnersMsg
	Ch chan poller.RunnersMsg
}
type runsTickMsg struct {
	M  poller.RunsMsg
	Ch chan poller.RunsMsg
}

func poolRepos(inv *inventory.Inventory) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range inv.Pools {
		if !seen[p.Repo] {
			seen[p.Repo] = true
			out = append(out, p.Repo)
		}
	}
	return out
}

// ===== Program context =====

var pCtx context.Context

func programCtx() context.Context {
	if pCtx == nil {
		pCtx = context.Background()
	}
	return pCtx
}

// SetContext is called by the TUI subcommand entry point before starting the
// Bubbletea program. Allows external lifecycle to cancel pollers.
func SetContext(ctx context.Context) { pCtx = ctx }

// ===== Internal helpers (ring buffer + flash) =====

type ringBuffer struct {
	cap   int
	lines []string
}

func newRingBuffer(cap int) *ringBuffer { return &ringBuffer{cap: cap} }

func (r *ringBuffer) Push(s string) {
	r.lines = append(r.lines, s)
	if len(r.lines) > r.cap {
		r.lines = r.lines[len(r.lines)-r.cap:]
	}
}

func (r *ringBuffer) Lines() []string { return append([]string(nil), r.lines...) }

type flash struct {
	Text    string
	IsError bool
	Until   time.Time
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Application-specific messages (pollers, action results, window resize)
	// always run through our handlers, even when a huh form is open —
	// otherwise polls would stop while a modal was up. We do this BEFORE
	// the form-routing block.
	switch v := msg.(type) {
	case forceRedrawMsg:
		return m, nil
	case tea.WindowSizeMsg:
		m.Width, m.Height = v.Width, v.Height
		// don't return — let the form see resize too if it's open
	case hostsTickMsg:
		// Snapshot cursor's vertical position before the state change so we
		// can snap to the same spot if the cursored row disappears.
		oldIdx := CursorIndex(m.Cursor, m.Hosts, m.Expanded)
		if v.M.Err != nil {
			m.Errs["hosts/"+v.M.Host] = v.M.Err.Error()
		} else {
			delete(m.Errs, "hosts/"+v.M.Host)
			m.Hosts[v.M.Host] = v.M.State
		}
		// Clear pending overlays whose slot no longer exists in state — the
		// "deleting" label was a transient hint and the row is gone.
		for key := range m.Pending {
			h, n, ok := parsePendingKey(key)
			if !ok {
				delete(m.Pending, key)
				continue
			}
			hs := m.Hosts[h]
			if hs == nil {
				continue
			}
			if _, exists := hs.Slots[n]; !exists {
				delete(m.Pending, key)
			}
		}
		// Clear PendingPools entries whose repo now appears in state — the
		// "creating" phantom row has been superseded by the real slot row.
		for key := range m.PendingPools {
			parts := strings.SplitN(key, "|", 2)
			if len(parts) != 2 {
				delete(m.PendingPools, key)
				continue
			}
			h, repo := parts[0], parts[1]
			hs := m.Hosts[h]
			if hs == nil {
				continue
			}
			for _, s := range hs.Slots {
				if s.Repo == repo {
					delete(m.PendingPools, key)
					break
				}
			}
		}
		if m.Cursor.Host == "" && len(m.Hosts) > 0 {
			m.Cursor = FirstCursor(m.Hosts, m.Expanded)
		} else {
			m.Cursor = EnsureCursorValid(m.Cursor, m.Hosts, m.Expanded, oldIdx)
		}
		return m, waitForHostsMsg(v.Ch)
	case runnersTickMsg:
		if v.M.Err != nil {
			m.Errs["runners/"+v.M.Repo] = v.M.Err.Error()
		} else {
			delete(m.Errs, "runners/"+v.M.Repo)
			m.Runners[v.M.Repo] = v.M.State
		}
		return m, waitForRunnersMsg(v.Ch)
	case runsTickMsg:
		if v.M.Err != nil {
			m.Errs["runs/"+v.M.Repo] = v.M.Err.Error()
		} else {
			delete(m.Errs, "runs/"+v.M.Repo)
			m.Runs[v.M.Repo] = v.M.State
		}
		return m, waitForRunsMsg(v.Ch)
	case ActionLogMsg:
		return m.onActionLog(v)
	case ActionResultMsg:
		return m.onActionResult(v)
	case AccessibleReposLoadedMsg:
		if v.Err != nil {
			m.Flash = &flash{Text: "list repos failed: " + v.Err.Error(), IsError: true, Until: time.Now().Add(5 * time.Second)}
			return m, nil
		}
		host := m.pickerHost
		if host == "" {
			return m, nil
		}
		fwr := NewMultiSelectForm(
			"Pools on "+host,
			"Pick repos to add a slot for. Existing pools scale up; new ones get created. (x to toggle, / to filter, enter to submit)",
			v.Repos,
		)
		invPath := m.InventoryPath
		hostStates := m.Hosts
		pending := m.PendingPools
		return m.openForm(fwr, func(result interface{}) tea.Cmd {
			picked, _ := result.([]string)
			if len(picked) == 0 {
				return nil
			}
			for _, repo := range picked {
				pending[host+"|"+repo] = "creating"
			}
			return AddPoolsCmd(invPath, host, picked, hostStates)
		})
	}

	// If a form is active, route remaining msgs (KeyMsg, huh-internal,
	// resize) to it. huh emits its own internal messages between Enter and
	// the State transition — only forwarding KeyMsg leaves it stuck.
	if m.Form != nil {
		newForm, cmd := m.Form.Form.Update(msg)
		if f, ok := newForm.(*huh.Form); ok {
			m.Form.Form = f
		}
		switch m.Form.Form.State {
		case huh.StateCompleted:
			result := m.Form.Result()
			cb := m.formOnSubmit
			m.Form = nil
			m.formOnSubmit = nil
			var actionCmd tea.Cmd
			if cb != nil {
				actionCmd = cb(result)
			}
			if actionCmd != nil {
				return m, tea.Batch(actionCmd, tea.ClearScreen)
			}
			return m, tea.ClearScreen
		case huh.StateAborted:
			m.Form = nil
			m.formOnSubmit = nil
			return m, tea.ClearScreen
		default:
			return m, cmd
		}
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		return m.handleKey(km)
	}
	return m, nil
}

func (m Model) View() string {
	return m.renderView()
}

