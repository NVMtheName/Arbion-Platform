package marketintelligence

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeEquityProvider struct {
	calls int
	err   error
}

func (provider *fakeEquityProvider) LatestEquityQuote(_ context.Context, symbol string) (QuoteObservation, error) {
	provider.calls++
	if provider.err != nil {
		return QuoteObservation{}, provider.err
	}
	bid, ask := Decimal("100.10"), Decimal("100.20")
	return QuoteObservation{Symbol: symbol, AssetClass: Equity, Currency: "USD", Bid: &bid, Ask: &ask}, nil
}

type fakeCryptoProvider struct{ calls int }

type fakeTopOnlyCryptoProvider struct{ calls int }

func (provider *fakeTopOnlyCryptoProvider) TopCryptoMarkets(_ context.Context, currency string, _ int) ([]CryptoMarketObservation, error) {
	provider.calls++
	return []CryptoMarketObservation{{ID: "bitcoin", Symbol: "BTC", Name: "Bitcoin", Currency: currency, CurrentPrice: Decimal("60000")}}, nil
}

func (provider *fakeCryptoProvider) TopCryptoMarkets(_ context.Context, currency string, _ int) ([]CryptoMarketObservation, error) {
	provider.calls++
	return []CryptoMarketObservation{{ID: "bitcoin", Symbol: "BTC", Name: "Bitcoin", Currency: currency, CurrentPrice: Decimal("60000")}}, nil
}

func (provider *fakeCryptoProvider) CryptoMarkets(_ context.Context, currency string, symbols []string) (CryptoMarketBatch, error) {
	provider.calls++
	markets := make([]CryptoMarketObservation, 0, len(symbols))
	for _, symbol := range symbols {
		markets = append(markets, CryptoMarketObservation{ID: symbol + "-USD", Symbol: symbol, Name: symbol, Currency: currency, CurrentPrice: Decimal("100")})
	}
	return CryptoMarketBatch{Markets: markets}, nil
}

type fakeFilingProvider struct{ calls int }

func (provider *fakeFilingProvider) RecentInsiderFilings(_ context.Context, cik string, _ int) ([]InsiderFilingObservation, error) {
	provider.calls++
	return []InsiderFilingObservation{{IssuerCIK: cik, AccessionNumber: "0000000000-26-000001", Form: "4"}}, nil
}

