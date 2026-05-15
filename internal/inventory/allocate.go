// internal/inventory/allocate.go
package inventory

import "github.com/m-meyer2k/bobsled/internal/state"

// Allocate maps the inventory to a per-host desired state. Allocation is
// deterministic: pools are processed in inventory order, and within a spread
// the first host is filled before the next is touched. Slot numbers on each
// host start at 1 and remain dense.
func Allocate(inv *Inventory) map[string]*state.State {
	return AllocateWithCurrent(inv, nil)
}

// AllocateWithCurrent is Allocate, but when current[host] already has exactly
// the desired number of slots for a pool's repo, the existing slot indices
// are preserved (even if sparse — e.g. {1,2,4,5}). When the count differs,
// allocation falls back to dense renumbering from the next free slot.
//
// This lets `bobsled slot remove <host> <N>` delete a non-tail slot without
// subsequent `apply` runs treating the resulting sparse layout as drift.
func AllocateWithCurrent(inv *Inventory, current map[string]*state.State) map[string]*state.State {
	out := map[string]*state.State{}
	for name := range inv.Hosts {
		out[name] = &state.State{
			Repos:     map[string]state.RepoConfig{},
			Instances: map[int]state.Instance{},
		}
	}

	nextSlot := func(s *state.State) int {
		used := 0
		for slot := range s.Instances {
			if slot > used {
				used = slot
			}
		}
		return used + 1
	}

	// currentSlotsFor returns the slot indices on host h currently serving repo r,
	// or nil if no current state is available for h.
	currentSlotsFor := func(h, r string) []int {
		if current == nil {
			return nil
		}
		cs, ok := current[h]
		if !ok || cs == nil {
			return nil
		}
		var slots []int
		for slot, inst := range cs.Instances {
			if inst.Repo == r {
				slots = append(slots, slot)
			}
		}
		return slots
	}

	for _, p := range inv.Pools {
		remaining := p.Count
		for _, h := range p.Spread {
			if remaining == 0 {
				break
			}
			s := out[h]
			free := inv.Hosts[h].Capacity - len(s.Instances)
			take := remaining
			if take > free {
				take = free
			}
			s.Repos[p.Repo] = state.RepoConfig{Labels: append([]string(nil), p.Labels...)}

			// Preserve current indices when the host already has exactly `take`
			// slots for this repo. Avoids dense-renumbering drift after a
			// sparse slot removal.
			existing := currentSlotsFor(h, p.Repo)
			if len(existing) == take && take > 0 {
				for _, slot := range existing {
					s.Instances[slot] = state.Instance{Repo: p.Repo}
				}
			} else {
				for i := 0; i < take; i++ {
					s.Instances[nextSlot(s)] = state.Instance{Repo: p.Repo}
				}
			}
			remaining -= take
		}
	}
	return out
}
