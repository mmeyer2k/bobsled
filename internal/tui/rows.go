// internal/tui/rows.go
package tui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/m-meyer2k/bobsled/internal/tui/poller"
)

type RowKind int

const (
	RowHost RowKind = iota
	RowRepo
	RowSlot
)

type Row struct {
	Kind       RowKind
	Host       string
	Repo       string            // for RowRepo and RowSlot
	HostState  *poller.HostState // RowHost only
	Slot       poller.SlotState  // RowSlot only
	RunnerName string            // RowSlot only
	SlotCount  int               // RowRepo only — number of slots in this group
}

// BuildRows flattens the tree into a vertical row list ready for rendering.
// Hosts are sorted alphabetically; repos alphabetically within each host;
// slots numerically within each repo. Collapsed hosts skip their children;
// collapsed repo groups skip their slot rows.
func BuildRows(hosts map[string]*poller.HostState, runners map[string]*poller.RepoRunners, expanded map[string]bool) []Row {
	names := make([]string, 0, len(hosts))
	for k := range hosts {
		names = append(names, k)
	}
	sort.Strings(names)

	rows := make([]Row, 0, len(names)*4)
	for _, hostName := range names {
		h := hosts[hostName]
		rows = append(rows, Row{Kind: RowHost, Host: hostName, HostState: h})
		if !expanded[hostName] {
			continue
		}
		// Group slots by repo. Skip orphan slots (no repo in state.yaml) —
		// they're leftover systemd units from prior deletes, not real pool
		// members. Showing them as an anonymous "(N)" group is noise.
		byRepo := map[string][]poller.SlotState{}
		for _, s := range h.Slots {
			if s.Repo == "" {
				continue
			}
			byRepo[s.Repo] = append(byRepo[s.Repo], s)
		}
		repoNames := make([]string, 0, len(byRepo))
		for r := range byRepo {
			repoNames = append(repoNames, r)
		}
		sort.Strings(repoNames)
		for _, repo := range repoNames {
			slots := byRepo[repo]
			// Enabled slots first, then disabled. Numeric within each group.
			sort.Slice(slots, func(i, j int) bool {
				if slots[i].Enabled != slots[j].Enabled {
					return slots[i].Enabled
				}
				return slots[i].N < slots[j].N
			})
			rows = append(rows, Row{
				Kind:      RowRepo,
				Host:      hostName,
				Repo:      repo,
				SlotCount: len(slots),
			})
			repoKey := repoExpandKey(hostName, repo)
			// Default: repo group is expanded unless explicitly collapsed.
			if val, ok := expanded[repoKey]; ok && !val {
				continue
			}
			for _, s := range slots {
				rows = append(rows, Row{
					Kind:       RowSlot,
					Host:       hostName,
					Repo:       repo,
					Slot:       s,
					RunnerName: matchRunner(hostName, s.N, repo, runners),
				})
			}
		}
	}
	return rows
}

// repoExpandKey returns the Expanded-map key for a (host, repo) pair.
func repoExpandKey(host, repo string) string {
	return "repo/" + host + "/" + repo
}

// AppendPendingPoolRows is called by the render path after BuildRows to splice
// in phantom repo+slot rows for in-flight pool adds. For each pending entry
// whose repo is NOT already represented in the host's state, two rows are
// appended *under* the host header (if the host is expanded): a RowRepo and a
// RowSlot with Slot.N == 0 and UnitState == "creating". These rows aren't
// cursor-selectable (cursor uses BuildRows directly), so they're purely
// visual feedback. Pending entries for repos that already exist are silently
// dropped from the overlay since the real row already covers them.
func AppendPendingPoolRows(rows []Row, hosts map[string]*poller.HostState, expanded map[string]bool, pendingPools map[string]string) []Row {
	if len(pendingPools) == 0 {
		return rows
	}
	// Group by host so we can splice under the right host header.
	type phantom struct {
		host string
		repo string
	}
	perHost := map[string][]phantom{}
	for key, label := range pendingPools {
		if label == "" {
			continue
		}
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}
		host, repo := parts[0], parts[1]
		hs := hosts[host]
		if hs == nil {
			continue
		}
		// Repo already exists for real? Skip the phantom.
		alreadyReal := false
		for _, s := range hs.Slots {
			if s.Repo == repo {
				alreadyReal = true
				break
			}
		}
		if alreadyReal {
			continue
		}
		perHost[host] = append(perHost[host], phantom{host, repo})
	}
	if len(perHost) == 0 {
		return rows
	}
	// Sort phantoms within each host for stable rendering.
	for h := range perHost {
		sort.Slice(perHost[h], func(i, j int) bool { return perHost[h][i].repo < perHost[h][j].repo })
	}

	// Splice phantom rows into the existing list, inserting them after each
	// host's existing children (so they appear at the bottom of the host's
	// section, which is the least disruptive spot visually). For collapsed
	// hosts, skip — don't surprise-expand the tree.
	out := make([]Row, 0, len(rows)+2*len(pendingPools))
	i := 0
	for i < len(rows) {
		r := rows[i]
		out = append(out, r)
		if r.Kind != RowHost {
			i++
			continue
		}
		hostName := r.Host
		// Append all of this host's real children first.
		i++
		for i < len(rows) && rows[i].Kind != RowHost {
			out = append(out, rows[i])
			i++
		}
		// Then phantoms for this host, if expanded.
		if !expanded[hostName] {
			continue
		}
		for _, p := range perHost[hostName] {
			out = append(out,
				Row{Kind: RowRepo, Host: p.host, Repo: p.repo, SlotCount: 1},
				Row{Kind: RowSlot, Host: p.host, Repo: p.repo, Slot: poller.SlotState{N: 0, UnitState: "creating"}},
			)
		}
	}
	return out
}

func matchRunner(host string, slot int, repo string, runners map[string]*poller.RepoRunners) string {
	if runners == nil || runners[repo] == nil {
		return ""
	}
	want := "bobsled-" + host + "-" + strconv.Itoa(slot)
	for _, r := range runners[repo].Runners {
		if r.Name == want {
			return r.Name
		}
	}
	return ""
}
