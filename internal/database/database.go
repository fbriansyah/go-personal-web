// Package database owns the PostgreSQL connection. It is the only place that
// knows how pop is connected; `serve`, `migrate` and internal/content all
// receive a *pop.Connection from here (see ADR-0004 and ADR-0005).
package database

import (
	"context"
	"fmt"

	"github.com/gobuffalo/pop/v6"

	"github.com/fbriansyah/go-personal-web/internal/config"
)

// Open connects to PostgreSQL and verifies the connection is usable.
//
// pop.NewConnection does not dial, and Open only prepares the pool: without the
// ping below, a wrong host or password would surface at the first request
// rather than at startup. Failing here makes a broken database the error of the
// command that needed it, exactly as a failure to bind is.
func Open(ctx context.Context, cfg *config.Config) (*pop.Connection, error) {
	conn, err := pop.NewConnection(&pop.ConnectionDetails{
		Dialect:         "postgres",
		URL:             cfg.Database.URL.Raw(),
		Pool:            cfg.Database.Pool,
		IdlePool:        cfg.Database.IdlePool,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
	})
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	if err := conn.Open(); err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	if err := Ping(ctx, conn); err != nil {
		// Closing here keeps the caller's error path a plain `return err`.
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// Ping reports whether the database answers. pop's store interface exposes no
// Ping of its own, so the cheapest possible statement stands in for one.
func Ping(ctx context.Context, conn *pop.Connection) error {
	if _, err := conn.Store.ExecContext(ctx, "select 1"); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	return nil
}
