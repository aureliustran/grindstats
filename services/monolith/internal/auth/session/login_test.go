package session_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/services/monolith/internal/auth/authdomain"
)

// ─── successful login issues cookie-only tokens and a CSRF token ─────────────
//
// AUTH-002 scenario 1 / TC-01, TC-11

func TestLogin_SuccessfulLoginIssuesCookieOnlyTokensAndACSRFToken(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("user@example.com", "password123")
	d.repo.add(acct)

	body := `{"email":"user@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	d.router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}

	// Body must contain csrf_token and nothing else from the session
	var resp struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Data.CSRFToken == "" {
		t.Fatal("csrf_token missing from response body")
	}

	// No token string anywhere in the body (contract §1.1)
	bodyStr := w.Body.String()
	if strings.Count(bodyStr, ".") > 3 {
		// JWT tokens have exactly 2 dots; a body with more than 3 dots is
		// suspicious — rough heuristic for an accidental token in the body.
		t.Logf("WARNING: body may contain a JWT (too many dots): %s", bodyStr)
	}

	// Access cookie: Path=/, HttpOnly, Secure, SameSite=Lax
	var accessCookie, refreshCookie *http.Cookie
	for _, ck := range w.Result().Cookies() {
		switch ck.Name {
		case authmw.AccessCookieName:
			accessCookie = ck
		case authmw.RefreshCookieName:
			refreshCookie = ck
		}
	}
	if accessCookie == nil {
		t.Fatal("gs_access cookie not set")
	}
	if !accessCookie.HttpOnly {
		t.Error("gs_access cookie: HttpOnly must be true")
	}
	if !accessCookie.Secure {
		t.Error("gs_access cookie: Secure must be true")
	}
	if accessCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("gs_access cookie: want SameSite=Lax, got %v", accessCookie.SameSite)
	}
	if accessCookie.Path != "/" {
		t.Errorf("gs_access cookie: want Path=/, got %q", accessCookie.Path)
	}

	// Refresh cookie: Path=/api/v1/auth, HttpOnly, Secure, SameSite=Lax
	if refreshCookie == nil {
		t.Fatal("gs_refresh cookie not set")
	}
	if !refreshCookie.HttpOnly {
		t.Error("gs_refresh cookie: HttpOnly must be true")
	}
	if !refreshCookie.Secure {
		t.Error("gs_refresh cookie: Secure must be true")
	}
	if refreshCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("gs_refresh cookie: want SameSite=Lax, got %v", refreshCookie.SameSite)
	}
	if refreshCookie.Path != "/api/v1/auth" {
		t.Errorf("gs_refresh cookie: want Path=/api/v1/auth, got %q", refreshCookie.Path)
	}

	// Token strings must not appear in the body
	for _, ck := range w.Result().Cookies() {
		if strings.Contains(bodyStr, ck.Value) {
			t.Errorf("token string for cookie %q leaked into response body", ck.Name)
		}
	}
}

// ─── login creates a session record with a device label ──────────────────────
//
// AUTH-002 scenario 2 / TC-02

func TestLogin_LoginCreatesASessionRecordWithADeviceLabel(t *testing.T) {
	tests := []struct {
		name          string
		userAgent     string
		wantLabel     string // "" means check non-empty
		exactMatch    bool
	}{
		{
			name:      "known UA is stored",
			userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120",
			wantLabel: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120",
			exactMatch: true,
		},
		{
			name:      "Unknown device fallback for an unparseable one",
			userAgent: "",
			wantLabel: "Unknown device",
			exactMatch: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newTestDeps(t)
			acct := newTestAccount("dev@example.com", "pass")
			d.repo.add(acct)

			body := `{"email":"dev@example.com","password":"pass"}`
			req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tc.userAgent != "" {
				req.Header.Set("User-Agent", tc.userAgent)
			}
			w := httptest.NewRecorder()
			d.router().ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
			}

			// Check session record was stored in Redis with the expected label.
			sessions, err := authmw.AllSessions(req.Context(), d.rdb, acct.ID.String())
			if err != nil {
				t.Fatalf("AllSessions: %v", err)
			}
			if len(sessions) == 0 {
				t.Fatal("no session record found in Redis")
			}
			if tc.exactMatch && sessions[0].DeviceLabel != tc.wantLabel {
				t.Errorf("DeviceLabel: want %q, got %q", tc.wantLabel, sessions[0].DeviceLabel)
			}
			if !tc.exactMatch && sessions[0].DeviceLabel == "" {
				t.Error("DeviceLabel should not be empty")
			}
		})
	}
}

// ─── wrong password gives a generic failure without revealing which factor failed ──
//
// AUTH-002 scenario 4 / TC-04

func TestLogin_WrongPasswordGivesAGenericFailureWithoutRevealingWhichFactorFailed(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("user@example.com", "correctpassword")
	d.repo.add(acct)

	body := `{"email":"user@example.com","password":"wrongpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	d.router().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), auditmodel.ErrAuthInvalidCredentials)

	// Internal reason must NOT appear in the response body.
	bodyStr := w.Body.String()
	if strings.Contains(bodyStr, "bad_password") || strings.Contains(bodyStr, "BadPassword") {
		t.Errorf("bad_password reason leaked to client: %s", bodyStr)
	}

	// Audit event must record the internal reason.
	evts := d.audit.EventsOf(auditmodel.EvtAuthLoginFailed)
	if len(evts) == 0 {
		t.Fatal("no auth.login.failed audit event")
	}
	if got := evts[0].Fields["reason"]; got != auditmodel.LoginFailureReasonBadPassword {
		t.Errorf("audit reason: want bad_password, got %v", got)
	}
}

// ─── nonexistent email gives an identical generic failure ─────────────────────
//
// AUTH-002 scenario 5 / TC-05 — byte-equal body and status to the above.

func TestLogin_NonexistentEmailGivesAnIdenticalGenericFailure(t *testing.T) {
	d := newTestDeps(t)
	// known-email wrong-password request
	acct := newTestAccount("user@example.com", "correctpassword")
	d.repo.add(acct)

	makeWrongPW := func() ([]byte, int) {
		body := `{"email":"user@example.com","password":"wrongpassword"}`
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		d.router().ServeHTTP(w, req)
		return w.Body.Bytes(), w.Code
	}

	makeUnknownEmail := func() ([]byte, int) {
		body := `{"email":"nobody@example.com","password":"somepassword"}`
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		d.router().ServeHTTP(w, req)
		return w.Body.Bytes(), w.Code
	}

	wrongPWBody, wrongPWStatus := makeWrongPW()
	unknownBody, unknownStatus := makeUnknownEmail()

	if wrongPWStatus != unknownStatus {
		t.Errorf("status mismatch: wrong-pw %d, unknown %d", wrongPWStatus, unknownStatus)
	}
	if !bytes.Equal(wrongPWBody, unknownBody) {
		t.Errorf("body mismatch (enumeration oracle!):\nwrong-pw: %s\nunknown:  %s",
			wrongPWBody, unknownBody)
	}

	// Internal reasons differ in audit but are NOT in the response.
	evts := d.audit.EventsOf(auditmodel.EvtAuthLoginFailed)
	if len(evts) < 2 {
		t.Fatalf("want ≥2 login.failed events, got %d", len(evts))
	}
}

// ─── an unverified account can still log in ───────────────────────────────────
//
// AUTH-002 scenario 10 / TC-10

func TestLogin_AnUnverifiedAccountCanStillLogIn(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("unverified@example.com", "pass123")
	// EmailVerifiedAt is nil → IsVerified() = false
	d.repo.add(acct)

	body := `{"email":"unverified@example.com","password":"pass123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	d.router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}

	// Access token must carry email_verified: false.
	var accessCookieVal string
	for _, ck := range w.Result().Cookies() {
		if ck.Name == authmw.AccessCookieName {
			accessCookieVal = ck.Value
		}
	}
	if accessCookieVal == "" {
		t.Fatal("no gs_access cookie")
	}

	// Use ks.Check (with nil rdb to skip Redis) to decode the access token.
	claims, reason, err := d.ks.Check(req.Context(), nil, accessCookieVal, authmw.TypAccess)
	if err != nil || reason != "" {
		t.Fatalf("Check: reason=%v err=%v", reason, err)
	}
	if claims.EmailVerified {
		t.Error("email_verified claim must be false for an unverified account")
	}
}

// ─── suspended account is rejected AFTER password verification ────────────────
//
// Not an explicit scenario row but covers the D2 assertion tested via TC-04/05.

func TestLogin_SuspendedAccountIsRejectedAfterPasswordVerification(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("suspended@example.com", "pass123")
	acct.Status = auditmodel.AccountStatusSuspended
	d.repo.add(acct)

	body := `{"email":"suspended@example.com","password":"pass123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	d.router().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", w.Code, w.Body)
	}
	assertErrorCode(t, w.Body.Bytes(), auditmodel.ErrAuthAccountSuspended)

	evts := d.audit.EventsOf(auditmodel.EvtAuthLoginSuspended)
	if len(evts) == 0 {
		t.Fatal("no auth.login.suspended audit event")
	}

	// Suspended account with WRONG password must return 401, not 403.
	d.audit.Reset()
	body2 := `{"email":"suspended@example.com","password":"wrongpassword"}`
	req2 := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	d.router().ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("wrong pw + suspended: want 401, got %d", w2.Code)
	}
	assertErrorCode(t, w2.Body.Bytes(), auditmodel.ErrAuthInvalidCredentials)
}

