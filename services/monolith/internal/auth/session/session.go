// Package session implements the session lifecycle for the auth domain:
// login, CSRF issuance, backoff, refresh with rotation and replay detection,
// logout, logout-all, and GET /users/me.
//
// Handler exports Register for be-wiring to mount routes, and also implements
// authdomain.SessionIssuer so the OAuth and credentials handlers can call
// Issue and RevokeAll at the wave-4 composition root without importing this
// package (the interface lives in authdomain).
//
// All minting, verification, session storage, and backoff arithmetic come from
// libs/authmw. Nothing here reimplements what that library already does.
package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/services/monolith/internal/auth/authdomain"
)

// Handler implements the HTTP routes for the session lifecycle and the
// authdomain.SessionIssuer interface.
type Handler struct {
	ks       *authmw.KeySet
	rdb      *redis.Client
	accounts authdomain.AccountRepo
	audit    auditlog.Writer
	cookies  authmw.CookieOptions
	clock    func() time.Time
	logger   *slog.Logger
}

// New constructs a Handler. logger may be nil (falls back to slog.Default).
func New(
	ks *authmw.KeySet,
	rdb *redis.Client,
	accounts authdomain.AccountRepo,
	audit auditlog.Writer,
	cookies authmw.CookieOptions,
	logger *slog.Logger,
) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		ks:       ks,
		rdb:      rdb,
		accounts: accounts,
		audit:    audit,
		cookies:  cookies,
		clock:    time.Now,
		logger:   logger,
	}
}

// WithClock replaces the clock used for LastSeenAt and idle-expiry checks.
// Only for tests that control time (TC-07, TC-08).
func (h *Handler) WithClock(fn func() time.Time) *Handler {
	h.clock = fn
	return h
}

// Register mounts the auth session routes onto r.
//
//	POST /auth/login       — unauthenticated; validates credentials, issues session
//	POST /auth/refresh     — refresh-cookie only; rotates token pair
//	POST /auth/logout      — access cookie + X-CSRF-Token; ends current session
//	POST /auth/logout-all  — access cookie + X-CSRF-Token; ends every session
//	GET  /users/me         — access cookie; returns profile + CSRF token
//
// Handlers for logout, logout-all, and /users/me read authmw.Claims from the
// request context. When deployed behind the gateway's Protected group the
// claims are placed there by middleware.Auth; in tests a lightweight
// in-line middleware sets them directly (see handler_test helpers).
func (h *Handler) Register(r gin.IRouter) {
	r.POST("/auth/login", h.handleLogin)
	r.POST("/auth/refresh", h.handleRefresh)
	r.POST("/auth/logout", h.handleLogout)
	r.POST("/auth/logout-all", h.handleLogoutAll)
	r.GET("/users/me", h.handleMe)
}

// ─── SessionIssuer ────────────────────────────────────────────────────────────

// Issue implements authdomain.SessionIssuer. It mints a fresh access+refresh
// pair, writes the session record in Redis, sets both HttpOnly cookies on w,
// and returns the session-bound CSRF token.
//
// Issue never decides whether the caller may log in — that is the caller's
// responsibility, which is what lets the OAuth callback and the local-
// credentials handler share one session-creation path (contract §5).
func (h *Handler) Issue(
	ctx context.Context,
	w http.ResponseWriter,
	a authdomain.Account,
	userAgent, sourceIP string,
) (string, error) {
	sid := uuid.New().String()
	now := h.clock()

	csrf, err := generateCSRF()
	if err != nil {
		return "", fmt.Errorf("session.Issue: generate CSRF: %w", err)
	}

	accessToken, accessJTI, err := h.ks.MintAccess(
		a.ID.String(), sid,
		string(a.Role),
		"free", // TODO: tier will be owned by the subscription domain
		a.IsVerified(),
	)
	if err != nil {
		return "", fmt.Errorf("session.Issue: mint access token: %w", err)
	}

	refreshToken, refreshJTI, err := h.ks.MintRefresh(a.ID.String(), sid)
	if err != nil {
		return "", fmt.Errorf("session.Issue: mint refresh token: %w", err)
	}

	deviceLabel := parseDeviceLabel(userAgent)
	sess := authmw.Session{
		SID:         sid,
		RefreshJTI:  refreshJTI,
		DeviceLabel: deviceLabel,
		CreatedAt:   now,
		LastSeenAt:  now,
		CSRFToken:   csrf,
	}
	if err := authmw.StoreSession(ctx, h.rdb, a.ID.String(), sess); err != nil {
		return "", fmt.Errorf("session.Issue: store session: %w", err)
	}

	authmw.SetSessionCookies(w, accessToken, refreshToken, h.cookies)

	actorID := a.ID
	_ = h.audit.Write(ctx, auditmodel.EvtAuthLoginSucceeded, map[string]any{
		"user_id":      a.ID.String(),
		"session_jti":  accessJTI,
		"source_ip":   sourceIP,
		"device_label": deviceLabel,
	}, auditlog.WriteContext{ActorUserID: &actorID})

	return csrf, nil
}

// RevokeAll implements authdomain.SessionIssuer. It advances the user's
// blacklist epoch (now+1), invalidating every outstanding token in O(1)
// without per-token blacklist writes (FR-33, contract §3). Called by
// logout-all, password reset, and admin force-logout.
func (h *Handler) RevokeAll(ctx context.Context, userID uuid.UUID) error {
	if err := authmw.SetEpoch(ctx, h.rdb, userID.String()); err != nil {
		return fmt.Errorf("session.RevokeAll: SetEpoch: %w", err)
	}
	actorID := userID
	_ = h.audit.Write(ctx, auditmodel.EvtAuthLogoutAll, map[string]any{
		"user_id": userID.String(),
	}, auditlog.WriteContext{ActorUserID: &actorID})
	return nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// generateCSRF produces 32 cryptographically random bytes encoded as
// base64url without padding (contract §2.3).
func generateCSRF() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("session: generateCSRF: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// parseDeviceLabel returns a display label from a User-Agent string.
// An empty UA yields "Unknown device" (contract TC-02 fallback).
func parseDeviceLabel(ua string) string {
	if ua == "" {
		return "Unknown device"
	}
	if len(ua) > 120 {
		return ua[:120]
	}
	return ua
}

// requestIDFrom reads the request ID stored by the gateway's RequestID
// middleware under key "gateway.request_id", or falls back to the raw
// X-Request-ID header. Returns "" when neither is present (e.g. unit tests
// that omit the RequestID middleware).
func requestIDFrom(c *gin.Context) string {
	if v, ok := c.Get("gateway.request_id"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return c.GetHeader("X-Request-ID")
}
