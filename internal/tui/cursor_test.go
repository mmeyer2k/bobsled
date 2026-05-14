// internal/tui/cursor_test.go
package tui

import (
	"testing"

	"github.com/m-meyer2k/bobsled/internal/tui/poller"
	"github.com/stretchr/testify/require"
)

func twoHosts() map[string]*poller.HostState {
	return map[string]*poller.HostState{
		"h1": {Name: "h1", Slots: map[int]poller.SlotState{
			1: {N: 1, Repo: "a/foo"},
			2: {N: 2, Repo: "a/foo"},
			3: {N: 3, Repo: "b/bar"},
		}},
		"h2": {Name: "h2", Slots: map[int]poller.SlotState{
			1: {N: 1, Repo: "a/foo"},
		}},
	}
}

func TestCursor_WalksHostRepoSlot(t *testing.T) {
	hosts := twoHosts()
	expanded := map[string]bool{"h1": true, "h2": true}

	c := FirstCursor(hosts, expanded)
	require.Equal(t, Cursor{Host: "h1", Kind: CursorHost}, c)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, Cursor{Host: "h1", Repo: "a/foo", Kind: CursorRepo}, c, "after host comes the first repo group")

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, Cursor{Host: "h1", Repo: "a/foo", Slot: 1, Kind: CursorSlot}, c)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, Cursor{Host: "h1", Repo: "a/foo", Slot: 2, Kind: CursorSlot}, c)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, Cursor{Host: "h1", Repo: "b/bar", Kind: CursorRepo}, c)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, Cursor{Host: "h1", Repo: "b/bar", Slot: 3, Kind: CursorSlot}, c)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, Cursor{Host: "h2", Kind: CursorHost}, c)
}

func TestCursor_CollapsedHostSkipsChildren(t *testing.T) {
	hosts := twoHosts()
	expanded := map[string]bool{"h1": false, "h2": true}

	c := FirstCursor(hosts, expanded)
	require.Equal(t, Cursor{Host: "h1", Kind: CursorHost}, c)
	c = NextCursor(c, hosts, expanded)
	require.Equal(t, Cursor{Host: "h2", Kind: CursorHost}, c)
}

func TestCursor_CollapsedRepoSkipsSlots(t *testing.T) {
	hosts := twoHosts()
	expanded := map[string]bool{"h1": true, "h2": true}
	expanded[repoExpandKey("h1", "a/foo")] = false

	c := Cursor{Host: "h1", Repo: "a/foo", Kind: CursorRepo}
	c = NextCursor(c, hosts, expanded)
	require.Equal(t, Cursor{Host: "h1", Repo: "b/bar", Kind: CursorRepo}, c, "next should jump past collapsed repo's slots")
}

func TestCursor_PrevReverses(t *testing.T) {
	hosts := twoHosts()
	expanded := map[string]bool{"h1": true, "h2": true}
	c := Cursor{Host: "h2", Kind: CursorHost}
	c = PrevCursor(c, hosts, expanded)
	require.Equal(t, Cursor{Host: "h1", Repo: "b/bar", Slot: 3, Kind: CursorSlot}, c)
}
