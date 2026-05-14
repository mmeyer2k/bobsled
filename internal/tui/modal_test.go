// internal/tui/modal_test.go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestModal_RejectsBlankConfirm(t *testing.T) {
	mod := NewConfirmModal("Drain host h1", "Continue?", nil)
	require.False(t, mod.ReadyToConfirm())
}

func TestModal_AcceptsYes(t *testing.T) {
	called := false
	mod := NewConfirmModal("Drain host h1", "Continue?", func() tea.Cmd { called = true; return nil })
	mod = mod.OnKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	mod = mod.OnKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	mod = mod.OnKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	require.True(t, mod.ReadyToConfirm())
	_ = mod.Confirm()
	require.True(t, called)
}

func TestModal_EscapeCancels(t *testing.T) {
	mod := NewConfirmModal("Drain host h1", "Continue?", nil)
	mod = mod.OnKey(tea.KeyMsg{Type: tea.KeyEsc})
	require.True(t, mod.Cancelled)
}
