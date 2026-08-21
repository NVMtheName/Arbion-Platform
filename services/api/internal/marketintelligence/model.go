// Package marketintelligence defines provider-neutral, read-only market
// observations and source-selection policy. It deliberately has no broker,
// execution, or Neural Engine dependency.
package marketintelligence

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type AssetClass string

const (
	Equity AssetClass = "EQUITY"
	Option AssetClass = "OPTION"
	Crypto AssetClass = "CRYPTO"
)

type Capability string

const (
	EquityQuote     Capability = "EQUITY_QUOTE"
	EquityBars      Capability = "EQUITY_BARS"
	OptionData      Capability = "OPTION_DATA"
	CryptoMarkets   Capability = "CRYPTO_MARKETS"
	CryptoCandles   Capability = "CRYPTO_CANDLES"
	CryptoLiquidity Capability = "CRYPTO_LIQUIDITY"
	CryptoTrades    Capability = "CRYPTO_TRADES"
	CryptoStats     Capability = "CRYPTO_VENUE_STATS"
	InsiderFiling   Capability = "INSIDER_FILING"
)

type SourceRole string

const (
	BrokerAuthority   SourceRole = "BROKER_AUTHORITY"
	MarketObservation SourceRole = "MARKET_OBSERVATION"
	ReferenceData     SourceRole = "REFERENCE_DATA"
	PrimaryFiling     SourceRole = "PRIMARY_FILING"
)

type FeedQuality string

const (
	RealTimeConsolidated FeedQuality = "REAL_TIME_CONSOLIDATED"
	RealTimeSingleVenue  FeedQuality = "REAL_TIME_SINGLE_VENUE"
	Indicative           FeedQuality = "INDICATIVE"
	Delayed              FeedQuality = "DELAYED"
	AggregatedReference  FeedQuality = "AGGREGATED_REFERENCE"
	EndOfDay             FeedQuality = "END_OF_DAY"
	Filing               FeedQuality = "FILING"
)

type Decimal string

type VerificationState string

const (
	NotConfigured       VerificationState = "NOT_CONFIGURED"
	AwaitingObservation VerificationState = "AWAITING_OBSERVATION"
	Verified            VerificationState = "VERIFIED"
	VerificationExpired VerificationState = "VERIFICATION_EXPIRED"
	Degraded            VerificationState = "DEGRADED"
)

// CapabilityStatus reports process-local adapter verification without
// exposing provider error text, credentials, or claiming that a successful
// request is a consolidated or executable market observation.
type CapabilityStatus struct {
	Capability          Capability        `json:"capability"`
	Enabled             bool              `json:"enabled"`
	State               VerificationState `json:"state"`
	LastAttemptAt       *time.Time        `json:"last_attempt_at,omitempty"`
	LastSuccessAt       *time.Time        `json:"last_success_at,omitempty"`
	ConsecutiveFailures int               `json:"consecutive_failures"`
	FailureCategory     string            `json:"failure_category,omitempty"`
	RequestPolicy       *RequestPolicy    `json:"request_policy,omitempty"`
	RequestUsage        *RequestUsage     `json:"request_usage,omitempty"`
}

// RequestPolicy is Arbion's process-local protection around a capability. It
// is not a provider-published quota, allowance, or remaining-credit balance.
type RequestPolicy struct {
	CacheTTLMilliseconds           int64 `json:"cache_ttl_ms"`
	MinimumIntervalMilliseconds    int64 `json:"minimum_request_interval_ms"`
	VerificationWindowMilliseconds int64 `json:"verification_window_ms"`
}

// RequestUsage contains bounded, process-local aggregate counters. It carries
// no user, account, instrument, request, or provider-correlation dimensions.
type RequestUsage struct {
	CacheLookups      uint64 `json:"cache_lookups"`
	CacheHits         uint64 `json:"cache_hits"`
	ProviderAttempts  uint64 `json:"provider_attempts"`
	CountersSaturated bool   `json:"counters_saturated"`
}

