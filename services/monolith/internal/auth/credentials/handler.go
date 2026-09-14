// Package credentials implements the AUTH-001 credential-management endpoints:
// registration, email verification, password reset (both halves), and the
// OAuth-link confirmation (contract §1, rows 1–4, 7). It also implements
// authdomain.AccountProvisioner for the OAuth callback slice (Provisioner type).
//
// The single rule everything in AUTH-001 defends: a registrant can never end
// up with any role other than User (FR-04). No request field, header or
// payload value influences role assignment.
package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/libs/httpkit"
	"grindstats/libs/i18n"
	"grindstats/services/monolith/internal/auth/authdomain"
	"grindstats/services/monolith/internal/auth/hibp"
	"grindstats/services/monolith/internal/auth/mailer"
)

// Handler implements endpoints #1–#4 and #7 (contract §1) and
// authdomain.AccountProvisioner (implemented by the Provisioner type in this
// package and injected into the OAuth handler at the wave-4 composition root).
type Handler struct {
	accounts authdomain.AccountRepo
	oauthIds authdomain.OAuthIdentityRepo
	tokens   authdomain.LinkTokenRepo
	session  authdomain.SessionIssuer // consumed only for RevokeAll on password-reset confirm
	hasher   *PasswordHasher
	hibp     hibp.Client
	mailer   mailer.Mailer
	audit    auditlog.Writer
}

// New creates a Handler. All parameters are required.
func New(
	accounts authdomain.AccountRepo,
	oauthIds authdomain.OAuthIdentityRepo,
	tokens authdomain.LinkTokenRepo,
	session authdomain.SessionIssuer,
	hasher *PasswordHasher,
	hibpClient hibp.Client,
	m mailer.Mailer,
	aud auditlog.Writer,
) *Handler {
	return &Handler{
		accounts: accounts,
		oauthIds: oauthIds,
		tokens:   tokens,
		session:  session,
		hasher:   hasher,
		hibp:     hibpClient,
		mailer:   m,
		audit:    aud,
	}
}

// Register mounts the credential endpoints onto r. Called by the wave-4
// composition root; this slice never touches main.go or the gateway.
func (h *Handler) Register(r gin.IRouter) {
	r.POST("/auth/register", h.register)
	r.POST("/auth/verify-email", h.verifyEmail)
	r.POST("/auth/password-reset/request", h.requestPasswordReset)
	r.POST("/auth/password-reset/confirm", h.confirmPasswordReset)
	r.POST("/auth/oauth/link/confirm", h.confirmOAuthLink)
}

// ─── Request / response types ────────────────────────────────────────────────

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password"`
	// No "role" field — the absence is the enforcement of FR-04 (TC-06).
}

type tokenRequest struct {
	Token string `json:"token" binding:"required"`
}

type resetConfirmRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password"`
}

type statusResponse struct {
	Status string `json:"status"`
}

// ─── Interim VALIDATION_FAILED+details helper ────────────────────────────────
//
// libs/httpkit.Error cannot yet attach a "details" array to VALIDATION_FAILED.
// This local helper produces the identical envelope
//
//	{"error":{"code":"VALIDATION_FAILED","message":"...","details":[...]}}
//
// NOTE: do not replicate this pattern in other slices. The correct fix is an
// httpkit amendment; file one and remove this helper when it lands.
// See docs/stories/auth-epic/amendments/ for the pending amendment.

// ValidationDetail is one entry in the VALIDATION_FAILED details array
// (contract §1.1, rule values in run 1: "min_length", "format", "breached").
type ValidationDetail struct {
	Field string `json:"field"`
	Rule  string `json:"rule"`
}

type validationErrorBody struct {
	Code    auditmodel.ErrorCode `json:"code"`
	Message string               `json:"message"`
	Details []ValidationDetail   `json:"details"`
}

type validationErrorEnvelope struct {
	Error validationErrorBody `json:"error"`
}

