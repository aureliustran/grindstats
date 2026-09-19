//go:build integration

// Package integration contains end-to-end and cross-handler integration tests
// for the auth domain. Tests in this package are gated on the `integration`
// build tag so `go test ./...` stays fast by default.
//
// Run with:
//
//	GRINDSTATS_TEST_DB=... go test -tags=integration ./services/monolith/internal/auth/integration/...
package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/services/monolith/internal/auth/authdomain"
	"grindstats/services/monolith/internal/auth/credentials"
	"grindstats/services/monolith/internal/auth/hibp"
	"grindstats/services/monolith/internal/auth/mailer"
	"grindstats/services/monolith/internal/auth/session"
	"grindstats/services/monolith/internal/gateway"
)

// ─── Fakes ────────────────────────────────────────────────────────────────────

// fakeAccountRepo is a thread-safe in-memory AccountRepo for integration tests.
type fakeAccountRepo struct {
	mu      sync.Mutex
	byEmail map[string]*authdomain.Account
	byID    map[uuid.UUID]*authdomain.Account
}

func newFakeAccountRepo() *fakeAccountRepo {
	return &fakeAccountRepo{
		byEmail: make(map[string]*authdomain.Account),
		byID:    make(map[uuid.UUID]*authdomain.Account),
	}
}

func (r *fakeAccountRepo) seed(a authdomain.Account) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := a
	r.byEmail[a.EmailLower] = &cp
	r.byID[a.ID] = &cp
}

func (r *fakeAccountRepo) ByEmail(_ context.Context, emailLower string) (*authdomain.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.byEmail[emailLower]
	if a == nil {
		return nil, nil
	}
	cp := *a
	return &cp, nil
}

