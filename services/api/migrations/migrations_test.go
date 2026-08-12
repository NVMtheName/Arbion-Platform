package migrations

import (
	"io/fs"
	"strings"
	"testing"
)

func TestInitialMigrationDefinesRequiredSchema(t *testing.T) {
	body, err := fs.ReadFile(Files, "00001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"-- +goose Up", "CREATE TABLE users", "CREATE TABLE provider_connections", "CREATE TABLE automation_configs", "CREATE TABLE audit_events", "-- +goose Down"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("migration missing %q", required)
		}
	}
}
