package marketintelligence

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxCacheEntries = 256

type ServiceConfig struct {
	EquityProvider          EquityQuoteProvider
	EquitySourceID          string
	CryptoProvider          CryptoMarketProvider
	CryptoSourceID          string
	CryptoAssetProvider     CryptoAssetMarketProvider
	CryptoAssetSourceID     string
	CryptoCandleProvider    CryptoCandleProvider
	CryptoCandleSourceID    string
	CryptoLiquidityProvider CryptoLiquidityProvider
	CryptoLiquiditySourceID string
	CryptoTradeProvider     CryptoTradeProvider
	CryptoTradeSourceID     string
	CryptoStatsProvider     CryptoVenueStatsProvider
	CryptoStatsSourceID     string
	FilingProvider          InsiderFilingProvider
	EquityCacheTTL          time.Duration
	CryptoCacheTTL          time.Duration
	CryptoCandleCacheTTL    time.Duration
	CryptoLiquidityCacheTTL time.Duration
	CryptoTradeCacheTTL     time.Duration
	CryptoStatsCacheTTL     time.Duration
	FilingCacheTTL          time.Duration
	EquityInterval          time.Duration
	CryptoInterval          time.Duration
	CryptoCandleInterval    time.Duration
	CryptoLiquidityInterval time.Duration
	CryptoTradeInterval     time.Duration
	CryptoStatsInterval     time.Duration
	FilingInterval          time.Duration
}

type cacheEntry[T any] struct {
	value     T
	expiresAt time.Time
}

// Service wires production-approved, read-only market providers behind a
// bounded in-memory cache and a runtime source-health catalog. It does not
// expose an order or broker-write dependency.
type Service struct {
	mu sync.RWMutex

	equityProvider          EquityQuoteProvider
	equitySourceID          string
	cryptoProvider          CryptoMarketProvider
	cryptoAssets            CryptoAssetMarketProvider
	cryptoCandles           CryptoCandleProvider
	cryptoLiquidity         CryptoLiquidityProvider
	cryptoTrades            CryptoTradeProvider
	cryptoStats             CryptoVenueStatsProvider
	cryptoSourceID          string
	cryptoAssetSourceID     string
	cryptoCandleSourceID    string
	cryptoLiquiditySourceID string
	cryptoTradeSourceID     string
	cryptoStatsSourceID     string
	filingProvider          InsiderFilingProvider

	equityCacheTTL          time.Duration
	cryptoCacheTTL          time.Duration
	cryptoCandleCacheTTL    time.Duration
	cryptoLiquidityCacheTTL time.Duration
	cryptoTradeCacheTTL     time.Duration
	cryptoStatsCacheTTL     time.Duration
	filingCacheTTL          time.Duration
	equityPacer             requestPacer
	cryptoPacer             requestPacer
	cryptoCandlePacer       requestPacer
	cryptoLiquidityPacer    requestPacer
	cryptoTradePacer        requestPacer
	cryptoStatsPacer        requestPacer
	filingPacer             requestPacer

	sources        []Source
	equityCache    map[string]cacheEntry[QuoteObservation]
	cryptoCache    map[string]cacheEntry[[]CryptoMarketObservation]
	assetCache     map[string]cacheEntry[CryptoMarketBatch]
	candleCache    map[string]cacheEntry[CryptoCandleSeries]
	liquidityCache map[string]cacheEntry[CryptoLiquiditySnapshot]
	tradeCache     map[string]cacheEntry[CryptoTradeTape]
	statsCache     map[string]cacheEntry[CryptoVenueStats]
	filingCache    map[string]cacheEntry[[]InsiderFilingObservation]
	now            func() time.Time
}

type requestPacer struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

