package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testAICommitMarketTime(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Run("old evaluation clock cannot rescue an expired quote", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		old := f.now.Add(-marketDataMaxAge - time.Minute)
		setCommitMarketFixtureTime(t, &f, old, old)
		commitErr := f.commit(ctx)
		var current time.Time
		if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&current); err != nil {
			t.Fatal(err)
		}
		want := ErrCommitMarketDataStale
		if !sameAICommitActivityDay(old, current) {
			// Near UTC midnight, the older prepared evaluation hits the daily
			// activity guard first. Both checks must still reject and roll back.
			want = ErrCommitActivityUnavailable
		}
		if !errors.Is(commitErr, want) {
			t.Fatal("Paper accepted old quote or missed the earlier UTC-day guard", commitErr)
		}
		f.assertEmpty(t, ctx)
		err := f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, ExecutionResult{Status: WouldHaveSubmitted, ExpectedState: AIMonitoring}, f.now)
		if !errors.Is(err, ErrCommitMarketDataStale) {
			t.Fatal("generic AI writer accepted old quote", err)
		}
		f.assertEmpty(t, ctx)
	})
	t.Run("quote expires while waiting on ledger and rolls back", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		var now time.Time
		if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			t.Fatal(err)
		}
		observed := now.Add(-aiPaperMaximumMarketAge + 2*time.Second)
		setCommitMarketFixtureTime(t, &f, now, observed)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT id FROM paper_portfolios WHERE strategy_instance_id=$1 FOR UPDATE`, f.instance.ID); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() { result <- f.commit(ctx) }()
		waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
		for {
			var expired bool
			if err = pool.QueryRow(ctx, `SELECT clock_timestamp()>$1`, observed.Add(aiPaperMaximumMarketAge)).Scan(&expired); err != nil {
				t.Fatal(err)
			}
			if expired {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(10 * time.Millisecond):
			}
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-result; !errors.Is(err, ErrCommitMarketDataStale) {
			t.Fatal("ledger wait extended quote lifetime", err)
		}
		f.assertEmpty(t, ctx)
	})
	t.Run("expired quote does not erase denial evidence", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		old := f.now.Add(-marketDataMaxAge - time.Minute)
		setCommitMarketFixtureTime(t, &f, old, old)
		f.evaluation.Decision = risk.Deny
		if err := f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, ExecutionResult{Status: RiskDenied, ExpectedState: AIMonitoring}, old); err != nil {
			t.Fatal(err)
		}
		assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`' AND status='RISK_DENIED'`, 1)
		assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE strategy_instance_id='`+f.instance.ID+`'`, 0)
	})
}

func setCommitMarketFixtureTime(t *testing.T, f *paperBindingFixture, evaluated, observed time.Time) {
	t.Helper()
	f.now = evaluated
	f.decision.ProposedAction.CreatedAt = evaluated
	f.evaluation.Timestamp = evaluated
	f.fill.SimulatedAt = evaluated
	f.fill.MarketObservedAt = observed
	f.decision.QuoteReference.ObservedAt = observed
	var saved map[string]any
	if err := json.Unmarshal(f.decision.Rationale, &saved); err != nil {
		t.Fatal(err)
	}
	saved["quote_reference"] = f.decision.QuoteReference
	var err error
	f.decision.Rationale, err = json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
}