// ─── Google OAuth login for an existing account issues the same session shape ─
//
// AUTH-002 scenario 3 / TC-03  (unit on SessionIssuer.Issue)
// See issuer_test.go for this scenario.

// ─── helpers ─────────────────────────────────────────────────────────────────

func assertErrorCode(t *testing.T, body []byte, want auditmodel.ErrorCode) {
	t.Helper()
	var env struct {
		Error struct {
			Code auditmodel.ErrorCode `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("assertErrorCode: unmarshal: %v (body: %s)", err, body)
	}
	if env.Error.Code != want {
		t.Errorf("error code: want %q, got %q (body: %s)", want, env.Error.Code, body)
	}
}

func mustUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewRandom()
	if err != nil {
		t.Fatalf("uuid.NewRandom: %v", err)
	}
	return id
}

// localeBody sends the same POST and returns bodies for en-US and vi-VN.
// Used by locale-coverage assertions (backend.md §7).
func (d *testDeps) twoLocaleErrors(t *testing.T, path, body string) (enBody, viBody string) {
	t.Helper()
	do := func(lang string) string {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept-Language", lang)
		w := httptest.NewRecorder()
		d.router().ServeHTTP(w, req)
		return w.Body.String()
	}
	return do("en-US"), do("vi-VN")
}

// TestLogin_LocaleCoverage asserts that the error message is different under
// en-US and vi-VN (backend.md §7 locale requirement) while the error code is
// the same, and that the audit row was written in en-US.
func TestLogin_LocaleCoverage(t *testing.T) {
	d := newTestDeps(t)
	// Provide no account so both locales hit the same code path.
	enBody, viBody := d.twoLocaleErrors(t,
		"/auth/login",
		`{"email":"nobody@example.com","password":"pass"}`,
	)

	var enResp, viResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(enBody), &enResp); err != nil {
		t.Fatalf("en parse: %v", err)
	}
	if err := json.Unmarshal([]byte(viBody), &viResp); err != nil {
		t.Fatalf("vi parse: %v", err)
	}
	if enResp.Error.Code != viResp.Error.Code {
		t.Errorf("error codes differ: en=%q vi=%q", enResp.Error.Code, viResp.Error.Code)
	}
	if enResp.Error.Message == viResp.Error.Message {
		t.Errorf("messages are identical under en-US and vi-VN (want different): %q",
			enResp.Error.Message)
	}

	// Audit row message must be en-US (audit-and-errors.md §1a).
	evts := d.audit.EventsOf(auditmodel.EvtAuthLoginFailed)
	if len(evts) == 0 {
		t.Fatal("no login.failed audit event")
	}
	// The Fake stores the rendered message from the en-US template.
	if evts[0].Message == "" {
		t.Error("audit message must not be empty")
	}
	// Spot-check: the rendered message should not contain Vietnamese characters
	// (which would indicate the locale leaked into the audit path).
	for _, r := range evts[0].Message {
		if r > 0x024F { // beyond basic Latin and Latin Extended
			t.Errorf("audit message appears non-ASCII (locale leakage?): %q", evts[0].Message)
			break
		}
	}
}

// newTestAccountWithVerifiedAt returns an account with the given verified-at time.
func newTestAccountWithVerifiedAt(email, password string, verifiedAt *time.Time) authdomain.Account {
	a := newTestAccount(email, password)
	a.EmailVerifiedAt = verifiedAt
	return a
}
