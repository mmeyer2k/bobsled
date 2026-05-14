// internal/tui/keys.go
package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// handleKey returns the updated model and an optional cmd. Keys that touch
// only model state (nav, toggle expand) return nil cmd.
func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEnter:
		switch m.Cursor.Kind {
		case CursorHost:
			m.Expanded[m.Cursor.Host] = !m.Expanded[m.Cursor.Host]
		case CursorRepo:
			key := repoExpandKey(m.Cursor.Host, m.Cursor.Repo)
			// Default is expanded, so missing key + toggle → collapsed.
			if val, ok := m.Expanded[key]; ok {
				m.Expanded[key] = !val
			} else {
				m.Expanded[key] = false
			}
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
	case 'd':
		if m.Cursor.Host == "" {
			return m, nil
		}
		host := m.Cursor.Host
		var onConfirm func() tea.Cmd
		title := "Drain"
		body := "Disable matching units. In-flight jobs finish."
		switch m.Cursor.Kind {
		case CursorHost:
			title = "Drain host " + host
			body = "Disable every slot on this host."
			onConfirm = func() tea.Cmd { return DrainHostCmd(m.InventoryPath, host) }
		case CursorRepo:
			repo := m.Cursor.Repo
			title = fmt.Sprintf("Drain pool %s on %s", repo, host)
			body = "Disable every slot serving this repo on this host."
			onConfirm = func() tea.Cmd {
				return DrainRepoCmd(m.InventoryPath, host, repo, m.Hosts)
			}
		case CursorSlot:
			slot := m.Cursor.Slot
			title = fmt.Sprintf("Drain slot %d on %s", slot, host)
			onConfirm = func() tea.Cmd { return DrainSlotCmd(m.InventoryPath, host, slot) }
		}
		mod := NewConfirmModal(title, body, onConfirm)
		m.Modal = &mod
		return m, nil

	case 'D':
		if m.Cursor.Kind != CursorHost {
			m.Flash = &flash{Text: "D removes a whole host — put the cursor on a host row first.", Until: time.Now().Add(3 * time.Second)}
			return m, nil
		}
		host := m.Cursor.Host
		mod := NewConfirmModal("Remove host "+host,
			"Drain all slots, optionally gc runners, and drop from inventory.",
			func() tea.Cmd { return HostRemoveCmd(m.InventoryPath, host) })
		m.Modal = &mod
		return m, nil

	case 'r':
		if m.Cursor.Host == "" {
			return m, nil
		}
		host := m.Cursor.Host
		var onConfirm func() tea.Cmd
		title := "Reset cache"
		body := "Wipe cache. Next runs start cold."
		switch m.Cursor.Kind {
		case CursorHost:
			title = "Reset all caches on " + host
			body = "Wipe every slot's cache on this host."
			onConfirm = func() tea.Cmd { return CacheResetHostCmd(m.InventoryPath, host) }
		case CursorRepo:
			repo := m.Cursor.Repo
			title = fmt.Sprintf("Reset caches for %s on %s", repo, host)
			body = "Wipe the cache for every slot serving this repo on this host."
			onConfirm = func() tea.Cmd {
				return CacheResetRepoCmd(m.InventoryPath, host, repo, m.Hosts)
			}
		case CursorSlot:
			slot := m.Cursor.Slot
			onConfirm = func() tea.Cmd { return CacheResetSlotCmd(m.InventoryPath, host, slot) }
		}
		mod := NewConfirmModal(title, body, onConfirm)
		m.Modal = &mod
		return m, nil

	case 'g':
		mod := NewConfirmModal("GC orphan runners",
			"Delete GitHub-side runners not represented in inventory.",
			func() tea.Cmd { return GCCmd(m.InventoryPath) })
		m.Modal = &mod
		return m, nil

	case 'R':
		m.Flash = &flash{Text: "refresh queued — pollers tick every 2s anyway", Until: time.Now().Add(2 * time.Second)}
		return m, nil

	case '?':
		mod := NewConfirmModal("Keybindings",
			"j/k or ↑/↓   move\n"+
				"⏎            expand/collapse host\n"+
				"d            drain (slot or host)\n"+
				"D            remove host (drain + drop from inventory)\n"+
				"r            reset cache (slot or host)\n"+
				"g            gc orphan GitHub runners\n"+
				"a            add slot(s) — picker of App-accessible repos\n"+
				"A            add host (CLI only for v1 — use `bobsled host add`)\n"+
				"p            add pool (text prompt: owner/name on cursor's host)\n"+
				"P            remove pool (drain + drop from inventory)\n"+
				"R            refresh (pollers tick automatically)\n"+
				"?            this help\n"+
				"q / Ctrl-C   quit",
			nil)
		m.Modal = &mod
		return m, nil

	case 'p':
		if m.Cursor.Host == "" {
			m.Flash = &flash{Text: "No host under cursor — wait for the first poll.", Until: time.Now().Add(3 * time.Second)}
			return m, nil
		}
		host := m.Cursor.Host
		mod := NewPromptModal(
			"Add pool on "+host,
			"Type owner/name. Count defaults to 1; spread = this host. (The GitHub App must already be installed on the repo.)",
			func(text string) tea.Cmd {
				return RepoAddCmd(m.InventoryPath, text, host, 1)
			},
		)
		m.Modal = &mod
		return m, nil

	case 'P':
		// Determine the target repo from the cursor.
		repo := ""
		switch m.Cursor.Kind {
		case CursorRepo:
			repo = m.Cursor.Repo
		case CursorSlot:
			repo = m.Cursor.Repo
		}
		if repo == "" {
			m.Flash = &flash{Text: "Put the cursor on a repo or slot row — `P` removes that repo's pool.", Until: time.Now().Add(3 * time.Second)}
			return m, nil
		}
		capturedRepo := repo
		mod := NewConfirmModal("Remove pool "+repo,
			"Drain every slot for this repo across the fleet, gc its GitHub-side runners, and drop the pool from inventory.",
			func() tea.Cmd { return RepoRemoveCmd(m.InventoryPath, capturedRepo) })
		m.Modal = &mod
		return m, nil

	case 'a':
		if m.Cursor.Host == "" {
			m.Flash = &flash{Text: "No host under cursor — wait for the first poll.", Until: time.Now().Add(3 * time.Second)}
			return m, nil
		}
		host := m.Cursor.Host
		// On a repo or slot row, scale that exact (host, repo) up by 1.
		repo := ""
		switch m.Cursor.Kind {
		case CursorRepo:
			repo = m.Cursor.Repo
		case CursorSlot:
			repo = m.Cursor.Repo
		}
		if repo != "" {
			hostState := m.Hosts[host]
			current := 0
			if hostState != nil {
				for _, s := range hostState.Slots {
					if s.Repo == repo {
						current++
					}
				}
			}
			cmd := ScaleCmd(m.InventoryPath, host, repo, current+1)
			m.Flash = &flash{Text: fmt.Sprintf("scaling %s on %s to %d…", repo, host, current+1), Until: time.Now().Add(2 * time.Second)}
			return m, cmd
		}
		// On a host row: open the multi-select picker.
		if m.Client == nil {
			m.Flash = &flash{Text: "No GitHub client available; can't list repos.", IsError: true, Until: time.Now().Add(3 * time.Second)}
			return m, nil
		}
		m.pickerHost = host
		m.Flash = &flash{Text: "loading repos…", Until: time.Now().Add(3 * time.Second)}
		return m, LoadAccessibleReposCmd(m.Client)

	case 'A':
		m.Flash = &flash{Text: "v1: use `bobsled host add` (inline prompt coming).", Until: time.Now().Add(4 * time.Second)}
		return m, nil
	}
	return m, nil
}
