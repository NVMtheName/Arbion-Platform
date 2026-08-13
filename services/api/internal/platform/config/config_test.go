package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func validEnvironment() map[string]string {
	return map[string]string{"ARBION_ENV": "development", "DATABASE_URL": "postgres://user:pass@localhost/db?sslmode=disable", "REDIS_URL": "redis://localhost:6379/0", "CREDENTIAL_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)), "AI_INTERNAL_SERVICE_TOKEN": "test-internal-token"}
}
func load(values map[string]string) (Config, error) {
	return LoadFrom(func(k string) (string, bool) { v, ok := values[k]; return v, ok })
}
func TestLoadValidConfiguration(t *testing.T) {
	cfg, err := load(validEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.MaxConnections != 10 {
		t.Fatalf("unexpected pool size: %d", cfg.Database.MaxConnections)
	}
}
func TestLoadRejectsProductionWithoutTLS(t *testing.T) {
	values := validEnvironment()
	values["ARBION_ENV"] = "production"
	_, err := load(values)
	if err == nil || !strings.Contains(err.Error(), "TLS") {
		t.Fatalf("expected TLS error, got %v", err)
	}
}

func validProduction() map[string]string {
	values := validEnvironment()
	values["ARBION_ENV"] = "production"
	values["DATABASE_URL"] = "postgres://arbion:strong-password@postgres:5432/arbion?sslmode=disable"
	values["CREDENTIAL_ENCRYPTION_KEY"] = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	values["AI_INTERNAL_SERVICE_TOKEN"] = "0123456789abcdef0123456789abcdef"
	values["AUTH_ALLOWED_ORIGINS"] = "https://www.arbion.ai"
	return values
}

func TestCookieSecurityFollowsEnvironment(t *testing.T) {
	development, err := load(validEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	production, err := load(validProduction())
	if err != nil {
		t.Fatal(err)
	}
	if development.Auth.CookieSecure || !production.Auth.CookieSecure {
		t.Fatalf("cookie secure flags: development=%v production=%v", development.Auth.CookieSecure, production.Auth.CookieSecure)
	}
}

func TestProductionRejectsUnsafeSecretsAndOrigins(t *testing.T) {
	for name, mutate := range map[string]func(map[string]string){
		"development database password": func(v map[string]string) {
			v["DATABASE_URL"] = "postgres://arbion:local-development-only@postgres/arbion?sslmode=disable"
		},
		"short internal token": func(v map[string]string) { v["AI_INTERNAL_SERVICE_TOKEN"] = "short" },
		"wildcard origin":      func(v map[string]string) { v["AUTH_ALLOWED_ORIGINS"] = "https://*.arbion.ai" },
	} {
		t.Run(name, func(t *testing.T) {
			values := validProduction()
			mutate(values)
			if _, err := load(values); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestProductionSchwabConfigurationIsAllOrNothing(t *testing.T) {
	values := validProduction()
	values["SCHWAB_CLIENT_ID"] = "configured"
	if _, err := load(values); err == nil {
		t.Fatal("expected incomplete Schwab configuration rejection")
	}
	values["SCHWAB_CLIENT_SECRET"] = "secret"
	values["SCHWAB_REDIRECT_URI"] = "https://www.arbion.ai/api/connections/financial/schwab/callback"
	if _, err := load(values); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRejectsProductionPlaceholderKey(t *testing.T) {
	values := validEnvironment()
	values["ARBION_ENV"] = "production"
	values["DATABASE_URL"] = "postgres://user:pass@database.example/arbion?sslmode=require"
	if _, err := load(values); err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("expected placeholder key error, got %v", err)
	}
}
func TestLoadRejectsInvalidKey(t *testing.T) {
	values := validEnvironment()
	values["CREDENTIAL_ENCRYPTION_KEY"] = "not-a-key"
	if _, err := load(values); err == nil {
		t.Fatal("expected invalid key error")
	}
}
func TestLoadRejectsInvalidPoolBounds(t *testing.T) {
	values := validEnvironment()
	values["DATABASE_MAX_CONNECTIONS"] = "2"
	values["DATABASE_MIN_CONNECTIONS"] = "3"
	if _, err := load(values); err == nil {
		t.Fatal("expected pool bounds error")
	}
}
