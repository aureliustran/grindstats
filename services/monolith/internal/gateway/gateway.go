// Package gateway assembles the single middleware chain and route tree
// every request in the monolith passes through (docs/backend.md §2). Domain
// packages register their routes onto the returned *Server; none of them
// build their own gin.Engine or repeat this chain.
package gateway

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"grindstats/services/monolith/internal/gateway/middleware"
)

// Deps is what the gateway needs to assemble the chain. Adding a dependency
// here (e.g. a rate limiter's Redis client) is how a later story plugs into
// a reserved stage without changing this file's shape.
type Deps struct {
	Logger         *slog.Logger
	AllowedOrigins []string
}

// Server is the assembled router plus the one durable seam every domain
// registers onto: V1 is the versioned API group (docs/backend.md §5),
// created empty by this story and populated by the first story that adds a
// real endpoint (AUTH-002).
type Server struct {
	Engine *gin.Engine
	V1     gin.IRouter
}

// New assembles the gateway's middleware chain in the order every later
// story must respect:
//
//	recovery -> request ID -> logging -> CORS -> locale ->
//	[rate limit] -> [auth] -> [CSRF] -> [role] -> handler
//
// The bracketed stages are not built here. FR-20/21 (auth), FR-22 (role),
// FR-24 (CSRF) and NFR-04 (rate limiting) belong to the auth stories that
// specify them (docs/backend.md §1) — this function only reserves their
// position, because retrofitting stage order later is the expensive
// mistake, not adding a stage.
func New(deps Deps) *Server {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}

	engine := gin.New()
	engine.Use(
		middleware.Recovery(deps.Logger),
		middleware.RequestID(),
		middleware.Logging(deps.Logger),
		middleware.CORS(deps.AllowedOrigins),
		middleware.Locale(),
		// Reserved, in this order, for stories that specify them:
		//   rate limiting  (NFR-04)
		//   authentication (FR-20, FR-21)
		//   CSRF           (FR-24)
		//   role check     (FR-22)
	)

	return &Server{
		Engine: engine,
		V1:     engine.Group("/api/v1"),
	}
}
