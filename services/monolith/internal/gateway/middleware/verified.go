package middleware

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/libs/httpkit"
)

// authAPIPrefix is the one path subtree exempt from the unverified-write rule
// (contract §6, D4). Everything under /api/v1/auth/ is owned by the auth
// package itself and must remain reachable so an unverified user can still
// log out, refresh, verify their address, or reset their password.
const authAPIPrefix = "/api/v1/auth/"

// VerifiedWrite enforces FR-03: an account whose access token carries
// email_verified=false may not issue unsafe (state-mutating) HTTP requests
// outside the auth sub-tree.
//
// Contract §6, decision D4: this is the ONLY place FR-03 is enforced.
// Per-service checks were rejected because a domain that forgets the rule
// fails open silently; a single gateway check catches all current and future
// domains without any domain having to remember.
//
// Enforcement: unsafe method + email_verified=false + path not under
// /api/v1/auth/ → 403 AUTH_EMAIL_UNVERIFIED, emitting
// auth.write_blocked_unverified with user_id, path and request_id.
func VerifiedWrite(log auditlog.Writer, logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context) {
		// Safe methods never mutate state; the rule does not apply.
		if isSafeMethod(c.Request.Method) {
			c.Next()
			return
		}

		// No claims in context means auth did not run or passed through
		// because the route is public. No claims → no email_verified field →
		// rule does not apply.
		claims, ok := authmw.ClaimsFromContext(c.Request.Context())
		if !ok {
			c.Next()
			return
		}

		// Accounts that have verified their email are unrestricted.
		if claims.EmailVerified {
			c.Next()
			return
		}

		// The auth sub-tree is always reachable, verified or not — otherwise
		// an unverified user cannot verify their email at all.
		if strings.HasPrefix(c.Request.URL.Path, authAPIPrefix) {
			c.Next()
			return
		}

		// An unverified account is attempting a write outside /auth/. Emit
		// the audit event and reject with 403.
		reqID := RequestIDFrom(c)
		path := c.Request.URL.Path

		if log != nil {
			if err := log.Write(c.Request.Context(), auditmodel.EvtAuthWriteBlockedUnverified,
				map[string]any{
					"user_id":    claims.Sub,
					"path":       path,
					"request_id": reqID,
				},
				auditlog.WriteContext{RequestID: reqID},
			); err != nil {
				logger.ErrorContext(c.Request.Context(),
					"verified-write: failed to write audit event",
					"error", err, "request_id", reqID)
			}
		}

		logger.InfoContext(c.Request.Context(), "verified-write: blocking unverified write",
			"request_id", reqID,
			"user_id", claims.Sub,
			"path", path,
		)

		httpkit.Error(c, auditmodel.ErrAuthEmailUnverified)
	}
}
