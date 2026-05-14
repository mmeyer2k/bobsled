// internal/cli/host_add.go
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

func newHostAddCmd() *cobra.Command {
	var (
		sshT, bootstrapSSH, repo        string
		appKey, mintBinary, imageDigest string
		capacity, count                  int
		replace                          bool
		authorizedKeys                   string
	)
	c := &cobra.Command{
		Use:   "add <name>",
		Short: "Bootstrap + install + add to inventory in one shot",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			if _, exists := inv.Hosts[name]; exists && !replace {
				return fmt.Errorf("host %q already exists; use --replace to overwrite", name)
			}

			// 1) bootstrap
			runScript := exec.Command("ssh", bootstrapSSH, "bash -s")
			runScript.Stdin = bytes.NewReader(assets.BootstrapScript)
			runScript.Stdout = os.Stdout
			runScript.Stderr = os.Stderr
			if err := runScript.Run(); err != nil {
				return fmt.Errorf("bootstrap: %w", err)
			}
			keys, err := os.ReadFile(authorizedKeys)
			if err != nil {
				return fmt.Errorf("read authorized_keys: %w", err)
			}
			writeKeys := exec.Command("ssh", bootstrapSSH,
				"sudo install -m 0600 -o bobsled -g bobsled /dev/stdin /var/lib/bobsled/.ssh/authorized_keys")
			writeKeys.Stdin = bytes.NewReader(keys)
			writeKeys.Stdout = os.Stdout
			writeKeys.Stderr = os.Stderr
			if err := writeKeys.Run(); err != nil {
				return fmt.Errorf("install keys: %w", err)
			}

			// 2) install
			if imageDigest == "" {
				return fmt.Errorf("--image-digest required")
			}
			keyLocal := firstNonEmpty(appKey, expandHome(inv.GitHub.AppKey))
			if err := installToHost(sshT, mintBinary, imageDigest, keyLocal, inv.GitHub.AppID, name); err != nil {
				return fmt.Errorf("install: %w", err)
			}

			// 3) update inventory.yaml
			// When replacing, drop the existing entry first so AddHost succeeds.
			base := inv
			if _, exists := inv.Hosts[name]; exists && replace {
				base, err = inventory.RemoveHost(inv, name)
				if err != nil {
					return err
				}
			}
			newInv, err := inventory.AddHost(base, name, inventory.Host{
				SSH: sshT, BootstrapSSH: bootstrapSSH, Capacity: capacity,
			})
			if err != nil {
				return err
			}
			if repo != "" && count > 0 {
				newInv, err = inventory.AdjustPool(newInv, repo, count, []string{name})
				if err != nil {
					return err
				}
			}
			if err := inventory.Write(flagInventory, newInv); err != nil {
				return err
			}

			// 4) apply if a pool was added
			if repo != "" && count > 0 {
				return runApply(flagInventory)
			}
			fmt.Printf("host %s added\n", name)
			return nil
		},
	}
	c.Flags().StringVar(&sshT, "ssh", "", "SSH target after bootstrap (e.g. bobsled@host) (required)")
	c.Flags().StringVar(&bootstrapSSH, "bootstrap-ssh", "", "admin SSH target for the one-time bootstrap (required)")
	c.Flags().IntVar(&capacity, "capacity", 4, "max slots on this host")
	c.Flags().StringVar(&repo, "repo", "", "(optional) repo to add a pool entry for")
	c.Flags().IntVar(&count, "count", 0, "(optional) initial pool count when --repo is set")
	c.Flags().StringVar(&mintBinary, "mint-binary", "./bin/bobsled-mint", "local path to bobsled-mint")
	c.Flags().StringVar(&imageDigest, "image-digest", "", "wrapper image digest (sha256:...) (required)")
	c.Flags().StringVar(&appKey, "app-key", "", "override path to GitHub App private key")
	c.Flags().BoolVar(&replace, "replace", false, "allow replacing an existing host entry")
	c.Flags().StringVar(&authorizedKeys, "authorized-keys", os.ExpandEnv("$HOME/.ssh/id_ed25519.pub"), "operator pubkey to install on bobsled")
	_ = c.MarkFlagRequired("ssh")
	_ = c.MarkFlagRequired("bootstrap-ssh")
	_ = c.MarkFlagRequired("image-digest")
	return c
}
