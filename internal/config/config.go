// Package config handles API server configuration from env vars and config files.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all server configuration.
type Config struct {
	// Server
	Port    int    // HTTP port (default 8080)
	Host    string // Bind address (default "0.0.0.0")
	BaseURL string // Public URL for links (default "http://localhost:8080")

	// Database
	DatabaseURL    string // postgres:// or sqlite:// connection string
	DatabaseDriver string // "postgres" or "sqlite" (auto-detected from URL)

	// Auth
	JWTSecret        string // Secret for signing JWT tokens
	JWTExpiryMinutes int    // Access token lifetime (default 60)

	// Encryption
	EncryptionKey string // 32-byte hex key for encrypting API keys at rest

	// OAuth
	GitHubClientID     string // GitHub OAuth app client ID
	GitHubClientSecret string // GitHub OAuth app client secret
	GoogleClientID     string // Google OAuth client ID
	GoogleClientSecret string // Google OAuth client secret

	// agent-core settings
	DefaultModel    string // Default model for new agents
	DefaultProvider string // Default provider for new agents
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	cfg := &Config{
		Port:             envInt("PORT", 8080),
		Host:             envStr("HOST", "0.0.0.0"),
		BaseURL:          envStr("BASE_URL", "http://localhost:8080"),
		DatabaseURL:      envStr("DATABASE_URL", "sqlite://data/platform.db"),
		JWTSecret:        envStr("JWT_SECRET", ""),
		JWTExpiryMinutes: envInt("JWT_EXPIRY_MINUTES", 60),
		EncryptionKey:    envStr("ENCRYPTION_KEY", ""),
		GitHubClientID:     envStr("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: envStr("GITHUB_CLIENT_SECRET", ""),
		GoogleClientID:     envStr("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: envStr("GOOGLE_CLIENT_SECRET", ""),
		DefaultModel:       envStr("DEFAULT_MODEL", "gpt-4o"),
		DefaultProvider:    envStr("DEFAULT_PROVIDER", "openai"),
	}

	// Auto-detect driver from URL
	if len(cfg.DatabaseURL) >= 8 && cfg.DatabaseURL[:8] == "postgres" {
		cfg.DatabaseDriver = "postgres"
	} else {
		cfg.DatabaseDriver = "sqlite"
	}

	// Validate required fields
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
