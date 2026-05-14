// internal/cli/cache.go
package cli

import (
	"fmt"
	"strings"

	"github.com/m-meyer2k/bobsled/internal/cache"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/ssh"
	"github.com/spf13/cobra"
)

func newCacheCmd() *cobra.Command {
	c := &cobra.Command{Use: "cache", Short: "Manage per-(slot, repo) caches"}
	c.AddCommand(newCacheResetCmd())
	c.AddCommand(newCacheGcCmd())
	return c
}

func newCacheResetCmd() *cobra.Command {
	var (
		hostName string
		slot     int
		repo     string
	)
	c := &cobra.Command{
		Use:   "reset",
		Short: "Wipe a slot's cache, one repo within a slot, or all caches on a host",
		RunE: func(_ *cobra.Command, _ []string) error {
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			host, ok := inv.Hosts[hostName]
			if !ok {
				return fmt.Errorf("unknown host %q", hostName)
			}
			s := &ssh.Client{Target: host.SSH}
			var target string
			switch {
			case slot > 0 && repo != "":
				target = fmt.Sprintf(".cache/bobsled/slots/%d/%s/", slot, cache.RepoSlug(repo))
			case slot > 0:
				target = fmt.Sprintf(".cache/bobsled/slots/%d/", slot)
			default:
				target = ".cache/bobsled/slots/"
			}
			if strings.Contains(target, "..") {
				return fmt.Errorf("refusing to wipe suspicious path %q", target)
			}
			if _, err := s.Run("rm -rf '" + target + "'"); err != nil {
				return err
			}
			fmt.Printf("wiped %s on %s\n", target, hostName)
			return nil
		},
	}
	c.Flags().StringVar(&hostName, "host", "", "host (required)")
	c.Flags().IntVar(&slot, "slot", 0, "slot number (optional)")
	c.Flags().StringVar(&repo, "repo", "", "repo within slot (optional)")
	_ = c.MarkFlagRequired("host")
	return c
}

func newCacheGcCmd() *cobra.Command {
	var hostName string
	c := &cobra.Command{
		Use:   "gc",
		Short: "Remove non-current repo dirs to reclaim space",
		RunE: func(_ *cobra.Command, _ []string) error {
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			host, ok := inv.Hosts[hostName]
			if !ok {
				return fmt.Errorf("unknown host %q", hostName)
			}
			s := &ssh.Client{Target: host.SSH}
			script := `
set -eu
for slotdir in $HOME/.cache/bobsled/slots/*/; do
    [ -d "$slotdir" ] || continue
    cur=$(readlink "$slotdir/current" 2>/dev/null || echo "")
    for d in "$slotdir"*/; do
        name=$(basename "$d")
        [ "$name" = "current" ] && continue
        [ "$name" = "$cur" ] && continue
        echo "rm $slotdir$name"
        rm -rf "$d"
    done
done
`
			out, err := s.Run(script)
			fmt.Print(out)
			return err
		},
	}
	c.Flags().StringVar(&hostName, "host", "", "host (required)")
	_ = c.MarkFlagRequired("host")
	return c
}
