// Package config loads runtime configuration from the environment (12-factor).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
	LogLevel    slog.Level
}

// Load reads configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:    getenv("HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(strings.TrimSpace(getenv("LOG_LEVEL", "info")))); err != nil {
		return Config{}, fmt.Errorf("parse LOG_LEVEL: %w", err)
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
