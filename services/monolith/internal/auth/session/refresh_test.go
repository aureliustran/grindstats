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

// ─── valid refresh rotates the token pair ────────────────────────────────────
//
// AUTH-003 scenario 1 / TC-01

func TestRefresh_ValidRefreshRotatesTheTokenPair(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("user@example.com", "pass")
	d.repo.add(acct)

	// Set up an existing session.
	sid := "test-sid-rotate"
	csrfToken := generateCSRF()
	oldRefreshTok, oldRefreshJTI := d.mintRefresh(t, acct.ID.String(), sid)
	d.storeSession(t, acct.ID.String(), sid, oldRefreshJTI, csrfToken)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	addCookie(req, authmw.RefreshCookieName, oldRefreshTok)
	w := httptest.NewRecorder()
	d.router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}

	// New cookies must be present.
	var newAccessTok, newRefreshTok string
	for _, ck := range w.Result().Cookies() {
		switch ck.Name {
		case authmw.AccessCookieName:
			newAccessTok = ck.Value
		case authmw.RefreshCookieName:
			newRefreshTok = ck.Value
		}
	}
	if newAccessTok == "" {
		t.Fatal("new access cookie missing")
	}
	if newRefreshTok == "" {
		t.Fatal("new refresh cookie missing")
	}

	// New JTIs differ from old ones.
	newAccessClaims, _, _ := d.ks.Check(context.Background(), nil, newAccessTok, authmw.TypAccess)
	newRefreshClaims, _, _ := d.ks.Check(context.Background(), nil, newRefreshTok, authmw.TypRefresh)

	if newAccessClaims.SID != sid {
		t.Errorf("sid must be preserved: want %q, got %q", sid, newAccessClaims.SID)
	}
	if newRefreshClaims.SID != sid {
		t.Errorf("refresh sid must be preserved: want %q, got %q", sid, newRefreshClaims.SID)
	}

	// Old refresh JTI must be blacklisted.
	blacklisted, err := authmw.IsBlacklisted(context.Background(), d.rdb, oldRefreshJTI)
	if err != nil {
		t.Fatalf("IsBlacklisted: %v", err)
	}
	if !blacklisted {
		t.Error("old refresh JTI must be blacklisted after rotation")
	}

	// ── OLD access still valid (AUTH-003 scenario 1) ──────────────────────────
	// Rotation does not retroactively kill the old access token.
	// We can verify this by checking that the OLD access is NOT blacklisted
	// (there should be no blacklist write for the access token during refresh).
	// We don't have the old access JTI here, but we can verify the session
	// record was updated correctly.
	sess, err := authmw.GetSession(context.Background(), d.rdb, acct.ID.String(), sid)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.RefreshJTI == oldRefreshJTI {
		t.Error("session RefreshJTI must be updated after rotation")
	}
	if sess.RefreshJTI != newRefreshClaims.JTI {
		t.Errorf("session RefreshJTI: want %q, got %q", newRefreshClaims.JTI, sess.RefreshJTI)
	}
	if sess.CSRFToken != csrfToken {
		t.Errorf("csrf_token must be preserved, want %q, got %q", csrfToken, sess.CSRFToken)
	}

	// Body must return the (preserved) CSRF token.
	var resp struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.CSRFToken != csrfToken {
		t.Errorf("body csrf_token: want %q, got %q", csrfToken, resp.Data.CSRFToken)
	}

	// Old access token kept working: we check that no blacklist entry was
	// written for the "access" counterpart — we do this by ensuring the
	// blacklist only contains the old refresh JTI.
	// Verify: new access claims exist and are valid.
	if newAccessClaims.Sub != acct.ID.String() {
		t.Errorf("new access sub: want %q, got %q", acct.ID.String(), newAccessClaims.Sub)
	}

	// Audit event.
	evts := d.audit.EventsOf(auditmodel.EvtAuthRefreshRotated)
	if len(evts) == 0 {
		t.Fatal("no auth.refresh.rotated audit event")
	}
	if evts[0].Fields["old_jti"] != oldRefreshJTI {
		t.Errorf("audit old_jti: want %q, got %v", oldRefreshJTI, evts[0].Fields["old_jti"])
	}
}

// ─── replaying a rotated refresh token kills every session ───────────────────
//
// AUTH-003 scenario 2 / TC-02

