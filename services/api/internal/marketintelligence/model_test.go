package marketintelligence

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func decimal(value string) *Decimal {
	d := Decimal(value)
	return &d
}

func validQuote(now time.Time) QuoteObservation {
	return QuoteObservation{
		Symbol:     "AAPL",
		AssetClass: Equity,
		Currency:   "USD",
		Bid:        decimal("226.120000000000000001"),
		Ask:        decimal("226.13"),
		Provenance: Provenance{
			Provider:          "alpaca",
			Role:              MarketObservation,
			Feed:              "iex",
			Quality:           RealTimeSingleVenue,
			Venue:             "IEX",
			ProviderTimestamp: now.Add(-2 * time.Second),
			ReceivedAt:        now.Add(-time.Second),
		},
	}
}

func TestValidateQuotePreservesExactDecimalsAndFreshness(t *testing.T) {
	now := time.Date(2026, time.August, 20, 20, 0, 0, 0, time.UTC)
	if err := ValidateQuote(validQuote(now), now, FreshnessPolicy{MaxAge: 15 * time.Second, MaxFutureSkew: time.Second}); err != nil {
		t.Fatalf("valid quote rejected: %v", err)
	}
}

func TestValidateQuoteFailsClosedForMalformedOrMissingData(t *testing.T) {
	now := time.Date(2026, time.August, 20, 20, 0, 0, 0, time.UTC)
	policy := FreshnessPolicy{MaxAge: 15 * time.Second, MaxFutureSkew: time.Second}

	tests := []struct {
		name   string
		mutate func(*QuoteObservation)
		want   error
	}{
		{name: "missing prices", mutate: func(q *QuoteObservation) { q.Bid, q.Ask = nil, nil }, want: ErrInvalidObservation},
		{name: "exponent notation", mutate: func(q *QuoteObservation) { q.Bid = decimal("2.2612e2") }, want: ErrInvalidObservation},
		{name: "negative price", mutate: func(q *QuoteObservation) { q.Bid = decimal("-1.00") }, want: ErrInvalidObservation},
		{name: "missing feed", mutate: func(q *QuoteObservation) { q.Provenance.Feed = "" }, want: ErrMissingProvenance},
		{name: "unknown quality", mutate: func(q *QuoteObservation) { q.Provenance.Quality = "BEST" }, want: ErrMissingProvenance},
		{name: "stale", mutate: func(q *QuoteObservation) { q.Provenance.ProviderTimestamp = now.Add(-16 * time.Second) }, want: ErrStaleObservation},
		{name: "future provider time", mutate: func(q *QuoteObservation) { q.Provenance.ProviderTimestamp = now.Add(2 * time.Second) }, want: ErrFutureObservation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quote := validQuote(now)
			tt.mutate(&quote)
			if err := ValidateQuote(quote, now, policy); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestValidateCryptoCandlesPreservesExactValuesAndRejectsInventedRanges(t *testing.T) {
	now := time.Date(2026, time.August, 21, 15, 0, 0, 0, time.UTC)
	series := CryptoCandleSeries{
		Symbol: "BTC", Currency: "USD", GranularitySeconds: 900, ExpectedIntervals: 96,
		Candles: []CryptoCandle{
			{Start: now.Add(-30 * time.Minute), Low: "70000.000000000000000001", High: "70200", Open: "70025", Close: "70180", Volume: "1.234567890123456789"},
			{Start: now.Add(-15 * time.Minute), Low: "70100", High: "70300", Open: "70180", Close: "70250", Volume: "2.5"},
		},
		Provenance: Provenance{Provider: "coinbase", Role: MarketObservation, Feed: "rest_candles", Quality: RealTimeSingleVenue, Venue: "coinbase_exchange", ProviderTimestamp: now.Add(-15 * time.Minute), ReceivedAt: now},
	}
	if err := ValidateCryptoCandleSeries(series, now, FreshnessPolicy{MaxAge: 25 * time.Hour, MaxFutureSkew: time.Minute}); err != nil {
		t.Fatalf("valid candle series rejected: %v", err)
	}
	invalid := series
	invalid.Candles = append([]CryptoCandle(nil), series.Candles...)
	invalid.Candles[1].Close = "70301"
	if err := ValidateCryptoCandleSeries(invalid, now, FreshnessPolicy{MaxAge: 25 * time.Hour, MaxFutureSkew: time.Minute}); !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("out-of-range close accepted: %v", err)
	}
	invalid = series
	invalid.Candles = append([]CryptoCandle(nil), series.Candles...)
	invalid.Candles[1].Volume = "2e1"
	if err := ValidateCryptoCandleSeries(invalid, now, FreshnessPolicy{MaxAge: 25 * time.Hour, MaxFutureSkew: time.Minute}); !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("exponent volume accepted: %v", err)
	}
}

func TestSelectSourceNeverSilentlyDowngradesQuality(t *testing.T) {
	sources := []Source{
		{ID: "alpaca_iex", Label: "Alpaca IEX", Role: MarketObservation, Feed: "iex", Quality: RealTimeSingleVenue, Capabilities: []Capability{EquityQuote}, Enabled: true, Healthy: true},
		{ID: "alpaca_sip", Label: "Alpaca SIP", Role: MarketObservation, Feed: "sip", Quality: RealTimeConsolidated, Capabilities: []Capability{EquityQuote}, Enabled: false, Healthy: false},
	}

	_, err := SelectSource(SelectionPolicy{
		Capability:         EquityQuote,
		PreferredProviders: []string{"alpaca_sip", "alpaca_iex"},
		AllowedQualities:   []FeedQuality{RealTimeConsolidated},
		AllowFallback:      true,
	}, sources)
	if !errors.Is(err, ErrNoEligibleSource) {
		t.Fatalf("single-venue feed satisfied consolidated policy: %v", err)
	}
}

func TestSelectSourceRecordsExplicitFallback(t *testing.T) {
	sources := []Source{
		{ID: "primary", Label: "Primary", Role: MarketObservation, Feed: "sip", Quality: RealTimeConsolidated, Capabilities: []Capability{EquityQuote}, Enabled: true, Healthy: false},
		{ID: "secondary", Label: "Secondary", Role: MarketObservation, Feed: "sip", Quality: RealTimeConsolidated, Capabilities: []Capability{EquityQuote}, Enabled: true, Healthy: true},
	}
	policy := SelectionPolicy{
		Capability:         EquityQuote,
		PreferredProviders: []string{"primary", "secondary"},
		AllowedQualities:   []FeedQuality{RealTimeConsolidated},
		AllowFallback:      true,
	}

	selection, err := SelectSource(policy, sources)
	if err != nil {
		t.Fatalf("fallback rejected: %v", err)
	}
	if !selection.Fallback || selection.FallbackFrom != "primary" || selection.Source.ID != "secondary" {
		t.Fatalf("fallback provenance missing: %+v", selection)
	}

	policy.AllowFallback = false
	if _, err := SelectSource(policy, sources); !errors.Is(err, ErrNoEligibleSource) {
		t.Fatalf("fallback occurred without permission: %v", err)
	}
}

func TestIndicativeOptionsCannotSatisfyConsolidatedPolicy(t *testing.T) {
	source := Source{ID: "alpaca_options_basic", Label: "Alpaca Options", Role: MarketObservation, Feed: "indicative", Quality: Indicative, Capabilities: []Capability{OptionData}, Enabled: true, Healthy: true}
	_, err := SelectSource(SelectionPolicy{
		Capability:         OptionData,
		PreferredProviders: []string{source.ID},
		AllowedQualities:   []FeedQuality{RealTimeConsolidated},
	}, []Source{source})
	if !errors.Is(err, ErrNoEligibleSource) {
		t.Fatalf("indicative options satisfied consolidated policy: %v", err)
	}
}

func TestDefaultSourcesAreSafeAndDisabled(t *testing.T) {
	sources := DefaultSources()
	if len(sources) != 8 {
		t.Fatalf("unexpected source count: %d", len(sources))
	}
	for _, source := range sources {
		if source.Enabled || source.Healthy {
			t.Fatalf("source enabled before adapter wiring: %+v", source)
		}
	}
	encoded, err := json.Marshal(sources)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret", "api_key", "credential", "yfinance", "openinsider"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("unsafe catalog field or source exposed: %s", forbidden)
		}
	}
}
