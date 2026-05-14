// internal/cli/host_bootstrap.go
package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/m-meyer2k/bobsled/assets"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/spf13/cobra"
)

func newHostCmd() *cobra.Command {
	host := &cobra.Command{Use: "host", Short: "Host lifecycle operations"}
	host.AddCommand(newHostBootstrapCmd())
	host.AddCommand(newHostInstallCmd())
	host.AddCommand(newHostUpgradeCmd())
	host.AddCommand(newHostRotateKeyCmd())
	host.AddCommand(newHostAddCmd())
	host.AddCommand(newHostRemoveCmd())
	return host
}

func newHostBootstrapCmd() *cobra.Command {
	var authorizedKeysPath string
	c := &cobra.Command{
		Use:   "bootstrap <host>",
		Short: "Create the bobsled user on a host (one-shot, requires admin SSH)",
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
			if host.BootstrapSSH == "" {
				return fmt.Errorf("host %q has no bootstrap_ssh set", name)
			}
			keys, err := os.ReadFile(authorizedKeysPath)
			if err != nil {
				return fmt.Errorf("read authorized_keys: %w", err)
			}

			// 1) Run bootstrap script as admin user.
			run := exec.Command("ssh", host.BootstrapSSH, "bash -s")
			run.Stdin = bytes.NewReader(assets.BootstrapScript)
			run.Stdout = os.Stdout
			run.Stderr = os.Stderr
			if err := run.Run(); err != nil {
				return fmt.Errorf("bootstrap script: %w", err)
			}

			// 2) Install operator keys into bobsled's authorized_keys.
			install := exec.Command("ssh", host.BootstrapSSH,
				"sudo install -m 0600 -o bobsled -g bobsled /dev/stdin /var/lib/bobsled/.ssh/authorized_keys")
			install.Stdin = bytes.NewReader(keys)
			install.Stdout = os.Stdout
			install.Stderr = os.Stderr
			if err := install.Run(); err != nil {
				return fmt.Errorf("install authorized_keys: %w", err)
			}
			fmt.Printf("host %s bootstrapped — subsequent operations connect as %s\n", name, host.SSH)
			return nil
		},
	}
	c.Flags().StringVar(&authorizedKeysPath, "authorized-keys",
		os.ExpandEnv("$HOME/.ssh/id_ed25519.pub"),
		"operator pubkey(s) to install on the bobsled user")
	return c
}
