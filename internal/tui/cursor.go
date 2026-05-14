// internal/tui/cursor.go
package tui

import (
	"github.com/m-meyer2k/bobsled/internal/tui/poller"
)

type CursorKind int

const (
	CursorHost CursorKind = iota
	CursorRepo
	CursorSlot
)

type Cursor struct {
	Host string
	Repo string // when Kind == CursorRepo or CursorSlot
	Kind CursorKind
	Slot int // when Kind == CursorSlot
}

// FirstCursor returns the cursor pointing at the first host header.
func FirstCursor(hosts map[string]*poller.HostState, expanded map[string]bool) Cursor {
	rows := BuildRows(hosts, nil, expanded)
	if len(rows) == 0 {
		return Cursor{}
	}
	return cursorForRow(rows[0])
}

// NextCursor returns the cursor one row down. Out-of-tree → returns input unchanged.
func NextCursor(c Cursor, hosts map[string]*poller.HostState, expanded map[string]bool) Cursor {
	rows := BuildRows(hosts, nil, expanded)
	i := cursorIndex(c, rows)
	if i < 0 || i+1 >= len(rows) {
		return c
	}
	return cursorForRow(rows[i+1])
}

// PrevCursor returns the cursor one row up. Out-of-tree → returns input unchanged.
func PrevCursor(c Cursor, hosts map[string]*poller.HostState, expanded map[string]bool) Cursor {
	rows := BuildRows(hosts, nil, expanded)
	i := cursorIndex(c, rows)
	if i <= 0 {
		return c
	}
	return cursorForRow(rows[i-1])
}

func cursorForRow(r Row) Cursor {
	switch r.Kind {
	case RowHost:
		return Cursor{Host: r.Host, Kind: CursorHost}
	case RowRepo:
		return Cursor{Host: r.Host, Repo: r.Repo, Kind: CursorRepo}
	case RowSlot:
		return Cursor{Host: r.Host, Repo: r.Repo, Slot: r.Slot.N, Kind: CursorSlot}
	}
	return Cursor{}
}

// CursorIndex returns the row index the cursor currently points at, or -1
// if it doesn't match any row. Useful for snapshotting before a state change.
func CursorIndex(c Cursor, hosts map[string]*poller.HostState, expanded map[string]bool) int {
	return cursorIndex(c, BuildRows(hosts, nil, expanded))
}

// EnsureCursorValid checks the cursor against the current rows. If it still
// matches a row, it's returned unchanged. Otherwise the cursor snaps to the
// row at preferredIdx (clamped to [0, len(rows)-1]), or {} if no rows exist.
// Use the old row index (captured before the state change) as preferredIdx
// so the visual cursor stays at the same vertical spot.
func EnsureCursorValid(c Cursor, hosts map[string]*poller.HostState, expanded map[string]bool, preferredIdx int) Cursor {
	rows := BuildRows(hosts, nil, expanded)
	if cursorIndex(c, rows) >= 0 {
		return c
	}
	if len(rows) == 0 {
		return Cursor{}
	}
	if preferredIdx >= len(rows) {
		preferredIdx = len(rows) - 1
	}
	if preferredIdx < 0 {
		preferredIdx = 0
	}
	return cursorForRow(rows[preferredIdx])
}

func cursorIndex(c Cursor, rows []Row) int {
	for i, r := range rows {
		switch c.Kind {
		case CursorHost:
			if r.Kind == RowHost && r.Host == c.Host {
				return i
			}
		case CursorRepo:
			if r.Kind == RowRepo && r.Host == c.Host && r.Repo == c.Repo {
				return i
			}
		case CursorSlot:
			if r.Kind == RowSlot && r.Host == c.Host && r.Repo == c.Repo && r.Slot.N == c.Slot {
				return i
			}
		}
	}
	return -1
}
