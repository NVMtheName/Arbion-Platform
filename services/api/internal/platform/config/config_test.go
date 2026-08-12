package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func validEnvironment() map[string]string {
	return map[string]string{"ARBION_ENV": "development", "DATABASE_URL": "postgres://user:pass@localhost/db?sslmode=disable", "REDIS_URL": "redis://localhost:6379/0", "CREDENTIAL_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32))}
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