type Source struct {
	ID               string             `json:"id"`
	Label            string             `json:"label"`
	Role             SourceRole         `json:"role"`
	Feed             string             `json:"feed"`
	Quality          FeedQuality        `json:"quality"`
	Capabilities     []Capability       `json:"capabilities"`
	CapabilityStatus []CapabilityStatus `json:"capability_status"`
	Enabled          bool               `json:"enabled"`
	Healthy          bool               `json:"healthy"`
}

type Provenance struct {
	Provider          string      `json:"provider"`
	ProviderRequestID string      `json:"provider_request_id,omitempty"`
	Role              SourceRole  `json:"role"`
	Feed              string      `json:"feed"`
	Quality           FeedQuality `json:"quality"`
	Venue             string      `json:"venue,omitempty"`
	ProviderTimestamp time.Time   `json:"provider_timestamp"`
	ReceivedAt        time.Time   `json:"received_at"`
}

type QuoteObservation struct {
	Symbol     string     `json:"symbol"`
	AssetClass AssetClass `json:"asset_class"`
	Currency   string     `json:"currency"`
	Bid        *Decimal   `json:"bid,omitempty"`
	Ask        *Decimal   `json:"ask,omitempty"`
	Mark       *Decimal   `json:"mark,omitempty"`
	Last       *Decimal   `json:"last,omitempty"`
	Provenance Provenance `json:"provenance"`
}

type CryptoMarketObservation struct {
	ID               string     `json:"id"`
	Symbol           string     `json:"symbol"`
	Name             string     `json:"name"`
	Currency         string     `json:"currency"`
	CurrentPrice     Decimal    `json:"current_price"`
	Bid              *Decimal   `json:"bid,omitempty"`
	Ask              *Decimal   `json:"ask,omitempty"`
	MarketCap        *Decimal   `json:"market_cap,omitempty"`
	MarketCapRank    *int       `json:"market_cap_rank,omitempty"`
	Volume24H        *Decimal   `json:"volume_24h,omitempty"`
	Volume24HUnit    string     `json:"volume_24h_unit,omitempty"`
	ChangePercent24H *Decimal   `json:"change_percent_24h,omitempty"`
	Provenance       Provenance `json:"provenance"`
}

// CryptoMarketBatch preserves partial coverage without inventing prices for
// assets that do not have an approved quote-currency product.
type CryptoMarketBatch struct {
	Markets            []CryptoMarketObservation `json:"markets"`
	UnavailableSymbols []string                  `json:"unavailable_symbols"`
}

// CryptoCandle is one provider-reported interval. Missing intervals are not
// synthesized, because an absent Coinbase bucket means no ticks were recorded.
type CryptoCandle struct {
	Start  time.Time `json:"start"`
	Low    Decimal   `json:"low"`
	High   Decimal   `json:"high"`
	Open   Decimal   `json:"open"`
	Close  Decimal   `json:"close"`
	Volume Decimal   `json:"volume"`
}

// CryptoCandleSeries is bounded historical venue evidence. It is not a
// portfolio return, cost-basis, or executable-price record.
type CryptoCandleSeries struct {
	Symbol             string         `json:"symbol"`
	Currency           string         `json:"currency"`
	GranularitySeconds int            `json:"granularity_seconds"`
	ExpectedIntervals  int            `json:"expected_intervals"`
	Candles            []CryptoCandle `json:"candles"`
	Provenance         Provenance     `json:"provenance"`
}

// CryptoBookLevel is one exact provider-reported price level. Size is the
// available base-asset quantity at that venue level, not an executable quote.
type CryptoBookLevel struct {
	Price Decimal `json:"price"`
	Size  Decimal `json:"size"`
}

// CryptoLiquiditySnapshot is a bounded, point-in-time view of one venue book.
// It is deliberately distinct from account balances and order capabilities.
type CryptoLiquiditySnapshot struct {
	Symbol         string            `json:"symbol"`
	Currency       string            `json:"currency"`
	ProductID      string            `json:"product_id"`
	Depth          int               `json:"depth"`
	Bids           []CryptoBookLevel `json:"bids"`
	Asks           []CryptoBookLevel `json:"asks"`
	Last           Decimal           `json:"last"`
	MidMarket      Decimal           `json:"mid_market"`
	SpreadBPS      Decimal           `json:"spread_bps"`
	SpreadAbsolute Decimal           `json:"spread_absolute"`
	Provenance     Provenance        `json:"provenance"`
}

