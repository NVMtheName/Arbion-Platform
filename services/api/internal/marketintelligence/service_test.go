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

func (provider *fakeCryptoProvider) RecentCryptoCandles(_ context.Context, symbol, currency string, granularity, limit int) (CryptoCandleSeries, error) {
	provider.calls++
	now := time.Now().UTC().Truncate(time.Minute)
	return CryptoCandleSeries{
		Symbol: symbol, Currency: currency, GranularitySeconds: granularity, ExpectedIntervals: limit,
		Candles:    []CryptoCandle{{Start: now.Add(-15 * time.Minute), Low: "99", High: "101", Open: "100", Close: "100.5", Volume: "2"}},
		Provenance: Provenance{Provider: "coinbase", Role: MarketObservation, Feed: "rest_candles", Quality: RealTimeSingleVenue, Venue: "coinbase_exchange", ProviderTimestamp: now.Add(-15 * time.Minute), ReceivedAt: now},
	}, nil
}

func (provider *fakeCryptoProvider) CryptoLiquidity(_ context.Context, symbol, currency string, depth int) (CryptoLiquiditySnapshot, error) {
	provider.calls++
	now := time.Now().UTC()
	return CryptoLiquiditySnapshot{
		Symbol: symbol, Currency: currency, ProductID: symbol + "-" + currency, Depth: depth,
		Bids: []CryptoBookLevel{{Price: "99.9", Size: "2"}}, Asks: []CryptoBookLevel{{Price: "100.1", Size: "3"}},
		Last: "100", MidMarket: "100", SpreadBPS: "20", SpreadAbsolute: "0.2",
		Provenance: Provenance{Provider: "coinbase", Role: MarketObservation, Feed: "advanced_trade_public_product_book", Quality: RealTimeSingleVenue, Venue: "coinbase_advanced_trade", ProviderTimestamp: now, ReceivedAt: now},
	}, nil
}

func (provider *fakeCryptoProvider) RecentCryptoTrades(_ context.Context, symbol, currency string, limit int) (CryptoTradeTape, error) {
	provider.calls++
	now := time.Now().UTC()
	return CryptoTradeTape{
		Symbol: symbol, Currency: currency, ProductID: symbol + "-" + currency, Limit: limit,
		Trades:  []CryptoTradeObservation{{Price: "100", Size: "0.5", Time: now, Side: "BUY"}},
		BestBid: "99.9", BestAsk: "100.1",
		Provenance: Provenance{Provider: "coinbase", Role: MarketObservation, Feed: "advanced_trade_public_market_trades", Quality: RealTimeSingleVenue, Venue: "coinbase_advanced_trade", ProviderTimestamp: now, ReceivedAt: now},
	}, nil
}

