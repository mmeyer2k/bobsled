// internal/cli/registry.go
package cli

import (
	"fmt"

	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/ssh"
	"github.com/spf13/cobra"
)

func newRegistryCmd() *cobra.Command {
	r := &cobra.Command{Use: "registry", Short: "Per-host pull-through registry operations"}
	r.AddCommand(newRegistryStatusCmd())
	r.AddCommand(newRegistryRestartCmd())
	r.AddCommand(newRegistryGCCmd())
	return r
}

func registrySSH(name string) (*ssh.Client, error) {
	inv, err := inventory.Load(flagInventory)
	if err != nil {
		return nil, err
	}
	host, ok := inv.Hosts[name]
	if !ok {
		return nil, fmt.Errorf("unknown host %q", name)
	}
	return &ssh.Client{Target: host.SSH}, nil
}

func newRegistryStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <host>",
		Short: "Show registry unit state and cached image catalog",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := registrySSH(args[0])
			if err != nil {
				return err
			}
			out, err := s.Run("systemctl --user status bobsled-registry.service --no-pager 2>&1 || true")
			if err != nil {
				return err
			}
			fmt.Println(out)
			cat, err := s.Run("curl -fsS http://127.0.0.1:5000/v2/_catalog 2>&1 || true")
			if err != nil {
				return err
			}
			fmt.Println("--- catalog")
			fmt.Println(cat)
			return nil
		},
	}
}

func newRegistryRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart <host>",
		Short: "Restart the registry unit",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := registrySSH(args[0])
			if err != nil {
				return err
			}
			if _, err := s.Run("systemctl --user restart bobsled-registry.service"); err != nil {
				return err
			}
			fmt.Printf("registry restarted on %s\n", args[0])
			return nil
		},
	}
}

func newRegistryGCCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gc <host>",
		Short: "Force a registry GC sweep (restarts the unit; zot runs GC on startup)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := registrySSH(args[0])
			if err != nil {
				return err
			}
			if _, err := s.Run("systemctl --user restart bobsled-registry.service"); err != nil {
				return err
			}
			fmt.Printf("registry restarted on %s; GC will run per configured interval\n", args[0])
			return nil
		},
	}
}
