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
	rows = AppendPendingPoolRows(rows, m.Hosts, m.Expanded, m.PendingPools)
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
		// Phantom row for a pending pool add — no real slot exists yet.
		// AppendPendingPoolRows synthesizes these with Slot.N == 0 and
		// UnitState == "creating".
		if r.Slot.N == 0 && r.Slot.UnitState == "creating" {
			padded := fmt.Sprintf("%-12s", "creating")
			return fmt.Sprintf("       —  %s  %s", slotActivating.Render(padded), "—")
		}
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
	navItems := []hint{{"↑/↓", "move"}}
	if m.Cursor.Kind == CursorHost || m.Cursor.Kind == CursorRepo {
		navItems = append(navItems, hint{"⏎", "expand"})
	}
	globalItems := []hint{
		{"R", "refresh"},
		{"?", "help"},
		{"q", "quit"},
	}
	actionItems := m.contextualActions()

	secs := []section{{"NAV", navItems}}
	if len(actionItems) > 0 {
		secs = append(secs, section{"ACTIONS", actionItems})
	}
	secs = append(secs, section{"GLOBAL", globalItems})

	help := renderLegendTable(secs, m.Width)

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

// renderLegendTable lays out the footer hints as a small table: one row per
// section, fixed-width section label gutter on the left, then a grid of
// `key  desc` cells aligned to a uniform column width shared by every row.
//
// The shared cell width is computed across all sections so the user's eye can
// scan straight down the column of keys (or the column of descriptions) — that
// alignment is the whole point. Cells are styled (bold cyan key, dim desc) but
// padding is added to the *raw* strings before styling because ANSI escapes
// break `fmt %-Ns` width.
func renderLegendTable(secs []section, termWidth int) string {
	if len(secs) == 0 {
		return ""
	}

	// Section-label column: fixed width, padded so all section rows line up.
	labelW := 0
	for _, s := range secs {
		if w := lipgloss.Width(s.label); w > labelW {
			labelW = w
		}
	}

	// Cell width = max(key) + keyDescGap + max(desc), shared across sections.
	const keyDescGap = 1   // spaces between key and desc inside one cell
	const cellGap = 3      // spaces between cells in a row
	const labelGap = 2     // spaces between section label gutter and first cell
	maxKey, maxDesc := 0, 0
	for _, s := range secs {
		for _, h := range s.hints {
			if w := lipgloss.Width(h.key); w > maxKey {
				maxKey = w
			}
			if w := lipgloss.Width(h.desc); w > maxDesc {
				maxDesc = w
			}
		}
	}
	cellW := maxKey + keyDescGap + maxDesc
	if maxDesc == 0 {
		cellW = maxKey
	}

	// Decide how many cells fit per row given the terminal width.
	cellsPerRow := 0
	if termWidth > 0 {
		avail := termWidth - labelW - labelGap
		if avail < cellW {
			cellsPerRow = 1 // give up on grid alignment, one cell per row
		} else {
			cellsPerRow = (avail + cellGap) / (cellW + cellGap)
			if cellsPerRow < 1 {
				cellsPerRow = 1
			}
		}
	}

	var b strings.Builder
	cellSep := strings.Repeat(" ", cellGap)
	for i, s := range secs {
		if i > 0 {
			b.WriteByte('\n')
		}
		// Section label gutter — pad raw, then style.
		labelPadded := fmt.Sprintf("%-*s", labelW, s.label)
		b.WriteString(sectionLabelStyle.Render(labelPadded))
		b.WriteString(strings.Repeat(" ", labelGap))

		// Hint cells. When we wrap to a second line for one section, indent
		// past the label gutter so the cell grid stays aligned.
		indent := strings.Repeat(" ", labelW+labelGap)
		for j, h := range s.hints {
			if j > 0 {
				if cellsPerRow > 0 && j%cellsPerRow == 0 {
					b.WriteByte('\n')
					b.WriteString(indent)
				} else {
					b.WriteString(cellSep)
				}
			}
			b.WriteString(renderHintCell(h, maxKey, cellW))
		}
	}
	return b.String()
}

// renderHintCell formats one `key  desc` cell padded to cellW visual columns.
// The key is right-aligned to keyW so vertical scanning hits a clean column
// of keys; descriptions hang left after a single space.
func renderHintCell(h hint, keyW, cellW int) string {
	keyPad := keyW - lipgloss.Width(h.key)
	if keyPad < 0 {
		keyPad = 0
	}
	styledKey := strings.Repeat(" ", keyPad) + keyStyle.Render(h.key)
	if h.desc == "" {
		// Pad cell out to cellW visually so later cells in the row stay aligned.
		tail := cellW - keyW
		if tail < 0 {
			tail = 0
		}
		return styledKey + strings.Repeat(" ", tail)
	}
	cell := styledKey + " " + descStyle.Render(h.desc)
	visible := keyW + 1 + lipgloss.Width(h.desc)
	if pad := cellW - visible; pad > 0 {
		cell += strings.Repeat(" ", pad)
	}
	return cell
}

func (m Model) contextualActions() []hint {
	switch m.Cursor.Kind {
	case CursorHost:
		return []hint{
			{"d", "drain host"},
			{"r", "reset caches"},
			{"g", "gc"},
			{"a", "add pool"},
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
