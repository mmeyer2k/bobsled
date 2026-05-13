// internal/ssh/ssh.go
package ssh

import (
	"bytes"
	"fmt"
	"os/exec"
)

type Client struct {
	Target string
	Extra  []string
}

func (c *Client) Run(cmd string) (string, error) {
	args := append([]string{}, c.Extra...)
	args = append(args, c.Target, cmd)
	out, err := exec.Command("ssh", args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return string(out), fmt.Errorf("ssh %s: exit status %d: %s", c.Target, ee.ExitCode(), bytes.TrimSpace(ee.Stderr))
		}
		return string(out), fmt.Errorf("ssh %s: %w", c.Target, err)
	}
	return string(out), nil
}

func (c *Client) PutFile(local, remote string) error {
	args := append([]string{}, c.Extra...)
	args = append(args, local, c.Target+":"+remote)
	out, err := exec.Command("scp", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("scp %s -> %s: %w: %s", local, remote, err, bytes.TrimSpace(out))
	}
	return nil
}

func (c *Client) PutBytes(data []byte, remote string) error {
	args := append([]string{}, c.Extra...)
	args = append(args, c.Target, "cat > '"+remote+"'")
	cmd := exec.Command("ssh", args...)
	cmd.Stdin = bytes.NewReader(data)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh put %s: %w: %s", remote, err, stderr.String())
	}
	return nil
}
