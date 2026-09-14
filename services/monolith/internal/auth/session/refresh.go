package session

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/libs/httpkit"
)

const idleSessionWindow = 14 * 24 * time.Hour

// handleRefresh implements POST /auth/refresh (contract §1 row 9, FR-30/31/34).
//
// Flow:
//  1. Read and validate the refresh cookie through authmw's check chain.
//  2. Replay detection: if the refresh JTI is blacklisted, the token was
//     already rotated. Advance the epoch (revoking every session), emit
//     auth.refresh.replay_detected at critical, and return 401. No grace
//     window (plan.md §7; single-flight on the client is the agreed mitigation).
//  3. Idle-session check: if now − last_seen_at > 14 days → AUTH_SESSION_EXPIRED.
//  4. Mint a new pair with the SAME sid; blacklist the old refresh jti with
//     the token's remaining life; update the session record; return the
//     existing CSRF token.
//
// The OLD access token keeps working until it expires (AUTH-003 scenario 1).
// No blacklist write for it here — that is deliberate (brief §refresh).
func (h *Handler) handleRefresh(c *gin.Context) {
	ctx := c.Request.Context()
	sourceIP := c.ClientIP()

	refreshStr, err := c.Cookie(authmw.RefreshCookieName)
	if err != nil || refreshStr == "" {
		httpkit.Error(c, auditmodel.ErrAuthInvalidToken)
		return
	}

	claims, reason, ioErr := h.ks.Check(ctx, h.rdb, refreshStr, authmw.TypRefresh)

	// ── Redis I/O error ──────────────────────────────────────────────────────
	if ioErr != nil {
		// /auth/refresh always fails closed (contract §6, degradation.go).
		h.logger.ErrorContext(ctx, "session: refresh Redis error", "error", ioErr)
		httpkit.ServiceUnavailable(c)
		return
	}

	// ── Replay detection (FR-31) ─────────────────────────────────────────────
	// A blacklisted refresh token means the token was already rotated and is
	// being replayed — treat as theft: revoke every session immediately.
	if reason == auditmodel.TokenRejectReasonBlacklisted {
		// The token passed signature+expiry checks (steps 1-2 of Check)
		// before hitting the blacklist in step 3. Re-running Check with
		// nil rdb gives us the validated claims without Redis round-trips.
		claimsFromTok, _, _ := h.ks.Check(ctx, nil, refreshStr, authmw.TypRefresh)
		sub := claimsFromTok.Sub
		jti := claimsFromTok.JTI

		if sub != "" {
			_ = authmw.SetEpoch(ctx, h.rdb, sub)
		}

		_ = h.audit.Write(ctx, auditmodel.EvtAuthRefreshReplayDetected, map[string]any{
			"user_id":      sub,
			"replayed_jti": jti,
			"source_ip":    sourceIP,
		}, auditlog.WriteContext{})

		httpkit.Error(c, auditmodel.ErrAuthInvalidToken)
		return
	}

	// ── Other token failure ──────────────────────────────────────────────────
	if reason != "" {
		switch reason {
		case auditmodel.TokenRejectReasonEpochStale:
			httpkit.Error(c, auditmodel.ErrAuthSessionExpired)
		default:
			httpkit.Error(c, auditmodel.ErrAuthInvalidToken)
		}
		return
	}

	// Claims are now valid (signature ✓, expiry ✓, not blacklisted, epoch ✓).
	userID := claims.Sub
	sid := claims.SID
	refreshJTI := claims.JTI
	refreshIAT := claims.IAT

	// ── Idle-session check (FR-34) ───────────────────────────────────────────
	sess, err := authmw.GetSession(ctx, h.rdb, userID, sid)
	if err == redis.Nil || sess == nil {
		// Session record gone (manual deletion or TTL expiry).
		httpkit.Error(c, auditmodel.ErrAuthSessionExpired)
		return
	}
	if err != nil {
		h.logger.ErrorContext(ctx, "session: GetSession error", "error", err)
		httpkit.ServiceUnavailable(c)
		return
	}

	now := h.clock()
	if now.Sub(sess.LastSeenAt) > idleSessionWindow {
		httpkit.Error(c, auditmodel.ErrAuthSessionExpired)
		return
	}

	// ── Rotate: mint new pair, blacklist old refresh jti ─────────────────────
	newAccess, _, mintErr := h.ks.MintAccess(
		userID, sid,
		claims.Role, claims.Tier,
		claims.EmailVerified,
	)
	if mintErr != nil {
		h.logger.ErrorContext(ctx, "session: MintAccess error", "error", mintErr)
		httpkit.InternalError(c)
		return
	}

	newRefresh, newRefreshJTI, mintErr := h.ks.MintRefresh(userID, sid)
	if mintErr != nil {
		h.logger.ErrorContext(ctx, "session: MintRefresh error", "error", mintErr)
		httpkit.InternalError(c)
		return
	}

	// Blacklist the old refresh jti with a TTL equal to its remaining life.
	// The token's expiry is IAT + RefreshTTL (mirrors how MintRefresh sets exp).
	oldRefreshExp := time.Unix(refreshIAT, 0).Add(authmw.RefreshTTL)
	if blacklistErr := authmw.BlacklistJTI(ctx, h.rdb, refreshJTI, oldRefreshExp); blacklistErr != nil {
		h.logger.ErrorContext(ctx, "session: BlacklistJTI old refresh", "error", blacklistErr)
		httpkit.ServiceUnavailable(c)
		return
	}

	// Update session record: new refresh JTI and last_seen_at; keep sid and
	// csrf_token (rotation preserves the CSRF binding, contract §2.3).
	updatedSess := authmw.Session{
		SID:         sess.SID,
		RefreshJTI:  newRefreshJTI,
		DeviceLabel: sess.DeviceLabel,
		CreatedAt:   sess.CreatedAt,
		LastSeenAt:  now,
		CSRFToken:   sess.CSRFToken,
	}
	if rotateErr := authmw.RotateSession(ctx, h.rdb, userID, updatedSess); rotateErr != nil {
		h.logger.ErrorContext(ctx, "session: RotateSession error", "error", rotateErr)
		httpkit.ServiceUnavailable(c)
		return
	}

	authmw.SetSessionCookies(c.Writer, newAccess, newRefresh, h.cookies)

	_ = h.audit.Write(ctx, auditmodel.EvtAuthRefreshRotated, map[string]any{
		"user_id": userID,
		"old_jti": refreshJTI,
		"new_jti": newRefreshJTI,
	}, auditlog.WriteContext{})

	httpkit.OK(c, gin.H{"csrf_token": sess.CSRFToken})
}
