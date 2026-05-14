// internal/inventory/write_test.go
package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWrite_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inv.yaml")
	in := sampleInv()
	require.NoError(t, Write(path, in))

	got, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, in.GitHub, got.GitHub)
	require.Equal(t, in.Hosts, got.Hosts)
	require.Equal(t, in.Pools, got.Pools)
}

func TestWrite_NoLeftoverTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inv.yaml")
	require.NoError(t, Write(path, sampleInv()))
	ents, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, ents, 1)
	require.Equal(t, "inv.yaml", ents[0].Name())
}
