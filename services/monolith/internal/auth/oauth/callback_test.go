package oauth_test

import (
	"context"
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
	oauthpkg "grindstats/services/monolith/internal/auth/oauth"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// testStateKey is a fixed HMAC key for tests.
var testStateKey = []byte("test-hmac-key-for-oauth-state-signing-32b")

// mockGoogleServer creates an httptest.Server acting as a minimal OAuth2 provider.
func mockGoogleServer(t *testing.T, sub, email string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "fake-access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":   sub,
				"email": email,
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

// fakeProvisioner is an authdomain.AccountProvisioner that returns a preset result.
type fakeProvisioner struct {
	result *authdomain.Account
	err    error
	calls  []provisionCall
}

type provisionCall struct {
	provider auditmodel.LinkedProvider
	subject  string
	email    string
}

func (p *fakeProvisioner) ProvisionFromOAuth(_ context.Context, provider auditmodel.LinkedProvider, subject, email string) (*authdomain.Account, error) {
	p.calls = append(p.calls, provisionCall{provider: provider, subject: subject, email: email})
	return p.result, p.err
}

// buildOAuthHandler creates an oauthpkg.Handler pointed at a mock Google server.
func buildOAuthHandler(
	t *testing.T,
	mockSrv *httptest.Server,
	provisioner authdomain.AccountProvisioner,
	accounts *fakeAccountRepo,
	tokens *fakeLinkTokenRepo,
	session *fakeSessionIssuer,
	m *fakeMailer,
	aud *auditlog.Fake,
) *oauthpkg.Handler {
	t.Helper()
	cfg := oauthpkg.Config{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURL:  "http://localhost/api/v1/auth/oauth/google/callback",
		StateKey:     testStateKey,
		CookieSecure: false,
	}
	h := oauthpkg.New(cfg, provisioner, accounts, tokens, session, m, aud)
	if mockSrv != nil {
		h.SetTestEndpoints(
			mockSrv.URL+"/token",
			mockSrv.URL+"/auth",
			mockSrv.URL+"/userinfo",
		)
		h.SetTestHTTPClient(mockSrv.Client())
	}
	return h
}

func routerWith(h *oauthpkg.Handler) *gin.Engine {
	r := gin.New()
	h.Register(r)
	return r
}

// Scenario: OAuth login matching an existing local account requires proof of control (TC-08)
// Then: redirect to /?auth_error=oauth_link_required, no session, no merge,
// auth.oauth.link_required written, link token emailed.
func TestCallback_OAuth_login_matching_an_existing_local_account_requires_proof_of_control(t *testing.T) {
	existingUserID := uuid.New()
	at := time.Now()
	existingAcc := authdomain.Account{
		ID:              existingUserID,
		Email:           "existing@example.com",
		EmailLower:      "existing@example.com",
		Role:            auditmodel.RoleUser,
		Status:          auditmodel.AccountStatusActive,
		EmailVerifiedAt: &at,
		CreatedAt:       time.Now(),
	}

	accounts := newFakeAccountRepo()
	accounts.seed(existingAcc)

	tokens := newFakeLinkTokenRepo()
	session := &fakeSessionIssuer{}
	m := &fakeMailer{}
	fake := auditlog.NewFake()

	// Provisioner returns ErrLinkRequired (local account exists, no identity row)
	provisioner := &fakeProvisioner{err: authdomain.ErrLinkRequired}

	mockSrv := mockGoogleServer(t, "google-sub-link", "existing@example.com")
	defer mockSrv.Close()

	h := buildOAuthHandler(t, mockSrv, provisioner, accounts, tokens, session, m, fake)
	router := routerWith(h)

	// Step 1: call /authorize to get a valid state cookie
	wAuth := httptest.NewRecorder()
	reqAuth := httptest.NewRequest(http.MethodGet, "/auth/oauth/google", nil)
	router.ServeHTTP(wAuth, reqAuth)
	require.Equal(t, http.StatusFound, wAuth.Code)

	var stateCookieValue string
	for _, c := range wAuth.Result().Cookies() {
		if c.Name == "gs_oauth_state" {
			stateCookieValue = c.Value
		}
	}
	require.NotEmpty(t, stateCookieValue, "gs_oauth_state cookie must be set by /authorize")

	// Extract the state value from the redirect URL
	authRedirect := wAuth.Header().Get("Location")
	stateParam := extractQueryParam(authRedirect, "state")
	require.NotEmpty(t, stateParam)

	// Step 2: simulate the callback
	wCB := httptest.NewRecorder()
	reqCB := httptest.NewRequest(http.MethodGet,
		"/auth/oauth/google/callback?code=fake-code&state="+stateParam, nil)
	reqCB.AddCookie(&http.Cookie{Name: "gs_oauth_state", Value: stateCookieValue})
	router.ServeHTTP(wCB, reqCB)

	// Must redirect to /?auth_error=oauth_link_required (contract §4.3)
	assert.Equal(t, http.StatusFound, wCB.Code)
	assert.Equal(t, "/?auth_error=oauth_link_required", wCB.Header().Get("Location"),
		"must redirect to oauth_link_required when ErrLinkRequired (§4.3)")

	// No session must be issued (§4.3: no session, no merge)
	assert.Equal(t, 0, session.issueCnt,
		"no session must be issued when proof of control is required (§4.3)")

	// One oauth_link token must have been issued
	require.Len(t, tokens.issued, 1, "one oauth_link token must be issued")

	// Token must have been emailed
	assert.Len(t, m.oauthLinks, 1, "oauth link token must be sent by email")

	// auth.oauth.link_required audit event must be written
	evts := fake.EventsOf(auditmodel.EvtAuthOauthLinkRequired)
	require.Len(t, evts, 1, "auth.oauth.link_required event must be written")
	assert.Equal(t, existingUserID.String(), evts[0].Fields["user_id"])
	assert.Equal(t, auditmodel.LinkedProviderGoogle, evts[0].Fields["provider"])
}

// Callback with user-denied error redirects to oauth_cancelled.
func TestCallback_user_denied_redirects_to_oauth_cancelled(t *testing.T) {
	h := buildOAuthHandler(t, nil,
		&fakeProvisioner{}, newFakeAccountRepo(), newFakeLinkTokenRepo(),
		&fakeSessionIssuer{}, &fakeMailer{}, auditlog.NewFake())
	router := routerWith(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/auth/oauth/google/callback?error=access_denied", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/?auth_error=oauth_cancelled", w.Header().Get("Location"))
}

// Callback with a bad state (or missing cookie) redirects to oauth_failed.
func TestCallback_bad_state_redirects_to_oauth_failed(t *testing.T) {
	h := buildOAuthHandler(t, nil,
		&fakeProvisioner{}, newFakeAccountRepo(), newFakeLinkTokenRepo(),
		&fakeSessionIssuer{}, &fakeMailer{}, auditlog.NewFake())
	router := routerWith(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/auth/oauth/google/callback?code=abc&state=tampered-state", nil)
	// Provide a bogus cookie that won't verify
	req.AddCookie(&http.Cookie{Name: "gs_oauth_state", Value: "bad.cookie"})
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/?auth_error=oauth_failed", w.Header().Get("Location"))
}

// Authorize sets the state cookie and redirects to the consent screen.
func TestAuthorize_sets_state_cookie_and_redirects(t *testing.T) {
	mockSrv := mockGoogleServer(t, "", "")
	defer mockSrv.Close()

	h := buildOAuthHandler(t, mockSrv,
		&fakeProvisioner{}, newFakeAccountRepo(), newFakeLinkTokenRepo(),
		&fakeSessionIssuer{}, &fakeMailer{}, auditlog.NewFake())
	router := routerWith(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/oauth/google", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	location := w.Header().Get("Location")
	assert.Contains(t, location, "state=", "authorize URL must contain state parameter")
	assert.Contains(t, location, "code_challenge=", "authorize URL must contain PKCE challenge")
	assert.Contains(t, location, "code_challenge_method=S256", "PKCE method must be S256")

	// gs_oauth_state cookie must be set with HttpOnly, correct TTL
	var stateCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "gs_oauth_state" {
			stateCookie = c
		}
	}
	require.NotNil(t, stateCookie, "gs_oauth_state cookie must be set")
	assert.True(t, stateCookie.HttpOnly)
	assert.Equal(t, 600, stateCookie.MaxAge)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// extractQueryParam returns the value of a query parameter from a raw URL string.
func extractQueryParam(rawURL, key string) string {
	idx := strings.Index(rawURL, "?")
	if idx < 0 {
		return ""
	}
	for _, part := range strings.Split(rawURL[idx+1:], "&") {
		if strings.HasPrefix(part, key+"=") {
			return part[len(key)+1:]
		}
	}
	return ""
}

// compile-time check
var _ authdomain.AccountProvisioner = (*fakeProvisioner)(nil)
