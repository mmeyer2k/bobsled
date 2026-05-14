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
	m.Hosts["h1"] = &poller.HostState{Name: "h1", Slots: map[int]poller.SlotState{1: {N: 1}, 2: {N: 2}}}
	m.Hosts["h2"] = &poller.HostState{Name: "h2", Slots: map[int]poller.SlotState{1: {N: 1}}}
	m.Cursor = FirstCursor(m.Hosts, m.Expanded)
	return m
}

func TestKey_J_MovesCursorDown(t *testing.T) {
	m := modelWithTwoHosts(t)
	require.Equal(t, "h1", m.Cursor.Host)
	require.Equal(t, CursorHost, m.Cursor.Kind)
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	mm := mNew.(Model)
	require.Equal(t, CursorSlot, mm.Cursor.Kind)
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

func TestKey_D_OpensDrainSlotModal(t *testing.T) {
	m := modelWithTwoHosts(t)
	m.Cursor = Cursor{Host: "h1", Kind: CursorSlot, Slot: 1}
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Modal)
	require.Contains(t, mm.Modal.Title, "Drain slot")
}

func TestKey_RoutedToModalWhenOpen(t *testing.T) {
	m := modelWithTwoHosts(t)
	m.Cursor = Cursor{Host: "h1", Kind: CursorSlot, Slot: 1}
	m, _ = updateOnce(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m, _ = updateOnce(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m, _ = updateOnce(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m, _ = updateOnce(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	require.True(t, m.Modal.ReadyToConfirm())
}

func updateOnce(m Model, msg tea.Msg) (Model, tea.Cmd) {
	r, c := m.Update(msg)
	return r.(Model), c
}

func TestKey_R_FlashesRefreshMessage(t *testing.T) {
	m := modelWithTwoHosts(t)
	m.Cursor = Cursor{Host: "h1", Kind: CursorHost}
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Flash)
	require.Contains(t, mm.Flash.Text, "refresh")
}

func TestKey_Question_OpensHelpModal(t *testing.T) {
	m := modelWithTwoHosts(t)
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Modal)
	require.Contains(t, mm.Modal.Title, "Keybindings")
}

func TestKey_CapitalD_OnSlotFlashes(t *testing.T) {
	m := modelWithTwoHosts(t)
	m.Cursor = Cursor{Host: "h1", Kind: CursorSlot, Slot: 1}
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Flash)
	require.Contains(t, mm.Flash.Text, "host")
}

func TestKey_CapitalD_OnHostOpensRemoveModal(t *testing.T) {
	m := modelWithTwoHosts(t)
	m.Cursor = Cursor{Host: "h1", Kind: CursorHost}
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Modal)
	require.Contains(t, mm.Modal.Title, "Remove host")
}

func TestKey_G_OpensGCModal(t *testing.T) {
	m := modelWithTwoHosts(t)
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Modal)
	require.Contains(t, mm.Modal.Title, "GC")
}

func TestKey_A_OnHostRowFlashes(t *testing.T) {
	m := modelWithTwoHosts(t)
	m.Cursor = Cursor{Host: "h1", Kind: CursorHost}
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Flash)
	require.Contains(t, mm.Flash.Text, "slot row")
}

func TestKey_A_OnSlotInvokesScale(t *testing.T) {
	m := modelWithTwoHosts(t)
	// Set up a slot with an assigned repo so the scale path runs.
	m.Hosts["h1"].Slots[1] = poller.SlotState{N: 1, Repo: "acme/foo"}
	m.Cursor = Cursor{Host: "h1", Kind: CursorSlot, Slot: 1}
	mNew, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	mm := mNew.(Model)
	require.NotNil(t, cmd, "should dispatch a scale command")
	require.NotNil(t, mm.Flash)
	require.Contains(t, mm.Flash.Text, "scaling")
}

func TestKey_P_OnSlotOpensRemovePoolModal(t *testing.T) {
	m := modelWithTwoHosts(t)
	m.Hosts["h1"].Slots[1] = poller.SlotState{N: 1, Repo: "acme/foo"}
	m.Cursor = Cursor{Host: "h1", Kind: CursorSlot, Slot: 1}
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Modal)
	require.Contains(t, mm.Modal.Title, "Remove pool acme/foo")
}

func TestKey_P_OnHostFlashes(t *testing.T) {
	m := modelWithTwoHosts(t)
	m.Cursor = Cursor{Host: "h1", Kind: CursorHost}
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Flash)
	require.Contains(t, mm.Flash.Text, "slot row")
}

func TestKey_LowercaseP_FlashesAddHint(t *testing.T) {
	m := modelWithTwoHosts(t)
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Flash)
	require.Contains(t, mm.Flash.Text, "repo add")
}
