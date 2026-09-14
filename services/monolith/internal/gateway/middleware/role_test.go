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

// FR-22: a User token on a SystemAdmin-only route returns 403, never 404.
func TestRequireRole_UserTokenOnAdminRouteIsForbiddenNotNotFound(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	fake := auditlog.NewFake()
	sub, sid := uuid.New().String(), uuid.New().String()
	tokenStr, _ := mintAccess(t, ks, sub, sid, true) // role defaults to User

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Auth(ks, rdb, fake, nil))
	r.Use(middleware.RequireRole(auditmodel.RoleSystemAdmin, fake, nil))
	r.GET("/api/v1/admin/users", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	setCookie(req, authmw.AccessCookieName, tokenStr)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusForbidden)
	assertErrorCode(t, rec, "AUTH_FORBIDDEN")
	if rec.Code == http.StatusNotFound {
		t.Error("admin route must return 403, never 404 (FR-22)")
	}
}

func TestRequireRole_UserTokenOnAdminRouteWritesAuditEvent(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	fake := auditlog.NewFake()
	sub, sid := uuid.New().String(), uuid.New().String()
	tokenStr, _ := mintAccess(t, ks, sub, sid, true)

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Auth(ks, rdb, fake, nil))
	r.Use(middleware.RequireRole(auditmodel.RoleSystemAdmin, fake, nil))
	r.GET("/api/v1/admin/users", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	setCookie(req, authmw.AccessCookieName, tokenStr)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	events := fake.EventsOf(auditmodel.EvtAdminActionDenied)
	if len(events) != 1 {
		t.Fatalf("expected 1 admin.action.denied event, got %d", len(events))
	}
	if events[0].Fields["user_id"] != sub {
		t.Errorf("user_id = %v, want %v", events[0].Fields["user_id"], sub)
	}
	if events[0].Fields["path"] != "/api/v1/admin/users" {
		t.Errorf("path = %v, want /api/v1/admin/users", events[0].Fields["path"])
	}
}

func TestRequireRole_MatchingRolePassesThrough(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	fake := auditlog.NewFake()
	sub, sid := uuid.New().String(), uuid.New().String()
	tokenStr, _, err := ks.MintAccess(sub, sid, string(auditmodel.RoleSystemAdmin), "free", true)
	if err != nil {
		t.Fatalf("MintAccess: %v", err)
	}

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Auth(ks, rdb, fake, nil))
	r.Use(middleware.RequireRole(auditmodel.RoleSystemAdmin, fake, nil))
	r.GET("/api/v1/admin/users", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	setCookie(req, authmw.AccessCookieName, tokenStr)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)
	if fake.Count() != 0 {
		t.Errorf("no admin.action.denied event expected for a matching role, got %d events", fake.Count())
	}
}