func NewService(config ServiceConfig) (*Service, error) {
	if config.EquityProvider != nil {
		if config.EquitySourceID != "alpaca_iex" && config.EquitySourceID != "alpaca_sip" {
			return nil, errors.New("unsupported equity source")
		}
		if config.EquityCacheTTL <= 0 || config.EquityInterval <= 0 {
			return nil, errors.New("equity cache and request policies must be positive")
		}
	}
	if (config.CryptoProvider != nil || config.CryptoAssetProvider != nil) && (config.CryptoCacheTTL <= 0 || config.CryptoInterval <= 0) {
		return nil, errors.New("crypto cache and request policies must be positive")
	}
	if config.CryptoProvider != nil && config.CryptoSourceID != "coingecko_rest" && config.CryptoSourceID != "coinbase_exchange" {
		return nil, errors.New("unsupported crypto source")
	}
	if config.CryptoAssetProvider != nil && config.CryptoAssetSourceID != "coinbase_exchange" {
		return nil, errors.New("unsupported crypto asset source")
	}
	if config.CryptoCandleProvider != nil && config.CryptoCandleSourceID != "coinbase_exchange" {
		return nil, errors.New("unsupported crypto candle source")
	}
	if config.CryptoCandleProvider != nil && (config.CryptoCandleCacheTTL <= 0 || config.CryptoCandleInterval <= 0) {
		return nil, errors.New("crypto candle cache and request policies must be positive")
	}
	if config.CryptoLiquidityProvider != nil && config.CryptoLiquiditySourceID != "coinbase_exchange" {
		return nil, errors.New("unsupported crypto liquidity source")
	}
	if config.CryptoLiquidityProvider != nil && (config.CryptoLiquidityCacheTTL <= 0 || config.CryptoLiquidityInterval <= 0) {
		return nil, errors.New("crypto liquidity cache and request policies must be positive")
	}
	if config.CryptoTradeProvider != nil && config.CryptoTradeSourceID != "coinbase_exchange" {
		return nil, errors.New("unsupported crypto trade source")
	}
	if config.CryptoTradeProvider != nil && (config.CryptoTradeCacheTTL <= 0 || config.CryptoTradeInterval <= 0) {
		return nil, errors.New("crypto trade cache and request policies must be positive")
	}
	if config.CryptoStatsProvider != nil && config.CryptoStatsSourceID != "coinbase_exchange" {
		return nil, errors.New("unsupported crypto venue stats source")
	}
	if config.CryptoStatsProvider != nil && (config.CryptoStatsCacheTTL <= 0 || config.CryptoStatsInterval <= 0) {
		return nil, errors.New("crypto venue stats cache and request policies must be positive")
	}
	if config.FilingProvider != nil && (config.FilingCacheTTL <= 0 || config.FilingInterval <= 0) {
		return nil, errors.New("filing cache and request policies must be positive")
	}

	assetProvider, assetSourceID := config.CryptoAssetProvider, config.CryptoAssetSourceID
	if assetProvider == nil {
		assetProvider = cryptoAssetProvider(config.CryptoProvider)
		assetSourceID = config.CryptoSourceID
	}
	service := &Service{
		equityProvider:          config.EquityProvider,
		equitySourceID:          config.EquitySourceID,
		cryptoProvider:          config.CryptoProvider,
		cryptoAssets:            assetProvider,
		cryptoCandles:           config.CryptoCandleProvider,
		cryptoLiquidity:         config.CryptoLiquidityProvider,
		cryptoTrades:            config.CryptoTradeProvider,
		cryptoStats:             config.CryptoStatsProvider,
		cryptoSourceID:          config.CryptoSourceID,
		cryptoAssetSourceID:     assetSourceID,
		cryptoCandleSourceID:    config.CryptoCandleSourceID,
		cryptoLiquiditySourceID: config.CryptoLiquiditySourceID,
		cryptoTradeSourceID:     config.CryptoTradeSourceID,
		cryptoStatsSourceID:     config.CryptoStatsSourceID,
		filingProvider:          config.FilingProvider,
		equityCacheTTL:          config.EquityCacheTTL,
		cryptoCacheTTL:          config.CryptoCacheTTL,
		cryptoCandleCacheTTL:    config.CryptoCandleCacheTTL,
		cryptoLiquidityCacheTTL: config.CryptoLiquidityCacheTTL,
		cryptoTradeCacheTTL:     config.CryptoTradeCacheTTL,
		cryptoStatsCacheTTL:     config.CryptoStatsCacheTTL,
		filingCacheTTL:          config.FilingCacheTTL,
		equityPacer:             requestPacer{interval: config.EquityInterval},
		cryptoPacer:             requestPacer{interval: config.CryptoInterval},
		cryptoCandlePacer:       requestPacer{interval: config.CryptoCandleInterval},
		cryptoLiquidityPacer:    requestPacer{interval: config.CryptoLiquidityInterval},
		cryptoTradePacer:        requestPacer{interval: config.CryptoTradeInterval},
		cryptoStatsPacer:        requestPacer{interval: config.CryptoStatsInterval},
		filingPacer:             requestPacer{interval: config.FilingInterval},
		sources:                 DefaultSources(),
		equityCache:             make(map[string]cacheEntry[QuoteObservation]),
		cryptoCache:             make(map[string]cacheEntry[[]CryptoMarketObservation]),
		assetCache:              make(map[string]cacheEntry[CryptoMarketBatch]),
		candleCache:             make(map[string]cacheEntry[CryptoCandleSeries]),
		liquidityCache:          make(map[string]cacheEntry[CryptoLiquiditySnapshot]),
		tradeCache:              make(map[string]cacheEntry[CryptoTradeTape]),
		statsCache:              make(map[string]cacheEntry[CryptoVenueStats]),
		filingCache:             make(map[string]cacheEntry[[]InsiderFilingObservation]),
		now:                     func() time.Time { return time.Now().UTC() },
	}
	service.setCapabilityEnabled(config.EquitySourceID, EquityQuote, config.EquityProvider != nil)
	service.setCapabilityEnabled(config.CryptoSourceID, CryptoMarkets, config.CryptoProvider != nil)
	service.setCapabilityEnabled(assetSourceID, CryptoMarkets, assetProvider != nil)
	service.setCapabilityEnabled(config.CryptoCandleSourceID, CryptoCandles, config.CryptoCandleProvider != nil)
	service.setCapabilityEnabled(config.CryptoLiquiditySourceID, CryptoLiquidity, config.CryptoLiquidityProvider != nil)
	service.setCapabilityEnabled(config.CryptoTradeSourceID, CryptoTrades, config.CryptoTradeProvider != nil)
	service.setCapabilityEnabled(config.CryptoStatsSourceID, CryptoStats, config.CryptoStatsProvider != nil)
	service.setCapabilityEnabled("sec_edgar", InsiderFiling, config.FilingProvider != nil)
	return service, nil
}

