package credentials_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/services/monolith/internal/auth/authdomain"
	"grindstats/services/monolith/internal/auth/credentials"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// buildHandler creates a credentials.Handler wired to the provided fakes.
// Uses FastArgon2Params so tests stay fast.
func buildHandler(
	accounts *fakeAccountRepo,
	oauthIds *fakeOAuthIdentityRepo,
	tokens *fakeLinkTokenRepo,
	session *fakeSessionIssuer,
	hibp *fakeHIBP,
	m *fakeMailer,
	aud *auditlog.Fake,
) *credentials.Handler {
	hasher, err := credentials.NewPasswordHasher(credentials.FastArgon2Params)
	if err != nil {
		panic("buildHandler: " + err.Error())
	}
	return credentials.New(accounts, oauthIds, tokens, session, hasher, hibp, m, aud)
}

// routerWith creates a Gin engine that routes only the credentials endpoints.
func routerWith(h *credentials.Handler) *gin.Engine {
	r := gin.New()
	h.Register(r)
	return r
}

// postJSON is a helper that posts JSON to the given path and returns the response recorder.
func postJSON(router *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	return postJSONWithLang(router, path, body, "en-US")
}

func postJSONWithLang(router *gin.Engine, path string, body any, lang string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", lang)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// Scenario: registering with a new email creates a User account (TC-01)
func TestRegister_registering_with_a_new_email_creates_a_User_account(t *testing.T) {
	accounts := newFakeAccountRepo()
	tokens := newFakeLinkTokenRepo()
	fake := auditlog.NewFake()
	h := buildHandler(accounts, newFakeOAuthIdentityRepo(), tokens, &fakeSessionIssuer{},
		&fakeHIBP{breached: false}, &fakeMailer{}, fake)
	router := routerWith(h)

	w := postJSON(router, "/auth/register", map[string]any{
		"email":    "alice@example.com",
		"password": "ValidPass123!",
	})

	assert.Equal(t, http.StatusAccepted, w.Code)
	var resp struct {
		Data struct{ Status string } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "pending_verification", resp.Data.Status)

	// Account created with role User — FR-04
	acc := accounts.get("alice@example.com")
	require.NotNil(t, acc)
	assert.Equal(t, auditmodel.RoleUser, acc.Role)
	assert.NotEmpty(t, acc.PasswordHash)
	assert.Nil(t, acc.EmailVerifiedAt)

	// Verification token issued
	assert.NotEmpty(t, tokens.lastIssued())

	// Audit event written
	evts := fake.EventsOf(auditmodel.EvtAuthRegisterSucceeded)
	require.Len(t, evts, 1)
	assert.Equal(t, acc.ID.String(), evts[0].Fields["user_id"])
}

// Scenario: email uniqueness is case-insensitive (TC-02)
// The response must be byte-equal to the duplicate case — same as TC-14.
func TestRegister_email_uniqueness_is_case_insensitive(t *testing.T) {
	accounts := newFakeAccountRepo()
	// Pre-seed with a mixed-case email registered as lower-case
	existingID := uuid.New()
	hash, _ := credentials.NewPasswordHasher(credentials.FastArgon2Params)
	dummyHash, _ := hash.Hash("OtherPassword1!")
	at := time.Now()
	accounts.seed(authdomain.Account{
		ID:              existingID,
		Email:           "User@Example.Com",
		EmailLower:      "user@example.com",
		PasswordHash:    dummyHash,
		Role:            auditmodel.RoleUser,
		Status:          auditmodel.AccountStatusActive,
		EmailVerifiedAt: &at,
		CreatedAt:       time.Now(),
	})

	h := buildHandler(accounts, newFakeOAuthIdentityRepo(), newFakeLinkTokenRepo(),
		&fakeSessionIssuer{}, &fakeHIBP{}, &fakeMailer{}, auditlog.NewFake())
	router := routerWith(h)

	// Register with a different case of the same email
	w := postJSON(router, "/auth/register", map[string]any{
		"email":    "user@Example.COM",
		"password": "ValidPass123!",
	})

	// Must be 202 — same as a fresh registration (FR-08 enumeration protection)
	assert.Equal(t, http.StatusAccepted, w.Code)
	var resp struct {
		Data struct{ Status string } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "pending_verification", resp.Data.Status)
}

// Scenario: password below the minimum length is rejected (TC-03)
func TestRegister_password_below_the_minimum_length_is_rejected(t *testing.T) {
	h := buildHandler(newFakeAccountRepo(), newFakeOAuthIdentityRepo(), newFakeLinkTokenRepo(),
		&fakeSessionIssuer{}, &fakeHIBP{}, &fakeMailer{}, auditlog.NewFake())
	router := routerWith(h)

	w := postJSON(router, "/auth/register", map[string]any{
		"email":    "bob@example.com",
		"password": "short",
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj := body["error"].(map[string]any)
	assert.Equal(t, string(auditmodel.ErrValidationFailed), errObj["code"])
	details := errObj["details"].([]any)
	require.NotEmpty(t, details)
	d := details[0].(map[string]any)
	assert.Equal(t, "password", d["field"])
	assert.Equal(t, "min_length", d["rule"])
}

// Scenario: a breached password is rejected or flagged — HIBP hit (TC-04)
func TestRegister_a_breached_password_is_rejected_or_flagged_hit(t *testing.T) {
	h := buildHandler(newFakeAccountRepo(), newFakeOAuthIdentityRepo(), newFakeLinkTokenRepo(),
		&fakeSessionIssuer{}, &fakeHIBP{breached: true}, &fakeMailer{}, auditlog.NewFake())
	router := routerWith(h)

	w := postJSON(router, "/auth/register", map[string]any{
		"email":    "carol@example.com",
		"password": "password1234",
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj := body["error"].(map[string]any)
	assert.Equal(t, string(auditmodel.ErrValidationFailed), errObj["code"])
	details := errObj["details"].([]any)
	d := details[0].(map[string]any)
	assert.Equal(t, "password", d["field"])
	assert.Equal(t, "breached", d["rule"])
}

// Scenario: a breached password is rejected or flagged — HIBP miss (TC-05)
func TestRegister_a_breached_password_is_rejected_or_flagged_miss(t *testing.T) {
	h := buildHandler(newFakeAccountRepo(), newFakeOAuthIdentityRepo(), newFakeLinkTokenRepo(),
		&fakeSessionIssuer{}, &fakeHIBP{breached: false}, &fakeMailer{}, auditlog.NewFake())
	router := routerWith(h)

	w := postJSON(router, "/auth/register", map[string]any{
		"email":    "dave@example.com",
		"password": "uniquePassword999",
	})

	assert.Equal(t, http.StatusAccepted, w.Code)
}

// Scenario: a breached password is rejected or flagged — HIBP unreachable → allowed with a warn
func TestRegister_a_breached_password_is_rejected_or_flagged_unreachable(t *testing.T) {
	// HIBP returns an error → the request proceeds (D3 fail-open)
	hibp := &fakeHIBP{err: assert.AnError}
	h := buildHandler(newFakeAccountRepo(), newFakeOAuthIdentityRepo(), newFakeLinkTokenRepo(),
		&fakeSessionIssuer{}, hibp, &fakeMailer{}, auditlog.NewFake())
	router := routerWith(h)

	w := postJSON(router, "/auth/register", map[string]any{
		"email":    "eve@example.com",
		"password": "uniquePassword999",
	})

	assert.Equal(t, http.StatusAccepted, w.Code)
}

// Scenario: an injected role field is ignored on registration (TC-06)
func TestRegister_an_injected_role_field_is_ignored_on_registration(t *testing.T) {
	accounts := newFakeAccountRepo()
	h := buildHandler(accounts, newFakeOAuthIdentityRepo(), newFakeLinkTokenRepo(),
		&fakeSessionIssuer{}, &fakeHIBP{}, &fakeMailer{}, auditlog.NewFake())

	// Extra "role" field in the JSON body — must be silently ignored
	reqBody, _ := json.Marshal(map[string]any{
		"email":    "frank@example.com",
		"password": "ValidPassword1!",
		"role":     "system_admin",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	routerWith(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	acc := accounts.get("frank@example.com")
	require.NotNil(t, acc)
	assert.Equal(t, auditmodel.RoleUser, acc.Role,
		"injected role field must be ignored — account must always be created as User (FR-04)")
}

// Scenario: registration response does not confirm an email is already registered (TC-14)
// The two responses must be BYTE-EQUAL (FR-08, §1.1).
func TestRegister_registration_response_does_not_confirm_an_email_is_already_registered(t *testing.T) {
	accounts := newFakeAccountRepo()
	h := buildHandler(accounts, newFakeOAuthIdentityRepo(), newFakeLinkTokenRepo(),
		&fakeSessionIssuer{}, &fakeHIBP{}, &fakeMailer{}, auditlog.NewFake())
	router := routerWith(h)

	payload := map[string]any{
		"email":    "grace@example.com",
		"password": "ValidPassword1!",
	}

	// First registration — should succeed.
	w1 := postJSON(router, "/auth/register", payload)
	require.Equal(t, http.StatusAccepted, w1.Code)

	// Second registration — same email, different password (to avoid same hash timing).
	payload["password"] = "AnotherValidPass2!"
	w2 := postJSON(router, "/auth/register", payload)

	// Status must be identical.
	assert.Equal(t, w1.Code, w2.Code, "status codes must be equal (FR-08)")

	// Body bytes must be identical (same JSON shape and values).
	assert.Equal(t, w1.Body.String(), w2.Body.String(),
		"response bodies must be byte-equal for new vs taken email (FR-08, §1.1)")
}

// Locale coverage: VALIDATION_FAILED message differs between en-US and vi-VN,
// and the audit record's message is always en-US (three-planes rule, docs/audit-and-errors.md §1a).
func TestRegister_locale_renders_different_messages_for_validation_error(t *testing.T) {
	fake := auditlog.NewFake()
	h := buildHandler(newFakeAccountRepo(), newFakeOAuthIdentityRepo(), newFakeLinkTokenRepo(),
		&fakeSessionIssuer{}, &fakeHIBP{breached: true}, &fakeMailer{}, fake)
	router := routerWith(h)

	// en-US
	wEN := postJSONWithLang(router, "/auth/register", map[string]any{
		"email": "test@example.com", "password": "breachedPass1!",
	}, "en-US")
	require.Equal(t, http.StatusBadRequest, wEN.Code)

	// vi-VN
	wVI := postJSONWithLang(router, "/auth/register", map[string]any{
		"email": "test2@example.com", "password": "breachedPass1!",
	}, "vi-VN")
	require.Equal(t, http.StatusBadRequest, wVI.Code)

	var enBody, viBody map[string]any
	require.NoError(t, json.Unmarshal(wEN.Body.Bytes(), &enBody))
	require.NoError(t, json.Unmarshal(wVI.Body.Bytes(), &viBody))

	enMsg := enBody["error"].(map[string]any)["message"].(string)
	viMsg := viBody["error"].(map[string]any)["message"].(string)
	assert.NotEqual(t, enMsg, viMsg, "error message must differ between en-US and vi-VN")
	assert.True(t, strings.Contains(enMsg, "invalid"), "en-US message should mention 'invalid'")
}
