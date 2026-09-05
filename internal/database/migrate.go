package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"sort"

	"github.com/gobuffalo/pop/v6"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver used by lockMigrations

	"github.com/fbriansyah/go-personal-web/internal/config"
)

// Migrations travel inside the binary, so a deployment carries its schema with
// it and needs neither soda nor a migrations directory on disk (see ADR-0007).
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrationLockID identifies the advisory lock guarding `migrate up`. The value
// is arbitrary but must never change: it is what two concurrent deployments
// agree on.
const migrationLockID int64 = 8072026091505

// NewMigrator builds the migrator over the embedded migrations.
//
// Migrator.SchemaPath is left empty on purpose. Set, it would make every
// migration end by shelling out to pg_dump, which is absent from the image.
func NewMigrator(conn *pop.Connection) (pop.Migrator, error) {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return pop.Migrator{}, fmt.Errorf("migrations: %w", err)
	}
	box, err := pop.NewMigrationBox(sub, conn)
	if err != nil {
		return pop.Migrator{}, fmt.Errorf("migrations: %w", err)
	}
	return box.Migrator, nil
}

// Up applies every pending migration, holding a PostgreSQL advisory lock for
// the duration.
//
// Pop's Migrator.exec takes no lock of its own, so two deployments migrating at
// once would interleave in the middle of DDL. The lock is taken on a connection
// of its own because an advisory lock belongs to a session, and pop's pool
// would happily release it from a different one than took it.
func Up(ctx context.Context, cfg *config.Config, conn *pop.Connection) error {
	unlock, err := lockMigrations(ctx, cfg)
	if err != nil {
		return err
	}
	defer unlock()

	m, err := NewMigrator(conn)
	if err != nil {
		return err
	}
	return m.Up()
}

// Down rolls back the given number of migrations. It takes the same lock as Up.
func Down(ctx context.Context, cfg *config.Config, conn *pop.Connection, steps int) error {
	unlock, err := lockMigrations(ctx, cfg)
	if err != nil {
		return err
	}
	defer unlock()

	m, err := NewMigrator(conn)
	if err != nil {
		return err
	}
	return m.Down(steps)
}

// Status writes the applied/pending table for every migration.
func Status(conn *pop.Connection, out io.Writer) error {
	m, err := NewMigrator(conn)
	if err != nil {
		return err
	}
	return m.Status(out)
}

// Pending reports the migrations that have not been applied, oldest first, as
// "<version>_<name>".
//
// It deliberately does not call CreateSchemaMigrations: `serve` calls this on
// every startup, and `serve` performs no DDL. A missing schema_migration table
// simply means nothing has been applied yet.
func Pending(ctx context.Context, conn *pop.Connection) ([]string, error) {
	m, err := NewMigrator(conn)
	if err != nil {
		return nil, err
	}

	// The table name comes from pop's dialect, never from user input, so it is
	// safe to interpolate where a placeholder is not allowed.
	table := conn.MigrationTableName()

	var exists bool
	if err := conn.Store.GetContext(ctx, &exists, "select to_regclass($1) is not null", table); err != nil {
		return nil, fmt.Errorf("migrations: checking for %s: %w", table, err)
	}

	applied := map[string]bool{}
	if exists {
		var versions []string
		if err := conn.Store.SelectContext(ctx, &versions, "select version from "+table); err != nil {
			return nil, fmt.Errorf("migrations: reading %s: %w", table, err)
		}
		for _, v := range versions {
			applied[v] = true
		}
	}

	var pending []string
	for _, mf := range m.UpMigrations.Migrations {
		if !applied[mf.Version] {
			pending = append(pending, mf.Version+"_"+mf.Name)
		}
	}
	sort.Strings(pending)
	return pending, nil
}

// lockMigrations takes the advisory lock on a dedicated, pinned connection and
// returns the function that releases it.
//
// The lock is blocking: a second `migrate up` waits for the first rather than
// failing, which is what a deployment wants. Cancelling ctx aborts the wait.
func lockMigrations(ctx context.Context, cfg *config.Config) (func(), error) {
	db, err := sql.Open("pgx", cfg.Database.URL.Raw())
	if err != nil {
		return nil, fmt.Errorf("migrations: opening lock connection: %w", err)
	}
	// Conn pins a single session; the lock and its release must run on the same
	// one, which a pool does not guarantee.
	session, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrations: opening lock connection: %w", err)
	}
	if _, err := session.ExecContext(ctx, "select pg_advisory_lock($1)", migrationLockID); err != nil {
		_ = session.Close()
		_ = db.Close()
		return nil, fmt.Errorf("migrations: acquiring lock: %w", err)
	}

	return func() {
		// Closing the session would release the lock anyway; the explicit
		// unlock keeps the release visible in pg_locks without waiting for the
		// connection to go away.
		_, _ = session.ExecContext(context.WithoutCancel(ctx),
			"select pg_advisory_unlock($1)", migrationLockID)
		_ = session.Close()
		_ = db.Close()
	}, nil
}
