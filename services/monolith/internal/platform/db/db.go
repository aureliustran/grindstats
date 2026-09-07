// Package db wires the shared Postgres connection pool used by every domain
// package. One instance, one schema per domain (docs/backend.md §3) — domain
// packages take a *pgxpool.Pool from here and never open their own connection.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"grindstats/services/monolith/internal/platform/config"
)

// Connect opens a pooled connection to Postgres and verifies it with a ping.
func Connect(ctx context.Context, cfg config.Postgres) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	return pool, nil
}
