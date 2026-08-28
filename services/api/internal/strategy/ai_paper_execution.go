package strategy

import (
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

const (
	aiPaperDecimalPlaces         = 10
	aiPaperBasisPointDenominator = 10_000
	aiPaperMaximumBasisPoints    = 1_000
	aiPaperMaximumMarketAge      = 2 * time.Minute
	aiPaperMaximumFutureOffset   = 5 * time.Second
)

var aiPaperDecimalPattern = regexp.MustCompile(`^(0|[1-9]\d*)(\.\d{1,10})?$`)
var aiPaperSymbolPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9.-]{0,19}$`)

// AIPaperPortfolioSnapshot is an isolated simulation account. It never
// represents, mutates, or reserves assets in a connected financial account.
type AIPaperPortfolioSnapshot struct {
	Currency  string
	Cash      string
	Positions map[string]string
}

// AIPaperMarketReference is provider-derived pricing evidence supplied by the
// Go control plane. The simulator has no provider client and cannot fetch data
// or submit an order itself.
type AIPaperMarketReference struct {
	Symbol     string
	Price      string
	Basis      string
	Provider   string
	Feed       string
	Quality    string
	ObservedAt time.Time
}

type AIPaperSimulationConfig struct {
	FeeBasisPoints      uint32
	SlippageBasisPoints uint32
}

// AIPaperFill is deterministic simulation evidence, not a broker fill. The
// type deliberately contains no broker order identifier or execution route.
type AIPaperFill struct {
	Status                    ExecutionStatus `json:"status"`
	Reason                    string          `json:"reason"`
	SimulationOnly            bool            `json:"simulation_only"`
	Symbol                    string          `json:"symbol,omitempty"`
	Instrument                string          `json:"instrument,omitempty"`
	Side                      string          `json:"side,omitempty"`
	Quantity                  string          `json:"quantity,omitempty"`
	RequestedNotional         string          `json:"requested_notional,omitempty"`
	ReferencePrice            string          `json:"reference_price,omitempty"`
	FillPrice                 string          `json:"fill_price,omitempty"`
	GrossNotional             string          `json:"gross_notional,omitempty"`
	Fee                       string          `json:"fee,omitempty"`
	CashDelta                 string          `json:"cash_delta,omitempty"`
	PositionDelta             string          `json:"position_delta,omitempty"`
	PreviousCash              string          `json:"previous_cash,omitempty"`
	PreviousPositionQuantity  string          `json:"previous_position_quantity,omitempty"`
	ResultingCash             string          `json:"resulting_cash,omitempty"`
	ResultingPositionQuantity string          `json:"resulting_position_quantity,omitempty"`
	PricingBasis              string          `json:"pricing_basis,omitempty"`
	MarketProvider            string          `json:"market_provider,omitempty"`
	MarketFeed                string          `json:"market_feed,omitempty"`
	MarketQuality             string          `json:"market_quality,omitempty"`
	MarketObservedAt          time.Time       `json:"market_observed_at,omitempty"`
	SimulatedAt               time.Time       `json:"simulated_at,omitempty"`
}

// SimulateAIPaperSpotFill applies an already-authorized AI proposal to an
// isolated paper portfolio. It performs exact decimal accounting, prohibits
// margin and short sales, and has no dependency capable of broker execution.
func SimulateAIPaperSpotFill(action risk.ProposedAction, evaluation risk.RiskEvaluation, instrument string, portfolio AIPaperPortfolioSnapshot, market AIPaperMarketReference, config AIPaperSimulationConfig, simulatedAt time.Time) AIPaperFill {
	result := AIPaperFill{Status: SimulatedRejected, Reason: "invalid_simulation_input", SimulationOnly: true}
	if evaluation.Decision != risk.Allow {
		result.Status = RiskDenied
		result.Reason = "risk_not_allowed"
		return result
	}
	if action.Source != risk.SourceAI || action.Option != nil || (instrument != "EQUITY" && instrument != "CRYPTO") || action.RequiresMargin {
		return result
	}
	symbol := strings.ToUpper(strings.TrimSpace(action.Instrument))
	if !aiPaperSymbolPattern.MatchString(symbol) || symbol != action.Instrument || market.Symbol != symbol || portfolio.Currency != "USD" {
		return result
	}
	if config.FeeBasisPoints > aiPaperMaximumBasisPoints || config.SlippageBasisPoints > aiPaperMaximumBasisPoints || simulatedAt.IsZero() || market.ObservedAt.IsZero() || market.ObservedAt.Before(simulatedAt.Add(-aiPaperMaximumMarketAge)) || market.ObservedAt.After(simulatedAt.Add(aiPaperMaximumFutureOffset)) {
		return result
	}
	if strings.TrimSpace(market.Provider) == "" || strings.TrimSpace(market.Feed) == "" || strings.TrimSpace(market.Quality) == "" || strings.TrimSpace(market.Basis) == "" {
		return result
	}
	if action.EstimatedPrice == nil || !validAIPaperDecimal(action.Quantity) || !validAIPaperDecimal(action.Notional) || !validAIPaperDecimal(*action.EstimatedPrice) || !validAIPaperDecimal(market.Price) || !validAIPaperDecimal(portfolio.Cash) {
		return result
	}
	quantity, _ := new(big.Rat).SetString(action.Quantity)
	requestedNotional, _ := new(big.Rat).SetString(action.Notional)
	estimatedPrice, _ := new(big.Rat).SetString(*action.EstimatedPrice)
	referencePrice, _ := new(big.Rat).SetString(market.Price)
	cash, _ := new(big.Rat).SetString(portfolio.Cash)
	if quantity.Sign() <= 0 || requestedNotional.Sign() <= 0 || estimatedPrice.Sign() <= 0 || referencePrice.Sign() <= 0 || cash.Sign() < 0 || estimatedPrice.Cmp(referencePrice) != 0 {
		return result
	}
	// The risk engine authorized action.Notional. Never allow a quantity/price
	// pair to expand that authorization before the configured paper assumptions.
	if new(big.Rat).Mul(quantity, referencePrice).Cmp(requestedNotional) > 0 {
		result.Reason = "quantity_exceeds_authorized_notional"
		return result
	}

	side := strings.ToUpper(strings.TrimSpace(action.Side))
	if (action.ActionType == risk.ActionBuy && side != "BUY") || (action.ActionType == risk.ActionSell && side != "SELL") || (action.ActionType != risk.ActionBuy && action.ActionType != risk.ActionSell) {
		return result
	}
	positionQuantity := new(big.Rat)
	if raw, ok := portfolio.Positions[symbol]; ok {
		if !validAIPaperDecimal(raw) {
			return result
		}
		positionQuantity, _ = new(big.Rat).SetString(raw)
	}

	slippage := new(big.Rat).SetFrac64(int64(config.SlippageBasisPoints), aiPaperBasisPointDenominator)
	fillPrice := new(big.Rat).Set(referencePrice)
	if side == "BUY" {
		fillPrice.Mul(fillPrice, new(big.Rat).Add(big.NewRat(1, 1), slippage))
		fillPrice = quantizeAIPaper(fillPrice, true)
	} else {
		fillPrice.Mul(fillPrice, new(big.Rat).Sub(big.NewRat(1, 1), slippage))
		fillPrice = quantizeAIPaper(fillPrice, false)
	}
	gross := new(big.Rat).Mul(quantity, fillPrice)
	gross = quantizeAIPaper(gross, side == "BUY")
	feeRate := new(big.Rat).SetFrac64(int64(config.FeeBasisPoints), aiPaperBasisPointDenominator)
	fee := quantizeAIPaper(new(big.Rat).Mul(gross, feeRate), true)

	positionDelta := new(big.Rat).Set(quantity)
	cashDelta := new(big.Rat)
	if side == "BUY" {
		cashDelta.Neg(new(big.Rat).Add(gross, fee))
		if cash.Cmp(new(big.Rat).Neg(cashDelta)) < 0 {
			result.Reason = "insufficient_paper_cash"
			return result
		}
	} else {
		if positionQuantity.Cmp(quantity) < 0 {
			result.Reason = "insufficient_paper_position"
			return result
		}
		positionDelta.Neg(positionDelta)
		cashDelta.Sub(gross, fee)
		if cashDelta.Sign() < 0 {
			result.Reason = "fee_exceeds_simulated_proceeds"
			return result
		}
	}
	resultingCash := new(big.Rat).Add(cash, cashDelta)
	resultingPosition := new(big.Rat).Add(positionQuantity, positionDelta)
	if resultingCash.Sign() < 0 || resultingPosition.Sign() < 0 {
		return result
	}

	result.Status = SimulatedFilled
	result.Reason = "paper_simulation_only_no_broker_order"
	result.Symbol = symbol
	result.Instrument = instrument
	result.Side = side
	result.Quantity = quantity.FloatString(aiPaperDecimalPlaces)
	result.RequestedNotional = requestedNotional.FloatString(aiPaperDecimalPlaces)
	result.ReferencePrice = referencePrice.FloatString(aiPaperDecimalPlaces)
	result.FillPrice = fillPrice.FloatString(aiPaperDecimalPlaces)
	result.GrossNotional = gross.FloatString(aiPaperDecimalPlaces)
	result.Fee = fee.FloatString(aiPaperDecimalPlaces)
	result.CashDelta = cashDelta.FloatString(aiPaperDecimalPlaces)
	result.PositionDelta = positionDelta.FloatString(aiPaperDecimalPlaces)
	result.PreviousCash = cash.FloatString(aiPaperDecimalPlaces)
	result.PreviousPositionQuantity = positionQuantity.FloatString(aiPaperDecimalPlaces)
	result.ResultingCash = resultingCash.FloatString(aiPaperDecimalPlaces)
	result.ResultingPositionQuantity = resultingPosition.FloatString(aiPaperDecimalPlaces)
	result.PricingBasis = market.Basis
	result.MarketProvider = market.Provider
	result.MarketFeed = market.Feed
	result.MarketQuality = market.Quality
	result.MarketObservedAt = market.ObservedAt
	result.SimulatedAt = simulatedAt
	return result
}

func validAIPaperDecimal(value string) bool {
	return aiPaperDecimalPattern.MatchString(value)
}

func quantizeAIPaper(value *big.Rat, upward bool) *big.Rat {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(aiPaperDecimalPlaces), nil)
	scaledNumerator := new(big.Int).Mul(value.Num(), scale)
	quotient, remainder := new(big.Int).QuoRem(scaledNumerator, value.Denom(), new(big.Int))
	if upward && remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return new(big.Rat).SetFrac(quotient, scale)
}
