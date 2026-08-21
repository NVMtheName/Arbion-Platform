package marketintelligence

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestPostgresHealthHistoryAggregatesAndPrunesSafeOutcomes(t *testing.T) {
	databaseURL := os.Getenv("STRATEGY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("STRATEGY_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := NewPostgresHealthStore(pool)

	base := time.Date(2026, time.August, 21, 18, 1, 0, 0, time.UTC)
	if _, err = pool.Exec(ctx, `DELETE FROM market_source_health_buckets WHERE source_id='alpaca_iex' AND capability='EQUITY_QUOTE'`); err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []HealthOutcome{
		{SourceID: "alpaca_iex", Capability: EquityQuote, State: Verified, ObservedAt: base},
		{SourceID: "alpaca_iex", Capability: EquityQuote, State: Degraded, FailureCategory: "TIMEOUT", ObservedAt: base.Add(time.Minute)},
		{SourceID: "alpaca_iex", Capability: EquityQuote, State: Verified, ObservedAt: base.Add(7 * time.Minute)},
	} {
		if err = store.RecordOutcome(ctx, outcome); err != nil {
			t.Fatal(err)
		}
	}
	buckets, err := store.Hourly(ctx, base.Truncate(time.Hour), base.Truncate(time.Hour).Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 1 || buckets[0].CompletedAttempts != 3 || buckets[0].Successes != 2 || buckets[0].Failures != 1 || buckets[0].LastState != Verified || buckets[0].FailureCategory != "" {
		t.Fatalf("unexpected durable health aggregate: %+v", buckets)
	}

	oldBucket := base.Add(-31 * 24 * time.Hour).Truncate(healthStorageInterval)
	if _, err = pool.Exec(ctx, `INSERT INTO market_source_health_buckets(source_id,capability,bucket_started_at,completed_attempts,successes,failures,last_state,last_observed_at) VALUES('alpaca_iex','EQUITY_QUOTE',$1,1,1,0,'VERIFIED',$1)`, oldBucket); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordOutcome(ctx, HealthOutcome{SourceID: "alpaca_iex", Capability: EquityQuote, State: Verified, ObservedAt: base.Add(10 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	var oldRows int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM market_source_health_buckets WHERE source_id='alpaca_iex' AND capability='EQUITY_QUOTE' AND bucket_started_at=$1`, oldBucket).Scan(&oldRows); err != nil || oldRows != 0 {
		t.Fatalf("expired health bucket was not pruned: count=%d err=%v", oldRows, err)
	}
}
