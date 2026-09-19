package session

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/libs/httpkit"
)

// handleLogin implements POST /auth/login (contract §1 row 8, FR-10..14).
//
// Login order of operations (contract §1.1, decision D2):
//
//  1. Validate request body.
//  2. Look up account. Check backoff for known accounts.
//  3. Always argon2id-verify (a dummy comparison for unknown email).
//  4. Wrong password or unknown email → 401 AUTH_INVALID_CREDENTIALS
//     (byte-identical for both cases — contract §1.1, FR-08/FR-14).
//  5. Correct password but suspended → 403 AUTH_ACCOUNT_SUSPENDED.
//     Checking suspension only AFTER correct password is decision D2:
//     moving it earlier reintroduces the enumeration oracle.
//  6. Success: clear fail counters, issue session, return CSRF token.
func (h *Handler) handleLogin(c *gin.Context) {
	var req struct {
		Email    string `json:"email"    binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, auditmodel.ErrValidationFailed)
		return
	}
	if req.Email == "" || req.Password == "" {
		httpkit.Error(c, auditmodel.ErrValidationFailed)
		return
	}

	ctx := c.Request.Context()
	sourceIP := c.ClientIP()
	emailLower := strings.ToLower(req.Email)

	// ── 2. Account lookup ───────────────────────────────────────────────────
	account, err := h.accounts.ByEmail(ctx, emailLower)
	if err != nil {
		h.logger.ErrorContext(ctx, "session: ByEmail error", "error", err)
		httpkit.InternalError(c)
		return
	}

	// ── Backoff check (known accounts only) ─────────────────────────────────
	// The brief orders this as step 1 but it is implemented after the account
	// lookup because the backoff key is keyed by user_id, which requires the
	// account row. The brief's ordering describes intent, not line order.
	if account != nil {
		inBackoff, backoffErr := authmw.InBackoff(ctx, h.rdb, account.ID.String())
		if backoffErr != nil {
			h.logger.ErrorContext(ctx, "session: InBackoff error", "error", backoffErr)
			httpkit.ServiceUnavailable(c)
			return
		}
		if inBackoff {
			ttl, _ := authmw.GetLoginBackoffTTL(ctx, h.rdb, account.ID.String())
			c.Header("Retry-After", fmt.Sprintf("%d", int(ttl.Seconds())))
			actorID := account.ID
			_ = h.audit.Write(ctx, auditmodel.EvtAuthLoginBackoffTriggered, map[string]any{
				"user_id":       account.ID.String(),
				"delay_seconds": int(ttl.Seconds()),
				"source_ip":     sourceIP,
			}, auditlog.WriteContext{ActorUserID: &actorID})
			httpkit.Error(c, auditmodel.ErrAuthRateLimited)
			return
		}
	}

	// ── 3. Argon2id verification — always, regardless of account existence ───
	// An unknown email or an account with no local password (OAuth-only)
	// runs the dummy comparison instead, so the timing is indistinguishable
	// from a correct-format but wrong password (contract §1.1, FR-08/FR-14).
	var correct bool
	if account != nil && account.PasswordHash != "" {
		var verifyErr error
		correct, verifyErr = h.hasher.Verify(req.Password, account.PasswordHash)
		if verifyErr != nil {
			// Malformed hash stored in the database; log it but treat as
			// mismatch so the response is still indistinguishable from wrong
			// password.
			h.logger.ErrorContext(ctx, "session: argon2id verify malformed hash", "error", verifyErr)
			correct = false
		}
	} else {
		h.hasher.DummyVerify(req.Password)
	}

	// ── 4. Wrong password or unknown email → 401 AUTH_INVALID_CREDENTIALS ───
	// The response body and status must be byte-identical for both cases
	// (enumeration protection, contract §1.1).
	if !correct || account == nil {
		reason := auditmodel.LoginFailureReasonBadPassword
		if account == nil {
			reason = auditmodel.LoginFailureReasonUnknownEmail
		}

		fields := map[string]any{
			"reason":    reason,
			"source_ip": sourceIP,
		}
		if account != nil {
			actorID := account.ID
			fields["user_id"] = account.ID.String()
			_ = h.audit.Write(ctx, auditmodel.EvtAuthLoginFailed, fields,
				auditlog.WriteContext{ActorUserID: &actorID})
		} else {
			_ = h.audit.Write(ctx, auditmodel.EvtAuthLoginFailed, fields,
				auditlog.WriteContext{})
		}

		// Increment fail counters and possibly set backoff.
		if account != nil {
			count, _ := authmw.IncrLoginFailUser(ctx, h.rdb, account.ID.String())
			if count >= 5 {
				// Step = failures beyond the 4th; step 1 at failure 5,
				// step 2 at failure 6, etc.
				step := int(count) - 4
				_ = authmw.SetLoginBackoff(ctx, h.rdb, account.ID.String(), step)
			}
		}
		_, _ = authmw.IncrLoginFailIP(ctx, h.rdb, sourceIP)

		// Byte-identical response for both unknown-email and bad-password.
		httpkit.Error(c, auditmodel.ErrAuthInvalidCredentials)
		return
	}

	// ── 5. Suspended check — ONLY after correct password (D2) ───────────────
	if account.IsSuspended() {
		actorID := account.ID
		_ = h.audit.Write(ctx, auditmodel.EvtAuthLoginSuspended, map[string]any{
			"user_id":   account.ID.String(),
			"source_ip": sourceIP,
		}, auditlog.WriteContext{ActorUserID: &actorID})
		httpkit.Error(c, auditmodel.ErrAuthAccountSuspended)
		return
	}

	// ── 6. Success ────────────────────────────────────────────────────────────
	_ = authmw.ClearLoginFail(ctx, h.rdb, account.ID.String(), sourceIP)

	csrfToken, err := h.Issue(ctx, c.Writer, *account, c.GetHeader("User-Agent"), sourceIP)
	if err != nil {
		h.logger.ErrorContext(ctx, "session: Issue failed", "error", err)
		httpkit.InternalError(c)
		return
	}

	httpkit.OK(c, gin.H{"csrf_token": csrfToken})
}
