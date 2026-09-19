// Package config loads process configuration from environment variables.
package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// Postgres holds connection settings for the shared Postgres instance.
// One instance, one schema per domain (docs/backend.md §3) — this is the
// single connection every domain package pools through.
type Postgres struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
	SSLMode  string
}

// DSN returns a libpq-style connection string for pgx.
func (p Postgres) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		p.Host, p.Port, p.User, p.Password, p.Database, p.SSLMode,
	)
}

// Redis holds connection settings for the shared Redis instance.
type Redis struct {
	Addr     string
	Password string
	DB       int
}

// Auth holds all settings required by the auth domain (AUTH-001, AUTH-002,
// AUTH-003). Values correspond to the AUTH_* environment variables.
type Auth struct {
	// JWTPrivateKeyPath is the path to the RS256 signing key (PEM, PKCS#1 or
	// PKCS#8). AUTH_JWT_PRIVATE_KEY_PATH.
	JWTPrivateKeyPath string
	// JWTPublicKeysDir is a directory whose *.pub files are the verifying
	// public keys (each named <kid>.pub, PEM PKIX). AUTH_JWT_PUBLIC_KEYS_DIR.
	JWTPublicKeysDir string
	// CookieSecure controls the Secure attribute on session cookies.
	// Set false via AUTH_COOKIE_SECURE=false for plain-http local dev.
	CookieSecure bool
	// HIBPEnabled enables the Have I Been Pwned k-anonymity password check
	// (D3). AUTH_HIBP_ENABLED=false disables it in local dev or tests.
	HIBPEnabled bool
	// GoogleClientID / GoogleClientSecret / GoogleRedirectURL are the Google
	// OAuth2 application credentials (AUTH_GOOGLE_CLIENT_ID, etc.).
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	// GoogleStateKey is a 32-byte hex-encoded HMAC key for signing the OAuth
	// state cookie (AUTH_GOOGLE_STATE_KEY). Treat as a secret.
	GoogleStateKey []byte
	// MailerMode selects the mailer implementation: "dev" logs links to the
	// structured log; "noop" drops every send silently. AUTH_MAILER_MODE.
	MailerMode string
	// BaseURL is the public base URL used when composing emailed deep-links
	// (e.g. "http://localhost:5173" in local dev). AUTH_BASE_URL.
	BaseURL string
}

// Config is the full set of settings the monolith reads from the environment.
type Config struct {
	Postgres       Postgres
	Redis          Redis
	Auth           Auth
	ServerPort     string
	AllowedOrigins []string
}

// Load reads configuration from environment variables, applying the same
// defaults as docker-compose.yml so local `go run` works against `docker
// compose up` without extra setup.
func Load() Config {
	stateKeyHex := getEnv("AUTH_GOOGLE_STATE_KEY", "")
	stateKey, _ := hex.DecodeString(stateKeyHex)

	return Config{
		Postgres: Postgres{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnv("POSTGRES_PORT", "5433"),
			User:     getEnv("POSTGRES_USER", "grindstats"),
			Password: getEnv("POSTGRES_PASSWORD", "grindstats"),
			Database: getEnv("POSTGRES_DB", "grindstats"),
			SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
		},
		Redis: Redis{
			Addr:     fmt.Sprintf("%s:%s", getEnv("REDIS_HOST", "localhost"), getEnv("REDIS_PORT", "6379")),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Auth: Auth{
			JWTPrivateKeyPath:  getEnv("AUTH_JWT_PRIVATE_KEY_PATH", ".local/jwt/signing.key"),
			JWTPublicKeysDir:   getEnv("AUTH_JWT_PUBLIC_KEYS_DIR", ".local/jwt/public"),
			CookieSecure:       getEnvBool("AUTH_COOKIE_SECURE", true),
			HIBPEnabled:        getEnvBool("AUTH_HIBP_ENABLED", true),
			GoogleClientID:     getEnv("AUTH_GOOGLE_CLIENT_ID", ""),
			GoogleClientSecret: getEnv("AUTH_GOOGLE_CLIENT_SECRET", ""),
			GoogleRedirectURL:  getEnv("AUTH_GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/auth/oauth/google/callback"),
			GoogleStateKey:     stateKey,
			MailerMode:         getEnv("AUTH_MAILER_MODE", "dev"),
			BaseURL:            getEnv("AUTH_BASE_URL", "http://localhost:5173"),
		},
		ServerPort:     getEnv("SERVER_PORT", "8080"),
		AllowedOrigins: getEnvList("CORS_ALLOWED_ORIGINS", "http://localhost:5173"),
	}
}

func getEnvList(key, fallback string) []string {
	raw := getEnv(key, fallback)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return fallback
	}
	return n
}

func getEnvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	}
	return fallback
}
