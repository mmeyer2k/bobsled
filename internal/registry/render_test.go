package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m-meyer2k/bobsled/internal/inventory"
)

func TestRenderRegistryConfigGolden(t *testing.T) {
	r := &inventory.Registry{
		ImageDigest: "sha256:deadbeef",
		GCInterval:  "1h",
		GCRetention: "336h",
		Upstreams: []inventory.Upstream{
			{Name: "docker.io", URL: "https://registry-1.docker.io"},
			{Name: "ghcr.io", URL: "https://ghcr.io"},
			{Name: "quay.io", URL: "https://quay.io"},
		},
	}
	got, err := RenderConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "registry-config.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != strings.TrimSpace(string(want)) {
		t.Errorf("rendered config differs from golden:\n--- want\n%s\n--- got\n%s", want, got)
	}
}

func TestRenderRegistriesConfGolden(t *testing.T) {
	r := &inventory.Registry{
		Upstreams: []inventory.Upstream{
			{Name: "docker.io", URL: "https://registry-1.docker.io"},
			{Name: "ghcr.io", URL: "https://ghcr.io"},
			{Name: "quay.io", URL: "https://quay.io"},
		},
	}
	got, err := RenderRegistriesConf(r)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "registries.golden.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != strings.TrimSpace(string(want)) {
		t.Errorf("rendered registries.conf differs from golden:\n--- want\n%s\n--- got\n%s", want, got)
	}
}
