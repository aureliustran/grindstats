// Integration tests for cmd/migrate.
//
// These tests require a live Postgres instance.  They are gated behind the
// GRINDSTATS_TEST_DB environment variable so that "go test ./..." stays fast
// and green without a database (docs/backend.md §7).
//
// To run:
//
//	GRINDSTATS_TEST_DB="host=localhost port=5433 user=grindstats password=grindstats dbname=grindstats sslmode=disable" \
//	  go test ./services/monolith/cmd/migrate/...
//
// Each test cleans up the schemas and tracking table it creates so tests are
// independent and can be run in any order.
package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// testConn opens a connection to the test database or skips the test.
func testConn(t *testing.T) *pgx.Conn {
	t.Helper()
	dsn := os.Getenv("GRINDSTATS_TEST_DB")
	if dsn == "" {
		t.Skip("GRINDSTATS_TEST_DB not set; skipping integration test")
	}
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

// migrationsPath returns the absolute path to infra/db/migrations.
// It resolses the path relative to this source file so the tests work
// regardless of the working directory the test binary is invoked from.
func migrationsPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// file = .../services/monolith/cmd/migrate/migrate_test.go
	// root  = ../../../../  (four levels up)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
	p := filepath.Clean(filepath.Join(root, "infra", "db", "migrations"))
	return p
}

// cleanDB drops the auth and audit schemas and the tracking table so each
// test starts from a blank slate.
func cleanDB(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	stmts := []string{
		`DROP SCHEMA IF EXISTS audit CASCADE`,
		`DROP SCHEMA IF EXISTS auth CASCADE`,
		`DROP TABLE IF EXISTS public.schema_migrations`,
	}
	for _, s := range stmts {
		if _, err := conn.Exec(ctx, s); err != nil {
			t.Fatalf("cleanDB: %v", err)
		}
	}
}

// newRunner creates a Runner backed by conn and pointing at the migrations dir.
func newRunner(conn *pgx.Conn, dir string) *Runner {
	return NewRunner(conn, dir)
}

// isPgUniqueViolation returns true when err is a Postgres unique-constraint
// violation (SQLSTATE 23505).
func isPgUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

// TestMigrationsApplyAndRollbackCleanly verifies that every migration applies
// cleanly to an empty database and then rolls back cleanly, leaving the
// database in the exact state it was before apply.
func TestMigrationsApplyAndRollbackCleanly(t *testing.T) {
	conn := testConn(t)
	ctx := context.Background()
	dir := migrationsPath(t)

	cleanDB(t, conn)
	t.Cleanup(func() { cleanDB(t, conn) })

	r := newRunner(conn, dir)

	if err := r.EnsureTrackingTable(ctx); err != nil {
		t.Fatalf("EnsureTrackingTable: %v", err)
	}

	// Apply all migrations.
	if err := r.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Confirm tables exist after up.
	tables := []string{
		"auth.users",
		"auth.oauth_identities",
		"auth.link_tokens",
		"audit.records",
	}
	for _, tbl := range tables {
		var exists bool
		schema, name := splitTable(tbl)
		err := conn.QueryRow(ctx,
			`SELECT EXISTS (
                SELECT 1 FROM information_schema.tables
                WHERE table_schema = $1 AND table_name = $2
            )`, schema, name).Scan(&exists)
		if err != nil {
			t.Fatalf("check table %s: %v", tbl, err)
		}
		if !exists {
			t.Errorf("table %s does not exist after Up", tbl)
		}
	}

	// Confirm tracking row was recorded.
	var count int
	if err := conn.QueryRow(ctx,
		`SELECT COUNT(*) FROM public.schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count == 0 {
		t.Error("schema_migrations is empty after Up")
	}

	// Roll back the last migration.
	if err := r.Down(ctx); err != nil {
		t.Fatalf("Down: %v", err)
	}

	// Confirm tables no longer exist after down.
	for _, tbl := range tables {
		var exists bool
		schema, name := splitTable(tbl)
		err := conn.QueryRow(ctx,
			`SELECT EXISTS (
                SELECT 1 FROM information_schema.tables
                WHERE table_schema = $1 AND table_name = $2
            )`, schema, name).Scan(&exists)
		if err != nil {
			t.Fatalf("check table %s after Down: %v", tbl, err)
		}
		if exists {
			t.Errorf("table %s still exists after Down", tbl)
		}
	}

	// Confirm tracking row was removed.
	if err := conn.QueryRow(ctx,
		`SELECT COUNT(*) FROM public.schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations after Down: %v", err)
	}
	if count != 0 {
		t.Errorf("schema_migrations has %d rows after Down; want 0", count)
	}
}

