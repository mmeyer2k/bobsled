// internal/tui/layout.go
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	hostHeader     = lipgloss.NewStyle().Bold(true)
	hostUnreach    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	slotActive     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	slotActivating = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	slotFailed     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	cursorStyle    = lipgloss.NewStyle().Reverse(true)
	footerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	flashErrStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	keyStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	descStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	sepStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// renderView returns the full screen frame.
func (m Model) renderView() string {
	if m.Width == 0 {
		return "loading…"
	}
	// When a huh form is active it owns the screen entirely — the bordered,
	// centered look that huh provides replaces the normal tree view.
	if m.Form != nil {
		return m.Form.Form.View()
	}
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")
	b.WriteString(m.renderTree())
	b.WriteString("\n")
	b.WriteString(m.renderWorkload())
	b.WriteString("\n")
	b.WriteString(m.renderRecent())
	b.WriteString("\n")
	b.WriteString(m.renderFooter())
	return b.String()
}

func (m Model) renderHeader() string {
	totalSlots, busy := 0, 0
	for _, h := range m.Hosts {
		totalSlots += len(h.Slots)
	}
	for _, rep := range m.Runs {
		busy += len(rep.InProgress)
	}
	left := headerStyle.Render("bobsled top")
	right := fmt.Sprintf("fleet: %d hosts · %d slots · %d busy   ↻ %s",
		len(m.Hosts), totalSlots, busy, hostsInterval)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
}

func (m Model) renderTree() string {
	rows := BuildRows(m.Hosts, m.Runners, m.Expanded)
	var b strings.Builder
	for _, r := range rows {
		line := m.renderRow(r)
		if m.isCursorOn(r) {
			line = cursorStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (m Model) renderRow(r Row) string {
	switch r.Kind {
	case RowHost:
		marker := "▼"
		if !m.Expanded[r.Host] {
			marker = "▶"
		}
		reach := "[ok]"
		if r.HostState != nil && !r.HostState.Reachable {
			reach = hostUnreach.Render("●UNREACHABLE")
		}
		used, cap := 0, 0
		if r.HostState != nil {
			used = len(r.HostState.Slots)
			cap = r.HostState.Capacity
		}
		return hostHeader.Render(fmt.Sprintf("%s %-12s cap=%d used=%d   %s", marker, r.Host, cap, used, reach))

	case RowRepo:
		key := repoExpandKey(r.Host, r.Repo)
		marker := "▼"
		if val, ok := m.Expanded[key]; ok && !val {
			marker = "▶"
		}
		return "  " + hostHeader.Render(fmt.Sprintf("%s %s (%d)", marker, r.Repo, r.SlotCount))

	default: // RowSlot
		label := r.Slot.UnitState
		stateStyle := slotActive
		if !r.Slot.Enabled {
			// disabled but still doing whatever it was doing → draining
			if r.Slot.UnitState == "active" || r.Slot.UnitState == "activating" {
				label = "draining"
				stateStyle = slotActivating // amber
			} else {
				label = "disabled"
				stateStyle = slotFailed // dim red
			}
		} else {
			switch r.Slot.UnitState {
			case "activating":
				stateStyle = slotActivating
			case "failed":
				stateStyle = slotFailed
			}
		}
		runner := r.RunnerName
		if runner == "" {
			runner = "—"
		}
		// Pad the state label to a fixed width BEFORE styling — lipgloss adds
		// ANSI escapes that throw off fmt's width calculation.
		padded := fmt.Sprintf("%-10s", label)
		return fmt.Sprintf("      %2d  %s  %s", r.Slot.N, stateStyle.Render(padded), runner)
	}
}

func (m Model) isCursorOn(r Row) bool {
	switch r.Kind {
	case RowHost:
		return m.Cursor.Kind == CursorHost && m.Cursor.Host == r.Host
	case RowRepo:
		return m.Cursor.Kind == CursorRepo && m.Cursor.Host == r.Host && m.Cursor.Repo == r.Repo
	default: // RowSlot
		return m.Cursor.Kind == CursorSlot && m.Cursor.Host == r.Host && m.Cursor.Repo == r.Repo && m.Cursor.Slot == r.Slot.N
	}
}

func (m Model) renderWorkload() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("─ Workload ─") + "\n")
	for repo, rs := range m.Runs {
		queued := len(rs.Queued)
		ip := len(rs.InProgress)
		fmt.Fprintf(&b, "  %-20s queued: %d   in-progress: %d\n", repo, queued, ip)
	}
	return b.String()
}

func (m Model) renderRecent() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("─ Recent ─") + "\n")
	for repo, rs := range m.Runs {
		shown := 0
		for _, r := range rs.Recent {
			if shown >= 3 {
				break
			}
			fmt.Fprintf(&b, "  #%d %-9s %-18s  %s\n", r.ID, r.Status, repo, r.Name)
			shown++
		}
	}
	return b.String()
}

