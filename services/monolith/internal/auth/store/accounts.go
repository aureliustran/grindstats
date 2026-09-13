// Package store implements the authdomain repository interfaces over a pgx
// connection pool.  It is the only package in the repo that writes SQL against
// the auth.* or audit.* schemas (docs/backend.md §2).
//
// Compact-code translation (audit-and-errors.md §6) happens here and only
// here: incoming Account values carry readable Go enum types
// (auditmodel.Role, auditmodel.AccountStatus, …); the SQL layer sees only the
// 8-char compact codes that are stored in char(8) columns.  Nothing outside
// this package ever sees a compact code.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"grindstats/libs/auditmodel"
	"grindstats/services/monolith/internal/auth/authdomain"
)

// pgUniqueViolation is the SQLSTATE code for a unique-constraint violation.
const pgUniqueViolation = "23505"

// AccountStore implements authdomain.AccountRepo over a pgx pool.
type AccountStore struct {
	db *pgxpool.Pool
}

// NewAccountStore creates an AccountStore backed by the given pool.
// The pool is the shared instance from platform/db — AccountStore never opens
// its own connection.
func NewAccountStore(db *pgxpool.Pool) *AccountStore {
	return &AccountStore{db: db}
}

// ByEmail returns the account whose email_lower matches emailLower.
// Returns (nil, nil) — not an error — when no account exists, so callers
// can treat absence as an ordinary branch without risking a logged error
// diverging from the found-but-wrong-password branch (FR-08).
func (s *AccountStore) ByEmail(ctx context.Context, emailLower string) (*authdomain.Account, error) {
	const q = `
		SELECT id, email, email_lower, password_hash,
		       role, status, email_verified_at, created_at
		FROM auth.users
		WHERE email_lower = $1`

	row := s.db.QueryRow(ctx, q, emailLower)
	a, err := scanAccount(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // (nil, nil) contract — absence is not an error
	}
	if err != nil {
		return nil, fmt.Errorf("store: ByEmail: %w", err)
	}
	return a, nil
}

// ByID returns the account with the given primary key.
// Returns authdomain.ErrNotFound when no account exists.
func (s *AccountStore) ByID(ctx context.Context, id uuid.UUID) (*authdomain.Account, error) {
	const q = `
		SELECT id, email, email_lower, password_hash,
		       role, status, email_verified_at, created_at
		FROM auth.users
		WHERE id = $1`

	row := s.db.QueryRow(ctx, q, pgtype.UUID{Bytes: [16]byte(id), Valid: true})
	a, err := scanAccount(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, authdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: ByID: %w", err)
	}
	return a, nil
}

// Create inserts a new account row.  Returns authdomain.ErrEmailTaken when
// email_lower already exists — detected from the unique-constraint violation
// (SQLSTATE 23505), not from a pre-check SELECT, so concurrent registrations
// for the same address reliably produce exactly one success (FR-08).
func (s *AccountStore) Create(ctx context.Context, a authdomain.Account) error {
	roleCode, ok := auditmodel.RoleCompact[a.Role]
	if !ok {
		return fmt.Errorf("store: Create: unknown role %q", a.Role)
	}
	statusCode, ok := auditmodel.AccountStatusCompact[a.Status]
	if !ok {
		return fmt.Errorf("store: Create: unknown status %q", a.Status)
	}

	var passwordHash *string
	if a.PasswordHash != "" {
		passwordHash = &a.PasswordHash
	}

	const q = `
		INSERT INTO auth.users
		       (id, email, email_lower, password_hash, role, status, created_at, updated_at)
		VALUES ($1, $2,    $3,          $4,            $5,   $6,     $7,         $7)`

	_, err := s.db.Exec(ctx, q,
		pgtype.UUID{Bytes: [16]byte(a.ID), Valid: true},
		a.Email,
		a.EmailLower,
		passwordHash,
		roleCode,
		statusCode,
		a.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return authdomain.ErrEmailTaken
		}
		return fmt.Errorf("store: Create: %w", err)
	}
	return nil
}

// SetPasswordHash updates the password_hash column for the given account.
// Used by the credentials slice on password-reset confirmation (FR-07).
func (s *AccountStore) SetPasswordHash(ctx context.Context, id uuid.UUID, hash string) error {
	const q = `
		UPDATE auth.users
		SET    password_hash = $1, updated_at = now()
		WHERE  id = $2`

	tag, err := s.db.Exec(ctx, q, hash, pgtype.UUID{Bytes: [16]byte(id), Valid: true})
	if err != nil {
		return fmt.Errorf("store: SetPasswordHash: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return authdomain.ErrNotFound
	}
	return nil
}

// MarkEmailVerified sets email_verified_at if it is currently NULL.
// Idempotent: calling it on an already-verified account is a no-op.
func (s *AccountStore) MarkEmailVerified(ctx context.Context, id uuid.UUID, at time.Time) error {
	const q = `
		UPDATE auth.users
		SET    email_verified_at = $1, updated_at = now()
		WHERE  id = $2
		  AND  email_verified_at IS NULL`

	_, err := s.db.Exec(ctx, q, at, pgtype.UUID{Bytes: [16]byte(id), Valid: true})
	if err != nil {
		return fmt.Errorf("store: MarkEmailVerified: %w", err)
	}
	return nil
}

// scanAccount reads one auth.users row from row into an Account.  It
// translates compact DB codes back to readable auditmodel enum values so that
// nothing outside this package ever sees a compact code.
func scanAccount(row pgx.Row) (*authdomain.Account, error) {
	var (
		rawID           pgtype.UUID
		email           string
		emailLower      string
		passwordHash    *string
		roleCode        string
		statusCode      string
		emailVerifiedAt *time.Time
		createdAt       time.Time
	)
	if err := row.Scan(
		&rawID,
		&email,
		&emailLower,
		&passwordHash,
		&roleCode,
		&statusCode,
		&emailVerifiedAt,
		&createdAt,
	); err != nil {
		return nil, err // caller distinguishes pgx.ErrNoRows
	}

	role, ok := auditmodel.CompactToRole[roleCode]
	if !ok {
		return nil, fmt.Errorf("store: unknown role compact code %q", roleCode)
	}
	status, ok := auditmodel.CompactToAccountStatus[statusCode]
	if !ok {
		return nil, fmt.Errorf("store: unknown status compact code %q", statusCode)
	}

	a := &authdomain.Account{
		ID:              uuid.UUID(rawID.Bytes),
		Email:           email,
		EmailLower:      emailLower,
		PasswordHash:    "",
		Role:            role,
		Status:          status,
		EmailVerifiedAt: emailVerifiedAt,
		CreatedAt:       createdAt,
	}
	if passwordHash != nil {
		a.PasswordHash = *passwordHash
	}
	return a, nil
}
