package session_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
)

// ─── logout ends only the current session ────────────────────────────────────
//
// AUTH-003 scenario 3 / TC-05

func TestLogout_LogoutEndsOnlyTheCurrentSession(t *testing.T) {
	d := newTestDeps(t)
	acctA := newTestAccount("deviceA@example.com", "pass")
	d.repo.add(acctA)

	// ── Device A: set up session ──────────────────────────────────────────────
	_, claimsA := d.mintAccess(t, acctA)
	csrfA := generateCSRF()
	_, refreshJTIA := d.mintRefresh(t, acctA.ID.String(), claimsA.SID)
	d.storeSession(t, acctA.ID.String(), claimsA.SID, refreshJTIA, csrfA)

	// ── Device B: set up a second session for the same user ───────────────────
	_, claimsB := d.mintAccess(t, acctA)
	csrfB := generateCSRF()
	_, refreshJTIB := d.mintRefresh(t, acctA.ID.String(), claimsB.SID)
	d.storeSession(t, acctA.ID.String(), claimsB.SID, refreshJTIB, csrfB)

	// ── Logout device A ───────────────────────────────────────────────────────
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("X-CSRF-Token", csrfA)
	w := httptest.NewRecorder()
	d.routerWithClaims(claimsA).ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("logout: want 204, got %d: %s", w.Code, w.Body)
	}

	// Device A's access JTI must be blacklisted.
	blacklistedAccess, err := authmw.IsBlacklisted(context.Background(), d.rdb, claimsA.JTI)
	if err != nil {
		t.Fatalf("IsBlacklisted access A: %v", err)
	}
	if !blacklistedAccess {
		t.Error("device A access JTI must be blacklisted")
	}

	// Device A's refresh JTI must be blacklisted.
	blacklistedRefresh, err := authmw.IsBlacklisted(context.Background(), d.rdb, refreshJTIA)
	if err != nil {
		t.Fatalf("IsBlacklisted refresh A: %v", err)
	}
	if !blacklistedRefresh {
		t.Error("device A refresh JTI must be blacklisted")
	}

	// Device A's session record must be gone.
	sessA, _ := authmw.GetSession(context.Background(), d.rdb, acctA.ID.String(), claimsA.SID)
	if sessA != nil {
		t.Error("device A session record must be deleted")
	}

	// ── Device B must be untouched ────────────────────────────────────────────
	blacklistedB, err := authmw.IsBlacklisted(context.Background(), d.rdb, refreshJTIB)
	if err != nil {
		t.Fatalf("IsBlacklisted refresh B: %v", err)
	}
	if blacklistedB {
		t.Error("device B refresh JTI must NOT be blacklisted")
	}

	sessB, err := authmw.GetSession(context.Background(), d.rdb, acctA.ID.String(), claimsB.SID)
	if err != nil {
		t.Fatalf("GetSession B: %v", err)
	}
	if sessB == nil {
		t.Error("device B session record must still exist")
	}

	// Cookies must be cleared on the logout response.
	var accessClearedMaxAge, refreshClearedMaxAge int
	for _, ck := range w.Result().Cookies() {
		switch ck.Name {
		case authmw.AccessCookieName:
			accessClearedMaxAge = ck.MaxAge
		case authmw.RefreshCookieName:
			refreshClearedMaxAge = ck.MaxAge
		}
	}
	if accessClearedMaxAge != -1 {
		t.Errorf("access cookie MaxAge: want -1 (clear), got %d", accessClearedMaxAge)
	}
	if refreshClearedMaxAge != -1 {
		t.Errorf("refresh cookie MaxAge: want -1 (clear), got %d", refreshClearedMaxAge)
	}

	// Audit event.
	evts := d.audit.EventsOf(auditmodel.EvtAuthLogout)
	if len(evts) == 0 {
		t.Fatal("no auth.logout audit event")
	}
	if evts[0].Fields["session_jti"] != claimsA.JTI {
		t.Errorf("audit session_jti: want %q, got %v", claimsA.JTI, evts[0].Fields["session_jti"])
	}
}

