// internal/tui/model.go
package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/tui/poller"
)

const (
	hostsInterval   = 2 * time.Second
	runnersInterval = 3 * time.Second
	runsInterval    = 15 * time.Second
)

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
	StatusLog *ringBuffer
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
	switch v := msg.(type) {
	case tea.KeyMsg:
		if m.Form != nil {
			newForm, cmd := m.Form.Form.Update(v)
			m.Form.Form = newForm.(*huh.Form)
			if m.Form.Form.State == huh.StateCompleted {
				result := m.Form.Result()
				cb := m.formOnSubmit
				m.Form = nil
				m.formOnSubmit = nil
				if cb != nil {
					if next := cb(result); next != nil {
						return m, tea.Batch(cmd, next)
					}
				}
				return m, cmd
			}
			if m.Form.Form.State == huh.StateAborted {
				m.Form = nil
				m.formOnSubmit = nil
				return m, cmd
			}
			return m, cmd
		}
		return m.handleKey(v)
	case tea.WindowSizeMsg:
		m.Width, m.Height = v.Width, v.Height
		return m, nil

	case hostsTickMsg:
		if v.M.Err != nil {
			m.Errs["hosts/"+v.M.Host] = v.M.Err.Error()
		} else {
			delete(m.Errs, "hosts/"+v.M.Host)
			m.Hosts[v.M.Host] = v.M.State
		}
		// First valid tick: park the cursor on the first host.
		if m.Cursor.Host == "" && len(m.Hosts) > 0 {
			m.Cursor = FirstCursor(m.Hosts, m.Expanded)
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
		return m.openForm(fwr, func(result interface{}) tea.Cmd {
			picked, _ := result.([]string)
			if len(picked) == 0 {
				return nil
			}
			return AddPoolsCmd(invPath, host, picked, hostStates)
		})
	default:
		if m.Form != nil {
			newForm, cmd := m.Form.Form.Update(msg)
			m.Form.Form = newForm.(*huh.Form)
			return m, cmd
		}
		return m, nil
	}
}

func (m Model) View() string {
	return m.renderView()
}

