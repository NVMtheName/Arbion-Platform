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
	outcome := projectPaperExecutionCosts(PaperRealizedOutcome{Status: PaperRealizedAvailable, FillCount: 2}, "1000", fills)
	if outcome.Status != PaperExecutionCostsAvailable || outcome.CalculationMethod != PaperExecutionCostMethod || outcome.HistoricalCoverage != PaperCoverageCompleteGenesis {
		t.Fatalf("execution cost contract unavailable: %#v", outcome)
	}
	if outcome.TotalFees != "0.7755625000" || outcome.TotalAdverseSlippage != "0.3875000000" || outcome.TotalExplicitCost != "1.1630625000" ||
		outcome.ProviderReferenceNotional != "155.0000000000" || outcome.GrossNotional != "155.1125000000" || outcome.AllInCostRateBPS != "75.0362903226" ||
		outcome.FillNotionalResidual != "0.0000000000" || outcome.MaximumAbsoluteFillResidual != "0.0000000000" || outcome.ResidualBoundPerFill != "0.0000000001" ||
		outcome.FillCount != 2 || outcome.BuyFillCount != 1 || outcome.SellFillCount != 1 {
		t.Fatalf("execution cost totals changed: %#v", outcome)
	}
	if len(outcome.Symbols) != 1 || outcome.Symbols[0].Symbol != "BTC" || outcome.Symbols[0].TotalFees != outcome.TotalFees ||
		outcome.Symbols[0].AdverseSlippage != outcome.TotalAdverseSlippage || outcome.Symbols[0].TotalExplicitCost != outcome.TotalExplicitCost ||
		outcome.Symbols[0].ProviderReferenceNotional != outcome.ProviderReferenceNotional || outcome.Symbols[0].AllInCostRateBPS != outcome.AllInCostRateBPS {
		t.Fatalf("symbol attribution changed: %#v", outcome.Symbols)
	}
	if len(outcome.Sides) != 2 || outcome.Sides[0].Side != "BUY" || outcome.Sides[0].TotalExplicitCost != "0.7512500000" ||
		outcome.Sides[0].ProviderReferenceNotional != "100.0000000000" || outcome.Sides[0].AllInCostRateBPS != "75.1250000000" ||
		outcome.Sides[1].Side != "SELL" || outcome.Sides[1].TotalExplicitCost != "0.4118125000" ||
		outcome.Sides[1].ProviderReferenceNotional != "55.0000000000" || outcome.Sides[1].AllInCostRateBPS != "74.8750000000" {
		t.Fatalf("side attribution changed: %#v", outcome.Sides)
	}
	if outcome.TimelineSampleCount != 2 || outcome.TimelineCapped || len(outcome.Timeline) != 2 ||
		outcome.Timeline[0].Sequence != 1 || outcome.Timeline[0].FillID != "buy" || outcome.Timeline[0].CumulativeAllInCostRateBPS != "75.1250000000" || outcome.Timeline[0].CumulativeRateChange != "FIRST" || outcome.Timeline[0].SideTransition != "FIRST" || outcome.Timeline[0].SameSideStreak != 1 ||
		outcome.Timeline[1].Sequence != 2 || outcome.Timeline[1].FillID != "sell" || outcome.Timeline[1].ExplicitCost != "0.4118125000" || outcome.Timeline[1].CumulativeAllInCostRateBPS != outcome.AllInCostRateBPS || outcome.Timeline[1].CumulativeRateChange != "FELL" || outcome.Timeline[1].SideTransition != "BUY_TO_SELL" || outcome.Timeline[1].OppositeSideElapsedSeconds != "3600.0000000000" {
		t.Fatalf("execution cost timeline changed: %#v", outcome.Timeline)
	}
	if outcome.TradeSequence.Status != PaperExecutionCostsAvailable || outcome.TradeSequence.CalculationMethod != PaperTradeSequenceMethod ||
		outcome.TradeSequence.StartingCash != "1000.0000000000" || outcome.TradeSequence.ProviderReferenceTurnoverToStartingCashBPS != "1550.0000000000" ||
		outcome.TradeSequence.ExplicitCostToStartingCashBPS != "11.6306250000" || outcome.TradeSequence.OppositeSideTransitionCount != 1 ||
		outcome.TradeSequence.BuyToSellReversalCount != 1 || outcome.TradeSequence.SellToBuyReversalCount != 0 || len(outcome.TradeSequence.Symbols) != 1 ||
		outcome.TradeSequence.Symbols[0].LongestSameSideStreak != 1 || outcome.TradeSequence.Symbols[0].FirstSide != "BUY" || outcome.TradeSequence.Symbols[0].LastSide != "SELL" {
		t.Fatalf("trade sequence contract changed: %#v", outcome.TradeSequence)
	}
	if len(outcome.MarketProviders) != 1 || outcome.MarketProviders[0] != "coinbase" || len(outcome.MarketFeeds) != 1 || outcome.MarketFeeds[0] != "rest_ticker" {
		t.Fatalf("market attribution changed: %#v", outcome)
	}
}

