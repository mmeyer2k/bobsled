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
)

// renderView returns the full screen frame.
func (m Model) renderView() string {
	if m.Width == 0 {
		return "loading…"
	}
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")
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
	if r.Kind == RowHost {
		marker := "▼"
		if !m.Expanded[r.Host] {
			marker = "▶"
		}
		reach := "[ok]"
		if r.HostState != nil && !r.HostState.Reachable {
			reach = hostUnreach.Render("●UNREACHABLE")
		}
		used := 0
		cap := 0
		if r.HostState != nil {
			used = len(r.HostState.Slots)
			cap = r.HostState.Capacity
		}
		return hostHeader.Render(fmt.Sprintf("%s %-12s cap=%d used=%d   %s", marker, r.Host, cap, used, reach))
	}
	stateStyle := slotActive
	switch r.Slot.UnitState {
	case "activating":
		stateStyle = slotActivating
	case "failed":
		stateStyle = slotFailed
	}
	runner := r.RunnerName
	if runner == "" {
		runner = "—"
	}
	return fmt.Sprintf("    %d  %s  %-18s  %s", r.Slot.N, stateStyle.Render(r.Slot.UnitState), r.Slot.Repo, runner)
}

func (m Model) isCursorOn(r Row) bool {
	if r.Kind == RowHost {
		return m.Cursor.Host == r.Host && m.Cursor.Kind == CursorHost
	}
	return m.Cursor.Host == r.Host && m.Cursor.Kind == CursorSlot && m.Cursor.Slot == r.Slot.N
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
	help := footerStyle.Render(
		"j/k:nav  ⏎:expand/collapse  a:add slot  A:add host  d:drain  D:remove host  r:reset cache  l:logs  g:gc  R:refresh  ?:help  q:quit")
	if m.Flash != nil && time.Now().Before(m.Flash.Until) {
		style := footerStyle
		if m.Flash.IsError {
			style = flashErrStyle
		}
		return style.Render(m.Flash.Text) + "\n" + help
	}
	return help
}
