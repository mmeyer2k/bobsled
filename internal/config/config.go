// internal/config/config.go
package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	AppID      int64  `yaml:"app_id"`
	AppKeyPath string `yaml:"app_key_path"`
	GitHubAPI  string `yaml:"github_api"`
	HostLabel  string `yaml:"host_label"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.GitHubAPI == "" {
		c.GitHubAPI = "https://api.github.com"
	}
	return &c, c.validate()
}

func (c *Config) validate() error {
	if c.AppID == 0 {
		return errors.New("config: app_id required")
	}
	if c.AppKeyPath == "" {
		return errors.New("config: app_key_path required")
	}
	if c.HostLabel == "" {
		return errors.New("config: host_label required")
	}
	return nil
}
