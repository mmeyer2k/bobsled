// internal/cli/shared.go
package cli

import (
	"fmt"

	"github.com/m-meyer2k/bobsled/assets"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/registry"
	"github.com/m-meyer2k/bobsled/internal/ssh"
)

// installToHost performs the same on-host install that `bobsled host install`
// does, exposed as a function so other commands (host add) can call it.
func installToHost(sshTarget, mintBinary, imageDigest, appKey string, appID int64, hostLabel string, reg *inventory.Registry) error {
	s := &ssh.Client{Target: sshTarget}
	if err := s.PutFile(mintBinary, ".local/bin/bobsled-mint"); err != nil {
		return err
	}
	if _, err := s.Run("chmod 0755 .local/bin/bobsled-mint"); err != nil {
		return err
	}
	if err := s.PutBytes(assets.SystemdUnit, ".config/systemd/user/bobsled@.service"); err != nil {
		return err
	}
	if err := s.PutBytes(assets.RegistryUnit, ".config/systemd/user/bobsled-registry.service"); err != nil {
		return err
	}

	cfg := fmt.Sprintf(
		"app_id: %d\napp_key_path: /var/lib/bobsled/app-key.pem\nhost_label: %s\n",
		appID, hostLabel,
	)
	if err := s.PutBytes([]byte(cfg), "config.yaml"); err != nil {
		return err
	}
	if err := s.PutFile(appKey, "app-key.pem"); err != nil {
		return err
	}
	if _, err := s.Run("chmod 0600 app-key.pem"); err != nil {
		return err
	}
	env := fmt.Sprintf("BOBSLED_IMAGE_DIGEST=%s\n", imageDigest)
	if err := s.PutBytes([]byte(env), "image-digest.env"); err != nil {
		return err
	}

	// Registry artifacts.
	regCfg, err := registry.RenderConfig(reg)
	if err != nil {
		return fmt.Errorf("render registry config: %w", err)
	}
	if err := s.PutBytes(regCfg, "registry-config.json"); err != nil {
		return err
	}
	regsConf, err := registry.RenderRegistriesConf(reg)
	if err != nil {
		return fmt.Errorf("render registries.conf: %w", err)
	}
	if err := s.PutBytes(regsConf, "registries.conf"); err != nil {
		return err
	}
	regEnv := fmt.Sprintf("BOBSLED_REGISTRY_DIGEST=%s\n", reg.ImageDigest)
	if err := s.PutBytes([]byte(regEnv), "registry-image-digest.env"); err != nil {
		return err
	}

	if _, err := s.Run("systemctl --user daemon-reload"); err != nil {
		return err
	}
	if _, err := s.Run("systemctl --user enable --now bobsled-registry.service"); err != nil {
		return fmt.Errorf("enable registry: %w", err)
	}
	return nil
}

// runApply mirrors the reconcile flow of `bobsled apply` but takes the
// inventory path explicitly. Used by `host add` after inventory mutation.
func runApply(invPath string) error {
	inv, err := inventory.Load(invPath)
	if err != nil {
		return err
	}
	desired := inventory.Allocate(inv)
	for name, host := range inv.Hosts {
		if err := applyHost(name, host, desired[name]); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
