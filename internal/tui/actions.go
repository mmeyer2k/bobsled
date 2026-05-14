// internal/tui/actions.go
package tui

import (
	"context"
	"fmt"
	"os/exec"
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

// DrainSlotCmd runs `bobsled drain --host <h> --slot <n>` and emits
// ActionResultMsg when done.
func DrainSlotCmd(inventoryPath, host string, slot int) tea.Cmd {
	desc := fmt.Sprintf("drain %s slot %d", host, slot)
	return runAction(desc, inventoryPath, "drain", "--host", host, "--slot", fmt.Sprint(slot))
}

// DrainHostCmd runs `bobsled drain --host <h>` and emits ActionResultMsg when done.
func DrainHostCmd(inventoryPath, host string) tea.Cmd {
	desc := fmt.Sprintf("drain host %s", host)
	return runAction(desc, inventoryPath, "drain", "--host", host)
}

func runAction(description, inventory string, args ...string) tea.Cmd {
	return func() tea.Msg {
		full := append([]string{"--inventory", inventory}, args...)
		cmd := exec.CommandContext(context.Background(), "./bin/bobsled", full...)
		_, err := cmd.CombinedOutput()
		return ActionResultMsg{Description: description, Err: err}
	}
}
