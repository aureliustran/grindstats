package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/libs/httpkit"
)

// Auth validates the gs_access JWT on every request that reaches this
// middleware. It implements the four-step check chain from contract §6
// (signature → expiry → blacklist:{jti} → iat vs epoch) entirely through
// libs/authmw — the order and each check is defined there, never here.
//
// On success the decoded Claims are stored in the request context via
// authmw.WithClaims so that downstream handlers can read them without
// touching Redis (FR-21).
//
// On token failure (any step) the middleware returns 401 AUTH_INVALID_TOKEN
// regardless of which check failed, and emits auth.token.rejected with the
// specific reason. The reason must never reach the HTTP response body; it
// appears only in the audit record (audit-and-errors.md §4).
//
// On Redis I/O error the middleware calls authmw.CheckDegradation:
//   - GET/HEAD → FailOpen: pass the request on signature+expiry alone, warn.
//   - Everything else (including /auth/refresh and /auth/logout*) → FailClosed:
//     503 SERVICE_UNAVAILABLE, error log.
//
// Public routes opt out by route group, not by a path list here. Adding a
// public-path list to this file is the failure the partition was drawn to
// prevent; see be-wiring's brief for how route groups make the opt-out work.
func Auth(
	ks *authmw.KeySet,
	rdb *redis.Client,
	log auditlog.Writer,
	logger *slog.Logger,
) gin.HandlerFunc {
	logger = logOr(logger)
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// Extract the access token from the HttpOnly cookie (contract §2.2).
		tokenStr, err := c.Cookie(authmw.AccessCookieName)
		if err != nil || tokenStr == "" {
			// No cookie present — treat as a missing/bad signature so the
			// caller gets AUTH_INVALID_TOKEN without revealing that no token
			// existed at all (enumeration protection).
			httpkit.Error(c, auditmodel.ErrAuthInvalidToken)
			return
		}

		claims, reason, ioErr := ks.Check(ctx, rdb, tokenStr, authmw.TypAccess)

		if ioErr != nil {
			// Redis is unreachable — apply the degradation policy.
			switch authmw.CheckDegradation(c.Request.Method, c.Request.URL.Path) {
			case authmw.FailOpen:
				logger.WarnContext(ctx, "auth: Redis unreachable, failing open (read-only request)",
					"request_id", RequestIDFrom(c),
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"error", ioErr,
				)
				// Re-run the check without Redis (nil rdb) to get the decoded claims
				// for the context; signature+expiry already passed, so this returns
				// the claims or a signature/expiry failure — never a Redis error.
				claims, reason, _ = ks.Check(ctx, nil, tokenStr, authmw.TypAccess)
				if reason != "" {
					// Signature or expiry failed even without Redis.
					writeTokenRejected(c, log, logger, reason, "")
					return
				}
				// Store degraded claims and proceed.
				c.Request = c.Request.WithContext(authmw.WithClaims(ctx, claims))
				c.Next()
				return
			default: // FailClosed
				logger.ErrorContext(ctx, "auth: Redis unreachable, failing closed",
					"request_id", RequestIDFrom(c),
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"error", ioErr,
				)
				httpkit.ServiceUnavailable(c)
				return
			}
		}

		if reason != "" {
			// One of the four checks failed; record which one, but never leak
			// it to the caller (auth-and-errors.md §4).
			writeTokenRejected(c, log, logger, reason, claims.JTI)
			return
		}

		// All four checks passed. Store claims so downstream handlers never
		// call Redis (FR-21): they read from context, not from the blacklist.
		c.Request = c.Request.WithContext(authmw.WithClaims(ctx, claims))
		c.Next()
	}
}

// writeTokenRejected emits the audit event and returns 401 AUTH_INVALID_TOKEN.
// The reason is stored in the audit record and never in the HTTP response.
func writeTokenRejected(
	c *gin.Context,
	log auditlog.Writer,
	logger *slog.Logger,
	reason auditmodel.TokenRejectReason,
	jti string,
) {
	ctx := c.Request.Context()
	reqID := RequestIDFrom(c)

	if log != nil {
		// request_id appears both as a declared required field (so it is
		// captured in the JSONB fields column) and in WriteContext (so it
		// lands in the dedicated request_id column). NFR-07: never log the
		// token value itself, only the jti handle.
		if err := log.Write(ctx, auditmodel.EvtAuthTokenRejected, map[string]any{
			"reason":     reason,
			"jti":        jti,
			"request_id": reqID,
		}, auditlog.WriteContext{RequestID: reqID}); err != nil {
			logger.ErrorContext(ctx, "auth: failed to write token-rejected audit event",
				"error", err, "request_id", reqID)
		}
	}

	// 401 regardless of which check failed — never leak the reason to the
	// caller (audit-and-errors.md §4).
	httpkit.Error(c, auditmodel.ErrAuthInvalidToken)
}
