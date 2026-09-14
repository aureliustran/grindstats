// Google OAuth login for an existing account issues the same session shape
// AUTH-002 scenario 3 / TC-03
package session_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/services/monolith/internal/auth/authdomain"
)

// TestGoogleOAuthLoginForAnExistingAccountIssuesTheSameSessionShape verifies
// that SessionIssuer.Issue produces the same cookie attributes, CSRF structure,
// and session record shape regardless of the caller (local login or OAuth
// callback).  This is "one issuer → one shape" (contract §5, TC-03).
func TestGoogleOAuthLoginForAnExistingAccountIssuesTheSameSessionShape(t *testing.T) {
	d := newTestDeps(t)
	now := time.Now()
	verifiedAt := now
	acct := authdomain.Account{
		ID:              uuid.New(),
		Email:           "google_user@example.com",
		EmailLower:      "google_user@example.com",
		PasswordHash:    "", // OAuth-only: no local password
		Role:            auditmodel.RoleUser,
		Status:          auditmodel.AccountStatusActive,
		EmailVerifiedAt: &verifiedAt,
	}

	w := httptest.NewRecorder()
	csrfToken, err := d.handler.Issue(
		context.Background(),
		w,
		acct,
		"Mozilla/5.0 (Macintosh; Intel Mac OS X) AppleWebKit/537.36",
		"1.2.3.4",
	)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if csrfToken == "" {
		t.Fatal("csrfToken must not be empty")
	}

	// Both cookies set with correct attributes (same shape as the login path).
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
		t.Fatal("gs_access cookie not set by Issue")
	}
	if refreshCookie == nil {
		t.Fatal("gs_refresh cookie not set by Issue")
	}
	assertCookieAttrs(t, "gs_access", accessCookie, "/", true, true)
	assertCookieAttrs(t, "gs_refresh", refreshCookie, "/api/v1/auth", true, true)

	// Session record exists in Redis with the expected shape.
	sessions, err := authmw.AllSessions(context.Background(), d.rdb, acct.ID.String())
	if err != nil {
		t.Fatalf("AllSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(sessions))
	}
	sess := sessions[0]
	if sess.SID == "" {
		t.Error("session SID must not be empty")
	}
	if sess.RefreshJTI == "" {
		t.Error("session RefreshJTI must not be empty")
	}
	if sess.CSRFToken != csrfToken {
		t.Errorf("session CSRFToken %q != returned token %q", sess.CSRFToken, csrfToken)
	}
	if sess.CreatedAt.IsZero() || sess.LastSeenAt.IsZero() {
		t.Error("CreatedAt and LastSeenAt must be set")
	}

	// Audit event must be written.
	evts := d.audit.EventsOf(auditmodel.EvtAuthLoginSucceeded)
	if len(evts) == 0 {
		t.Fatal("no auth.login.succeeded audit event")
	}
	if evts[0].Fields["user_id"] != acct.ID.String() {
		t.Errorf("audit user_id: want %q, got %v", acct.ID.String(), evts[0].Fields["user_id"])
	}
}

// TestRevokeAll_AdvancesEpochAndEmitsAuditEvent verifies that RevokeAll
// writes the epoch and emits auth.logout_all.
func TestRevokeAll_AdvancesEpochAndEmitsAuditEvent(t *testing.T) {
	d := newTestDeps(t)
	userID := uuid.New()

	if err := d.handler.RevokeAll(context.Background(), userID); err != nil {
		t.Fatalf("RevokeAll: %v", err)
	}

	epoch, err := authmw.GetEpoch(context.Background(), d.rdb, userID.String())
	if err != nil {
		t.Fatalf("GetEpoch: %v", err)
	}
	if epoch <= 0 {
		t.Errorf("epoch must be > 0 after RevokeAll, got %d", epoch)
	}
	nowPlusOne := time.Now().Unix() + 1
	if epoch < nowPlusOne-1 || epoch > nowPlusOne+2 {
		t.Errorf("epoch %d not close to now+1 (%d)", epoch, nowPlusOne)
	}

	evts := d.audit.EventsOf(auditmodel.EvtAuthLogoutAll)
	if len(evts) == 0 {
		t.Fatal("no auth.logout_all audit event")
	}
}

// ─── cookie attribute helper (used across multiple test files) ────────────────

func assertCookieAttrs(t *testing.T, name string, ck *http.Cookie, path string, httpOnly, secure bool) {
	t.Helper()
	if ck.HttpOnly != httpOnly {
		t.Errorf("%s cookie: HttpOnly want %v, got %v", name, httpOnly, ck.HttpOnly)
	}
	if ck.Secure != secure {
		t.Errorf("%s cookie: Secure want %v, got %v", name, secure, ck.Secure)
	}
	if ck.Path != path {
		t.Errorf("%s cookie: Path want %q, got %q", name, path, ck.Path)
	}
	if ck.SameSite != http.SameSiteLaxMode {
		t.Errorf("%s cookie: SameSite want Lax, got %v", name, ck.SameSite)
	}
}