func TestServiceCachesObservationsAndTracksSourceHealth(t *testing.T) {
	equity := &fakeEquityProvider{}
	crypto := &fakeCryptoProvider{}
	filings := &fakeFilingProvider{}
	service, err := NewService(ServiceConfig{
		EquityProvider: equity, EquitySourceID: "alpaca_iex", EquityCacheTTL: time.Minute, EquityInterval: time.Millisecond,
		CryptoProvider: crypto, CryptoSourceID: "coingecko_rest", CryptoCacheTTL: time.Minute, CryptoInterval: time.Millisecond,
		FilingProvider: filings, FilingCacheTTL: time.Minute, FilingInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, cached, err := service.LatestEquityQuote(context.Background(), " aapl "); err != nil || cached {
		t.Fatalf("unexpected first equity result: cached=%v err=%v", cached, err)
	}
	if _, cached, err := service.LatestEquityQuote(context.Background(), "AAPL"); err != nil || !cached || equity.calls != 1 {
		t.Fatalf("equity cache was not used: cached=%v calls=%d err=%v", cached, equity.calls, err)
	}
	if _, _, err := service.TopCryptoMarkets(context.Background(), "usd", 2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.RecentInsiderFilings(context.Background(), "0000320193", 2); err != nil {
		t.Fatal(err)
	}

	sources := service.Sources()
	for _, source := range sources {
		if source.ID == "alpaca_iex" || source.ID == "coingecko_rest" || source.ID == "sec_edgar" {
			if !source.Enabled || !source.Healthy {
				t.Fatalf("configured source did not become healthy: %+v", source)
			}
		}
	}
}

func TestServiceCanonicalizesAndCachesPortfolioCryptoMarkets(t *testing.T) {
	crypto := &fakeCryptoProvider{}
	service, err := NewService(ServiceConfig{
		CryptoProvider: crypto, CryptoSourceID: "coinbase_exchange",
		CryptoCacheTTL: time.Minute, CryptoInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, cached, err := service.CryptoMarkets(context.Background(), " usd ", []string{"eth", "BTC", "ETH"})
	if err != nil || cached || len(batch.Markets) != 2 || batch.Markets[0].Symbol != "BTC" || batch.Markets[1].Symbol != "ETH" {
		t.Fatalf("unexpected first portfolio batch: cached=%v batch=%+v err=%v", cached, batch, err)
	}
	if _, cached, err = service.CryptoMarkets(context.Background(), "USD", []string{"ETH", "BTC"}); err != nil || !cached || crypto.calls != 1 {
		t.Fatalf("portfolio crypto cache was not canonical: cached=%v calls=%d err=%v", cached, crypto.calls, err)
	}
	if _, _, err = service.CryptoMarkets(context.Background(), "USD", []string{"BTC-USD"}); !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("invalid portfolio symbol was accepted: %v", err)
	}
}

func TestServiceKeepsPortfolioVenueSeparateFromGlobalCryptoProvider(t *testing.T) {
	global := &fakeTopOnlyCryptoProvider{}
	portfolio := &fakeCryptoProvider{}
	service, err := NewService(ServiceConfig{
		CryptoProvider: global, CryptoSourceID: "coingecko_rest",
		CryptoAssetProvider: portfolio, CryptoAssetSourceID: "coinbase_exchange",
		CryptoCacheTTL: time.Minute, CryptoInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.TopCryptoMarkets(context.Background(), "USD", 1); err != nil {
		t.Fatal(err)
	}
	for _, source := range service.Sources() {
		if source.ID == "coingecko_rest" && (!source.Enabled || !source.Healthy) {
			t.Fatalf("global crypto source health was not isolated: %+v", source)
		}
		if source.ID == "coinbase_exchange" && (!source.Enabled || source.Healthy) {
			t.Fatalf("portfolio source became healthy before it was called: %+v", source)
		}
	}
	if _, _, err = service.CryptoMarkets(context.Background(), "USD", []string{"BTC"}); err != nil {
		t.Fatal(err)
	}
	if global.calls != 1 || portfolio.calls != 1 {
		t.Fatalf("crypto provider boundaries crossed: global=%d portfolio=%d", global.calls, portfolio.calls)
	}
	sources := service.Sources()
	for _, sourceID := range []string{"coingecko_rest", "coinbase_exchange"} {
		found := false
		for _, source := range sources {
			if source.ID == sourceID {
				found = source.Enabled && source.Healthy
			}
		}
		if !found {
			t.Fatalf("configured crypto source was not independently healthy: %s %+v", sourceID, sources)
		}
	}
}

func TestServiceFailsClosedAndMarksProviderUnhealthy(t *testing.T) {
	provider := &fakeEquityProvider{err: errors.New("provider failed")}
	service, err := NewService(ServiceConfig{EquityProvider: provider, EquitySourceID: "alpaca_iex", EquityCacheTTL: time.Minute, EquityInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.LatestEquityQuote(context.Background(), "SPY"); err == nil {
		t.Fatal("expected provider error")
	}
	for _, source := range service.Sources() {
		if source.ID == "alpaca_iex" && (!source.Enabled || source.Healthy) {
			t.Fatalf("failed provider source state is misleading: %+v", source)
		}
	}
	if _, _, err = service.TopCryptoMarkets(context.Background(), "usd", 1); !errors.Is(err, ErrNoEligibleSource) {
		t.Fatalf("unconfigured provider did not fail closed: %v", err)
	}
}

func TestServiceRejectsUnsupportedSourceOrMissingCachePolicy(t *testing.T) {
	provider := &fakeEquityProvider{}
	if _, err := NewService(ServiceConfig{EquityProvider: provider, EquitySourceID: "yfinance", EquityCacheTTL: time.Minute, EquityInterval: time.Millisecond}); err == nil {
		t.Fatal("research-only source was accepted")
	}
	if _, err := NewService(ServiceConfig{EquityProvider: provider, EquitySourceID: "alpaca_iex"}); err == nil {
		t.Fatal("missing bounded cache policy was accepted")
	}
}

func TestServicePacingHonorsRequestCancellationWithoutCallingProvider(t *testing.T) {
	provider := &fakeEquityProvider{}
	service, err := NewService(ServiceConfig{
		EquityProvider: provider, EquitySourceID: "alpaca_iex",
		EquityCacheTTL: time.Minute, EquityInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.LatestEquityQuote(context.Background(), "SPY"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if _, _, err = service.LatestEquityQuote(ctx, "AAPL"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("paced request did not honor cancellation: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider was called after paced request cancellation: %d", provider.calls)
	}
}

func TestInvalidQueryDoesNotDegradeAHealthyProvider(t *testing.T) {
	provider := &fakeEquityProvider{}
	service, err := NewService(ServiceConfig{
		EquityProvider: provider, EquitySourceID: "alpaca_iex",
		EquityCacheTTL: time.Minute, EquityInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.LatestEquityQuote(context.Background(), "SPY"); err != nil {
		t.Fatal(err)
	}
	provider.err = ErrInvalidObservation
	if _, _, err = service.LatestEquityQuote(context.Background(), "not valid"); !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("invalid query error was not preserved: %v", err)
	}
	for _, source := range service.Sources() {
		if source.ID == "alpaca_iex" && !source.Healthy {
			t.Fatalf("invalid user query degraded provider health: %+v", source)
		}
	}
}