// CryptoTradeObservation is one public venue tick with provider-reported side.
// Public trade identity is intentionally omitted from the normalized model.
type CryptoTradeObservation struct {
	Price Decimal   `json:"price"`
	Size  Decimal   `json:"size"`
	Time  time.Time `json:"time"`
	Side  string    `json:"side"`
}

// CryptoTradeTape is a bounded point-in-time public market observation. It is
// not account execution history, order-flow analysis, or an execution feed.
type CryptoTradeTape struct {
	Symbol     string                   `json:"symbol"`
	Currency   string                   `json:"currency"`
	ProductID  string                   `json:"product_id"`
	Limit      int                      `json:"limit"`
	Trades     []CryptoTradeObservation `json:"trades"`
	BestBid    Decimal                  `json:"best_bid"`
	BestAsk    Decimal                  `json:"best_ask"`
	Provenance Provenance               `json:"provenance"`
}

// SourceReceipt records when Arbion received a provider response whose
// contract does not include an event timestamp. It must not be presented as a
// provider observation time.
type SourceReceipt struct {
	Provider          string      `json:"provider"`
	ProviderRequestID string      `json:"provider_request_id,omitempty"`
	Role              SourceRole  `json:"role"`
	Feed              string      `json:"feed"`
	Quality           FeedQuality `json:"quality"`
	Venue             string      `json:"venue,omitempty"`
	ReceivedAt        time.Time   `json:"received_at"`
}

// CryptoVenueStats is one exact rolling-window response from a single venue.
// Coinbase does not timestamp this response, so Receipt is intentionally
// separate from event-time Provenance.
type CryptoVenueStats struct {
	Symbol      string        `json:"symbol"`
	Currency    string        `json:"currency"`
	ProductID   string        `json:"product_id"`
	Open        Decimal       `json:"open"`
	High        Decimal       `json:"high"`
	Low         Decimal       `json:"low"`
	Last        Decimal       `json:"last"`
	Volume24H   Decimal       `json:"volume_24h"`
	Volume30Day Decimal       `json:"volume_30day"`
	VolumeUnit  string        `json:"volume_unit"`
	Receipt     SourceReceipt `json:"receipt"`
}

type OptionContractObservation struct {
	Symbol            string    `json:"symbol"`
	Underlying        string    `json:"underlying"`
	PutCall           string    `json:"put_call"`
	Expiration        string    `json:"expiration"`
	Strike            Decimal   `json:"strike"`
	Bid               *Decimal  `json:"bid,omitempty"`
	Ask               *Decimal  `json:"ask,omitempty"`
	Mark              *Decimal  `json:"mark,omitempty"`
	Delta             *Decimal  `json:"delta,omitempty"`
	ImpliedVolatility *Decimal  `json:"implied_volatility,omitempty"`
	OpenInterest      *int      `json:"open_interest,omitempty"`
	Volume            *int      `json:"volume,omitempty"`
	ProviderTimestamp time.Time `json:"provider_timestamp"`
}

type OptionChainObservation struct {
	Symbol          string                      `json:"symbol"`
	UnderlyingPrice *Decimal                    `json:"underlying_price,omitempty"`
	Contracts       []OptionContractObservation `json:"contracts"`
	Provenance      Provenance                  `json:"provenance"`
}

type InsiderFilingObservation struct {
	IssuerCIK       string     `json:"issuer_cik"`
	AccessionNumber string     `json:"accession_number"`
	Form            string     `json:"form"`
	IsAmendment     bool       `json:"is_amendment"`
	FiledAt         time.Time  `json:"filed_at"`
	ReportDate      string     `json:"report_date,omitempty"`
	PrimaryDocument string     `json:"primary_document"`
	SourceURL       string     `json:"source_url"`
	Provenance      Provenance `json:"provenance"`
}

type FreshnessPolicy struct {
	MaxAge        time.Duration
	MaxFutureSkew time.Duration
}

type SelectionPolicy struct {
	Capability         Capability
	PreferredProviders []string
	AllowedQualities   []FeedQuality
	AllowFallback      bool
}

