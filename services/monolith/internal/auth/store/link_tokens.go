package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"grindstats/libs/auditmodel"
	"grindstats/services/monolith/internal/auth/authdomain"
)

// LinkTokenStore implements authdomain.LinkTokenRepo over a pgx pool.
type LinkTokenStore struct {
	db *pgxpool.Pool
}

// NewLinkTokenStore creates a LinkTokenStore backed by the given pool.
func NewLinkTokenStore(db *pgxpool.Pool) *LinkTokenStore {
	return &LinkTokenStore{db: db}
}

// Issue generates a 32-byte cryptographically random token, stores only its
// SHA-256 hash in auth.link_tokens, and returns the raw token string to be
// emailed.  The raw token never appears in the database, logs, or any
// persistent store (SEC-04).
func (s *LinkTokenStore) Issue(ctx context.Context, userID uuid.UUID, kind auditmodel.LinkKind, ttl time.Duration, payload any) (string, error) {
	kindCode, ok := auditmodel.LinkKindCompact[kind]
	if !ok {
		return "", fmt.Errorf("store: Issue: unknown link kind %q", kind)
	}

	// Generate a 32-byte random token and encode it as base64url (no padding)
	// so it is safe to embed in a URL query parameter.
	rawBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, rawBytes); err != nil {
		return "", fmt.Errorf("store: Issue: generate token: %w", err)
	}
	rawToken := base64.RawURLEncoding.EncodeToString(rawBytes)

	// Hash the encoded token string.  Consume receives the same string and
	// hashes it identically to look up the row.
	hash := sha256.Sum256([]byte(rawToken))

	// Marshal the payload to JSON.  NULL is stored when payload is nil.
	var payloadJSON []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("store: Issue: marshal payload: %w", err)
		}
		payloadJSON = b
	}

	const q = `
		INSERT INTO auth.link_tokens
		       (id, user_id, kind, token_hash, expires_at, payload)
		VALUES ($1, $2,      $3,   $4,         $5,         $6)`

	expiresAt := time.Now().UTC().Add(ttl)
	_, err := s.db.Exec(ctx, q,
		pgtype.UUID{Bytes: [16]byte(uuid.New()), Valid: true},
		pgtype.UUID{Bytes: [16]byte(userID), Valid: true},
		kindCode,
		hash[:],
		expiresAt,
		payloadJSON,
	)
	if err != nil {
		return "", fmt.Errorf("store: Issue: %w", err)
	}
	return rawToken, nil
}

// Consume validates and marks a token consumed in one atomic UPDATE … RETURNING
// statement.  The WHERE clause guards on consumed_at IS NULL AND expires_at >
// now(), so a concurrent second call on the same token hits the guard and
// returns ErrLinkInvalid — exactly one caller can succeed.
//
// Returns ErrLinkInvalid (wrapped in *authdomain.LinkInvalidError) for unknown,
// expired, and already-consumed tokens alike so callers cannot accidentally
// produce three different HTTP responses.  The Reason field in
// *LinkInvalidError is for the audit record only.
func (s *LinkTokenStore) Consume(ctx context.Context, kind auditmodel.LinkKind, raw string) (*authdomain.LinkToken, error) {
	kindCode, ok := auditmodel.LinkKindCompact[kind]
	if !ok {
		return nil, fmt.Errorf("store: Consume: unknown link kind %q", kind)
	}

	// Recompute the hash the same way Issue did.
	hash := sha256.Sum256([]byte(raw))

	// One atomic UPDATE: mark consumed if the token matches, has not been
	// consumed, and has not expired.  RETURNING gives us the row data without
	// a separate SELECT round trip.
	const updateQ = `
		UPDATE auth.link_tokens
		SET    consumed_at = now()
		WHERE  token_hash  = $1
		  AND  kind        = $2
		  AND  consumed_at IS NULL
		  AND  expires_at  > now()
		RETURNING id, user_id, kind, payload, created_at`

	var (
		rawID     pgtype.UUID
		rawUserID pgtype.UUID
		kindBack  string
		payload   []byte
		createdAt time.Time
	)
	err := s.db.QueryRow(ctx, updateQ, hash[:], kindCode).Scan(
		&rawID, &rawUserID, &kindBack, &payload, &createdAt,
	)
	if err == nil {
		return &authdomain.LinkToken{
			ID:        uuid.UUID(rawID.Bytes),
			UserID:    uuid.UUID(rawUserID.Bytes),
			Kind:      kind,
			Payload:   payload,
			CreatedAt: createdAt,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("store: Consume: %w", err)
	}

	// UPDATE returned no rows.  Classify the rejection reason for the
	// audit record.  We query by token_hash alone (not kind) so we can
	// distinguish "token never existed" from "wrong kind presented" and
	// from "token existed but is expired or consumed".
	//
	// This follow-up SELECT is safe: the atomicity requirement (one success
	// among concurrent callers) is satisfied by the UPDATE above.  This
	// is purely a classification query for audit purposes.
	const classifyQ = `
		SELECT kind, expires_at, consumed_at
		FROM   auth.link_tokens
		WHERE  token_hash = $1`

	var (
		storedKindCode string
		expiresAt      time.Time
		consumedAt     *time.Time
	)
	err = s.db.QueryRow(ctx, classifyQ, hash[:]).Scan(&storedKindCode, &expiresAt, &consumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// No row with this hash — token was never issued.
		return nil, &authdomain.LinkInvalidError{Reason: auditmodel.LinkRejectReasonUnknown}
	}
	if err != nil {
		return nil, fmt.Errorf("store: Consume: classify: %w", err)
	}

	// Token hash exists but the UPDATE missed — determine which guard failed.
	if storedKindCode != kindCode {
		// Hash matched a row of a different kind; treat as unknown to avoid
		// leaking that a token with this hash exists for a different purpose.
		return nil, &authdomain.LinkInvalidError{Reason: auditmodel.LinkRejectReasonUnknown}
	}
	if consumedAt != nil {
		return nil, &authdomain.LinkInvalidError{Reason: auditmodel.LinkRejectReasonAlreadyConsumed}
	}
	// expires_at <= now()
	return nil, &authdomain.LinkInvalidError{Reason: auditmodel.LinkRejectReasonExpired}
}
