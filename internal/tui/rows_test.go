// internal/tui/rows_test.go
package tui

import (
	"testing"

	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/tui/poller"
	"github.com/stretchr/testify/require"
)

func TestBuildRows_HostThenRepoThenSlots(t *testing.T) {
	hosts := map[string]*poller.HostState{
		"h1": {Name: "h1", Reachable: true, Capacity: 4, Slots: map[int]poller.SlotState{
			1: {N: 1, UnitState: "active", Repo: "acme/foo"},
			2: {N: 2, UnitState: "activating", Repo: "acme/foo"},
			3: {N: 3, UnitState: "active", Repo: "acme/bar"},
		}},
	}
	runners := map[string]*poller.RepoRunners{
		"acme/foo": {Runners: []ghapp.RunnerRef{{Name: "bobsled-h1-1"}}},
	}
	expanded := map[string]bool{"h1": true}

	rows := BuildRows(hosts, runners, expanded)
	require.Equal(t, RowHost, rows[0].Kind)
	require.Equal(t, RowRepo, rows[1].Kind)
	require.Equal(t, "acme/bar", rows[1].Repo)
	require.Equal(t, 1, rows[1].SlotCount)
	require.Equal(t, RowSlot, rows[2].Kind)
	require.Equal(t, 3, rows[2].Slot.N)
	require.Equal(t, "acme/bar", rows[2].Repo)
	require.Equal(t, RowRepo, rows[3].Kind)
	require.Equal(t, "acme/foo", rows[3].Repo)
	require.Equal(t, 2, rows[3].SlotCount)
	require.Equal(t, RowSlot, rows[4].Kind)
	require.Equal(t, 1, rows[4].Slot.N)
	require.Equal(t, "acme/foo", rows[4].Repo)
}

func TestBuildRows_HostCollapsedHasNoChildren(t *testing.T) {
	hosts := map[string]*poller.HostState{
		"h1": {Name: "h1", Slots: map[int]poller.SlotState{1: {N: 1, Repo: "a/b"}}},
	}
	rows := BuildRows(hosts, nil, map[string]bool{"h1": false})
	require.Len(t, rows, 1)
	require.Equal(t, RowHost, rows[0].Kind)
}

func TestBuildRows_CollapsedRepoHasNoSlots(t *testing.T) {
	hosts := map[string]*poller.HostState{
		"h1": {Name: "h1", Slots: map[int]poller.SlotState{1: {N: 1, Repo: "a/b"}}},
	}
	expanded := map[string]bool{"h1": true}
	expanded[repoExpandKey("h1", "a/b")] = false
	rows := BuildRows(hosts, nil, expanded)
	require.Len(t, rows, 2)
	require.Equal(t, RowHost, rows[0].Kind)
	require.Equal(t, RowRepo, rows[1].Kind)
}