type Selection struct {
	Source       Source `json:"source"`
	Fallback     bool   `json:"fallback"`
	FallbackFrom string `json:"fallback_from,omitempty"`
}

// EquityQuoteProvider is read-only by construction. Implementations return a
// normalized observation and cannot preview, place, replace, or cancel orders.
type EquityQuoteProvider interface {
	LatestEquityQuote(context.Context, string) (QuoteObservation, error)
}

// CryptoMarketProvider returns aggregate reference observations. It does not
// imply that any returned value is executable on a particular venue.
type CryptoMarketProvider interface {
	TopCryptoMarkets(context.Context, string, int) ([]CryptoMarketObservation, error)
}

// CryptoAssetMarketProvider is an optional, read-only extension for valuing a
// bounded set of connected portfolio assets. The returned observations remain
// market evidence, not executable prices.
type CryptoAssetMarketProvider interface {
	CryptoMarkets(context.Context, string, []string) (CryptoMarketBatch, error)
}

// CryptoCandleProvider returns bounded, read-only historical observations for
// one venue product. Implementations must preserve provider-reported gaps.
type CryptoCandleProvider interface {
	RecentCryptoCandles(context.Context, string, string, int, int) (CryptoCandleSeries, error)
}

// CryptoLiquidityProvider returns a bounded, keyless venue snapshot. It has no
// preview, placement, replacement, or cancellation method by construction.
type CryptoLiquidityProvider interface {
	CryptoLiquidity(context.Context, string, string, int) (CryptoLiquiditySnapshot, error)
}

// CryptoTradeProvider returns recent public ticks only. It deliberately has no
// account-order or provider-write method.
type CryptoTradeProvider interface {
	RecentCryptoTrades(context.Context, string, string, int) (CryptoTradeTape, error)
}

// CryptoVenueStatsProvider returns rolling public venue statistics. Its
// receipt time is not interchangeable with a provider event timestamp.
type CryptoVenueStatsProvider interface {
	CryptoVenueStats(context.Context, string, string) (CryptoVenueStats, error)
}

type InsiderFilingProvider interface {
	RecentInsiderFilings(context.Context, string, int) ([]InsiderFilingObservation, error)
}

var (
	ErrInvalidObservation    = errors.New("invalid market observation")
	ErrMissingProvenance     = errors.New("market observation provenance is incomplete")
	ErrFutureObservation     = errors.New("market observation is future-dated")
	ErrStaleObservation      = errors.New("market observation is stale")
	ErrNoEligibleSource      = errors.New("no eligible market data source")
	ErrInstrumentUnavailable = errors.New("market instrument unavailable")
)

var (
	decimalPattern       = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)
	signedDecimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)
	cikPattern           = regexp.MustCompile(`^[0-9]{10}$`)
	accessionPattern     = regexp.MustCompile(`^[0-9]{10}-[0-9]{2}-[0-9]{6}$`)
)

func validAssetClass(value AssetClass) bool {
	switch value {
	case Equity, Option, Crypto:
		return true
	default:
		return false
	}
}

func validRole(value SourceRole) bool {
	switch value {
	case BrokerAuthority, MarketObservation, ReferenceData, PrimaryFiling:
		return true
	default:
		return false
	}
}

func validQuality(value FeedQuality) bool {
	switch value {
	case RealTimeConsolidated, RealTimeSingleVenue, Indicative, Delayed, AggregatedReference, EndOfDay, Filing:
		return true
	default:
		return false
	}
}

func validDecimal(value *Decimal) bool {
	if value == nil {
		return true
	}
	s := string(*value)
	if len(s) == 0 || len(s) > 128 || !decimalPattern.MatchString(s) {
		return false
	}
	r, ok := new(big.Rat).SetString(s)
	return ok && r.Sign() >= 0
}

func validSignedDecimal(value *Decimal) bool {
	if value == nil {
		return true
	}
	s := string(*value)
	if len(s) == 0 || len(s) > 128 || !signedDecimalPattern.MatchString(s) {
		return false
	}
	_, ok := new(big.Rat).SetString(s)
	return ok
}

