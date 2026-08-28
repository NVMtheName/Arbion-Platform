package strategy

import (
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

func TestSimulateAIPaperSpotFillBuyUsesAdverseSlippageAndFees(t *testing.T) {
	now := time.Date(2026, 8, 28, 15, 0, 0, 0, time.UTC)
	price := "100.0000000000"
	fill := SimulateAIPaperSpotFill(
		risk.ProposedAction{Source: risk.SourceAI, ActionType: risk.ActionBuy, Instrument: "BTC", Side: "BUY", Quantity: "2.0000000000", Notional: "200.0000000000", EstimatedPrice: &price},
		risk.RiskEvaluation{Decision: risk.Allow},
		"CRYPTO",
		AIPaperPortfolioSnapshot{Currency: "USD", Cash: "1000.0000000000", Positions: map[string]string{"BTC": "1.0000000000"}},
		AIPaperMarketReference{Symbol: "BTC", Price: price, Basis: "ASK", Provider: "coinbase", Feed: "advanced_trade", Quality: "BROKER_REALTIME", ObservedAt: now.Add(-time.Second)},
		AIPaperSimulationConfig{FeeBasisPoints: 50, SlippageBasisPoints: 25},
		now,
	)
	if fill.Status != SimulatedFilled || !fill.SimulationOnly || fill.Reason != "paper_simulation_only_no_broker_order" {
		t.Fatalf("unexpected fill status: %#v", fill)
	}
	if fill.FillPrice != "100.2500000000" || fill.GrossNotional != "200.5000000000" || fill.Fee != "1.0025000000" || fill.CashDelta != "-201.5025000000" || fill.ResultingCash != "798.4975000000" || fill.ResultingPositionQuantity != "3.0000000000" {
		t.Fatalf("unexpected exact accounting: %#v", fill)
	}
	if fill.MarketProvider != "coinbase" || fill.PricingBasis != "ASK" || !fill.MarketObservedAt.Equal(now.Add(-time.Second)) {
		t.Fatalf("market provenance was not preserved: %#v", fill)
	}
}

func TestSimulateAIPaperSpotFillSellCannotShort(t *testing.T) {
	now := time.Date(2026, 8, 28, 15, 0, 0, 0, time.UTC)
	price := "50.0000000000"
	baseAction := risk.ProposedAction{Source: risk.SourceAI, ActionType: risk.ActionSell, Instrument: "SPY", Side: "SELL", Quantity: "2.0000000000", Notional: "100.0000000000", EstimatedPrice: &price}
	market := AIPaperMarketReference{Symbol: "SPY", Price: price, Basis: "BID", Provider: "schwab", Feed: "level_one_equity", Quality: "BROKER_REALTIME", ObservedAt: now}
	portfolio := AIPaperPortfolioSnapshot{Currency: "USD", Cash: "10.0000000000", Positions: map[string]string{"SPY": "1.0000000000"}}

	rejected := SimulateAIPaperSpotFill(baseAction, risk.RiskEvaluation{Decision: risk.Allow}, "EQUITY", portfolio, market, AIPaperSimulationConfig{}, now)
	if rejected.Status != SimulatedRejected || rejected.Reason != "insufficient_paper_position" {
		t.Fatalf("short sale was not rejected: %#v", rejected)
	}

	baseAction.Quantity = "1.0000000000"
	filled := SimulateAIPaperSpotFill(baseAction, risk.RiskEvaluation{Decision: risk.Allow}, "EQUITY", portfolio, market, AIPaperSimulationConfig{FeeBasisPoints: 10, SlippageBasisPoints: 20}, now)
	if filled.Status != SimulatedFilled || filled.FillPrice != "49.9000000000" || filled.GrossNotional != "49.9000000000" || filled.Fee != "0.0499000000" || filled.CashDelta != "49.8501000000" || filled.ResultingCash != "59.8501000000" || filled.ResultingPositionQuantity != "0.0000000000" {
		t.Fatalf("unexpected sell accounting: %#v", filled)
	}
}

func TestSimulateAIPaperSpotFillFailsClosed(t *testing.T) {
	now := time.Date(2026, 8, 28, 15, 0, 0, 0, time.UTC)
	price := "100.0000000000"
	action := risk.ProposedAction{Source: risk.SourceAI, ActionType: risk.ActionBuy, Instrument: "BTC", Side: "BUY", Quantity: "1.0000000000", Notional: "100.0000000000", EstimatedPrice: &price}
	portfolio := AIPaperPortfolioSnapshot{Currency: "USD", Cash: "1000.0000000000", Positions: map[string]string{}}
	market := AIPaperMarketReference{Symbol: "BTC", Price: price, Basis: "ASK", Provider: "coinbase", Feed: "advanced_trade", Quality: "BROKER_REALTIME", ObservedAt: now}

	tests := []struct {
		name       string
		action     risk.ProposedAction
		evaluation risk.RiskEvaluation
		portfolio  AIPaperPortfolioSnapshot
		market     AIPaperMarketReference
		config     AIPaperSimulationConfig
		wantStatus ExecutionStatus
		wantReason string
	}{
		{name: "risk denial", action: action, evaluation: risk.RiskEvaluation{Decision: risk.Deny}, portfolio: portfolio, market: market, wantStatus: RiskDenied, wantReason: "risk_not_allowed"},
		{name: "insufficient cash", action: action, evaluation: risk.RiskEvaluation{Decision: risk.Allow}, portfolio: AIPaperPortfolioSnapshot{Currency: "USD", Cash: "99.0000000000", Positions: map[string]string{}}, market: market, wantStatus: SimulatedRejected, wantReason: "insufficient_paper_cash"},
		{name: "stale market", action: action, evaluation: risk.RiskEvaluation{Decision: risk.Allow}, portfolio: portfolio, market: AIPaperMarketReference{Symbol: "BTC", Price: price, Basis: "ASK", Provider: "coinbase", Feed: "advanced_trade", Quality: "BROKER_REALTIME", ObservedAt: now.Add(-3 * time.Minute)}, wantStatus: SimulatedRejected, wantReason: "invalid_simulation_input"},
		{name: "missing provenance", action: action, evaluation: risk.RiskEvaluation{Decision: risk.Allow}, portfolio: portfolio, market: AIPaperMarketReference{Symbol: "BTC", Price: price, Basis: "ASK", Provider: "coinbase", ObservedAt: now}, wantStatus: SimulatedRejected, wantReason: "invalid_simulation_input"},
		{name: "non-decimal quantity", action: func() risk.ProposedAction { copy := action; copy.Quantity = "1/2"; return copy }(), evaluation: risk.RiskEvaluation{Decision: risk.Allow}, portfolio: portfolio, market: market, wantStatus: SimulatedRejected, wantReason: "invalid_simulation_input"},
		{name: "quantity exceeds authorized notional", action: func() risk.ProposedAction { copy := action; copy.Quantity = "11.0000000000"; return copy }(), evaluation: risk.RiskEvaluation{Decision: risk.Allow}, portfolio: portfolio, market: market, wantStatus: SimulatedRejected, wantReason: "quantity_exceeds_authorized_notional"},
		{name: "non-AI source", action: func() risk.ProposedAction { copy := action; copy.Source = risk.SourceUI; return copy }(), evaluation: risk.RiskEvaluation{Decision: risk.Allow}, portfolio: portfolio, market: market, wantStatus: SimulatedRejected, wantReason: "invalid_simulation_input"},
		{name: "margin", action: func() risk.ProposedAction { copy := action; copy.RequiresMargin = true; return copy }(), evaluation: risk.RiskEvaluation{Decision: risk.Allow}, portfolio: portfolio, market: market, wantStatus: SimulatedRejected, wantReason: "invalid_simulation_input"},
		{name: "excessive assumptions", action: action, evaluation: risk.RiskEvaluation{Decision: risk.Allow}, portfolio: portfolio, market: market, config: AIPaperSimulationConfig{FeeBasisPoints: 1001}, wantStatus: SimulatedRejected, wantReason: "invalid_simulation_input"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := SimulateAIPaperSpotFill(test.action, test.evaluation, "CRYPTO", test.portfolio, test.market, test.config, now)
			if got.Status != test.wantStatus || got.Reason != test.wantReason || !got.SimulationOnly {
				t.Fatalf("unexpected result: %#v", got)
			}
		})
	}
}

