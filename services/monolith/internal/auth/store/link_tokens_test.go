// Integration tests for LinkTokenStore.
//
// Gated behind GRINDSTATS_TEST_DB — see accounts_test.go for the skip logic
// and setup conventions shared across this package's test files.
package store

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"grindstats/libs/auditmodel"
	"grindstats/services/monolith/internal/auth/authdomain"
)

// ---------------------------------------------------------------------------
// TestLinkTokenStore_Issue_StoresOnlyHashNotRawToken
//
// Brief scenario: "Issue stores only a hash — the raw token appears nowhere
// in the row."  SEC-04: the token is emailed; the database never holds a
// value from which the raw token can be reconstructed.
// ---------------------------------------------------------------------------

func TestLinkTokenStore_Issue_StoresOnlyHashNotRawToken(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	accounts := NewAccountStore(pool)
	tokens := NewLinkTokenStore(pool)

	user := newTestAccount("hash_check@example.com")
	if err := accounts.Create(ctx, user); err != nil {
		t.Fatalf("Create user: %v", err)
	}

	rawToken, err := tokens.Issue(ctx, user.ID, auditmodel.LinkKindEmailVerification, time.Hour, nil)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if rawToken == "" {
		t.Fatal("Issue returned empty token")
	}

	// The raw token must NOT appear in the token_hash column.
	// token_hash is bytea — we check that the column value is not the UTF-8
	// bytes of the raw token string.
	var hashBytes []byte
	err = pool.QueryRow(ctx,
		`SELECT token_hash FROM auth.link_tokens
		 WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`,
		[16]byte(user.ID),
	).Scan(&hashBytes)
	if err != nil {
		t.Fatalf("SELECT token_hash: %v", err)
	}
	if strings.Contains(string(hashBytes), rawToken) {
		t.Error("token_hash column contains the raw token — raw token must not be stored")
	}
	// Also verify the stored hash is 32 bytes (SHA-256 output).
	if len(hashBytes) != 32 {
		t.Errorf("token_hash length: got %d bytes, want 32 (SHA-256)", len(hashBytes))
	}
}

// ---------------------------------------------------------------------------
// TestLinkTokenStore_Consume_SucceedsOnceReturnsErrLinkInvalidOnSecond
//
// Brief scenario: "Consume succeeds once and returns ErrLinkInvalid on the
// second attempt, including when both run concurrently."
// ---------------------------------------------------------------------------

func TestLinkTokenStore_Consume_SucceedsOnceReturnsErrLinkInvalidOnSecond(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	accounts := NewAccountStore(pool)
	tokens := NewLinkTokenStore(pool)

	user := newTestAccount("once@example.com")
	if err := accounts.Create(ctx, user); err != nil {
		t.Fatalf("Create user: %v", err)
	}

	rawToken, err := tokens.Issue(ctx, user.ID, auditmodel.LinkKindEmailVerification, time.Hour, nil)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// First consume should succeed.
	lt, err := tokens.Consume(ctx, auditmodel.LinkKindEmailVerification, rawToken)
	if err != nil {
		t.Fatalf("Consume (first): %v", err)
	}
	if lt == nil {
		t.Fatal("Consume (first): returned nil LinkToken")
	}
	if lt.UserID != user.ID {
		t.Errorf("LinkToken.UserID: got %v, want %v", lt.UserID, user.ID)
	}

	// Second consume must return ErrLinkInvalid.
	_, err = tokens.Consume(ctx, auditmodel.LinkKindEmailVerification, rawToken)
	if !errors.Is(err, authdomain.ErrLinkInvalid) {
		t.Errorf("Consume (second): got %v, want ErrLinkInvalid", err)
	}
	// The reason should be AlreadyConsumed (we can unwrap for this check).
	var linkErr *authdomain.LinkInvalidError
	if errors.As(err, &linkErr) {
		if linkErr.Reason != auditmodel.LinkRejectReasonAlreadyConsumed {
			t.Errorf("Reason: got %q, want AlreadyConsumed", linkErr.Reason)
		}
	}
}

// ---------------------------------------------------------------------------
// TestLinkTokenStore_Consume_ConcurrentAttemptsSingleSuccess
//
// The concurrency variant: two goroutines race to consume the same token.
// Exactly one must succeed and one must get ErrLinkInvalid.
// ---------------------------------------------------------------------------

