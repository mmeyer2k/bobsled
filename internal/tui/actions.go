// internal/tui/actions.go
package tui

import (
	"context"
	"fmt"
	"os"
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

// HostRemoveCmd runs `bobsled host remove <host>` (no --wipe by default).
func HostRemoveCmd(inventoryPath, host string) tea.Cmd {
	desc := fmt.Sprintf("remove host %s", host)
	return runAction(desc, inventoryPath, "host", "remove", host)
}

// CacheResetSlotCmd resets a single slot's cache.
func CacheResetSlotCmd(inventoryPath, host string, slot int) tea.Cmd {
	desc := fmt.Sprintf("cache reset %s slot %d", host, slot)
	return runAction(desc, inventoryPath, "cache", "reset", "--host", host, "--slot", fmt.Sprint(slot))
}

// CacheResetHostCmd resets every slot's cache on the host.
func CacheResetHostCmd(inventoryPath, host string) tea.Cmd {
	desc := fmt.Sprintf("cache reset host %s", host)
	return runAction(desc, inventoryPath, "cache", "reset", "--host", host)
}

// GCCmd runs `bobsled gc` to delete orphan GitHub-side runners.
func GCCmd(inventoryPath string) tea.Cmd {
	return runAction("gc orphan runners", inventoryPath, "gc")
}

// RepoRemoveCmd runs `bobsled repo remove <owner/name>`.
func RepoRemoveCmd(inventoryPath, repo string) tea.Cmd {
	desc := fmt.Sprintf("remove pool %s", repo)
	return runAction(desc, inventoryPath, "repo", "remove", repo)
}

// ScaleCmd shells out to `bobsled scale --host h --repo r --count N`.
func ScaleCmd(inventoryPath, host, repo string, count int) tea.Cmd {
	desc := fmt.Sprintf("scale %s on %s to %d", repo, host, count)
	return runAction(desc, inventoryPath, "scale", "--host", host, "--repo", repo, "--count", fmt.Sprint(count))
}

func runAction(description, inventory string, args ...string) tea.Cmd {
	return func() tea.Msg {
		exe, err := os.Executable()
		if err != nil {
			exe = "bobsled"
		}
		full := append([]string{"--inventory", inventory}, args...)
		cmd := exec.CommandContext(context.Background(), exe, full...)
		out, runErr := cmd.CombinedOutput()
		if runErr != nil && len(out) > 0 {
			runErr = fmt.Errorf("%w: %s", runErr, string(out))
		}
		return ActionResultMsg{Description: description, Err: runErr}
	}
}
