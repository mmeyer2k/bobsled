// internal/cli/host_remove.go
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/ssh"
	"github.com/spf13/cobra"
)

func newHostRemoveCmd() *cobra.Command {
	var (
		wipe         bool
		leaveRunners bool
		timeout      time.Duration
	)
	c := &cobra.Command{
		Use:   "remove <name>",
		Short: "Drain a host, remove it from inventory, optionally wipe its user",
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

			// 1) drain all slots on this host
			s := &ssh.Client{Target: host.SSH}
			if _, err := s.Run("systemctl --user disable 'bobsled@*' 2>/dev/null || true"); err != nil {
				return fmt.Errorf("disable: %w", err)
			}
			// poll until inactive
			deadline := time.Now().Add(timeout)
			for time.Now().Before(deadline) {
				out, _ := s.Run("systemctl --user is-active 'bobsled@*' 2>&1 || true")
				if !strings.Contains(out, "active") {
					break
				}
				time.Sleep(5 * time.Second)
			}

			// 2) gc orphans
			if !leaveRunners {
				self, err := os.Executable()
				if err != nil {
					self = os.Args[0]
				}
				_ = exec.Command(self, "--inventory", flagInventory, "gc").Run()
			}

			// 3) optional full wipe
			if wipe {
				wipeCmd := exec.Command("ssh", host.BootstrapSSH,
					"sudo systemctl stop user@$(id -u bobsled).service 2>/dev/null; "+
						"sudo userdel -r bobsled 2>/dev/null; "+
						"sudo rm -rf /var/lib/bobsled")
				wipeCmd.Stdout = os.Stdout
				wipeCmd.Stderr = os.Stderr
				if err := wipeCmd.Run(); err != nil {
					return fmt.Errorf("wipe: %w", err)
				}
			}

			// 4) inventory mutation
			newInv, err := inventory.RemoveHost(inv, name)
			if err != nil {
				return err
			}
			if err := inventory.Write(flagInventory, newInv); err != nil {
				return err
			}
			fmt.Printf("host %s removed\n", name)
			return nil
		},
	}
	c.Flags().BoolVar(&wipe, "wipe", false, "also userdel -r bobsled and rm -rf /var/lib/bobsled on the host")
	c.Flags().BoolVar(&leaveRunners, "leave-runners", false, "don't gc GitHub-side runners after drain")
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "max time to wait for drain")
	return c
}
