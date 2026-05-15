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
		var title, body string
		var run func() tea.Cmd
		switch m.Cursor.Kind {
		case CursorHost:
			title = "Drain host " + host + "?"
			body = "Drain every slot (idle: stop now; busy: finish current job), then remove the host from inventory."
			run = func() tea.Cmd { return HostRemoveCmd(m.InventoryPath, host) }
		case CursorRepo:
			repo := m.Cursor.Repo
			title = "Drain pool " + repo + " on " + host + "?"
			body = "Drain every slot for this repo (idle: stop now; busy: finish current job), then remove the pool from inventory."
			run = func() tea.Cmd { return RepoRemoveCmd(m.InventoryPath, repo) }
		case CursorSlot:
			slot := m.Cursor.Slot
			enabled := false
			if hs := m.Hosts[host]; hs != nil {
				enabled = hs.Slots[slot].Enabled
			}
			if enabled {
				title = fmt.Sprintf("Drain slot %d on %s?", slot, host)
				body = "Stop this slot. Idle: immediate. Busy: finish the current job, then exit. Slot stays in the pool (disabled) until deleted."
				run = func() tea.Cmd { return DrainSlotCmd(m.InventoryPath, host, slot) }
			} else {
				title = fmt.Sprintf("Delete slot %d on %s?", slot, host)
				body = "Remove this exact slot from the pool. (The slot is already stopped; this prunes its state.yaml entry and decrements the pool count.)"
				pendingKey := fmt.Sprintf("%s:%d", host, slot)
				run = func() tea.Cmd {
					m.Pending[pendingKey] = "deleting"
					return SlotRemoveCmd(m.InventoryPath, host, slot)
				}
			}
		}
		fwr := NewConfirmForm(title, body)
		return m.openForm(fwr, func(result interface{}) tea.Cmd {
			if confirmed, _ := result.(bool); confirmed {
				return run()
			}
			return nil
		})

	case 'e':
		if m.Cursor.Kind != CursorSlot {
			return m, nil
		}
		host := m.Cursor.Host
		slot := m.Cursor.Slot
		if hs := m.Hosts[host]; hs != nil && hs.Slots[slot].Enabled {
			m.Flash = &flash{Text: fmt.Sprintf("slot %d is already enabled", slot), Until: time.Now().Add(3 * time.Second)}
			return m, nil
		}
		fwr := NewConfirmForm(fmt.Sprintf("Enable slot %d on %s?", slot, host),
			"Re-arm this slot — fresh JIT config, runner re-registers, container restarts.")
		return m.openForm(fwr, func(result interface{}) tea.Cmd {
			if confirmed, _ := result.(bool); confirmed {
				return SlotEnableCmd(m.InventoryPath, host, slot)
			}
			return nil
		})

	case 'r':
		if m.Cursor.Host == "" {
			return m, nil
		}
		host := m.Cursor.Host
		var title, body string
		var run func() tea.Cmd
		switch m.Cursor.Kind {
		case CursorHost:
			title = "Reset all caches on " + host + "?"
			body = "Wipe every slot's cache on this host."
			run = func() tea.Cmd { return CacheResetHostCmd(m.InventoryPath, host) }
		case CursorRepo:
			repo := m.Cursor.Repo
			title = fmt.Sprintf("Reset caches for %s on %s?", repo, host)
			body = "Wipe the cache for every slot serving this repo on this host."
			captured := m.Hosts
			run = func() tea.Cmd { return CacheResetRepoCmd(m.InventoryPath, host, repo, captured) }
		case CursorSlot:
			slot := m.Cursor.Slot
			title = fmt.Sprintf("Reset cache for slot %d on %s?", slot, host)
			body = "Wipe this slot's cache. Next run starts cold."
			run = func() tea.Cmd { return CacheResetSlotCmd(m.InventoryPath, host, slot) }
		}
		fwr := NewConfirmForm(title, body)
		return m.openForm(fwr, func(result interface{}) tea.Cmd {
			if confirmed, _ := result.(bool); confirmed {
				return run()
			}
			return nil
		})

	case 'g':
		fwr := NewConfirmForm("GC orphan runners?",
			"Delete GitHub-side runners not represented in inventory.")
		return m.openForm(fwr, func(result interface{}) tea.Cmd {
			if confirmed, _ := result.(bool); confirmed {
				return GCCmd(m.InventoryPath)
			}
			return nil
		})

	case 'R':
		m.Flash = &flash{Text: "refresh queued — pollers tick every 2s anyway", Until: time.Now().Add(2 * time.Second)}
		return m, nil

	case '?':
		fwr := NewConfirmForm("Keybindings",
			"j/k or ↑/↓   move\n"+
				"⏎            expand/collapse host or repo group\n"+
				"d            drain & remove (host / pool / slot)\n"+
				"e            enable a disabled slot\n"+
				"r            reset cache (host / repo / slot)\n"+
				"g            gc orphan GitHub runners\n"+
				"a            add slot (picker on host; +1 on repo/slot)\n"+
				"p            add pool by name\n"+
				"R            refresh\n"+
				"?            this help\n"+
				"q / Ctrl-C   quit")
		return m.openForm(fwr, func(result interface{}) tea.Cmd { return nil })

	case 'p':
		if m.Cursor.Host == "" {
			m.Flash = &flash{Text: "No host under cursor — wait for the first poll.", Until: time.Now().Add(3 * time.Second)}
			return m, nil
		}
		host := m.Cursor.Host
		fwr := NewInputForm("Add pool on "+host,
			"Type owner/name. Count defaults to 1; spread = this host.",
			"owner/name")
		return m.openForm(fwr, func(result interface{}) tea.Cmd {
			repo, _ := result.(string)
			if repo == "" {
				return nil
			}
			return RepoAddCmd(m.InventoryPath, repo, host, 1)
		})

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
