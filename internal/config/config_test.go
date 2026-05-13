// internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
app_id: 123456
app_key_path: /var/lib/bobsled/app-key.pem
github_api: https://api.github.com
host_label: h1
`), 0o600))

	c, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, int64(123456), c.AppID)
	require.Equal(t, "/var/lib/bobsled/app-key.pem", c.AppKeyPath)
	require.Equal(t, "https://api.github.com", c.GitHubAPI)
	require.Equal(t, "h1", c.HostLabel)
}

func TestLoad_DefaultsGitHubAPI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
app_id: 1
app_key_path: /tmp/k
host_label: h1
`), 0o600))
	c, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "https://api.github.com", c.GitHubAPI)
}

func TestLoad_MissingRequired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`app_id: 1`), 0o600))
	_, err := Load(path)
	require.Error(t, err)
}
