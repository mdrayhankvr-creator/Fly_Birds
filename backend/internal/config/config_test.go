package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SIGNING_KEY_B64", "")
	t.Setenv("AUTH_COOKIE_SECURE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 8080 || cfg.DatabaseURL != "" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if !cfg.AuthCookieSecure {
		t.Fatal("cookies should default to Secure")
	}
}

func TestDatabaseAuthRequiresStrongSigningKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("JWT_SIGNING_KEY_B64", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing JWT key accepted")
	}
	t.Setenv("JWT_SIGNING_KEY_B64", base64.StdEncoding.EncodeToString([]byte("short")))
	if _, err := Load(); err == nil {
		t.Fatal("short JWT key accepted")
	}
	t.Setenv("JWT_SIGNING_KEY_B64", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32))))
	t.Setenv("AUTH_COOKIE_SECURE", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.JWTSigningKey) != 32 || cfg.AuthCookieSecure {
		t.Fatal("auth configuration not loaded")
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	for _, raw := range []string{"abc", "0", "65536"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("PORT", raw)
			if _, err := Load(); err == nil {
				t.Fatal("expected invalid PORT error")
			}
		})
	}
}
