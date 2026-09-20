package executionsim

import (
	"math/big"
	"time"
)

const (
	BookSweepSyntheticSource = "SYNTHETIC_FIXTURE"
	BookSweepFilled          = "FILLED"
	BookSweepDepthExhausted  = "DEPTH_EXHAUSTED"
	BookSweepLimitReached    = "LIMIT_REACHED"

	bookSweepMaximumLevels = 100
	bookSweepMaximumAge    = 24 * time.Hour
)

// BookLevel is an explicitly supplied synthetic price/quantity level, not a
// provider quote or evidence of queue position. Both values must be positive
// exact decimals within the laboratory's 20-integer/10-fractional-digit bounds.
type BookLevel struct {
	Price    string
	Quantity string
}

// BookSweepInput describes one fictional, immediately marketable limit-order
// sweep against one immutable visible book. It cannot fetch data or authorize
// an action. EvaluatedAt is the explicit synthetic replay clock, not a claim
// of real wall-clock freshness. MaxAge is bounded to (0, 24h]; observations
// after EvaluatedAt are rejected. Both sides need 1..100 strictly sorted levels.
type BookSweepInput struct {
	Scope             Scope
	Source            string
	Symbol            string
	Side              string
	LimitPrice        string
	RemainingQuantity string
	Bids              []BookLevel
	Asks              []BookLevel
	ObservedAt        time.Time
	EvaluatedAt       time.Time
	MaxAge            time.Duration
	FeeBasisPoints    uint32
}

// BookSweepFill is a derived synthetic incremental fill. Price, quantity,
// gross, and fee are exact; no rounding or missing liquidity is inferred.
// It is not a broker fill, order identity, or a lifecycle event on its own.
type BookSweepFill struct {
	Price         string
	Quantity      string
	GrossNotional string
	Fee           string
}

type BookSweepResult struct {
	Scope              Scope
	Source             string
	Symbol             string
	Side               string
	ObservedAt         time.Time
	EvaluatedAt        time.Time
	Fills              []BookSweepFill
	FilledQuantity     string
	RemainingQuantity  string
	TotalGrossNotional string
	TotalFees          string
	StopReason         string
}

// SweepBook consumes asks from lowest price for a BUY or bids from highest
// price for a SELL, stopping at the limit, requested quantity, or visible
// depth. It validates both sides before computing any result. It never changes
// its inputs, a journal, a reservation, or a portfolio. The existing lifecycle
// engine must independently validate any fixture events derived by a caller.
//
// This is a single-book hypothetical calculation, not passive matching,
// historical replay, replenishment, latency, market impact, or execution
// certainty. Reusing a book in another call does not track consumed liquidity:
// callers must not reuse the snapshot as fresh depth for independent orders.
// Invalid data, arithmetic outside the decimal bounds, or any amount requiring
// rounding returns ErrInvalid and an empty result, never partial evidence.
func SweepBook(input BookSweepInput) (BookSweepResult, error) {
	scope := input.Scope
	if !scope.SimulationOnly || input.Source != BookSweepSyntheticSource || !identifier.MatchString(scope.Owner) || !identifier.MatchString(scope.Account) || !identifier.MatchString(scope.Run) || (scope.Provider != "coinbase" && scope.Provider != "schwab") || !symbolPattern.MatchString(input.Symbol) || (input.Side != "BUY" && input.Side != "SELL") || !positive(input.LimitPrice) || !positive(input.RemainingQuantity) || input.FeeBasisPoints > 10_000 {
		return BookSweepResult{}, ErrInvalid
	}
	if input.ObservedAt.IsZero() || input.EvaluatedAt.IsZero() || input.MaxAge <= 0 || input.MaxAge > bookSweepMaximumAge || input.ObservedAt.After(input.EvaluatedAt) || input.ObservedAt.Before(input.EvaluatedAt.Add(-input.MaxAge)) {
		return BookSweepResult{}, ErrInvalid
	}
	if !validBookSweepLevels(input.Bids, false) || !validBookSweepLevels(input.Asks, true) || number(input.Bids[0].Price).Cmp(number(input.Asks[0].Price)) >= 0 {
		return BookSweepResult{}, ErrInvalid
	}

	result := BookSweepResult{
		Scope: scope, Source: input.Source, Symbol: input.Symbol, Side: input.Side,
		ObservedAt: input.ObservedAt, EvaluatedAt: input.EvaluatedAt,
		Fills: []BookSweepFill{}, StopReason: BookSweepDepthExhausted,
	}
	levels := input.Asks
	if input.Side == "SELL" {
		levels = input.Bids
	}
	requested := number(input.RemainingQuantity)
	remaining := new(big.Rat).Set(requested)
	limit := number(input.LimitPrice)
	feeRate := new(big.Rat).SetFrac64(int64(input.FeeBasisPoints), 10_000)
	totalGross, totalFees := new(big.Rat), new(big.Rat)
	for _, level := range levels {
		price := number(level.Price)
		comparison := price.Cmp(limit)
		if (input.Side == "BUY" && comparison > 0) || (input.Side == "SELL" && comparison < 0) {
			result.StopReason = BookSweepLimitReached
			break
		}
		quantity := number(level.Quantity)
		if quantity.Cmp(remaining) > 0 {
			quantity.Set(remaining)
		}
		gross := new(big.Rat).Mul(quantity, price)
		fee := new(big.Rat).Mul(gross, feeRate)
		priceText, priceOK := exactBookSweepDecimal(price)
		quantityText, quantityOK := exactBookSweepDecimal(quantity)
		grossText, grossOK := exactBookSweepDecimal(gross)
		feeText, feeOK := exactBookSweepDecimal(fee)
		if !priceOK || !quantityOK || !grossOK || !feeOK {
			return BookSweepResult{}, ErrInvalid
		}
		result.Fills = append(result.Fills, BookSweepFill{Price: priceText, Quantity: quantityText, GrossNotional: grossText, Fee: feeText})
		remaining.Sub(remaining, quantity)
		totalGross.Add(totalGross, gross)
		totalFees.Add(totalFees, fee)
		if remaining.Sign() == 0 {
			result.StopReason = BookSweepFilled
			break
		}
	}
	var filledOK, remainingOK, grossOK, feesOK bool
	result.FilledQuantity, filledOK = exactBookSweepDecimal(new(big.Rat).Sub(requested, remaining))
	result.RemainingQuantity, remainingOK = exactBookSweepDecimal(remaining)
	result.TotalGrossNotional, grossOK = exactBookSweepDecimal(totalGross)
	result.TotalFees, feesOK = exactBookSweepDecimal(totalFees)
	if !filledOK || !remainingOK || !grossOK || !feesOK {
		return BookSweepResult{}, ErrInvalid
	}
	return result, nil
}

func validBookSweepLevels(levels []BookLevel, ascending bool) bool {
	if len(levels) == 0 || len(levels) > bookSweepMaximumLevels {
		return false
	}
	var previous *big.Rat
	for _, level := range levels {
		if !positive(level.Price) || !positive(level.Quantity) {
			return false
		}
		price := number(level.Price)
		if previous != nil {
			comparison := price.Cmp(previous)
			if (ascending && comparison <= 0) || (!ascending && comparison >= 0) {
				return false
			}
		}
		previous = price
	}
	return true
}

// Reuse the laboratory's canonical representation only after proving that it
// would not round the amount or exceed the same bounds applied to event input.
func exactBookSweepDecimal(value *big.Rat) (string, bool) {
	text := fixed(value)
	return text, validDecimal(text) && value.Cmp(number(text)) == 0
}
