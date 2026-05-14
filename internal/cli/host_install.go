// internal/cli/host_install.go
package cli

import (
	"fmt"

	"github.com/m-meyer2k/bobsled/assets"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/ssh"
	"github.com/spf13/cobra"
)

func newHostInstallCmd() *cobra.Command {
	var (
		mintBinary  string
		imageDigest string
		appKeyPath  string
	)
	c := &cobra.Command{
		Use:   "install <host>",
		Short: "Push mint binary, systemd unit, config, app key, and image digest to a host",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			host, ok := inv.Hosts[name]
			if !ok {
				return fmt.Errorf("host %q not in inventory", name)
			}
			s := &ssh.Client{Target: host.SSH}

			if err := s.PutFile(mintBinary, ".local/bin/bobsled-mint"); err != nil {
				return err
			}
			if _, err := s.Run("chmod 0755 .local/bin/bobsled-mint"); err != nil {
				return err
			}
			if err := s.PutBytes(assets.SystemdUnit, ".config/systemd/user/bobsled@.service"); err != nil {
				return err
			}
			cfg := fmt.Sprintf(
				"app_id: %d\napp_key_path: /var/lib/bobsled/app-key.pem\nhost_label: %s\n",
				inv.GitHub.AppID, name,
			)
			if err := s.PutBytes([]byte(cfg), "config.yaml"); err != nil {
				return err
			}
			keyLocal := appKeyPath
			if keyLocal == "" {
				keyLocal = expandHome(inv.GitHub.AppKey)
			}
			if err := s.PutFile(keyLocal, "app-key.pem"); err != nil {
				return err
			}
			if _, err := s.Run("chmod 0600 app-key.pem"); err != nil {
				return err
			}
			env := fmt.Sprintf("BOBSLED_IMAGE_DIGEST=%s\n", imageDigest)
			if err := s.PutBytes([]byte(env), "image-digest.env"); err != nil {
				return err
			}
			if _, err := s.Run("systemctl --user daemon-reload"); err != nil {
				return err
			}
			fmt.Printf("host %s installed (image=%s)\n", name, imageDigest)
			return nil
		},
	}
	c.Flags().StringVar(&mintBinary, "mint-binary", "./bin/bobsled-mint", "path to bobsled-mint binary")
	c.Flags().StringVar(&imageDigest, "image-digest", "", "wrapper image digest (sha256:...) — required")
	c.Flags().StringVar(&appKeyPath, "app-key", "", "override path to App private key")
	_ = c.MarkFlagRequired("image-digest")
	return c
}
