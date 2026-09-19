//go:build integration

package integration

// e2e_test.go — SRS-AUTH-001 §6 acceptance criteria 1–4 as a runnable test.
//
// This test requires a live Postgres (GRINDSTATS_TEST_DB) and either a live
// Redis (GRINDSTATS_TEST_REDIS) or a miniredis fallback. It exercises the
// complete session lifecycle:
//
//	register → consume the verification link → login
//	  → assert: two cookies with the contract's attributes, CSRF token in the
//	            body, and no token string anywhere in the body
//	  → authenticated GET /users/me
//	  → POST /auth/refresh → new pair, same sid
//	  → replay the pre-rotation refresh token → 401, and every session is dead
//	  → (fresh login) unsafe request with and without X-CSRF-Token → 200 / 403
//	  → logout → the captured access cookie is rejected on the next call,
//	             without waiting for expiry
//
// Session routes are split across the gateway's public and protected groups
// via session.Handler.RegisterPublic / RegisterProtected (AMD-005, resolved).

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/auditlog"
	"grindstats/libs/authmw"
	"grindstats/services/monolith/internal/auth/credentials"
	"grindstats/services/monolith/internal/auth/hibp"
	"grindstats/services/monolith/internal/auth/oauth"
	"grindstats/services/monolith/internal/auth/session"
	"grindstats/services/monolith/internal/auth/store"
	"grindstats/services/monolith/internal/gateway"
)

// ─── e2e stack ────────────────────────────────────────────────────────────────

// e2eStack builds the full gateway+handler stack against a real Postgres pool
// and either a real Redis or miniredis.
type e2eStack struct {
	server      *httptest.Server
	linkCapture *capturingMailer
	rdb         *redis.Client
}

// capturingMailer captures emitted deep-links so the e2e test can consume
// them without a real email inbox.
type capturingMailer struct {
	mu             sync.Mutex
	verifications  []string
	passwordResets []string
	oauthLinks     []string
}

func (m *capturingMailer) SendVerification(_ context.Context, _, rawToken string) error {
	m.mu.Lock()
	m.verifications = append(m.verifications, rawToken)
	m.mu.Unlock()
	return nil
}
func (m *capturingMailer) SendPasswordReset(_ context.Context, _, rawToken string) error {
	m.mu.Lock()
	m.passwordResets = append(m.passwordResets, rawToken)
	m.mu.Unlock()
	return nil
}
func (m *capturingMailer) SendOAuthLink(_ context.Context, _, rawToken string) error {
	m.mu.Lock()
	m.oauthLinks = append(m.oauthLinks, rawToken)
	m.mu.Unlock()
	return nil
}
func (m *capturingMailer) popVerification() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.verifications) == 0 {
		return "", false
	}
	tok := m.verifications[0]
	m.verifications = m.verifications[1:]
	return tok, true
}

