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
	keyStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	descStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	sepStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	sectionLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
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
		// Pending overlay wins: while a slot-remove is in flight, show
		// "deleting" regardless of the underlying systemd state.
		pendingKey := fmt.Sprintf("%s:%d", r.Host, r.Slot.N)
		if p, ok := m.Pending[pendingKey]; ok {
			label = p
			stateStyle = slotActivating // amber — work in progress
		} else if !r.Slot.Enabled {
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
		// Pad to the longest possible systemd state (`deactivating` = 12)
		// BEFORE styling — lipgloss adds ANSI escapes that throw off fmt width.
		padded := fmt.Sprintf("%-12s", label)
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
	navItems := []hint{{"↑/↓", ""}}
	if m.Cursor.Kind == CursorHost || m.Cursor.Kind == CursorRepo {
		navItems = append(navItems, hint{"⏎", "expand"})
	}
	globalItems := []hint{
		{"R", "refresh"},
		{"?", "help"},
		{"q", "quit"},
	}
	actionItems := m.contextualActions()

	rows := []string{formatSections([]section{{"NAV", navItems}})}
	if len(actionItems) > 0 {
		rows = append(rows, formatSections([]section{{"ACTIONS", actionItems}}))
	}
	rows = append(rows, formatSections([]section{{"GLOBAL", globalItems}}))
	help := strings.Join(rows, "\n")

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

type section struct {
	label string
	hints []hint
}

func formatHints(hs []hint) string {
	parts := make([]string, 0, len(hs))
	for _, h := range hs {
		if h.desc == "" {
			parts = append(parts, keyStyle.Render(h.key))
		} else {
			parts = append(parts, keyStyle.Render(h.key)+" "+descStyle.Render(h.desc))
		}
	}
	return strings.Join(parts, descStyle.Render("  ·  "))
}

func formatSections(secs []section) string {
	parts := make([]string, 0, len(secs))
	for _, s := range secs {
		parts = append(parts, sectionLabelStyle.Render(s.label)+"  "+formatHints(s.hints))
	}
	return strings.Join(parts, descStyle.Render("     "))
}

func (m Model) contextualActions() []hint {
	switch m.Cursor.Kind {
	case CursorHost:
		return []hint{
			{"d", "drain host"},
			{"r", "reset caches"},
			{"g", "gc"},
			{"a", "add pool"},
			{"p", "name pool"},
			{"A", "add host"},
		}
	case CursorRepo:
		return []hint{
			{"d", "drain pool"},
			{"r", "reset caches"},
			{"a", "+1 slot"},
		}
	case CursorSlot:
		out := []hint{}
		if m.slotIsEnabled(m.Cursor.Host, m.Cursor.Slot) {
			out = append(out, hint{"d", "drain"})
		} else {
			out = append(out,
				hint{"e", "enable"},
				hint{"d", "delete"},
			)
		}
		out = append(out, hint{"r", "reset cache"})
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
