package session_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
)

// ─── five consecutive failures trigger backoff ────────────────────────────────
//
// AUTH-002 scenario 6 / TC-06

func TestLogin_FiveConsecutiveFailuresTriggerBackoff(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("victim@example.com", "correctpass")
	d.repo.add(acct)

	fail := func() int {
		body := `{"email":"victim@example.com","password":"wrongpass"}`
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		d.router().ServeHTTP(w, req)
		return w.Code
	}

	// Failures 1-4 must return 401 (not yet in backoff).
	for i := 1; i <= 4; i++ {
		if code := fail(); code != http.StatusUnauthorized {
			t.Errorf("failure %d: want 401, got %d", i, code)
		}
	}

	// 5th failure sets backoff; the request should still see 401 for this one
	// (backoff affects the NEXT request, because the key is set AFTER verify).
	fail()

	// 6th request: now in backoff → 429.
	body := `{"email":"victim@example.com","password":"anypassword"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	d.router().ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("6th attempt: want 429, got %d: %s", w.Code, w.Body)
	}
	assertErrorCode(t, w.Body.Bytes(), auditmodel.ErrAuthRateLimited)

	retryAfter := w.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("Retry-After header must be set on 429")
	}
	seconds, err := strconv.Atoi(retryAfter)
	if err != nil || seconds <= 0 {
		t.Errorf("Retry-After: want positive integer seconds, got %q", retryAfter)
	}

	// Audit must record backoff_triggered.
	evts := d.audit.EventsOf(auditmodel.EvtAuthLoginBackoffTriggered)
	if len(evts) == 0 {
		t.Fatal("no auth.login.backoff_triggered audit event")
	}
}

// ─── backoff increases with continued failures ────────────────────────────────
//
// AUTH-002 scenario 7 / TC-07 — unit on the delay sequence (30 → 60 → 120, capped).
// The arithmetic lives in libs/authmw.BackoffDelay; this test validates that
// sequence so the regression is caught here if either the library changes or
// the handler chooses the wrong step.

func TestLogin_BackoffIncreasesWithContinuedFailures(t *testing.T) {
	cases := []struct {
		step    int
		wantSec int
	}{
		{1, 30},
		{2, 60},
		{3, 120},
		{4, 240},
		{5, 480},
		{6, 960},
		{7, 1920},
		{8, 3600}, // capped at 3600
		{9, 3600},
		{100, 3600},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("step%d", tc.step), func(t *testing.T) {
			got := authmw.BackoffDelay(tc.step)
			if int(got.Seconds()) != tc.wantSec {
				t.Errorf("step %d: want %ds, got %v", tc.step, tc.wantSec, got)
			}
		})
	}
}

// ─── a successful login resets the failure counter ───────────────────────────
//
// AUTH-002 scenario 8 / TC-08

func TestLogin_ASuccessfulLoginResetsTheFailureCounter(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("reset@example.com", "correctpass")
	d.repo.add(acct)

	// Two failures first.
	failLogin := func() {
		body := `{"email":"reset@example.com","password":"wrong"}`
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		d.router().ServeHTTP(w, req)
	}
	failLogin()
	failLogin()

	// Verify fail counter is non-zero.
	count, err := authmw.GetLoginFailUser(context.Background(), d.rdb, acct.ID.String())
	if err != nil {
		t.Fatalf("GetLoginFailUser: %v", err)
	}
	if count < 2 {
		t.Fatalf("want fail count ≥ 2, got %d", count)
	}

	// Successful login.
	body := `{"email":"reset@example.com","password":"correctpass"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	d.router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("successful login: want 200, got %d: %s", w.Code, w.Body)
	}

	// Fail counter must be cleared.
	countAfter, err := authmw.GetLoginFailUser(context.Background(), d.rdb, acct.ID.String())
	if err != nil {
		t.Fatalf("GetLoginFailUser after success: %v", err)
	}
	if countAfter != 0 {
		t.Errorf("fail counter must be 0 after successful login, got %d", countAfter)
	}
}

// ─── backoff blocks the account even from a new source IP ────────────────────
//
// AUTH-002 scenario 9 / TC-09

func TestLogin_BackoffBlocksTheAccountEvenFromANewSourceIP(t *testing.T) {
	d := newTestDeps(t)
	acct := newTestAccount("blocked@example.com", "realpass")
	d.repo.add(acct)

	// Trigger five failures from IP 1.2.3.4.
	for i := 0; i < 5; i++ {
		body := `{"email":"blocked@example.com","password":"wrong"}`
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", "1.2.3.4")
		w := httptest.NewRecorder()
		d.router().ServeHTTP(w, req)
	}

	// 6th attempt from a completely different IP must still be 429.
	body := `{"email":"blocked@example.com","password":"realpass"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "9.8.7.6") // different source IP
	w := httptest.NewRecorder()
	d.router().ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("new-IP attempt: want 429 (backoff is per-account), got %d: %s",
			w.Code, w.Body)
	}
	assertErrorCode(t, w.Body.Bytes(), auditmodel.ErrAuthRateLimited)
}
