// internal/tui/rows.go
package tui

import (
	"sort"
	"strconv"

	"github.com/m-meyer2k/bobsled/internal/tui/poller"
)

type RowKind int

const (
	RowHost RowKind = iota
	RowSlot
)

type Row struct {
	Kind       RowKind
	Host       string
	HostState  *poller.HostState // nil if Kind==RowSlot
	Slot       poller.SlotState  // zero if Kind==RowHost
	RunnerName string            // matched runner from runners[repo], if found
}

// BuildRows flattens the tree into a vertical row list ready for rendering.
// Hosts are sorted alphabetically; slots numerically. Collapsed hosts skip
// their slot rows entirely.
func BuildRows(hosts map[string]*poller.HostState, runners map[string]*poller.RepoRunners, expanded map[string]bool) []Row {
	names := make([]string, 0, len(hosts))
	for k := range hosts {
		names = append(names, k)
	}
	sort.Strings(names)

	rows := make([]Row, 0, len(names)*4)
	for _, name := range names {
		h := hosts[name]
		rows = append(rows, Row{Kind: RowHost, Host: name, HostState: h})
		if !expanded[name] {
			continue
		}
		slotNums := make([]int, 0, len(h.Slots))
		for n := range h.Slots {
			slotNums = append(slotNums, n)
		}
		sort.Ints(slotNums)
		for _, n := range slotNums {
			slot := h.Slots[n]
			rows = append(rows, Row{
				Kind:       RowSlot,
				Host:       name,
				Slot:       slot,
				RunnerName: matchRunner(name, n, slot.Repo, runners),
			})
		}
	}
	return rows
}

func matchRunner(host string, slot int, repo string, runners map[string]*poller.RepoRunners) string {
	if runners == nil || runners[repo] == nil {
		return ""
	}
	want := "bobsled-" + host + "-" + strconv.Itoa(slot)
	for _, r := range runners[repo].Runners {
		if r.Name == want {
			return r.Name
		}
	}
	return ""
}