// TestUpIsIdempotent verifies that running Up twice applies nothing on the
// second call.
func TestUpIsIdempotent(t *testing.T) {
	conn := testConn(t)
	ctx := context.Background()
	dir := migrationsPath(t)

	cleanDB(t, conn)
	t.Cleanup(func() { cleanDB(t, conn) })

	r := newRunner(conn, dir)
	if err := r.EnsureTrackingTable(ctx); err != nil {
		t.Fatalf("EnsureTrackingTable: %v", err)
	}

	// First apply.
	if err := r.Up(ctx); err != nil {
		t.Fatalf("first Up: %v", err)
	}

	// Record the tracking rows after the first apply.
	applied1, err := r.appliedVersions(ctx)
	if err != nil {
		t.Fatalf("appliedVersions after first Up: %v", err)
	}

	// Second apply — must be a no-op (no error, same rows).
	if err := r.Up(ctx); err != nil {
		t.Fatalf("second Up: %v", err)
	}

	applied2, err := r.appliedVersions(ctx)
	if err != nil {
		t.Fatalf("appliedVersions after second Up: %v", err)
	}

	if len(applied1) != len(applied2) {
		t.Errorf("second Up changed applied count: before=%d after=%d",
			len(applied1), len(applied2))
	}
	for v := range applied1 {
		if _, ok := applied2[v]; !ok {
			t.Errorf("version %q disappeared after second Up", v)
		}
	}
}

// TestEmailLowerRejectsVariantDuplicate verifies that the UNIQUE constraint on
// auth.users.email_lower prevents a case-variant duplicate address from being
// inserted (FR-01 case-insensitive uniqueness).
func TestEmailLowerRejectsVariantDuplicate(t *testing.T) {
	conn := testConn(t)
	ctx := context.Background()
	dir := migrationsPath(t)

	cleanDB(t, conn)
	t.Cleanup(func() { cleanDB(t, conn) })

	r := newRunner(conn, dir)
	if err := r.EnsureTrackingTable(ctx); err != nil {
		t.Fatalf("EnsureTrackingTable: %v", err)
	}
	if err := r.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Use deterministic UUIDs so the test is repeatable.
	const (
		uuid1 = "00000000-0000-0000-0000-000000000001"
		uuid2 = "00000000-0000-0000-0000-000000000002"
		// char(8) values — any 8-char string satisfies the NOT NULL constraint.
		// These are placeholder codes; the actual compact codes come from
		// libs/auditmodel/generated.go and are not known to this slice.
		roleCode   = "RXXXXXXX"
		statusCode = "SXXXXXXX"
	)

	// Insert first user: email as typed preserves case, email_lower is canonical.
	_, err := conn.Exec(ctx,
		`INSERT INTO auth.users(id, email, email_lower, role, status)
         VALUES ($1, $2, $3, $4, $5)`,
		uuid1, "Taken@Example.com", "taken@example.com", roleCode, statusCode)
	if err != nil {
		t.Fatalf("insert first user: %v", err)
	}

	// Insert second user with the same email_lower (different casing of email).
	_, err = conn.Exec(ctx,
		`INSERT INTO auth.users(id, email, email_lower, role, status)
         VALUES ($1, $2, $3, $4, $5)`,
		uuid2, "taken@example.com", "taken@example.com", roleCode, statusCode)

	if err == nil {
		t.Fatal("expected unique violation on email_lower duplicate, got nil error")
	}
	if !isPgUniqueViolation(err) {
		t.Fatalf("expected unique violation (23505), got: %v", err)
	}
}

// splitTable splits "schema.table" into (schema, table).
func splitTable(qualified string) (string, string) {
	for i, ch := range qualified {
		if ch == '.' {
			return qualified[:i], qualified[i+1:]
		}
	}
	return "public", qualified
}