func ValidateQuote(observation QuoteObservation, now time.Time, policy FreshnessPolicy) error {
	if strings.TrimSpace(observation.Symbol) == "" || !validAssetClass(observation.AssetClass) || strings.TrimSpace(observation.Currency) == "" {
		return fmt.Errorf("%w: quote identity", ErrInvalidObservation)
	}
	if observation.Bid == nil && observation.Ask == nil && observation.Mark == nil && observation.Last == nil {
		return fmt.Errorf("%w: quote contains no price", ErrInvalidObservation)
	}
	if !validDecimal(observation.Bid) || !validDecimal(observation.Ask) || !validDecimal(observation.Mark) || !validDecimal(observation.Last) {
		return fmt.Errorf("%w: quote price", ErrInvalidObservation)
	}
	return ValidateProvenance(observation.Provenance, now, policy)
}

func ValidateCryptoMarket(observation CryptoMarketObservation, now time.Time, policy FreshnessPolicy) error {
	if !boundedText(observation.ID, 128) || !boundedText(observation.Symbol, 32) || !boundedText(observation.Name, 256) || !boundedText(observation.Currency, 12) {
		return fmt.Errorf("%w: crypto market identity", ErrInvalidObservation)
	}
	if !validDecimal(&observation.CurrentPrice) || !validDecimal(observation.Bid) || !validDecimal(observation.Ask) || !validDecimal(observation.MarketCap) || !validDecimal(observation.Volume24H) || !validSignedDecimal(observation.ChangePercent24H) {
		return fmt.Errorf("%w: crypto market value", ErrInvalidObservation)
	}
	if observation.Volume24H != nil && observation.Volume24HUnit != "" && !boundedText(observation.Volume24HUnit, 12) {
		return fmt.Errorf("%w: crypto volume unit", ErrInvalidObservation)
	}
	if observation.MarketCapRank != nil && *observation.MarketCapRank <= 0 {
		return fmt.Errorf("%w: crypto market rank", ErrInvalidObservation)
	}
	return ValidateProvenance(observation.Provenance, now, policy)
}

func ValidateCryptoCandleSeries(series CryptoCandleSeries, now time.Time, policy FreshnessPolicy) error {
	if !boundedText(series.Symbol, 32) || !boundedText(series.Currency, 12) || series.ExpectedIntervals < 1 || series.ExpectedIntervals > 300 || len(series.Candles) == 0 || len(series.Candles) > series.ExpectedIntervals {
		return fmt.Errorf("%w: crypto candle identity or bounds", ErrInvalidObservation)
	}
	switch series.GranularitySeconds {
	case 60, 300, 900, 3600, 21600, 86400:
	default:
		return fmt.Errorf("%w: crypto candle granularity", ErrInvalidObservation)
	}
	var previous time.Time
	for _, candle := range series.Candles {
		if candle.Start.IsZero() || candle.Start.Unix()%int64(series.GranularitySeconds) != 0 || !previous.IsZero() && !candle.Start.After(previous) || candle.Start.After(now.Add(policy.MaxFutureSkew)) ||
			!validDecimal(&candle.Low) || !validDecimal(&candle.High) || !validDecimal(&candle.Open) || !validDecimal(&candle.Close) || !validDecimal(&candle.Volume) {
			return fmt.Errorf("%w: crypto candle value", ErrInvalidObservation)
		}
		low, lowOK := new(big.Rat).SetString(string(candle.Low))
		high, highOK := new(big.Rat).SetString(string(candle.High))
		open, openOK := new(big.Rat).SetString(string(candle.Open))
		closeValue, closeOK := new(big.Rat).SetString(string(candle.Close))
		if !lowOK || !highOK || !openOK || !closeOK || low.Cmp(high) > 0 || open.Cmp(low) < 0 || open.Cmp(high) > 0 || closeValue.Cmp(low) < 0 || closeValue.Cmp(high) > 0 {
			return fmt.Errorf("%w: crypto candle OHLC range", ErrInvalidObservation)
		}
		previous = candle.Start
	}
	if !series.Provenance.ProviderTimestamp.Equal(series.Candles[len(series.Candles)-1].Start) {
		return fmt.Errorf("%w: crypto candle timestamp", ErrInvalidObservation)
	}
	return ValidateProvenance(series.Provenance, now, policy)
}

