// internal/tui/modal.go
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Modal struct {
	Title     string
	Body      string
	Input     string
	OnConfirm func() tea.Cmd
	Cancelled bool
}

func NewConfirmModal(title, body string, onConfirm func() tea.Cmd) Modal {
	return Modal{Title: title, Body: body, OnConfirm: onConfirm}
}

func (m Modal) OnKey(msg tea.KeyMsg) Modal {
	switch msg.Type {
	case tea.KeyEsc:
		m.Cancelled = true
		return m
	case tea.KeyBackspace:
		if len(m.Input) > 0 {
			m.Input = m.Input[:len(m.Input)-1]
		}
		return m
	case tea.KeyRunes:
		m.Input += string(msg.Runes)
		return m
	}
	return m
}

func (m Modal) ReadyToConfirm() bool {
	return strings.ToLower(strings.TrimSpace(m.Input)) == "yes"
}

func (m Modal) Confirm() tea.Cmd {
	if m.OnConfirm == nil || !m.ReadyToConfirm() {
		return nil
	}
	return m.OnConfirm()
}

// Render returns the styled modal contents. Caller composes with View.
func (m Modal) Render(width int) string {
	box := "╭── " + m.Title + " ──╮\n│\n│  " + m.Body + "\n│\n│  Type 'yes' to confirm: " + m.Input + "_\n│\n│  [⏎ confirm]   [esc cancel]\n╰" + strings.Repeat("─", len(m.Title)+8) + "╯"
	return box
}

// InlinePrompt is a stub for now — Phase-13 follow-up will flesh it out.
type InlinePrompt struct {
	Label    string
	Fields   []string
	Values   []string
	Focused  int
	OnSubmit func(values []string) tea.Cmd
}
