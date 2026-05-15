// internal/cli/slot.go
package cli

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/ssh"
	"github.com/m-meyer2k/bobsled/internal/state"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newSlotCmd() *cobra.Command {
	c := &cobra.Command{Use: "slot", Short: "Per-slot operations"}
	c.AddCommand(newSlotRemoveCmd())
	c.AddCommand(newSlotEnableCmd())
	return c
}

func newSlotEnableCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "enable <host> <slot>",
		Short: "Re-enable a disabled slot (re-arm the unit + start the runner)",
		Long: "Adds the bobsled@N symlink back to default.target.wants and starts " +
			"the unit. ExecStartPre re-runs mint to get a fresh JIT config. The " +
			"slot must already exist in state.yaml — to bring back a fully " +
			"deleted slot, use `bobsled scale` or the TUI's `+1 slot`.",
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			hostName := args[0]
			slot, err := strconv.Atoi(args[1])
			if err != nil || slot <= 0 {
				return fmt.Errorf("slot must be a positive integer, got %q", args[1])
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

			curYAML, err := s.Run("flock -x state.yaml -c 'cat state.yaml 2>/dev/null || true'")
			if err != nil {
				return fmt.Errorf("read state: %w", err)
			}
			var cur state.State
			if strings.TrimSpace(curYAML) != "" {
				if err := yaml.Unmarshal([]byte(curYAML), &cur); err != nil {
					return fmt.Errorf("parse state: %w", err)
				}
			}
			if _, ok := cur.Instances[slot]; !ok {
				return fmt.Errorf("slot %d not present on host %q — use `bobsled scale` or the TUI's `+1 slot` to add a new slot", slot, hostName)
			}
			if _, err := s.Run(fmt.Sprintf("systemctl --user enable --now bobsled@%d", slot)); err != nil {
				return fmt.Errorf("enable slot %d: %w", slot, err)
			}
			fmt.Printf("enabled slot %d on %s\n", slot, hostName)
			return nil
		},
	}
	return c
}

func newSlotRemoveCmd() *cobra.Command {
	var (
		timeout   time.Duration
		pollEvery time.Duration
	)
	c := &cobra.Command{
		Use:   "remove <host> <slot>",
		Short: "Gracefully remove a single slot from a host's pool",
		Long: "Soft-drains the slot (systemctl --user disable so any in-flight " +
			"job finishes), waits until the unit is inactive, then removes the " +
			"slot from state.yaml and decrements the pool count in inventory.yaml. " +
			"Does NOT run apply — re-running apply would diff the now-sparse " +
			"layout. Subsequent applies preserve the sparse indices via " +
			"AllocateWithCurrent.",
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			hostName := args[0]
			slot, err := strconv.Atoi(args[1])
			if err != nil || slot <= 0 {
				return fmt.Errorf("slot must be a positive integer, got %q", args[1])
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

			// 1) Load remote state.yaml.
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
					return fmt.Errorf("parse state: %w", err)
				}
			}
			if cur.Instances == nil {
				cur.Instances = map[int]state.Instance{}
			}
			if cur.Repos == nil {
				cur.Repos = map[string]state.RepoConfig{}
			}
			inst, ok := cur.Instances[slot]
			if !ok {
				return fmt.Errorf("slot %d not present on host %q", slot, hostName)
			}
			repo := inst.Repo

			// 2) Drain: soft-disable if currently enabled. Always idempotent.
			enabled, err := slotIsEnabled(s, slot)
			if err != nil {
				return fmt.Errorf("check enabled state: %w", err)
			}
			if enabled {
				if _, err := s.Run(fmt.Sprintf("systemctl --user disable bobsled@%d", slot)); err != nil {
					return fmt.Errorf("disable slot %d: %w", slot, err)
				}
			}

			// 3) Wait until the unit is inactive/failed (job finishes).
			if err := waitSlotInactive(s, slot, timeout, pollEvery); err != nil {
				return err
			}

			// 4) Mutate state.yaml: drop the slot, prune unreferenced repo. Use
			//    the existing atomic-rename idiom over flock.
			newState, err := removeSlotFromState(&cur, slot)
			if err != nil {
				return err
			}
			newYAML, err := yaml.Marshal(newState)
			if err != nil {
				return err
			}
			if err := s.PutBytes(newYAML, ".state.yaml.tmp"); err != nil {
				return err
			}
			if _, err := s.Run("flock -x state.yaml -c 'mv .state.yaml.tmp state.yaml'"); err != nil {
				return err
			}

			// 5) Decrement the pool count in inventory.yaml. If the slot's repo
			//    isn't in any pool (manual cruft), skip silently.
			poolExists := false
			for _, p := range inv.Pools {
				if p.Repo == repo {
					poolExists = true
					break
				}
			}
			if poolExists {
				newInv, err := inventory.AdjustPool(inv, repo, -1, nil)
				if err != nil {
					return fmt.Errorf("adjust pool: %w", err)
				}
				if err := inventory.Write(flagInventory, newInv); err != nil {
					return err
				}
			}

			// 6) Delete the GitHub-side runner registration. The systemd unit
			//    is now stopped, but if the container was hard-killed (idle
			//    drain path) the runner stays registered as "offline" until
			//    GitHub's own reaper picks it up minutes later. Delete it now.
			//    Both calls retry up to 3 attempts (5s apart) on transient
			//    failures; if all attempts fail we warn — `bobsled gc` mops up.
			runnerName := fmt.Sprintf("bobsled-%s-%d", hostName, slot)
			if repo != "" {
				client := &ghapp.Client{
					APIBase: "https://api.github.com",
					AppID:   inv.GitHub.AppID,
					KeyPath: expandHome(inv.GitHub.AppKey),
					HTTP:    &http.Client{Timeout: 30 * time.Second},
					Now:     time.Now,
				}
				ctx := context.Background()
				var runners []ghapp.RunnerRef
				if lerr := retryAPI(func() error {
					var e error
					runners, e = client.ListRepoRunners(ctx, repo)
					return e
				}, 3, 5*time.Second); lerr != nil {
					fmt.Printf("warning: list github runners for %s: %v\n", repo, lerr)
				} else {
					for _, r := range runners {
						if r.Name != runnerName {
							continue
						}
						runner := r
						if derr := retryAPI(func() error {
							return client.DeleteRepoRunner(ctx, repo, runner.ID)
						}, 3, 5*time.Second); derr != nil {
							fmt.Printf("warning: delete github runner %s id=%d: %v\n", runner.Name, runner.ID, derr)
						}
						break
					}
				}
			}

			fmt.Printf("removed slot %d (%s) from %s\n", slot, repo, hostName)
			return nil
		},
	}
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "max time to wait for in-flight job to finish")
	c.Flags().DurationVar(&pollEvery, "poll", 5*time.Second, "interval between is-active checks")
	return c
}

