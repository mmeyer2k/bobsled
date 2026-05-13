// internal/inventory/inventory_test.go
package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "inv.yaml")
	require.NoError(t, os.WriteFile(p, []byte(`
github:
  app_id: 123
  app_key: ~/keys/bobsled.pem
hosts:
  h1:
    ssh: bobsled@h1
    bootstrap_ssh: mike@h1
    capacity: 8
  h2:
    ssh: bobsled@h2
    bootstrap_ssh: mike@h2
    capacity: 4
pools:
  - repo: acme/foo
    count: 6
    labels: [bobsled, podman]
    spread: [h1, h2]
  - repo: acme/bar
    count: 2
    labels: [bobsled, podman, bar-secrets]
    spread: [h1]
`), 0o600))
	inv, err := Load(p)
	require.NoError(t, err)
	require.Equal(t, int64(123), inv.GitHub.AppID)
	require.Equal(t, 8, inv.Hosts["h1"].Capacity)
	require.Equal(t, "bobsled@h1", inv.Hosts["h1"].SSH)
	require.Equal(t, []string{"h1", "h2"}, inv.Pools[0].Spread)
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"missing app_id", "github:\n  app_key: k\nhosts: {h1: {ssh: x, capacity: 1}}\npools: []", "app_id"},
		{"unknown spread host", "github: {app_id: 1, app_key: k}\nhosts: {h1: {ssh: x, capacity: 1}}\npools: [{repo: a/b, count: 1, labels: [], spread: [h2]}]", "unknown host"},
		{"capacity exceeded", "github: {app_id: 1, app_key: k}\nhosts: {h1: {ssh: x, capacity: 1}}\npools: [{repo: a/b, count: 5, labels: [], spread: [h1]}]", "exceeds capacity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "i.yaml")
			require.NoError(t, os.WriteFile(p, []byte(tc.yaml), 0o600))
			_, err := Load(p)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}
