package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 8080 || cfg.DatabaseURL != "" {
		t.Fatalf("unexpected defaults: %+v", cfg)
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
