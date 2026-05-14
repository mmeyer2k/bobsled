// internal/tui/actions.go
package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/tui/poller"
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

// RepoAddCmd runs `bobsled repo add <owner/name> --host <h> --count 1`.
func RepoAddCmd(inventoryPath, repo, host string, count int) tea.Cmd {
	desc := fmt.Sprintf("add pool %s on %s (count=%d)", repo, host, count)
	return runAction(desc, inventoryPath, "repo", "add", repo, "--host", host, "--count", fmt.Sprint(count))
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

// AccessibleReposLoadedMsg carries the result of ListAccessibleRepos.
type AccessibleReposLoadedMsg struct {
	Repos []string
	Err   error
}

// LoadAccessibleReposCmd kicks off the API call to list every repo the App
// can manage.
func LoadAccessibleReposCmd(client *ghapp.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return AccessibleReposLoadedMsg{Err: fmt.Errorf("no ghapp client")}
		}
		repos, err := client.ListAccessibleRepos(context.Background())
		return AccessibleReposLoadedMsg{Repos: repos, Err: err}
	}
}

// AddPoolsCmd handles each picked repo: scale up if present, repo-add if new.
func AddPoolsCmd(inventoryPath, host string, repos []string, hostStates map[string]*poller.HostState) tea.Cmd {
	return func() tea.Msg {
		for _, repo := range repos {
			currentOnHost := 0
			if h := hostStates[host]; h != nil {
				for _, s := range h.Slots {
					if s.Repo == repo {
						currentOnHost++
					}
				}
			}
			if currentOnHost > 0 {
				// existing pool on this host → scale up by 1
				if err := runActionSync(inventoryPath, "scale", "--host", host, "--repo", repo, "--count", fmt.Sprint(currentOnHost+1)); err != nil {
					return ActionResultMsg{Description: "scale " + repo, Err: err}
				}
				continue
			}
			// not on this host (or new repo) → repo add (or scale if pool exists elsewhere)
			if err := runActionSync(inventoryPath, "repo", "add", repo, "--host", host, "--count", "1"); err != nil {
				// Pool may already exist on other hosts; try scale instead.
				if err2 := runActionSync(inventoryPath, "scale", "--host", host, "--repo", repo, "--count", "1"); err2 != nil {
					return ActionResultMsg{Description: "add " + repo, Err: err2}
				}
			}
		}
		return ActionResultMsg{Description: fmt.Sprintf("added/scaled %d pools on %s", len(repos), host)}
	}
}

// runActionSync invokes ./bin/bobsled <args> synchronously, returning any error.
func runActionSync(inventory string, args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		exe = "bobsled"
	}
	full := append([]string{"--inventory", inventory}, args...)
	out, err := exec.CommandContext(context.Background(), exe, full...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}
