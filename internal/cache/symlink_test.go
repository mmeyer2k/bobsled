// internal/cache/symlink_test.go
package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureCurrent_CreatesAndPoints(t *testing.T) {
	slot := filepath.Join(t.TempDir(), "slots", "7")
	require.NoError(t, EnsureCurrent(slot, "acme/foo"))
	st, err := os.Stat(filepath.Join(slot, "acme--foo"))
	require.NoError(t, err)
	require.True(t, st.IsDir())
	target, err := os.Readlink(filepath.Join(slot, "current"))
	require.NoError(t, err)
	require.Equal(t, "acme--foo", target)
}

func TestEnsureCurrent_SwitchesReposPreservesOld(t *testing.T) {
	slot := filepath.Join(t.TempDir(), "slots", "7")
	require.NoError(t, EnsureCurrent(slot, "acme/foo"))
	require.NoError(t, EnsureCurrent(slot, "acme/bar"))
	target, err := os.Readlink(filepath.Join(slot, "current"))
	require.NoError(t, err)
	require.Equal(t, "acme--bar", target)
	_, err = os.Stat(filepath.Join(slot, "acme--foo"))
	require.NoError(t, err)
}

func TestRepoSlug(t *testing.T) {
	require.Equal(t, "acme--foo", RepoSlug("acme/foo"))
	require.Equal(t, "octocat--hello-world", RepoSlug("octocat/hello-world"))
}
