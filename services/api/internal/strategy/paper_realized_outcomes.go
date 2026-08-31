package strategy

import (
	"math/big"
	"sort"
	"strings"
	"time"
)

const (
	PaperRealizedAvailable       = "AVAILABLE"
	PaperRealizedNoSales         = "NO_REALIZED_SALES"
	PaperRealizedUnavailable     = "UNAVAILABLE"
	PaperAverageCostIncludedFees = "AVERAGE_COST_INCLUDED_FEES"
	PaperCoverageCompleteGenesis = "COMPLETE_FROM_PORTFOLIO_GENESIS"
)

type paperRealizedFill struct {
	ID                        string
	Symbol                    string
	Instrument                string
	Side                      string
	Quantity                  string
	GrossNotional             string
	Fee                       string
	PreviousCash              string
	PreviousPositionQuantity  string
	ResultingCash             string
	ResultingPositionQuantity string
	SimulatedAt               time.Time
	SimulationOnly            bool
}

type paperRealizedPosition struct {
	quantity      *big.Rat
	averageCost   *big.Rat
	realized      *big.Rat
	fees          *big.Rat
	buyFillCount  int
	sellFillCount int
}

func paperRat(value string) (*big.Rat, bool) {
	v, ok := new(big.Rat).SetString(value)
	return v, ok
}

func paperZero() *big.Rat { return new(big.Rat) }

func paperAdd(left, right *big.Rat) *big.Rat {
	return new(big.Rat).Add(left, right)
}

func paperSub(left, right *big.Rat) *big.Rat {
	return new(big.Rat).Sub(left, right)
}

func paperMul(left, right *big.Rat) *big.Rat {
	return new(big.Rat).Mul(left, right)
}

func paperDecimal(value *big.Rat) string { return value.FloatString(10) }

func unavailablePaperRealizedOutcome(fillCount int) PaperRealizedOutcome {
	return PaperRealizedOutcome{
		Status:    PaperRealizedUnavailable,
		FillCount: fillCount,
		Symbols:   []PaperRealizedSymbolOutcome{},
	}
}