func cryptoAssetProvider(provider CryptoMarketProvider) CryptoAssetMarketProvider {
	assets, _ := provider.(CryptoAssetMarketProvider)
	return assets
}

func (service *Service) Sources() []Source {
	service.mu.RLock()
	defer service.mu.RUnlock()
	result := make([]Source, len(service.sources))
	for index, source := range service.sources {
		result[index] = source
		result[index].Capabilities = append([]Capability(nil), source.Capabilities...)
		result[index].CapabilityStatus = cloneCapabilityStatuses(source.CapabilityStatus)
	}
	return result
}

func cloneCapabilityStatuses(statuses []CapabilityStatus) []CapabilityStatus {
	result := append([]CapabilityStatus(nil), statuses...)
	for index := range result {
		if result[index].LastAttemptAt != nil {
			value := *result[index].LastAttemptAt
			result[index].LastAttemptAt = &value
		}
		if result[index].LastSuccessAt != nil {
			value := *result[index].LastSuccessAt
			result[index].LastSuccessAt = &value
		}
	}
	return result
}

func (service *Service) LatestEquityQuote(ctx context.Context, symbol string) (QuoteObservation, bool, error) {
	if service.equityProvider == nil {
		return QuoteObservation{}, false, ErrNoEligibleSource
	}
	key := strings.ToUpper(strings.TrimSpace(symbol))
	if !validEquitySymbol(key) {
		return QuoteObservation{}, false, ErrInvalidObservation
	}
	if value, ok := cacheValue(service, service.equityCache, key); ok {
		return value, true, nil
	}
	if err := service.equityPacer.wait(ctx); err != nil {
		return QuoteObservation{}, false, err
	}
	service.recordProviderAttempt(service.equitySourceID, EquityQuote)
	observation, err := service.equityProvider.LatestEquityQuote(ctx, key)
	if err != nil {
		service.recordProviderError(service.equitySourceID, EquityQuote, err)
		return QuoteObservation{}, false, err
	}
	service.recordProviderSuccess(service.equitySourceID, EquityQuote)
	service.storeEquity(key, observation)
	return observation, false, nil
}

