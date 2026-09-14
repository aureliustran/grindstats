package credentials_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/services/monolith/internal/auth/authdomain"
)

// Scenario: verification token expires after 24 hours (TC-10)
// The fake repo uses "already consumed" to simulate expiry (both map to the same error).
func TestVerifyEmail_verification_token_expires_after_24_hours(t *testing.T) {
	tokens := newFakeLinkTokenRepo()
	userID := uuid.New()
	expiredRaw := "expired-token-raw"
	tokens.seedInvalid(expiredRaw, auditmodel.LinkKindEmailVerification, userID)

	fake := auditlog.NewFake()
	h := buildHandler(newFakeAccountRepo(), newFakeOAuthIdentityRepo(), tokens,
		&fakeSessionIssuer{}, &fakeHIBP{}, &fakeMailer{}, fake)
	router := routerWith(h)

	w := postJSON(router, "/auth/verify-email", map[string]any{"token": expiredRaw})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, string(auditmodel.ErrAuthLinkInvalid), body["error"].(map[string]any)["code"])

	// auth.link.rejected event must be written
	evts := fake.EventsOf(auditmodel.EvtAuthLinkRejected)
	require.Len(t, evts, 1)
	assert.Equal(t, auditmodel.LinkKindEmailVerification, evts[0].Fields["kind"])
}

// Scenario: verification token is single-use (TC-11)
// A second use is rejected; the account remains verified after the first use.
func TestVerifyEmail_verification_token_is_single_use(t *testing.T) {
	accounts := newFakeAccountRepo()
	tokens := newFakeLinkTokenRepo()
	userID := uuid.New()
	at := time.Now()
	accounts.seed(authdomain.Account{
		ID:           userID,
		Email:        "h@example.com",
		EmailLower:   "h@example.com",
		Role:         auditmodel.RoleUser,
		Status:       auditmodel.AccountStatusActive,
		CreatedAt:    time.Now(),
	})

	fake := auditlog.NewFake()
	h := buildHandler(accounts, newFakeOAuthIdentityRepo(), tokens,
		&fakeSessionIssuer{}, &fakeHIBP{}, &fakeMailer{}, fake)
	router := routerWith(h)

	// Issue a real token and consume it the first time
	rawToken, err := tokens.Issue(nil, userID, auditmodel.LinkKindEmailVerification, 24*time.Hour, nil)
	require.NoError(t, err)

	// First use succeeds
	w1 := postJSON(router, "/auth/verify-email", map[string]any{"token": rawToken})
	assert.Equal(t, http.StatusOK, w1.Code)
	var ok struct {
		Data struct{ Status string } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &ok))
	assert.Equal(t, "verified", ok.Data.Status)

	// account now has EmailVerifiedAt set — it stays set
	acc := accounts.get("h@example.com")
	assert.NotNil(t, acc)
	assert.NotNil(t, acc.EmailVerifiedAt, "account should be verified after first use")
	_ = at // suppress unused warning

	// Second use must be rejected
	w2 := postJSON(router, "/auth/verify-email", map[string]any{"token": rawToken})
	assert.Equal(t, http.StatusBadRequest, w2.Code)
	var errBody map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &errBody))
	assert.Equal(t, string(auditmodel.ErrAuthLinkInvalid), errBody["error"].(map[string]any)["code"])

	// Account must still be verified (not un-verified by the rejection)
	acc2 := accounts.get("h@example.com")
	assert.NotNil(t, acc2.EmailVerifiedAt,
		"second use rejection must not un-verify the account")
}
