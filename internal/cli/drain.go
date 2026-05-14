// internal/cli/drain.go
package cli

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/ssh"
	"github.com/m-meyer2k/bobsled/internal/state"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newDrainCmd() *cobra.Command {
	var (
		hostName  string
		slot      int
		pollEvery time.Duration
		timeout   time.Duration
	)
	c := &cobra.Command{
		Use:   "drain",
		Short: "Smart-drain matching units: stop idle slots immediately, soft-disable busy ones",
		Long: "Drain checks each slot's busy state via the GitHub API. " +
			"Idle slots are stopped immediately (`systemctl --user disable --now`). " +
			"Busy slots are soft-disabled (`systemctl --user disable`) so they finish " +
			"their current job and then exit naturally. " +
			"There is a sub-second race window between the busy check and the systemctl " +
			"action; this is acknowledged and acceptable.",
		RunE: func(_ *cobra.Command, _ []string) error {
			if hostName == "" {
				return fmt.Errorf("--host required")
			}
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			host, ok := inv.Hosts[hostName]
			if !ok {
				return fmt.Errorf("unknown host %q", hostName)
			}
			sc := &ssh.Client{Target: host.SSH}

			// Discover live units on the host.
			list, err := sc.Run("systemctl --user list-units 'bobsled@*' --all --no-legend --plain")
			if err != nil {
				return err
			}
			type target struct {
				N    int
				Repo string
			}
			// Read state.yaml so we know which slot serves which repo.
			stateYAML, _ := sc.Run("cat state.yaml 2>/dev/null || true")
			var st state.State
			_ = yaml.Unmarshal([]byte(stateYAML), &st)

			var targets []target
			for _, line := range strings.Split(list, "\n") {
				f := strings.Fields(line)
				if len(f) == 0 || !strings.HasPrefix(f[0], "bobsled@") {
					continue
				}
				var n int
				if _, err := fmt.Sscanf(f[0], "bobsled@%d.service", &n); err != nil || n <= 0 {
					continue
				}
				if slot > 0 && n != slot {
					continue
				}
				repo := ""
				if inst, ok := st.Instances[n]; ok {
					repo = inst.Repo
				}
				targets = append(targets, target{N: n, Repo: repo})
			}
			if len(targets) == 0 {
				fmt.Println("no matching units")
				return nil
			}

			// For the busy check we need a GitHub client. If we can't build one
			// (no app key etc.), fall back to hard-stop everywhere.
			var client *ghapp.Client
			if inv.GitHub.AppID != 0 && inv.GitHub.AppKey != "" {
				client = &ghapp.Client{
					APIBase: "https://api.github.com",
					AppID:   inv.GitHub.AppID,
					KeyPath: expandHome(inv.GitHub.AppKey),
					HTTP:    &http.Client{Timeout: 30 * time.Second},
					Now:     time.Now,
				}
			}

			// Per-repo runners cache so we hit the API at most once per repo.
			runnersByRepo := map[string][]ghapp.RunnerRef{}
			isBusy := func(repo string, n int) (bool, error) {
				runners, ok := runnersByRepo[repo]
				if !ok && client != nil {
					r, err := client.ListRepoRunners(context.Background(), repo)
					if err != nil {
						return false, err
					}
					runnersByRepo[repo] = r
					runners = r
				}
				want := fmt.Sprintf("bobsled-%s-%d", hostName, n)
				for _, r := range runners {
					if r.Name == want {
						return r.Busy, nil
					}
				}
				return false, nil // no matching runner registered → treat as idle
			}

			var stoppedNow, softDrained []int
			for _, t := range targets {
				busy := false
				if t.Repo != "" && client != nil {
					b, err := isBusy(t.Repo, t.N)
					if err != nil {
						fmt.Printf("slot %d: busy check failed (%v) — defaulting to hard stop\n", t.N, err)
					} else {
						busy = b
					}
				}
				cmd := fmt.Sprintf("systemctl --user disable --now bobsled@%d", t.N)
				if busy {
					cmd = fmt.Sprintf("systemctl --user disable bobsled@%d", t.N)
				}
				if _, err := sc.Run(cmd); err != nil {
					return fmt.Errorf("slot %d: %w", t.N, err)
				}
				if busy {
					softDrained = append(softDrained, t.N)
				} else {
					stoppedNow = append(stoppedNow, t.N)
				}
			}

			fmt.Printf("drained: stopped-now=%v soft-drained=%v\n", stoppedNow, softDrained)
			_ = pollEvery
			_ = timeout // currently unused; retained for back-compat
			_ = unitNames
			return nil
		},
	}
	c.Flags().StringVar(&hostName, "host", "", "host name from inventory (required)")
	c.Flags().IntVar(&slot, "slot", 0, "filter to a single slot (optional)")
	c.Flags().DurationVar(&pollEvery, "poll", 5*time.Second, "poll interval")
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "give up after this long")
	return c
}

func unitNames(slots []int) []string {
	out := make([]string, len(slots))
	for i, n := range slots {
		out[i] = fmt.Sprintf("bobsled@%d.service", n)
	}
	return out
}
