// internal/tui/keys.go
package tui

import tea "github.com/charmbracelet/bubbletea"

// handleKey returns the updated model and an optional cmd. Keys that touch
// only model state (nav, toggle expand) return nil cmd.
func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEnter:
		if m.Cursor.Kind == CursorHost {
			m.Expanded[m.Cursor.Host] = !m.Expanded[m.Cursor.Host]
		}
		return m, nil
	case tea.KeyUp:
		m.Cursor = PrevCursor(m.Cursor, m.Hosts, m.Expanded)
		return m, nil
	case tea.KeyDown:
		m.Cursor = NextCursor(m.Cursor, m.Hosts, m.Expanded)
		return m, nil
	}
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 {
		return m, nil
	}
	switch msg.Runes[0] {
	case 'q':
		return m, tea.Quit
	case 'j':
		m.Cursor = NextCursor(m.Cursor, m.Hosts, m.Expanded)
	case 'k':
		m.Cursor = PrevCursor(m.Cursor, m.Hosts, m.Expanded)
	}
	return m, nil
}