func (service *Service) TopCryptoMarkets(ctx context.Context, currency string, limit int) ([]CryptoMarketObservation, bool, error) {
	if service.cryptoProvider == nil {
		return nil, false, ErrNoEligibleSource
	}
	currency = strings.ToLower(strings.TrimSpace(currency))
	if !validReferenceCurrency(currency) || limit < 1 || limit > 100 {
		return nil, false, ErrInvalidObservation
	}
	key := currency + ":" + decimalKey(limit)
	if value, ok := cacheValue(service, service.cryptoCache, key); ok {
		return append([]CryptoMarketObservation(nil), value...), true, nil
	}
	if err := service.cryptoPacer.wait(ctx); err != nil {
		return nil, false, err
	}
	service.recordProviderAttempt(service.cryptoSourceID, CryptoMarkets)
	observations, err := service.cryptoProvider.TopCryptoMarkets(ctx, currency, limit)
	if err != nil {
		service.recordProviderError(service.cryptoSourceID, CryptoMarkets, err)
		return nil, false, err
	}
	service.recordProviderSuccess(service.cryptoSourceID, CryptoMarkets)
	copyValue := append([]CryptoMarketObservation(nil), observations...)
	service.storeCrypto(key, copyValue)
	return append([]CryptoMarketObservation(nil), copyValue...), false, nil
}

// CryptoMarkets returns last-trade observations for a bounded, canonical set
// of portfolio symbols. Unsupported USD products remain explicit in the batch.
func (service *Service) CryptoMarkets(ctx context.Context, currency string, symbols []string) (CryptoMarketBatch, bool, error) {
	if service.cryptoAssets == nil {
		return CryptoMarketBatch{}, false, ErrNoEligibleSource
	}
	canonical, ok := canonicalCryptoSymbols(currency, symbols)
	if !ok {
		return CryptoMarketBatch{}, false, ErrInvalidObservation
	}
	key := "assets:usd:" + strings.Join(canonical, ",")
	if value, cached := cacheValue(service, service.assetCache, key); cached {
		return cloneCryptoBatch(value), true, nil
	}
	if err := service.cryptoPacer.wait(ctx); err != nil {
		return CryptoMarketBatch{}, false, err
	}
	service.recordProviderAttempt(service.cryptoAssetSourceID, CryptoMarkets)
	batch, err := service.cryptoAssets.CryptoMarkets(ctx, "USD", canonical)
	if err != nil {
		service.recordProviderError(service.cryptoAssetSourceID, CryptoMarkets, err)
		return CryptoMarketBatch{}, false, err
	}
	service.recordProviderSuccess(service.cryptoAssetSourceID, CryptoMarkets)
	batch = cloneCryptoBatch(batch)
	service.storeCryptoAssets(key, batch)
	return cloneCryptoBatch(batch), false, nil
}

func canonicalCryptoSymbols(currency string, symbols []string) ([]string, bool) {
	if !strings.EqualFold(strings.TrimSpace(currency), "usd") || len(symbols) == 0 || len(symbols) > 32 {
		return nil, false
	}
	seen := make(map[string]struct{}, len(symbols))
	canonical := make([]string, 0, len(symbols))
	for _, value := range symbols {
		symbol := strings.ToUpper(strings.TrimSpace(value))
		if len(symbol) == 0 || len(symbol) > 12 {
			return nil, false
		}
		for _, character := range symbol {
			if (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
				return nil, false
			}
		}
		if _, exists := seen[symbol]; exists {
			continue
		}
		seen[symbol] = struct{}{}
		canonical = append(canonical, symbol)
	}
	if len(canonical) == 0 {
		return nil, false
	}
	sort.Strings(canonical)
	return canonical, true
}

func cloneCryptoBatch(batch CryptoMarketBatch) CryptoMarketBatch {
	return CryptoMarketBatch{
		Markets:            append([]CryptoMarketObservation(nil), batch.Markets...),
		UnavailableSymbols: append([]string(nil), batch.UnavailableSymbols...),
	}
}

