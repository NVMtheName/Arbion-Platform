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

func TestAuthenticationMigrationIsVersionedAndConstrained(t *testing.T) {
	body, err := fs.ReadFile(Files, "00002_user_authentication.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"ALTER TABLE users", "normalized_email", "password_hash", "email_verified_at", "users_normalized_email_unique_idx"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("authentication migration missing %q", required)
		}
	}
}

func TestNonLiveStrategyMigrationSeparatesSimulationAndHistory(t *testing.T) {
	body, err := fs.ReadFile(Files, "00008_nonlive_strategy.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"strategy_instances", "strategy_state_transitions", "paper_portfolios", "paper_positions", "nonlive_execution_records", "decision_journal_entries", "WOULD_HAVE_SUBMITTED", "reject_nonlive_history_mutation"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("non-live strategy migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"broker_orders", "SchwabExecutionAdapter", "LIVE')"} {
		if strings.Contains(string(body), prohibited) {
			t.Errorf("non-live strategy migration unexpectedly contains %q", prohibited)
		}
	}
}
