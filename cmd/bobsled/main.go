// cmd/bobsled/main.go
package main

import (
	"fmt"
	"os"

	"github.com/m-meyer2k/bobsled/internal/cli"
)

func main() {
	if err := cli.NewRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
