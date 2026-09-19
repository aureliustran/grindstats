package config

import (
	"encoding/hex"
	"testing"
)

func TestLoad_DefaultsMatchDockerCompose(t *testing.T) {
	for _, key := range []string{
		"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_USER", "POSTGRES_PASSWORD",
		"POSTGRES_DB", "POSTGRES_SSLMODE", "REDIS_HOST", "REDIS_PORT",
		"REDIS_PASSWORD", "REDIS_DB", "SERVER_PORT",
		"AUTH_JWT_PRIVATE_KEY_PATH", "AUTH_JWT_PUBLIC_KEYS_DIR",
		"AUTH_COOKIE_SECURE", "AUTH_HIBP_ENABLED",
		"AUTH_GOOGLE_CLIENT_ID", "AUTH_GOOGLE_CLIENT_SECRET",
		"AUTH_GOOGLE_REDIRECT_URL", "AUTH_GOOGLE_STATE_KEY",
		"AUTH_MAILER_MODE", "AUTH_BASE_URL",
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
	// Auth defaults
	if got, want := cfg.Auth.JWTPrivateKeyPath, ".local/jwt/signing.key"; got != want {
		t.Errorf("Auth.JWTPrivateKeyPath = %q, want %q", got, want)
	}
	if got, want := cfg.Auth.JWTPublicKeysDir, ".local/jwt/public"; got != want {
		t.Errorf("Auth.JWTPublicKeysDir = %q, want %q", got, want)
	}
	if !cfg.Auth.CookieSecure {
		t.Errorf("Auth.CookieSecure default = false, want true")
	}
	if !cfg.Auth.HIBPEnabled {
		t.Errorf("Auth.HIBPEnabled default = false, want true")
	}
	if got, want := cfg.Auth.MailerMode, "dev"; got != want {
		t.Errorf("Auth.MailerMode = %q, want %q", got, want)
	}
	if got, want := cfg.Auth.BaseURL, "http://localhost:5173"; got != want {
		t.Errorf("Auth.BaseURL = %q, want %q", got, want)
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

func TestLoad_AuthBoolFalse(t *testing.T) {
	t.Setenv("AUTH_COOKIE_SECURE", "false")
	t.Setenv("AUTH_HIBP_ENABLED", "0")

	cfg := Load()

	if cfg.Auth.CookieSecure {
		t.Errorf("Auth.CookieSecure = true, want false when AUTH_COOKIE_SECURE=false")
	}
	if cfg.Auth.HIBPEnabled {
		t.Errorf("Auth.HIBPEnabled = true, want false when AUTH_HIBP_ENABLED=0")
	}
}

func TestLoad_AuthBoolTrue(t *testing.T) {
	t.Setenv("AUTH_COOKIE_SECURE", "true")
	t.Setenv("AUTH_HIBP_ENABLED", "1")

	cfg := Load()

	if !cfg.Auth.CookieSecure {
		t.Errorf("Auth.CookieSecure = false, want true when AUTH_COOKIE_SECURE=true")
	}
	if !cfg.Auth.HIBPEnabled {
		t.Errorf("Auth.HIBPEnabled = false, want true when AUTH_HIBP_ENABLED=1")
	}
}

func TestLoad_GoogleStateKeyDecodedFromHex(t *testing.T) {
	const rawHex = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	t.Setenv("AUTH_GOOGLE_STATE_KEY", rawHex)

	cfg := Load()

	want, _ := hex.DecodeString(rawHex)
	if got := cfg.Auth.GoogleStateKey; string(got) != string(want) {
		t.Errorf("Auth.GoogleStateKey = %x, want %x", got, want)
	}
}

func TestLoad_GoogleStateKeyEmptyOnBadHex(t *testing.T) {
	t.Setenv("AUTH_GOOGLE_STATE_KEY", "not-hex")

	cfg := Load()

	// hex.DecodeString returns an error → the key is nil/empty. The application
	// will fail at startup when attempting to use a nil StateKey; this is the
	// intended behaviour (misconfiguration should surface at startup, not runtime).
	if len(cfg.Auth.GoogleStateKey) != 0 {
		t.Errorf("Auth.GoogleStateKey should be empty for invalid hex, got len=%d", len(cfg.Auth.GoogleStateKey))
	}
}