// ─── logout-all ends every session without enumerating tokens ────────────────
//
// AUTH-003 scenario 4 / TC-06

func TestLogoutAll_LogoutAllEndsEverySessionWithoutEnumeratingTokens(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("alldevices@example.com", "pass")
	d.repo.add(acct)

	// Set up two sessions.
	_, claims := d.mintAccess(t, acct)
	csrf := generateCSRF()
	_, refreshJTI := d.mintRefresh(t, acct.ID.String(), claims.SID)
	d.storeSession(t, acct.ID.String(), claims.SID, refreshJTI, csrf)

	_, claims2 := d.mintAccess(t, acct)
	csrf2 := generateCSRF()
	_, refreshJTI2 := d.mintRefresh(t, acct.ID.String(), claims2.SID)
	d.storeSession(t, acct.ID.String(), claims2.SID, refreshJTI2, csrf2)

	// Count the blacklist-writes before logout-all (should be zero from setup).
	// We verify post-condition: zero blacklist writes, one epoch write.
	// Approach: note that BlacklistJTI writes are caught by checking that
	// refreshJTI1 and refreshJTI2 are NOT in the blacklist after logout-all
	// (the epoch mechanism handles revocation without per-token blacklist ops).

	req := httptest.NewRequest(http.MethodPost, "/auth/logout-all", nil)
	req.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	d.routerWithClaims(claims).ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("logout-all: want 204, got %d: %s", w.Code, w.Body)
	}

	// Epoch must be set (the O(1) revocation mechanism).
	epoch, err := authmw.GetEpoch(context.Background(), d.rdb, acct.ID.String())
	if err != nil {
		t.Fatalf("GetEpoch: %v", err)
	}
	if epoch <= 0 {
		t.Error("epoch must be set after logout-all")
	}

	// Zero per-token blacklist writes: neither JTI should be in the blacklist.
	// (logout-all uses epoch, not per-token blacklisting — NFR-05)
	bl1, _ := authmw.IsBlacklisted(context.Background(), d.rdb, refreshJTI)
	if bl1 {
		t.Error("logout-all must NOT write per-token blacklist entries (use epoch instead)")
	}
	bl2, _ := authmw.IsBlacklisted(context.Background(), d.rdb, refreshJTI2)
	if bl2 {
		t.Error("logout-all must NOT write per-token blacklist entries (use epoch instead)")
	}

	// Cookies must be cleared.
	for _, ck := range w.Result().Cookies() {
		if ck.MaxAge != -1 {
			t.Errorf("cookie %q MaxAge: want -1, got %d", ck.Name, ck.MaxAge)
		}
	}

	// Audit event (emitted by RevokeAll).
	evts := d.audit.EventsOf(auditmodel.EvtAuthLogoutAll)
	if len(evts) == 0 {
		t.Fatal("no auth.logout_all audit event")
	}
}

// ─── logout: 403 on missing CSRF token ───────────────────────────────────────

func TestLogout_Returns403OnMissingCSRFToken(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("user@example.com", "pass")
	d.repo.add(acct)

	_, claims := d.mintAccess(t, acct)
	csrf := generateCSRF()
	_, refreshJTI := d.mintRefresh(t, acct.ID.String(), claims.SID)
	d.storeSession(t, acct.ID.String(), claims.SID, refreshJTI, csrf)

	// No X-CSRF-Token header.
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	w := httptest.NewRecorder()
	d.routerWithClaims(claims).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", w.Code, w.Body)
	}
	assertErrorCode(t, w.Body.Bytes(), auditmodel.ErrAuthCsrfFailed)
}

// ─── GET /users/me returns profile + csrf_token ───────────────────────────────

