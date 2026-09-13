// Package authdomain defines the cross-slice types, interfaces, and sentinel
// errors for the auth domain.  Every backend auth slice imports this package;
// no wave-3 slice (credentials, session, oauth, gateway-authz) imports another
// wave-3 slice's package.  The shared seams live here precisely so that wave-3
// agents can be dispatched in parallel without each needing to know the
// internal package layout of its neighbours.
//
// Contract reference: docs/stories/auth-epic/contract.md §5.
// Do not edit interfaces in this file without following the amendment protocol
// in docs/shared-contract.md §4 — three slices are built against them.
package authdomain

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"grindstats/libs/auditmodel"
)

// Account is the canonical in-memory representation of an auth.users row.
// Enum fields use the readable Go types from auditmodel; compact DB codes
// (e.g. "ROLE2001") never escape the store package — translation happens at
// the boundary where a row is read, which is store/ and nowhere else
// (audit-and-errors.md §6).
type Account struct {
	ID              uuid.UUID
	Email           string     // as the user typed it (mixed case preserved)
	EmailLower      string     // lower-cased, the uniqueness key (FR-01)
	PasswordHash    string     // "" when NULL (OAuth-only accounts have no password)
	Role            auditmodel.Role
	Status          auditmodel.AccountStatus
	EmailVerifiedAt *time.Time // nil until the user completes FR-03
	CreatedAt       time.Time
}

// IsVerified reports whether the account's email address has been confirmed.
// FR-03 and D4: unverified accounts may not perform unsafe-method requests;
// this is checked once, in the gateway middleware, not per domain.
func (a Account) IsVerified() bool { return a.EmailVerifiedAt != nil }

// IsSuspended reports whether the account is administratively suspended.
// D2: the suspension status is only surfaced AFTER password verification
// succeeds, so this is called on the authenticated path, never before
// credentials are checked.
func (a Account) IsSuspended() bool { return a.Status == auditmodel.AccountStatusSuspended }

// LinkToken is the value returned by LinkTokenRepo.Consume on success.  The
// caller uses it to extract the payload (for the OAuth-link path) and to
// supply user_id to the audit record.  It is never returned to an HTTP client.
type LinkToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Kind      auditmodel.LinkKind
	Payload   []byte    // raw JSON bytes; nil for email-verification and password-reset tokens
	CreatedAt time.Time
}

// AccountRepo is the persistence interface for auth.users.  Implemented by
// store.AccountStore; lives here so be-auth-credentials and be-auth-session
// are kept from importing each other.
type AccountRepo interface {
	// ByEmail returns the account whose email_lower matches the given
	// (already lower-cased) address.  Returns (nil, nil) — not an error —
	// when no account exists.  Callers must treat absence as an ordinary
	// branch: FR-08 requires the unknown-email response to be
	// indistinguishable from the bad-password response, and a code path
	// that returns an error is one that will eventually log or respond
	// differently.
	ByEmail(ctx context.Context, emailLower string) (*Account, error)

	// ByID returns the account with the given primary key.  Returns
	// ErrNotFound when no account exists.
	ByID(ctx context.Context, id uuid.UUID) (*Account, error)

	// Create inserts a new account row.  Returns ErrEmailTaken when
	// email_lower already exists — detected from the unique-constraint
	// violation rather than a pre-check SELECT, so concurrent registrations
	// for the same address reliably produce exactly one success (FR-08).
	Create(ctx context.Context, a Account) error

	// SetPasswordHash updates the password_hash column for the given account.
	// Used by the credentials slice on password-reset confirmation (FR-07).
	SetPasswordHash(ctx context.Context, id uuid.UUID, hash string) error

	// MarkEmailVerified sets email_verified_at if it is currently NULL.
	// Idempotent: calling it on an already-verified account is a no-op.
	MarkEmailVerified(ctx context.Context, id uuid.UUID, at time.Time) error
}

// OAuthIdentityRepo manages the auth.oauth_identities table.
type OAuthIdentityRepo interface {
	// BySubject returns the full Account linked to the given provider
	// identity.  Returns ErrNotFound when no oauth_identities row matches.
	BySubject(ctx context.Context, provider auditmodel.LinkedProvider, subject string) (*Account, error)

	// Link writes a new oauth_identities row joining userID to the given
	// provider identity.  A duplicate (provider, subject) pair is a
	// database-level unique-constraint violation; the caller handles that
	// path.
	Link(ctx context.Context, userID uuid.UUID, provider auditmodel.LinkedProvider, subject, email string) error
}

