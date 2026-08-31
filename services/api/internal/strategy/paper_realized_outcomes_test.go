package strategy

import (
	"testing"
	"time"
)

func TestProjectPaperRealizedOutcomeReplaysExactAverageCost(t *testing.T) {
	first := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	portfolio := PaperPortfolio{
		Cash: "842.1500000000",
		Positions: []PaperPosition{{
			Symbol: "BTC", Instrument: "CRYPTO", Quantity: "1.5000000000", AveragePrice: "111.1000000000", IsOpen: true,
		}},
	}
	fills := []paperRealizedFill{
		{
			ID: "buy-1", Symbol: "BTC", Instrument: "CRYPTO", Side: "BUY", Quantity: "1.0000000000",
			GrossNotional: "100.0000000000", Fee: "1.0000000000", PreviousCash: "1000.0000000000",
			PreviousPositionQuantity: "0.0000000000", ResultingCash: "899.0000000000", ResultingPositionQuantity: "1.0000000000", SimulatedAt: first, SimulationOnly: true,
		},
		{
			ID: "buy-2", Symbol: "BTC", Instrument: "CRYPTO", Side: "BUY", Quantity: "1.0000000000",
			GrossNotional: "120.0000000000", Fee: "1.2000000000", PreviousCash: "899.0000000000",
			PreviousPositionQuantity: "1.0000000000", ResultingCash: "777.8000000000", ResultingPositionQuantity: "2.0000000000", SimulatedAt: first.Add(time.Hour), SimulationOnly: true,
		},
		{
			ID: "sell-1", Symbol: "BTC", Instrument: "CRYPTO", Side: "SELL", Quantity: "0.5000000000",
			GrossNotional: "65.0000000000", Fee: "0.6500000000", PreviousCash: "777.8000000000",
			PreviousPositionQuantity: "2.0000000000", ResultingCash: "842.1500000000", ResultingPositionQuantity: "1.5000000000", SimulatedAt: first.Add(2 * time.Hour), SimulationOnly: true,
		},
	}

	outcome := projectPaperRealizedOutcome("1000.0000000000", portfolio, fills)
	if outcome.Status != PaperRealizedAvailable || outcome.CalculationMethod != PaperAverageCostIncludedFees || outcome.HistoricalCoverage != PaperCoverageCompleteGenesis {
		t.Fatalf("realized contract unavailable: %#v", outcome)
	}
	if outcome.TotalRealizedProfitLoss != "8.8000000000" || outcome.FillCount != 3 || outcome.SellFillCount != 1 || outcome.FirstFillAt == nil || outcome.LastFillAt == nil {
		t.Fatalf("realized totals changed: %#v", outcome)
	}
	if len(outcome.Symbols) != 1 {
		t.Fatalf("symbol attribution missing: %#v", outcome.Symbols)
	}
	symbol := outcome.Symbols[0]
	if symbol.Symbol != "BTC" || symbol.Instrument != "CRYPTO" || symbol.RealizedProfitLoss != "8.8000000000" || symbol.TotalFees != "2.8500000000" || symbol.BuyFillCount != 2 || symbol.SellFillCount != 1 || symbol.EndingPositionQuantity != "1.5000000000" || symbol.EndingAverageCost != "111.1000000000" {
		t.Fatalf("symbol outcome changed: %#v", symbol)
	}
}

func TestProjectPaperRealizedOutcomeFailsClosedOnBrokenChain(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	portfolio := PaperPortfolio{Cash: "899.0000000000", Positions: []PaperPosition{{Symbol: "BTC", Instrument: "CRYPTO", Quantity: "1.0000000000", AveragePrice: "101.0000000000"}}}
	fills := []paperRealizedFill{{
		ID: "buy-1", Symbol: "BTC", Instrument: "CRYPTO", Side: "BUY", Quantity: "1.0000000000",
		GrossNotional: "100.0000000000", Fee: "1.0000000000", PreviousCash: "999.0000000000",
		PreviousPositionQuantity: "0", ResultingCash: "898.0000000000", ResultingPositionQuantity: "1", SimulatedAt: now, SimulationOnly: true,
	}}
	outcome := projectPaperRealizedOutcome("1000.0000000000", portfolio, fills)
	if outcome.Status != PaperRealizedUnavailable || outcome.TotalRealizedProfitLoss != "" || len(outcome.Symbols) != 0 {
		t.Fatalf("broken fill chain was inferred: %#v", outcome)
	}
}

func TestProjectPaperRealizedOutcomeProvesEmptyGenesis(t *testing.T) {
	outcome := projectPaperRealizedOutcome("1000", PaperPortfolio{Cash: "1000", Positions: []PaperPosition{}}, nil)
	if outcome.Status != PaperRealizedNoSales || outcome.TotalRealizedProfitLoss != "0.0000000000" || outcome.FillCount != 0 || outcome.SellFillCount != 0 {
		t.Fatalf("empty complete ledger changed: %#v", outcome)
	}
}

func TestProjectPaperRealizedOutcomeRejectsAnyNonSimulationFill(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	portfolio := PaperPortfolio{Cash: "899.0000000000", Positions: []PaperPosition{{Symbol: "BTC", Instrument: "CRYPTO", Quantity: "1.0000000000", AveragePrice: "101.0000000000"}}}
	fills := []paperRealizedFill{{
		ID: "buy-1", Symbol: "BTC", Instrument: "CRYPTO", Side: "BUY", Quantity: "1.0000000000",
		GrossNotional: "100.0000000000", Fee: "1.0000000000", PreviousCash: "1000.0000000000",
		PreviousPositionQuantity: "0", ResultingCash: "899.0000000000", ResultingPositionQuantity: "1", SimulatedAt: now, SimulationOnly: false,
	}}
	outcome := projectPaperRealizedOutcome("1000.0000000000", portfolio, fills)
	if outcome.Status != PaperRealizedUnavailable {
		t.Fatalf("non-simulation history was accepted: %#v", outcome)
	}
}