// newE2EStack constructs the full stack with real DB + Redis (or miniredis
// fallback). Skips the test if GRINDSTATS_TEST_DB is not set.
func newE2EStack(t *testing.T) *e2eStack {
	t.Helper()

	dbURL := os.Getenv("GRINDSTATS_TEST_DB")
	if dbURL == "" {
		t.Skip("GRINDSTATS_TEST_DB not set; skipping e2e test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("e2e: connect postgres: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("e2e: ping postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	// Redis: prefer GRINDSTATS_TEST_REDIS env var; fall back to miniredis.
	var rdb *redis.Client
	if redisAddr := os.Getenv("GRINDSTATS_TEST_REDIS"); redisAddr != "" {
		rdb = redis.NewClient(&redis.Options{Addr: redisAddr})
	} else {
		mr := miniredis.RunT(t)
		rdb = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	}
	t.Cleanup(func() { _ = rdb.Close() })

	// Generate in-memory RSA keys so the e2e test doesn't need files on disk.
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("e2e: generate RSA key: %v", err)
	}
	ks := authmw.NewKeySetFromMemory(
		"e2e-kid",
		priv,
		map[string]*rsa.PublicKey{"e2e-kid": &priv.PublicKey},
	)

	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	auditWriter := auditlog.New(pool, logger)

	accountStore := store.NewAccountStore(pool)
	oauthIdentityStore := store.NewOAuthIdentityStore(pool)
	linkTokenStore := store.NewLinkTokenStore(pool)

	hasher, err := credentials.NewPasswordHasher(credentials.FastArgon2Params)
	if err != nil {
		t.Fatalf("e2e: init hasher: %v", err)
	}

	cookieOpts := authmw.CookieOptions{Secure: false} // plain http in tests
	sessionHandler := session.New(ks, rdb, accountStore, auditWriter, cookieOpts, hasher, logger)

	hibpClient := hibp.New(false, 2*time.Second)
	cap := &capturingMailer{}

	credHandler := credentials.New(
		accountStore, oauthIdentityStore, linkTokenStore,
		sessionHandler, hasher, hibpClient, cap, auditWriter,
	)

	provisioner := credentials.NewProvisioner(accountStore, oauthIdentityStore, auditWriter)
	oauthHandler := oauth.New(
		oauth.Config{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURL:  "http://localhost/api/v1/auth/oauth/google/callback",
			StateKey:     []byte("test-state-key-32-bytes-for-hmac"),
			CookieSecure: false,
		},
		provisioner, accountStore, linkTokenStore, sessionHandler, cap, auditWriter,
	)

	srv := gateway.New(gateway.Deps{
		Logger:   logger,
		KeySet:   ks,
		Redis:    rdb,
		AuditLog: auditWriter,
	})

	// Route wiring — mirrors main.go.
	credHandler.Register(srv.V1)
	oauthHandler.Register(srv.V1)
	sessionHandler.RegisterPublic(srv.V1)
	sessionHandler.RegisterProtected(srv.Protected)

	ts := httptest.NewServer(srv.Engine)
	t.Cleanup(ts.Close)

	return &e2eStack{server: ts, linkCapture: cap, rdb: rdb}
}

// ─── e2e helper methods ───────────────────────────────────────────────────────

func (s *e2eStack) do(t *testing.T, method, path string, body any, cookies []*http.Cookie, headers map[string]string) *http.Response {
	t.Helper()
	var bodyReader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("e2e.do: marshal: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, s.server.URL+path, bodyReader)
	if err != nil {
		t.Fatalf("e2e.do: build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := s.server.Client().Do(req)
	if err != nil {
		t.Fatalf("e2e.do %s %s: %v", method, path, err)
	}
	return resp
}

func (s *e2eStack) postJSON(t *testing.T, path string, body any, cookies []*http.Cookie, extraHeaders map[string]string) *http.Response {
	return s.do(t, http.MethodPost, path, body, cookies, extraHeaders)
}

func (s *e2eStack) getJSON(t *testing.T, path string, cookies []*http.Cookie) *http.Response {
	return s.do(t, http.MethodGet, path, nil, cookies, nil)
}

func readJSON(t *testing.T, r *http.Response, out any) {
	t.Helper()
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		t.Fatalf("readJSON: %v", err)
	}
}

func cookiesFromResponse(r *http.Response) []*http.Cookie {
	return r.Cookies()
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// ─── SRS-AUTH-001 §6 end-to-end test ─────────────────────────────────────────

// TestE2E_AuthLifecycle exercises the complete session lifecycle from
// registration through logout. Requires GRINDSTATS_TEST_DB (see newE2EStack).
func TestE2E_AuthLifecycle(t *testing.T) {
	s := newE2EStack(t)
	ctx := context.Background()
	_ = ctx

	const email = "e2e@example.com"
	const password = "E2eTestPassword123!"

	// ── Stage 1: Register ─────────────────────────────────────────────────────
	t.Run("register", func(t *testing.T) {
		r := s.postJSON(t, "/api/v1/auth/register", map[string]string{
			"email": email, "password": password,
		}, nil, nil)
		if r.StatusCode != http.StatusAccepted {
			t.Fatalf("register: want 202, got %d", r.StatusCode)
		}
		var body struct {
			Data struct{ Status string } `json:"data"`
		}
		readJSON(t, r, &body)
		if body.Data.Status != "pending_verification" {
			t.Errorf("register: status = %q, want pending_verification", body.Data.Status)
		}
	})

	// ── Stage 2: Consume the verification link ────────────────────────────────
	t.Run("verify_email", func(t *testing.T) {
		tok, ok := s.linkCapture.popVerification()
		if !ok {
			t.Fatal("verify_email: no verification token captured")
		}
		r := s.postJSON(t, "/api/v1/auth/verify-email", map[string]string{"token": tok}, nil, nil)
		if r.StatusCode != http.StatusOK {
			t.Fatalf("verify_email: want 200, got %d", r.StatusCode)
		}
	})

	// ── Stage 3: Login ────────────────────────────────────────────────────────
	var accessCookie, refreshCookie *http.Cookie
	var csrfToken string

	t.Run("login", func(t *testing.T) {
		r := s.postJSON(t, "/api/v1/auth/login", map[string]string{
			"email": email, "password": password,
		}, nil, nil)
		if r.StatusCode != http.StatusOK {
			t.Fatalf("login: want 200, got %d", r.StatusCode)
		}

		// Contract §2.2: two HttpOnly cookies.
		accessCookie = findCookie(r.Cookies(), authmw.AccessCookieName)
		refreshCookie = findCookie(r.Cookies(), authmw.RefreshCookieName)
		if accessCookie == nil {
			t.Fatal("login: missing gs_access cookie")
		}
		if refreshCookie == nil {
			t.Fatal("login: missing gs_refresh cookie")
		}
		if !accessCookie.HttpOnly {
			t.Errorf("login: gs_access cookie not HttpOnly")
		}
		if !refreshCookie.HttpOnly {
			t.Errorf("login: gs_refresh cookie not HttpOnly")
		}

		// Contract §2.3: CSRF token in response body, no raw JWT string.
		var body struct {
			Data struct {
				CSRFToken string `json:"csrf_token"`
				User      struct{ ID string } `json:"user"`
			} `json:"data"`
		}
		readJSON(t, r, &body)
		csrfToken = body.Data.CSRFToken
		if csrfToken == "" {
			t.Error("login: csrf_token missing from response body")
		}

		// Contract §2.3: token strings must not appear in the body.
		rawBody := fmt.Sprintf("%+v", body)
		if strings.Contains(rawBody, accessCookie.Value) {
			t.Error("login: access token string leaked into response body")
		}
	})

	if accessCookie == nil || refreshCookie == nil {
		t.Skip("skipping remaining stages: login failed")
	}

	// ── Stage 4: GET /users/me  ─────────────────────────────────────────────
	t.Run("users_me", func(t *testing.T) {
		r := s.getJSON(t, "/api/v1/users/me", []*http.Cookie{accessCookie})
		if r.StatusCode != http.StatusOK {
			t.Fatalf("users/me: want 200, got %d", r.StatusCode)
		}
		var body struct {
			Data struct {
				User struct {
					Email         string `json:"email"`
					EmailVerified bool   `json:"email_verified"`
				} `json:"user"`
				CSRFToken string `json:"csrf_token"`
			} `json:"data"`
		}
		readJSON(t, r, &body)
		if body.Data.User.Email != email {
			t.Errorf("users/me: email = %q, want %q", body.Data.User.Email, email)
		}
		if !body.Data.User.EmailVerified {
			t.Error("users/me: email_verified = false, want true (AMD-003 — verified in stage 2)")
		}
		if body.Data.CSRFToken == "" {
			t.Error("users/me: csrf_token missing from response body")
		}
	})

	// ── Stage 5: POST /auth/refresh ───────────────────────────────────────────
	var newAccessCookie, newRefreshCookie *http.Cookie
	var rotatedCSRFToken string

	t.Run("refresh", func(t *testing.T) {
		r := s.postJSON(t, "/api/v1/auth/refresh", nil, []*http.Cookie{refreshCookie}, nil)
		if r.StatusCode != http.StatusOK {
			t.Fatalf("refresh: want 200, got %d", r.StatusCode)
		}
		newAccessCookie = findCookie(r.Cookies(), authmw.AccessCookieName)
		newRefreshCookie = findCookie(r.Cookies(), authmw.RefreshCookieName)
		if newAccessCookie == nil || newRefreshCookie == nil {
			t.Fatal("refresh: missing cookies in response")
		}
		// The new access token must differ from the original.
		if newAccessCookie.Value == accessCookie.Value {
			t.Error("refresh: access token was not rotated")
		}
		var body struct {
			Data struct {
				CSRFToken string `json:"csrf_token"`
			} `json:"data"`
		}
		readJSON(t, r, &body)
		rotatedCSRFToken = body.Data.CSRFToken
		// Rotation preserves the CSRF binding (contract §2.3) — same token as login.
		if rotatedCSRFToken != csrfToken {
			t.Errorf("refresh: csrf_token = %q, want unchanged %q (rotation preserves it)", rotatedCSRFToken, csrfToken)
		}
	})

	// ── Stage 6: Replay the pre-rotation refresh token ───────────────────────
	t.Run("replay_old_refresh_kills_sessions", func(t *testing.T) {
		if newRefreshCookie == nil {
			t.Skip("skipping: no new refresh cookie from Stage 5")
		}
		// Replaying the OLD refresh token must return 401 and kill every session.
		r := s.postJSON(t, "/api/v1/auth/refresh", nil, []*http.Cookie{refreshCookie}, nil)
		if r.StatusCode != http.StatusUnauthorized {
			t.Errorf("replay: want 401, got %d", r.StatusCode)
		}
		r.Body.Close()

		// After replay detection, even the new refresh token must be dead.
		r2 := s.postJSON(t, "/api/v1/auth/refresh", nil, []*http.Cookie{newRefreshCookie}, nil)
		if r2.StatusCode != http.StatusUnauthorized {
			t.Errorf("replay: new refresh after replay: want 401, got %d", r2.StatusCode)
		}
		r2.Body.Close()
	})

	// ── Stage 7 + 8: fresh login, then CSRF enforcement and logout ────────────
	// Stage 6 advanced the epoch (replay detection), which kills every session
	// for this user, including the one just rotated in stage 5 — so these
	// final stages need a fresh login, exactly as the original design intended.
	var freshAccess *http.Cookie
	var freshCSRF string

	t.Run("fresh_login_for_final_stages", func(t *testing.T) {
		r := s.postJSON(t, "/api/v1/auth/login", map[string]string{
			"email": email, "password": password,
		}, nil, nil)
		if r.StatusCode != http.StatusOK {
			t.Fatalf("fresh_login: want 200, got %d", r.StatusCode)
		}
		freshAccess = findCookie(r.Cookies(), authmw.AccessCookieName)
		if freshAccess == nil {
			t.Fatal("fresh_login: missing gs_access cookie")
		}
		var body struct {
			Data struct {
				CSRFToken string `json:"csrf_token"`
			} `json:"data"`
		}
		readJSON(t, r, &body)
		freshCSRF = body.Data.CSRFToken
		if freshCSRF == "" {
			t.Fatal("fresh_login: csrf_token missing from response body")
		}
	})

	if freshAccess == nil {
		t.Skip("skipping stages 7-8: fresh login failed")
	}

	t.Run("csrf_enforcement_on_unsafe_request", func(t *testing.T) {
		// logout without X-CSRF-Token must be rejected — the session must stay
		// alive so stage 8 can still log it out correctly.
		r := s.postJSON(t, "/api/v1/auth/logout", nil, []*http.Cookie{freshAccess}, nil)
		r.Body.Close()
		if r.StatusCode != http.StatusForbidden {
			t.Errorf("logout without CSRF: want 403, got %d", r.StatusCode)
		}

		// Confirm the session is still alive: /users/me must still succeed.
		r2 := s.getJSON(t, "/api/v1/users/me", []*http.Cookie{freshAccess})
		r2.Body.Close()
		if r2.StatusCode != http.StatusOK {
			t.Errorf("users/me after rejected logout: want 200 (session must survive a 403), got %d", r2.StatusCode)
		}
	})

	t.Run("logout", func(t *testing.T) {
		r := s.postJSON(t, "/api/v1/auth/logout", nil, []*http.Cookie{freshAccess},
			map[string]string{"X-CSRF-Token": freshCSRF})
		r.Body.Close()
		if r.StatusCode != http.StatusNoContent {
			t.Fatalf("logout: want 204, got %d", r.StatusCode)
		}

		// The captured access cookie must be rejected on the next call,
		// without waiting for its natural expiry.
		r2 := s.getJSON(t, "/api/v1/users/me", []*http.Cookie{freshAccess})
		r2.Body.Close()
		if r2.StatusCode != http.StatusUnauthorized {
			t.Errorf("users/me after logout: want 401, got %d", r2.StatusCode)
		}
	})
}

// ─── Route registration smoke tests ──────────────────────────────────────────
//
// These tests use the in-memory stack (newTestStack) rather than a live DB.
// They prove that every route from contract §1 is registered and returns a
// non-404 response — a 401/403/400 proves the route exists and the right
// middleware group applied (even without a valid token or body).

// TestRouteRegistration_PublicEndpointsAreReachable asserts that the public
// auth endpoints respond with a non-404 status, confirming they are registered
// on the V1 route group.
func TestRouteRegistration_PublicEndpointsAreReachable(t *testing.T) {
	ts := newTestStack(t)

	// POST with empty body → expect 400 VALIDATION_FAILED (not 404).
	publicPOST := []string{
		"/api/v1/auth/register",
		"/api/v1/auth/verify-email",
		"/api/v1/auth/password-reset/request",
		"/api/v1/auth/password-reset/confirm",
		"/api/v1/auth/oauth/link/confirm",
		"/api/v1/auth/login",
		"/api/v1/auth/refresh",
	}
	for _, path := range publicPOST {
		t.Run("POST "+path, func(t *testing.T) {
			r := ts.post(t, path, map[string]string{})
			r.Body.Close()
			if r.StatusCode == http.StatusNotFound {
				t.Errorf("route %s: got 404 — route not registered", path)
			}
		})
	}
}

// TestRouteRegistration_AuthenticatedEndpointsReturn401WithoutToken asserts
// that the session endpoints registered on srv.Protected return 401 (not 404)
// when called without a valid token — the gateway's auth middleware rejects
// the request before the handler runs (AMD-005: these routes are correctly
// behind auth middleware, not merely reachable).
func TestRouteRegistration_AuthenticatedEndpointsReturn401WithoutToken(t *testing.T) {
	ts := newTestStack(t)

	postRoutes := []string{
		"/api/v1/auth/logout",
		"/api/v1/auth/logout-all",
	}
	for _, path := range postRoutes {
		t.Run("POST "+path, func(t *testing.T) {
			r := ts.post(t, path, map[string]string{})
			r.Body.Close()
			if r.StatusCode == http.StatusNotFound {
				t.Errorf("route %s: got 404 — route not registered", path)
			}
			if r.StatusCode != http.StatusUnauthorized {
				t.Errorf("route %s: got %d, want 401 (auth middleware must reject a missing token)", path, r.StatusCode)
			}
		})
	}

	// GET /users/me — expects 401 without token.
	t.Run("GET /api/v1/users/me", func(t *testing.T) {
		r := ts.do(t, http.MethodGet, "/api/v1/users/me", nil, nil, nil)
		r.Body.Close()
		if r.StatusCode == http.StatusNotFound {
			t.Error("route /api/v1/users/me: got 404 — route not registered")
		}
		if r.StatusCode != http.StatusUnauthorized {
			t.Errorf("route /api/v1/users/me: got %d, want 401 (auth middleware must reject a missing token)", r.StatusCode)
		}
	})
}
