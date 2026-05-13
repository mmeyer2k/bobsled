// internal/state/state.go
package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type RepoConfig struct {
	Labels []string `yaml:"labels"`
}

type Instance struct {
	Repo string `yaml:"repo"`
}

type State struct {
	Repos     map[string]RepoConfig `yaml:"repos"`
	Instances map[int]Instance      `yaml:"instances"`
}

func Load(path string) (*State, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &State{Repos: map[string]RepoConfig{}, Instances: map[int]Instance{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var s State
	if err := yaml.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if s.Repos == nil {
		s.Repos = map[string]RepoConfig{}
	}
	if s.Instances == nil {
		s.Instances = map[int]Instance{}
	}
	return &s, nil
}

// Write atomically replaces path with the marshalled state via a same-dir
// rename(2). Readers see either the old or new file, never a partial write.
func Write(path string, s *State) error {
	b, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".state.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	cleanup := func() { _ = os.Remove(tmp.Name()) }
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		cleanup()
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
