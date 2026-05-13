// internal/state/state_test.go
package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	s := &State{
		Repos:     map[string]RepoConfig{"acme/foo": {Labels: []string{"bobsled", "podman"}}},
		Instances: map[int]Instance{1: {Repo: "acme/foo"}, 2: {Repo: "acme/foo"}},
	}
	require.NoError(t, Write(path, s))
	got, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, s.Repos, got.Repos)
	require.Equal(t, s.Instances, got.Instances)
}

func TestLoad_MissingFile_ReturnsEmpty(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Empty(t, s.Instances)
	require.Empty(t, s.Repos)
}

func TestWrite_NoLeftoverTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	require.NoError(t, Write(path, &State{Repos: map[string]RepoConfig{"x/y": {Labels: []string{"bobsled"}}}}))
	ents, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, ents, 1)
	require.Equal(t, "state.yaml", ents[0].Name())
}
