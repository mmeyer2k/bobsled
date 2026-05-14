// internal/cli/drain.go
package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/ssh"
	"github.com/spf13/cobra"
)

func newDrainCmd() *cobra.Command {
	var (
		hostName  string
		slot      int
		pollEvery time.Duration
		timeout   time.Duration
	)
	c := &cobra.Command{
		Use:   "drain",
		Short: "Disable matching units; wait for in-flight jobs to finish",
		RunE: func(_ *cobra.Command, _ []string) error {
			if hostName == "" {
				return fmt.Errorf("--host required")
			}
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			host, ok := inv.Hosts[hostName]
			if !ok {
				return fmt.Errorf("unknown host %q", hostName)
			}
			s := &ssh.Client{Target: host.SSH}

			list, err := s.Run("systemctl --user list-units 'bobsled@*' --all --no-legend --plain")
			if err != nil {
				return err
			}
			var targets []int
			for _, line := range strings.Split(list, "\n") {
				f := strings.Fields(line)
				if len(f) == 0 || !strings.HasPrefix(f[0], "bobsled@") {
					continue
				}
				var n int
				_, _ = fmt.Sscanf(f[0], "bobsled@%d.service", &n)
				if slot > 0 && n != slot {
					continue
				}
				targets = append(targets, n)
			}
			if len(targets) == 0 {
				fmt.Println("no matching units")
				return nil
			}
			for _, n := range targets {
				if _, err := s.Run(fmt.Sprintf("systemctl --user disable bobsled@%d", n)); err != nil {
					return err
				}
			}
			deadline := time.Now().Add(timeout)
			for time.Now().Before(deadline) {
				out, _ := s.Run("systemctl --user is-active " + strings.Join(unitNames(targets), " "))
				if !strings.Contains(out, "active") {
					fmt.Println("drained")
					return nil
				}
				time.Sleep(pollEvery)
			}
			return fmt.Errorf("drain timed out after %s", timeout)
		},
	}
	c.Flags().StringVar(&hostName, "host", "", "host name from inventory (required)")
	c.Flags().IntVar(&slot, "slot", 0, "filter to a single slot (optional)")
	c.Flags().DurationVar(&pollEvery, "poll", 5*time.Second, "poll interval")
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "give up after this long")
	return c
}

func unitNames(slots []int) []string {
	out := make([]string, len(slots))
	for i, n := range slots {
		out[i] = fmt.Sprintf("bobsled@%d.service", n)
	}
	return out
}