func ValidateCryptoLiquidity(snapshot CryptoLiquiditySnapshot, now time.Time, policy FreshnessPolicy) error {
	if !boundedText(snapshot.Symbol, 32) || !boundedText(snapshot.Currency, 12) || !boundedText(snapshot.ProductID, 64) ||
		snapshot.Depth < 1 || snapshot.Depth > 10 || len(snapshot.Bids) == 0 || len(snapshot.Bids) > snapshot.Depth || len(snapshot.Asks) == 0 || len(snapshot.Asks) > snapshot.Depth {
		return fmt.Errorf("%w: crypto liquidity identity or bounds", ErrInvalidObservation)
	}
	if snapshot.ProductID != strings.ToUpper(snapshot.Symbol)+"-"+strings.ToUpper(snapshot.Currency) ||
		!positiveDecimal(snapshot.Last) || !positiveDecimal(snapshot.MidMarket) || !positiveDecimal(snapshot.SpreadBPS) || !positiveDecimal(snapshot.SpreadAbsolute) {
		return fmt.Errorf("%w: crypto liquidity summary", ErrInvalidObservation)
	}
	if err := validateBookSide(snapshot.Bids, true); err != nil {
		return err
	}
	if err := validateBookSide(snapshot.Asks, false); err != nil {
		return err
	}
	bestBid, bidOK := new(big.Rat).SetString(string(snapshot.Bids[0].Price))
	bestAsk, askOK := new(big.Rat).SetString(string(snapshot.Asks[0].Price))
	mid, midOK := new(big.Rat).SetString(string(snapshot.MidMarket))
	spread, spreadOK := new(big.Rat).SetString(string(snapshot.SpreadAbsolute))
	if !bidOK || !askOK || !midOK || !spreadOK || bestBid.Cmp(bestAsk) >= 0 || mid.Cmp(bestBid) < 0 || mid.Cmp(bestAsk) > 0 {
		return fmt.Errorf("%w: crypto liquidity book", ErrInvalidObservation)
	}
	expectedSpread := new(big.Rat).Sub(new(big.Rat).Set(bestAsk), bestBid)
	expectedMid := new(big.Rat).Quo(new(big.Rat).Add(new(big.Rat).Set(bestAsk), bestBid), big.NewRat(2, 1))
	if spread.Cmp(expectedSpread) != 0 || mid.Cmp(expectedMid) != 0 {
		return fmt.Errorf("%w: crypto liquidity summary mismatch", ErrInvalidObservation)
	}
	return ValidateProvenance(snapshot.Provenance, now, policy)
}

func ValidateCryptoTradeTape(tape CryptoTradeTape, now time.Time, policy FreshnessPolicy) error {
	if !boundedText(tape.Symbol, 32) || !boundedText(tape.Currency, 12) || !boundedText(tape.ProductID, 64) ||
		tape.ProductID != strings.ToUpper(tape.Symbol)+"-"+strings.ToUpper(tape.Currency) || tape.Limit != 25 || len(tape.Trades) == 0 || len(tape.Trades) > tape.Limit ||
		!positiveDecimal(tape.BestBid) || !positiveDecimal(tape.BestAsk) {
		return fmt.Errorf("%w: crypto trade tape identity or bounds", ErrInvalidObservation)
	}
	bestBid, bidOK := new(big.Rat).SetString(string(tape.BestBid))
	bestAsk, askOK := new(big.Rat).SetString(string(tape.BestAsk))
	if !bidOK || !askOK || bestBid.Cmp(bestAsk) >= 0 {
		return fmt.Errorf("%w: crypto trade tape market", ErrInvalidObservation)
	}
	var previous time.Time
	for _, trade := range tape.Trades {
		if !positiveDecimal(trade.Price) || !positiveDecimal(trade.Size) || trade.Time.IsZero() || trade.Time.After(now.Add(policy.MaxFutureSkew)) ||
			(!previous.IsZero() && trade.Time.After(previous)) || (trade.Side != "BUY" && trade.Side != "SELL") {
			return fmt.Errorf("%w: crypto trade tape tick", ErrInvalidObservation)
		}
		previous = trade.Time
	}
	if !tape.Provenance.ProviderTimestamp.Equal(tape.Trades[0].Time) {
		return fmt.Errorf("%w: crypto trade tape timestamp", ErrInvalidObservation)
	}
	return ValidateProvenance(tape.Provenance, now, policy)
}

