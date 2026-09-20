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
			want := ErrCommitActionCooldown
			if kind == "daily quota" {
				want = ErrCommitActionLimit
			}
			if !((first == nil && errors.Is(second, want)) || (second == nil && errors.Is(first, want))) {
				t.Fatal("expected one saved action and one activity-boundary refusal", first, second)
			}
			for _, table := range []string{"strategy_evaluation_events", "decision_journal_entries", "nonlive_execution_records"} {
				assertCount(t, pool, `SELECT count(*) FROM `+table+` WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
			}
			assertCount(t, pool, `SELECT count(*) FROM risk_evaluations WHERE user_id='`+f.instance.UserID+`'`, 1)
			assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE user_id='`+f.instance.UserID+`'`, 0)
		})
	}
	for _, mode := range []ExecutionMode{Paper, Shadow} {
		t.Run(string(mode)+" saved denial consumes daily quota", func(t *testing.T) {
			f := newNonLiveBindingFixture(t, ctx, pool, mode, json.RawMessage(`{"risk_parameters":{"max_trades_per_day":1}}`))
			denied := f.evaluation
			denied.Decision = risk.Deny
			if err := f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, denied, ExecutionResult{Status: RiskDenied, ExpectedState: AIMonitoring}, f.now); err != nil {
				t.Fatal(err)
			}
			other := distinctActivityFixture(t, ctx, f)
			commit := commitShadowBindingFixture
			if mode == Paper {
				commit = func(ctx context.Context, f paperBindingFixture) error { return f.commit(ctx) }
			}
			if err := commit(ctx, other); !errors.Is(err, ErrCommitActionLimit) {
				t.Fatal("saved denial was omitted from quota", err)
			}
			assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
			assertCount(t, pool, `SELECT count(*) FROM strategy_evaluation_events WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
			assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE strategy_instance_id='`+f.instance.ID+`'`, 0)
			assertCount(t, pool, `SELECT count(*) FROM paper_positions WHERE paper_portfolio_id=(SELECT id FROM paper_portfolios WHERE strategy_instance_id='`+f.instance.ID+`')`, 0)
			if mode == Paper {
				assertCount(t, pool, `SELECT count(*) FROM paper_portfolios WHERE strategy_instance_id='`+f.instance.ID+`' AND cash=1000 AND version=1`, 1)
			}
		})
	}
	t.Run("Paper fresh cash does not bypass accepted-action cooldown", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		if err := f.commit(ctx); err != nil {
			t.Fatal(err)
		}
		other := distinctActivityFixture(t, ctx, f)
		other.fill = SimulateAIPaperSpotFill(*other.decision.ProposedAction, other.evaluation, "CRYPTO",
			AIPaperPortfolioSnapshot{Currency: "USD", Cash: f.fill.ResultingCash, Positions: map[string]string{"BTC": f.fill.ResultingPositionQuantity}},
			AIPaperMarketReference{Symbol: "BTC", Price: "100", Basis: "ASK", Provider: "coinbase", Feed: "rest_ticker", Quality: "REAL_TIME_SINGLE_VENUE", ObservedAt: f.now}, AIPaperSimulationConfig{}, f.now)
		if other.fill.Status != SimulatedFilled {
			t.Fatal("invalid next test fill")
		}
		if err := other.commit(ctx); !errors.Is(err, ErrCommitActionCooldown) {
			t.Fatal("fresh ledger bypassed cooldown", err)
		}
		if err := f.commit(ctx); !errors.Is(err, ErrDuplicate) {
			t.Fatal("cooldown hid exact duplicate", err)
		}
		assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
		assertCount(t, pool, `SELECT count(*) FROM paper_portfolios WHERE strategy_instance_id='`+f.instance.ID+`' AND cash=`+f.fill.ResultingCash+` AND version=2`, 1)
	})
	t.Run("abstention is not a quota action", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{"risk_parameters":{"max_trades_per_day":1}}`))
		if err := f.store.CommitAIAbstention(ctx, f.instance, "abstain:"+f.instance.ID, json.RawMessage(`{"decision":"ABSTAIN"}`), f.now); err != nil {
			t.Fatal(err)
		}
		if err := commitShadowBindingFixture(ctx, f); err != nil {
			t.Fatal("abstention consumed an action slot", err)
		}
	})
	t.Run("denial does not start cooldown", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{"risk_parameters":{"max_trades_per_day":3}}`))
		denied := f.evaluation
		denied.Decision = risk.Deny
		if err := f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, denied, ExecutionResult{Status: RiskDenied, ExpectedState: AIMonitoring}, f.now); err != nil {
			t.Fatal(err)
		}
		if err := commitShadowBindingFixture(ctx, distinctActivityFixture(t, ctx, f)); err != nil {
			t.Fatal("denial incorrectly started cooldown", err)
		}
	})
	t.Run("another account keeps its own quota and cooldown", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{"risk_parameters":{"max_trades_per_day":1}}`))
		other := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{"risk_parameters":{"max_trades_per_day":1}}`), f.instance.UserID)
		if err := commitShadowBindingFixture(ctx, f); err != nil {
			t.Fatal(err)
		}
		if err := commitShadowBindingFixture(ctx, other); err != nil {
			t.Fatal("activity leaked between accounts", err)
		}
	})
	t.Run("future saved activity is unavailable", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		denied := f.evaluation
		denied.Decision = risk.Deny
		// Create malformed timing at insertion in the isolated database only;
		// never alter immutable history or disable its protections.
		future := f.now.Add(time.Hour)
		decision := f.decision
		quote := *decision.QuoteReference
		quote.ObservedAt = future
		decision.QuoteReference = &quote
		decision.Rationale, _ = json.Marshal(map[string]any{"decision": "PROPOSE", "quote_reference": quote})
		if err := f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, decision, denied, ExecutionResult{Status: RiskDenied, ExpectedState: AIMonitoring}, future); err != nil {
			t.Fatal(err)
		}
		if err := commitShadowBindingFixture(ctx, distinctActivityFixture(t, ctx, f)); !errors.Is(err, ErrCommitActivityUnavailable) {
			t.Fatal("future activity accepted", err)
		}
	})
	for _, policy := range []string{`null`, `{"max_trades_per_day":"1"}`, `{"max_trades_per_day":-1}`} {
		t.Run("invalid pinned activity policy "+policy, func(t *testing.T) {
			f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{"risk_parameters":`+policy+`}`))
			if err := commitShadowBindingFixture(ctx, f); !errors.Is(err, ErrEvaluationConfigurationChanged) {
				t.Fatal("invalid pinned activity policy accepted", err)
			}
			f.assertEmpty(t, ctx)
		})
	}
}
