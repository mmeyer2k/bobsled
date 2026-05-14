// internal/inventory/mutate_test.go
package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func sampleInv() *Inventory {
	return &Inventory{
		GitHub: GitHubAuth{AppID: 1, AppKey: "/tmp/k"},
		Hosts: map[string]Host{
			"h1": {SSH: "bobsled@h1", BootstrapSSH: "mike@h1", Capacity: 4},
		},
		Pools: []Pool{
			{Repo: "acme/foo", Count: 2, Labels: []string{"bobsled"}, Spread: []string{"h1"}},
		},
	}
}

func TestAddHost_New(t *testing.T) {
	out, err := AddHost(sampleInv(), "h2", Host{SSH: "bobsled@h2", BootstrapSSH: "mike@h2", Capacity: 8})
	require.NoError(t, err)
	require.Equal(t, 8, out.Hosts["h2"].Capacity)
	require.Equal(t, "bobsled@h1", out.Hosts["h1"].SSH, "existing hosts preserved")
}

func TestAddHost_AlreadyExists(t *testing.T) {
	_, err := AddHost(sampleInv(), "h1", Host{SSH: "x", Capacity: 1})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

func TestRemoveHost(t *testing.T) {
	inv := sampleInv()
	inv.Pools[0].Spread = []string{"h1", "h2"}
	inv.Hosts["h2"] = Host{SSH: "bobsled@h2", Capacity: 4}

	out, err := RemoveHost(inv, "h2")
	require.NoError(t, err)
	require.NotContains(t, out.Hosts, "h2")
	require.Equal(t, []string{"h1"}, out.Pools[0].Spread, "h2 removed from spread")
}

func TestRemoveHost_PrunesEmptyPool(t *testing.T) {
	inv := sampleInv()
	out, err := RemoveHost(inv, "h1")
	require.NoError(t, err)
	require.NotContains(t, out.Hosts, "h1")
	require.Empty(t, out.Pools, "pool with empty spread is pruned")
}

func TestRemoveHost_NotFound(t *testing.T) {
	_, err := RemoveHost(sampleInv(), "nope")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestAdjustPool_IncreaseExisting(t *testing.T) {
	out, err := AdjustPool(sampleInv(), "acme/foo", +3, nil)
	require.NoError(t, err)
	require.Equal(t, 5, out.Pools[0].Count)
}

func TestAdjustPool_DecreaseToZeroPrunes(t *testing.T) {
	out, err := AdjustPool(sampleInv(), "acme/foo", -2, nil)
	require.NoError(t, err)
	require.Empty(t, out.Pools)
}

func TestAdjustPool_CreatesNewPool(t *testing.T) {
	out, err := AdjustPool(sampleInv(), "acme/new", +1, []string{"h1"})
	require.NoError(t, err)
	require.Len(t, out.Pools, 2)
	pool := out.Pools[1]
	require.Equal(t, "acme/new", pool.Repo)
	require.Equal(t, 1, pool.Count)
	require.Equal(t, []string{"h1"}, pool.Spread)
}

func TestAdjustPool_NewPoolWithoutHostsErrors(t *testing.T) {
	_, err := AdjustPool(sampleInv(), "acme/new", +1, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "spread")
}
