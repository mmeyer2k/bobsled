// internal/tui/rows.go
package tui

import (
	"sort"
	"strconv"

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
		// Group slots by repo.
		byRepo := map[string][]poller.SlotState{}
		for _, s := range h.Slots {
			byRepo[s.Repo] = append(byRepo[s.Repo], s)
		}
		repoNames := make([]string, 0, len(byRepo))
		for r := range byRepo {
			repoNames = append(repoNames, r)
		}
		sort.Strings(repoNames)
		for _, repo := range repoNames {
			slots := byRepo[repo]
			sort.Slice(slots, func(i, j int) bool { return slots[i].N < slots[j].N })
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