func (r *fakeAccountRepo) ByID(_ context.Context, id uuid.UUID) (*authdomain.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.byID[id]
	if a == nil {
		return nil, authdomain.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (r *fakeAccountRepo) Create(_ context.Context, a authdomain.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	lower := strings.ToLower(a.Email)
	if _, exists := r.byEmail[lower]; exists {
		return authdomain.ErrEmailTaken
	}
	cp := a
	cp.EmailLower = lower
	r.byEmail[lower] = &cp
	r.byID[a.ID] = &cp
	return nil
}

func (r *fakeAccountRepo) SetPasswordHash(_ context.Context, id uuid.UUID, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.byID[id]
	if a == nil {
		return authdomain.ErrNotFound
	}
	a.PasswordHash = hash
	r.byEmail[a.EmailLower].PasswordHash = hash
	return nil
}

func (r *fakeAccountRepo) MarkEmailVerified(_ context.Context, id uuid.UUID, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.byID[id]
	if a == nil {
		return authdomain.ErrNotFound
	}
	a.EmailVerifiedAt = &at
	r.byEmail[a.EmailLower].EmailVerifiedAt = &at
	return nil
}

// fakeOAuthIdentityRepo is a no-op stub for tests that don't exercise OAuth.
type fakeOAuthIdentityRepo struct{}

func (r *fakeOAuthIdentityRepo) BySubject(_ context.Context, _ auditmodel.LinkedProvider, _ string) (*authdomain.Account, error) {
	return nil, authdomain.ErrNotFound
}
func (r *fakeOAuthIdentityRepo) Link(_ context.Context, _ uuid.UUID, _ auditmodel.LinkedProvider, _, _ string) error {
	return nil
}

// fakeLinkTokenRepo issues tokens that immediately expire (irrelevant for
// enumeration tests which don't consume tokens).
type fakeLinkTokenRepo struct {
	mu     sync.Mutex
	tokens map[string]*authdomain.LinkToken
}

func newFakeLinkTokenRepo() *fakeLinkTokenRepo {
	return &fakeLinkTokenRepo{tokens: make(map[string]*authdomain.LinkToken)}
}

func (r *fakeLinkTokenRepo) Issue(_ context.Context, userID uuid.UUID, kind auditmodel.LinkKind, _ time.Duration, payload any) (string, error) {
	raw := uuid.NewString()
	var rawPayload json.RawMessage
	if payload != nil {
		b, _ := json.Marshal(payload)
		rawPayload = b
	}
	r.mu.Lock()
	r.tokens[raw] = &authdomain.LinkToken{
		ID:        uuid.New(),
		UserID:    userID,
		Kind:      kind,
		Payload:   rawPayload,
		CreatedAt: time.Now(),
	}
	r.mu.Unlock()
	return raw, nil
}

func (r *fakeLinkTokenRepo) Consume(_ context.Context, _ auditmodel.LinkKind, raw string) (*authdomain.LinkToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lt, ok := r.tokens[raw]
	if !ok {
		return nil, &authdomain.LinkInvalidError{Reason: auditmodel.LinkRejectReasonUnknown}
	}
	delete(r.tokens, raw)
	return lt, nil
}

// noopMailer silently drops every send.
type noopMailer struct{}

func (*noopMailer) SendVerification(_ context.Context, _, _ string) error  { return nil }
func (*noopMailer) SendPasswordReset(_ context.Context, _, _ string) error { return nil }
func (*noopMailer) SendOAuthLink(_ context.Context, _, _ string) error     { return nil }

// ─── Test stack helpers ───────────────────────────────────────────────────────

// testStack holds all the wired-up components for an integration test.
type testStack struct {
	engine      *httptest.Server
	accountRepo *fakeAccountRepo
	linkTokens  *fakeLinkTokenRepo
}

// newTestStack builds the full gateway+handler stack with in-memory fakes.
// It mirrors the wiring in main.go (wave 4) so integration tests exercise
// the same composition.
func newTestStack(t *testing.T) *testStack {
	t.Helper()

	// Generate an in-memory RSA key pair (2048-bit, acceptable for tests).
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("newTestStack: generate RSA key: %v", err)
	}
	ks := authmw.NewKeySetFromMemory(
		"test-kid",
		priv,
		map[string]*rsa.PublicKey{"test-kid": &priv.PublicKey},
	)

	// Miniredis for session storage and rate-limiting.
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)) // discard
	auditFake := auditlog.NewFake()

	accountRepo := newFakeAccountRepo()
	oauthRepo := &fakeOAuthIdentityRepo{}
	linkRepo := newFakeLinkTokenRepo()

	hasher, err := credentials.NewPasswordHasher(credentials.FastArgon2Params)
	if err != nil {
		t.Fatalf("newTestStack: init hasher: %v", err)
	}

	cookieOpts := authmw.CookieOptions{Secure: false} // plain http in tests
	sessionHandler := session.New(ks, rdb, accountRepo, auditFake, cookieOpts, hasher, logger)

	hibpClient := hibp.New(false, 2*time.Second) // disabled — no live HIBP in tests
	m := mailer.NewDev(logger, "http://localhost:5173")

	credHandler := credentials.New(
		accountRepo, oauthRepo, linkRepo,
		sessionHandler, hasher, hibpClient, m, auditFake,
	)

	srv := gateway.New(gateway.Deps{
		Logger:   logger,
		KeySet:   ks,
		Redis:    rdb,
		AuditLog: auditFake,
	})

	// Wire routes (same grouping as main.go — AMD-005).
	credHandler.Register(srv.V1)
	sessionHandler.RegisterPublic(srv.V1)
	sessionHandler.RegisterProtected(srv.Protected)

	ts := httptest.NewServer(srv.Engine)
	t.Cleanup(ts.Close)

	return &testStack{
		engine:      ts,
		accountRepo: accountRepo,
		linkTokens:  linkRepo,
	}
}

