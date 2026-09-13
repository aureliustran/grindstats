// Integration tests for AccountStore.
//
// These tests require a live Postgres instance with the auth schema applied.
// They are gated behind GRINDSTATS_TEST_DB so that "go test ./..." stays green
// without a database (docs/backend.md §7).
//
// To run:
//
//	GRINDSTATS_TEST_DB="host=localhost port=5433 user=grindstats password=grindstats dbname=grindstats sslmode=disable" \
//	  go test ./services/monolith/internal/auth/store/...
//
// Each test cleans the auth schema tables it writes so tests are independent.
package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"grindstats/libs/auditmodel"
	"grindstats/services/monolith/internal/auth/authdomain"
)

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

// testPool opens a pool to the test database or skips the test.
func testPool(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("GRINDSTATS_TEST_DB")
	if dsn == "" {
		t.Skip("GRINDSTATS_TEST_DB not set; skipping integration test (set it to a libpq DSN)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("pool.Ping: %v", err)
	}

	applyMigration(t, ctx, pool)

	return ctx, pool
}

// applyMigration runs the up-migration SQL if the auth schema does not yet
// exist, so a freshly created test database is ready without a separate step.
func applyMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.schemata
			WHERE schema_name = 'auth'
		)`).Scan(&exists)
	if err != nil {
		t.Fatalf("applyMigration: check schema: %v", err)
	}
	if exists {
		return // already applied
	}

	upSQL := readMigrationSQL(t, "0001_auth_and_audit_schema.up.sql")
	if _, err := pool.Exec(ctx, upSQL); err != nil {
		t.Fatalf("applyMigration: exec up SQL: %v", err)
	}
}

// readMigrationSQL reads a migration file from infra/db/migrations/ relative
// to the repo root, resolving the path from this source file's location.
func readMigrationSQL(t *testing.T, filename string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// file = .../services/monolith/internal/auth/store/accounts_test.go
	// root = ../../../../../../ (six levels up)
	root := filepath.Clean(filepath.Join(filepath.Dir(file),
		"..", "..", "..", "..", "..", ".."))
	p := filepath.Join(root, "infra", "db", "migrations", filename)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("readMigrationSQL: %v", err)
	}
	return string(b)
}

// cleanAuthTables truncates auth.users (cascades to oauth_identities and
// link_tokens) so each test starts from an empty state.
func cleanAuthTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `TRUNCATE auth.users CASCADE`)
	if err != nil {
		t.Fatalf("cleanAuthTables: %v", err)
	}
}

// newTestAccount builds an Account value suitable for insertion.
func newTestAccount(email string) authdomain.Account {
	return authdomain.Account{
		ID:           uuid.New(),
		Email:        email,
		EmailLower:   strings.ToLower(email),
		PasswordHash: "$argon2id$v=19$m=65536,t=3,p=4$fakesalt$fakehash",
		Role:         auditmodel.RoleUser,
		Status:       auditmodel.AccountStatusActive,
		CreatedAt:    time.Now().UTC().Truncate(time.Microsecond),
	}
}

// ---------------------------------------------------------------------------
// TestAccountStore_CreateThenByEmail_RoundTripsWithEnumTranslation
//
// Brief scenario: "Create then ByEmail round-trips an account, with role and
// status surviving the compact-code translation."
// ---------------------------------------------------------------------------

func TestAccountStore_CreateThenByEmail_RoundTripsWithEnumTranslation(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	store := NewAccountStore(pool)
	want := newTestAccount("alice@example.com")

	if err := store.Create(ctx, want); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.ByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("ByEmail: %v", err)
	}
	if got == nil {
		t.Fatal("ByEmail returned nil, want account")
	}

	if got.ID != want.ID {
		t.Errorf("ID: got %v, want %v", got.ID, want.ID)
	}
	if got.Email != want.Email {
		t.Errorf("Email: got %q, want %q", got.Email, want.Email)
	}
	if got.EmailLower != want.EmailLower {
		t.Errorf("EmailLower: got %q, want %q", got.EmailLower, want.EmailLower)
	}
	if got.PasswordHash != want.PasswordHash {
		t.Errorf("PasswordHash: got %q, want %q", got.PasswordHash, want.PasswordHash)
	}
	// Enum fields must be readable Go values, never compact DB codes.
	if got.Role != auditmodel.RoleUser {
		t.Errorf("Role: got %q, want %q", got.Role, auditmodel.RoleUser)
	}
	if got.Status != auditmodel.AccountStatusActive {
		t.Errorf("Status: got %q, want %q", got.Status, auditmodel.AccountStatusActive)
	}
	if got.IsVerified() {
		t.Error("IsVerified: got true, want false (email not verified yet)")
	}
}

// ---------------------------------------------------------------------------
// TestAccountStore_ByEmail_ReturnsNilNilForUnknownAddress
//
// Brief scenario: "ByEmail returns (nil, nil), not an error, for an unknown
// address."  This is load-bearing for FR-08: a code path that returns an error
// for a missing address will eventually log or respond differently from the
// bad-password path.
// ---------------------------------------------------------------------------

func TestAccountStore_ByEmail_ReturnsNilNilForUnknownAddress(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	store := NewAccountStore(pool)

	got, err := store.ByEmail(ctx, "nobody@example.com")
	if err != nil {
		t.Fatalf("ByEmail returned error %v, want (nil, nil)", err)
	}
	if got != nil {
		t.Fatalf("ByEmail returned account %+v, want nil", got)
	}
}

// ---------------------------------------------------------------------------
// TestAccountStore_Create_ReturnErrEmailTakenForCaseVariantAddress
//
// Brief scenario: "a second Create with a case-variant address returns
// ErrEmailTaken (FR-01)."  email_lower is the uniqueness key, so
// "Alice@Example.com" and "alice@example.com" must collide.
// ---------------------------------------------------------------------------

func TestAccountStore_Create_ReturnErrEmailTakenForCaseVariantAddress(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	store := NewAccountStore(pool)

	first := newTestAccount("Alice@Example.com")
	if err := store.Create(ctx, first); err != nil {
		t.Fatalf("Create first: %v", err)
	}

	// Attempt to create a second account with a case-variant address.
	second := newTestAccount("alice@example.com") // same email_lower
	err := store.Create(ctx, second)
	if !errors.Is(err, authdomain.ErrEmailTaken) {
		t.Errorf("Create second: got %v, want ErrEmailTaken", err)
	}
}

// ---------------------------------------------------------------------------
// TestAccountStore_ByID_ReturnsErrNotFoundForMissingID
// ---------------------------------------------------------------------------

func TestAccountStore_ByID_ReturnsErrNotFoundForMissingID(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	store := NewAccountStore(pool)

	_, err := store.ByID(ctx, uuid.New())
	if !errors.Is(err, authdomain.ErrNotFound) {
		t.Errorf("ByID: got %v, want ErrNotFound", err)
	}
}

// ---------------------------------------------------------------------------
// TestAccountStore_MarkEmailVerified_SetsVerifiedAt
// ---------------------------------------------------------------------------

func TestAccountStore_MarkEmailVerified_SetsVerifiedAt(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	store := NewAccountStore(pool)
	a := newTestAccount("bob@example.com")
	if err := store.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	verifiedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.MarkEmailVerified(ctx, a.ID, verifiedAt); err != nil {
		t.Fatalf("MarkEmailVerified: %v", err)
	}

	got, err := store.ByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("ByID after verify: %v", err)
	}
	if !got.IsVerified() {
		t.Error("IsVerified: got false after MarkEmailVerified")
	}
}

// ---------------------------------------------------------------------------
// TestAccountStore_MarkEmailVerified_IsIdempotent
// ---------------------------------------------------------------------------

func TestAccountStore_MarkEmailVerified_IsIdempotent(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	store := NewAccountStore(pool)
	a := newTestAccount("carol@example.com")
	if err := store.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	first := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.MarkEmailVerified(ctx, a.ID, first); err != nil {
		t.Fatalf("first MarkEmailVerified: %v", err)
	}

	// Second call should not error and should not overwrite the timestamp.
	second := first.Add(time.Hour)
	if err := store.MarkEmailVerified(ctx, a.ID, second); err != nil {
		t.Fatalf("second MarkEmailVerified: %v", err)
	}

	got, err := store.ByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.EmailVerifiedAt == nil {
		t.Fatal("EmailVerifiedAt is nil")
	}
	// Timestamp should match the first call, not the second.
	if !got.EmailVerifiedAt.Equal(first) {
		t.Errorf("EmailVerifiedAt: got %v, want %v (first call wins)", got.EmailVerifiedAt, first)
	}
}

// ---------------------------------------------------------------------------
// TestAccountStore_SetPasswordHash_UpdatesHash
// ---------------------------------------------------------------------------

func TestAccountStore_SetPasswordHash_UpdatesHash(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	store := NewAccountStore(pool)
	a := newTestAccount("dave@example.com")
	if err := store.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newHash := "$argon2id$v=19$m=65536,t=3,p=4$newsalt$newhash"
	if err := store.SetPasswordHash(ctx, a.ID, newHash); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	got, err := store.ByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.PasswordHash != newHash {
		t.Errorf("PasswordHash: got %q, want %q", got.PasswordHash, newHash)
	}
}

// ---------------------------------------------------------------------------
// TestAccountStore_CompactCodeNeverEscapesStore
//
// Verifies that the compact DB code (e.g., "ROLE2001") does not appear in the
// Account fields returned by ByEmail.  This is the automated enforcement of
// "nothing outside store ever sees a compact code".
// ---------------------------------------------------------------------------

func TestAccountStore_CompactCodeNeverEscapesStore(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	store := NewAccountStore(pool)
	a := newTestAccount("enum_check@example.com")
	if err := store.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.ByEmail(ctx, "enum_check@example.com")
	if err != nil || got == nil {
		t.Fatalf("ByEmail: got (%v, %v)", got, err)
	}

	// Role and Status must be readable values, not 8-char codes.
	roleStr := string(got.Role)
	if len(roleStr) == 8 && isAllCapsOrDigits(roleStr) {
		t.Errorf("Role looks like a compact code: %q", roleStr)
	}
	statusStr := string(got.Status)
	if len(statusStr) == 8 && isAllCapsOrDigits(statusStr) {
		t.Errorf("Status looks like a compact code: %q", statusStr)
	}
}

// isAllCapsOrDigits returns true when s consists entirely of uppercase ASCII
// letters and digits — the pattern of compact DB codes.
func isAllCapsOrDigits(s string) bool {
	for _, r := range s {
		if !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return len(s) > 0
}

// ---------------------------------------------------------------------------
// TestAccountStore_CreateWithNullPassword_SurvivesRoundTrip
//
// OAuth-only accounts have no password.  PasswordHash must be "" (not "NULL").
// ---------------------------------------------------------------------------

func TestAccountStore_CreateWithNullPassword_SurvivesRoundTrip(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	store := NewAccountStore(pool)
	a := newTestAccount("oauthonly@example.com")
	a.PasswordHash = "" // OAuth-only — no password

	if err := store.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.ByEmail(ctx, "oauthonly@example.com")
	if err != nil {
		t.Fatalf("ByEmail: %v", err)
	}
	if got == nil {
		t.Fatal("ByEmail returned nil")
	}
	if got.PasswordHash != "" {
		t.Errorf("PasswordHash: got %q, want %q (empty string for OAuth-only)", got.PasswordHash, "")
	}

	// ByID should not require pgx to know what SELECT 1 returns — just verify
	// the row is reachable by primary key too.
	gotByID, err := store.ByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if gotByID.PasswordHash != "" {
		t.Errorf("ByID PasswordHash: got %q, want empty", gotByID.PasswordHash)
	}
}

// Compile-time check that AccountStore satisfies the interface.
var _ authdomain.AccountRepo = (*AccountStore)(nil)

// Compile-time check that pgx.Rows is a pgx.Row (used by scanAccount).
var _ pgx.Row = pgx.Rows(nil)
