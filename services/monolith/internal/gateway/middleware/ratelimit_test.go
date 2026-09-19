package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"grindstats/libs/auditmodel"
	"grindstats/libs/httpkit"
	"grindstats/services/monolith/internal/gateway/middleware"
)

// AUTH_RATE_LIMITED must map to 429 in the generated error code table, since
// RateLimit relies on httpkit.Error to write that status for it.
func TestRateLimit_CodeMapsTo429(t *testing.T) {
	require.Equal(t, http.StatusTooManyRequests, httpkit.StatusFor(auditmodel.ErrAuthRateLimited))
}

// A slow, legitimate retrier (well under the limit, just spaced further apart
// than the window) must never be locked out. Each request refreshing the
// window's TTL would turn a 5/min limit into a permanent lockout for anyone
// polling slower than once a minute.
func TestRateLimit_SlowRetrierIsNotLockedOut(t *testing.T) {
	mr, rdb := newTestRedis(t)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.RateLimit(rdb, nil))
	r.POST("/api/v1/auth/register", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
		req.RemoteAddr = "203.0.113.5:1234"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (slow retrier must not be locked out)", i+1, rec.Code)
		}
		mr.FastForward(50 * time.Second)
	}
}

func newRateLimitRouter(t *testing.T) *gin.Engine {
	t.Helper()
	_, rdb := newTestRedis(t)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.RateLimit(rdb, nil))
	r.POST("/api/v1/auth/register", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/api/v1/auth/login", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

// NFR-04: register is limited to 5/min per IP. The 6th request in the window
// is rejected with 429 and a Retry-After header.
func TestRateLimit_SixthRegisterRequestInWindowIsRejected(t *testing.T) {
	r := newRateLimitRouter(t)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
		req.RemoteAddr = "203.0.113.5:1234"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
	req.RemoteAddr = "203.0.113.5:1234"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusTooManyRequests)
	assertErrorCode(t, rec, "AUTH_RATE_LIMITED")
	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected a Retry-After header on 429")
	}
}

// A different source IP has its own bucket and is unaffected by another IP's
// exhausted limit.
func TestRateLimit_DifferentIPHasIndependentBucket(t *testing.T) {
	r := newRateLimitRouter(t)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
		req.RemoteAddr = "203.0.113.5:1234"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
	req.RemoteAddr = "198.51.100.9:1234"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)
}

// login has a higher limit (10/min) than register (5/min) — confirms the
// per-endpoint bucket is keyed by rule, not shared globally per IP.
func TestRateLimit_LoginHasItsOwnHigherLimit(t *testing.T) {
	r := newRateLimitRouter(t)

	// Exhaust register's 5/min bucket.
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
		req.RemoteAddr = "203.0.113.5:1234"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}

	// Login from the same IP must still work — separate bucket.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "203.0.113.5:1234"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)
}

// A path with no configured rule is never rate-limited by this middleware.
func TestRateLimit_UnconfiguredPathPassesThroughUnlimited(t *testing.T) {
	_, rdb := newTestRedis(t)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.RateLimit(rdb, nil))
	r.GET("/api/v1/users/me", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
		req.RemoteAddr = "203.0.113.5:1234"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d to an unconfigured path: status = %d, want 200", i+1, rec.Code)
		}
	}
}