// RecentCryptoCandles returns one canonical, bounded venue series. The cache
// reduces provider polling without changing the response's observation times.
func (service *Service) RecentCryptoCandles(ctx context.Context, symbol, currency string, granularitySeconds, limit int) (CryptoCandleSeries, bool, error) {
	if service.cryptoCandles == nil {
		return CryptoCandleSeries{}, false, ErrNoEligibleSource
	}
	canonical, ok := canonicalCryptoSymbols(currency, []string{symbol})
	if !ok || len(canonical) != 1 || granularitySeconds != 900 || limit != 96 {
		return CryptoCandleSeries{}, false, ErrInvalidObservation
	}
	key := "candles:" + canonical[0] + ":usd:900:96"
	if value, cached := cacheValue(service, service.candleCache, key); cached {
		return cloneCryptoCandleSeries(value), true, nil
	}
	if err := service.cryptoCandlePacer.wait(ctx); err != nil {
		return CryptoCandleSeries{}, false, err
	}
	service.recordProviderAttempt(service.cryptoCandleSourceID, CryptoCandles)
	series, err := service.cryptoCandles.RecentCryptoCandles(ctx, canonical[0], "USD", granularitySeconds, limit)
	if err != nil {
		service.recordProviderError(service.cryptoCandleSourceID, CryptoCandles, err)
		return CryptoCandleSeries{}, false, err
	}
	service.recordProviderSuccess(service.cryptoCandleSourceID, CryptoCandles)
	series = cloneCryptoCandleSeries(series)
	service.storeCryptoCandles(key, series)
	return cloneCryptoCandleSeries(series), false, nil
}

func cloneCryptoCandleSeries(series CryptoCandleSeries) CryptoCandleSeries {
	series.Candles = append([]CryptoCandle(nil), series.Candles...)
	return series
}

// CryptoLiquidity returns exactly the production-approved ten-level book
// contract for one canonical connected asset.
func (service *Service) CryptoLiquidity(ctx context.Context, symbol, currency string, depth int) (CryptoLiquiditySnapshot, bool, error) {
	if service.cryptoLiquidity == nil {
		return CryptoLiquiditySnapshot{}, false, ErrNoEligibleSource
	}
	canonical, ok := canonicalCryptoSymbols(currency, []string{symbol})
	if !ok || len(canonical) != 1 || depth != 10 {
		return CryptoLiquiditySnapshot{}, false, ErrInvalidObservation
	}
	key := "liquidity:" + canonical[0] + ":usd:10"
	if value, cached := cacheValue(service, service.liquidityCache, key); cached {
		return cloneCryptoLiquidity(value), true, nil
	}
	if err := service.cryptoLiquidityPacer.wait(ctx); err != nil {
		return CryptoLiquiditySnapshot{}, false, err
	}
	service.recordProviderAttempt(service.cryptoLiquiditySourceID, CryptoLiquidity)
	snapshot, err := service.cryptoLiquidity.CryptoLiquidity(ctx, canonical[0], "USD", depth)
	if err != nil {
		service.recordProviderError(service.cryptoLiquiditySourceID, CryptoLiquidity, err)
		return CryptoLiquiditySnapshot{}, false, err
	}
	service.recordProviderSuccess(service.cryptoLiquiditySourceID, CryptoLiquidity)
	snapshot = cloneCryptoLiquidity(snapshot)
	service.storeCryptoLiquidity(key, snapshot)
	return cloneCryptoLiquidity(snapshot), false, nil
}

func cloneCryptoLiquidity(snapshot CryptoLiquiditySnapshot) CryptoLiquiditySnapshot {
	snapshot.Bids = append([]CryptoBookLevel(nil), snapshot.Bids...)
	snapshot.Asks = append([]CryptoBookLevel(nil), snapshot.Asks...)
	return snapshot
}

