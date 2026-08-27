package platformops

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestPostgresSnapshotUsesAggregateSafeTablesAndCurrentAdmin(t *testing.T) {
	databaseURL := os.Getenv("STRATEGY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("STRATEGY_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpContext(ctx, database, "."); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	unique := time.Now().UTC().UnixNano()
	var superadminID, userID string
	if err = pool.QueryRow(ctx, `INSERT INTO users(external_id,role,status) VALUES($1,'superadmin','active') RETURNING id::text`, fmt.Sprintf("operations-superadmin-%d", unique)).Scan(&superadminID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO users(external_id,role,status) VALUES($1,'user','active') RETURNING id::text`, fmt.Sprintf("operations-user-%d", unique)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	store := NewPostgresStore(pool)
	active, err := store.ActiveSuperadmin(ctx, superadminID)
	if err != nil || !active {
		t.Fatalf("current superadmin was not recognized: active=%v err=%v", active, err)
	}
	active, err = store.ActiveSuperadmin(ctx, userID)
	if err != nil || active {
		t.Fatalf("ordinary user was treated as current superadmin: active=%v err=%v", active, err)
	}
	facts, err := store.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("aggregate snapshot did not compile against the migrated schema: %v", err)
	}
	if facts.OpenGlobalBreakers != 0 || facts.ExecutionBoundary.LiveMandates != 0 || facts.ExecutionBoundary.ExecutableRiskEvaluations != 0 {
		t.Fatalf("empty execution boundary was not safe: %#v", facts)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO risk_circuit_breakers(scope,scope_id,state,reason,source,engaged_by_user_id) VALUES('GLOBAL',NULL,'OPEN','operations integration review','ADMIN_UI',$1)`, superadminID); err != nil {
		t.Fatal(err)
	}
	facts, err = store.Snapshot(ctx, time.Now().UTC())
	if err != nil || facts.OpenGlobalBreakers != 1 {
		t.Fatalf("open global stop was absent from aggregate evidence: %#v err=%v", facts, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE risk_circuit_breakers SET state='CLOSED',released_by_user_id=$1,released_at=now() WHERE scope='GLOBAL' AND state='OPEN'`, superadminID); err != nil {
		t.Fatal(err)
	}
}
