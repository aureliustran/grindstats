package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/libs/httpkit"
)

// RequireRole returns middleware that enforces a minimum role on the route
// group it is applied to. The only role accepted in run 1 is
// auditmodel.RoleSystemAdmin; run 2 (AUTH-005) mounts routes behind it.
//
// Contract §6, FR-22: a user token on an admin-only route returns 403
// AUTH_FORBIDDEN, never 404. The SRS explicitly rejects 404 here because
// hiding admin routes gives a misleading privacy guarantee — an attacker that
// discovers the path still hits 403, and a real operator that misconfigures
// the role still gets an actionable error rather than silent misdirection.
//
// The middleware emits admin.action.denied for every forbidden attempt,
// including the user_id, path and request_id — so the audit log surfaces
// any systematic probing of admin endpoints.
func RequireRole(
	required auditmodel.Role,
	log auditlog.Writer,
	logger *slog.Logger,
) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context) {
		claims, ok := authmw.ClaimsFromContext(c.Request.Context())
		if !ok {
			// No claims = auth did not run (should not happen if this
			// middleware is applied to a protected group, but fail safely).
			httpkit.Error(c, auditmodel.ErrAuthForbidden)
			return
		}

		if auditmodel.Role(claims.Role) == required {
			// Role matches — allow the request through.
			c.Next()
			return
		}

		reqID := RequestIDFrom(c)
		path := c.Request.URL.Path

		if log != nil {
			if err := log.Write(c.Request.Context(), auditmodel.EvtAdminActionDenied,
				map[string]any{
					"user_id":    claims.Sub,
					"path":       path,
					"request_id": reqID,
				},
				auditlog.WriteContext{RequestID: reqID},
			); err != nil {
				logger.ErrorContext(c.Request.Context(),
					"role: failed to write admin.action.denied audit event",
					"error", err, "request_id", reqID)
			}
		}

		logger.WarnContext(c.Request.Context(), "role: forbidden admin access attempt",
			"request_id", reqID,
			"user_id", claims.Sub,
			"required_role", string(required),
			"actual_role", claims.Role,
			"path", path,
		)

		// 403, never 404 (FR-22, contract §6).
		httpkit.Error(c, auditmodel.ErrAuthForbidden)
	}
}
