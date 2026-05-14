// internal/tui/actions.go
package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ActionLogMsg is emitted by long-running actions as they produce output.
type ActionLogMsg struct{ Line string }

// ActionResultMsg is emitted when an action finishes.
type ActionResultMsg struct {
	Description string
	Err         error
}

func (m Model) onActionLog(msg ActionLogMsg) (Model, tea.Cmd) {
	m.StatusLog.Push(msg.Line)
	return m, nil
}

func (m Model) onActionResult(msg ActionResultMsg) (Model, tea.Cmd) {
	if msg.Err != nil {
		m.Flash = &flash{Text: fmt.Sprintf("%s failed: %v", msg.Description, msg.Err), IsError: true, Until: time.Now().Add(5 * time.Second)}
	} else {
		m.Flash = &flash{Text: msg.Description + " ✓", Until: time.Now().Add(3 * time.Second)}
	}
	return m, nil
}
