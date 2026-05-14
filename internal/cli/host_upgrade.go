// internal/cli/host_upgrade.go
package cli

import (
	"fmt"
	"strings"

	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/registry"
	"github.com/m-meyer2k/bobsled/internal/ssh"
	"github.com/spf13/cobra"
)

func newHostUpgradeCmd() *cobra.Command {
	var (
		mintBinary     string
		imageDigest    string
		registryDigest string
	)
	c := &cobra.Command{
		Use:   "upgrade <host>",
		Short: "Replace mint binary and/or image digest; units pick it up at next restart",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			host, ok := inv.Hosts[name]
			if !ok {
				return fmt.Errorf("unknown host %q", name)
			}
			s := &ssh.Client{Target: host.SSH}
			if mintBinary != "" {
				if err := s.PutFile(mintBinary, ".local/bin/bobsled-mint"); err != nil {
					return err
				}
				if _, err := s.Run("chmod 0755 .local/bin/bobsled-mint"); err != nil {
					return err
				}
			}
			if imageDigest != "" {
				if err := s.PutBytes([]byte(fmt.Sprintf("BOBSLED_IMAGE_DIGEST=%s\n", imageDigest)), ".image-digest.env.tmp"); err != nil {
					return err
				}
				if _, err := s.Run("mv .image-digest.env.tmp image-digest.env"); err != nil {
					return err
				}
			}
			if registryDigest != "" {
				if !strings.HasPrefix(registryDigest, "sha256:") {
					return fmt.Errorf("--registry-digest must start with sha256:")
				}
				line := fmt.Sprintf("BOBSLED_REGISTRY_DIGEST=%s\n", registryDigest)
				if err := s.PutBytes([]byte(line), ".registry-image-digest.env.tmp"); err != nil {
					return err
				}
				if _, err := s.Run("mv .registry-image-digest.env.tmp registry-image-digest.env"); err != nil {
					return err
				}
			}
			// Re-render registry config from inventory and push. Cheap and
			// idempotent; lets operators apply registry.* changes without
			// re-running host install. If config bytes change, restart.
			reg := inv.LoadedRegistry()
			regCfg, err := registry.RenderConfig(reg)
			if err != nil {
				return fmt.Errorf("render registry config: %w", err)
			}
			if err := s.PutBytes(regCfg, "registry-config.json"); err != nil {
				return err
			}
			regsConf, err := registry.RenderRegistriesConf(reg)
			if err != nil {
				return fmt.Errorf("render registries.conf: %w", err)
			}
			if err := s.PutBytes(regsConf, "registries.conf"); err != nil {
				return err
			}
			if _, err := s.Run("systemctl --user restart bobsled-registry.service"); err != nil {
				return fmt.Errorf("restart registry after config update: %w", err)
			}
			if _, err := s.Run("systemctl --user daemon-reload"); err != nil {
				return err
			}
			fmt.Printf("host %s upgraded (running units pick up changes on next restart)\n", name)
			return nil
		},
	}
	c.Flags().StringVar(&mintBinary, "mint-binary", "", "replacement bobsled-mint binary")
	c.Flags().StringVar(&imageDigest, "image-digest", "", "new wrapper image digest")
	c.Flags().StringVar(&registryDigest, "registry-digest", "", "new zot image digest (sha256:...); restarts the registry on this host")
	return c
}