// writeValidationError writes a 400 VALIDATION_FAILED with the given details
// array. It mirrors what httpkit.Error does for the status and message, adding
// only the details field that httpkit currently cannot.
func writeValidationError(c *gin.Context, details []ValidationDetail) {
	locale := httpkit.Locale(c)
	message := i18n.Render(string(auditmodel.ErrValidationFailed), locale)
	c.AbortWithStatusJSON(http.StatusBadRequest, validationErrorEnvelope{
		Error: validationErrorBody{
			Code:    auditmodel.ErrValidationFailed,
			Message: message,
			Details: details,
		},
	})
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// register handles POST /auth/register (contract §1 row 1).
//
// Enumeration protection (FR-08, §1.1): the response is byte-identical whether
// the email exists or not. Timing is equalized: we always run argon2id.Hash
// before attempting the DB insert, so a taken-address branch costs the same
// wall-clock time as a new-address branch.
func (h *Handler) register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, auditmodel.ErrValidationFailed)
		return
	}

	// Password policy (FR-02, D3): minimum length 10, then HIBP.
	if len(req.Password) < 10 {
		writeValidationError(c, []ValidationDetail{{Field: "password", Rule: "min_length"}})
		return
	}

	// HIBP check (D3): reject on hit, skip-and-warn on unreachable / timeout.
	hibpCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if breached, err := h.hibp.IsBreached(hibpCtx, req.Password); err != nil {
		slog.WarnContext(c.Request.Context(), "credentials: HIBP unreachable, skipping check", "error", err)
	} else if breached {
		writeValidationError(c, []ValidationDetail{{Field: "password", Rule: "breached"}})
		return
	}

	// Hash BEFORE attempting Create — this is the timing equalizer for the
	// taken-email case. A return before this call would be a timing oracle (§1.1).
	hash, err := h.hasher.Hash(req.Password)
	if err != nil {
		httpkit.InternalError(c)
		return
	}

	emailLower := strings.ToLower(req.Email)
	acc := authdomain.Account{
		ID:           uuid.New(),
		Email:        req.Email,
		EmailLower:   emailLower,
		PasswordHash: hash,
		Role:         auditmodel.RoleUser,   // always; FR-04
		Status:       auditmodel.AccountStatusActive,
		CreatedAt:    time.Now(),
	}

	if err := h.accounts.Create(c.Request.Context(), acc); err != nil {
		if errors.Is(err, authdomain.ErrEmailTaken) {
			// FR-08: same 202 as a new registration — no mail, no indication.
			httpkit.Data(c, http.StatusAccepted, statusResponse{Status: "pending_verification"})
			return
		}
		httpkit.InternalError(c)
		return
	}

	// Issue 24-hour verification token and email it.
	rawToken, err := h.tokens.Issue(c.Request.Context(), acc.ID,
		auditmodel.LinkKindEmailVerification, 24*time.Hour, nil)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "credentials: issue verification token", "error", err)
	} else if err := h.mailer.SendVerification(c.Request.Context(), acc.Email, rawToken); err != nil {
		slog.ErrorContext(c.Request.Context(), "credentials: send verification email", "error", err)
	}

	uid := acc.ID
	_ = h.audit.Write(c.Request.Context(), auditmodel.EvtAuthRegisterSucceeded,
		map[string]any{"user_id": uid.String()},
		auditlog.WriteContext{},
	)

	httpkit.Data(c, http.StatusAccepted, statusResponse{Status: "pending_verification"})
}

// verifyEmail handles POST /auth/verify-email (contract §1 row 2).
func (h *Handler) verifyEmail(c *gin.Context) {
	var req tokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, auditmodel.ErrValidationFailed)
		return
	}

	lt, err := h.tokens.Consume(c.Request.Context(), auditmodel.LinkKindEmailVerification, req.Token)
	if err != nil {
		if errors.Is(err, authdomain.ErrLinkInvalid) {
			h.writeLinkRejected(c, auditmodel.LinkKindEmailVerification, err)
			httpkit.Error(c, auditmodel.ErrAuthLinkInvalid)
			return
		}
		httpkit.InternalError(c)
		return
	}

	if err := h.accounts.MarkEmailVerified(c.Request.Context(), lt.UserID, time.Now()); err != nil {
		httpkit.InternalError(c)
		return
	}

	uid := lt.UserID
	_ = h.audit.Write(c.Request.Context(), auditmodel.EvtAuthEmailVerified,
		map[string]any{"user_id": uid.String()},
		auditlog.WriteContext{ActorUserID: &uid},
	)

	httpkit.OK(c, statusResponse{Status: "verified"})
}

// requestPasswordReset handles POST /auth/password-reset/request (contract §1 row 3).
//
// Enumeration protection (FR-08, §1.1): the response is byte-identical whether
// the email exists or not. Timing is equalized via TimingDummyVerify so both
// paths spend comparable wall-clock time.
func (h *Handler) requestPasswordReset(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, auditmodel.ErrValidationFailed)
		return
	}

	emailLower := strings.ToLower(req.Email)
	acc, err := h.accounts.ByEmail(c.Request.Context(), emailLower)
	if err != nil {
		httpkit.InternalError(c)
		return
	}

	// Timing equalization: always run a dummy argon2id compare regardless of
	// whether the address resolves to an account (FR-08, §1.1).
	h.hasher.TimingDummyVerify(req.Email)

	if acc == nil {
		// Unknown address: same 202, no mail, no token. Audit without user_id —
		// the absence is the internal signal that no account was found.
		_ = h.audit.Write(c.Request.Context(), auditmodel.EvtAuthPasswordResetRequested,
			map[string]any{"source_ip": c.ClientIP()},
			auditlog.WriteContext{},
		)
		httpkit.Data(c, http.StatusAccepted, statusResponse{Status: "sent"})
		return
	}

	// Issue 1-hour reset token and email it.
	rawToken, err := h.tokens.Issue(c.Request.Context(), acc.ID,
		auditmodel.LinkKindPasswordReset, time.Hour, nil)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "credentials: issue reset token", "error", err)
	} else if err := h.mailer.SendPasswordReset(c.Request.Context(), acc.Email, rawToken); err != nil {
		slog.ErrorContext(c.Request.Context(), "credentials: send reset email", "error", err)
	}

	uid := acc.ID
	_ = h.audit.Write(c.Request.Context(), auditmodel.EvtAuthPasswordResetRequested,
		map[string]any{
			"source_ip": c.ClientIP(),
			"user_id":   uid.String(),
		},
		auditlog.WriteContext{},
	)

	httpkit.Data(c, http.StatusAccepted, statusResponse{Status: "sent"})
}

