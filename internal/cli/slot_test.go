// internal/cli/slot_test.go
package cli

import (
	"testing"

	"github.com/m-meyer2k/bobsled/internal/state"
	"github.com/stretchr/testify/require"
)

func TestRemoveSlotFromState_RemovesMiddleSlot(t *testing.T) {
	cur := &state.State{
		Repos: map[string]state.RepoConfig{"a/b": {Labels: []string{"bobsled"}}},
		Instances: map[int]state.Instance{
			1: {Repo: "a/b"},
			2: {Repo: "a/b"},
			3: {Repo: "a/b"},
			4: {Repo: "a/b"},
			5: {Repo: "a/b"},
		},
	}
	got, err := removeSlotFromState(cur, 4)
	require.NoError(t, err)
	require.Len(t, got.Instances, 4)
	for _, slot := range []int{1, 2, 3, 5} {
		_, ok := got.Instances[slot]
		require.Truef(t, ok, "slot %d preserved", slot)
	}
	_, has4 := got.Instances[4]
	require.False(t, has4, "slot 4 removed")
	// Original state untouched (pure function).
	require.Len(t, cur.Instances, 5, "input not mutated")
}

func TestRemoveSlotFromState_MissingSlotErrors(t *testing.T) {
	cur := &state.State{
		Repos:     map[string]state.RepoConfig{"a/b": {Labels: []string{"bobsled"}}},
		Instances: map[int]state.Instance{1: {Repo: "a/b"}, 2: {Repo: "a/b"}},
	}
	_, err := removeSlotFromState(cur, 99)
	require.Error(t, err)
	require.Contains(t, err.Error(), "99")
}

func TestRemoveSlotFromState_PrunesUnreferencedRepo(t *testing.T) {
	// If removing the slot drops the last instance for a repo, the repo's
	// entry in state.Repos must also be pruned. Otherwise subsequent reads
	// see a "ghost" repo with no slots, which the diff / allocator would
	// treat as drift.
	cur := &state.State{
		Repos: map[string]state.RepoConfig{
			"a/b": {Labels: []string{"bobsled"}},
			"c/d": {Labels: []string{"bobsled"}},
		},
		Instances: map[int]state.Instance{
			1: {Repo: "a/b"},
			2: {Repo: "c/d"},
		},
	}
	got, err := removeSlotFromState(cur, 2)
	require.NoError(t, err)
	require.Contains(t, got.Repos, "a/b")
	require.NotContains(t, got.Repos, "c/d", "unreferenced repo pruned")
}

func TestRemoveSlotFromState_KeepsRepoWhenOtherSlotsRemain(t *testing.T) {
	cur := &state.State{
		Repos: map[string]state.RepoConfig{"a/b": {Labels: []string{"bobsled"}}},
		Instances: map[int]state.Instance{
			1: {Repo: "a/b"},
			2: {Repo: "a/b"},
		},
	}
	got, err := removeSlotFromState(cur, 1)
	require.NoError(t, err)
	require.Contains(t, got.Repos, "a/b", "repo retained while other slots use it")
	require.Len(t, got.Instances, 1)
}
