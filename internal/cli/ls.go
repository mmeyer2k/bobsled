// internal/cli/ls.go
package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/ssh"
	"github.com/m-meyer2k/bobsled/internal/state"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "Show all slots across the fleet",
		RunE: func(_ *cobra.Command, _ []string) error {
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "HOST\tREGISTRY\tSLOT\tSTATE\tREPO")
			for name, host := range inv.Hosts {
				s := &ssh.Client{Target: host.SSH}
				regOut, _ := s.Run("systemctl --user is-active bobsled-registry.service 2>&1 || true")
				regStatus := strings.TrimSpace(regOut)
				if regStatus == "" {
					regStatus = "unknown"
				}
				list, err := s.Run("systemctl --user list-units 'bobsled@*' --all --no-legend --plain")
				if err != nil {
					fmt.Fprintf(w, "%s\t%s\t-\tERROR\t%v\n", name, regStatus, err)
					continue
				}
				stateYAML, _ := s.Run("cat state.yaml 2>/dev/null || true")
				var st state.State
				_ = yaml.Unmarshal([]byte(stateYAML), &st)
				for _, line := range strings.Split(list, "\n") {
					f := strings.Fields(line)
					if len(f) < 3 || !strings.HasPrefix(f[0], "bobsled@") {
						continue
					}
					var slot int
					_, _ = fmt.Sscanf(f[0], "bobsled@%d.service", &slot)
					repo := "?"
					if inst, ok := st.Instances[slot]; ok {
						repo = inst.Repo
					}
					fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n", name, regStatus, slot, f[2], repo)
				}
			}
			return w.Flush()
		},
	}
}
