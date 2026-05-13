// internal/inventory/diff.go
package inventory

import "github.com/m-meyer2k/bobsled/internal/state"

type Diff struct {
	Added   []int
	Removed []int
	Changed []int
}

func DiffStates(current, desired *state.State) Diff {
	var d Diff
	for slot, want := range desired.Instances {
		got, ok := current.Instances[slot]
		switch {
		case !ok:
			d.Added = append(d.Added, slot)
		case got.Repo != want.Repo:
			d.Changed = append(d.Changed, slot)
		}
	}
	for slot := range current.Instances {
		if _, ok := desired.Instances[slot]; !ok {
			d.Removed = append(d.Removed, slot)
		}
	}
	return d
}
