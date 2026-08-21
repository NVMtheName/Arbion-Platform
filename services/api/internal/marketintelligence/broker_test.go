package marketintelligence

import (
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

func TestNormalizeBrokerQuotePreservesEntitlementAndPrecision(t *testing.T) {
	now := time.Date(2026, 8, 21, 14, 0, 0, 0, time.UTC)
	realtime := true
	bid, ask := financial.Decimal("100.123456789"), financial.Decimal("100.20")
	quote, err := NormalizeBrokerQuote("schwab", "USD", financial.Quote{
		Symbol: "spy", Bid: &bid, Ask: &ask, ProviderTimestamp: now.Add(-time.Second), Realtime: &realtime,
	}, now, FreshnessPolicy{MaxAge: time.Minute, MaxFutureSkew: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if quote.Bid == nil || *quote.Bid != "100.123456789" || quote.Provenance.Quality != RealTimeConsolidated || quote.Provenance.Role != BrokerAuthority {
		t.Fatalf("broker quote provenance or precision changed: %+v", quote)
	}
}

func TestNormalizeBrokerOptionChainLabelsUnknownEntitlementIndicative(t *testing.T) {
	now := time.Date(2026, 8, 21, 14, 0, 0, 0, time.UTC)
	underlying, strike, bid := financial.Decimal("100.1"), financial.Decimal("95"), financial.Decimal("1.25")
	chain, err := NormalizeBrokerOptionChain("schwab", financial.OptionChain{
		Symbol: "SPY", UnderlyingPrice: &underlying, ProviderTimestamp: now.Add(-time.Second),
		Contracts: []financial.OptionContract{{
			Symbol: "SPY   261016P00095000", Underlying: "SPY", PutCall: "PUT", Expiration: "2026-10-16",
			Strike: strike, Bid: &bid, ProviderTimestamp: now.Add(-time.Second),
		}},
	}, now, FreshnessPolicy{MaxAge: time.Minute, MaxFutureSkew: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(chain.Contracts) != 1 || chain.Provenance.Quality != Indicative {
		t.Fatalf("option-chain provenance changed: %+v", chain)
	}
}