// slotIsEnabled reports whether `bobsled@<n>` is currently enabled (in
// systemd's "is-enabled" sense). A non-zero exit just means "not enabled" —
// not an error condition for our flow.
func slotIsEnabled(s *ssh.Client, slot int) (bool, error) {
	out, _ := s.Run(fmt.Sprintf("systemctl --user is-enabled bobsled@%d 2>&1 || true", slot))
	return strings.Contains(out, "enabled"), nil
}

// waitSlotInactive polls `systemctl --user is-active bobsled@N` until it
// returns "inactive" or "failed", or the timeout elapses.
func waitSlotInactive(s *ssh.Client, slot int, timeout, poll time.Duration) error {
	deadline := time.Now().Add(timeout)
	cmd := fmt.Sprintf("systemctl --user is-active bobsled@%d 2>&1 || true", slot)
	for {
		out, _ := s.Run(cmd)
		state := strings.TrimSpace(out)
		if state == "inactive" || state == "failed" || state == "" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("slot %d still %q after %s — aborting", slot, state, timeout)
		}
		time.Sleep(poll)
	}
}

// retryAPI runs fn up to attempts times, sleeping interval between tries.
// Returns the last error if every attempt fails; nil on first success.
func retryAPI(fn func() error, attempts int, interval time.Duration) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i < attempts-1 {
			time.Sleep(interval)
		}
	}
	return err
}

// removeSlotFromState returns a copy of cur with the given slot removed from
// Instances. If no remaining instance references the slot's repo, that repo's
// entry in Repos is also pruned. Does not mutate cur.
func removeSlotFromState(cur *state.State, slot int) (*state.State, error) {
	inst, ok := cur.Instances[slot]
	if !ok {
		return nil, fmt.Errorf("slot %d not present in state", slot)
	}
	out := &state.State{
		Repos:     make(map[string]state.RepoConfig, len(cur.Repos)),
		Instances: make(map[int]state.Instance, len(cur.Instances)-1),
	}
	for k, v := range cur.Repos {
		out.Repos[k] = state.RepoConfig{Labels: append([]string(nil), v.Labels...)}
	}
	for k, v := range cur.Instances {
		if k == slot {
			continue
		}
		out.Instances[k] = v
	}
	// Prune the repo if no other slot references it.
	stillUsed := false
	for _, v := range out.Instances {
		if v.Repo == inst.Repo {
			stillUsed = true
			break
		}
	}
	if !stillUsed {
		delete(out.Repos, inst.Repo)
	}
	return out, nil
}
