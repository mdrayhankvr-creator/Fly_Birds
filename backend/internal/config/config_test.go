package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
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

func TestMigrationTimeout(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("MIGRATION_TIMEOUT", "")
	cfg, err := Load()
	if err != nil || cfg.MigrationTimeout != 5*time.Minute {
		t.Fatalf("default: %s %v", cfg.MigrationTimeout, err)
	}
	t.Setenv("MIGRATION_TIMEOUT", "15m")
	if cfg, err := Load(); err != nil || cfg.MigrationTimeout != 15*time.Minute {
		t.Fatalf("15m: %s %v", cfg.MigrationTimeout, err)
	}
	for _, bad := range []string{"soon", "5s"} {
		t.Setenv("MIGRATION_TIMEOUT", bad)
		if _, err := Load(); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
}

// TestEmailSettings covers the "forgot password" relay: off by default, and
// when it is on, enough of it must be right that the email can actually be
// sent and the link in it can point somewhere.
func TestEmailSettings(t *testing.T) {
	clear := func() {
		for _, name := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_FROM_NAME", "SMTP_SECURITY", "PUBLIC_BASE_URL"} {
			t.Setenv(name, "")
		}
	}

	clear()
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmailEnabled() {
		t.Fatal("email should be off unless SMTP_HOST is set")
	}

	// A complete relay, with the port and security defaulted.
	clear()
	t.Setenv("SMTP_HOST", "smtp-relay.brevo.com")
	t.Setenv("SMTP_USERNAME", "school@example.edu.bd")
	t.Setenv("SMTP_PASSWORD", "relay-key")
	t.Setenv("SMTP_FROM", "noreply@example.edu.bd")
	t.Setenv("PUBLIC_BASE_URL", "https://school.example.edu.bd")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.EmailEnabled() || cfg.SMTPPort != 587 || cfg.SMTPSecurity != "starttls" {
		t.Fatalf("unexpected relay defaults: %+v", cfg)
	}

	// The link has to point somewhere, so the public address is required.
	t.Setenv("PUBLIC_BASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("a relay with no PUBLIC_BASE_URL was accepted")
	}
	t.Setenv("PUBLIC_BASE_URL", "https://school.example.edu.bd")

	for name, set := range map[string]func(){
		"sender missing":        func() { t.Setenv("SMTP_FROM", "") },
		"sender not an address": func() { t.Setenv("SMTP_FROM", "noreply") },
		"port not a number":     func() { t.Setenv("SMTP_PORT", "smtp") },
		"port out of range":     func() { t.Setenv("SMTP_PORT", "70000") },
		"unknown security":      func() { t.Setenv("SMTP_SECURITY", "ssl") },
		"password with no user": func() { t.Setenv("SMTP_USERNAME", "") },
		// A relay password must never be sent in the clear.
		"password in the clear": func() { t.Setenv("SMTP_SECURITY", "none") },
	} {
		clear()
		t.Setenv("SMTP_HOST", "smtp-relay.brevo.com")
		t.Setenv("SMTP_USERNAME", "school@example.edu.bd")
		t.Setenv("SMTP_PASSWORD", "relay-key")
		t.Setenv("SMTP_FROM", "noreply@example.edu.bd")
		t.Setenv("PUBLIC_BASE_URL", "https://school.example.edu.bd")
		set()
		if _, err := Load(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	// Settings without a host are a half-finished configuration, not email
	// quietly left off.
	clear()
	t.Setenv("SMTP_FROM", "noreply@example.edu.bd")
	if _, err := Load(); err == nil {
		t.Fatal("SMTP_FROM with no SMTP_HOST was accepted")
	}
}
