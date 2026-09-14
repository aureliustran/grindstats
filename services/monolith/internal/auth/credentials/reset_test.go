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
	"grindstats/services/monolith/internal/auth/credentials"
)

// Scenario: password reset completes and logs out every session (TC-12)
func TestReset_password_reset_completes_and_logs_out_every_session(t *testing.T) {
	accounts := newFakeAccountRepo()
	tokens := newFakeLinkTokenRepo()
	session := &fakeSessionIssuer{}
	userID := uuid.New()

	hasher, err := credentials.NewPasswordHasher(credentials.FastArgon2Params)
	require.NoError(t, err)
	oldHash, err := hasher.Hash("OldPassword1!")
	require.NoError(t, err)
	accounts.seed(authdomain.Account{
		ID:           userID,
		Email:        "i@example.com",
		EmailLower:   "i@example.com",
		PasswordHash: oldHash,
		Role:         auditmodel.RoleUser,
		Status:       auditmodel.AccountStatusActive,
		CreatedAt:    time.Now(),
	})

	// Issue a reset token for this user
	rawToken, err := tokens.Issue(nil, userID, auditmodel.LinkKindPasswordReset, time.Hour, nil)
	require.NoError(t, err)

	fake := auditlog.NewFake()
	h := buildHandler(accounts, newFakeOAuthIdentityRepo(), tokens,
		session, &fakeHIBP{}, &fakeMailer{}, fake)
	router := routerWith(h)

	w := postJSON(router, "/auth/password-reset/confirm", map[string]any{
		"token":    rawToken,
		"password": "NewStrongPass1!",
	})

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data struct{ Status string } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "reset", resp.Data.Status)

	// RevokeAll must have been called exactly once with the correct user ID
	assert.Equal(t, 1, session.revokeCallCount(),
		"SessionIssuer.RevokeAll must be called exactly once on success")
	assert.Equal(t, userID, session.revokeAllIDs[0],
		"RevokeAll must be called with the correct user ID")

	// Password hash must be updated
	acc := accounts.get("i@example.com")
	require.NotNil(t, acc)
	assert.NotEqual(t, oldHash, acc.PasswordHash, "password hash must be updated after reset")

	// Audit event
	evts := fake.EventsOf(auditmodel.EvtAuthPasswordResetCompleted)
	require.Len(t, evts, 1)
	assert.Equal(t, userID.String(), evts[0].Fields["user_id"])
}

// Scenario: expired or already-used reset token is rejected (TC-13)
// Password must remain unchanged and RevokeAll must NOT be called.
func TestReset_expired_or_already_used_reset_token_is_rejected(t *testing.T) {
	accounts := newFakeAccountRepo()
	tokens := newFakeLinkTokenRepo()
	session := &fakeSessionIssuer{}
	userID := uuid.New()

	hasher, err := credentials.NewPasswordHasher(credentials.FastArgon2Params)
	require.NoError(t, err)
	oldHash, err := hasher.Hash("OldPassword1!")
	require.NoError(t, err)
	accounts.seed(authdomain.Account{
		ID:           userID,
		Email:        "j@example.com",
		EmailLower:   "j@example.com",
		PasswordHash: oldHash,
		Role:         auditmodel.RoleUser,
		Status:       auditmodel.AccountStatusActive,
		CreatedAt:    time.Now(),
	})

	// Seed an already-consumed (expired/used) reset token
	invalidRaw := "invalid-reset-token"
	tokens.seedInvalid(invalidRaw, auditmodel.LinkKindPasswordReset, userID)

	fake := auditlog.NewFake()
	h := buildHandler(accounts, newFakeOAuthIdentityRepo(), tokens,
		session, &fakeHIBP{}, &fakeMailer{}, fake)
	router := routerWith(h)

	w := postJSON(router, "/auth/password-reset/confirm", map[string]any{
		"token":    invalidRaw,
		"password": "NewStrongPass1!",
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, string(auditmodel.ErrAuthLinkInvalid), body["error"].(map[string]any)["code"])

	// RevokeAll must NOT have been called (TC-13)
	assert.Equal(t, 0, session.revokeCallCount(),
		"SessionIssuer.RevokeAll must NOT be called on a rejected token (TC-13)")

	// Password must be unchanged
	acc := accounts.get("j@example.com")
	require.NotNil(t, acc)
	assert.Equal(t, oldHash, acc.PasswordHash,
		"password must remain unchanged when the reset token is rejected (TC-13)")

	// auth.link.rejected must be written
	evts := fake.EventsOf(auditmodel.EvtAuthLinkRejected)
	require.Len(t, evts, 1)
	assert.Equal(t, auditmodel.LinkKindPasswordReset, evts[0].Fields["kind"])
}

// Scenario: password-reset request does not confirm whether the email exists (TC-15)
// The two responses must be BYTE-EQUAL (FR-08, §1.1).
func TestReset_password_reset_request_does_not_confirm_whether_the_email_exists(t *testing.T) {
	accounts := newFakeAccountRepo()
	userID := uuid.New()
	accounts.seed(authdomain.Account{
		ID:         userID,
		Email:      "k@example.com",
		EmailLower: "k@example.com",
		Role:       auditmodel.RoleUser,
		Status:     auditmodel.AccountStatusActive,
		CreatedAt:  time.Now(),
	})

	h := buildHandler(accounts, newFakeOAuthIdentityRepo(), newFakeLinkTokenRepo(),
		&fakeSessionIssuer{}, &fakeHIBP{}, &fakeMailer{}, auditlog.NewFake())
	router := routerWith(h)

	// Known email
	wKnown := postJSON(router, "/auth/password-reset/request", map[string]any{
		"email": "k@example.com",
	})
	// Unknown email
	wUnknown := postJSON(router, "/auth/password-reset/request", map[string]any{
		"email": "nobody@example.com",
	})

	// Status must be identical
	assert.Equal(t, wKnown.Code, wUnknown.Code, "status codes must be equal (FR-08)")

	// Body bytes must be byte-equal (TC-15)
	assert.Equal(t, wKnown.Body.String(), wUnknown.Body.String(),
		"response bodies must be byte-equal for known vs unknown email (FR-08, §1.1, TC-15)")
}
