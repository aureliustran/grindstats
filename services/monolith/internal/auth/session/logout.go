package session

import (
	"crypto/subtle"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/libs/httpkit"
)

// handleLogout implements POST /auth/logout (contract §1 row 10, FR-32).
//
// Reads claims from context (set by gateway auth middleware or test helper).
// Validates the X-CSRF-Token header constant-time against the session record.
// Blacklists the access jti and the session's refresh jti with their
// remaining lifetimes; deletes the session record; clears both cookies.
// Other sessions (other devices) are untouched — only the current sid is
// affected.
func (h *Handler) handleLogout(c *gin.Context) {
	claims, ok := authmw.ClaimsFromContext(c.Request.Context())
	if !ok {
		httpkit.Error(c, auditmodel.ErrAuthInvalidToken)
		return
	}

	ctx := c.Request.Context()

	// ── CSRF validation ───────────────────────────────────────────────────────
	sess, csrfOK := h.validateCSRF(c, claims)
	if !csrfOK {
		return // response already written by validateCSRF
	}

	// ── Blacklist access jti ──────────────────────────────────────────────────
	// Access token's expiry = IAT + 15-minute TTL (matches MintAccess).
	accessExp := time.Unix(claims.IAT, 0).Add(authmw.AccessTTL)
	if err := authmw.BlacklistJTI(ctx, h.rdb, claims.JTI, accessExp); err != nil {
		h.logger.ErrorContext(ctx, "session: BlacklistJTI access", "error", err)
		httpkit.ServiceUnavailable(c)
		return
	}

	// ── Blacklist refresh jti ─────────────────────────────────────────────────
	// Refresh token's expiry is approximated by last_seen_at + RefreshTTL.
	refreshExp := sess.LastSeenAt.Add(authmw.RefreshTTL)
	if err := authmw.BlacklistJTI(ctx, h.rdb, sess.RefreshJTI, refreshExp); err != nil {
		h.logger.ErrorContext(ctx, "session: BlacklistJTI refresh", "error", err)
		httpkit.ServiceUnavailable(c)
		return
	}

	// ── Delete session record ─────────────────────────────────────────────────
	if err := authmw.DeleteSession(ctx, h.rdb, claims.Sub, claims.SID); err != nil {
		h.logger.WarnContext(ctx, "session: DeleteSession error (continuing)", "error", err)
		// Not fatal — the jti blacklists above are the real revocation.
	}

	authmw.ClearCookies(c.Writer, h.cookies)

	actorID, _ := uuidFromSub(claims.Sub)
	_ = h.audit.Write(ctx, auditmodel.EvtAuthLogout, map[string]any{
		"user_id":     claims.Sub,
		"session_jti": claims.JTI,
	}, auditlog.WriteContext{ActorUserID: &actorID})

	c.Status(204)
}

// handleLogoutAll implements POST /auth/logout-all (contract §1 row 11, FR-33).
//
// One epoch write invalidates every outstanding token in O(1). No per-token
// blacklist writes, no session enumeration — AUTH-003 scenario 6 asserts this
// property. Clears cookies for the current device.
func (h *Handler) handleLogoutAll(c *gin.Context) {
	claims, ok := authmw.ClaimsFromContext(c.Request.Context())
	if !ok {
		httpkit.Error(c, auditmodel.ErrAuthInvalidToken)
		return
	}

	ctx := c.Request.Context()

	// ── CSRF validation ───────────────────────────────────────────────────────
	_, csrfOK := h.validateCSRF(c, claims)
	if !csrfOK {
		return
	}

	// ── One epoch write — no per-token ops (FR-33, NFR-05) ───────────────────
	userID, _ := uuidFromSub(claims.Sub)
	if err := h.RevokeAll(ctx, userID); err != nil {
		h.logger.ErrorContext(ctx, "session: RevokeAll error", "error", err)
		httpkit.ServiceUnavailable(c)
		return
	}

	authmw.ClearCookies(c.Writer, h.cookies)

	c.Status(204)
}

// ─── shared helpers ───────────────────────────────────────────────────────────

// validateCSRF reads the X-CSRF-Token header and compares it constant-time
// against the bound CSRF token stored in the session record. Returns the
// session record on success. Writes the error response and returns (nil, false)
// on failure.
func (h *Handler) validateCSRF(c *gin.Context, claims authmw.Claims) (*authmw.Session, bool) {
	presented := c.GetHeader("X-CSRF-Token")
	if presented == "" {
		httpkit.Error(c, auditmodel.ErrAuthCsrfFailed)
		return nil, false
	}

	sess, err := authmw.GetSession(c.Request.Context(), h.rdb, claims.Sub, claims.SID)
	if err == redis.Nil || sess == nil {
		httpkit.Error(c, auditmodel.ErrAuthCsrfFailed)
		return nil, false
	}
	if err != nil {
		h.logger.ErrorContext(c.Request.Context(), "session: GetSession for CSRF", "error", err)
		httpkit.Error(c, auditmodel.ErrAuthCsrfFailed)
		return nil, false
	}

	if subtle.ConstantTimeCompare([]byte(presented), []byte(sess.CSRFToken)) != 1 {
		httpkit.Error(c, auditmodel.ErrAuthCsrfFailed)
		return nil, false
	}

	return sess, true
}