func ValidateCryptoVenueStats(stats CryptoVenueStats, now time.Time, maxReceiptAge, maxFutureSkew time.Duration) error {
	if !boundedText(stats.Symbol, 32) || !boundedText(stats.Currency, 12) || stats.ProductID != stats.Symbol+"-"+stats.Currency || stats.VolumeUnit != stats.Symbol {
		return fmt.Errorf("%w: venue stats identity", ErrInvalidObservation)
	}
	for _, value := range []*Decimal{&stats.Volume24H, &stats.Volume30Day} {
		if !validDecimal(value) {
			return fmt.Errorf("%w: venue stats decimal", ErrInvalidObservation)
		}
	}
	if !positiveDecimal(stats.Open) || !positiveDecimal(stats.High) || !positiveDecimal(stats.Low) || !positiveDecimal(stats.Last) {
		return fmt.Errorf("%w: venue stats price", ErrInvalidObservation)
	}
	openValue, _ := new(big.Rat).SetString(string(stats.Open))
	highValue, _ := new(big.Rat).SetString(string(stats.High))
	lowValue, _ := new(big.Rat).SetString(string(stats.Low))
	lastValue, _ := new(big.Rat).SetString(string(stats.Last))
	volume24H, _ := new(big.Rat).SetString(string(stats.Volume24H))
	volume30Day, _ := new(big.Rat).SetString(string(stats.Volume30Day))
	if highValue.Cmp(lowValue) < 0 || openValue.Cmp(lowValue) < 0 || openValue.Cmp(highValue) > 0 || lastValue.Cmp(lowValue) < 0 || lastValue.Cmp(highValue) > 0 || volume30Day.Cmp(volume24H) < 0 {
		return fmt.Errorf("%w: venue stats relationship", ErrInvalidObservation)
	}
	return ValidateSourceReceipt(stats.Receipt, now, maxReceiptAge, maxFutureSkew)
}

func ValidateSourceReceipt(receipt SourceReceipt, now time.Time, maxAge, maxFutureSkew time.Duration) error {
	if !boundedText(receipt.Provider, 128) || !boundedText(receipt.Feed, 128) || !validRole(receipt.Role) || !validQuality(receipt.Quality) || receipt.ReceivedAt.IsZero() || maxAge <= 0 || maxFutureSkew < 0 {
		return fmt.Errorf("%w: source receipt", ErrMissingProvenance)
	}
	if receipt.ReceivedAt.After(now.Add(maxFutureSkew)) {
		return ErrFutureObservation
	}
	if now.Sub(receipt.ReceivedAt) > maxAge {
		return ErrStaleObservation
	}
	return nil
}

func positiveDecimal(value Decimal) bool {
	if !validDecimal(&value) {
		return false
	}
	parsed, ok := new(big.Rat).SetString(string(value))
	return ok && parsed.Sign() > 0
}

func validateBookSide(levels []CryptoBookLevel, descending bool) error {
	var prior *big.Rat
	for _, level := range levels {
		if !positiveDecimal(level.Price) || !positiveDecimal(level.Size) {
			return fmt.Errorf("%w: crypto liquidity level", ErrInvalidObservation)
		}
		price, ok := new(big.Rat).SetString(string(level.Price))
		if !ok || prior != nil && (descending && prior.Cmp(price) <= 0 || !descending && prior.Cmp(price) >= 0) {
			return fmt.Errorf("%w: crypto liquidity ordering", ErrInvalidObservation)
		}
		prior = price
	}
	return nil
}

