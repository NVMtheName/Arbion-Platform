package risk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestPostgresGlobalBreakerIsDurableExclusiveAndSuperadminBound(t *testing.T) {
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
	if err = pool.QueryRow(ctx, `INSERT INTO users(external_id,role,status) VALUES($1,'superadmin','active') RETURNING id::text`, fmt.Sprintf("global-breaker-superadmin-%d", unique)).Scan(&superadminID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO users(external_id,role,status) VALUES($1,'user','active') RETURNING id::text`, fmt.Sprintf("global-breaker-user-%d", unique)).Scan(&userID); err != nil {
		t.Fatal(err)
	}

	store := NewPostgresBreakerStore(pool)
	active, err := store.ActiveSuperadmin(ctx, superadminID)
	if err != nil || !active {
		t.Fatalf("active superadmin was not recognized: active=%v err=%v", active, err)
	}
	active, err = store.ActiveSuperadmin(ctx, userID)
	if err != nil || active {
		t.Fatalf("ordinary user was treated as superadmin: active=%v err=%v", active, err)
	}
	if _, err = store.OpenGlobalBreaker(ctx, userID); !errors.Is(err, ErrBreakerAdminRequired) {
		t.Fatalf("ordinary user read the global stop: %v", err)
	}

	engagedAt := time.Now().UTC().Truncate(time.Microsecond)
	engaged, err := store.EngageGlobalBreaker(ctx, superadminID, "platform provider integrity review", engagedAt)
	if err != nil || engaged.Scope != ScopeGlobal || engaged.ScopeID != nil || engaged.State != BreakerOpen || engaged.Source != "ADMIN_UI" || engaged.EngagedByUserID == nil || *engaged.EngagedByUserID != superadminID {
		t.Fatalf("global stop was not durably superadmin-bound: %#v err=%v", engaged, err)
	}
	if _, err = store.EngageGlobalBreaker(ctx, superadminID, "duplicate platform stop", engagedAt.Add(time.Second)); !errors.Is(err, ErrBreakerConflict) {
		t.Fatalf("a second open global stop was accepted: %v", err)
	}
	current, err := store.OpenGlobalBreaker(ctx, superadminID)
	if err != nil || current == nil || current.ID != engaged.ID || current.State != BreakerOpen {
		t.Fatalf("open global stop was not durable: %#v err=%v", current, err)
	}

	releasedAt := engagedAt.Add(time.Minute)
	released, err := store.ReleaseGlobalBreaker(ctx, superadminID, releasedAt)
	if err != nil || released.State != BreakerClosed || released.ReleasedByUserID == nil || *released.ReleasedByUserID != superadminID || released.ReleasedAt == nil || !released.ReleasedAt.Equal(releasedAt) {
		t.Fatalf("global release evidence was incomplete: %#v err=%v", released, err)
	}
	current, err = store.OpenGlobalBreaker(ctx, superadminID)
	if err != nil || current != nil {
		t.Fatalf("released global stop still appeared open: %#v err=%v", current, err)
	}
}
