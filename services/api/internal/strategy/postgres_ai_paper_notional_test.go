package strategy

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

func TestValidAIPaperCommitRejectsBalancedGrossMismatch(t *testing.T) {
	for _, side := range []string{"BUY", "SELL"} {
		for _, gross := range []string{"1", "101"} {
			t.Run(side+" gross "+gross, func(t *testing.T) {
				i, d, e, fill, now := aiPaperNotionalFixture(t, side, "1", "100", "100")
				rebalanceAIPaperNotional(t, &fill, gross)
				if validAIPaperCommit(i, i.StateVersion, d, e, fill, now) {
					t.Fatal("accepted gross inconsistent with quantity and fill price despite balanced cash accounting")
				}
			})
		}
	}
}

func TestValidAIPaperCommitQuantizesGrossBySide(t *testing.T) {
	for _, tc := range []struct {
		side, correct, opposite string
	}{
		{"BUY", "0.1111111111", "0.1111111110"},
		{"SELL", "0.1111111110", "0.1111111111"},
	} {
		t.Run(tc.side, func(t *testing.T) {
			// The exact product is 0.11111111108888888889: neither
			// unrounded equality nor a one-quantum tolerance is sufficient.
			i, d, e, fill, now := aiPaperNotionalFixture(t, tc.side, "0.3333333333", "0.3333333333", "1")
			if fill.GrossNotional != tc.correct {
				t.Fatal("fixture did not establish the expected side-conservative product", fill.GrossNotional)
			}
			if !validAIPaperCommit(i, i.StateVersion, d, e, fill, now) {
				t.Fatal("correctly quantized nonintegral product was rejected")
			}
			rebalanceAIPaperNotional(t, &fill, tc.opposite)
			if validAIPaperCommit(i, i.StateVersion, d, e, fill, now) {
				t.Fatal("accepted the opposite rounding direction, one decimal-storage quantum away")
			}
		})
	}
}

func TestValidAIPaperCommitAcceptsEquivalentNotionalFormatting(t *testing.T) {
	for _, side := range []string{"BUY", "SELL"} {
		t.Run(side, func(t *testing.T) {
			i, d, e, fill, now := aiPaperNotionalFixture(t, side, "1.0000000000", "100.0000000000", "100")
			fill.Quantity = "1"
			fill.FillPrice = "100.00"
			fill.GrossNotional = "100.0"
			if !validAIPaperCommit(i, i.StateVersion, d, e, fill, now) {
				t.Fatal("numerically equivalent quantity, price, and gross were rejected")
			}
		})
	}
}

// This is the same valid owner/action/quote/risk setup as the existing decimal
// formatting test, parameterized without changing its original fixture.
func aiPaperNotionalFixture(t *testing.T, side, quantity, price, notional string) (Instance, Decision, risk.RiskEvaluation, AIPaperFill, time.Time) {
	t.Helper()
	now := time.Date(2026, 8, 29, 20, 26, 55, 0, time.UTC)
	mandateID, mandateVersion, state := "mandate", 2, string(AIMonitoring)
	i := Instance{
		ID: "instance", UserID: "owner", AutomationMandateID: mandateID,
		MandateVersion: mandateVersion, FinancialAccountID: "account", CapitalBucketID: "bucket",
		StrategyIdentifier: "ai_shadow", ExecutionMode: Paper,
		CurrentState: AIMonitoring, StateVersion: 1, Status: "ACTIVE",
	}
	actionType, basis := risk.ActionBuy, "ASK"
	if side == "SELL" {
		actionType, basis = risk.ActionSell, "BID"
	}
	action := risk.ProposedAction{
		ID: "paper-action", CorrelationID: "scheduled:paper-action",
		FinancialAccountID: i.FinancialAccountID, Source: risk.SourceAI,
		ActionType: actionType, MandateID: &mandateID, MandateVersion: &mandateVersion,
		Instrument: "BTC", Side: side, Quantity: quantity, Notional: notional, EstimatedPrice: &price,
		StrategyInstanceID: &i.ID, StrategyState: &state, CreatedAt: now,
	}
	e := risk.RiskEvaluation{
		ID: "evaluation", UserID: i.UserID, AccountID: i.FinancialAccountID,
		MandateID: &mandateID, MandateVersion: &mandateVersion,
		Decision: risk.Allow, Mode: "PAPER", Timestamp: now,
	}
	quote := &AIProposalQuoteReference{
		Symbol: "BTC", Side: side, Price: price, Basis: basis,
		Provider: "coinbase", Feed: "exchange", Quality: "REALTIME", ObservedAt: now,
	}
	rationale, err := json.Marshal(map[string]any{"decision": "PROPOSE", "quote_reference": quote})
	if err != nil {
		t.Fatal(err)
	}
	d := Decision{ProposedAction: &action, Source: "AI", InstrumentType: "CRYPTO", ProposedState: AIMonitoring, QuoteReference: quote, Rationale: rationale}
	fill := SimulateAIPaperSpotFill(action, e, "CRYPTO",
		AIPaperPortfolioSnapshot{Currency: "USD", Cash: "1000", Positions: map[string]string{"BTC": "2"}},
		AIPaperMarketReference{Symbol: "BTC", Price: price, Basis: basis, Provider: quote.Provider, Feed: quote.Feed, Quality: quote.Quality, ObservedAt: now},
		AIPaperSimulationConfig{}, now)
	if fill.Status != SimulatedFilled || !validAIPaperCommit(i, i.StateVersion, d, e, fill, now) {
		t.Fatal("invalid notional regression fixture", fill)
	}
	return i, d, e, fill, now
}

func rebalanceAIPaperNotional(t *testing.T, fill *AIPaperFill, gross string) {
	t.Helper()
	parse := func(value string) *big.Rat {
		t.Helper()
		n, ok := new(big.Rat).SetString(value)
		if !ok {
			t.Fatal("invalid fixture amount", value)
		}
		return n
	}
	delta := new(big.Rat)
	if fill.Side == "BUY" {
		delta.Neg(new(big.Rat).Add(parse(gross), parse(fill.Fee)))
	} else {
		delta.Sub(parse(gross), parse(fill.Fee))
	}
	fill.GrossNotional = gross
	fill.CashDelta = delta.FloatString(10)
	fill.ResultingCash = new(big.Rat).Add(parse(fill.PreviousCash), delta).FloatString(10)
}
