// internal/cli/tui.go
package cli

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/tui"
	"github.com/spf13/cobra"
)

func newTuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Live full-screen view of the fleet with keypress actions",
		RunE: func(_ *cobra.Command, _ []string) error {
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			c := &ghapp.Client{
				APIBase: "https://api.github.com",
				AppID:   inv.GitHub.AppID,
				KeyPath: expandHome(inv.GitHub.AppKey),
				HTTP:    &http.Client{Timeout: 30 * time.Second},
				Now:     time.Now,
			}
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			tui.SetContext(ctx)
			m := tui.New(inv, c, flagInventory)
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}
}
