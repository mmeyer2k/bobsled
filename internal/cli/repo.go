// internal/cli/repo.go
package cli

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/spf13/cobra"
)

func newRepoCmd() *cobra.Command {
	c := &cobra.Command{Use: "repo", Short: "Add or remove a repo pool"}
	c.AddCommand(newRepoAddCmd())
	c.AddCommand(newRepoRemoveCmd())
	return c
}

func newRepoAddCmd() *cobra.Command {
	var (
		hosts  []string
		count  int
		labels []string
	)
	c := &cobra.Command{
		Use:   "add <owner/name>",
		Short: "Add a new repo pool and apply",
		Long: "Adds a pool entry to inventory.yaml and runs apply. The GitHub App " +
			"must already be installed on the repo — `bobsled repo add` does NOT " +
			"install the App.",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			repo := args[0]
			if len(hosts) == 0 {
				return fmt.Errorf("--host required (one or more)")
			}
			if count <= 0 {
				return fmt.Errorf("--count must be > 0")
			}
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			// Sanity: all named hosts exist.
			for _, h := range hosts {
				if _, ok := inv.Hosts[h]; !ok {
					return fmt.Errorf("host %q not in inventory", h)
				}
			}
			// Reject duplicate repo.
			for _, p := range inv.Pools {
				if p.Repo == repo {
					return fmt.Errorf("pool for %q already exists; use `bobsled scale` to resize", repo)
				}
			}
			newInv, err := inventory.AdjustPool(inv, repo, count, hosts)
			if err != nil {
				return err
			}
			// Override the default labels if --labels was given.
			if len(labels) > 0 {
				for i := range newInv.Pools {
					if newInv.Pools[i].Repo == repo {
						newInv.Pools[i].Labels = append([]string(nil), labels...)
					}
				}
			}
			if err := inventory.Write(flagInventory, newInv); err != nil {
				return err
			}
			if err := runApply(flagInventory); err != nil {
				return err
			}
			fmt.Printf("added pool %s (count=%d, spread=%v)\n", repo, count, hosts)
			return nil
		},
	}
	c.Flags().StringSliceVar(&hosts, "host", nil, "host(s) to spread the pool across (required, repeatable)")
	c.Flags().IntVar(&count, "count", 0, "number of runners to start (required)")
	c.Flags().StringSliceVar(&labels, "labels", nil, "override default labels (default: self-hosted,linux,x64,bobsled,podman)")
	return c
}

func newRepoRemoveCmd() *cobra.Command {
	var (
		leaveRunners bool
		timeout      string
	)
	c := &cobra.Command{
		Use:   "remove <owner/name>",
		Short: "Drain a repo's slots, optionally gc its GitHub runners, drop the pool",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			repo := args[0]
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			found := false
			var poolCount int
			var poolSpread []string
			for _, p := range inv.Pools {
				if p.Repo == repo {
					found = true
					poolCount = p.Count
					poolSpread = append([]string(nil), p.Spread...)
					break
				}
			}
			if !found {
				return fmt.Errorf("no pool for %q in inventory", repo)
			}

			// 1) Drop the pool from inventory by scaling to 0.
			newInv, err := inventory.AdjustPool(inv, repo, -poolCount, nil)
			if err != nil {
				return err
			}
			if err := inventory.Write(flagInventory, newInv); err != nil {
				return err
			}

			// 2) Reconcile — apply will see the pool gone and disable matching units.
			if err := runApply(flagInventory); err != nil {
				return err
			}

			// 3) Clean up GitHub-side runner registrations directly. Can't
			//    delegate to `bobsled gc` here: gc walks inv.Pools to decide
			//    what's expected, and the pool we want to clean up is already
			//    gone from inventory. Retry each call up to 3 times, 5s apart.
			if !leaveRunners {
				client := &ghapp.Client{
					APIBase: "https://api.github.com",
					AppID:   inv.GitHub.AppID,
					KeyPath: expandHome(inv.GitHub.AppKey),
					HTTP:    &http.Client{Timeout: 30 * time.Second},
					Now:     time.Now,
				}
				ctx := context.Background()
				var runners []ghapp.RunnerRef
				if lerr := retryAPI(func() error {
					var e error
					runners, e = client.ListRepoRunners(ctx, repo)
					return e
				}, 3, 5*time.Second); lerr != nil {
					fmt.Printf("warning: list github runners for %s: %v\n", repo, lerr)
				} else {
					for _, r := range runners {
						if !strings.HasPrefix(r.Name, "bobsled-") {
							continue
						}
						runner := r
						if derr := retryAPI(func() error {
							return client.DeleteRepoRunner(ctx, repo, runner.ID)
						}, 3, 5*time.Second); derr != nil {
							fmt.Printf("warning: delete github runner %s id=%d: %v\n", runner.Name, runner.ID, derr)
							continue
						}
						fmt.Printf("deleted github runner %s\n", runner.Name)
					}
				}
			}

			fmt.Printf("removed pool %s (was count=%d, spread=%v)\n", repo, poolCount, poolSpread)
			_ = timeout // currently unused; reserved for future drain-wait
			return nil
		},
	}
	c.Flags().BoolVar(&leaveRunners, "leave-runners", false, "skip `bobsled gc` after drop")
	c.Flags().StringVar(&timeout, "timeout", "30m", "max time to wait for drain (currently unused; apply is sync)")
	return c
}
