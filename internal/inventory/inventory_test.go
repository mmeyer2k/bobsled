// internal/inventory/inventory_test.go
package inventory

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestInventoryRegistryDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inv.yaml")
	must(t, os.WriteFile(path, []byte(`
github:
  app_id: 1
  app_key: /tmp/key.pem
hosts:
  h1:
    ssh: bobsled@h1
    capacity: 1
`), 0o644))
	inv, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	r := inv.LoadedRegistry()
	if r.ImageDigest == "" || !strings.HasPrefix(r.ImageDigest, "sha256:") {
		t.Errorf("default image digest missing or malformed: %q", r.ImageDigest)
	}
	if r.GCInterval != "1h" {
		t.Errorf("default gc_interval = %q, want 1h", r.GCInterval)
	}
	if r.GCRetention != "336h" {
		t.Errorf("default gc_retention = %q, want 336h", r.GCRetention)
	}
	want := []Upstream{
		{Name: "docker.io", URL: "https://registry-1.docker.io"},
		{Name: "ghcr.io", URL: "https://ghcr.io"},
		{Name: "quay.io", URL: "https://quay.io"},
	}
	if !reflect.DeepEqual(r.Upstreams, want) {
		t.Errorf("default upstreams = %+v, want %+v", r.Upstreams, want)
	}
}

func TestInventoryRegistryOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inv.yaml")
	must(t, os.WriteFile(path, []byte(`
github:
  app_id: 1
  app_key: /tmp/key.pem
hosts:
  h1:
    ssh: bobsled@h1
    capacity: 1
registry:
  image_digest: sha256:deadbeef
  gc_interval: 30m
  upstreams:
    - name: docker.io
      url: https://registry-1.docker.io
    - name: extra.example.com
      url: https://extra.example.com
`), 0o644))
	inv, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	r := inv.LoadedRegistry()
	if r.ImageDigest != "sha256:deadbeef" {
		t.Errorf("image_digest = %q, want sha256:deadbeef", r.ImageDigest)
	}
	if r.GCInterval != "30m" {
		t.Errorf("gc_interval = %q, want 30m", r.GCInterval)
	}
	if r.GCRetention != "336h" {
		t.Errorf("gc_retention = %q, want default 336h", r.GCRetention)
	}
	if len(r.Upstreams) != 2 || r.Upstreams[1].Name != "extra.example.com" {
		t.Errorf("upstream override not applied: %+v", r.Upstreams)
	}
}

func TestInventoryRegistryValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantSub string
	}{
		{
			name: "upstream missing name",
			yaml: `
github:
  app_id: 1
  app_key: /tmp/key.pem
hosts:
  h1: {ssh: bobsled@h1, capacity: 1}
registry:
  upstreams:
    - name: ""
      url: https://example.com
`,
			wantSub: "registry.upstreams[0] missing name",
		},
		{
			name: "upstream missing url",
			yaml: `
github:
  app_id: 1
  app_key: /tmp/key.pem
hosts:
  h1: {ssh: bobsled@h1, capacity: 1}
registry:
  upstreams:
    - name: docker.io
      url: ""
`,
			wantSub: "registry.upstreams[0] (docker.io) missing url",
		},
		{
			name: "image_digest missing sha256 prefix",
			yaml: `
github:
  app_id: 1
  app_key: /tmp/key.pem
hosts:
  h1: {ssh: bobsled@h1, capacity: 1}
registry:
  image_digest: deadbeef
`,
			wantSub: `registry.image_digest "deadbeef" must start with sha256:`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "inv.yaml")
			must(t, os.WriteFile(path, []byte(tc.yaml), 0o644))
			_, err := Load(path)
			if err == nil {
				t.Fatalf("Load: want error containing %q, got nil", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("Load: error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func must(t *testing.T, err error) { t.Helper(); if err != nil { t.Fatal(err) } }