func ValidateInsiderFiling(observation InsiderFilingObservation, now time.Time, maxFutureSkew time.Duration) error {
	if maxFutureSkew < 0 || !cikPattern.MatchString(observation.IssuerCIK) || !accessionPattern.MatchString(observation.AccessionNumber) || !validInsiderForm(observation.Form) || !boundedText(observation.PrimaryDocument, 512) {
		return fmt.Errorf("%w: insider filing identity", ErrInvalidObservation)
	}
	if observation.IsAmendment != strings.HasSuffix(observation.Form, "/A") || observation.FiledAt.IsZero() || observation.FiledAt.After(now.Add(maxFutureSkew)) {
		return fmt.Errorf("%w: insider filing time or amendment", ErrInvalidObservation)
	}
	if observation.ReportDate != "" {
		if _, err := time.Parse("2006-01-02", observation.ReportDate); err != nil {
			return fmt.Errorf("%w: insider filing report date", ErrInvalidObservation)
		}
	}
	sourceURL, err := url.Parse(observation.SourceURL)
	if err != nil || sourceURL.Scheme != "https" || sourceURL.Host != "www.sec.gov" || !strings.HasPrefix(sourceURL.Path, "/Archives/edgar/data/") || sourceURL.RawQuery != "" || sourceURL.Fragment != "" {
		return fmt.Errorf("%w: insider filing source", ErrInvalidObservation)
	}
	return ValidateProvenance(observation.Provenance, now, FreshnessPolicy{MaxAge: 200 * 365 * 24 * time.Hour, MaxFutureSkew: maxFutureSkew})
}

func validInsiderForm(form string) bool {
	switch form {
	case "3", "3/A", "4", "4/A", "5", "5/A":
		return true
	default:
		return false
	}
}

func boundedText(value string, maximum int) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maximum {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func ValidateProvenance(provenance Provenance, now time.Time, policy FreshnessPolicy) error {
	if strings.TrimSpace(provenance.Provider) == "" || strings.TrimSpace(provenance.Feed) == "" || !validRole(provenance.Role) || !validQuality(provenance.Quality) || provenance.ProviderTimestamp.IsZero() || provenance.ReceivedAt.IsZero() {
		return ErrMissingProvenance
	}
	if policy.MaxAge <= 0 || policy.MaxFutureSkew < 0 {
		return fmt.Errorf("%w: freshness policy", ErrInvalidObservation)
	}
	if provenance.ProviderTimestamp.After(now.Add(policy.MaxFutureSkew)) || provenance.ReceivedAt.After(now.Add(policy.MaxFutureSkew)) || provenance.ProviderTimestamp.After(provenance.ReceivedAt.Add(policy.MaxFutureSkew)) {
		return ErrFutureObservation
	}
	if now.Sub(provenance.ProviderTimestamp) > policy.MaxAge {
		return ErrStaleObservation
	}
	return nil
}

func SelectSource(policy SelectionPolicy, sources []Source) (Selection, error) {
	if policy.Capability == "" || len(policy.PreferredProviders) == 0 || len(policy.AllowedQualities) == 0 {
		return Selection{}, ErrNoEligibleSource
	}

	allowedQuality := make(map[FeedQuality]struct{}, len(policy.AllowedQualities))
	for _, quality := range policy.AllowedQualities {
		if !validQuality(quality) {
			return Selection{}, ErrNoEligibleSource
		}
		allowedQuality[quality] = struct{}{}
	}

	byID := make(map[string]Source, len(sources))
	for _, source := range sources {
		byID[source.ID] = source
	}

	primary := policy.PreferredProviders[0]
	for index, provider := range policy.PreferredProviders {
		if index > 0 && !policy.AllowFallback {
			break
		}
		source, ok := byID[provider]
		if !ok || !source.Enabled || !source.Healthy || !sourceSupports(source, policy.Capability) {
			continue
		}
		if _, ok := allowedQuality[source.Quality]; !ok {
			continue
		}
		selection := Selection{Source: source}
		if index > 0 {
			selection.Fallback = true
			selection.FallbackFrom = primary
		}
		return selection, nil
	}

	return Selection{}, ErrNoEligibleSource
}

func sourceSupports(source Source, capability Capability) bool {
	if strings.TrimSpace(source.ID) == "" || strings.TrimSpace(source.Feed) == "" || !validRole(source.Role) || !validQuality(source.Quality) {
		return false
	}
	for _, candidate := range source.Capabilities {
		if candidate == capability {
			return true
		}
	}
	return false
}
