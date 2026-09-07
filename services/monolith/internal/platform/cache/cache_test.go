package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"grindstats/services/monolith/internal/platform/config"
)

// requireClient skips unless GRINDSTATS_TEST_DB is set (docs/backend.md §7).
func requireClient(t *testing.T) (context.Context, *redis.Client) {
	t.Helper()
	if os.Getenv("GRINDSTATS_TEST_DB") == "" {
		t.Skip("set GRINDSTATS_TEST_DB=1 with `docker compose up -d redis` to run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	client, err := Connect(ctx, config.Load().Redis)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	return ctx, client
}

func TestConnect_PingsRunningRedis(t *testing.T) {
	ctx, client := requireClient(t)

	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestConnect_RoundTripsAKey(t *testing.T) {
	ctx, client := requireClient(t)

	key := "grindstats:test:" + t.Name()
	t.Cleanup(func() { client.Del(context.Background(), key) })

	if err := client.Set(ctx, key, "pong", time.Minute).Err(); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := client.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "pong" {
		t.Errorf("Get = %q, want %q", got, "pong")
	}
}

// The JWT blacklist entries expire with their token, so TTLs must survive the
// round trip (CLAUDE.md "Auth").
func TestConnect_KeyRetainsItsTTL(t *testing.T) {
	ctx, client := requireClient(t)

	key := "grindstats:test:" + t.Name()
	t.Cleanup(func() { client.Del(context.Background(), key) })

	if err := client.Set(ctx, key, "1", time.Minute).Err(); err != nil {
		t.Fatalf("Set: %v", err)
	}

	ttl, err := client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 || ttl > time.Minute {
		t.Errorf("TTL = %v, want (0, 1m]", ttl)
	}
}

func TestConnect_UnreachableHostReturnsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := config.Load().Redis
	cfg.Addr = "127.0.0.1:1" // nothing listens here

	client, err := Connect(ctx, cfg)
	if err == nil {
		client.Close()
		t.Fatal("Connect succeeded against an unreachable port, want error")
	}
}
