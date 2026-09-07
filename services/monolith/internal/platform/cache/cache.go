// Package cache wires the shared Redis client used for the JWT
// blacklist/session strategy, rate limiting and hot-read caching
// (CLAUDE.md "Stack"). Every domain package that touches Redis takes a
// *redis.Client from here rather than dialing its own connection.
package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"grindstats/services/monolith/internal/platform/config"
)

// Connect opens a Redis client and verifies it with a PING.
func Connect(ctx context.Context, cfg config.Redis) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("cache: ping: %w", err)
	}

	return client, nil
}