func (service *Service) RecentCryptoTrades(ctx context.Context, symbol, currency string, limit int) (CryptoTradeTape, bool, error) {
	if service.cryptoTrades == nil {
		return CryptoTradeTape{}, false, ErrNoEligibleSource
	}
	canonical, ok := canonicalCryptoSymbols(currency, []string{symbol})
	if !ok || len(canonical) != 1 || limit != 25 {
		return CryptoTradeTape{}, false, ErrInvalidObservation
	}
	key := "trades:" + canonical[0] + ":usd:25"
	if value, cached := cacheValue(service, service.tradeCache, key); cached {
		return cloneCryptoTradeTape(value), true, nil
	}
	if err := service.cryptoTradePacer.wait(ctx); err != nil {
		return CryptoTradeTape{}, false, err
	}
	service.recordProviderAttempt(service.cryptoTradeSourceID, CryptoTrades)
	tape, err := service.cryptoTrades.RecentCryptoTrades(ctx, canonical[0], "USD", limit)
	if err != nil {
		service.recordProviderError(service.cryptoTradeSourceID, CryptoTrades, err)
		return CryptoTradeTape{}, false, err
	}
	service.recordProviderSuccess(service.cryptoTradeSourceID, CryptoTrades)
	tape = cloneCryptoTradeTape(tape)
	service.storeCryptoTradeTape(key, tape)
	return cloneCryptoTradeTape(tape), false, nil
}

func cloneCryptoTradeTape(tape CryptoTradeTape) CryptoTradeTape {
	tape.Trades = append([]CryptoTradeObservation(nil), tape.Trades...)
	return tape
}

func (service *Service) CryptoVenueStats(ctx context.Context, symbol, currency string) (CryptoVenueStats, bool, error) {
	if service.cryptoStats == nil {
		return CryptoVenueStats{}, false, ErrNoEligibleSource
	}
	canonical, ok := canonicalCryptoSymbols(currency, []string{symbol})
	if !ok || len(canonical) != 1 {
		return CryptoVenueStats{}, false, ErrInvalidObservation
	}
	key := "venue-stats:" + canonical[0] + ":usd"
	if value, cached := cacheValue(service, service.statsCache, key); cached {
		return value, true, nil
	}
	if err := service.cryptoStatsPacer.wait(ctx); err != nil {
		return CryptoVenueStats{}, false, err
	}
	service.recordProviderAttempt(service.cryptoStatsSourceID, CryptoStats)
	stats, err := service.cryptoStats.CryptoVenueStats(ctx, canonical[0], "USD")
	if err != nil {
		service.recordProviderError(service.cryptoStatsSourceID, CryptoStats, err)
		return CryptoVenueStats{}, false, err
	}
	service.recordProviderSuccess(service.cryptoStatsSourceID, CryptoStats)
	service.storeCryptoVenueStats(key, stats)
	return stats, false, nil
}

func (service *Service) RecentInsiderFilings(ctx context.Context, cik string, limit int) ([]InsiderFilingObservation, bool, error) {
	if service.filingProvider == nil {
		return nil, false, ErrNoEligibleSource
	}
	cik = strings.TrimSpace(cik)
	if !validCIKQuery(cik) || limit < 1 || limit > 100 {
		return nil, false, ErrInvalidObservation
	}
	key := cik + ":" + decimalKey(limit)
	if value, ok := cacheValue(service, service.filingCache, key); ok {
		return append([]InsiderFilingObservation(nil), value...), true, nil
	}
	if err := service.filingPacer.wait(ctx); err != nil {
		return nil, false, err
	}
	service.recordProviderAttempt("sec_edgar", InsiderFiling)
	observations, err := service.filingProvider.RecentInsiderFilings(ctx, cik, limit)
	if err != nil {
		service.recordProviderError("sec_edgar", InsiderFiling, err)
		return nil, false, err
	}
	service.recordProviderSuccess("sec_edgar", InsiderFiling)
	copyValue := append([]InsiderFilingObservation(nil), observations...)
	service.storeFilings(key, copyValue)
	return append([]InsiderFilingObservation(nil), copyValue...), false, nil
}

