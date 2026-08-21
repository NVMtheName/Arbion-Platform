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
	EquityQuote   Capability = "EQUITY_QUOTE"
	EquityBars    Capability = "EQUITY_BARS"
	OptionData    Capability = "OPTION_DATA"
	CryptoMarkets Capability = "CRYPTO_MARKETS"
	InsiderFiling Capability = "INSIDER_FILING"
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

type Source struct {
	ID           string       `json:"id"`
	Label        string       `json:"label"`
	Role         SourceRole   `json:"role"`
	Feed         string       `json:"feed"`
	Quality      FeedQuality  `json:"quality"`
	Capabilities []Capability `json:"capabilities"`
	Enabled      bool         `json:"enabled"`
	Healthy      bool         `json:"healthy"`
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

type InsiderFilingProvider interface {
	RecentInsiderFilings(context.Context, string, int) ([]InsiderFilingObservation, error)
}

var (
	ErrInvalidObservation = errors.New("invalid market observation")
	ErrMissingProvenance  = errors.New("market observation provenance is incomplete")
	ErrFutureObservation  = errors.New("market observation is future-dated")
	ErrStaleObservation   = errors.New("market observation is stale")
	ErrNoEligibleSource   = errors.New("no eligible market data source")
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
