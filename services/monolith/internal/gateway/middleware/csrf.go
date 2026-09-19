package middleware

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/authmw"
	"grindstats/libs/auditmodel"
	"grindstats/libs/httpkit"
)

// csrfTokenHeader is the header the SPA attaches to every unsafe request after
// login (contract §2.3, §8.2).
const csrfTokenHeader = "X-CSRF-Token"

// refreshPath is exempt from CSRF per decision D7: the SPA holds the CSRF
// token in memory only, and after a reload with an expired access token the
// boot sequence is refresh → /users/me. Requiring CSRF on refresh deadlocks
// that boot path; refresh is protected instead by SameSite=Lax and the
// refresh cookie's Path=/api/v1/auth scope.
const refreshPath = "/api/v1/auth/refresh"

// CSRF validates the X-CSRF-Token header on unsafe requests that carry a
// validated access token. It reads the bound CSRF token from the session
// record in Redis (user_sessions:{sub}, field = sid) and compares
// constant-time.
//
// Exemptions (contract §2.3, D7):
//   - Safe methods (GET, HEAD, OPTIONS): no state mutation, so no CSRF risk.
//   - POST /api/v1/auth/refresh: exempt by decision D7 (see const refreshPath).
//
// The unauthenticated auth endpoints (#1–#7 in contract §1) never reach this
// middleware because they are registered on a route group without the auth
// stage; they have no session to bind a CSRF token to.
func CSRF(rdb *redis.Client, logger *slog.Logger) gin.HandlerFunc {
	logger = logOr(logger)
	return func(c *gin.Context) {
		// Exempt safe methods — no state mutation, no CSRF risk.
		if isSafeMethod(c.Request.Method) {
			c.Next()
			return
		}

		// Exempt POST /auth/refresh (D7).
		if strings.EqualFold(c.Request.Method, http.MethodPost) &&
			c.Request.URL.Path == refreshPath {
			c.Next()
			return
		}

		// Retrieve the claims the auth middleware already validated and stored.
		claims, ok := authmw.ClaimsFromContext(c.Request.Context())
		if !ok {
			// Auth middleware did not run or passed without claims; treat as
			// a CSRF failure to avoid silent bypass.
			httpkit.Error(c, auditmodel.ErrAuthCsrfFailed)
			return
		}

		presented := c.GetHeader(csrfTokenHeader)
		if presented == "" {
			logger.WarnContext(c.Request.Context(), "csrf: missing X-CSRF-Token header",
				"request_id", RequestIDFrom(c),
				"user_id", claims.Sub,
			)
			httpkit.Error(c, auditmodel.ErrAuthCsrfFailed)
			return
		}

		// Look up the session record to get the bound CSRF token.
		session, err := authmw.GetSession(c.Request.Context(), rdb, claims.Sub, claims.SID)
		if err != nil || session == nil {
			// Redis error or session not found. Fail closed: no session → no
			// CSRF token to verify against → reject.
			logger.WarnContext(c.Request.Context(), "csrf: session not found or Redis error",
				"request_id", RequestIDFrom(c),
				"user_id", claims.Sub,
				"sid", claims.SID,
				"error", err,
			)
			httpkit.Error(c, auditmodel.ErrAuthCsrfFailed)
			return
		}

		// Constant-time comparison prevents timing-oracle attacks.
		if subtle.ConstantTimeCompare([]byte(presented), []byte(session.CSRFToken)) != 1 {
			logger.WarnContext(c.Request.Context(), "csrf: token mismatch",
				"request_id", RequestIDFrom(c),
				"user_id", claims.Sub,
			)
			httpkit.Error(c, auditmodel.ErrAuthCsrfFailed)
			return
		}

		c.Next()
	}
}

// isSafeMethod returns true for HTTP methods that do not mutate state and are
// therefore exempt from CSRF validation (contract §2.3).
func isSafeMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}
