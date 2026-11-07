package config_test

import (
	"strings"
	"testing"

	"github.com/IlliaVern/go-todo-app/internal/platform/config"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DB_PASSWORD", "pw")
	t.Setenv("AUTH_SIGNING_KEY", "key")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := config.Config{
		Server:                config.Server{Port: "8000"},
		LogLevel:              "info",
		SigningKey:            "key",
		AuthRequestsPerMinute: 20,
		DB: config.DB{
			Host: "localhost", Port: "5436", Username: "postgres", Password: "pw", Name: "postgres", SSLMode: "disable",
		},
	}
	if cfg != want {
		t.Errorf("cfg = %+v\nwant  %+v", cfg, want)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("DB_PASSWORD", "pw")
	t.Setenv("AUTH_SIGNING_KEY", "key")
	t.Setenv("PORT", "9000")
	t.Setenv("DB_HOST", "postgres")
	t.Setenv("DB_PORT", "5432")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "9000" || cfg.DB.Host != "postgres" || cfg.DB.Port != "5432" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestLoadRequiresSecrets(t *testing.T) {
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("AUTH_SIGNING_KEY", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected an error for empty secrets")
	}
	for _, name := range []string{"DB_PASSWORD", "AUTH_SIGNING_KEY"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not mention %s", err, name)
		}
	}
}

func TestLoadRejectsRateLimitBelowOne(t *testing.T) {
	t.Setenv("DB_PASSWORD", "pw")
	t.Setenv("AUTH_SIGNING_KEY", "key")
	for _, v := range []string{"0", "-5"} {
		t.Setenv("AUTH_RATE_LIMIT_PER_MINUTE", v)
		if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "AUTH_RATE_LIMIT_PER_MINUTE") {
			t.Errorf("value %s: err = %v, want a rate limit error", v, err)
		}
	}
}