// projectPaperRealizedOutcome replays the immutable fill chain from portfolio
// genesis. It validates quantity and cash transitions before calculating any
// owner-facing result. One broken link makes the whole projection unavailable.
func projectPaperRealizedOutcome(
	startingCash string,
	portfolio PaperPortfolio,
	fills []paperRealizedFill,
) PaperRealizedOutcome {
	initialCash, ok := paperRat(startingCash)
	if !ok || initialCash.Sign() <= 0 {
		return unavailablePaperRealizedOutcome(len(fills))
	}
	if len(fills) == 0 {
		if len(portfolio.Positions) != 0 {
			return unavailablePaperRealizedOutcome(0)
		}
		return PaperRealizedOutcome{
			Status:                  PaperRealizedNoSales,
			CalculationMethod:       PaperAverageCostIncludedFees,
			HistoricalCoverage:      PaperCoverageCompleteGenesis,
			TotalRealizedProfitLoss: "0.0000000000",
			Symbols:                 []PaperRealizedSymbolOutcome{},
		}
	}

	positions := map[string]*paperRealizedPosition{}
	currentCash := new(big.Rat).Set(initialCash)
	totalRealized := paperZero()
	var firstFillAt, lastFillAt time.Time
	totalSellFills := 0
	seenIDs := map[string]struct{}{}

	for index, fill := range fills {
		if fill.ID == "" || fill.Symbol == "" || (fill.Instrument != "EQUITY" && fill.Instrument != "CRYPTO") || (fill.Side != "BUY" && fill.Side != "SELL") || fill.SimulatedAt.IsZero() || !fill.SimulationOnly {
			return unavailablePaperRealizedOutcome(len(fills))
		}
		if _, exists := seenIDs[fill.ID]; exists {
			return unavailablePaperRealizedOutcome(len(fills))
		}
		seenIDs[fill.ID] = struct{}{}
		if index > 0 && fill.SimulatedAt.Before(fills[index-1].SimulatedAt) {
			return unavailablePaperRealizedOutcome(len(fills))
		}
		quantity, quantityOK := paperRat(fill.Quantity)
		gross, grossOK := paperRat(fill.GrossNotional)
		fee, feeOK := paperRat(fill.Fee)
		previousCash, previousCashOK := paperRat(fill.PreviousCash)
		previousQuantity, previousQuantityOK := paperRat(fill.PreviousPositionQuantity)
		resultingCash, resultingCashOK := paperRat(fill.ResultingCash)
		resultingQuantity, resultingQuantityOK := paperRat(fill.ResultingPositionQuantity)
		if !quantityOK || !grossOK || !feeOK || !previousCashOK || !previousQuantityOK || !resultingCashOK || !resultingQuantityOK || quantity.Sign() <= 0 || gross.Sign() <= 0 || fee.Sign() < 0 || previousCash.Cmp(currentCash) != 0 {
			return unavailablePaperRealizedOutcome(len(fills))
		}

		key := fill.Instrument + ":" + fill.Symbol
		position := positions[key]
		if position == nil {
			position = &paperRealizedPosition{
				quantity:    paperZero(),
				averageCost: paperZero(),
				realized:    paperZero(),
				fees:        paperZero(),
			}
			positions[key] = position
		}
		if previousQuantity.Cmp(position.quantity) != 0 {
			return unavailablePaperRealizedOutcome(len(fills))
		}

		position.fees = paperAdd(position.fees, fee)
		if fill.Side == "BUY" {
			expectedQuantity := paperAdd(position.quantity, quantity)
			expectedCash := paperSub(currentCash, paperAdd(gross, fee))
			if expectedQuantity.Cmp(resultingQuantity) != 0 || expectedCash.Cmp(resultingCash) != 0 {
				return unavailablePaperRealizedOutcome(len(fills))
			}
			cost := paperAdd(paperMul(position.quantity, position.averageCost), paperAdd(gross, fee))
			averageCostText := new(big.Rat).Quo(cost, expectedQuantity).FloatString(10)
			averageCost, averageOK := paperRat(averageCostText)
			if !averageOK {
				return unavailablePaperRealizedOutcome(len(fills))
			}
			position.quantity = expectedQuantity
			position.averageCost = averageCost
			position.buyFillCount++
		} else {
			if position.quantity.Cmp(quantity) < 0 {
				return unavailablePaperRealizedOutcome(len(fills))
			}
			expectedQuantity := paperSub(position.quantity, quantity)
			expectedCash := paperAdd(currentCash, paperSub(gross, fee))
			if expectedQuantity.Cmp(resultingQuantity) != 0 || expectedCash.Cmp(resultingCash) != 0 {
				return unavailablePaperRealizedOutcome(len(fills))
			}
			realizedText := paperSub(paperSub(gross, fee), paperMul(quantity, position.averageCost)).FloatString(10)
			realized, realizedOK := paperRat(realizedText)
			if !realizedOK {
				return unavailablePaperRealizedOutcome(len(fills))
			}
			position.realized = paperAdd(position.realized, realized)
			totalRealized = paperAdd(totalRealized, realized)
			position.quantity = expectedQuantity
			if expectedQuantity.Sign() == 0 {
				position.averageCost = paperZero()
			}
			position.sellFillCount++
			totalSellFills++
		}
		currentCash = resultingCash
		if index == 0 {
			firstFillAt = fill.SimulatedAt
		}
		lastFillAt = fill.SimulatedAt
	}

	portfolioCash, cashOK := paperRat(portfolio.Cash)
	if !cashOK || portfolioCash.Cmp(currentCash) != 0 {
		return unavailablePaperRealizedOutcome(len(fills))
	}
	portfolioPositions := map[string]PaperPosition{}
	for _, position := range portfolio.Positions {
		key := position.Instrument + ":" + position.Symbol
		if _, exists := portfolioPositions[key]; exists {
			return unavailablePaperRealizedOutcome(len(fills))
		}
		portfolioPositions[key] = position
	}
	for key, state := range positions {
		stored, exists := portfolioPositions[key]
		if !exists {
			return unavailablePaperRealizedOutcome(len(fills))
		}
		storedQuantity, quantityOK := paperRat(stored.Quantity)
		storedAverage, averageOK := paperRat(stored.AveragePrice)
		if !quantityOK || !averageOK || storedQuantity.Cmp(state.quantity) != 0 || (state.quantity.Sign() != 0 && storedAverage.Cmp(state.averageCost) != 0) {
			return unavailablePaperRealizedOutcome(len(fills))
		}
		delete(portfolioPositions, key)
	}
	if len(portfolioPositions) != 0 {
		return unavailablePaperRealizedOutcome(len(fills))
	}

	keys := make([]string, 0, len(positions))
	for key := range positions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	symbols := make([]PaperRealizedSymbolOutcome, 0, len(keys))
	for _, key := range keys {
		state := positions[key]
		separator := strings.IndexByte(key, ':')
		instrument, symbol := key[:separator], key[separator+1:]
		endingAverage := ""
		if state.quantity.Sign() != 0 {
			endingAverage = paperDecimal(state.averageCost)
		}
		symbols = append(symbols, PaperRealizedSymbolOutcome{
			Symbol:                 symbol,
			Instrument:             instrument,
			RealizedProfitLoss:     paperDecimal(state.realized),
			BuyFillCount:           state.buyFillCount,
			SellFillCount:          state.sellFillCount,
			TotalFees:              paperDecimal(state.fees),
			EndingPositionQuantity: paperDecimal(state.quantity),
			EndingAverageCost:      endingAverage,
		})
	}
	status := PaperRealizedAvailable
	if totalSellFills == 0 {
		status = PaperRealizedNoSales
	}
	return PaperRealizedOutcome{
		Status:                  status,
		CalculationMethod:       PaperAverageCostIncludedFees,
		HistoricalCoverage:      PaperCoverageCompleteGenesis,
		TotalRealizedProfitLoss: paperDecimal(totalRealized),
		FillCount:               len(fills),
		SellFillCount:           totalSellFills,
		FirstFillAt:             &firstFillAt,
		LastFillAt:              &lastFillAt,
		Symbols:                 symbols,
	}
}
