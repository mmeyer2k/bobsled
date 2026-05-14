// internal/cli/gc.go
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

func newGcCmd() *cobra.Command {
	var dryRun bool
	c := &cobra.Command{
		Use:   "gc",
		Short: "Delete GitHub-side runners not represented in inventory",
		RunE: func(_ *cobra.Command, _ []string) error {
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			client := &ghapp.Client{
				APIBase: "https://api.github.com",
				AppID:   inv.GitHub.AppID,
				KeyPath: expandHome(inv.GitHub.AppKey),
				HTTP:    &http.Client{Timeout: 30 * time.Second},
				Now:     time.Now,
			}
			ctx := context.Background()
			desired := inventory.Allocate(inv)

			for _, pool := range inv.Pools {
				runners, err := client.ListRepoRunners(ctx, pool.Repo)
				if err != nil {
					return fmt.Errorf("%s: %w", pool.Repo, err)
				}
				expected := map[string]bool{}
				for hostName, st := range desired {
					for slot, inst := range st.Instances {
						if inst.Repo == pool.Repo {
							expected[fmt.Sprintf("bobsled-%s-%d", hostName, slot)] = true
						}
					}
				}
				for _, r := range runners {
					if !strings.HasPrefix(r.Name, "bobsled-") {
						continue
					}
					if expected[r.Name] {
						continue
					}
					if dryRun {
						fmt.Printf("[dry-run] would delete %s id=%d (repo=%s)\n", r.Name, r.ID, pool.Repo)
						continue
					}
					if err := client.DeleteRepoRunner(ctx, pool.Repo, r.ID); err != nil {
						fmt.Printf("delete %s: %v\n", r.Name, err)
						continue
					}
					fmt.Printf("deleted %s (repo=%s)\n", r.Name, pool.Repo)
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be deleted")
	return c
}
