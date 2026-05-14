// internal/runner/mint.go
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/m-meyer2k/bobsled/internal/cache"
	"github.com/m-meyer2k/bobsled/internal/config"
	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/state"
)

type Options struct {
	ConfigPath string
	StatePath  string
	Instance   int
	OutputPath string
	CacheRoot  string
	APIBase    string // override config.GitHubAPI (for tests)
}

func Mint(ctx context.Context, opts Options) error {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	st, err := state.Load(opts.StatePath)
	if err != nil {
		return fmt.Errorf("state: %w", err)
	}
	inst, ok := st.Instances[opts.Instance]
	if !ok {
		return fmt.Errorf("state: no entry for instance %d", opts.Instance)
	}
	repoCfg, ok := st.Repos[inst.Repo]
	if !ok {
		return fmt.Errorf("state: no repo entry for %q", inst.Repo)
	}

	slotDir := filepath.Join(opts.CacheRoot, "slots", fmt.Sprintf("%d", opts.Instance))
	if err := cache.EnsureCurrent(slotDir, inst.Repo); err != nil {
		return fmt.Errorf("cache: %w", err)
	}

	apiBase := opts.APIBase
	if apiBase == "" {
		apiBase = cfg.GitHubAPI
	}
	c := &ghapp.Client{
		APIBase: apiBase, AppID: cfg.AppID, KeyPath: cfg.AppKeyPath,
		HTTP: &http.Client{Timeout: 30 * time.Second}, Now: time.Now,
	}
	name := fmt.Sprintf("bobsled-%s-%d", cfg.HostLabel, opts.Instance)
	jit, err := c.GenerateJITConfig(ctx, inst.Repo, ghapp.JITRequest{
		Name: name, Labels: append([]string(nil), repoCfg.Labels...),
	})
	if err != nil && is409Conflict(err) {
		// Orphan registration from a previous run. Find it, delete it, retry.
		runners, listErr := c.ListRepoRunners(ctx, inst.Repo)
		if listErr != nil {
			return fmt.Errorf("jit: 409 conflict, list runners to recover: %w (original: %v)", listErr, err)
		}
		for _, r := range runners {
			if r.Name == name {
				if delErr := c.DeleteRepoRunner(ctx, inst.Repo, r.ID); delErr != nil {
					return fmt.Errorf("jit: 409 conflict, delete orphan %d: %w (original: %v)", r.ID, delErr, err)
				}
				break
			}
		}
		// Retry once.
		jit, err = c.GenerateJITConfig(ctx, inst.Repo, ghapp.JITRequest{
			Name: name, Labels: append([]string(nil), repoCfg.Labels...),
		})
	}
	if err != nil {
		return fmt.Errorf("jit: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0o700); err != nil {
		return err
	}
	tmp := opts.OutputPath + ".tmp"
	b, _ := json.Marshal(jit)
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, opts.OutputPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func is409Conflict(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "409") || strings.Contains(s, "Already exists")
}