func (m Model) renderFooter() string {
	row1Items := []hint{
		{"j/k", "nav"},
	}
	if m.Cursor.Kind == CursorHost || m.Cursor.Kind == CursorRepo {
		row1Items = append(row1Items, hint{"⏎", "expand"})
	}
	row1Items = append(row1Items,
		hint{"R", "refresh"},
		hint{"?", "help"},
		hint{"q", "quit"},
	)

	row2Items := m.contextualActions()

	sep := sepStyle.Render(strings.Repeat("─", m.Width))
	help := sep + "\n" + formatHints(row1Items) + "\n" + formatHints(row2Items)

	if m.Flash != nil && time.Now().Before(m.Flash.Until) {
		style := footerStyle
		if m.Flash.IsError {
			style = flashErrStyle
		}
		return style.Render(m.Flash.Text) + "\n" + help
	}
	return help
}

type hint struct{ key, desc string }

func formatHints(hs []hint) string {
	parts := make([]string, 0, len(hs))
	for _, h := range hs {
		parts = append(parts, keyStyle.Render(h.key)+" "+descStyle.Render(h.desc))
	}
	return strings.Join(parts, descStyle.Render("  ·  "))
}

func (m Model) contextualActions() []hint {
	switch m.Cursor.Kind {
	case CursorHost:
		out := []hint{}
		if m.hostHasEnabledSlots(m.Cursor.Host) {
			out = append(out, hint{"d", "drain host"})
		}
		out = append(out,
			hint{"D", "remove host"},
			hint{"r", "reset caches"},
			hint{"g", "gc"},
			hint{"a", "add pool"},
			hint{"p", "name pool"},
			hint{"A", "add host"},
		)
		return out
	case CursorRepo:
		out := []hint{}
		if m.repoHasEnabledSlots(m.Cursor.Host, m.Cursor.Repo) {
			out = append(out, hint{"d", "drain pool"})
		}
		out = append(out,
			hint{"r", "reset caches"},
			hint{"a", "+1 slot"},
			hint{"P", "remove pool"},
		)
		return out
	case CursorSlot:
		out := []hint{}
		if m.slotIsEnabled(m.Cursor.Host, m.Cursor.Slot) {
			out = append(out, hint{"d", "drain"})
		}
		out = append(out,
			hint{"r", "reset cache"},
			hint{"a", "+1 slot"},
			hint{"P", "remove pool"},
		)
		return out
	}
	return nil
}

func (m Model) hostHasEnabledSlots(host string) bool {
	h := m.Hosts[host]
	if h == nil {
		return false
	}
	for _, s := range h.Slots {
		if s.Enabled {
			return true
		}
	}
	return false
}

func (m Model) repoHasEnabledSlots(host, repo string) bool {
	h := m.Hosts[host]
	if h == nil {
		return false
	}
	for _, s := range h.Slots {
		if s.Repo == repo && s.Enabled {
			return true
		}
	}
	return false
}

func (m Model) slotIsEnabled(host string, n int) bool {
	h := m.Hosts[host]
	if h == nil {
		return false
	}
	return h.Slots[n].Enabled
}
