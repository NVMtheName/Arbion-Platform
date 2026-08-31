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
	PaperExecutionTimelineLimit    = 12
	PaperTradeSequenceMethod       = "COMPLETE_IMMUTABLE_FILL_SEQUENCE"
)

type paperExecutionSymbolState struct {
	fees, slippage, reference, gross *big.Rat
	fillCount, buyCount, sellCount   int
}

type paperTradeSequenceState struct {
	symbol, instrument, firstSide, lastSide        string
	fillCount, buyCount, sellCount                 int
	sameCount, oppositeCount, buyToSell, sellToBuy int
	currentSameSideStreak, longestSameSideStreak   int
	firstFillAt, lastFillAt                        time.Time
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
		MarketProviders: []string{}, MarketFeeds: []string{}, MarketQualities: []string{}, Sides: []PaperExecutionSideCost{}, Symbols: []PaperExecutionSymbolCost{}, Timeline: []PaperExecutionCheckpoint{},
		TradeSequence: PaperTradeSequenceEvidence{Status: PaperExecutionCostsUnavailable, FillCount: fillCount, Symbols: []PaperTradeSequenceSymbol{}},
	}
}

func paperExecutionCostRate(cost, reference *big.Rat) (string, bool) {
	if reference.Sign() <= 0 {
		return "", false
	}
	rate := new(big.Rat).Mul(new(big.Rat).Quo(cost, reference), big.NewRat(10000, 1))
	return paperDecimal(rate), true
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
func projectPaperExecutionCosts(realized PaperRealizedOutcome, startingCash string, fills []paperRealizedFill) PaperExecutionCosts {
	if realized.Status == PaperRealizedUnavailable || realized.FillCount != len(fills) {
		return unavailablePaperExecutionCosts(len(fills))
	}
	startingCashValue, startingCashOK := paperRat(startingCash)
	if !startingCashOK || startingCashValue.Sign() <= 0 {
		return unavailablePaperExecutionCosts(len(fills))
	}
	if len(fills) == 0 {
		return PaperExecutionCosts{
			Status: PaperExecutionCostsNoFills, CalculationMethod: PaperExecutionCostMethod, HistoricalCoverage: PaperCoverageCompleteGenesis,
			TotalFees: "0.0000000000", TotalAdverseSlippage: "0.0000000000", TotalExplicitCost: "0.0000000000",
			ProviderReferenceNotional: "0.0000000000", GrossNotional: "0.0000000000", AllInCostRateBPS: "0.0000000000",
			FillNotionalResidual: "0.0000000000", MaximumAbsoluteFillResidual: "0.0000000000", ResidualBoundPerFill: "0.0000000001",
			MarketProviders: []string{}, MarketFeeds: []string{}, MarketQualities: []string{}, Sides: []PaperExecutionSideCost{}, Symbols: []PaperExecutionSymbolCost{}, Timeline: []PaperExecutionCheckpoint{},
			TradeSequence: PaperTradeSequenceEvidence{Status: PaperExecutionCostsNoFills, CalculationMethod: PaperTradeSequenceMethod, HistoricalCoverage: PaperCoverageCompleteGenesis,
				StartingCash: paperDecimal(startingCashValue), ProviderReferenceTurnoverToStartingCashBPS: "0.0000000000", ExplicitCostToStartingCashBPS: "0.0000000000", Symbols: []PaperTradeSequenceSymbol{}},
		}
	}

	totalFees, totalSlippage, totalReference, totalGross := paperZero(), paperZero(), paperZero(), paperZero()
	totalResidual, maximumAbsoluteResidual := paperZero(), paperZero()
	timeline := make([]PaperExecutionCheckpoint, 0, len(fills))
	var previousCumulativeRate *big.Rat
	states := map[string]*paperExecutionSymbolState{}
	sequenceStates := map[string]*paperTradeSequenceState{}
	sideStates := map[string]*paperExecutionSymbolState{}
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
		signedFillResidual := paperSub(storedFillNotional, gross)
		totalResidual = paperAdd(totalResidual, signedFillResidual)
		absoluteFillResidual := new(big.Rat).Set(signedFillResidual)
		if absoluteFillResidual.Sign() < 0 {
			absoluteFillResidual.Neg(absoluteFillResidual)
		}
		if absoluteFillResidual.Cmp(decimalStorageBound) > 0 {
			return unavailablePaperExecutionCosts(len(fills))
		}
		if absoluteFillResidual.Cmp(maximumAbsoluteResidual) > 0 {
			maximumAbsoluteResidual = new(big.Rat).Set(absoluteFillResidual)
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
			state = &paperExecutionSymbolState{fees: paperZero(), slippage: paperZero(), reference: paperZero(), gross: paperZero()}
			states[key] = state
		}
		sideState := sideStates[fill.Side]
		if sideState == nil {
			sideState = &paperExecutionSymbolState{fees: paperZero(), slippage: paperZero(), reference: paperZero(), gross: paperZero()}
			sideStates[fill.Side] = sideState
		}
		state.fees = paperAdd(state.fees, fee)
		state.slippage = paperAdd(state.slippage, slippage)
		state.reference = paperAdd(state.reference, referenceNotional)
		state.gross = paperAdd(state.gross, gross)
		state.fillCount++
		sideState.fees = paperAdd(sideState.fees, fee)
		sideState.slippage = paperAdd(sideState.slippage, slippage)
		sideState.reference = paperAdd(sideState.reference, referenceNotional)
		sideState.gross = paperAdd(sideState.gross, gross)
		sideState.fillCount++
		if fill.Side == "BUY" {
			state.buyCount++
		} else {
			state.sellCount++
		}
		totalFees = paperAdd(totalFees, fee)
		totalSlippage = paperAdd(totalSlippage, slippage)
		totalReference = paperAdd(totalReference, referenceNotional)
		totalGross = paperAdd(totalGross, gross)
		sequenceState := sequenceStates[key]
		sideTransition, oppositeSideElapsed := "FIRST", ""
		if sequenceState == nil {
			sequenceState = &paperTradeSequenceState{symbol: fill.Symbol, instrument: fill.Instrument, firstSide: fill.Side, lastSide: fill.Side,
				currentSameSideStreak: 1, longestSameSideStreak: 1, firstFillAt: fill.SimulatedAt, lastFillAt: fill.SimulatedAt}
			sequenceStates[key] = sequenceState
		} else if fill.Side == sequenceState.lastSide {
			sideTransition = "SAME_SIDE"
			sequenceState.sameCount++
			sequenceState.currentSameSideStreak++
			if sequenceState.currentSameSideStreak > sequenceState.longestSameSideStreak {
				sequenceState.longestSameSideStreak = sequenceState.currentSameSideStreak
			}
		} else {
			oppositeSideElapsed = paperDecimal(new(big.Rat).SetFrac(big.NewInt(fill.SimulatedAt.Sub(sequenceState.lastFillAt).Nanoseconds()), big.NewInt(int64(time.Second))))
			sequenceState.oppositeCount++
			sequenceState.currentSameSideStreak = 1
			if sequenceState.lastSide == "BUY" {
				sideTransition = "BUY_TO_SELL"
				sequenceState.buyToSell++
			} else {
				sideTransition = "SELL_TO_BUY"
				sequenceState.sellToBuy++
			}
		}
		sequenceState.fillCount++
		if fill.Side == "BUY" {
			sequenceState.buyCount++
		} else {
			sequenceState.sellCount++
		}
		sequenceState.lastSide = fill.Side
		sequenceState.lastFillAt = fill.SimulatedAt
		fillExplicitCost := paperAdd(fee, slippage)
		cumulativeExplicitCost := paperAdd(totalFees, totalSlippage)
		cumulativeRate := new(big.Rat).Mul(new(big.Rat).Quo(cumulativeExplicitCost, totalReference), big.NewRat(10000, 1))
		cumulativeRateText := paperDecimal(cumulativeRate)
		savedCumulativeRate, savedRateOK := paperRat(cumulativeRateText)
		if !savedRateOK {
			return unavailablePaperExecutionCosts(len(fills))
		}
		rateChange := "FIRST"
		if previousCumulativeRate != nil {
			switch savedCumulativeRate.Cmp(previousCumulativeRate) {
			case -1:
				rateChange = "FELL"
			case 0:
				rateChange = "HELD"
			case 1:
				rateChange = "ROSE"
			}
		}
		previousCumulativeRate = new(big.Rat).Set(savedCumulativeRate)
		timeline = append(timeline, PaperExecutionCheckpoint{
			Sequence: index + 1, FillID: fill.ID, ExecutionRecordID: fill.ExecutionRecordID, ProposedActionID: fill.ProposedActionID, RiskEvaluationID: fill.RiskEvaluationID,
			Symbol: fill.Symbol, Instrument: fill.Instrument, Side: fill.Side, Fee: paperDecimal(fee), AdverseSlippage: paperDecimal(slippage),
			ExplicitCost: paperDecimal(fillExplicitCost), ProviderReferenceNotional: paperDecimal(referenceNotional), GrossNotional: paperDecimal(gross), FillNotionalResidual: paperDecimal(signedFillResidual),
			CumulativeFees: paperDecimal(totalFees), CumulativeAdverseSlippage: paperDecimal(totalSlippage), CumulativeExplicitCost: paperDecimal(cumulativeExplicitCost),
			CumulativeProviderReferenceNotional: paperDecimal(totalReference), CumulativeGrossNotional: paperDecimal(totalGross), CumulativeAllInCostRateBPS: cumulativeRateText, CumulativeRateChange: rateChange,
			SymbolSequence: sequenceState.fillCount, SameSideStreak: sequenceState.currentSameSideStreak, SideTransition: sideTransition, OppositeSideElapsedSeconds: oppositeSideElapsed,
			MarketProvider: fill.MarketProvider, MarketFeed: fill.MarketFeed, MarketQuality: fill.MarketQuality, MarketObservedAt: fill.MarketObservedAt, SimulatedAt: fill.SimulatedAt,
		})
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
		explicitCost := paperAdd(state.fees, state.slippage)
		costRate, ok := paperExecutionCostRate(explicitCost, state.reference)
		if !ok {
			return unavailablePaperExecutionCosts(len(fills))
		}
		symbols = append(symbols, PaperExecutionSymbolCost{Symbol: key[separator+1:], Instrument: key[:separator], TotalFees: paperDecimal(state.fees), AdverseSlippage: paperDecimal(state.slippage), TotalExplicitCost: paperDecimal(explicitCost), ProviderReferenceNotional: paperDecimal(state.reference), GrossNotional: paperDecimal(state.gross), AllInCostRateBPS: costRate, FillCount: state.fillCount, BuyFillCount: state.buyCount, SellFillCount: state.sellCount})
	}
	sides := make([]PaperExecutionSideCost, 0, 2)
	for _, side := range []string{"BUY", "SELL"} {
		state := sideStates[side]
		if state == nil {
			continue
		}
		explicitCost := paperAdd(state.fees, state.slippage)
		costRate, ok := paperExecutionCostRate(explicitCost, state.reference)
		if !ok {
			return unavailablePaperExecutionCosts(len(fills))
		}
		sides = append(sides, PaperExecutionSideCost{Side: side, TotalFees: paperDecimal(state.fees), AdverseSlippage: paperDecimal(state.slippage), TotalExplicitCost: paperDecimal(explicitCost), ProviderReferenceNotional: paperDecimal(state.reference), GrossNotional: paperDecimal(state.gross), AllInCostRateBPS: costRate, FillCount: state.fillCount})
	}
	totalExplicitCost := paperAdd(totalFees, totalSlippage)
	totalCostRate, ok := paperExecutionCostRate(totalExplicitCost, totalReference)
	if !ok {
		return unavailablePaperExecutionCosts(len(fills))
	}
	timelineSampleCount := len(timeline)
	timelineCapped := timelineSampleCount > PaperExecutionTimelineLimit
	if timelineCapped {
		timeline = append([]PaperExecutionCheckpoint(nil), timeline[timelineSampleCount-PaperExecutionTimelineLimit:]...)
	}
	sequenceSymbols := make([]PaperTradeSequenceSymbol, 0, len(sequenceStates))
	totalSameSideTransitions, totalOppositeSideTransitions, totalBuyToSell, totalSellToBuy := 0, 0, 0, 0
	for _, key := range keys {
		state := sequenceStates[key]
		firstFillAt, lastFillAt := state.firstFillAt, state.lastFillAt
		sequenceSymbols = append(sequenceSymbols, PaperTradeSequenceSymbol{Symbol: state.symbol, Instrument: state.instrument, FillCount: state.fillCount,
			BuyFillCount: state.buyCount, SellFillCount: state.sellCount, SameSideTransitionCount: state.sameCount, OppositeSideTransitionCount: state.oppositeCount,
			BuyToSellReversalCount: state.buyToSell, SellToBuyReversalCount: state.sellToBuy, LongestSameSideStreak: state.longestSameSideStreak,
			FirstSide: state.firstSide, LastSide: state.lastSide, FirstFillAt: &firstFillAt, LastFillAt: &lastFillAt})
		totalSameSideTransitions += state.sameCount
		totalOppositeSideTransitions += state.oppositeCount
		totalBuyToSell += state.buyToSell
		totalSellToBuy += state.sellToBuy
	}
	turnoverToStartingCash := new(big.Rat).Mul(new(big.Rat).Quo(totalReference, startingCashValue), big.NewRat(10000, 1))
	explicitCostToStartingCash := new(big.Rat).Mul(new(big.Rat).Quo(totalExplicitCost, startingCashValue), big.NewRat(10000, 1))
	return PaperExecutionCosts{
		Status: PaperExecutionCostsAvailable, CalculationMethod: PaperExecutionCostMethod, HistoricalCoverage: PaperCoverageCompleteGenesis,
		TotalFees: paperDecimal(totalFees), TotalAdverseSlippage: paperDecimal(totalSlippage), TotalExplicitCost: paperDecimal(totalExplicitCost),
		ProviderReferenceNotional: paperDecimal(totalReference), GrossNotional: paperDecimal(totalGross), AllInCostRateBPS: totalCostRate,
		FillNotionalResidual: paperDecimal(totalResidual), MaximumAbsoluteFillResidual: paperDecimal(maximumAbsoluteResidual), ResidualBoundPerFill: "0.0000000001",
		FillCount: len(fills), BuyFillCount: buyCount, SellFillCount: sellCount, FirstFillAt: &firstFillAt, LastFillAt: &lastFillAt,
		MarketProviders: sortedPaperSet(providers), MarketFeeds: sortedPaperSet(feeds), MarketQualities: sortedPaperSet(qualities), Sides: sides, Symbols: symbols,
		TimelineSampleCount: timelineSampleCount, TimelineCapped: timelineCapped, Timeline: timeline,
		TradeSequence: PaperTradeSequenceEvidence{Status: PaperExecutionCostsAvailable, CalculationMethod: PaperTradeSequenceMethod, HistoricalCoverage: PaperCoverageCompleteGenesis,
			StartingCash: paperDecimal(startingCashValue), ProviderReferenceTurnoverToStartingCashBPS: paperDecimal(turnoverToStartingCash), ExplicitCostToStartingCashBPS: paperDecimal(explicitCostToStartingCash),
			FillCount: len(fills), SameSideTransitionCount: totalSameSideTransitions, OppositeSideTransitionCount: totalOppositeSideTransitions,
			BuyToSellReversalCount: totalBuyToSell, SellToBuyReversalCount: totalSellToBuy, Symbols: sequenceSymbols},
	}
}
