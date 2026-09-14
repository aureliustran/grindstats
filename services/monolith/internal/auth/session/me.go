package session

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/libs/httpkit"
	"grindstats/services/monolith/internal/auth/authdomain"
)

// meUser is the user sub-object in the /users/me response (contract §1 row 12,
// AMD-003 for email_verified).
type meUser struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	Tier          string `json:"tier"`           // hardcoded "free" — TODO: subscription domain
	EmailVerified bool   `json:"email_verified"` // AMD-003: read from Account.IsVerified(), not the claim
}

// meResponse is the full /users/me success body enveloped by httpkit.OK.
type meResponse struct {
	User      meUser `json:"user"`
	CSRFToken string `json:"csrf_token"`
}

// handleMe implements GET /users/me (contract §1 row 12, D6, AMD-003).
//
// Reads the validated claims from context (placed by the gateway auth
// middleware or a test helper). Loads the account from the AccountRepo to
// get the email and email_verified status — not from the token claim — so
// that a newly-verified account is correctly reflected without requiring the
// user to obtain a fresh token (AMD-003). Re-issues the session-bound CSRF
// token from the session record so the SPA can refresh its in-memory copy.
func (h *Handler) handleMe(c *gin.Context) {
	claims, ok := authmw.ClaimsFromContext(c.Request.Context())
	if !ok {
		httpkit.Error(c, auditmodel.ErrAuthInvalidToken)
		return
	}

	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		httpkit.Error(c, auditmodel.ErrAuthInvalidToken)
		return
	}

	ctx := c.Request.Context()

	// AMD-003: email_verified comes from the account row, not the token claim.
	account, err := h.accounts.ByID(ctx, userID)
	if err != nil {
		if errors.Is(err, authdomain.ErrNotFound) {
			httpkit.Error(c, auditmodel.ErrAuthInvalidToken)
			return
		}
		h.logger.ErrorContext(ctx, "session: ByID error", "error", err)
		httpkit.InternalError(c)
		return
	}

	sess, err := authmw.GetSession(ctx, h.rdb, claims.Sub, claims.SID)
	if err == redis.Nil || sess == nil {
		// Session record gone — this can happen if logout ran concurrently or
		// the session TTL elapsed. Treat as expired.
		httpkit.Error(c, auditmodel.ErrAuthSessionExpired)
		return
	}
	if err != nil {
		h.logger.ErrorContext(ctx, "session: GetSession error", "error", err)
		httpkit.InternalError(c)
		return
	}

	httpkit.OK(c, meResponse{
		User: meUser{
			ID:            account.ID.String(),
			Email:         account.Email,
			Role:          string(account.Role),
			Tier:          "free", // TODO: subscription domain will own this
			EmailVerified: account.IsVerified(),
		},
		CSRFToken: sess.CSRFToken,
	})
}

// uuidFromSub parses a UUID from a Claims.Sub string. Returns zero UUID on
// parse failure (the caller decides whether that is fatal).
func uuidFromSub(sub string) (uuid.UUID, error) {
	return uuid.Parse(sub)
}
