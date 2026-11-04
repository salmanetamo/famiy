package config

import (
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

func TestLoadRejectsBadValues(t *testing.T) {
	tests := map[string]map[string]string{
		"bad bool":         {"REGISTRATION_ENABLED": "maybe"},
		"bad int":          {"MAX_USERS": "ten"},
		"negative cap":     {"MAX_USERS": "-1"},
		"bad duration":     {"SESSION_TTL": "soon"},
		"zero duration":    {"SESSION_TTL": "0s"},
		"bad proxy":        {"TRUSTED_PROXIES": "not-a-cidr"},
		"missing db url":   {"DATABASE_URL_UNSET": "1"},
		"bad log level":    {"LOG_LEVEL": "loud"},
		"bare ip not cidr": {"TRUSTED_PROXIES": "10.0.0.1"},
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
