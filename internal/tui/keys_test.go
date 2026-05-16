// internal/tui/keys_test.go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/tui/poller"
	"github.com/stretchr/testify/require"
)

func modelWithTwoHosts(t *testing.T) Model {
	t.Helper()
	inv := &inventory.Inventory{
		Hosts: map[string]inventory.Host{
			"h1": {SSH: "bobsled@h1", Capacity: 4},
			"h2": {SSH: "bobsled@h2", Capacity: 4},
		},
	}
	m := New(inv, nil, "inventory.yaml")
	// Slots need a Repo to render — rows.go filters out orphan (no-repo)
	// slots, so without it the j-from-host cursor has no child to descend to.
	m.Hosts["h1"] = &poller.HostState{Name: "h1", Slots: map[int]poller.SlotState{
		1: {N: 1, Repo: "acme/foo"},
		2: {N: 2, Repo: "acme/foo"},
	}}
	m.Hosts["h2"] = &poller.HostState{Name: "h2", Slots: map[int]poller.SlotState{
		1: {N: 1, Repo: "acme/foo"},
	}}
	m.Cursor = FirstCursor(m.Hosts, m.Expanded)
	return m
}

func TestKey_J_MovesCursorDown(t *testing.T) {
	m := modelWithTwoHosts(t)
	require.Equal(t, "h1", m.Cursor.Host)
	require.Equal(t, CursorHost, m.Cursor.Kind)
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	mm := mNew.(Model)
	// With 3-level tree, j from a host moves to the first repo group, not a slot.
	require.Equal(t, CursorRepo, mm.Cursor.Kind)
	require.Equal(t, "h1", mm.Cursor.Host)
}

func TestKey_K_MovesCursorUp(t *testing.T) {
	m := modelWithTwoHosts(t)
	m.Cursor = Cursor{Host: "h2", Kind: CursorHost}
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	mm := mNew.(Model)
	require.Equal(t, "h1", mm.Cursor.Host)
}

func TestKey_Enter_TogglesExpand(t *testing.T) {
	m := modelWithTwoHosts(t)
	require.True(t, m.Expanded["h1"])
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.False(t, mNew.(Model).Expanded["h1"])
}

func TestKey_Q_Quits(t *testing.T) {
	m := modelWithTwoHosts(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd, "q must return tea.Quit")
}

func TestKey_D_OpensForm(t *testing.T) {
	m := modelWithTwoHosts(t)
	m.Cursor = Cursor{Host: "h1", Kind: CursorSlot, Slot: 1}
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Form, "d on a slot should open a huh form")
	require.NotNil(t, mm.Form.Form, "form field must be set")
}

func TestKey_R_FlashesRefreshMessage(t *testing.T) {
	m := modelWithTwoHosts(t)
	m.Cursor = Cursor{Host: "h1", Kind: CursorHost}
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Flash)
	require.Contains(t, mm.Flash.Text, "refresh")
}

func TestKey_Question_OpensForm(t *testing.T) {
	m := modelWithTwoHosts(t)
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Form, "? should open a huh form for keybindings help")
}

// Capital-D was the dedicated "remove host" key. It's gone — `d` is now
// context-sensitive and unifies drain+remove across host / pool / slot rows
// (see TestContextualActions_HostRowAllDisabled and TestKey_D_OpensForm).

func TestKey_G_OpensForm(t *testing.T) {
	m := modelWithTwoHosts(t)
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Form, "g should open a huh confirm form for GC")
}

func TestKey_A_NoClientFlashes(t *testing.T) {
	// modelWithTwoHosts passes nil client; 'a' should flash a clear error.
	m := modelWithTwoHosts(t)
	m.Cursor = Cursor{Host: "h1", Kind: CursorHost}
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Flash)
	require.Contains(t, mm.Flash.Text, "No GitHub client")
}

func TestKey_A_OnSlotScalesUp(t *testing.T) {
	// On a slot row with a repo, 'a' scales up directly (no form needed).
	m := modelWithTwoHosts(t)
	m.Hosts["h1"].Slots[1] = poller.SlotState{N: 1, Repo: "acme/foo"}
	m.Cursor = Cursor{Host: "h1", Kind: CursorSlot, Slot: 1, Repo: "acme/foo"}
	mNew, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	mm := mNew.(Model)
	// Direct scale: cmd should be non-nil and no form opened.
	require.Nil(t, mm.Form, "slot row 'a' dispatches ScaleCmd directly, no form")
	require.NotNil(t, cmd, "ScaleCmd should be returned")
	require.NotNil(t, mm.Flash)
}

// Capital-P was the dedicated "remove pool" key. It's gone — `d` on a repo
// row now drains+removes the pool, matching the unified context-sensitive
// keymap (covered by TestContextualActions_*).

func TestKey_Enter_TogglesRepoExpand(t *testing.T) {
	m := modelWithTwoHosts(t)
	// Ensure slots have repos so repo groups exist.
	m.Hosts["h1"].Slots[1] = poller.SlotState{N: 1, Repo: "acme/foo"}
	m.Hosts["h1"].Slots[2] = poller.SlotState{N: 2, Repo: "acme/foo"}
	m.Cursor = Cursor{Host: "h1", Repo: "acme/foo", Kind: CursorRepo}

	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := mNew.(Model)
	key := repoExpandKey("h1", "acme/foo")
	require.Equal(t, false, mm.Expanded[key], "first Enter collapses the repo group (default was expanded)")

	mNew, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = mNew.(Model)
	require.Equal(t, true, mm.Expanded[key], "second Enter re-expands")
}

// Lowercase `p` was the text-input "add pool by name" key. It's gone — the
// `a` picker now includes a `+ enter repo name manually` option at the top
// that opens the input form when selected, so the single-entry path lives
// under `a` (see openManualAddMsg in model.go).

