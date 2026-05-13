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
	require.Len(t, got["h1"].Instances, 4)
	require.Len(t, got["h2"].Instances, 2)
	require.Equal(t, []string{"bobsled", "podman"}, got["h1"].Repos["acme/foo"].Labels)
}

func TestAllocate_MultiplePoolsShareHost(t *testing.T) {
	inv := &Inventory{
		Hosts: map[string]Host{"h1": {SSH: "x", Capacity: 8}, "h2": {SSH: "y", Capacity: 4}},
		Pools: []Pool{
			{Repo: "acme/foo", Count: 6, Spread: []string{"h1", "h2"}, Labels: []string{"bobsled"}},
			{Repo: "acme/bar", Count: 2, Spread: []string{"h1"}, Labels: []string{"bobsled", "bar"}},
		},
	}
	got := Allocate(inv)
	require.Len(t, got["h1"].Instances, 6, "4 foo + 2 bar")
	require.Len(t, got["h2"].Instances, 2)

	fooCount, barCount := 0, 0
	for _, inst := range got["h1"].Instances {
		switch inst.Repo {
		case "acme/foo":
			fooCount++
		case "acme/bar":
			barCount++
		}
	}
	require.Equal(t, 4, fooCount)
	require.Equal(t, 2, barCount)
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

var _ = state.Instance{}
