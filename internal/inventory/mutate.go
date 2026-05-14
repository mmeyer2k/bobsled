// internal/inventory/mutate.go
package inventory

import "fmt"

// AddHost returns a copy of inv with the given host added. Errors if the host
// name is already present. Does not mutate inv.
func AddHost(inv *Inventory, name string, h Host) (*Inventory, error) {
	if _, exists := inv.Hosts[name]; exists {
		return nil, fmt.Errorf("host %q already exists", name)
	}
	out := cloneInventory(inv)
	out.Hosts[name] = h
	return out, nil
}

// RemoveHost returns a copy of inv without the given host. Also drops the host
// from any pool's spread, and prunes pools whose spread becomes empty.
func RemoveHost(inv *Inventory, name string) (*Inventory, error) {
	if _, exists := inv.Hosts[name]; !exists {
		return nil, fmt.Errorf("host %q not found", name)
	}
	out := cloneInventory(inv)
	delete(out.Hosts, name)
	pools := make([]Pool, 0, len(out.Pools))
	for _, p := range out.Pools {
		spread := make([]string, 0, len(p.Spread))
		for _, h := range p.Spread {
			if h != name {
				spread = append(spread, h)
			}
		}
		if len(spread) == 0 {
			continue
		}
		p.Spread = spread
		pools = append(pools, p)
	}
	out.Pools = pools
	return out, nil
}

// AdjustPool changes the count of an existing pool by delta, or creates a new
// pool when none exists for repo. New pools require a non-empty spread.
// A delta that brings count to 0 or below prunes the pool entirely.
func AdjustPool(inv *Inventory, repo string, delta int, spread []string) (*Inventory, error) {
	out := cloneInventory(inv)
	for i := range out.Pools {
		if out.Pools[i].Repo == repo {
			out.Pools[i].Count += delta
			if out.Pools[i].Count <= 0 {
				out.Pools = append(out.Pools[:i], out.Pools[i+1:]...)
			}
			return out, nil
		}
	}
	if len(spread) == 0 {
		return nil, fmt.Errorf("creating new pool for %q requires a non-empty spread", repo)
	}
	out.Pools = append(out.Pools, Pool{
		Repo:   repo,
		Count:  delta,
		Labels: []string{"self-hosted", "linux", "x64", "bobsled", "podman"},
		Spread: append([]string(nil), spread...),
	})
	return out, nil
}

func cloneInventory(inv *Inventory) *Inventory {
	out := &Inventory{
		GitHub: inv.GitHub,
		Hosts:  make(map[string]Host, len(inv.Hosts)),
		Pools:  make([]Pool, len(inv.Pools)),
	}
	for k, v := range inv.Hosts {
		out.Hosts[k] = v
	}
	for i, p := range inv.Pools {
		p.Labels = append([]string(nil), p.Labels...)
		p.Spread = append([]string(nil), p.Spread...)
		out.Pools[i] = p
	}
	return out
}