// LinkTokenRepo manages the auth.link_tokens table, which holds all three
// single-use emailed link types (email verification, password reset, OAuth
// link) under one roof (SEC-04).
type LinkTokenRepo interface {
	// Issue generates a cryptographically random token, stores only its
	// SHA-256 hash, and returns the raw token to be emailed.  The raw
	// token never appears in the database, logs, or any persistent store
	// (SEC-04).
	Issue(ctx context.Context, userID uuid.UUID, kind auditmodel.LinkKind, ttl time.Duration, payload any) (string, error)

	// Consume validates and marks a token consumed in one atomic
	// UPDATE … RETURNING statement.  The WHERE clause includes
	// consumed_at IS NULL AND expires_at > now(), so a concurrent second
	// call on the same token misses the guard and returns ErrLinkInvalid —
	// exactly one caller can win.
	//
	// Returns ErrLinkInvalid for unknown, expired, and already-consumed
	// tokens alike.  The error wraps *LinkInvalidError so that the audit
	// layer can obtain the LinkRejectReason; the error value itself must not
	// drive three different response branches (SEC-04).
	Consume(ctx context.Context, kind auditmodel.LinkKind, raw string) (*LinkToken, error)
}

// SessionIssuer is implemented by the session slice (be-auth-session) and
// consumed by the credentials and OAuth handlers at the wave-4 composition
// root.  It lives here so be-auth-credentials and be-auth-oauth are kept from
// importing each other.
//
// Issue mints an access/refresh pair, writes the session hash entry in Redis,
// sets both cookies on w, and returns the session-bound CSRF token.  It never
// decides whether the caller *may* log in — that is the caller's concern.
type SessionIssuer interface {
	Issue(ctx context.Context, w http.ResponseWriter, a Account, userAgent, sourceIP string) (csrfToken string, err error)
	// RevokeAll advances the user's blacklist epoch, invalidating every
	// outstanding access and refresh token (FR-33).  Also triggered by
	// password reset (FR-07) and admin force-logout (FR-43).
	RevokeAll(ctx context.Context, userID uuid.UUID) error
}

// AccountProvisioner is implemented by the credentials slice
// (be-auth-credentials) and consumed by the OAuth handler at the wave-4
// composition root.  It lives here so be-auth-oauth never imports the
// credentials package.
//
// ProvisionFromOAuth returns the existing Account for this provider identity,
// or creates one with role User and a verified email.  Returns ErrLinkRequired
// when a local account owns the address but has no oauth_identities row for
// this provider (FR-06, §4.3).
type AccountProvisioner interface {
	ProvisionFromOAuth(ctx context.Context, provider auditmodel.LinkedProvider, subject, email string) (*Account, error)
}

// Sentinel errors returned by repository and domain operations.
var (
	// ErrEmailTaken is returned by AccountRepo.Create when email_lower
	// already exists in auth.users.  Detected from the unique-constraint
	// violation (SQLSTATE 23505), not from a pre-check SELECT, so it is
	// reliable under concurrent registration (FR-08).
	ErrEmailTaken = errors.New("authdomain: email taken")

	// ErrLinkInvalid is returned by LinkTokenRepo.Consume for any of:
	// unknown token, expired token, or already-consumed token.  All three
	// map to AUTH_LINK_INVALID (400) — the distinction must never reach the
	// client (SEC-04).  The concrete error value is *LinkInvalidError, which
	// carries a Reason for internal audit use; errors.Is(err, ErrLinkInvalid)
	// returns true for it.
	ErrLinkInvalid = errors.New("authdomain: link token invalid")

	// ErrLinkRequired is returned by AccountProvisioner.ProvisionFromOAuth
	// when the email is already owned by a local account with no linked
	// identity for this provider.  The OAuth flow emails a link token and
	// redirects the browser (FR-06, §4.3).
	ErrLinkRequired = errors.New("authdomain: oauth link requires proof of control")

	// ErrNotFound is returned by point-lookup methods when no matching row
	// exists.
	ErrNotFound = errors.New("authdomain: not found")
)

// LinkInvalidError wraps ErrLinkInvalid with the specific rejection reason for
// audit-record construction.  Callers that decide the HTTP response MUST use
// errors.Is(err, ErrLinkInvalid) — never a type switch on *LinkInvalidError.
// Callers that write an audit record MAY unwrap to *LinkInvalidError to obtain
// the Reason field.  The separation exists so the audit layer can distinguish
// the three cases without accidentally producing three different client-visible
// responses (SEC-04).
type LinkInvalidError struct {
	Reason auditmodel.LinkRejectReason
}

// Error implements the error interface.  The message is the same as
// ErrLinkInvalid so that logs and error strings are consistent.
func (e *LinkInvalidError) Error() string { return ErrLinkInvalid.Error() }

// Is makes errors.Is(err, ErrLinkInvalid) return true for *LinkInvalidError.
func (e *LinkInvalidError) Is(target error) bool { return target == ErrLinkInvalid }
