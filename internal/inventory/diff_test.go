// internal/inventory/diff_test.go
package inventory

import (
	"sort"
	"testing"

	"github.com/m-meyer2k/bobsled/internal/state"
	"github.com/stretchr/testify/require"
)

func TestDiffStates(t *testing.T) {
	cur := &state.State{Instances: map[int]state.Instance{1: {Repo: "a/b"}, 2: {Repo: "a/b"}, 3: {Repo: "x/y"}}}
	want := &state.State{Instances: map[int]state.Instance{1: {Repo: "a/b"}, 3: {Repo: "z/w"}, 4: {Repo: "a/b"}}}
	d := DiffStates(cur, want)
	sort.Ints(d.Added)
	sort.Ints(d.Removed)
	sort.Ints(d.Changed)
	require.Equal(t, []int{4}, d.Added)
	require.Equal(t, []int{2}, d.Removed)
	require.Equal(t, []int{3}, d.Changed)
}