func TestLinkTokenStore_Consume_ConcurrentAttemptsSingleSuccess(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	accounts := NewAccountStore(pool)
	tokens := NewLinkTokenStore(pool)

	user := newTestAccount("concurrent@example.com")
	if err := accounts.Create(ctx, user); err != nil {
		t.Fatalf("Create user: %v", err)
	}

	rawToken, err := tokens.Issue(ctx, user.ID, auditmodel.LinkKindEmailVerification, time.Hour, nil)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	type result struct {
		lt  *authdomain.LinkToken
		err error
	}
	results := make([]result, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := range 2 {
		i := i
		go func() {
			defer wg.Done()
			lt, err := tokens.Consume(ctx, auditmodel.LinkKindEmailVerification, rawToken)
			results[i] = result{lt, err}
		}()
	}
	wg.Wait()

	successes := 0
	failures := 0
	for _, r := range results {
		if r.err == nil {
			successes++
		} else if errors.Is(r.err, authdomain.ErrLinkInvalid) {
			failures++
		} else {
			t.Errorf("unexpected error: %v", r.err)
		}
	}
	if successes != 1 {
		t.Errorf("successes: got %d, want exactly 1", successes)
	}
	if failures != 1 {
		t.Errorf("failures: got %d, want exactly 1", failures)
	}
}

// ---------------------------------------------------------------------------
// TestLinkTokenStore_Consume_ReturnsErrLinkInvalidForExpiredToken
//
// Brief scenario: "Consume returns ErrLinkInvalid for an expired token, and
// the row is left unconsumed."  Expiry must not consume the row — an expired
// token that is written back as consumed would produce a misleading audit record.
// ---------------------------------------------------------------------------

func TestLinkTokenStore_Consume_ReturnsErrLinkInvalidForExpiredToken(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	accounts := NewAccountStore(pool)
	tokens := NewLinkTokenStore(pool)

	user := newTestAccount("expired@example.com")
	if err := accounts.Create(ctx, user); err != nil {
		t.Fatalf("Create user: %v", err)
	}

	// Issue a token with a TTL that has already expired.
	rawToken, err := tokens.Issue(ctx, user.ID, auditmodel.LinkKindPasswordReset, -time.Second, nil)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	_, err = tokens.Consume(ctx, auditmodel.LinkKindPasswordReset, rawToken)
	if !errors.Is(err, authdomain.ErrLinkInvalid) {
		t.Fatalf("Consume expired: got %v, want ErrLinkInvalid", err)
	}
	var linkErr *authdomain.LinkInvalidError
	if errors.As(err, &linkErr) {
		if linkErr.Reason != auditmodel.LinkRejectReasonExpired {
			t.Errorf("Reason: got %q, want Expired", linkErr.Reason)
		}
	}

	// The row must not have been marked consumed.
	var consumedAt *time.Time
	err = pool.QueryRow(ctx,
		`SELECT consumed_at FROM auth.link_tokens
		 WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`,
		[16]byte(user.ID),
	).Scan(&consumedAt)
	if err != nil {
		t.Fatalf("SELECT consumed_at: %v", err)
	}
	if consumedAt != nil {
		t.Error("consumed_at was set for an expired token — row must be left unconsumed")
	}
}

// ---------------------------------------------------------------------------
// TestLinkTokenStore_Consume_ReturnsErrLinkInvalidForUnknownToken
//
// A token that was never issued must return ErrLinkInvalid with reason Unknown.
// ---------------------------------------------------------------------------

func TestLinkTokenStore_Consume_ReturnsErrLinkInvalidForUnknownToken(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	tokens := NewLinkTokenStore(pool)

	_, err := tokens.Consume(ctx, auditmodel.LinkKindEmailVerification, "neverissuedtoken")
	if !errors.Is(err, authdomain.ErrLinkInvalid) {
		t.Errorf("Consume unknown: got %v, want ErrLinkInvalid", err)
	}
	var linkErr *authdomain.LinkInvalidError
	if errors.As(err, &linkErr) {
		if linkErr.Reason != auditmodel.LinkRejectReasonUnknown {
			t.Errorf("Reason: got %q, want Unknown", linkErr.Reason)
		}
	}
}

// ---------------------------------------------------------------------------
// TestLinkTokenStore_Issue_PayloadSurvivesRoundTrip
//
// The OAuth-link path stores a JSON payload; verify it is returned on Consume.
// ---------------------------------------------------------------------------

func TestLinkTokenStore_Issue_PayloadSurvivesRoundTrip(t *testing.T) {
	ctx, pool := testPool(t)
	cleanAuthTables(t, ctx, pool)

	accounts := NewAccountStore(pool)
	tokens := NewLinkTokenStore(pool)

	user := newTestAccount("payload@example.com")
	if err := accounts.Create(ctx, user); err != nil {
		t.Fatalf("Create user: %v", err)
	}

	type oauthPayload struct {
		Provider string `json:"provider"`
		Subject  string `json:"subject"`
		Email    string `json:"email"`
	}
	want := oauthPayload{Provider: "google", Subject: "sub123", Email: "payload@example.com"}

	rawToken, err := tokens.Issue(ctx, user.ID, auditmodel.LinkKindOauthLink, time.Hour, want)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	lt, err := tokens.Consume(ctx, auditmodel.LinkKindOauthLink, rawToken)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if lt.Payload == nil {
		t.Fatal("Consume: Payload is nil, want JSON")
	}
	if !strings.Contains(string(lt.Payload), "google") {
		t.Errorf("Payload: %q does not contain expected provider", lt.Payload)
	}
}

// ---------------------------------------------------------------------------
// TestLinkTokenStore_Consume_ErrLinkInvalidSatisfiesErrorsIs
//
// errors.Is(err, ErrLinkInvalid) must return true for any failure path, so
// callers can use a single errors.Is check rather than a type switch.
// ---------------------------------------------------------------------------

func TestLinkTokenStore_Consume_ErrLinkInvalidSatisfiesErrorsIs(t *testing.T) {
	ctx, pool := testPool(t)

	tokens := NewLinkTokenStore(pool)

	_, err := tokens.Consume(ctx, auditmodel.LinkKindEmailVerification, "bogus")
	if !errors.Is(err, authdomain.ErrLinkInvalid) {
		t.Errorf("errors.Is(err, ErrLinkInvalid): got false, want true; err=%v", err)
	}
}

// Compile-time checks.
var _ authdomain.LinkTokenRepo = (*LinkTokenStore)(nil)
var _ authdomain.OAuthIdentityRepo = (*OAuthIdentityStore)(nil)

// Ensure uuid.New is available (import check).
var _ = uuid.New
