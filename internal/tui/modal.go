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
	// OnConfirm is the "typed yes" path: invoked when Input == "yes".
	OnConfirm func() tea.Cmd
	// OnSubmit is the free-text path: invoked with Input on Enter,
	// regardless of value (as long as it's non-empty).
	OnSubmit  func(text string) tea.Cmd
	Cancelled bool
}

func NewConfirmModal(title, body string, onConfirm func() tea.Cmd) Modal {
	return Modal{Title: title, Body: body, OnConfirm: onConfirm}
}

// NewPromptModal creates a free-text input modal. OnSubmit receives the typed
// input when the user presses Enter.
func NewPromptModal(title, body string, onSubmit func(text string) tea.Cmd) Modal {
	return Modal{Title: title, Body: body, OnSubmit: onSubmit}
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

// ReadyToConfirm returns true when Enter should fire the action.
// Confirm modals require literal "yes". Prompt modals accept any non-empty
// input.
func (m Modal) ReadyToConfirm() bool {
	if m.OnSubmit != nil {
		return strings.TrimSpace(m.Input) != ""
	}
	return strings.ToLower(strings.TrimSpace(m.Input)) == "yes"
}

// Confirm dispatches the appropriate callback.
func (m Modal) Confirm() tea.Cmd {
	if !m.ReadyToConfirm() {
		return nil
	}
	if m.OnSubmit != nil {
		return m.OnSubmit(strings.TrimSpace(m.Input))
	}
	if m.OnConfirm != nil {
		return m.OnConfirm()
	}
	return nil
}

// Render returns the styled modal contents. Caller composes with View.
func (m Modal) Render(width int) string {
	prompt := "Type 'yes' to confirm: "
	if m.OnSubmit != nil {
		prompt = "> "
	}
	box := "╭── " + m.Title + " ──╮\n│\n│  " + m.Body + "\n│\n│  " + prompt + m.Input + "_\n│\n│  [⏎ submit]   [esc cancel]\n╰" + strings.Repeat("─", len(m.Title)+8) + "╯"
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
