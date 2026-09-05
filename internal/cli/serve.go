package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gobuffalo/pop/v6"
	"github.com/spf13/cobra"

	"github.com/fbriansyah/go-personal-web/internal/database"
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
			log := newLogger(cmd, cfg)
			usePopLogger(log)

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			// Open pings, so an unreachable or misconfigured database is the
			// error of `serve` rather than of the first visitor.
			conn, err := database.Open(ctx, cfg)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()

			if err := requireCurrentSchema(ctx, conn); err != nil {
				return err
			}

			return server.Run(ctx, cfg, conn, log)
		},
	}
	addServerFlags(cmd)
	return cmd
}

// requireCurrentSchema refuses to serve a database older than the binary.
//
// `serve` applies no migrations and creates no tables: running DDL because a
// pod restarted is exactly what ADR-0007 rules out. It only reports, and it
// reports once and clearly, because the alternative is every request failing
// with an SQL error that names a missing column instead of the real cause.
func requireCurrentSchema(ctx context.Context, conn *pop.Connection) error {
	pending, err := database.Pending(ctx, conn)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	return fmt.Errorf("database schema is %d migration(s) behind\n  pending: %s\n  run: personal-web migrate up",
		len(pending), strings.Join(pending, "\n           "))
}