func (provider *fakeCryptoProvider) CryptoVenueStats(_ context.Context, symbol, currency string) (CryptoVenueStats, error) {
	provider.calls++
	now := time.Now().UTC()
	return CryptoVenueStats{
		Symbol: symbol, Currency: currency, ProductID: symbol + "-" + currency,
		Open: "99", High: "110", Low: "90", Last: "100", Volume24H: "250", Volume30Day: "7500", VolumeUnit: symbol,
		Receipt: SourceReceipt{Provider: "coinbase", Role: MarketObservation, Feed: "exchange_public_product_stats", Quality: RealTimeSingleVenue, Venue: "coinbase_exchange", ReceivedAt: now},
	}, nil
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

func TestServiceKeepsCandleCacheAndSourceSeparate(t *testing.T) {
	global := &fakeTopOnlyCryptoProvider{}
	candles := &fakeCryptoProvider{}
	service, err := NewService(ServiceConfig{
		CryptoProvider: global, CryptoSourceID: "coingecko_rest", CryptoCacheTTL: time.Minute, CryptoInterval: time.Millisecond,
		CryptoCandleProvider: candles, CryptoCandleSourceID: "coinbase_exchange", CryptoCandleCacheTTL: time.Minute, CryptoCandleInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, cached, err := service.RecentCryptoCandles(context.Background(), " btc ", "usd", 900, 96)
	if err != nil || cached || first.Symbol != "BTC" || len(first.Candles) != 1 {
		t.Fatalf("unexpected first candle series: cached=%v series=%+v err=%v", cached, first, err)
	}
	first.Candles[0].Close = "0"
	second, cached, err := service.RecentCryptoCandles(context.Background(), "BTC", "USD", 900, 96)
	if err != nil || !cached || candles.calls != 1 || second.Candles[0].Close != "100.5" {
		t.Fatalf("candle cache was not isolated or cloned: cached=%v calls=%d series=%+v err=%v", cached, candles.calls, second, err)
	}
	if _, _, err = service.RecentCryptoCandles(context.Background(), "BTC", "USD", 3600, 24); !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("unbounded candle contract accepted: %v", err)
	}
	if global.calls != 0 {
		t.Fatalf("global crypto source was used for connected history: calls=%d", global.calls)
	}
}

func TestServiceKeepsLiquidityCacheBoundedAndCloned(t *testing.T) {
	provider := &fakeCryptoProvider{}
	service, err := NewService(ServiceConfig{
		CryptoLiquidityProvider: provider, CryptoLiquiditySourceID: "coinbase_exchange",
		CryptoLiquidityCacheTTL: time.Minute, CryptoLiquidityInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, cached, err := service.CryptoLiquidity(context.Background(), " btc ", "usd", 10)
	if err != nil || cached || first.ProductID != "BTC-USD" || len(first.Bids) != 1 {
		t.Fatalf("unexpected first liquidity result: cached=%v snapshot=%+v err=%v", cached, first, err)
	}
	first.Bids[0].Size = "0"
	second, cached, err := service.CryptoLiquidity(context.Background(), "BTC", "USD", 10)
	if err != nil || !cached || provider.calls != 1 || second.Bids[0].Size != "2" {
		t.Fatalf("liquidity cache was not isolated or cloned: cached=%v calls=%d snapshot=%+v err=%v", cached, provider.calls, second, err)
	}
	if _, _, err = service.CryptoLiquidity(context.Background(), "BTC", "USD", 25); !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("unbounded liquidity contract accepted: %v", err)
	}
}

func TestServiceKeepsPublicTradeTapeBoundedAndCloned(t *testing.T) {
	provider := &fakeCryptoProvider{}
	service, err := NewService(ServiceConfig{
		CryptoTradeProvider: provider, CryptoTradeSourceID: "coinbase_exchange",
		CryptoTradeCacheTTL: time.Minute, CryptoTradeInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, cached, err := service.RecentCryptoTrades(context.Background(), " btc ", "usd", 25)
	if err != nil || cached || first.ProductID != "BTC-USD" || len(first.Trades) != 1 {
		t.Fatalf("unexpected first trade tape: cached=%v tape=%+v err=%v", cached, first, err)
	}
	first.Trades[0].Size = "0"
	second, cached, err := service.RecentCryptoTrades(context.Background(), "BTC", "USD", 25)
	if err != nil || !cached || provider.calls != 1 || second.Trades[0].Size != "0.5" {
		t.Fatalf("trade cache was not isolated or cloned: cached=%v calls=%d tape=%+v err=%v", cached, provider.calls, second, err)
	}
}

func TestServiceCachesCanonicalVenueStats(t *testing.T) {
	provider := &fakeCryptoProvider{}
	service, err := NewService(ServiceConfig{
		CryptoStatsProvider: provider, CryptoStatsSourceID: "coinbase_exchange",
		CryptoStatsCacheTTL: time.Minute, CryptoStatsInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, cached, err := service.CryptoVenueStats(context.Background(), " btc ", "usd")
	if err != nil || cached || first.ProductID != "BTC-USD" || first.Volume30Day != "7500" {
		t.Fatalf("unexpected first venue stats: cached=%v stats=%+v err=%v", cached, first, err)
	}
	second, cached, err := service.CryptoVenueStats(context.Background(), "BTC", "USD")
	if err != nil || !cached || provider.calls != 1 || second.Receipt.Feed != "exchange_public_product_stats" {
		t.Fatalf("venue stats cache failed: cached=%v calls=%d stats=%+v err=%v", cached, provider.calls, second, err)
	}
	if _, _, err = service.CryptoVenueStats(context.Background(), "BTC", "EUR"); !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("unsupported quote currency accepted: %v", err)
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