func (pacer *requestPacer) wait(ctx context.Context) error {
	for {
		pacer.mu.Lock()
		now := time.Now()
		delay := pacer.next.Sub(now)
		if delay <= 0 {
			pacer.next = now.Add(pacer.interval)
			pacer.mu.Unlock()
			return nil
		}
		pacer.mu.Unlock()

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Probe verifies configured sources without making source availability a
// process-readiness dependency. Each adapter records its own healthy state.
func (service *Service) Probe(ctx context.Context) {
	var wait sync.WaitGroup
	if service.equityProvider != nil {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _, _ = service.LatestEquityQuote(ctx, "SPY")
		}()
	}
	if service.cryptoProvider != nil {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _, _ = service.TopCryptoMarkets(ctx, "usd", 1)
		}()
	}
	if service.filingProvider != nil {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _, _ = service.RecentInsiderFilings(ctx, "0000320193", 1)
		}()
	}
	wait.Wait()
}

func cacheValue[T any](service *Service, cache map[string]cacheEntry[T], key string) (T, bool) {
	service.mu.RLock()
	entry, ok := cache[key]
	now := service.now()
	service.mu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return entry.value, true
	}
	var zero T
	return zero, false
}

func (service *Service) storeEquity(key string, value QuoteObservation) {
	service.mu.Lock()
	defer service.mu.Unlock()
	pruneCache(service.equityCache, service.now())
	service.equityCache[key] = cacheEntry[QuoteObservation]{value: value, expiresAt: service.now().Add(service.equityCacheTTL)}
}

func (service *Service) storeCrypto(key string, value []CryptoMarketObservation) {
	service.mu.Lock()
	defer service.mu.Unlock()
	pruneCache(service.cryptoCache, service.now())
	service.cryptoCache[key] = cacheEntry[[]CryptoMarketObservation]{value: value, expiresAt: service.now().Add(service.cryptoCacheTTL)}
}

func (service *Service) storeCryptoAssets(key string, value CryptoMarketBatch) {
	service.mu.Lock()
	defer service.mu.Unlock()
	pruneCache(service.assetCache, service.now())
	service.assetCache[key] = cacheEntry[CryptoMarketBatch]{value: cloneCryptoBatch(value), expiresAt: service.now().Add(service.cryptoCacheTTL)}
}

func (service *Service) storeCryptoCandles(key string, value CryptoCandleSeries) {
	service.mu.Lock()
	defer service.mu.Unlock()
	pruneCache(service.candleCache, service.now())
	service.candleCache[key] = cacheEntry[CryptoCandleSeries]{value: cloneCryptoCandleSeries(value), expiresAt: service.now().Add(service.cryptoCandleCacheTTL)}
}

func (service *Service) storeCryptoLiquidity(key string, value CryptoLiquiditySnapshot) {
	service.mu.Lock()
	defer service.mu.Unlock()
	pruneCache(service.liquidityCache, service.now())
	service.liquidityCache[key] = cacheEntry[CryptoLiquiditySnapshot]{value: cloneCryptoLiquidity(value), expiresAt: service.now().Add(service.cryptoLiquidityCacheTTL)}
}

func (service *Service) storeCryptoTradeTape(key string, value CryptoTradeTape) {
	service.mu.Lock()
	defer service.mu.Unlock()
	pruneCache(service.tradeCache, service.now())
	service.tradeCache[key] = cacheEntry[CryptoTradeTape]{value: cloneCryptoTradeTape(value), expiresAt: service.now().Add(service.cryptoTradeCacheTTL)}
}

func (service *Service) storeCryptoVenueStats(key string, value CryptoVenueStats) {
	service.mu.Lock()
	defer service.mu.Unlock()
	pruneCache(service.statsCache, service.now())
	service.statsCache[key] = cacheEntry[CryptoVenueStats]{value: value, expiresAt: service.now().Add(service.cryptoStatsCacheTTL)}
}

func (service *Service) storeFilings(key string, value []InsiderFilingObservation) {
	service.mu.Lock()
	defer service.mu.Unlock()
	pruneCache(service.filingCache, service.now())
	service.filingCache[key] = cacheEntry[[]InsiderFilingObservation]{value: value, expiresAt: service.now().Add(service.filingCacheTTL)}
}

