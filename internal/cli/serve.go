package cli

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/fbriansyah/go-personal-web/internal/server"
)

func newServeCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the HTTP server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.config(cmd)
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return server.Run(ctx, cfg, newLogger(cmd, cfg))
		},
	}
	addServerFlags(cmd)
	return cmd
}
