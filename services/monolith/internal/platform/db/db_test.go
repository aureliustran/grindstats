package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"grindstats/services/monolith/internal/platform/config"
)

// requirePool skips unless GRINDSTATS_TEST_DB is set (docs/backend.md §7), so
// `go test ./...` stays fast without a running docker-compose.
func requirePool(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	if os.Getenv("GRINDSTATS_TEST_DB") == "" {
		t.Skip("set GRINDSTATS_TEST_DB=1 with `docker compose up -d postgres` to run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	pool, err := Connect(ctx, config.Load().Postgres)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(pool.Close)

	return ctx, pool
}

func TestConnect_PingsRunningPostgres(t *testing.T) {
	ctx, pool := requirePool(t)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestConnect_RoundTripsAQuery(t *testing.T) {
	ctx, pool := requirePool(t)

	var n int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&n); err != nil {
		t.Fatalf("QueryRow: %v", err)
	}
	if n != 1 {
		t.Errorf("SELECT 1 = %d, want 1", n)
	}
}

func TestConnect_WritesAndReadsBackARow(t *testing.T) {
	ctx, pool := requirePool(t)

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "CREATE TEMP TABLE connectivity_check (value text)"); err != nil {
		t.Fatalf("CREATE TEMP TABLE: %v", err)
	}
	if _, err := conn.Exec(ctx, "INSERT INTO connectivity_check (value) VALUES ($1)", "grindstats"); err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	var value string
	if err := conn.QueryRow(ctx, "SELECT value FROM connectivity_check").Scan(&value); err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if value != "grindstats" {
		t.Errorf("value = %q, want %q", value, "grindstats")
	}
}

func TestConnect_UnreachableHostReturnsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := config.Load().Postgres
	cfg.Port = "1" // nothing listens here

	pool, err := Connect(ctx, cfg)
	if err == nil {
		pool.Close()
		t.Fatal("Connect succeeded against an unreachable port, want error")
	}
}
