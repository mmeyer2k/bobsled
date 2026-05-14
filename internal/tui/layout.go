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

// formatKeys renders a slice of (key, desc) pairs as "key desc   ·   key desc …".
func formatKeys(pairs ...[2]string) string {
	parts := make([]string, 0, len(pairs))
	for _, kv := range pairs {
		parts = append(parts, keyStyle.Render(kv[0])+" "+descStyle.Render(kv[1]))
	}
	return strings.Join(parts, descStyle.Render("  ·  "))
}

func (m Model) renderFooter() string {
	row1 := formatKeys(
		[2]string{"j/k", "nav"},
		[2]string{"⏎", "expand"},
		[2]string{"R", "refresh"},
		[2]string{"?", "help"},
		[2]string{"q", "quit"},
	)
	row2 := formatKeys(
		[2]string{"a", "add slot"},
		[2]string{"p/P", "pool +/-"},
		[2]string{"A", "add host"},
		[2]string{"d", "drain"},
		[2]string{"D", "remove host"},
		[2]string{"r", "reset cache"},
		[2]string{"g", "gc"},
		[2]string{"l", "logs"},
	)
	sep := sepStyle.Render(strings.Repeat("─", m.Width))
	help := sep + "\n" + row1 + "\n" + row2

	if m.Flash != nil && time.Now().Before(m.Flash.Until) {
		style := footerStyle
		if m.Flash.IsError {
			style = flashErrStyle
		}
		return style.Render(m.Flash.Text) + "\n" + help
	}
	return help
}
