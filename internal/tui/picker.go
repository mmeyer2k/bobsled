// internal/tui/picker.go
package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Picker is a hand-rolled list with filter-as-you-type and space-toggle
// multi-select. It's overlaid on top of the main view, like Modal.
type Picker struct {
	Title    string
	Body     string
	Items    []string        // all candidate items (sorted)
	Filter   string          // typed filter
	Cursor   int             // index into filteredIndexes
	Selected map[string]bool // selected items, by raw item value
	OnPick   func(picked []string) tea.Cmd
	OnCancel func() tea.Cmd
}

func NewPicker(title, body string, items []string, onPick func([]string) tea.Cmd) Picker {
	sorted := append([]string(nil), items...)
	sort.Strings(sorted)
	return Picker{
		Title:    title,
		Body:     body,
		Items:    sorted,
		Selected: map[string]bool{},
		OnPick:   onPick,
	}
}

// filtered returns the items matching the current filter, preserving order.
func (p Picker) filtered() []string {
	if p.Filter == "" {
		return p.Items
	}
	f := strings.ToLower(p.Filter)
	out := make([]string, 0, len(p.Items))
	for _, it := range p.Items {
		if strings.Contains(strings.ToLower(it), f) {
			out = append(out, it)
		}
	}
	return out
}

// PickerOutcome signals what happened after a key press.
type PickerOutcome int

const (
	PickerKeep PickerOutcome = iota
	PickerCancelled
	PickerSubmitted
)

func (p Picker) OnKey(msg tea.KeyMsg) (Picker, PickerOutcome) {
	switch msg.Type {
	case tea.KeyEsc:
		return p, PickerCancelled
	case tea.KeyEnter:
		return p, PickerSubmitted
	case tea.KeyUp:
		if p.Cursor > 0 {
			p.Cursor--
		}
		return p, PickerKeep
	case tea.KeyDown:
		if p.Cursor < len(p.filtered())-1 {
			p.Cursor++
		}
		return p, PickerKeep
	case tea.KeyBackspace:
		if len(p.Filter) > 0 {
			p.Filter = p.Filter[:len(p.Filter)-1]
			p.Cursor = 0
		}
		return p, PickerKeep
	case tea.KeyRunes:
		// Space toggles the item at the cursor (multi-select).
		if len(msg.Runes) == 1 && msg.Runes[0] == ' ' {
			items := p.filtered()
			if p.Cursor >= 0 && p.Cursor < len(items) {
				it := items[p.Cursor]
				if p.Selected[it] {
					delete(p.Selected, it)
				} else {
					p.Selected[it] = true
				}
			}
			return p, PickerKeep
		}
		// All other runes append to the filter.
		p.Filter += string(msg.Runes)
		p.Cursor = 0
		return p, PickerKeep
	}
	return p, PickerKeep
}

// Picked returns the selected items as a slice (deterministic order).
func (p Picker) Picked() []string {
	out := make([]string, 0, len(p.Selected))
	for k := range p.Selected {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var (
	pickerCursorStyle = lipgloss.NewStyle().Reverse(true)
	pickerSelected    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
)

func (p Picker) Render() string {
	var b strings.Builder
	b.WriteString("╭── " + p.Title + " ──╮\n")
	b.WriteString("│  " + p.Body + "\n")
	b.WriteString("│  filter: " + p.Filter + "_\n")
	b.WriteString("│\n")
	items := p.filtered()
	max := 12
	start := 0
	if p.Cursor >= max {
		start = p.Cursor - max + 1
	}
	end := start + max
	if end > len(items) {
		end = len(items)
	}
	for i := start; i < end; i++ {
		it := items[i]
		marker := "  "
		if p.Selected[it] {
			marker = pickerSelected.Render("✓ ")
		}
		line := "│  " + marker + it
		if i == p.Cursor {
			line = pickerCursorStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("│\n")
	b.WriteString("│  space:toggle  ⏎:submit  esc:cancel  (type to filter)\n")
	b.WriteString("╰─\n")
	return b.String()
}
