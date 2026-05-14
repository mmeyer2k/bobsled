// internal/cli/scale.go
package cli

import (
	"fmt"
	"strings"

	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/ssh"
	"github.com/m-meyer2k/bobsled/internal/state"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newScaleCmd() *cobra.Command {
	var (
		hostName string
		repo     string
		count    int
		labels   []string
	)
	c := &cobra.Command{
		Use:   "scale",
		Short: "Set the count of runners for a (host, repo) pair",
		RunE: func(_ *cobra.Command, _ []string) error {
			if hostName == "" || repo == "" || count < 0 {
				return fmt.Errorf("--host, --repo, --count required")
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

			// Touch state.yaml so flock has something to lock.
			if _, err := s.Run("touch state.yaml"); err != nil {
				return err
			}
			curYAML, _ := s.Run("flock -x state.yaml -c 'cat state.yaml 2>/dev/null || true'")
			var cur state.State
			if strings.TrimSpace(curYAML) != "" {
				_ = yaml.Unmarshal([]byte(curYAML), &cur)
			}
			if cur.Instances == nil {
				cur.Instances = map[int]state.Instance{}
			}
			if cur.Repos == nil {
				cur.Repos = map[string]state.RepoConfig{}
			}
			for slot, inst := range cur.Instances {
				if inst.Repo == repo {
					delete(cur.Instances, slot)
				}
			}
			if len(labels) > 0 {
				cur.Repos[repo] = state.RepoConfig{Labels: labels}
			} else if _, ok := cur.Repos[repo]; !ok {
				return fmt.Errorf("no labels for repo %q in state; pass --labels", repo)
			}
			next := 1
			for i := 0; i < count; i++ {
				for {
					if _, ok := cur.Instances[next]; !ok {
						break
					}
					next++
				}
				if next > host.Capacity {
					return fmt.Errorf("slot %d exceeds host capacity %d", next, host.Capacity)
				}
				cur.Instances[next] = state.Instance{Repo: repo}
				next++
			}

			newYAML, _ := yaml.Marshal(cur)
			if err := s.PutBytes(newYAML, ".state.yaml.tmp"); err != nil {
				return err
			}
			if _, err := s.Run("flock -x state.yaml -c 'mv .state.yaml.tmp state.yaml'"); err != nil {
				return err
			}
			for slot, inst := range cur.Instances {
				if inst.Repo == repo {
					if _, err := s.Run(fmt.Sprintf("systemctl --user enable --now bobsled@%d", slot)); err != nil {
						return err
					}
				}
			}
			fmt.Printf("scaled %s on %s to %d\n", repo, hostName, count)
			return nil
		},
	}
	c.Flags().StringVar(&hostName, "host", "", "host (required)")
	c.Flags().StringVar(&repo, "repo", "", "owner/repo (required)")
	c.Flags().IntVar(&count, "count", -1, "desired count (required)")
	c.Flags().StringSliceVar(&labels, "labels", nil, "labels (default: existing repo labels)")
	return c
}
