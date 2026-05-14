// internal/tui/keys.go
package tui

import (
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
	case 'd':
		var onConfirm func() tea.Cmd
		title := "Drain slot"
		body := "Disable this slot. Existing jobs run to completion."
		if m.Cursor.Kind == CursorHost {
			title = "Drain host " + m.Cursor.Host
			body = "Disable every slot on this host."
			host := m.Cursor.Host
			onConfirm = func() tea.Cmd {
				return DrainHostCmd(m.InventoryPath, host)
			}
		} else {
			host, slot := m.Cursor.Host, m.Cursor.Slot
			onConfirm = func() tea.Cmd {
				return DrainSlotCmd(m.InventoryPath, host, slot)
			}
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
		var onConfirm func() tea.Cmd
		title := "Reset cache"
		body := "Wipe this (slot, repo) cache. Next run starts cold."
		if m.Cursor.Kind == CursorHost {
			title = "Reset all caches on " + m.Cursor.Host
			body = "Wipe every slot's cache on this host."
			host := m.Cursor.Host
			onConfirm = func() tea.Cmd { return CacheResetHostCmd(m.InventoryPath, host) }
		} else {
			host, slot := m.Cursor.Host, m.Cursor.Slot
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
				"a            add slot (CLI only for v1 — use `bobsled scale`)\n"+
				"A            add host (CLI only for v1 — use `bobsled host add`)\n"+
				"R            refresh (pollers tick automatically)\n"+
				"?            this help\n"+
				"q / Ctrl-C   quit",
			nil)
		m.Modal = &mod
		return m, nil

	case 'a':
		m.Flash = &flash{Text: "v1: use `bobsled scale --host h --repo r --count N` (inline prompt coming).", Until: time.Now().Add(4 * time.Second)}
		return m, nil

	case 'A':
		m.Flash = &flash{Text: "v1: use `bobsled host add` (inline prompt coming).", Until: time.Now().Add(4 * time.Second)}
		return m, nil
	}
	return m, nil
}
