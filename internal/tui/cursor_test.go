// internal/tui/cursor_test.go
package tui

import (
	"testing"

	"github.com/m-meyer2k/bobsled/internal/tui/poller"
	"github.com/stretchr/testify/require"
)

func twoHosts() map[string]*poller.HostState {
	return map[string]*poller.HostState{
		"h1": {Name: "h1", Slots: map[int]poller.SlotState{1: {N: 1}, 2: {N: 2}}},
		"h2": {Name: "h2", Slots: map[int]poller.SlotState{1: {N: 1}}},
	}
}

func TestCursor_MovesThroughTree(t *testing.T) {
	expanded := map[string]bool{"h1": true, "h2": true}
	hosts := twoHosts()

	c := FirstCursor(hosts, expanded)
	require.Equal(t, "h1", c.Host)
	require.Equal(t, CursorHost, c.Kind)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, CursorSlot, c.Kind)
	require.Equal(t, 1, c.Slot)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, 2, c.Slot)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, "h2", c.Host)
	require.Equal(t, CursorHost, c.Kind)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, "h2", c.Host)
	require.Equal(t, CursorSlot, c.Kind)
	require.Equal(t, 1, c.Slot)

	// Past the end: stays put
	last := c
	c = NextCursor(c, hosts, expanded)
	require.Equal(t, last, c)
}

func TestCursor_SkipsCollapsedSlots(t *testing.T) {
	expanded := map[string]bool{"h1": false, "h2": true}
	hosts := twoHosts()

	c := FirstCursor(hosts, expanded)
	require.Equal(t, "h1", c.Host)
	require.Equal(t, CursorHost, c.Kind)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, "h2", c.Host)
	require.Equal(t, CursorHost, c.Kind)
}
