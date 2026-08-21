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
	EquityProvider      EquityQuoteProvider
	EquitySourceID      string
	CryptoProvider      CryptoMarketProvider
	CryptoSourceID      string
	CryptoAssetProvider CryptoAssetMarketProvider
	CryptoAssetSourceID string
	FilingProvider      InsiderFilingProvider
	EquityCacheTTL      time.Duration
	CryptoCacheTTL      time.Duration
	FilingCacheTTL      time.Duration
	EquityInterval      time.Duration
	CryptoInterval      time.Duration
	FilingInterval      time.Duration
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

	equityProvider      EquityQuoteProvider
	equitySourceID      string
	cryptoProvider      CryptoMarketProvider
	cryptoAssets        CryptoAssetMarketProvider
	cryptoSourceID      string
	cryptoAssetSourceID string
	filingProvider      InsiderFilingProvider

	equityCacheTTL time.Duration
	cryptoCacheTTL time.Duration
	filingCacheTTL time.Duration
	equityPacer    requestPacer
	cryptoPacer    requestPacer
	filingPacer    requestPacer

	sources     []Source
	equityCache map[string]cacheEntry[QuoteObservation]
	cryptoCache map[string]cacheEntry[[]CryptoMarketObservation]
	assetCache  map[string]cacheEntry[CryptoMarketBatch]
	filingCache map[string]cacheEntry[[]InsiderFilingObservation]
	now         func() time.Time
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
	if config.FilingProvider != nil && (config.FilingCacheTTL <= 0 || config.FilingInterval <= 0) {
		return nil, errors.New("filing cache and request policies must be positive")
	}

	assetProvider, assetSourceID := config.CryptoAssetProvider, config.CryptoAssetSourceID
	if assetProvider == nil {
		assetProvider = cryptoAssetProvider(config.CryptoProvider)
		assetSourceID = config.CryptoSourceID
	}
	service := &Service{
		equityProvider:      config.EquityProvider,
		equitySourceID:      config.EquitySourceID,
		cryptoProvider:      config.CryptoProvider,
		cryptoAssets:        assetProvider,
		cryptoSourceID:      config.CryptoSourceID,
		cryptoAssetSourceID: assetSourceID,
		filingProvider:      config.FilingProvider,
		equityCacheTTL:      config.EquityCacheTTL,
		cryptoCacheTTL:      config.CryptoCacheTTL,
		filingCacheTTL:      config.FilingCacheTTL,
		equityPacer:         requestPacer{interval: config.EquityInterval},
		cryptoPacer:         requestPacer{interval: config.CryptoInterval},
		filingPacer:         requestPacer{interval: config.FilingInterval},
		sources:             DefaultSources(),
		equityCache:         make(map[string]cacheEntry[QuoteObservation]),
		cryptoCache:         make(map[string]cacheEntry[[]CryptoMarketObservation]),
		assetCache:          make(map[string]cacheEntry[CryptoMarketBatch]),
		filingCache:         make(map[string]cacheEntry[[]InsiderFilingObservation]),
		now:                 func() time.Time { return time.Now().UTC() },
	}
	service.setEnabled(config.EquitySourceID, config.EquityProvider != nil)
	service.setEnabled(config.CryptoSourceID, config.CryptoProvider != nil)
	service.setEnabled(assetSourceID, assetProvider != nil)
	service.setEnabled("sec_edgar", config.FilingProvider != nil)
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
	}
	return result
}

func (service *Service) LatestEquityQuote(ctx context.Context, symbol string) (QuoteObservation, bool, error) {
	if service.equityProvider == nil {
		return QuoteObservation{}, false, ErrNoEligibleSource
	}
	key := strings.ToUpper(strings.TrimSpace(symbol))
	if value, ok := cacheValue(service, service.equityCache, key); ok {
		return value, true, nil
	}
	if err := service.equityPacer.wait(ctx); err != nil {
		return QuoteObservation{}, false, err
	}
	observation, err := service.equityProvider.LatestEquityQuote(ctx, key)
	if err != nil {
		service.recordProviderError(service.equitySourceID, err)
		return QuoteObservation{}, false, err
	}
	service.setHealthy(service.equitySourceID, true)
	service.storeEquity(key, observation)
	return observation, false, nil
}

func (service *Service) TopCryptoMarkets(ctx context.Context, currency string, limit int) ([]CryptoMarketObservation, bool, error) {
	if service.cryptoProvider == nil {
		return nil, false, ErrNoEligibleSource
	}
	key := strings.ToLower(strings.TrimSpace(currency)) + ":" + decimalKey(limit)
	if value, ok := cacheValue(service, service.cryptoCache, key); ok {
		return append([]CryptoMarketObservation(nil), value...), true, nil
	}
	if err := service.cryptoPacer.wait(ctx); err != nil {
		return nil, false, err
	}
	observations, err := service.cryptoProvider.TopCryptoMarkets(ctx, currency, limit)
	if err != nil {
		service.recordProviderError(service.cryptoSourceID, err)
		return nil, false, err
	}
	service.setHealthy(service.cryptoSourceID, true)
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
	batch, err := service.cryptoAssets.CryptoMarkets(ctx, "USD", canonical)
	if err != nil {
		service.recordProviderError(service.cryptoAssetSourceID, err)
		return CryptoMarketBatch{}, false, err
	}
	service.setHealthy(service.cryptoAssetSourceID, true)
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

func (service *Service) RecentInsiderFilings(ctx context.Context, cik string, limit int) ([]InsiderFilingObservation, bool, error) {
	if service.filingProvider == nil {
		return nil, false, ErrNoEligibleSource
	}
	key := strings.TrimSpace(cik) + ":" + decimalKey(limit)
	if value, ok := cacheValue(service, service.filingCache, key); ok {
		return append([]InsiderFilingObservation(nil), value...), true, nil
	}
	if err := service.filingPacer.wait(ctx); err != nil {
		return nil, false, err
	}
	observations, err := service.filingProvider.RecentInsiderFilings(ctx, cik, limit)
	if err != nil {
		service.recordProviderError("sec_edgar", err)
		return nil, false, err
	}
	service.setHealthy("sec_edgar", true)
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

func (service *Service) setEnabled(id string, enabled bool) {
	if id == "" || !enabled {
		return
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	for index := range service.sources {
		if service.sources[index].ID == id {
			service.sources[index].Enabled = true
			return
		}
	}
}

func (service *Service) setHealthy(id string, healthy bool) {
	service.mu.Lock()
	defer service.mu.Unlock()
	for index := range service.sources {
		if service.sources[index].ID == id {
			service.sources[index].Healthy = healthy
			return
		}
	}
}

func (service *Service) recordProviderError(id string, err error) {
	if errors.Is(err, ErrInvalidObservation) {
		return
	}
	service.setHealthy(id, false)
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
