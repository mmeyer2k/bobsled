// internal/cli/host_rotate_key.go
package cli

import (
	"fmt"

	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/ssh"
	"github.com/spf13/cobra"
)

func newHostRotateKeyCmd() *cobra.Command {
	var keyPath string
	c := &cobra.Command{
		Use:   "rotate-key <host>",
		Short: "Replace the GitHub App private key on a host",
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
			if err := s.PutFile(expandHome(keyPath), ".app-key.pem.tmp"); err != nil {
				return err
			}
			if _, err := s.Run("chmod 0600 .app-key.pem.tmp && mv .app-key.pem.tmp app-key.pem"); err != nil {
				return err
			}
			fmt.Printf("rotated key on %s\n", name)
			return nil
		},
	}
	c.Flags().StringVar(&keyPath, "key", "", "local path to new App private key (required)")
	_ = c.MarkFlagRequired("key")
	return c
}
