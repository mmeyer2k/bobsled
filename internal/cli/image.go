// internal/cli/image.go
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func newImageCmd() *cobra.Command {
	c := &cobra.Command{Use: "image", Short: "Build or push the wrapper image"}
	c.AddCommand(newImageBuildCmd())
	c.AddCommand(newImagePushCmd())
	return c
}

func newImageBuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "build",
		Short: "Run scripts/build-image.sh; print sha256:<digest>",
		RunE: func(_ *cobra.Command, _ []string) error {
			cmd := exec.Command("./scripts/build-image.sh")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		},
	}
}

func newImagePushCmd() *cobra.Command {
	var registry string
	c := &cobra.Command{
		Use:   "push <digest>",
		Short: "Tag the local image and push to a registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			digest := strings.TrimPrefix(args[0], "sha256:")
			localTag := "bobsled:" + digest
			remoteTag := registry + ":" + digest
			if out, err := exec.Command("podman", "tag", localTag, remoteTag).CombinedOutput(); err != nil {
				return fmt.Errorf("podman tag: %s: %w", out, err)
			}
			cmd := exec.Command("podman", "push", remoteTag)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		},
	}
	c.Flags().StringVar(&registry, "registry", "", "destination registry repo (required)")
	_ = c.MarkFlagRequired("registry")
	return c
}