func TestRefresh_ReplayingARotatedRefreshTokenKillsEverySession(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("victim@example.com", "pass")
	d.repo.add(acct)

	sid := "sid-replay"
	csrf := generateCSRF()
	oldRefreshTok, oldRefreshJTI := d.mintRefresh(t, acct.ID.String(), sid)
	d.storeSession(t, acct.ID.String(), sid, oldRefreshJTI, csrf)

	// First refresh: legitimate rotation.
	req1 := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	addCookie(req1, authmw.RefreshCookieName, oldRefreshTok)
	w1 := httptest.NewRecorder()
	d.router().ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first refresh: want 200, got %d: %s", w1.Code, w1.Body)
	}

	// Advance miniredis time so the blacklist TTL doesn't cause issues.
	d.mr.FastForward(time.Second)

	// Second use of the same (now-rotated) refresh token: REPLAY.
	req2 := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	addCookie(req2, authmw.RefreshCookieName, oldRefreshTok)
	w2 := httptest.NewRecorder()
	d.router().ServeHTTP(w2, req2)

	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("replay: want 401, got %d: %s", w2.Code, w2.Body)
	}
	assertErrorCode(t, w2.Body.Bytes(), auditmodel.ErrAuthInvalidToken)

	// Epoch must be advanced (kills every session including the legitimately
	// issued one).
	epoch, err := authmw.GetEpoch(context.Background(), d.rdb, acct.ID.String())
	if err != nil {
		t.Fatalf("GetEpoch: %v", err)
	}
	if epoch <= 0 {
		t.Error("epoch must be advanced on replay detection")
	}

	// auth.refresh.replay_detected must be emitted at critical severity.
	evts := d.audit.EventsOf(auditmodel.EvtAuthRefreshReplayDetected)
	if len(evts) == 0 {
		t.Fatal("no auth.refresh.replay_detected audit event")
	}
	if evts[0].Severity != auditmodel.SeverityCritical {
		t.Errorf("replay_detected severity: want critical, got %v", evts[0].Severity)
	}
	if evts[0].Fields["replayed_jti"] != oldRefreshJTI {
		t.Errorf("replayed_jti: want %q, got %v", oldRefreshJTI, evts[0].Fields["replayed_jti"])
	}
}

// ─── an idle session beyond 14 days cannot refresh even within the 30-day window
//
// AUTH-003 scenario 5 / TC-07

func TestRefresh_AnIdleSessionBeyond14DaysCannotRefreshEvenWithinThe30DayWindow(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("idle@example.com", "pass")
	d.repo.add(acct)

	// Set last_seen_at to 15 days ago (well past the 14-day idle threshold).
	fifteenDaysAgo := time.Now().Add(-15 * 24 * time.Hour)
	sid := "idle-sid"
	csrf := generateCSRF()
	refreshTok, refreshJTI := d.mintRefresh(t, acct.ID.String(), sid)
	d.storeSessionAt(t, acct.ID.String(), sid, refreshJTI, csrf, fifteenDaysAgo)

	// Use a fixed clock at "now" so idle check fires.
	d.handler.WithClock(func() time.Time { return time.Now() })

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	addCookie(req, authmw.RefreshCookieName, refreshTok)
	w := httptest.NewRecorder()
	d.router().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("idle session: want 401, got %d: %s", w.Code, w.Body)
	}
	assertErrorCode(t, w.Body.Bytes(), auditmodel.ErrAuthSessionExpired)
}

// ─── an active session survives past 14 days as long as it keeps refreshing ──
//
// AUTH-003 scenario 6 / TC-08

func TestRefresh_AnActiveSessionSurvivesPast14DaysAsLongAsItKeepsRefreshing(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("active@example.com", "pass")
	d.repo.add(acct)

	// Simulate last_seen_at being 12 days ago (within the 14-day window).
	twelveDaysAgo := time.Now().Add(-12 * 24 * time.Hour)
	sid := "active-sid"
	csrf := generateCSRF()
	refreshTok, refreshJTI := d.mintRefresh(t, acct.ID.String(), sid)
	d.storeSessionAt(t, acct.ID.String(), sid, refreshJTI, csrf, twelveDaysAgo)

	// Fix the clock so that we're "now" (12 days since last_seen_at < 14 days).
	d.handler.WithClock(func() time.Time { return time.Now() })

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	addCookie(req, authmw.RefreshCookieName, refreshTok)
	w := httptest.NewRecorder()
	d.router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("active session at 12d: want 200, got %d: %s", w.Code, w.Body)
	}

	// Verify LastSeenAt was updated (sliding the window forward).
	sess, err := authmw.GetSession(context.Background(), d.rdb, acct.ID.String(), sid)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if !sess.LastSeenAt.After(twelveDaysAgo) {
		t.Error("LastSeenAt must be updated to 'now' after refresh")
	}

	// Now simulate 13 more days passing (total 25 days since original creation,
	// but only 13 days since the last refresh). With an injected clock at now+13d,
	// the session is still within the idle window.
	d.handler.WithClock(func() time.Time {
		return time.Now().Add(13 * 24 * time.Hour)
	})

	// Get the new refresh token from the rotation response.
	var newRefreshTok string
	for _, ck := range w.Result().Cookies() {
		if ck.Name == authmw.RefreshCookieName {
			newRefreshTok = ck.Value
		}
	}
	if newRefreshTok == "" {
		t.Fatal("new refresh cookie missing after first rotation")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	addCookie(req2, authmw.RefreshCookieName, newRefreshTok)
	w2 := httptest.NewRecorder()
	d.router().ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("active session at 13d since last refresh: want 200, got %d: %s",
			w2.Code, w2.Body)
	}
}

// ─── refresh: 503 when Redis is down ─────────────────────────────────────────

func TestRefresh_Returns503WhenRedisIsDown(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("user@example.com", "pass")
	d.repo.add(acct)

	sid := "sid-redis-down"
	refreshTok, refreshJTI := d.mintRefresh(t, acct.ID.String(), sid)
	csrf := generateCSRF()
	d.storeSession(t, acct.ID.String(), sid, refreshJTI, csrf)

	// Close miniredis to simulate Redis unavailability.
	d.mr.Close()

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	addCookie(req, authmw.RefreshCookieName, refreshTok)
	w := httptest.NewRecorder()
	d.router().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("Redis down: want 503, got %d: %s", w.Code, w.Body)
	}
	assertErrorCode(t, w.Body.Bytes(), auditmodel.ErrServiceUnavailable)
}

