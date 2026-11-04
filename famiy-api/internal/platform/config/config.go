// Package config loads runtime configuration from the environment (12-factor).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
	LogLevel    slog.Level

	// Accounts and sessions (ADR-004).
	RegistrationEnabled bool
	MaxUsers            int
	SessionTTL          time.Duration
	CookieSecure        bool
	AllowedOrigins      []string       // extra browser origins allowed to make unsafe requests
	TrustedProxies      []netip.Prefix // peers whose X-Forwarded-For is believed
}

// Load reads configuration from environment variables. Unsafe-by-default
// settings are avoided: registration is CLOSED and cookies are Secure unless
// the environment says otherwise.
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

	var err error
	if cfg.RegistrationEnabled, err = getbool("REGISTRATION_ENABLED", false); err != nil {
		return Config{}, err
	}
	if cfg.CookieSecure, err = getbool("COOKIE_SECURE", true); err != nil {
		return Config{}, err
	}
	if cfg.MaxUsers, err = getint("MAX_USERS", 10); err != nil {
		return Config{}, err
	}
	if cfg.MaxUsers < 0 {
		return Config{}, errors.New("MAX_USERS must not be negative")
	}
	if cfg.SessionTTL, err = time.ParseDuration(getenv("SESSION_TTL", "720h")); err != nil {
		return Config{}, fmt.Errorf("parse SESSION_TTL: %w", err)
	}
	if cfg.SessionTTL <= 0 {
		return Config{}, errors.New("SESSION_TTL must be positive")
	}
	cfg.AllowedOrigins = splitList(os.Getenv("ALLOWED_ORIGINS"))
	for _, p := range splitList(os.Getenv("TRUSTED_PROXIES")) {
		prefix, perr := netip.ParsePrefix(p)
		if perr != nil {
			return Config{}, fmt.Errorf("parse TRUSTED_PROXIES entry %q (want CIDR, e.g. 10.0.0.0/8): %w", p, perr)
		}
		cfg.TrustedProxies = append(cfg.TrustedProxies, prefix)
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getbool(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}
	return b, nil
}

func getint(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return n, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
