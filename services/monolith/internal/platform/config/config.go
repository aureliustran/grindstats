// Package config loads process configuration from environment variables.
package config

import (
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

// Config is the full set of settings the monolith reads from the environment.
type Config struct {
	Postgres       Postgres
	Redis          Redis
	ServerPort     string
	AllowedOrigins []string
}

// Load reads configuration from environment variables, applying the same
// defaults as docker-compose.yml so local `go run` works against `docker
// compose up` without extra setup.
func Load() Config {
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
