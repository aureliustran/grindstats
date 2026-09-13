package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"grindstats/libs/auditmodel"
	"grindstats/services/monolith/internal/auth/authdomain"
)

// OAuthIdentityStore implements authdomain.OAuthIdentityRepo over a pgx pool.
type OAuthIdentityStore struct {
	db *pgxpool.Pool
}

// NewOAuthIdentityStore creates an OAuthIdentityStore backed by the given pool.
func NewOAuthIdentityStore(db *pgxpool.Pool) *OAuthIdentityStore {
	return &OAuthIdentityStore{db: db}
}

// BySubject returns the full Account linked to the given provider identity.
// Returns authdomain.ErrNotFound when no oauth_identities row matches.
func (s *OAuthIdentityStore) BySubject(ctx context.Context, provider auditmodel.LinkedProvider, subject string) (*authdomain.Account, error) {
	providerCode, ok := auditmodel.LinkedProviderCompact[provider]
	if !ok {
		return nil, fmt.Errorf("store: BySubject: unknown provider %q", provider)
	}

	// Join against auth.users to return the full Account in one query.
	const q = `
		SELECT u.id, u.email, u.email_lower, u.password_hash,
		       u.role, u.status, u.email_verified_at, u.created_at
		FROM   auth.users u
		JOIN   auth.oauth_identities oi ON oi.user_id = u.id
		WHERE  oi.provider = $1
		  AND  oi.subject  = $2`

	row := s.db.QueryRow(ctx, q, providerCode, subject)
	a, err := scanAccount(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, authdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: BySubject: %w", err)
	}
	return a, nil
}

// Link writes a new oauth_identities row joining userID to the given provider
// identity.  A duplicate (provider, subject) pair is rejected by the database
// unique constraint; the caller handles that path.
func (s *OAuthIdentityStore) Link(ctx context.Context, userID uuid.UUID, provider auditmodel.LinkedProvider, subject, email string) error {
	providerCode, ok := auditmodel.LinkedProviderCompact[provider]
	if !ok {
		return fmt.Errorf("store: Link: unknown provider %q", provider)
	}

	const q = `
		INSERT INTO auth.oauth_identities (id, user_id, provider, subject, email)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := s.db.Exec(ctx, q,
		pgtype.UUID{Bytes: [16]byte(uuid.New()), Valid: true},
		pgtype.UUID{Bytes: [16]byte(userID), Valid: true},
		providerCode,
		subject,
		email,
	)
	if err != nil {
		return fmt.Errorf("store: Link: %w", err)
	}
	return nil
}
