// internal/tui/rows_test.go
package tui

import (
	"testing"

	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/tui/poller"
	"github.com/stretchr/testify/require"
)

func TestBuildRows_HostThenSlots(t *testing.T) {
	hosts := map[string]*poller.HostState{
		"h1": {Name: "h1", Reachable: true, Capacity: 4, Slots: map[int]poller.SlotState{
			1: {N: 1, UnitState: "active", Repo: "acme/foo"},
			2: {N: 2, UnitState: "activating", Repo: "acme/foo"},
		}},
	}
	runners := map[string]*poller.RepoRunners{
		"acme/foo": {Runners: []ghapp.RunnerRef{{Name: "bobsled-h1-1"}}},
	}
	expanded := map[string]bool{"h1": true}

	rows := BuildRows(hosts, runners, expanded)
	require.Equal(t, RowHost, rows[0].Kind)
	require.Equal(t, "h1", rows[0].Host)
	require.Equal(t, RowSlot, rows[1].Kind)
	require.Equal(t, 1, rows[1].Slot.N)
	require.Equal(t, "active", rows[1].Slot.UnitState)
	require.Equal(t, "bobsled-h1-1", rows[1].RunnerName)
}

func TestBuildRows_HostCollapsedHasNoSlotRows(t *testing.T) {
	hosts := map[string]*poller.HostState{
		"h1": {Name: "h1", Slots: map[int]poller.SlotState{1: {N: 1}}},
	}
	rows := BuildRows(hosts, nil, map[string]bool{"h1": false})
	require.Len(t, rows, 1)
	require.Equal(t, RowHost, rows[0].Kind)
}
