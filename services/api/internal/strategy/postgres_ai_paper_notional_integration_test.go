package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testAIPaperFillNotionalIntegrity(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	for _, side := range []string{"BUY", "SELL"} {
		for _, gross := range []string{"1", "75"} {
			t.Run(side+" balanced invalid gross "+gross, func(t *testing.T) {
				f := paperNotionalFixture(t, ctx, pool, side, "100")
				validFill := f.fill
				before := paperNotionalStoredState(t, ctx, f)
				// Keep every old SQL cash/quantity equation internally balanced.
				// Only gross versus quantity * fill price is inconsistent.
				f.fill.GrossNotional = gross
				notional, _ := new(big.Rat).SetString(gross)
				cashDelta := new(big.Rat).Set(notional)
				if side == "BUY" {
					cashDelta.Neg(cashDelta)
				}
				cash, _ := new(big.Rat).SetString(f.fill.PreviousCash)
				f.fill.CashDelta = cashDelta.FloatString(10)
				f.fill.ResultingCash = new(big.Rat).Add(cash, cashDelta).FloatString(10)
				if err := f.commit(ctx); !errors.Is(err, ErrInvalid) {
					t.Fatal("balanced fill with incorrect price-times-quantity gross was accepted", err)
				}
				if after := paperNotionalStoredState(t, ctx, f); !reflect.DeepEqual(before, after) {
					t.Fatal("rejected fill changed runtime, capital, portfolio, or immutable history")
				}
				// A refusal must not leave an event claim that blocks a valid
				// retry of this same delivery, action, and risk evaluation.
				f.fill = validFill
				assertPaperNotionalCommitAndDuplicate(t, ctx, f)
			})
		}
	}
	for _, side := range []string{"BUY", "SELL"} {
		t.Run(side+" legitimate side-conservative product", func(t *testing.T) {
			f := paperNotionalFixture(t, ctx, pool, side, "100.0000000001")
			want := "50.0000000001"
			if side == "SELL" {
				want = "50.0000000000"
			}
			if f.fill.GrossNotional != want {
				t.Fatal("fixture did not exercise a product beyond ten decimal places", f.fill.GrossNotional, want)
			}
			assertPaperNotionalCommitAndDuplicate(t, ctx, f)
		})
	}
}

// Sale fixtures obtain inventory only from a real successful simulation commit.
// No immutable fill, history, or database constraint is edited to construct it.
func paperNotionalFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, side, price string) paperBindingFixture {
	t.Helper()
	f := newPaperBindingFixture(t, ctx, pool)
	portfolio := AIPaperPortfolioSnapshot{Currency: "USD", Cash: "1000", Positions: map[string]string{}}
	if side == "SELL" {
		if err := f.commit(ctx); err != nil {
			t.Fatal("seed simulation buy", err)
		}
		portfolio.Cash = f.fill.ResultingCash
		portfolio.Positions["BTC"] = f.fill.ResultingPositionQuantity
		f = distinctActivityFixture(t, ctx, f)
		f.now = time.Now().UTC().Truncate(time.Microsecond)
	}
	action := *f.decision.ProposedAction
	action.Side = side
	action.ActionType = risk.ActionBuy
	basis := "ASK"
	if side == "SELL" {
		action.ActionType = risk.ActionSell
		basis = "BID"
	}
	action.EstimatedPrice = &price
	// Covers the exact unrounded reference product for both fixtures while
	// preserving the existing mandate's proposal ceiling.
	action.Notional = "50.0000000001"
	action.CreatedAt = f.now
	f.decision.ProposedAction = &action
	f.evaluation.Timestamp = f.now
	quote := *f.decision.QuoteReference
	quote.Side, quote.Basis, quote.Price, quote.ObservedAt = side, basis, price, f.now
	f.decision.QuoteReference = &quote
	var err error
	f.decision.Rationale, err = json.Marshal(map[string]any{"decision": "PROPOSE", "quote_reference": quote})
	if err != nil {
		t.Fatal(err)
	}
	f.fill = SimulateAIPaperSpotFill(action, f.evaluation, "CRYPTO", portfolio,
		AIPaperMarketReference{Symbol: "BTC", Price: price, Basis: basis, Provider: quote.Provider, Feed: quote.Feed, Quality: quote.Quality, ObservedAt: f.now},
		AIPaperSimulationConfig{}, f.now)
	if f.fill.Status != SimulatedFilled || f.fill.Fee != "0.0000000000" {
		t.Fatal("invalid zero-fee notional fixture", f.fill)
	}
	return f
}