func pruneCache[T any](cache map[string]cacheEntry[T], now time.Time) {
	for key, entry := range cache {
		if !now.Before(entry.expiresAt) {
			delete(cache, key)
		}
	}
	if len(cache) < maxCacheEntries {
		return
	}
	var oldestKey string
	var oldest time.Time
	for key, entry := range cache {
		if oldestKey == "" || entry.expiresAt.Before(oldest) {
			oldestKey, oldest = key, entry.expiresAt
		}
	}
	delete(cache, oldestKey)
}

func (service *Service) setCapabilityEnabled(id string, capability Capability, enabled bool) {
	if id == "" || capability == "" || !enabled {
		return
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	for index := range service.sources {
		if service.sources[index].ID != id {
			continue
		}
		for statusIndex := range service.sources[index].CapabilityStatus {
			status := &service.sources[index].CapabilityStatus[statusIndex]
			if status.Capability == capability {
				status.Enabled = true
				status.State = AwaitingObservation
				refreshSourceHealth(&service.sources[index])
				return
			}
		}
	}
}

func (service *Service) recordProviderAttempt(id string, capability Capability) {
	now := service.now()
	service.mu.Lock()
	defer service.mu.Unlock()
	_, status := findCapabilityStatus(service.sources, id, capability)
	if status == nil || !status.Enabled {
		return
	}
	status.LastAttemptAt = timePointer(now)
}

func (service *Service) recordProviderSuccess(id string, capability Capability) {
	now := service.now()
	service.mu.Lock()
	defer service.mu.Unlock()
	source, status := findCapabilityStatus(service.sources, id, capability)
	if status == nil || !status.Enabled {
		return
	}
	status.State = Verified
	status.LastSuccessAt = timePointer(now)
	status.ConsecutiveFailures = 0
	status.FailureCategory = ""
	refreshSourceHealth(source)
}

func (service *Service) recordProviderError(id string, capability Capability, err error) {
	if errors.Is(err, ErrInstrumentUnavailable) || errors.Is(err, context.Canceled) {
		return
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	source, status := findCapabilityStatus(service.sources, id, capability)
	if status == nil || !status.Enabled {
		return
	}
	status.State = Degraded
	status.ConsecutiveFailures++
	status.FailureCategory = providerFailureCategory(err)
	refreshSourceHealth(source)
}

func findCapabilityStatus(sources []Source, id string, capability Capability) (*Source, *CapabilityStatus) {
	for sourceIndex := range sources {
		if sources[sourceIndex].ID != id {
			continue
		}
		for statusIndex := range sources[sourceIndex].CapabilityStatus {
			if sources[sourceIndex].CapabilityStatus[statusIndex].Capability == capability {
				return &sources[sourceIndex], &sources[sourceIndex].CapabilityStatus[statusIndex]
			}
		}
	}
	return nil, nil
}

func refreshSourceHealth(source *Source) {
	if source == nil {
		return
	}
	enabled, verified, degraded := false, false, false
	for _, status := range source.CapabilityStatus {
		if !status.Enabled {
			continue
		}
		enabled = true
		verified = verified || status.State == Verified
		degraded = degraded || status.State == Degraded
	}
	source.Enabled = enabled
	source.Healthy = enabled && verified && !degraded
}

func providerFailureCategory(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "TIMEOUT"
	case errors.Is(err, ErrStaleObservation):
		return "STALE_DATA"
	case errors.Is(err, ErrFutureObservation):
		return "FUTURE_DATED_DATA"
	case errors.Is(err, ErrMissingProvenance):
		return "MISSING_PROVENANCE"
	case errors.Is(err, ErrInvalidObservation):
		return "INVALID_DATA"
	default:
		return "UPSTREAM_FAILURE"
	}
}

func validEquitySymbol(value string) bool {
	if len(value) < 1 || len(value) > 15 || value[0] < 'A' || value[0] > 'Z' {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '.' && character != '-' {
			return false
		}
	}
	return true
}

func validReferenceCurrency(value string) bool {
	if len(value) < 2 || len(value) > 12 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func validCIKQuery(value string) bool {
	if len(value) < 1 || len(value) > 10 {
		return false
	}
	nonzero := false
	for index := range len(value) {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
		nonzero = nonzero || value[index] != '0'
	}
	return nonzero
}

func timePointer(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}

func decimalKey(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
