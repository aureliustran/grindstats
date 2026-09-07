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

	srv := gateway.New(gateway.Deps{
		Logger:         logger,
		AllowedOrigins: cfg.AllowedOrigins,
	})

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

	// srv.V1 ("/api/v1") is created empty here, awaiting AUTH-002's routes.

	addr := ":" + cfg.ServerPort
	logger.Info("listening", "addr", addr)
	if err := srv.Engine.Run(addr); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}
