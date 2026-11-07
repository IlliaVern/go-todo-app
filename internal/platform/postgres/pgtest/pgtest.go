// Package pgtest starts a throwaway PostgreSQL with the project schema for
// tests, using testcontainers.
//
// Tests using it are skipped with -short or when Docker is unavailable, unless
// REQUIRE_DOCKER=1 (set by `make check`) turns that into a failure.
package pgtest

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/IlliaVern/go-todo-app/internal/platform/postgres"
)

// Image must match the postgres service in docker/compose.yaml.
const Image = "postgres:18.4"

// SkipUnlessDocker skips t when Docker tests are not wanted or possible.
func SkipUnlessDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("REQUIRE_DOCKER") != "" {
		return
	}
	if testing.Short() {
		t.Skip("Docker-backed test skipped in -short mode")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)
}

// Start runs PostgreSQL with schema/000001_init.up.sql applied and returns a
// connection; the container is removed when t finishes.
func Start(t *testing.T) *pgxpool.Pool {
	t.Helper()
	SkipUnlessDocker(t)

	ctx := t.Context()
	pg, err := tcpostgres.Run(ctx, Image,
		tcpostgres.WithDatabase("todo"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.WithInitScripts(schemaPath(t)),
		tcpostgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, pg)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db, err := postgres.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

func schemaPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate pgtest source file")
	}
	// internal/platform/postgres/pgtest -> repository root
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "schema", "000001_init.up.sql")
}
