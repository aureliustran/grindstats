// Package main implements the cmd/migrate binary.
// This file contains the Runner type — the reusable migration logic used by
// the CLI entry point (main.go) and the integration tests (migrate_test.go).
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// trackingTable is the table that records which migrations have been applied.
// It lives in the public schema so it is not owned by any domain schema.
const trackingTable = "public.schema_migrations"

const createTrackingSQL = `
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    version    text        NOT NULL PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
)`

// Runner applies, rolls back and reports database migrations.
// Create one with NewRunner; reuse the same Runner across subcommands.
type Runner struct {
	conn *pgx.Conn
	dir  string
}

// NewRunner creates a Runner that reads migration files from dir and applies
// them through conn.  Call EnsureTrackingTable before Up/Down/Status.
func NewRunner(conn *pgx.Conn, dir string) *Runner {
	return &Runner{conn: conn, dir: dir}
}

// EnsureTrackingTable creates public.schema_migrations if it does not exist.
func (r *Runner) EnsureTrackingTable(ctx context.Context) error {
	_, err := r.conn.Exec(ctx, createTrackingSQL)
	return err
}

// Up applies every unapplied migration in ascending version order.
// Each migration is applied in its own transaction; the tracking row is
// inserted within the same transaction as the DDL (atomic).
// Returns nil and prints "nothing to apply" when already up to date.
func (r *Runner) Up(ctx context.Context) error {
	files, err := r.listFiles()
	if err != nil {
		return err
	}
	applied, err := r.appliedVersions(ctx)
	if err != nil {
		return fmt.Errorf("query applied versions: %w", err)
	}

	var count int
	for _, f := range files {
		if _, ok := applied[f.version]; ok {
			continue
		}
		sql, err := os.ReadFile(f.upPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", f.upPath, err)
		}
		if err := r.applyUp(ctx, f.version, string(sql)); err != nil {
			return fmt.Errorf("apply %s: %w", f.version, err)
		}
		fmt.Printf("applied   %s\n", f.version)
		count++
	}
	if count == 0 {
		fmt.Println("nothing to apply")
	}
	return nil
}

// Down rolls back the most recently applied migration.
// Returns nil and prints "nothing to roll back" when no migrations are applied.
func (r *Runner) Down(ctx context.Context) error {
	var latest string
	err := r.conn.QueryRow(ctx,
		`SELECT version FROM public.schema_migrations ORDER BY version DESC LIMIT 1`,
	).Scan(&latest)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			fmt.Println("nothing to roll back")
			return nil
		}
		return fmt.Errorf("query latest version: %w", err)
	}

	files, err := r.listFiles()
	if err != nil {
		return err
	}
	var target *migration
	for i := range files {
		if files[i].version == latest {
			target = &files[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("no migration file on disk for applied version %q", latest)
	}

	sql, err := os.ReadFile(target.downPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", target.downPath, err)
	}
	if err := r.applyDown(ctx, latest, string(sql)); err != nil {
		return fmt.Errorf("roll back %s: %w", latest, err)
	}
	fmt.Printf("rolled back %s\n", latest)
	return nil
}

// Status prints every known migration with its applied timestamp or "pending".
func (r *Runner) Status(ctx context.Context) error {
	files, err := r.listFiles()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Println("no migration files found in", r.dir)
		return nil
	}

	applied, err := r.appliedVersions(ctx)
	if err != nil {
		return fmt.Errorf("query applied versions: %w", err)
	}

	for _, f := range files {
		if at, ok := applied[f.version]; ok {
			fmt.Printf("applied   %s  (%s)\n", f.version, at.UTC().Format(time.RFC3339))
		} else {
			fmt.Printf("pending   %s\n", f.version)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

type migration struct {
	version  string // e.g. "0001_auth_and_audit_schema"
	upPath   string
	downPath string
}

// listFiles returns migrations sorted ascending by version string.
func (r *Runner) listFiles() ([]migration, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir %q: %w", r.dir, err)
	}
	var out []migration
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		base := strings.TrimSuffix(name, ".up.sql")
		out = append(out, migration{
			version:  base,
			upPath:   filepath.Join(r.dir, base+".up.sql"),
			downPath: filepath.Join(r.dir, base+".down.sql"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// appliedVersions returns every version present in the tracking table.
func (r *Runner) appliedVersions(ctx context.Context) (map[string]time.Time, error) {
	rows, err := r.conn.Query(ctx,
		`SELECT version, applied_at FROM public.schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]time.Time)
	for rows.Next() {
		var ver string
		var at time.Time
		if err := rows.Scan(&ver, &at); err != nil {
			return nil, err
		}
		result[ver] = at
	}
	return result, rows.Err()
}

// applyUp executes sql and records version in the tracking table, both in one
// transaction.
func (r *Runner) applyUp(ctx context.Context, version, sql string) error {
	tx, err := r.conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, sql); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO public.schema_migrations(version) VALUES($1)`, version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// applyDown executes sql and removes version from the tracking table, both in
// one transaction.
func (r *Runner) applyDown(ctx context.Context, version, sql string) error {
	tx, err := r.conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, sql); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM public.schema_migrations WHERE version = $1`, version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
