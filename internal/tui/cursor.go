// internal/tui/cursor.go
package tui

import (
	"sort"

	"github.com/m-meyer2k/bobsled/internal/tui/poller"
)

type CursorKind int

const (
	CursorHost CursorKind = iota
	CursorSlot
)

type Cursor struct {
	Host string
	Kind CursorKind
	Slot int // when Kind==CursorSlot
}

// FirstCursor returns the cursor pointing at the first host header.
func FirstCursor(hosts map[string]*poller.HostState, expanded map[string]bool) Cursor {
	names := sortedHostNames(hosts)
	if len(names) == 0 {
		return Cursor{}
	}
	return Cursor{Host: names[0], Kind: CursorHost}
}

// NextCursor returns the cursor one row down. Out-of-tree → returns input unchanged.
func NextCursor(c Cursor, hosts map[string]*poller.HostState, expanded map[string]bool) Cursor {
	names := sortedHostNames(hosts)
	for i, name := range names {
		switch {
		case c.Host == name && c.Kind == CursorHost:
			if expanded[name] {
				slots := sortedSlotNums(hosts[name])
				if len(slots) > 0 {
					return Cursor{Host: name, Kind: CursorSlot, Slot: slots[0]}
				}
			}
			if i+1 < len(names) {
				return Cursor{Host: names[i+1], Kind: CursorHost}
			}
			return c
		case c.Host == name && c.Kind == CursorSlot:
			slots := sortedSlotNums(hosts[name])
			idx := sort.SearchInts(slots, c.Slot)
			if idx+1 < len(slots) {
				return Cursor{Host: name, Kind: CursorSlot, Slot: slots[idx+1]}
			}
			if i+1 < len(names) {
				return Cursor{Host: names[i+1], Kind: CursorHost}
			}
			return c
		}
	}
	return c
}

// PrevCursor returns the cursor one row up. Out-of-tree → returns input unchanged.
func PrevCursor(c Cursor, hosts map[string]*poller.HostState, expanded map[string]bool) Cursor {
	names := sortedHostNames(hosts)
	for i, name := range names {
		switch {
		case c.Host == name && c.Kind == CursorHost:
			if i == 0 {
				return c
			}
			prev := names[i-1]
			if expanded[prev] {
				slots := sortedSlotNums(hosts[prev])
				if len(slots) > 0 {
					return Cursor{Host: prev, Kind: CursorSlot, Slot: slots[len(slots)-1]}
				}
			}
			return Cursor{Host: prev, Kind: CursorHost}
		case c.Host == name && c.Kind == CursorSlot:
			slots := sortedSlotNums(hosts[name])
			idx := sort.SearchInts(slots, c.Slot)
			if idx == 0 {
				return Cursor{Host: name, Kind: CursorHost}
			}
			return Cursor{Host: name, Kind: CursorSlot, Slot: slots[idx-1]}
		}
	}
	return c
}

func sortedHostNames(m map[string]*poller.HostState) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedSlotNums(h *poller.HostState) []int {
	if h == nil {
		return nil
	}
	out := make([]int, 0, len(h.Slots))
	for k := range h.Slots {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
