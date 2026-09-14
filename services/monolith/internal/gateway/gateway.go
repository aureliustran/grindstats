// Package gateway assembles the single middleware chain and route tree
// every request in the monolith passes through (docs/backend.md §2). Domain
// packages register their routes onto the returned *Server; none of them
// build their own gin.Engine or repeat this chain.
package gateway

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/services/monolith/internal/gateway/middleware"
)

// Deps is what the gateway needs to assemble the chain. Fields introduced by
// the auth epic (KeySet, Redis, AuditLog) are optional: when nil the
// corresponding stage is skipped. This allows GATE-001-era tests and the
// health handler to keep using New(Deps{}) without an auth setup.
//
// In production all three must be non-nil; the composition root (be-wiring,
// wave 4) is responsible for providing them.
type Deps struct {
	Logger         *slog.Logger
	AllowedOrigins []string

	// Auth epic — wave 3. All three are required together; providing a
	// partial set panics to catch misconfiguration at startup.
	KeySet   *authmw.KeySet  // RS256 key material for JWT verification
	Redis    *redis.Client   // Redis client for blacklist, epoch, CSRF, rate limit
	AuditLog auditlog.Writer // audit writer; nil only in tests
}

// Server is the assembled router plus the seams domain packages register onto.
//
//   - V1 is the /api/v1 base group. Public (unauthenticated) endpoints are
//     registered here; the rate-limit stage runs for all V1 routes.
//   - Protected is a V1 sub-group with the full auth chain applied
//     (auth → CSRF → verified-write). Authenticated endpoints are registered
//     here. When Deps.KeySet is nil (tests without auth), Protected == V1.
type Server struct {
	Engine    *gin.Engine
	V1        gin.IRouter // public base group; rate-limited
	Protected gin.IRouter // authenticated sub-group; auth+CSRF+verified-write
}

// New assembles the gateway's middleware chain in the order every later
// story must respect:
//
//	recovery → request ID → logging → CORS → locale →
//	rate limit → auth → CSRF → verified-write → [role] → handler
//
// The [role] stage is not applied globally; it is applied per-route-group by
// be-wiring (wave 4) via middleware.RequireRole. The five stages above it are
// applied here. Public route groups (unauthenticated auth endpoints) use
// server.V1; authenticated endpoints use server.Protected.
func New(deps Deps) *Server {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}

	// Panic if a partial auth setup is provided — all three must come together.
	authProvided := deps.KeySet != nil || deps.Redis != nil || deps.AuditLog != nil
	authComplete := deps.KeySet != nil && deps.Redis != nil
	if authProvided && !authComplete {
		panic("gateway.New: KeySet and Redis must both be provided (or both nil)")
	}

	engine := gin.New()

	// === Stages shared by every request (GATE-001, frozen) ===
	engine.Use(
		middleware.Recovery(deps.Logger),
		middleware.RequestID(),
		middleware.Logging(deps.Logger),
		middleware.CORS(deps.AllowedOrigins),
		middleware.Locale(),
	)

	// === Versioned API group (/api/v1) ===
	v1 := engine.Group("/api/v1")

	// Rate limit applies to all API routes; the specific limits are keyed per
	// endpoint path (contract §6, NFR-04).
	if deps.Redis != nil {
		v1.Use(middleware.RateLimit(deps.Redis, deps.Logger))
	}

	// === Protected sub-group: auth + CSRF + verified-write ===
	// Public routes (register, login, verify-email, etc.) are registered on
	// server.V1 by be-wiring. Authenticated routes go on server.Protected.
	//
	// This is the architectural opt-out mechanism: routes in a group without
	// the auth stage never pass through it, so no middleware path-list is
	// needed (contract §6: "public routes opt out by route group").
	protected := v1.Group("")
	if authComplete {
		protected.Use(
			middleware.Auth(deps.KeySet, deps.Redis, deps.AuditLog, deps.Logger),
			middleware.CSRF(deps.Redis, deps.Logger),
			middleware.VerifiedWrite(deps.AuditLog, deps.Logger),
		)
	}

	return &Server{
		Engine:    engine,
		V1:        v1,
		Protected: protected,
	}
}

// RequireRole is a convenience shim so be-wiring can apply the role check
// to a route group without importing the middleware package directly. This
// keeps the middleware package as the canonical home for the implementation
// while the gateway package owns the composition surface.
func RequireRole(
	required auditmodel.Role,
	log auditlog.Writer,
	logger *slog.Logger,
) gin.HandlerFunc {
	return middleware.RequireRole(required, log, logger)
}
