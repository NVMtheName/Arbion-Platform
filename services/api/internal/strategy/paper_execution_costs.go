package strategy

import (
	"math/big"
	"sort"
	"strings"
	"time"
)

const (
	PaperExecutionCostsAvailable   = "AVAILABLE"
	PaperExecutionCostsNoFills     = "NO_FILLS"
	PaperExecutionCostsUnavailable = "UNAVAILABLE"
	PaperExecutionCostMethod       = "SAVED_REFERENCE_VERSUS_SIMULATED_FILL"
)

type paperExecutionSymbolState struct {
	fees, slippage, gross          *big.Rat
	fillCount, buyCount, sellCount int
}

func paperValueIsUnique(seen map[string]struct{}, value string) bool {
	if _, exists := seen[value]; exists {
		return false
	}
	seen[value] = struct{}{}
	return true
}

func unavailablePaperExecutionCosts(fillCount int) PaperExecutionCosts {
	return PaperExecutionCosts{
		Status: PaperExecutionCostsUnavailable, FillCount: fillCount,
		MarketProviders: []string{}, MarketFeeds: []string{}, MarketQualities: []string{}, Symbols: []PaperExecutionSymbolCost{},
	}
}

func sortedPaperSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// projectPaperExecutionCosts attributes only explicit simulation costs saved in
// immutable fill evidence. It never estimates spreads, fees, or broker costs.
func projectPaperExecutionCosts(realized PaperRealizedOutcome, fills []paperRealizedFill) PaperExecutionCosts {
	if realized.Status == PaperRealizedUnavailable || realized.FillCount != len(fills) {
		return unavailablePaperExecutionCosts(len(fills))
	}
	if len(fills) == 0 {
		return PaperExecutionCosts{
			Status: PaperExecutionCostsNoFills, CalculationMethod: PaperExecutionCostMethod, HistoricalCoverage: PaperCoverageCompleteGenesis,
			TotalFees: "0.0000000000", TotalAdverseSlippage: "0.0000000000", GrossNotional: "0.0000000000",
			MarketProviders: []string{}, MarketFeeds: []string{}, MarketQualities: []string{}, Symbols: []PaperExecutionSymbolCost{},
		}
	}

	totalFees, totalSlippage, totalGross := paperZero(), paperZero(), paperZero()
	states := map[string]*paperExecutionSymbolState{}
	providers, feeds, qualities := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	seenFill, seenExecution, seenAction, seenRisk := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	buyCount, sellCount := 0, 0
	firstFillAt, lastFillAt := fills[0].SimulatedAt, fills[0].SimulatedAt
	for index, fill := range fills {
		if fill.ID == "" || fill.ExecutionRecordID == "" || fill.ProposedActionID == "" || fill.RiskEvaluationID == "" || fill.Symbol == "" ||
			(fill.Instrument != "EQUITY" && fill.Instrument != "CRYPTO") || (fill.Side != "BUY" && fill.Side != "SELL") || fill.PricingBasis == "" ||
			fill.MarketProvider == "" || fill.MarketFeed == "" || fill.MarketQuality == "" || fill.MarketObservedAt.IsZero() || fill.SimulatedAt.IsZero() || !fill.SimulationOnly ||
			fill.MarketObservedAt.Before(fill.SimulatedAt.Add(-2*time.Minute)) || fill.MarketObservedAt.After(fill.SimulatedAt.Add(5*time.Second)) ||
			(index > 0 && fill.SimulatedAt.Before(fills[index-1].SimulatedAt)) {
			return unavailablePaperExecutionCosts(len(fills))
		}
		if !paperValueIsUnique(seenFill, fill.ID) || !paperValueIsUnique(seenExecution, fill.ExecutionRecordID) ||
			!paperValueIsUnique(seenAction, fill.ProposedActionID) || !paperValueIsUnique(seenRisk, fill.RiskEvaluationID) {
			return unavailablePaperExecutionCosts(len(fills))
		}
		quantity, qOK := paperRat(fill.Quantity)
		requested, rqOK := paperRat(fill.RequestedNotional)
		reference, rOK := paperRat(fill.ReferencePrice)
		fillPrice, fpOK := paperRat(fill.FillPrice)
		gross, gOK := paperRat(fill.GrossNotional)
		fee, feeOK := paperRat(fill.Fee)
		if !qOK || !rqOK || !rOK || !fpOK || !gOK || !feeOK || quantity.Sign() <= 0 || requested.Sign() <= 0 || reference.Sign() <= 0 || fillPrice.Sign() <= 0 || gross.Sign() <= 0 || fee.Sign() < 0 {
			return unavailablePaperExecutionCosts(len(fills))
		}
		referenceNotional := paperMul(quantity, reference)
		if requested.Cmp(referenceNotional) < 0 {
			return unavailablePaperExecutionCosts(len(fills))
		}
		storedFillNotional, storedFillNotionalOK := paperRat(paperDecimal(paperMul(quantity, fillPrice)))
		decimalStorageBound, decimalStorageBoundOK := paperRat("0.0000000001")
		if !storedFillNotionalOK || !decimalStorageBoundOK {
			return unavailablePaperExecutionCosts(len(fills))
		}
		fillResidual := paperSub(storedFillNotional, gross)
		if fillResidual.Sign() < 0 {
			fillResidual.Neg(fillResidual)
		}
		if fillResidual.Cmp(decimalStorageBound) > 0 {
			return unavailablePaperExecutionCosts(len(fills))
		}
		var slippage *big.Rat
		if fill.Side == "BUY" {
			if fillPrice.Cmp(reference) < 0 {
				return unavailablePaperExecutionCosts(len(fills))
			}
			slippage = paperSub(gross, referenceNotional)
			buyCount++
		} else {
			if fillPrice.Cmp(reference) > 0 {
				return unavailablePaperExecutionCosts(len(fills))
			}
			slippage = paperSub(referenceNotional, gross)
			sellCount++
		}
		if slippage.Sign() < 0 {
			return unavailablePaperExecutionCosts(len(fills))
		}
		key := fill.Instrument + ":" + fill.Symbol
		state := states[key]
		if state == nil {
			state = &paperExecutionSymbolState{fees: paperZero(), slippage: paperZero(), gross: paperZero()}
			states[key] = state
		}
		state.fees = paperAdd(state.fees, fee)
		state.slippage = paperAdd(state.slippage, slippage)
		state.gross = paperAdd(state.gross, gross)
		state.fillCount++
		if fill.Side == "BUY" {
			state.buyCount++
		} else {
			state.sellCount++
		}
		totalFees = paperAdd(totalFees, fee)
		totalSlippage = paperAdd(totalSlippage, slippage)
		totalGross = paperAdd(totalGross, gross)
		providers[fill.MarketProvider] = struct{}{}
		feeds[fill.MarketFeed] = struct{}{}
		qualities[fill.MarketQuality] = struct{}{}
		lastFillAt = fill.SimulatedAt
	}
	keys := make([]string, 0, len(states))
	for key := range states {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	symbols := make([]PaperExecutionSymbolCost, 0, len(keys))
	for _, key := range keys {
		separator := strings.IndexByte(key, ':')
		state := states[key]
		symbols = append(symbols, PaperExecutionSymbolCost{Symbol: key[separator+1:], Instrument: key[:separator], TotalFees: paperDecimal(state.fees), AdverseSlippage: paperDecimal(state.slippage), GrossNotional: paperDecimal(state.gross), FillCount: state.fillCount, BuyFillCount: state.buyCount, SellFillCount: state.sellCount})
	}
	return PaperExecutionCosts{
		Status: PaperExecutionCostsAvailable, CalculationMethod: PaperExecutionCostMethod, HistoricalCoverage: PaperCoverageCompleteGenesis,
		TotalFees: paperDecimal(totalFees), TotalAdverseSlippage: paperDecimal(totalSlippage), GrossNotional: paperDecimal(totalGross),
		FillCount: len(fills), BuyFillCount: buyCount, SellFillCount: sellCount, FirstFillAt: &firstFillAt, LastFillAt: &lastFillAt,
		MarketProviders: sortedPaperSet(providers), MarketFeeds: sortedPaperSet(feeds), MarketQualities: sortedPaperSet(qualities), Symbols: symbols,
	}
}
