package config

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for _, k := range []string{"HTTP_ADDR", "LOG_LEVEL", "REGISTRATION_ENABLED", "COOKIE_SECURE", "MAX_USERS",
		"SESSION_TTL", "ALLOWED_ORIGINS", "TRUSTED_PROXIES"} {
		t.Setenv(k, "")
	}
	t.Setenv("DATABASE_URL", "postgres://x")
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func TestLoadDefaultsAreFailClosed(t *testing.T) {
	setEnv(t, nil)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RegistrationEnabled {
		t.Error("registration must default to closed")
	}
	if !cfg.CookieSecure {
		t.Error("cookies must default to Secure")
	}
	if cfg.MaxUsers != 10 || cfg.SessionTTL != 720*time.Hour {
		t.Errorf("MaxUsers=%d SessionTTL=%s", cfg.MaxUsers, cfg.SessionTTL)
	}
}

func TestLoadParsesValues(t *testing.T) {
	setEnv(t, map[string]string{
		"REGISTRATION_ENABLED": "true", "COOKIE_SECURE": "false", "MAX_USERS": "3", "SESSION_TTL": "2h",
		"ALLOWED_ORIGINS": " http://localhost:5173 , https://a.example ", "TRUSTED_PROXIES": "10.0.0.0/8, 172.16.0.0/12",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RegistrationEnabled || cfg.CookieSecure || cfg.MaxUsers != 3 || cfg.SessionTTL != 2*time.Hour {
		t.Errorf("unexpected: %+v", cfg)
	}
	if len(cfg.AllowedOrigins) != 2 || cfg.AllowedOrigins[0] != "http://localhost:5173" {
		t.Errorf("origins = %v", cfg.AllowedOrigins)
	}
	if len(cfg.TrustedProxies) != 2 {
		t.Errorf("proxies = %v", cfg.TrustedProxies)
	}
}

func TestAllowedOriginsAreNormalised(t *testing.T) {
	setEnv(t, map[string]string{"ALLOWED_ORIGINS": "HTTP://Localhost:5173/, https://App.Example.com"})
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"http://localhost:5173", "https://app.example.com"}
	if len(cfg.AllowedOrigins) != 2 || cfg.AllowedOrigins[0] != want[0] || cfg.AllowedOrigins[1] != want[1] {
		t.Fatalf("origins = %v, want %v", cfg.AllowedOrigins, want)
	}
}

func TestWarnings(t *testing.T) {
	has := func(ws []string, sub string) bool {
		for _, w := range ws {
			if strings.Contains(w, sub) {
				return true
			}
		}
		return false
	}
	lockedDown := Config{CookieSecure: true, TrustedProxies: []netip.Prefix{netip.MustParsePrefix("172.16.0.0/12")}}
	if w := lockedDown.Warnings(); len(w) != 0 {
		t.Errorf("a locked-down config should not warn, got %v", w)
	}
	if w := (Config{CookieSecure: false}).Warnings(); !has(w, "COOKIE_SECURE=false") {
		t.Errorf("no warning for insecure cookies: %v", w)
	}
	if w := (Config{CookieSecure: true, RegistrationEnabled: true, MaxUsers: 7}).Warnings(); !has(w, "REGISTRATION_ENABLED=true") || !has(w, "MAX_USERS=7") {
		t.Errorf("no warning for open registration: %v", w)
	}
	if w := (Config{CookieSecure: true}).Warnings(); !has(w, "TRUSTED_PROXIES is empty") {
		t.Errorf("no warning for empty TRUSTED_PROXIES behind HTTPS: %v", w)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	tests := map[string]map[string]string{
		"bad bool":              {"REGISTRATION_ENABLED": "maybe"},
		"bad int":               {"MAX_USERS": "ten"},
		"negative cap":          {"MAX_USERS": "-1"},
		"bad duration":          {"SESSION_TTL": "soon"},
		"zero duration":         {"SESSION_TTL": "0s"},
		"bad proxy":             {"TRUSTED_PROXIES": "not-a-cidr"},
		"missing db url":        {"DATABASE_URL_UNSET": "1"},
		"bad log level":         {"LOG_LEVEL": "loud"},
		"bare ip not cidr":      {"TRUSTED_PROXIES": "10.0.0.1"},
		"origin without scheme": {"ALLOWED_ORIGINS": "localhost:5173"},
		"origin with path":      {"ALLOWED_ORIGINS": "https://app.example.com/app"},
		"origin wrong scheme":   {"ALLOWED_ORIGINS": "ftp://app.example.com"},
	}
	for name, env := range tests {
		t.Run(name, func(t *testing.T) {
			setEnv(t, env)
			if _, ok := env["DATABASE_URL_UNSET"]; ok {
				t.Setenv("DATABASE_URL", "")
			}
			if _, err := Load(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
