package content_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/gobuffalo/pop/v6"
	"github.com/gobuffalo/pop/v6/logging"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/fbriansyah/go-personal-web/internal/config"
	"github.com/fbriansyah/go-personal-web/internal/content"
	"github.com/fbriansyah/go-personal-web/internal/database"
)

// conn is the migrated connection shared by every test in this package.
//
// Store is a concrete type by design (ADR-0004), so there is nothing to fake:
// these tests need PostgreSQL. Starting it here rather than expecting one to be
// running keeps `go test ./...` self-contained, at the cost of requiring Docker.
// Applying the embedded migrations to the fresh container also means every run
// exercises the migrator for free.
var conn *pop.Connection

func TestMain(m *testing.M) {
	// Pop logs every migration and query to stdout. Keep the failures and drop
	// the running commentary.
	pop.SetLogger(func(lvl logging.Level, format string, args ...any) {
		if lvl >= logging.Warn {
			log.Printf("pop: "+format, args...)
		}
	})

	code, err := runSuite(m)
	if err != nil {
		log.Printf("content tests: %v", err)
		os.Exit(1)
	}
	os.Exit(code)
}

// runSuite exists so that the container is torn down by defer; os.Exit in
// TestMain would skip it.
func runSuite(m *testing.M) (int, error) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("pw_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return 0, fmt.Errorf("starting postgres: %w", err)
	}
	defer func() { _ = testcontainers.TerminateContainer(container) }()

	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return 0, fmt.Errorf("connection string: %w", err)
	}

	cfg := &config.Config{Database: config.DatabaseConfig{
		URL:             config.DSN(url),
		Pool:            5,
		IdlePool:        2,
		ConnMaxLifetime: 30 * time.Minute,
	}}

	conn, err = database.Open(ctx, cfg)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()

	if err := database.Up(ctx, cfg, conn); err != nil {
		return 0, fmt.Errorf("migrating: %w", err)
	}
	pending, err := database.Pending(ctx, conn)
	if err != nil {
		return 0, err
	}
	if len(pending) != 0 {
		return 0, fmt.Errorf("migrations still pending after up: %v", pending)
	}

	return m.Run(), nil
}

// newStore returns a Store bound to a transaction that is rolled back when the
// test ends, so cases cannot see each other's rows.
func newStore(t *testing.T) *content.Store {
	t.Helper()

	tx, err := conn.NewTransaction()
	if err != nil {
		t.Fatalf("beginning transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.TX.Rollback() })
	return content.NewStore(tx)
}

// at returns a pointer to a time offset from now, for Publication Dates.
func at(d time.Duration) *time.Time {
	t := time.Now().Add(d)
	return &t
}