// do sends an HTTP request to the test server and returns the response.
func (ts *testStack) do(t *testing.T, method, path string, body any, cookies []*http.Cookie, headers map[string]string) *http.Response {
	t.Helper()
	var bodyReader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("do: marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, ts.engine.URL+path, bodyReader)
	if err != nil {
		t.Fatalf("do: build request: %v", err)
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
	resp, err := ts.engine.Client().Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	return resp
}

// post sends a JSON POST request to the test server and returns the response.
func (ts *testStack) post(t *testing.T, path string, body any) *http.Response {
	return ts.do(t, http.MethodPost, path, body, nil, nil)
}

// readBody reads the full response body as a string.
func readBody(t *testing.T, r *http.Response) string {
	t.Helper()
	defer r.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r.Body); err != nil {
		t.Fatalf("readBody: %v", err)
	}
	return strings.TrimSpace(buf.String())
}

// ─── TC-16: cross-endpoint enumeration regression ─────────────────────────────

// TestEnumeration_RegisterTakenAndNewAddressAreByteIdentical asserts that
// POST /auth/register with a taken email address returns a status code and
// body that are byte-identical to the response for a fresh (unknown) email
// address (FR-08, TC-14 at the integration level with the full middleware
// chain applied).
//
// This test does NOT require a live Postgres instance — it uses the in-memory
// fake store. It is gated on the `integration` build tag so it runs only in
// CI or when the developer explicitly invokes the integration suite.
func TestEnumeration_RegisterTakenAndNewAddressAreByteIdentical(t *testing.T) {
	ts := newTestStack(t)

	const existingEmail = "taken@example.com"
	const newEmail = "fresh@example.com"
	const password = "SecurePassword123!"

	// Pre-seed an existing account so the "taken email" branch fires.
	ts.accountRepo.seed(authdomain.Account{
		ID:         uuid.New(),
		Email:      existingEmail,
		EmailLower: strings.ToLower(existingEmail),
		Role:       auditmodel.RoleUser,
		Status:     auditmodel.AccountStatusActive,
		CreatedAt:  time.Now(),
	})

	// Register with a NEW email.
	r1 := ts.post(t, "/api/v1/auth/register", map[string]string{
		"email":    newEmail,
		"password": password,
	})
	body1 := readBody(t, r1)

	// Register with the TAKEN email.
	r2 := ts.post(t, "/api/v1/auth/register", map[string]string{
		"email":    existingEmail,
		"password": password,
	})
	body2 := readBody(t, r2)

	// Enumeration protection: status and body must be byte-identical.
	if r1.StatusCode != r2.StatusCode {
		t.Errorf("register: status codes differ: new=%d taken=%d", r1.StatusCode, r2.StatusCode)
	}
	if body1 != body2 {
		t.Errorf("register: response bodies differ:\nnew   = %s\ntaken = %s", body1, body2)
	}
}

// TestEnumeration_LoginUnknownEmailAndWrongPasswordAreByteIdentical asserts
// that POST /auth/login with a wrong password on a KNOWN email returns a
// status code and body that are byte-identical to the response for an UNKNOWN
// email (FR-08, TC-16 at the integration level).
//
// Byte equality is the correct assertion here: a distinguishable response is
// an account-enumeration oracle (audit-and-errors.md §4).
func TestEnumeration_LoginUnknownEmailAndWrongPasswordAreByteIdentical(t *testing.T) {
	ts := newTestStack(t)

	// Seed an account with a known, hashed password so the login handler can
	// verify credentials on the known-email path.
	hasher, err := credentials.NewPasswordHasher(credentials.FastArgon2Params)
	if err != nil {
		t.Fatalf("init hasher: %v", err)
	}
	hash, err := hasher.Hash("CorrectPassword123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	const knownEmail = "alice@example.com"
	ts.accountRepo.seed(authdomain.Account{
		ID:           uuid.New(),
		Email:        knownEmail,
		EmailLower:   strings.ToLower(knownEmail),
		PasswordHash: hash,
		Role:         auditmodel.RoleUser,
		Status:       auditmodel.AccountStatusActive,
		CreatedAt:    time.Now(),
	})

	// Login: KNOWN email, WRONG password.
	r1 := ts.post(t, "/api/v1/auth/login", map[string]string{
		"email":    knownEmail,
		"password": "WrongPassword456!",
	})
	body1 := readBody(t, r1)

	// Login: UNKNOWN email, any password.
	r2 := ts.post(t, "/api/v1/auth/login", map[string]string{
		"email":    "nobody@example.com",
		"password": "WrongPassword456!",
	})
	body2 := readBody(t, r2)

	// Enumeration protection: status and body must be byte-identical.
	if r1.StatusCode != r2.StatusCode {
		t.Errorf("login: status codes differ: known-wrong=%d unknown=%d", r1.StatusCode, r2.StatusCode)
	}
	if body1 != body2 {
		t.Errorf("login: response bodies differ:\nknown+wrong = %s\nunknown     = %s", body1, body2)
	}
}
