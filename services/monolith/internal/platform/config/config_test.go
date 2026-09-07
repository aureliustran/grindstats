package config

import "testing"

func TestLoad_DefaultsMatchDockerCompose(t *testing.T) {
	for _, key := range []string{
		"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_USER", "POSTGRES_PASSWORD",
		"POSTGRES_DB", "POSTGRES_SSLMODE", "REDIS_HOST", "REDIS_PORT",
		"REDIS_PASSWORD", "REDIS_DB", "SERVER_PORT",
	} {
		t.Setenv(key, "")
	}

	cfg := Load()

	if got, want := cfg.Postgres.DSN(), "host=localhost port=5433 user=grindstats password=grindstats dbname=grindstats sslmode=disable"; got != want {
		t.Errorf("DSN = %q, want %q", got, want)
	}
	if got, want := cfg.Redis.Addr, "localhost:6379"; got != want {
		t.Errorf("Redis.Addr = %q, want %q", got, want)
	}
	if got, want := cfg.ServerPort, "8080"; got != want {
		t.Errorf("ServerPort = %q, want %q", got, want)
	}
}

func TestLoad_EnvironmentOverridesDefaults(t *testing.T) {
	t.Setenv("POSTGRES_HOST", "postgres")
	t.Setenv("POSTGRES_PORT", "6543")
	t.Setenv("POSTGRES_SSLMODE", "require")
	t.Setenv("REDIS_HOST", "redis")
	t.Setenv("REDIS_DB", "3")

	cfg := Load()

	if got, want := cfg.Postgres.DSN(), "host=postgres port=6543 user=grindstats password=grindstats dbname=grindstats sslmode=require"; got != want {
		t.Errorf("DSN = %q, want %q", got, want)
	}
	if got, want := cfg.Redis.Addr, "redis:6379"; got != want {
		t.Errorf("Redis.Addr = %q, want %q", got, want)
	}
	if got, want := cfg.Redis.DB, 3; got != want {
		t.Errorf("Redis.DB = %d, want %d", got, want)
	}
}

func TestLoad_NonNumericRedisDBFallsBackToZero(t *testing.T) {
	t.Setenv("REDIS_DB", "not-a-number")

	if got := Load().Redis.DB; got != 0 {
		t.Errorf("Redis.DB = %d, want 0", got)
	}
}
