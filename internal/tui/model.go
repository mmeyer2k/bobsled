// internal/tui/model.go
package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

	Cursor    Cursor
	Expanded  map[string]bool
	Modal     *Modal
	Inline    *InlinePrompt
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

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		startHostsPoller(m),
		startRunnersPoller(m),
		startRunsPoller(m),
	)
}

// ===== Poller cmd plumbing =====

func startHostsPoller(m Model) tea.Cmd {
	targets := make([]string, 0, len(m.Inv.Hosts))
	for _, h := range m.Inv.Hosts {
		targets = append(targets, h.SSH)
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

// Update and View are added in later tasks. For now Update is a no-op so the
// type satisfies tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m Model) View() string {
	return "tui: loading…"
}

// Modal + InlinePrompt are stubs filled out in later tasks.
type Modal struct{}
type InlinePrompt struct{}
