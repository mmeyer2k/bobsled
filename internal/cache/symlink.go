// internal/cache/symlink.go
package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RepoSlug encodes "owner/repo" as a single filesystem-safe directory name.
func RepoSlug(repo string) string { return strings.ReplaceAll(repo, "/", "--") }

// EnsureCurrent creates slotDir/<slug>/ if absent and atomically repoints
// slotDir/current at it via a same-dir rename of a temp symlink.
func EnsureCurrent(slotDir, repo string) error {
	if err := os.MkdirAll(slotDir, 0o700); err != nil {
		return fmt.Errorf("mkdir slot: %w", err)
	}
	slug := RepoSlug(repo)
	if err := os.MkdirAll(filepath.Join(slotDir, slug), 0o700); err != nil {
		return fmt.Errorf("mkdir repo dir: %w", err)
	}
	cur := filepath.Join(slotDir, "current")
	if existing, err := os.Readlink(cur); err == nil && existing == slug {
		return nil
	}
	tmp := filepath.Join(slotDir, ".current.tmp")
	_ = os.Remove(tmp)
	if err := os.Symlink(slug, tmp); err != nil {
		return fmt.Errorf("create temp symlink: %w", err)
	}
	if err := os.Rename(tmp, cur); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename symlink: %w", err)
	}
	return nil
}
