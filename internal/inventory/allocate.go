// internal/inventory/allocate.go
package inventory

import "github.com/m-meyer2k/bobsled/internal/state"

// Allocate maps the inventory to a per-host desired state. Allocation is
// deterministic: pools are processed in inventory order, and within a spread
// the first host is filled before the next is touched. Slot numbers on each
// host start at 1 and remain dense.
func Allocate(inv *Inventory) map[string]*state.State {
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

	for _, p := range inv.Pools {
		// Determine the per-host quota: no host in the spread may receive more
		// than the smallest capacity in the spread. This ensures an even upper
		// bound before spill kicks in for the tail hosts.
		minCap := 0
		for i, h := range p.Spread {
			c := inv.Hosts[h].Capacity
			if i == 0 || c < minCap {
				minCap = c
			}
		}

		remaining := p.Count
		for _, h := range p.Spread {
			if remaining == 0 {
				break
			}
			s := out[h]
			free := inv.Hosts[h].Capacity - len(s.Instances)
			take := remaining
			if take > minCap {
				take = minCap
			}
			if take > free {
				take = free
			}
			s.Repos[p.Repo] = state.RepoConfig{Labels: append([]string(nil), p.Labels...)}
			for i := 0; i < take; i++ {
				s.Instances[nextSlot(s)] = state.Instance{Repo: p.Repo}
			}
			remaining -= take
		}
	}
	return out
}
