// internal/inventory/allocate_test.go
package inventory

import (
	"testing"

	"github.com/m-meyer2k/bobsled/internal/state"
	"github.com/stretchr/testify/require"
)

func TestAllocate_FillsPrimaryThenSpills(t *testing.T) {
	inv := &Inventory{
		Hosts: map[string]Host{
			"h1": {SSH: "x", Capacity: 4},
			"h2": {SSH: "y", Capacity: 4},
		},
		Pools: []Pool{
			{Repo: "acme/foo", Count: 6, Labels: []string{"bobsled", "podman"}, Spread: []string{"h1", "h2"}},
		},
	}
	got := Allocate(inv)
	require.Len(t, got["h1"].Instances, 4, "h1 fills to capacity")
	require.Len(t, got["h2"].Instances, 2, "h2 absorbs spill")
	require.Equal(t, []string{"bobsled", "podman"}, got["h1"].Repos["acme/foo"].Labels)
}

func TestAllocate_GreedyFillUsesFirstHostBeforeSpilling(t *testing.T) {
	// With h1 cap=8 and 6 foo + 2 bar all spreadable on h1, greedy fill puts
	// everything on h1 — h2 stays idle. If the operator wants load on h2,
	// they should lower h1's capacity or split into separate pools.
	inv := &Inventory{
		Hosts: map[string]Host{"h1": {SSH: "x", Capacity: 8}, "h2": {SSH: "y", Capacity: 4}},
		Pools: []Pool{
			{Repo: "acme/foo", Count: 6, Spread: []string{"h1", "h2"}, Labels: []string{"bobsled"}},
			{Repo: "acme/bar", Count: 2, Spread: []string{"h1"}, Labels: []string{"bobsled", "bar"}},
		},
	}
	got := Allocate(inv)
	require.Len(t, got["h1"].Instances, 8, "all foo (6) + all bar (2) land on h1 (cap 8)")
	require.Len(t, got["h2"].Instances, 0, "h2 unused because h1 absorbed everything")

	fooCount, barCount := 0, 0
	for _, inst := range got["h1"].Instances {
		switch inst.Repo {
		case "acme/foo":
			fooCount++
		case "acme/bar":
			barCount++
		}
	}
	require.Equal(t, 6, fooCount)
	require.Equal(t, 2, barCount)
}

func TestAllocate_SpillsWhenFirstHostFull(t *testing.T) {
	// With h1 cap=5 and a pool of 7, h1 fills (5) and h2 takes the spill (2).
	inv := &Inventory{
		Hosts: map[string]Host{"h1": {SSH: "x", Capacity: 5}, "h2": {SSH: "y", Capacity: 5}},
		Pools: []Pool{{Repo: "a/b", Count: 7, Spread: []string{"h1", "h2"}, Labels: []string{"bobsled"}}},
	}
	got := Allocate(inv)
	require.Len(t, got["h1"].Instances, 5)
	require.Len(t, got["h2"].Instances, 2)
}

func TestAllocate_Deterministic(t *testing.T) {
	inv := &Inventory{
		Hosts: map[string]Host{"h1": {SSH: "x", Capacity: 5}},
		Pools: []Pool{{Repo: "a/b", Count: 3, Spread: []string{"h1"}, Labels: []string{"bobsled"}}},
	}
	a := Allocate(inv)
	b := Allocate(inv)
	require.Equal(t, a["h1"].Instances, b["h1"].Instances)
	for i := 1; i <= 3; i++ {
		_, ok := a["h1"].Instances[i]
		require.True(t, ok)
	}
}

// AllocateWithCurrent preserves existing slot indices on a host when the
// host already has exactly the desired count of slots for a pool's repo.
// This is what enables `bobsled slot remove` to delete a non-tail slot
// without subsequent `apply` runs recreating it via dense renumbering.
func TestAllocateWithCurrent_PreservesSparseIndicesWhenCountMatches(t *testing.T) {
	inv := &Inventory{
		Hosts: map[string]Host{"h1": {SSH: "x", Capacity: 5}},
		Pools: []Pool{{Repo: "a/b", Count: 4, Spread: []string{"h1"}, Labels: []string{"bobsled"}}},
	}
	current := map[string]*state.State{
		"h1": {
			Repos:     map[string]state.RepoConfig{"a/b": {Labels: []string{"bobsled"}}},
			Instances: map[int]state.Instance{1: {Repo: "a/b"}, 2: {Repo: "a/b"}, 4: {Repo: "a/b"}, 5: {Repo: "a/b"}},
		},
	}
	got := AllocateWithCurrent(inv, current)
	require.Len(t, got["h1"].Instances, 4, "preserves count")
	for _, slot := range []int{1, 2, 4, 5} {
		_, ok := got["h1"].Instances[slot]
		require.Truef(t, ok, "slot %d preserved", slot)
	}
	_, has3 := got["h1"].Instances[3]
	require.False(t, has3, "slot 3 not added by dense renumbering")
}

func TestAllocateWithCurrent_RenumbersDenselyWhenCountDiffers(t *testing.T) {
	// If current has 3 slots for the repo on h1 but the pool wants 5,
	// don't try to be clever — fall back to dense allocation.
	inv := &Inventory{
		Hosts: map[string]Host{"h1": {SSH: "x", Capacity: 5}},
		Pools: []Pool{{Repo: "a/b", Count: 5, Spread: []string{"h1"}, Labels: []string{"bobsled"}}},
	}
	current := map[string]*state.State{
		"h1": {
			Repos:     map[string]state.RepoConfig{"a/b": {Labels: []string{"bobsled"}}},
			Instances: map[int]state.Instance{1: {Repo: "a/b"}, 2: {Repo: "a/b"}, 4: {Repo: "a/b"}},
		},
	}
	got := AllocateWithCurrent(inv, current)
	require.Len(t, got["h1"].Instances, 5)
	for i := 1; i <= 5; i++ {
		_, ok := got["h1"].Instances[i]
		require.Truef(t, ok, "dense slot %d allocated", i)
	}
}

func TestAllocateWithCurrent_NilCurrentMatchesPlainAllocate(t *testing.T) {
	inv := &Inventory{
		Hosts: map[string]Host{"h1": {SSH: "x", Capacity: 5}},
		Pools: []Pool{{Repo: "a/b", Count: 3, Spread: []string{"h1"}, Labels: []string{"bobsled"}}},
	}
	withNil := AllocateWithCurrent(inv, nil)
	plain := Allocate(inv)
	require.Equal(t, plain["h1"].Instances, withNil["h1"].Instances)
}

var _ = state.Instance{}