func assertPaperNotionalCommitAndDuplicate(t *testing.T, ctx context.Context, f paperBindingFixture) {
	t.Helper()
	if err := f.commit(ctx); err != nil {
		t.Fatal("valid side-conservative fill rejected", err)
	}
	var cash, quantity, gross string
	if err := f.store.db.QueryRow(ctx, `SELECT p.cash::text,x.quantity::text,l.gross_notional::text
		FROM paper_portfolios p JOIN paper_positions x ON x.paper_portfolio_id=p.id
		JOIN ai_paper_spot_fills l ON l.paper_portfolio_id=p.id AND l.proposed_action_id=$2
		WHERE p.strategy_instance_id=$1 AND x.symbol='BTC'`, f.instance.ID, f.decision.ProposedAction.ID).Scan(&cash, &quantity, &gross); err != nil ||
		!sameAIPaperDecimal(cash, f.fill.ResultingCash) || !sameAIPaperDecimal(quantity, f.fill.ResultingPositionQuantity) || !sameAIPaperDecimal(gross, f.fill.GrossNotional) {
		t.Fatal("valid fill ledger did not match exact simulation", cash, quantity, gross, err)
	}
	beforeDuplicate := paperNotionalStoredState(t, ctx, f)
	if err := f.commit(ctx); !errors.Is(err, ErrDuplicate) {
		t.Fatal("valid saved fill lost duplicate protection", err)
	}
	if after := paperNotionalStoredState(t, ctx, f); !reflect.DeepEqual(beforeDuplicate, after) {
		t.Fatal("duplicate fill changed saved state or history")
	}
}

// Capture complete deterministic row representations, not merely row counts:
// a failed commit must preserve cash, position basis, versions, timestamps,
// active capital claims, and every existing immutable record exactly.
func paperNotionalStoredState(t *testing.T, ctx context.Context, f paperBindingFixture) map[string]string {
	t.Helper()
	state := make(map[string]string)
	for _, table := range []string{"strategy_instances", "strategy_capital_reservations", "paper_portfolios", "risk_evaluations", "decision_journal_entries", "nonlive_execution_records", "ai_paper_spot_fills", "order_intents", "nonlive_strategy_schedules", "nonlive_schedule_runs"} {
		var rows string
		if err := f.store.db.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text FROM `+table+` r WHERE user_id=$1`, f.instance.UserID).Scan(&rows); err != nil {
			t.Fatal("capture owner-scoped "+table, err)
		}
		state[table] = rows
	}
	for _, table := range []string{"strategy_evaluation_events", "strategy_state_transitions", "strategy_lifecycle_events"} {
		var rows string
		if err := f.store.db.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text FROM `+table+` r WHERE strategy_instance_id=$1`, f.instance.ID).Scan(&rows); err != nil {
			t.Fatal("capture instance-scoped "+table, err)
		}
		state[table] = rows
	}
	var positions string
	if err := f.store.db.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text
		FROM paper_positions r WHERE paper_portfolio_id IN (SELECT id FROM paper_portfolios WHERE strategy_instance_id=$1 AND user_id=$2)`, f.instance.ID, f.instance.UserID).Scan(&positions); err != nil {
		t.Fatal("capture isolated positions", err)
	}
	state["paper_positions"] = positions
	return state
}
