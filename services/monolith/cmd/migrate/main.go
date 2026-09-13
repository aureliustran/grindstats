// Command migrate applies, rolls back, and reports the status of database
// migrations for the GrindStats monolith.
//
// Usage:
//
//	migrate up      — apply all pending migrations in ascending order,
//	                  each in its own transaction
//	migrate down    — roll back the most recently applied migration
//	migrate status  — list all migrations with applied/pending state
//
// # Environment variables
//
//	POSTGRES_HOST      defaults to localhost
//	POSTGRES_PORT      defaults to 5433
//	POSTGRES_USER      defaults to grindstats
//	POSTGRES_PASSWORD  defaults to grindstats
//	POSTGRES_DB        defaults to grindstats
//	POSTGRES_SSLMODE   defaults to disable
//	MIGRATIONS_DIR     path to the migrations directory; defaults to
//	                   "infra/db/migrations" relative to the working directory
//	                   (run from the repository root)
//	GRINDSTATS_TEST_DB raw DSN that overrides the POSTGRES_* vars above;
//	                   used by the integration tests, not for production use
//
// be-wiring must ensure POSTGRES_HOST, POSTGRES_PORT, POSTGRES_USER,
// POSTGRES_PASSWORD, POSTGRES_DB and POSTGRES_SSLMODE are set in every
// environment that runs migrations (they already exist for the server).
// MIGRATIONS_DIR is new and must be added to any deployment runbook that
// invokes this binary from a working directory other than the repository root.
//
// Nothing in this binary is called by the application server at startup.
// Migrations are a deliberate, separate operational step (docs/backend.md §3).
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"

	"grindstats/services/monolith/internal/platform/config"
)

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: migrate <up|down|status>")
	}

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, dsn())
	if err != nil {
		fatalf("connect to database: %v", err)
	}
	defer conn.Close(ctx)

	r := NewRunner(conn, migrationsDir())

	if err := r.EnsureTrackingTable(ctx); err != nil {
		fatalf("ensure tracking table: %v", err)
	}

	switch os.Args[1] {
	case "up":
		if err := r.Up(ctx); err != nil {
			fatalf("up: %v", err)
		}
	case "down":
		if err := r.Down(ctx); err != nil {
			fatalf("down: %v", err)
		}
	case "status":
		if err := r.Status(ctx); err != nil {
			fatalf("status: %v", err)
		}
	default:
		fatalf("unknown subcommand %q — valid subcommands: up, down, status", os.Args[1])
	}
}

// dsn returns the Postgres DSN.
// GRINDSTATS_TEST_DB, if set, overrides all POSTGRES_* vars.
// This variable is for test use only; do not set it in production.
func dsn() string {
	if v := os.Getenv("GRINDSTATS_TEST_DB"); v != "" {
		return v
	}
	return config.Load().Postgres.DSN()
}

// migrationsDir returns the directory containing migration SQL files.
// MIGRATIONS_DIR overrides; otherwise "infra/db/migrations" relative to cwd.
// Run the binary from the repository root for the default to resolve correctly.
func migrationsDir() string {
	if d := os.Getenv("MIGRATIONS_DIR"); d != "" {
		return d
	}
	return filepath.Join("infra", "db", "migrations")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "migrate: "+format+"\n", args...)
	os.Exit(1)
}
