// Command server is the single Gin binary that will host every domain
// package until the roadmap reaches Phase 10 (docs/backend.md §1).
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/auditlog"
	"grindstats/libs/authmw"
	"grindstats/services/monolith/internal/auth/credentials"
	"grindstats/services/monolith/internal/auth/hibp"
	"grindstats/services/monolith/internal/auth/mailer"
	"grindstats/services/monolith/internal/auth/oauth"
	"grindstats/services/monolith/internal/auth/session"
	"grindstats/services/monolith/internal/auth/store"
	"grindstats/services/monolith/internal/gateway"
	"grindstats/services/monolith/internal/gateway/health"
	"grindstats/services/monolith/internal/platform/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()
	ctx := context.Background()

	// pgxpool.New and redis.NewClient are both lazy: neither dials here, so
	// the process starts and serves /healthz and /readyz even when a
	// dependency is down at boot (docs/stories/GATE-001 AC: "the server
	// starts when a dependency is unavailable at boot"). Only a malformed
	// DSN fails at this point, which is a real config bug worth exiting on.
	pool, err := pgxpool.New(ctx, cfg.Postgres.DSN())
	if err != nil {
		logger.Error("build postgres pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	// ── Auth infrastructure ───────────────────────────────────────────────────

	// Load RS256 key material from AUTH_JWT_PRIVATE_KEY (a PEM value, not a
	// file path — see config.Auth.JWTPrivateKeyPEM). A startup failure here is
	// intentional: no key means no token can be minted or verified.
	keySet, err := authmw.LoadKeySetFromPEM(cfg.Auth.JWTPrivateKeyPEM, cfg.Auth.JWTPreviousPublicKeysPEM...)
	if err != nil {
		logger.Error("load JWT key set", "error", err)
		os.Exit(1)
	}

	// Audit writer — the real PgxWriter writes to audit.events in Postgres.
	auditWriter := auditlog.New(pool, logger)

	// ── Auth repositories (store layer) ──────────────────────────────────────

	accountStore := store.NewAccountStore(pool)
	oauthIdentityStore := store.NewOAuthIdentityStore(pool)
	linkTokenStore := store.NewLinkTokenStore(pool)

	// ── Gateway assembly ──────────────────────────────────────────────────────

	srv := gateway.New(gateway.Deps{
		Logger:         logger,
		AllowedOrigins: cfg.AllowedOrigins,
		KeySet:         keySet,
		Redis:          redisClient,
		AuditLog:       auditWriter,
	})

	// ── Health routes (GATE-001, unchanged) ───────────────────────────────────

	healthHandler := health.Handler{
		Logger: logger,
		Dependencies: []health.Dependency{
			{
				Name:     "postgres",
				Required: true,
				Timeout:  2 * time.Second,
				Check:    func(ctx context.Context) error { return pool.Ping(ctx) },
			},
			{
				Name:     "redis",
				Required: false, // NFR-02: fail open on read-only paths when Redis is down
				Timeout:  2 * time.Second,
				Check:    func(ctx context.Context) error { return redisClient.Ping(ctx).Err() },
			},
		},
	}
	healthHandler.RegisterRoutes(srv.Engine)

	// ── Auth domain handlers ──────────────────────────────────────────────────

	// Password hasher (argon2id, OWASP defaults — SEC-01). Shared by session
	// (login) and credentials (register, reset) — one hasher, one dummy hash.
	hasher, err := credentials.NewPasswordHasher(credentials.DefaultArgon2Params)
	if err != nil {
		logger.Error("init password hasher", "error", err)
		os.Exit(1)
	}

	// Session handler: implements the auth session lifecycle AND
	// authdomain.SessionIssuer (used by credentials and oauth handlers below).
	cookieOpts := authmw.CookieOptions{Secure: cfg.Auth.CookieSecure}
	sessionHandler := session.New(keySet, redisClient, accountStore, auditWriter, cookieOpts, hasher, logger)

	// HIBP password-breach check (D3). Fail-open on network errors.
	hibpClient := hibp.New(cfg.Auth.HIBPEnabled, 2*time.Second)

	// Mailer: "dev" logs deep-links to the structured log (local dev only).
	// "noop" or any other value → no-op (production would substitute a real
	// SMTP or SES implementation in a future story).
	var m mailer.Mailer
	if cfg.Auth.MailerMode == "dev" {
		m = mailer.NewDev(logger, cfg.Auth.BaseURL)
	} else {
		m = &noopMailer{}
	}

	// Credentials handler (register, verify-email, password-reset/*, oauth/link/confirm).
	credHandler := credentials.New(
		accountStore,
		oauthIdentityStore,
		linkTokenStore,
		sessionHandler,
		hasher,
		hibpClient,
		m,
		auditWriter,
	)

	// AccountProvisioner for the OAuth flow — implemented by credentials.Provisioner,
	// shared with the OAuth handler without the two packages importing each other.
	provisioner := credentials.NewProvisioner(accountStore, oauthIdentityStore, auditWriter)

	// OAuth handler (Google authorize + callback, PKCE, state-cookie HMAC).
	oauthHandler := oauth.New(
		oauth.Config{
			ClientID:     cfg.Auth.GoogleClientID,
			ClientSecret: cfg.Auth.GoogleClientSecret,
			RedirectURL:  cfg.Auth.GoogleRedirectURL,
			StateKey:     cfg.Auth.GoogleStateKey,
			CookieSecure: cfg.Auth.CookieSecure,
		},
		provisioner,
		accountStore,
		linkTokenStore,
		sessionHandler,
		m,
		auditWriter,
	)

	// ── Route registration ────────────────────────────────────────────────────
	//
	// Public (unauthenticated) endpoints are registered on srv.V1; the auth
	// middleware stage does NOT run for routes in this group. Authenticated
	// endpoints (logout, logout-all, /users/me) go on srv.Protected, which
	// applies auth+CSRF+verified-write. session.Handler exposes RegisterPublic
	// and RegisterProtected for exactly this split (AMD-005, resolved).
	credHandler.Register(srv.V1)
	oauthHandler.Register(srv.V1)
	sessionHandler.RegisterPublic(srv.V1)
	sessionHandler.RegisterProtected(srv.Protected)

	addr := ":" + cfg.ServerPort
	logger.Info("listening", "addr", addr)
	if err := srv.Engine.Run(addr); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}

// noopMailer silently drops every send. Used when AUTH_MAILER_MODE is not
// "dev". Production email delivery is out of scope for AUTH-001 run 1.
type noopMailer struct{}

func (*noopMailer) SendVerification(_ context.Context, _, _ string) error  { return nil }
func (*noopMailer) SendPasswordReset(_ context.Context, _, _ string) error { return nil }
func (*noopMailer) SendOAuthLink(_ context.Context, _, _ string) error     { return nil }
