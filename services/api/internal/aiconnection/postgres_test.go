package aiconnection

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type captureRow struct{}

func (captureRow) Scan(...any) error { return errors.New("stop after capturing query") }

type captureDB struct {
	query string
	args  []any
}

func (db *captureDB) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	db.query = query
	db.args = args
	return nil, errors.New("stop after capturing query")
}

func (db *captureDB) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	db.query = query
	db.args = args
	return captureRow{}
}

func (db *captureDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected Exec call")
}

func (db *captureDB) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("unexpected Begin call")
}

func TestCreateCastsCredentialHintForJSONConstruction(t *testing.T) {
	db := &captureDB{}
	_, err := NewPostgresStore(db, DefaultRegistry()).Create(
		context.Background(), "user-1", "openai", "OpenAI Production", "••••••••test",
	)
	if err == nil {
		t.Fatal("expected the capture row to stop the scan")
	}
	if !strings.Contains(db.query, "jsonb_build_object('hint',$4::text)") {
		t.Fatalf("credential hint must have an explicit PostgreSQL text type: %q", db.query)
	}
	if len(db.args) != 4 || db.args[3] != "••••••••test" {
		t.Fatalf("unexpected create arguments: %#v", db.args)
	}
}

func TestDurableDependenciesUseCurrentOwnerScopedSchema(t *testing.T) {
	db := &captureDB{}
	_, err := NewPostgresStore(db, DefaultRegistry()).HasDependencies(context.Background(), "user-1", "connection-1")
	if err == nil {
		t.Fatal("expected the capture row to stop the scan")
	}
	for _, required := range []string{
		"provider_connections",
		"provider_category='ai'",
		"neural_engine_preferences",
		"automation_mandates",
		"automation_mandate_versions",
		"snapshot->>'ai_provider_connection_id'",
	} {
		if !strings.Contains(db.query, required) {
			t.Fatalf("durable dependency query is missing %q: %s", required, db.query)
		}
	}
	if strings.Contains(db.query, "automation_configs") {
		t.Fatalf("durable dependency query uses the retired automation table: %s", db.query)
	}
	if len(db.args) != 2 || db.args[0] != "connection-1" || db.args[1] != "user-1" {
		t.Fatalf("unexpected dependency arguments: %#v", db.args)
	}
}

func TestRuntimeUseIncludesCurrentMandatesAndPinnedImmutableVersions(t *testing.T) {
	db := &captureDB{}
	_, err := NewPostgresStore(db, DefaultRegistry()).ConnectionInUse(context.Background(), "user-1", "connection-1")
	if err == nil {
		t.Fatal("expected the capture row to stop the scan")
	}
	for _, required := range []string{
		"provider_category='ai'",
		"m.status IN ('READY','PAUSED')",
		"strategy_instances",
		"i.status IN ('ACTIVE','PAUSED')",
		"v.version_number=i.mandate_version",
		"snapshot->>'ai_provider_connection_id'",
	} {
		if !strings.Contains(db.query, required) {
			t.Fatalf("runtime dependency query is missing %q: %s", required, db.query)
		}
	}
	if len(db.args) != 2 || db.args[0] != "connection-1" || db.args[1] != "user-1" {
		t.Fatalf("unexpected runtime arguments: %#v", db.args)
	}
}

func TestConnectionListProjectsOwnerScopedContinuityFacts(t *testing.T) {
	db := &captureDB{}
	_, err := NewPostgresStore(db, DefaultRegistry()).List(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected the capture query to stop the list")
	}
	for _, required := range []string{
		"p.user_id=$1",
		"p.provider_category='ai'",
		"m.status IN ('READY','PAUSED')",
		"i.status IN ('ACTIVE','PAUSED')",
		"v.version_number=i.mandate_version",
		"snapshot->>'ai_provider_connection_id'",
		"count(DISTINCT dependency.mandate_id)",
		"neural_engine_preferences",
	} {
		if !strings.Contains(db.query, required) {
			t.Fatalf("continuity projection is missing %q: %s", required, db.query)
		}
	}
	if len(db.args) != 1 || db.args[0] != "user-1" {
		t.Fatalf("unexpected continuity projection arguments: %#v", db.args)
	}
}

func TestVerificationUpdateIsCredentialGenerationBound(t *testing.T) {
	db := &captureDB{}
	_, err := NewPostgresStore(db, DefaultRegistry()).SetVerification(context.Background(), "user-1", "connection-1", "active", true, 7)
	if err == nil {
		t.Fatal("expected the capture row to stop the scan")
	}
	if !strings.Contains(db.query, "credential_generation=$5") {
		t.Fatalf("verification update is not generation-bound: %s", db.query)
	}
	if len(db.args) != 5 || db.args[4] != int64(7) {
		t.Fatalf("unexpected verification arguments: %#v", db.args)
	}
}
