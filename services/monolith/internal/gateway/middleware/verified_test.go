package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/services/monolith/internal/gateway/middleware"
)

// AUTH-001 TC-09: an unverified account is read-only on its own data.
func TestVerifiedWrite_UnverifiedAccountBlockedOnUnsafeWrite(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	fake := auditlog.NewFake()
	sub, sid := uuid.New().String(), uuid.New().String()
	tokenStr, _ := mintAccessUnverified(t, ks, sub, sid)

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Auth(ks, rdb, fake, nil))
	r.Use(middleware.VerifiedWrite(fake, nil))
	r.POST("/api/v1/data", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/data", nil)
	setCookie(req, authmw.AccessCookieName, tokenStr)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusForbidden)
	assertErrorCode(t, rec, "AUTH_EMAIL_UNVERIFIED")

	events := fake.EventsOf(auditmodel.EvtAuthWriteBlockedUnverified)
	if len(events) != 1 {
		t.Fatalf("expected 1 auth.write_blocked_unverified event, got %d", len(events))
	}
	if events[0].Fields["path"] != "/api/v1/data" {
		t.Errorf("path = %v, want /api/v1/data", events[0].Fields["path"])
	}
}

func TestVerifiedWrite_UnverifiedAccountCanStillRead(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	fake := auditlog.NewFake()
	sub, sid := uuid.New().String(), uuid.New().String()
	tokenStr, _ := mintAccessUnverified(t, ks, sub, sid)

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Auth(ks, rdb, fake, nil))
	r.Use(middleware.VerifiedWrite(fake, nil))
	r.GET("/api/v1/data", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/data", nil)
	setCookie(req, authmw.AccessCookieName, tokenStr)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)
}

func TestVerifiedWrite_UnverifiedAccountCanStillWriteUnderAuthPrefix(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	fake := auditlog.NewFake()
	sub, sid := uuid.New().String(), uuid.New().String()
	tokenStr, _ := mintAccessUnverified(t, ks, sub, sid)

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Auth(ks, rdb, fake, nil))
	r.Use(middleware.VerifiedWrite(fake, nil))
	r.POST("/api/v1/auth/verify-email", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify-email", nil)
	setCookie(req, authmw.AccessCookieName, tokenStr)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// Even unverified, the auth sub-tree must remain reachable — otherwise an
	// unverified user could never verify their email at all.
	assertStatus(t, rec, http.StatusOK)
}

func TestVerifiedWrite_VerifiedAccountWritesFreely(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	fake := auditlog.NewFake()
	sub, sid := uuid.New().String(), uuid.New().String()
	tokenStr, _ := mintAccess(t, ks, sub, sid, true) // email_verified = true

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Auth(ks, rdb, fake, nil))
	r.Use(middleware.VerifiedWrite(fake, nil))
	r.POST("/api/v1/data", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/data", nil)
	setCookie(req, authmw.AccessCookieName, tokenStr)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)
	if fake.Count() != 0 {
		t.Errorf("no audit events expected for a verified account's write, got %d", fake.Count())
	}
}

// mintAccessUnverified mints an access token with email_verified=false.
func mintAccessUnverified(t *testing.T, ks *authmw.KeySet, sub, sid string) (tokenStr, jti string) {
	t.Helper()
	var err error
	tokenStr, jti, err = ks.MintAccess(sub, sid, string(auditmodel.RoleUser), "free", false)
	if err != nil {
		t.Fatalf("mintAccessUnverified: %v", err)
	}
	return
}
