// internal/cli/root.go
package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var flagInventory string
var osUserHomeDir = os.UserHomeDir

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "bobsled",
		Short:         "Orchestrator for ephemeral GitHub Actions runners",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().StringVarP(&flagInventory, "inventory", "i", "inventory.yaml", "path to inventory.yaml")
	root.AddCommand(newHostCmd())
	root.AddCommand(newApplyCmd())
	root.AddCommand(newDrainCmd())
	root.AddCommand(newLsCmd())
	root.AddCommand(newGcCmd())
	root.AddCommand(newCacheCmd())
	root.AddCommand(newRegistryCmd())
	root.AddCommand(newImageCmd())
	root.AddCommand(newScaleCmd())
	root.AddCommand(newTuiCmd())
	root.AddCommand(newRepoCmd())
	return root
}

func expandHome(p string) string {
	if len(p) >= 2 && p[:2] == "~/" {
		if h, err := osUserHomeDir(); err == nil {
			return h + p[1:]
		}
	}
	return p
}
