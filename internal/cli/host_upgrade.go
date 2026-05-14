// internal/cli/host_upgrade.go
package cli

import (
	"fmt"

	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/ssh"
	"github.com/spf13/cobra"
)

func newHostUpgradeCmd() *cobra.Command {
	var (
		mintBinary  string
		imageDigest string
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
			if _, err := s.Run("systemctl --user daemon-reload"); err != nil {
				return err
			}
			fmt.Printf("host %s upgraded (running units pick up changes on next restart)\n", name)
			return nil
		},
	}
	c.Flags().StringVar(&mintBinary, "mint-binary", "", "replacement bobsled-mint binary")
	c.Flags().StringVar(&imageDigest, "image-digest", "", "new wrapper image digest")
	return c
}
