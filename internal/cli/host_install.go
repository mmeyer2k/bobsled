// internal/cli/host_install.go
package cli

import (
	"fmt"

	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/spf13/cobra"
)

func newHostInstallCmd() *cobra.Command {
	var (
		mintBinary  string
		imageDigest string
		appKeyPath  string
	)
	c := &cobra.Command{
		Use:   "install <host>",
		Short: "Push mint binary, systemd units (wrapper + registry), configs, app key, and image digests to a host",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			host, ok := inv.Hosts[name]
			if !ok {
				return fmt.Errorf("host %q not in inventory", name)
			}
			keyLocal := appKeyPath
			if keyLocal == "" {
				keyLocal = expandHome(inv.GitHub.AppKey)
			}
			if err := installToHost(host.SSH, mintBinary, imageDigest, keyLocal, inv.GitHub.AppID, name, inv.LoadedRegistry()); err != nil {
				return err
			}
			fmt.Printf("host %s installed (image=%s)\n", name, imageDigest)
			return nil
		},
	}
	c.Flags().StringVar(&mintBinary, "mint-binary", "./bin/bobsled-mint", "path to bobsled-mint binary")
	c.Flags().StringVar(&imageDigest, "image-digest", "", "wrapper image digest (sha256:...) — required")
	c.Flags().StringVar(&appKeyPath, "app-key", "", "override path to App private key")
	_ = c.MarkFlagRequired("image-digest")
	return c
}
