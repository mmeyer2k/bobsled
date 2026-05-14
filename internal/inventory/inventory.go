// internal/inventory/inventory.go
package inventory

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Inventory struct {
	GitHub   GitHubAuth      `yaml:"github"`
	Hosts    map[string]Host `yaml:"hosts"`
	Pools    []Pool          `yaml:"pools"`
	Registry *Registry       `yaml:"registry,omitempty"`
}

type Registry struct {
	ImageDigest string     `yaml:"image_digest"`
	GCInterval  string     `yaml:"gc_interval"`
	GCRetention string     `yaml:"gc_retention"`
	Upstreams   []Upstream `yaml:"upstreams"`
}

type Upstream struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// DefaultRegistryDigest is the pinned zot image. Bump when upgrading zot.
// TODO(task-13): the all-zeroes sentinel below must be replaced with the real
// ghcr.io/project-zot/zot-linux-amd64 digest as part of the smoke-script task.
const DefaultRegistryDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func defaultUpstreams() []Upstream {
	return []Upstream{
		{Name: "docker.io", URL: "https://registry-1.docker.io"},
		{Name: "ghcr.io", URL: "https://ghcr.io"},
		{Name: "quay.io", URL: "https://quay.io"},
	}
}

// LoadedRegistry returns the inventory's registry config with defaults filled in
// for any unset fields. Always returns a non-nil pointer.
func (inv *Inventory) LoadedRegistry() *Registry {
	r := Registry{
		ImageDigest: DefaultRegistryDigest,
		GCInterval:  "1h",
		GCRetention: "336h",
		Upstreams:   defaultUpstreams(),
	}
	if inv.Registry != nil {
		if inv.Registry.ImageDigest != "" {
			r.ImageDigest = inv.Registry.ImageDigest
		}
		if inv.Registry.GCInterval != "" {
			r.GCInterval = inv.Registry.GCInterval
		}
		if inv.Registry.GCRetention != "" {
			r.GCRetention = inv.Registry.GCRetention
		}
		if len(inv.Registry.Upstreams) > 0 {
			r.Upstreams = inv.Registry.Upstreams
		}
	}
	return &r
}

type GitHubAuth struct {
	AppID  int64  `yaml:"app_id"`
	AppKey string `yaml:"app_key"`
}

type Host struct {
	SSH          string `yaml:"ssh"`
	BootstrapSSH string `yaml:"bootstrap_ssh"`
	Capacity     int    `yaml:"capacity"`
}

type Pool struct {
	Repo   string   `yaml:"repo"`
	Count  int      `yaml:"count"`
	Labels []string `yaml:"labels"`
	Spread []string `yaml:"spread"`
}

func Load(path string) (*Inventory, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read inventory: %w", err)
	}
	var inv Inventory
	if err := yaml.Unmarshal(b, &inv); err != nil {
		return nil, fmt.Errorf("parse inventory: %w", err)
	}
	return &inv, inv.validate()
}

func (inv *Inventory) validate() error {
	if inv.GitHub.AppID == 0 {
		return errors.New("inventory: github.app_id required")
	}
	if inv.GitHub.AppKey == "" {
		return errors.New("inventory: github.app_key required")
	}
	if len(inv.Hosts) == 0 {
		return errors.New("inventory: at least one host required")
	}
	for name, h := range inv.Hosts {
		if h.SSH == "" {
			return fmt.Errorf("inventory: host %q missing ssh", name)
		}
		if h.Capacity <= 0 {
			return fmt.Errorf("inventory: host %q must have capacity > 0", name)
		}
	}
	for i, p := range inv.Pools {
		if p.Repo == "" {
			return fmt.Errorf("inventory: pool[%d] missing repo", i)
		}
		if p.Count <= 0 {
			return fmt.Errorf("inventory: pool[%d] (%s) must have count > 0", i, p.Repo)
		}
		cap := 0
		for _, h := range p.Spread {
			host, ok := inv.Hosts[h]
			if !ok {
				return fmt.Errorf("inventory: pool[%d] (%s) references unknown host %q", i, p.Repo, h)
			}
			cap += host.Capacity
		}
		if p.Count > cap {
			return fmt.Errorf("inventory: pool[%d] (%s) count %d exceeds capacity %d", i, p.Repo, p.Count, cap)
		}
	}
	if inv.Registry != nil {
		for i, u := range inv.Registry.Upstreams {
			if u.Name == "" {
				return fmt.Errorf("inventory: registry.upstreams[%d] missing name", i)
			}
			if u.URL == "" {
				return fmt.Errorf("inventory: registry.upstreams[%d] (%s) missing url", i, u.Name)
			}
		}
		if d := inv.Registry.ImageDigest; d != "" && !strings.HasPrefix(d, "sha256:") {
			return fmt.Errorf("inventory: registry.image_digest %q must start with sha256:", d)
		}
	}
	return nil
}
