package cli

import (
	"fmt"
	"log/slog"

	"github.com/gobuffalo/pop/v6"
	"github.com/gobuffalo/pop/v6/logging"
	"github.com/spf13/cobra"

	"github.com/fbriansyah/go-personal-web/internal/config"
	"github.com/fbriansyah/go-personal-web/internal/database"
)

func newMigrateCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply or inspect database migrations",
		Long: "Migrations are embedded in this binary. `serve` never applies them:\n" +
			"it refuses to start while any are pending, so deploying is\n" +
			"`personal-web migrate up && personal-web serve`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newMigrateUpCmd(a),
		newMigrateDownCmd(a),
		newMigrateStatusCmd(a),
	)
	return cmd
}

func newMigrateUpCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Apply every pending migration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withDatabase(cmd, func(conn *pop.Connection, cfg *config.Config) error {
				return database.Up(cmd.Context(), cfg, conn)
			})
		},
	}
}

func newMigrateDownCmd(a *app) *cobra.Command {
	var steps int
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Roll back applied migrations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if steps < 1 {
				return fmt.Errorf("--step must be at least 1, got %d", steps)
			}
			return a.withDatabase(cmd, func(conn *pop.Connection, cfg *config.Config) error {
				return database.Down(cmd.Context(), cfg, conn, steps)
			})
		},
	}
	// Rolling back is destructive, so the count is always explicit and defaults
	// to the smallest useful step rather than to "all".
	cmd.Flags().IntVar(&steps, "step", 1, "number of migrations to roll back")
	return cmd
}

func newMigrateStatusCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show which migrations have been applied",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withDatabase(cmd, func(conn *pop.Connection, _ *config.Config) error {
				return database.Status(conn, cmd.OutOrStdout())
			})
		},
	}
}

// withDatabase resolves the Effective Config, opens the database, and hands
// both to fn, closing the connection afterwards.
func (a *app) withDatabase(cmd *cobra.Command, fn func(*pop.Connection, *config.Config) error) error {
	cfg, err := a.config(cmd)
	if err != nil {
		return err
	}
	usePopLogger(newLogger(cmd, cfg))

	conn, err := database.Open(cmd.Context(), cfg)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	return fn(conn, cfg)
}

// usePopLogger routes pop's own output through our logger instead of stdout.
//
// pop.SetLogger sets a package-level variable, which is exactly the global
// state ADR-0001 avoids; it belongs to pop and there is nowhere else to put it.
// Setting it per invocation at least keeps migration output on the command's
// error stream, where a test can capture it.
func usePopLogger(log *slog.Logger) {
	pop.SetLogger(func(lvl logging.Level, s string, args ...any) {
		msg := s
		if len(args) > 0 {
			msg = fmt.Sprintf(s, args...)
		}
		switch lvl {
		case logging.SQL, logging.Debug:
			log.Debug(msg)
		case logging.Warn:
			log.Warn(msg)
		case logging.Error:
			log.Error(msg)
		default:
			log.Info(msg)
		}
	})
}