func TestSimulateAIPaperSpotFillRoundsConservatively(t *testing.T) {
	now := time.Date(2026, 8, 28, 15, 0, 0, 0, time.UTC)
	price := "0.3333333333"
	fill := SimulateAIPaperSpotFill(
		risk.ProposedAction{Source: risk.SourceAI, ActionType: risk.ActionBuy, Instrument: "XRP", Side: "BUY", Quantity: "3.0000000000", Notional: "1.0000000000", EstimatedPrice: &price},
		risk.RiskEvaluation{Decision: risk.Allow}, "CRYPTO",
		AIPaperPortfolioSnapshot{Currency: "USD", Cash: "2.0000000000", Positions: map[string]string{}},
		AIPaperMarketReference{Symbol: "XRP", Price: price, Basis: "ASK", Provider: "coinbase", Feed: "advanced_trade", Quality: "BROKER_REALTIME", ObservedAt: now},
		AIPaperSimulationConfig{FeeBasisPoints: 1, SlippageBasisPoints: 1}, now,
	)
	if fill.Status != SimulatedFilled || fill.FillPrice != "0.3333666667" || fill.GrossNotional != "1.0001000001" || fill.Fee != "0.0001000101" || fill.CashDelta != "-1.0002000102" {
		t.Fatalf("buy-side rounding was not conservative: %#v", fill)
	}
}
