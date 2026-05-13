// internal/ssh/ssh_test.go
package ssh

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func fakeBin(t *testing.T, name, stdoutText string, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	logFile := filepath.Join(dir, name+".log")
	p := filepath.Join(dir, name)
	script := fmt.Sprintf(`#!/usr/bin/env bash
printf '%%s\n' "$*" >>%s
printf '%%s' %q
exit %d
`, logFile, stdoutText, exitCode)
	require.NoError(t, os.WriteFile(p, []byte(script), 0o755))
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return logFile
}

func TestRun_PassesArgs(t *testing.T) {
	logFile := fakeBin(t, "ssh", "hello", 0)
	c := &Client{Target: "bobsled@h1"}
	out, err := c.Run("echo hi")
	require.NoError(t, err)
	require.Equal(t, "hello", out)
	logged, _ := os.ReadFile(logFile)
	require.Contains(t, string(logged), "bobsled@h1")
	require.Contains(t, string(logged), "echo hi")
}

func TestRun_NonZeroExit(t *testing.T) {
	fakeBin(t, "ssh", "", 7)
	c := &Client{Target: "bobsled@h1"}
	_, err := c.Run("nope")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "exit") || strings.Contains(err.Error(), "7"))
}
