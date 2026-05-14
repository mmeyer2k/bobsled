// internal/tui/picker_test.go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestPicker_FiltersAsYouType(t *testing.T) {
	p := NewPicker("Add", "Pick", []string{"acme/foo", "acme/bar", "u/baz"}, nil)
	p, _ = p.OnKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	p, _ = p.OnKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	got := p.filtered()
	require.Equal(t, []string{"acme/bar", "acme/foo"}, got)
}

func TestPicker_SpaceToggles(t *testing.T) {
	p := NewPicker("Add", "Pick", []string{"a", "b"}, nil)
	p, _ = p.OnKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	require.True(t, p.Selected["a"])
	p, _ = p.OnKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	require.False(t, p.Selected["a"])
}

func TestPicker_EnterSubmits(t *testing.T) {
	p := NewPicker("Add", "Pick", []string{"x"}, nil)
	p, _ = p.OnKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	_, outcome := p.OnKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.Equal(t, PickerSubmitted, outcome)
}

func TestPicker_EscCancels(t *testing.T) {
	p := NewPicker("Add", "Pick", []string{"x"}, nil)
	_, outcome := p.OnKey(tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, PickerCancelled, outcome)
}
