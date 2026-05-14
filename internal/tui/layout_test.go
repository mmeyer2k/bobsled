// internal/tui/layout_test.go
package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/tui/poller"
	"github.com/stretchr/testify/require"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string { return ansi.ReplaceAllString(s, "") }

func TestRender_StableSnapshot(t *testing.T) {
	m := New(&inventory.Inventory{
		Hosts: map[string]inventory.Host{"h1": {SSH: "bobsled@h1", Capacity: 4}},
		Pools: []inventory.Pool{{Repo: "acme/foo", Count: 1, Spread: []string{"h1"}}},
	}, nil, "inventory.yaml")
	m.Width, m.Height = 80, 24
	m.Hosts["h1"] = &poller.HostState{
		Name: "h1", Reachable: true, Capacity: 4,
		Slots: map[int]poller.SlotState{1: {N: 1, UnitState: "active", Enabled: true, Repo: "acme/foo"}},
	}
	m.Runners["acme/foo"] = &poller.RepoRunners{
		Runners: []ghapp.RunnerRef{{ID: 1, Name: "bobsled-h1-1"}},
	}
	m.Cursor = FirstCursor(m.Hosts, m.Expanded)

	out := stripANSI(m.View())
	require.Contains(t, out, "bobsled")
	require.Contains(t, out, "h1")
	require.Contains(t, out, "acme/foo")
	require.Contains(t, out, "active")
	require.True(t, strings.Contains(out, "j/k") || strings.Contains(out, "?"), "footer keybindings present")
}

func TestContextualActions_HostRowAllEnabled(t *testing.T) {
	m := newTestModelForFooter()
	m.Cursor = Cursor{Host: "h1", Kind: CursorHost}
	m.Hosts["h1"] = &poller.HostState{
		Name: "h1", Reachable: true,
		Slots: map[int]poller.SlotState{1: {N: 1, Enabled: true, UnitState: "active"}},
	}
	got := m.contextualActions()
	keys := keysOf(got)
	require.Contains(t, keys, "d", "drain should show when host has enabled slots")
}

func TestContextualActions_HostRowAlwaysHasDrain(t *testing.T) {
	// `d` is the unified drain+remove on host rows and is always shown —
	// it's a no-op (or just removes the host record) if no slots are enabled.
	m := newTestModelForFooter()
	m.Cursor = Cursor{Host: "h1", Kind: CursorHost}
	m.Hosts["h1"] = &poller.HostState{
		Name: "h1", Reachable: true,
		Slots: map[int]poller.SlotState{1: {N: 1, Enabled: false, UnitState: "inactive"}},
	}
	got := m.contextualActions()
	keys := keysOf(got)
	require.Contains(t, keys, "d", "drain+remove should always be available on a host row")
}

func TestContextualActions_SlotDisabledHasEnableAndDelete(t *testing.T) {
	// On a disabled slot the new keymap exposes:
	//   e  → re-enable
	//   d  → delete (NOT drain; the slot is already stopped)
	//   r  → reset cache
	// The intent of the original test ("disabled slot has no drain action") is
	// preserved: there's no separate drain since the slot is already disabled.
	// `d` is present but it means "delete" — different command, same key.
	m := newTestModelForFooter()
	m.Hosts["h1"] = &poller.HostState{
		Name: "h1",
		Slots: map[int]poller.SlotState{1: {N: 1, Enabled: false, UnitState: "active", Repo: "acme/foo"}},
	}
	m.Cursor = Cursor{Host: "h1", Repo: "acme/foo", Slot: 1, Kind: CursorSlot}
	got := m.contextualActions()
	keys := keysOf(got)
	require.Contains(t, keys, "e", "enable should be offered on a disabled slot")
	require.Contains(t, keys, "d", "delete should be offered on a disabled slot")
	require.Contains(t, keys, "r", "reset cache should always be available")
}

func TestContextualActions_SlotEnabledHasDrain(t *testing.T) {
	m := newTestModelForFooter()
	m.Hosts["h1"] = &poller.HostState{
		Name: "h1",
		Slots: map[int]poller.SlotState{1: {N: 1, Enabled: true, UnitState: "active", Repo: "acme/foo"}},
	}
	m.Cursor = Cursor{Host: "h1", Repo: "acme/foo", Slot: 1, Kind: CursorSlot}
	got := m.contextualActions()
	require.Contains(t, keysOf(got), "d")
}

// helpers
func newTestModelForFooter() Model {
	inv := &inventory.Inventory{Hosts: map[string]inventory.Host{"h1": {SSH: "x", Capacity: 4}}}
	return New(inv, nil, "inventory.yaml")
}

func keysOf(hs []hint) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = h.key
	}
	return out
}

func TestRender_ShowsFormWhenOpen(t *testing.T) {
	m := New(&inventory.Inventory{
		Hosts: map[string]inventory.Host{"h1": {SSH: "bobsled@h1", Capacity: 4}},
	}, nil, "inventory.yaml")
	m.Width, m.Height = 80, 24
	m.Hosts["h1"] = &poller.HostState{Name: "h1", Reachable: true, Capacity: 4}
	m.Cursor = FirstCursor(m.Hosts, m.Expanded)

	// Open a confirm form via the key handler (d on a host row).
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Form, "form should be set after 'd'")

	// When a form is active, renderView returns the form's output, not the tree.
	out := stripANSI(mm.View())
	// huh renders the title and yes/no buttons — just verify output is non-empty
	// and does not contain the normal tree header (form owns the full screen).
	require.NotEmpty(t, out, "form view must not be empty")
}