// confirmPasswordReset handles POST /auth/password-reset/confirm (contract §1 row 4).
//
// On a rejected token: password unchanged and SessionIssuer.RevokeAll NOT called.
// On success: password updated, then RevokeAll called (FR-07, logout-everywhere).
func (h *Handler) confirmPasswordReset(c *gin.Context) {
	var req resetConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, auditmodel.ErrValidationFailed)
		return
	}

	// Validate new password BEFORE consuming the token so an invalid password
	// doesn't burn a valid token.
	if len(req.Password) < 10 {
		writeValidationError(c, []ValidationDetail{{Field: "password", Rule: "min_length"}})
		return
	}

	hibpCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if breached, err := h.hibp.IsBreached(hibpCtx, req.Password); err != nil {
		slog.WarnContext(c.Request.Context(), "credentials: HIBP unreachable on reset, skipping", "error", err)
	} else if breached {
		writeValidationError(c, []ValidationDetail{{Field: "password", Rule: "breached"}})
		return
	}

	lt, err := h.tokens.Consume(c.Request.Context(), auditmodel.LinkKindPasswordReset, req.Token)
	if err != nil {
		if errors.Is(err, authdomain.ErrLinkInvalid) {
			// TC-13: rejected token → password unchanged, RevokeAll NOT called.
			h.writeLinkRejected(c, auditmodel.LinkKindPasswordReset, err)
			httpkit.Error(c, auditmodel.ErrAuthLinkInvalid)
			return
		}
		httpkit.InternalError(c)
		return
	}

	hash, err := h.hasher.Hash(req.Password)
	if err != nil {
		httpkit.InternalError(c)
		return
	}

	if err := h.accounts.SetPasswordHash(c.Request.Context(), lt.UserID, hash); err != nil {
		httpkit.InternalError(c)
		return
	}

	// RevokeAll: logout-everywhere (FR-07). Non-fatal if it errors — the
	// password is already updated and the user can re-authenticate with the new one.
	if err := h.session.RevokeAll(c.Request.Context(), lt.UserID); err != nil {
		slog.ErrorContext(c.Request.Context(), "credentials: revoke all sessions after reset", "error", err)
	}

	uid := lt.UserID
	_ = h.audit.Write(c.Request.Context(), auditmodel.EvtAuthPasswordResetCompleted,
		map[string]any{
			"user_id":   uid.String(),
			"source_ip": c.ClientIP(),
		},
		auditlog.WriteContext{ActorUserID: &uid},
	)

	httpkit.OK(c, statusResponse{Status: "reset"})
}

// confirmOAuthLink handles POST /auth/oauth/link/confirm (contract §1 row 7).
// Consumes an oauth_link token, writes the oauth_identities row. Issues no session.
func (h *Handler) confirmOAuthLink(c *gin.Context) {
	var req tokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, auditmodel.ErrValidationFailed)
		return
	}

	lt, err := h.tokens.Consume(c.Request.Context(), auditmodel.LinkKindOauthLink, req.Token)
	if err != nil {
		if errors.Is(err, authdomain.ErrLinkInvalid) {
			h.writeLinkRejected(c, auditmodel.LinkKindOauthLink, err)
			httpkit.Error(c, auditmodel.ErrAuthLinkInvalid)
			return
		}
		httpkit.InternalError(c)
		return
	}

	// Payload was written by the OAuth callback with provider/subject/email.
	var payload struct {
		Provider auditmodel.LinkedProvider `json:"provider"`
		Subject  string                    `json:"subject"`
		Email    string                    `json:"email"`
	}
	if err := json.Unmarshal(lt.Payload, &payload); err != nil {
		slog.ErrorContext(c.Request.Context(), "credentials: decode oauth link payload", "error", err)
		httpkit.InternalError(c)
		return
	}

	if err := h.oauthIds.Link(c.Request.Context(), lt.UserID,
		payload.Provider, payload.Subject, payload.Email); err != nil {
		httpkit.InternalError(c)
		return
	}

	httpkit.OK(c, statusResponse{Status: "linked"})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// writeLinkRejected writes the auth.link.rejected audit event. Extracts the
// rejection reason from a *authdomain.LinkInvalidError if available.
func (h *Handler) writeLinkRejected(c *gin.Context, kind auditmodel.LinkKind, err error) {
	reason := auditmodel.LinkRejectReasonUnknown
	var lie *authdomain.LinkInvalidError
	if errors.As(err, &lie) {
		reason = lie.Reason
	}
	_ = h.audit.Write(c.Request.Context(), auditmodel.EvtAuthLinkRejected,
		map[string]any{
			"kind":      kind,
			"reason":    reason,
			"source_ip": c.ClientIP(),
		},
		auditlog.WriteContext{},
	)
}
