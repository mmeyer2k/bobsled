// cmd/bobsled-mint/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/m-meyer2k/bobsled/internal/runner"
)

func main() {
	var (
		configPath = flag.String("config", defaultPath("config.yaml"), "path to config.yaml")
		statePath  = flag.String("state", defaultPath("state.yaml"), "path to state.yaml")
		instance   = flag.Int("instance", 0, "slot/instance number (required)")
		output     = flag.String("output", "", "path to write JIT config JSON (required)")
		cacheRoot  = flag.String("cache-root", defaultCacheRoot(), "root of slot caches")
	)
	flag.Parse()
	if *instance <= 0 || *output == "" {
		fmt.Fprintln(os.Stderr, "Usage: bobsled-mint --instance N --output PATH [--config ...] [--state ...] [--cache-root ...]")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := runner.Mint(ctx, runner.Options{
		ConfigPath: *configPath, StatePath: *statePath, Instance: *instance,
		OutputPath: *output, CacheRoot: *cacheRoot,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "bobsled-mint: %v\n", err)
		os.Exit(1)
	}
}

func home() string                   { h, _ := os.UserHomeDir(); return h }
func defaultPath(name string) string { return filepath.Join(home(), name) }
func defaultCacheRoot() string       { return filepath.Join(home(), ".cache", "bobsled") }