func TestMe_ReturnsProfileAndCSRFToken(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("me@example.com", "pass")
	d.repo.add(acct)

	_, claims := d.mintAccess(t, acct)
	csrf := generateCSRF()
	_, refreshJTI := d.mintRefresh(t, acct.ID.String(), claims.SID)
	d.storeSession(t, acct.ID.String(), claims.SID, refreshJTI, csrf)

	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	w := httptest.NewRecorder()
	d.routerWithClaims(claims).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}

	var resp struct {
		Data struct {
			User struct {
				ID            string `json:"id"`
				Email         string `json:"email"`
				Role          string `json:"role"`
				Tier          string `json:"tier"`
				EmailVerified bool   `json:"email_verified"`
			} `json:"user"`
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v (body: %s)", err, w.Body)
	}

	if resp.Data.User.ID != acct.ID.String() {
		t.Errorf("id: want %q, got %q", acct.ID.String(), resp.Data.User.ID)
	}
	if resp.Data.User.Email != acct.Email {
		t.Errorf("email: want %q, got %q", acct.Email, resp.Data.User.Email)
	}
	if resp.Data.User.Role != string(acct.Role) {
		t.Errorf("role: want %q, got %q", acct.Role, resp.Data.User.Role)
	}
	if resp.Data.User.Tier != "free" {
		t.Errorf("tier: want 'free', got %q", resp.Data.User.Tier)
	}
	if resp.Data.User.EmailVerified != acct.IsVerified() {
		t.Errorf("email_verified: want %v, got %v", acct.IsVerified(), resp.Data.User.EmailVerified)
	}
	if resp.Data.CSRFToken != csrf {
		t.Errorf("csrf_token: want %q, got %q", csrf, resp.Data.CSRFToken)
	}
}

// ─── /users/me: AMD-003 — email_verified from account row, not token ─────────

func TestMe_EmailVerifiedComesFromAccountRowNotTokenClaim(t *testing.T) {
	d := newTestDeps(t)

	// Account starts unverified.
	acct := newTestAccount("amd003@example.com", "pass")
	d.repo.add(acct)

	_, claims := d.mintAccess(t, acct)
	csrf := generateCSRF()
	_, refreshJTI := d.mintRefresh(t, acct.ID.String(), claims.SID)
	d.storeSession(t, acct.ID.String(), claims.SID, refreshJTI, csrf)

	// Check 1: unverified account returns email_verified:false
	req1 := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	w1 := httptest.NewRecorder()
	d.routerWithClaims(claims).ServeHTTP(w1, req1)

	var r1 meTestResp
	mustUnmarshalMeResp(t, w1.Body.Bytes(), &r1)
	if r1.Data.User.EmailVerified {
		t.Error("check1: email_verified must be false for unverified account")
	}

	// Now mark the account as verified in the repo (simulating an out-of-band
	// email verification — the access token's email_verified claim is still false
	// because the user hasn't refreshed).
	now := time.Now()
	acct.EmailVerifiedAt = &now
	d.repo.add(acct) // overwrite in fake repo

	// The claims in context still say email_verified:false (stale token).
	// /users/me must read from the account row, so it must return true.
	req2 := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	w2 := httptest.NewRecorder()
	// Still inject the original (stale) claims.
	d.routerWithClaims(claims).ServeHTTP(w2, req2)

	var r2 meTestResp
	mustUnmarshalMeResp(t, w2.Body.Bytes(), &r2)
	if !r2.Data.User.EmailVerified {
		t.Error("check2: email_verified must be true after account is verified, even with stale token claim")
	}
}

type meTestResp struct {
	Data struct {
		User struct {
			EmailVerified bool `json:"email_verified"`
		} `json:"user"`
	} `json:"data"`
}

func mustUnmarshalMeResp(t *testing.T, body []byte, out interface{}) {
	t.Helper()
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("unmarshal meResp: %v (body: %s)", err, body)
	}
}
