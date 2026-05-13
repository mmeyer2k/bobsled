// internal/inventory/inventory.go
package inventory

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Inventory struct {
	GitHub GitHubAuth      `yaml:"github"`
	Hosts  map[string]Host `yaml:"hosts"`
	Pools  []Pool          `yaml:"pools"`
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
	return nil
}
