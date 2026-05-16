// internal/tui/actions.go
package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
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
		// A failed add-pool action will never produce a real row, so the
		// "creating" phantom would stick forever waiting on a poll that
		// can't help. Clear any pending-pool entry whose repo still isn't
		// in state — succeeded pending entries get cleared by the next
		// poll, so this drop is safe for failure-only cases too.
		for key := range m.PendingPools {
			parts := strings.SplitN(key, "|", 2)
			if len(parts) != 2 {
				delete(m.PendingPools, key)
				continue
			}
			h, repo := parts[0], parts[1]
			hs := m.Hosts[h]
			if hs == nil {
				delete(m.PendingPools, key)
				continue
			}
			seen := false
			for _, s := range hs.Slots {
				if s.Repo == repo {
					seen = true
					break
				}
			}
			if !seen {
				delete(m.PendingPools, key)
			}
		}
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

// SlotRemoveCmd shells out to `bobsled slot remove <host> <slot>` — the
// graceful, index-preserving per-slot deletion path. Unlike ScaleCmd, this
// removes the *specific* slot index (not "the highest slot" after dense
// renumbering).
func SlotRemoveCmd(inventoryPath, host string, slot int) tea.Cmd {
	desc := fmt.Sprintf("remove slot %d on %s", slot, host)
	return runAction(desc, inventoryPath, "slot", "remove", host, fmt.Sprint(slot))
}

// SlotEnableCmd shells out to `bobsled slot enable <host> <slot>` — re-arm a
// previously disabled slot (re-create the default.target.wants symlink and
// start the unit, which re-runs mint to get a fresh JIT config).
func SlotEnableCmd(inventoryPath, host string, slot int) tea.Cmd {
	desc := fmt.Sprintf("enable slot %d on %s", slot, host)
	return runAction(desc, inventoryPath, "slot", "enable", host, fmt.Sprint(slot))
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

// DrainRepoCmd drains every slot on `host` that serves `repo` by issuing one
// drain --host h --slot N per matching slot.
func DrainRepoCmd(inventoryPath, host, repo string, hostStates map[string]*poller.HostState) tea.Cmd {
	return func() tea.Msg {
		hs := hostStates[host]
		if hs == nil {
			return ActionResultMsg{Description: "drain pool " + repo, Err: fmt.Errorf("no host state for %s", host)}
		}
		for n, s := range hs.Slots {
			if s.Repo != repo {
				continue
			}
			if err := runActionSync(inventoryPath, "drain", "--host", host, "--slot", fmt.Sprint(n)); err != nil {
				return ActionResultMsg{Description: fmt.Sprintf("drain %s slot %d", host, n), Err: err}
			}
		}
		return ActionResultMsg{Description: fmt.Sprintf("drained pool %s on %s", repo, host)}
	}
}

// CacheResetRepoCmd resets the cache for every slot on `host` that serves `repo`.
func CacheResetRepoCmd(inventoryPath, host, repo string, hostStates map[string]*poller.HostState) tea.Cmd {
	return func() tea.Msg {
		hs := hostStates[host]
		if hs == nil {
			return ActionResultMsg{Description: "reset cache " + repo, Err: fmt.Errorf("no host state for %s", host)}
		}
		for n, s := range hs.Slots {
			if s.Repo != repo {
				continue
			}
			if err := runActionSync(inventoryPath, "cache", "reset", "--host", host, "--slot", fmt.Sprint(n)); err != nil {
				return ActionResultMsg{Description: fmt.Sprintf("reset %s slot %d", host, n), Err: err}
			}
		}
		return ActionResultMsg{Description: fmt.Sprintf("reset caches for %s on %s", repo, host)}
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
