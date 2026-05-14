// internal/cli/apply.go
package cli

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/ssh"
	"github.com/m-meyer2k/bobsled/internal/state"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "Reconcile every host's state to match inventory.yaml",
		RunE: func(_ *cobra.Command, _ []string) error {
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			desired := inventory.Allocate(inv)

			var wg sync.WaitGroup
			var mu sync.Mutex
			var errs []string
			for name, host := range inv.Hosts {
				name, host := name, host
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := applyHost(name, host, desired[name]); err != nil {
						mu.Lock()
						errs = append(errs, fmt.Sprintf("%s: %v", name, err))
						mu.Unlock()
					}
				}()
			}
			wg.Wait()
			if len(errs) > 0 {
				return fmt.Errorf("apply errors:\n  %s", strings.Join(errs, "\n  "))
			}
			return nil
		},
	}
}

func applyHost(name string, host inventory.Host, want *state.State) error {
	s := &ssh.Client{Target: host.SSH}

	// Touch state.yaml so flock has a file to lock. (Idempotent.)
	if _, err := s.Run("touch state.yaml"); err != nil {
		return fmt.Errorf("touch state.yaml: %w", err)
	}
	curYAML, err := s.Run("flock -x state.yaml -c 'cat state.yaml 2>/dev/null || true'")
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}
	var cur state.State
	if strings.TrimSpace(curYAML) != "" {
		if err := yaml.Unmarshal([]byte(curYAML), &cur); err != nil {
			return fmt.Errorf("parse current state: %w", err)
		}
	}
	if cur.Instances == nil {
		cur.Instances = map[int]state.Instance{}
	}
	if cur.Repos == nil {
		cur.Repos = map[string]state.RepoConfig{}
	}

	d := inventory.DiffStates(&cur, want)

	newYAML, err := yaml.Marshal(want)
	if err != nil {
		return err
	}
	if err := s.PutBytes(newYAML, ".state.yaml.tmp"); err != nil {
		return err
	}
	if _, err := s.Run("flock -x state.yaml -c 'mv .state.yaml.tmp state.yaml'"); err != nil {
		return err
	}

	sort.Ints(d.Removed)
	for _, slot := range d.Removed {
		if _, err := s.Run(fmt.Sprintf("systemctl --user disable --now bobsled@%d", slot)); err != nil {
			return fmt.Errorf("disable slot %d: %w", slot, err)
		}
	}
	sort.Ints(d.Changed)
	for _, slot := range d.Changed {
		if _, err := s.Run(fmt.Sprintf("systemctl --user restart bobsled@%d", slot)); err != nil {
			return fmt.Errorf("restart slot %d: %w", slot, err)
		}
	}
	sort.Ints(d.Added)
	for _, slot := range d.Added {
		if _, err := s.Run(fmt.Sprintf("systemctl --user enable --now bobsled@%d", slot)); err != nil {
			return fmt.Errorf("enable slot %d: %w", slot, err)
		}
	}
	fmt.Printf("[%s] +%d -%d ~%d\n", name, len(d.Added), len(d.Removed), len(d.Changed))
	return nil
}
