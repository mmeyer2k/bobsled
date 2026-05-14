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
			body = "Stops every slot on this host. Idle slots stop immediately; busy slots finish their current job, then exit."
			run = func() tea.Cmd { return DrainHostCmd(m.InventoryPath, host) }
		case CursorRepo:
			repo := m.Cursor.Repo
			title = "Drain " + repo + " on " + host + "?"
			body = "Stops every slot serving this repo on this host. Idle slots stop immediately; busy slots finish their current job."
			captured := m.Hosts
			run = func() tea.Cmd { return DrainRepoCmd(m.InventoryPath, host, repo, captured) }
		case CursorSlot:
			slot := m.Cursor.Slot
			title = fmt.Sprintf("Drain slot %d on %s?", slot, host)
			body = "Stops this slot. If idle: immediate. If running a job: the job finishes, then the container exits."
			run = func() tea.Cmd { return DrainSlotCmd(m.InventoryPath, host, slot) }
		}
		fwr := NewConfirmForm(title, body)
		return m.openForm(fwr, func(result interface{}) tea.Cmd {
			if confirmed, _ := result.(bool); confirmed {
				return run()
			}
			return nil
		})

	case 'D':
		if m.Cursor.Kind != CursorHost {
			m.Flash = &flash{Text: "D removes a whole host — put the cursor on a host row.", Until: time.Now().Add(3 * time.Second)}
			return m, nil
		}
		host := m.Cursor.Host
		fwr := NewConfirmForm("Remove host "+host+"?",
			"Drain all slots, optionally gc runners, and drop from inventory.")
		return m.openForm(fwr, func(result interface{}) tea.Cmd {
			if confirmed, _ := result.(bool); confirmed {
				return HostRemoveCmd(m.InventoryPath, host)
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
				"d            drain (host / repo / slot)\n"+
				"D            remove host\n"+
				"r            reset cache (host / repo / slot)\n"+
				"g            gc orphan GitHub runners\n"+
				"a            add slot (picker on host; +1 on repo/slot)\n"+
				"p            add pool by name\n"+
				"P            remove pool\n"+
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

	case 'P':
		repo := m.Cursor.Repo
		if repo == "" {
			m.Flash = &flash{Text: "Put the cursor on a repo or slot row.", Until: time.Now().Add(3 * time.Second)}
			return m, nil
		}
		captured := repo
		fwr := NewConfirmForm("Remove pool "+repo+"?",
			"Drain every slot for this repo across the fleet, gc its GitHub-side runners, and drop from inventory.")
		return m.openForm(fwr, func(result interface{}) tea.Cmd {
			if confirmed, _ := result.(bool); confirmed {
				return RepoRemoveCmd(m.InventoryPath, captured)
			}
			return nil
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