func TestProjectPaperExecutionCostsProvesEmptyGenesis(t *testing.T) {
	outcome := projectPaperExecutionCosts(PaperRealizedOutcome{Status: PaperRealizedNoSales}, "1000", nil)
	if outcome.Status != PaperExecutionCostsNoFills || outcome.TotalFees != "0.0000000000" || outcome.TotalAdverseSlippage != "0.0000000000" ||
		outcome.TotalExplicitCost != "0.0000000000" || outcome.ProviderReferenceNotional != "0.0000000000" || outcome.AllInCostRateBPS != "0.0000000000" ||
		outcome.FillNotionalResidual != "0.0000000000" || outcome.MaximumAbsoluteFillResidual != "0.0000000000" || len(outcome.Sides) != 0 || len(outcome.Timeline) != 0 || outcome.TimelineSampleCount != 0 || outcome.TimelineCapped || outcome.FillCount != 0 ||
		outcome.TradeSequence.Status != PaperExecutionCostsNoFills || outcome.TradeSequence.StartingCash != "1000.0000000000" || len(outcome.TradeSequence.Symbols) != 0 {
		t.Fatalf("empty execution cost contract changed: %#v", outcome)
	}
}

func TestProjectPaperExecutionCostsFailsClosedWithoutExactStartingCash(t *testing.T) {
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	fill := exactExecutionFill("buy", "BTC", "BUY", "1.0000000000", "100.0000000000", "100.2500000000", "100.2500000000", "0.5012500000", now)
	outcome := projectPaperExecutionCosts(PaperRealizedOutcome{Status: PaperRealizedAvailable, FillCount: 1}, "", []paperRealizedFill{fill})
	if outcome.Status != PaperExecutionCostsUnavailable || outcome.TradeSequence.Status != PaperExecutionCostsUnavailable {
		t.Fatalf("missing starting cash did not fail closed: %#v", outcome)
	}
}

func TestProjectPaperExecutionCostsBoundsTimelineWithoutTruncatingTotals(t *testing.T) {
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	fills := make([]paperRealizedFill, 0, PaperExecutionTimelineLimit+1)
	for index := 0; index < PaperExecutionTimelineLimit+1; index++ {
		id := time.Duration(index).String()
		fills = append(fills, exactExecutionFill(id, "BTC", "BUY", "1.0000000000", "100.0000000000", "100.2500000000", "100.2500000000", "0.5012500000", now.Add(time.Duration(index)*time.Hour)))
	}
	outcome := projectPaperExecutionCosts(PaperRealizedOutcome{Status: PaperRealizedAvailable, FillCount: len(fills)}, "1000", fills)
	if outcome.Status != PaperExecutionCostsAvailable || outcome.FillCount != 13 || outcome.TimelineSampleCount != 13 || !outcome.TimelineCapped || len(outcome.Timeline) != PaperExecutionTimelineLimit {
		t.Fatalf("bounded timeline contract changed: %#v", outcome)
	}
	if outcome.Timeline[0].Sequence != 2 || outcome.Timeline[0].CumulativeRateChange != "HELD" || outcome.Timeline[len(outcome.Timeline)-1].Sequence != 13 || outcome.Timeline[len(outcome.Timeline)-1].CumulativeAllInCostRateBPS != outcome.AllInCostRateBPS {
		t.Fatalf("bounded timeline lost cumulative context: %#v", outcome.Timeline)
	}
}

func TestProjectPaperExecutionCostsFailsClosedWhenStoredNotionalResidualExceedsBound(t *testing.T) {
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	fill := exactExecutionFill("buy", "BTC", "BUY", "1.0000000000", "100.0000000000", "100.2500000000", "100.2499999998", "0.5012500000", now)
	outcome := projectPaperExecutionCosts(PaperRealizedOutcome{Status: PaperRealizedAvailable, FillCount: 1}, "1000", []paperRealizedFill{fill})
	if outcome.Status != PaperExecutionCostsUnavailable || outcome.TotalExplicitCost != "" || len(outcome.Sides) != 0 || len(outcome.Symbols) != 0 {
		t.Fatalf("excess fill-notional residual was inferred: %#v", outcome)
	}
}

func TestProjectPaperExecutionCostsFailsClosedOnInconsistentAttribution(t *testing.T) {
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	fill := exactExecutionFill("buy", "BTC", "BUY", "1.0000000000", "100.0000000000", "100.2500000000", "100.2500000000", "0.5012500000", now)
	fill.MarketProvider = ""
	outcome := projectPaperExecutionCosts(PaperRealizedOutcome{Status: PaperRealizedAvailable, FillCount: 1}, "1000", []paperRealizedFill{fill})
	if outcome.Status != PaperExecutionCostsUnavailable || outcome.TotalFees != "" || len(outcome.Symbols) != 0 {
		t.Fatalf("inconsistent attribution was inferred: %#v", outcome)
	}
}
