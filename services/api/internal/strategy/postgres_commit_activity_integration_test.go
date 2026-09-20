package strategy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func distinctActivityFixture(t *testing.T, ctx context.Context, f paperBindingFixture) paperBindingFixture {
	t.Helper()
	other := f
	action := *f.decision.ProposedAction
	action.ID += ":other"
	action.CorrelationID += ":other"
	other.decision.ProposedAction = &action
	other.evaluation.ID = paperBindingUUID(t, ctx, f.store.db)
	return other
}

func testAICommitActivity(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	for _, kind := range []string{"repeat action", "daily quota"} {
		t.Run("concurrent prepared Shadow "+kind, func(t *testing.T) {
			snapshot := json.RawMessage(`{}`)
			if kind == "daily quota" {
				snapshot = json.RawMessage(`{"risk_parameters":{"max_trades_per_day":1},"allowed_universe":{"symbols":["BTC","ETH"]}}`)
			}
			f := newNonLiveBindingFixture(t, ctx, pool, Shadow, snapshot)
			other := distinctActivityFixture(t, ctx, f)
			if kind == "daily quota" {
				// Another symbol avoids the repeat-action guard: only the daily
				// quota should prevent two stale evaluations using the last slot.
				other.decision.ProposedAction.Instrument = "ETH"
				quote := *other.decision.QuoteReference
				quote.Symbol = "ETH"
				other.decision.QuoteReference = &quote
				other.decision.Rationale, _ = json.Marshal(map[string]any{"decision": "PROPOSE", "quote_reference": quote})
			}
			results := make(chan error, 2)
			go func() { results <- commitShadowBindingFixture(ctx, f) }()
			go func() { results <- commitShadowBindingFixture(ctx, other) }()
			first, second := <-results, <-results
			if (first == nil) == (second == nil) {
				t.Fatal("expected one saved action and one activity-boundary refusal", first, second)
			}
			for _, table := range []string{"strategy_evaluation_events", "decision_journal_entries", "nonlive_execution_records"} {
				assertCount(t, pool, `SELECT count(*) FROM `+table+` WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
			}
		})
	}
}
