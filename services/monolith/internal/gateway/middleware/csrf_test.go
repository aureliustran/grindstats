package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/authmw"
	"grindstats/services/monolith/internal/gateway/middleware"
)

// storeCSRFSession writes a session record binding sid to csrfToken, so the
// CSRF middleware has something to compare the presented header against.
func storeCSRFSession(t *testing.T, rdb *redis.Client, userID, sid, csrfToken string) {
	t.Helper()
	now := time.Now()
	err := authmw.StoreSession(t.Context(), rdb, userID, authmw.Session{
		SID:         sid,
		RefreshJTI:  uuid.New().String(),
		DeviceLabel: "test",
		CreatedAt:   now,
		LastSeenAt:  now,
		CSRFToken:   csrfToken,
	})
	if err != nil {
		t.Fatalf("storeCSRFSession: %v", err)
	}
}

// newCSRFRouter assembles Auth + CSRF in the real gateway order, so
// ClaimsFromContext is populated by the time CSRF runs.
func newCSRFRouter(ks *authmw.KeySet, rdb *redis.Client) (*gin.Engine, *bool) {
	reached := false
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Auth(ks, rdb, nil, nil))
	r.Use(middleware.CSRF(rdb, nil))
	r.POST("/protected", func(c *gin.Context) { reached = true; c.Status(http.StatusOK) })
	r.GET("/protected", func(c *gin.Context) { reached = true; c.Status(http.StatusOK) })
	r.POST("/api/v1/auth/refresh", func(c *gin.Context) { reached = true; c.Status(http.StatusOK) })
	return r, &reached
}

func TestCSRF_MissingHeaderIsRejected(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	sub, sid := uuid.New().String(), uuid.New().String()
	tokenStr, _ := mintAccess(t, ks, sub, sid, true)
	storeCSRFSession(t, rdb, sub, sid, "the-real-token")

	r, reached := newCSRFRouter(ks, rdb)

	req := httptest.NewRequest(http.MethodPost, "/protected", nil)
	setCookie(req, authmw.AccessCookieName, tokenStr)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusForbidden)
	assertErrorCode(t, rec, "AUTH_CSRF_FAILED")
	if *reached {
		t.Error("downstream handler should not have been reached")
	}
}

func TestCSRF_WrongTokenIsRejected(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	sub, sid := uuid.New().String(), uuid.New().String()
	tokenStr, _ := mintAccess(t, ks, sub, sid, true)
	storeCSRFSession(t, rdb, sub, sid, "the-real-token")

	r, reached := newCSRFRouter(ks, rdb)

	req := httptest.NewRequest(http.MethodPost, "/protected", nil)
	setCookie(req, authmw.AccessCookieName, tokenStr)
	req.Header.Set("X-CSRF-Token", "the-wrong-token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusForbidden)
	assertErrorCode(t, rec, "AUTH_CSRF_FAILED")
	if *reached {
		t.Error("downstream handler should not have been reached")
	}
}

func TestCSRF_CorrectTokenPassesThrough(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	sub, sid := uuid.New().String(), uuid.New().String()
	tokenStr, _ := mintAccess(t, ks, sub, sid, true)
	storeCSRFSession(t, rdb, sub, sid, "the-real-token")

	r, reached := newCSRFRouter(ks, rdb)

	req := httptest.NewRequest(http.MethodPost, "/protected", nil)
	setCookie(req, authmw.AccessCookieName, tokenStr)
	req.Header.Set("X-CSRF-Token", "the-real-token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)
	if !*reached {
		t.Error("downstream handler should have been reached")
	}
}

func TestCSRF_SafeMethodIsExempt(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	sub, sid := uuid.New().String(), uuid.New().String()
	tokenStr, _ := mintAccess(t, ks, sub, sid, true)
	// Deliberately no session stored — GET must not need one.

	r, reached := newCSRFRouter(ks, rdb)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	setCookie(req, authmw.AccessCookieName, tokenStr)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)
	if !*reached {
		t.Error("downstream handler should have been reached (GET is CSRF-exempt)")
	}
}

// D7: refresh is CSRF-exempt because the SPA holds the token in memory only
// and requiring it on refresh would deadlock the post-reload boot path.
func TestCSRF_RefreshEndpointIsExempt(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	sub, sid := uuid.New().String(), uuid.New().String()
	tokenStr, _ := mintAccess(t, ks, sub, sid, true)
	// No session stored, no X-CSRF-Token header — refresh must still pass CSRF.

	r, reached := newCSRFRouter(ks, rdb)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	setCookie(req, authmw.AccessCookieName, tokenStr)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)
	if !*reached {
		t.Error("downstream handler should have been reached (refresh is CSRF-exempt, D7)")
	}
}
