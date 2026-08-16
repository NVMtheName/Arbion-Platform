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

func (db *captureDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected Query call")
}

func (db *captureDB) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	db.query = query
	db.args = args
	return captureRow{}
}

func (db *captureDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected Exec call")
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
