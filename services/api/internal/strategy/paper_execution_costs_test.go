package strategy

import (
	"testing"
	"time"
)

func exactExecutionFill(id, symbol, side, quantity, reference, fillPrice, gross, fee string, simulatedAt time.Time) paperRealizedFill {
	return paperRealizedFill{
		ID: id, ExecutionRecordID: "execution-" + id, ProposedActionID: "action-" + id, RiskEvaluationID: "risk-" + id,
		Symbol: symbol, Instrument: "CRYPTO", Side: side, Quantity: quantity, RequestedNotional: "100.0000000000",
		ReferencePrice: reference, FillPrice: fillPrice, GrossNotional: gross, Fee: fee,
		PricingBasis: "ADVERSE_25_BPS_PLUS_FEE_50_BPS", MarketProvider: "coinbase", MarketFeed: "rest_ticker", MarketQuality: "REAL_TIME_SINGLE_VENUE",
		MarketObservedAt: simulatedAt.Add(-time.Second), SimulatedAt: simulatedAt, SimulationOnly: true,
	}
}

func TestProjectPaperExecutionCostsAttributesExactFeesAndAdverseSlippage(t *testing.T) {
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	fills := []paperRealizedFill{
		exactExecutionFill("buy", "BTC", "BUY", "1.0000000000", "100.0000000000", "100.2500000000", "100.2500000000", "0.5012500000", now),
		exactExecutionFill("sell", "BTC", "SELL", "0.5000000000", "110.0000000000", "109.7250000000", "54.8625000000", "0.2743125000", now.Add(time.Hour)),
	}
	outcome := projectPaperExecutionCosts(PaperRealizedOutcome{Status: PaperRealizedAvailable, FillCount: 2}, fills)
	if outcome.Status != PaperExecutionCostsAvailable || outcome.CalculationMethod != PaperExecutionCostMethod || outcome.HistoricalCoverage != PaperCoverageCompleteGenesis {
		t.Fatalf("execution cost contract unavailable: %#v", outcome)
	}
	if outcome.TotalFees != "0.7755625000" || outcome.TotalAdverseSlippage != "0.3875000000" || outcome.GrossNotional != "155.1125000000" || outcome.FillCount != 2 || outcome.BuyFillCount != 1 || outcome.SellFillCount != 1 {
		t.Fatalf("execution cost totals changed: %#v", outcome)
	}
	if len(outcome.Symbols) != 1 || outcome.Symbols[0].Symbol != "BTC" || outcome.Symbols[0].TotalFees != outcome.TotalFees || outcome.Symbols[0].AdverseSlippage != outcome.TotalAdverseSlippage {
		t.Fatalf("symbol attribution changed: %#v", outcome.Symbols)
	}
	if len(outcome.MarketProviders) != 1 || outcome.MarketProviders[0] != "coinbase" || len(outcome.MarketFeeds) != 1 || outcome.MarketFeeds[0] != "rest_ticker" {
		t.Fatalf("market attribution changed: %#v", outcome)
	}
}

func TestProjectPaperExecutionCostsProvesEmptyGenesis(t *testing.T) {
	outcome := projectPaperExecutionCosts(PaperRealizedOutcome{Status: PaperRealizedNoSales}, nil)
	if outcome.Status != PaperExecutionCostsNoFills || outcome.TotalFees != "0.0000000000" || outcome.TotalAdverseSlippage != "0.0000000000" || outcome.FillCount != 0 {
		t.Fatalf("empty execution cost contract changed: %#v", outcome)
	}
}

func TestProjectPaperExecutionCostsFailsClosedOnInconsistentAttribution(t *testing.T) {
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	fill := exactExecutionFill("buy", "BTC", "BUY", "1.0000000000", "100.0000000000", "100.2500000000", "100.2500000000", "0.5012500000", now)
	fill.MarketProvider = ""
	outcome := projectPaperExecutionCosts(PaperRealizedOutcome{Status: PaperRealizedAvailable, FillCount: 1}, []paperRealizedFill{fill})
	if outcome.Status != PaperExecutionCostsUnavailable || outcome.TotalFees != "" || len(outcome.Symbols) != 0 {
		t.Fatalf("inconsistent attribution was inferred: %#v", outcome)
	}
}
