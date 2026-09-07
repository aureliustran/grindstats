// Package health implements the gateway's two probes (GATE-001): liveness,
// which answers no other question than "is the process running", and
// readiness, which checks each dependency under its own timeout and
// classifies failures as required (not-ready) or optional (degraded but
// ready) per SRS-AUTH-001 NFR-02.
package health

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"grindstats/libs/httpkit"
	"grindstats/services/monolith/internal/gateway/middleware"
)

// defaultTimeout bounds a Checker call when Dependency.Timeout is unset, so
// a hung dependency can never hang the probe (docs/stories/GATE-001 AC:
// "a hung dependency does not hang the probe").
const defaultTimeout = 2 * time.Second

// Checker reports whether a dependency is reachable. It must respect ctx's
// deadline — a Checker that ignores cancellation defeats the per-check
// timeout Handler enforces around it.
type Checker func(ctx context.Context) error

// Dependency is one thing readiness depends on. Required=false is the
// NFR-02 case: Redis unreachable degrades the response but does not pull
// the instance from the load balancer.
type Dependency struct {
	Name     string
	Check    Checker
	Required bool
	Timeout  time.Duration
}

func (d Dependency) timeout() time.Duration {
	if d.Timeout > 0 {
		return d.Timeout
	}
	return defaultTimeout
}

// Handler serves /healthz and /readyz.
type Handler struct {
	Dependencies []Dependency
	Logger       *slog.Logger
}

func (h Handler) logger() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

// RegisterRoutes mounts both probes. They are deliberately outside any
// version group and require no credential: their callers are a container
// runtime and a load balancer, neither of which can hold one.
func (h Handler) RegisterRoutes(r gin.IRouter) {
	r.GET("/healthz", h.Liveness)
	r.GET("/readyz", h.Readiness)
}

// Liveness answers only "is this process running" and consults no
// dependency. A liveness check that touches Postgres turns a brief outage
// into the orchestrator killing and restarting healthy containers, which is
// the failure this split exists to prevent.
func (h Handler) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "alive"})
}

type checkResult struct {
	dep Dependency
	err error
}

// Readiness runs every dependency check concurrently — sequentially, the
// probe's worst case would be the sum of every timeout rather than the
// largest one — and classifies the outcome: any required failure is
// not-ready (503), an optional failure alone is ready-but-degraded (200).
// No response body names which dependency failed; that detail is logged.
func (h Handler) Readiness(c *gin.Context) {
	ctx := c.Request.Context()
	results := make([]checkResult, len(h.Dependencies))

	var wg sync.WaitGroup
	for i, dep := range h.Dependencies {
		wg.Add(1)
		go func(i int, dep Dependency) {
			defer wg.Done()
			checkCtx, cancel := context.WithTimeout(ctx, dep.timeout())
			defer cancel()
			results[i] = checkResult{dep: dep, err: dep.Check(checkCtx)}
		}(i, dep)
	}
	wg.Wait()

	degraded := false
	for _, res := range results {
		if res.err == nil {
			continue
		}
		if res.dep.Required {
			h.logger().Error("readiness check failed",
				"request_id", middleware.RequestIDFrom(c),
				"dependency", res.dep.Name,
				"error", res.err,
			)
			httpkit.ServiceUnavailable(c)
			return
		}
		degraded = true
		h.logger().Warn("readiness check degraded",
			"request_id", middleware.RequestIDFrom(c),
			"dependency", res.dep.Name,
			"error", res.err,
		)
	}

	status := "ready"
	if degraded {
		status = "degraded"
	}
	c.JSON(http.StatusOK, gin.H{"status": status})
}
