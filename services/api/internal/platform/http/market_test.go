package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/marketintelligence"
	"github.com/arbion/platform/services/api/internal/platform/config"
)

type fakeMarketIntelligence struct {
	sources    []marketintelligence.Source
	history    marketintelligence.HealthHistory
	watchlist  []marketintelligence.WatchlistItem
	err        error
	historyErr error
	watchErr   error
}

func (fake fakeMarketIntelligence) Sources() []marketintelligence.Source { return fake.sources }
func (fake fakeMarketIntelligence) SourceHealthHistory(context.Context) (marketintelligence.HealthHistory, error) {
	return fake.history, fake.historyErr
}
func (fake fakeMarketIntelligence) ListWatchlist(context.Context, string) ([]marketintelligence.WatchlistItem, error) {
	return fake.watchlist, fake.watchErr
}
func (fake fakeMarketIntelligence) CreateWatchlistItem(_ context.Context, _ string, symbol string) (marketintelligence.WatchlistItem, error) {
	if fake.watchErr != nil {
		return marketintelligence.WatchlistItem{}, fake.watchErr
	}
	return marketintelligence.WatchlistItem{ID: "3f6ab43c-8abd-4056-a858-ccb8051a045f", AssetClass: marketintelligence.Crypto, Symbol: symbol, QuoteCurrency: "USD", CreatedAt: time.Now().UTC()}, nil
}
func (fake fakeMarketIntelligence) DeleteWatchlistItem(context.Context, string, string) error {
	return fake.watchErr
}
func (fake fakeMarketIntelligence) LatestEquityQuote(_ context.Context, symbol string) (marketintelligence.QuoteObservation, bool, error) {
	if fake.err != nil {
		return marketintelligence.QuoteObservation{}, false, fake.err
	}
	bid := marketintelligence.Decimal("101.25")
	return marketintelligence.QuoteObservation{Symbol: symbol, AssetClass: marketintelligence.Equity, Currency: "USD", Bid: &bid}, true, nil
}
func (fake fakeMarketIntelligence) TopCryptoMarkets(_ context.Context, currency string, _ int) ([]marketintelligence.CryptoMarketObservation, bool, error) {
	if fake.err != nil {
		return nil, false, fake.err
	}
	return []marketintelligence.CryptoMarketObservation{{ID: "bitcoin", Symbol: "BTC", Currency: currency, CurrentPrice: marketintelligence.Decimal("60000")}}, false, nil
}
func (fake fakeMarketIntelligence) CryptoMarkets(_ context.Context, currency string, symbols []string) (marketintelligence.CryptoMarketBatch, bool, error) {
	if fake.err != nil {
		return marketintelligence.CryptoMarketBatch{}, false, fake.err
	}
	markets := make([]marketintelligence.CryptoMarketObservation, 0, len(symbols))
	for _, symbol := range symbols {
		price := marketintelligence.Decimal("100")
		if symbol == "BTC" {
			price = "60000"
		}
		markets = append(markets, marketintelligence.CryptoMarketObservation{
			ID: symbol + "-USD", Symbol: symbol, Name: symbol, Currency: currency, CurrentPrice: price,
			Provenance: marketintelligence.Provenance{Provider: "coinbase", Role: marketintelligence.MarketObservation, Feed: "rest_ticker", Quality: marketintelligence.RealTimeSingleVenue, Venue: "coinbase_exchange", ProviderTimestamp: time.Now().UTC(), ReceivedAt: time.Now().UTC()},
		})
	}
	return marketintelligence.CryptoMarketBatch{Markets: markets}, false, nil
}
func (fake fakeMarketIntelligence) RecentCryptoCandles(_ context.Context, symbol, currency string, granularity, limit int) (marketintelligence.CryptoCandleSeries, bool, error) {
	if fake.err != nil {
		return marketintelligence.CryptoCandleSeries{}, false, fake.err
	}
	now := time.Now().UTC().Truncate(time.Minute)
	return marketintelligence.CryptoCandleSeries{
		Symbol: symbol, Currency: currency, GranularitySeconds: granularity, ExpectedIntervals: limit,
		Candles:    []marketintelligence.CryptoCandle{{Start: now.Add(-15 * time.Minute), Low: "59000", High: "61000", Open: "60000", Close: "60500", Volume: "10"}},
		Provenance: marketintelligence.Provenance{Provider: "coinbase", Role: marketintelligence.MarketObservation, Feed: "rest_candles", Quality: marketintelligence.RealTimeSingleVenue, Venue: "coinbase_exchange", ProviderTimestamp: now.Add(-15 * time.Minute), ReceivedAt: now},
	}, false, nil
}
func (fake fakeMarketIntelligence) CryptoLiquidity(_ context.Context, symbol, currency string, depth int) (marketintelligence.CryptoLiquiditySnapshot, bool, error) {
	if fake.err != nil {
		return marketintelligence.CryptoLiquiditySnapshot{}, false, fake.err
	}
	now := time.Now().UTC()
	return marketintelligence.CryptoLiquiditySnapshot{
		Symbol: symbol, Currency: currency, ProductID: symbol + "-" + currency, Depth: depth,
		Bids: []marketintelligence.CryptoBookLevel{{Price: "70186.90", Size: "0.12500000"}},
		Asks: []marketintelligence.CryptoBookLevel{{Price: "70187.20", Size: "0.25000000"}},
		Last: "70187.10", MidMarket: "70187.05", SpreadBPS: "0.042743", SpreadAbsolute: "0.30",
		Provenance: marketintelligence.Provenance{Provider: "coinbase", Role: marketintelligence.MarketObservation, Feed: "advanced_trade_public_product_book", Quality: marketintelligence.RealTimeSingleVenue, Venue: "coinbase_advanced_trade", ProviderTimestamp: now, ReceivedAt: now},
	}, false, nil
}
func (fake fakeMarketIntelligence) RecentCryptoTrades(_ context.Context, symbol, currency string, limit int) (marketintelligence.CryptoTradeTape, bool, error) {
	if fake.err != nil {
		return marketintelligence.CryptoTradeTape{}, false, fake.err
	}
	now := time.Now().UTC()
	return marketintelligence.CryptoTradeTape{
		Symbol: symbol, Currency: currency, ProductID: symbol + "-" + currency, Limit: limit,
		Trades:  []marketintelligence.CryptoTradeObservation{{Price: "70187.10", Size: "0.00012500", Time: now, Side: "BUY"}},
		BestBid: "70186.90", BestAsk: "70187.20",
		Provenance: marketintelligence.Provenance{Provider: "coinbase", Role: marketintelligence.MarketObservation, Feed: "advanced_trade_public_market_trades", Quality: marketintelligence.RealTimeSingleVenue, Venue: "coinbase_advanced_trade", ProviderTimestamp: now, ReceivedAt: now},
	}, false, nil
}
func (fake fakeMarketIntelligence) CryptoVenueStats(_ context.Context, symbol, currency string) (marketintelligence.CryptoVenueStats, bool, error) {
	if fake.err != nil {
		return marketintelligence.CryptoVenueStats{}, false, fake.err
	}
	return marketintelligence.CryptoVenueStats{
		Symbol: symbol, Currency: currency, ProductID: symbol + "-" + currency,
		Open: "72715.34", High: "79500", Low: "72303.97", Last: "77522.97", Volume24H: "19734.31498542", Volume30Day: "189836.08275489", VolumeUnit: symbol,
		Receipt: marketintelligence.SourceReceipt{Provider: "coinbase", Role: marketintelligence.MarketObservation, Feed: "exchange_public_product_stats", Quality: marketintelligence.RealTimeSingleVenue, Venue: "coinbase_exchange", ReceivedAt: time.Now().UTC()},
	}, false, nil
}
func (fake fakeMarketIntelligence) RecentInsiderFilings(_ context.Context, cik string, _ int) ([]marketintelligence.InsiderFilingObservation, bool, error) {
	if fake.err != nil {
		return nil, false, fake.err
	}
	return []marketintelligence.InsiderFilingObservation{{IssuerCIK: cik, Form: "4"}}, false, nil
}

type fakeBrokerMarketData struct {
	accounts  []financial.FinancialAccount
	balances  *financial.Balances
	positions []financial.Position
	quote     financial.Quote
	chain     financial.OptionChain
	query     financial.OptionChainRequest
	fills     financial.TradeFillPage
	orders    financial.OrderHistoryPage
	costs     financial.TradingCostSummary
}

func (fake *fakeBrokerMarketData) ListAccounts(context.Context, authorization.Principal) ([]financial.FinancialAccount, error) {
	return fake.accounts, nil
}

func (fake *fakeBrokerMarketData) GetAccount(_ context.Context, _ authorization.Principal, id string) (financial.FinancialAccount, error) {
	for _, account := range fake.accounts {
		if account.ID == id {
			return account, nil
		}
	}
	return financial.FinancialAccount{}, errors.New("not found")
}

func (fake *fakeBrokerMarketData) GetBalances(context.Context, authorization.Principal, string) (financial.Balances, error) {
	if fake.balances != nil {
		return *fake.balances, nil
	}
	return financial.Balances{Cash: &financial.Money{Amount: "25", Currency: "USD"}, AvailableCash: &financial.Money{Amount: "20", Currency: "USD"}}, nil
}

func (fake *fakeBrokerMarketData) GetPositions(context.Context, authorization.Principal, string) ([]financial.Position, error) {
	if fake.positions != nil {
		return fake.positions, nil
	}
	available := financial.Decimal("0.3")
	unavailable := financial.Decimal("0.2")
	return []financial.Position{{InstrumentType: "CRYPTO", Symbol: "BTC", Quantity: "0.5", AvailableQuantity: &available, UnavailableToTradeQuantity: &unavailable, Direction: "long"}}, nil
}

func (fake *fakeBrokerMarketData) GetTradeFills(context.Context, authorization.Principal, string) (financial.TradeFillPage, error) {
	return fake.fills, nil
}

func (fake *fakeBrokerMarketData) GetOrderHistory(context.Context, authorization.Principal, string) (financial.OrderHistoryPage, error) {
	return fake.orders, nil
}

func (fake *fakeBrokerMarketData) GetTradingCostSummary(context.Context, authorization.Principal, string) (financial.TradingCostSummary, error) {
	return fake.costs, nil
}

func (fake *fakeBrokerMarketData) GetQuote(context.Context, authorization.Principal, string, string) (financial.Quote, error) {
	return fake.quote, nil
}

func (fake *fakeBrokerMarketData) GetOptionChain(_ context.Context, _ authorization.Principal, _ string, query financial.OptionChainRequest) (financial.OptionChain, error) {
	fake.query = query
	return fake.chain, nil
}

func TestMarketSourcesAreNoStoreReadOnlyMetadata(t *testing.T) {
	handler := &authHandler{marketSources: marketintelligence.DefaultSources()}
	recorder := httptest.NewRecorder()
	handler.listMarketSources(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/markets/sources", nil))

	var response struct {
		Sources                []marketintelligence.Source `json:"sources"`
		StatusGeneratedAt      time.Time                   `json:"status_generated_at"`
		StatusSemantics        string                      `json:"status_semantics"`
		RequestUsageSemantics  string                      `json:"request_usage_semantics"`
		ProviderQuotaExposed   bool                        `json:"provider_quota_exposed"`
		ProviderErrorsExposed  bool                        `json:"provider_errors_exposed"`
		LiveExecutionAvailable bool                        `json:"live_execution_available"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected response metadata: status=%d cache=%q", recorder.Code, recorder.Header().Get("Cache-Control"))
	}
	if len(response.Sources) != 8 || response.LiveExecutionAvailable || response.ProviderQuotaExposed || response.ProviderErrorsExposed || response.StatusGeneratedAt.IsZero() || response.StatusSemantics != "PROCESS_LOCAL_TIME_BOUNDED_PROVIDER_VERIFICATION" || response.RequestUsageSemantics != "PROCESS_LOCAL_BOUNDED_AGGREGATES" {
		t.Fatalf("unexpected market source response: %+v", response)
	}
	for _, source := range response.Sources {
		if source.Enabled || source.Healthy {
			t.Fatalf("unwired source exposed as available: %+v", source)
		}
		for _, status := range source.CapabilityStatus {
			if status.Enabled || status.State != marketintelligence.NotConfigured || status.LastAttemptAt != nil || status.LastSuccessAt != nil {
				t.Fatalf("unwired capability exposed as verified: %+v", status)
			}
		}
	}
}

func TestMarketSourceHistoryIsNoStoreBoundedAndReadOnly(t *testing.T) {
	now := time.Date(2026, time.August, 21, 20, 0, 0, 0, time.UTC)
	handler := &authHandler{markets: fakeMarketIntelligence{history: marketintelligence.HealthHistory{
		Buckets: []marketintelligence.HealthBucket{{
			SourceID: "coinbase_exchange", Capability: marketintelligence.CryptoMarkets,
			IntervalStarted: now.Add(-time.Hour), LastObservedAt: now.Add(-30 * time.Minute),
			CompletedAttempts: 4, Successes: 3, Failures: 1, LastState: marketintelligence.Degraded, FailureCategory: "TIMEOUT",
		}},
		WindowStartedAt: now.Add(-24 * time.Hour), WindowEndedAt: now, WindowHours: 24, IntervalMinutes: 60,
	}}}
	recorder := httptest.NewRecorder()
	handler.marketSourceHistory(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/markets/source-history", nil))
	if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected history response metadata: status=%d cache=%q", recorder.Code, recorder.Header().Get("Cache-Control"))
	}
	var response struct {
		Buckets                  []marketintelligence.HealthBucket `json:"buckets"`
		WindowHours              int                               `json:"window_hours"`
		IntervalMinutes          int                               `json:"interval_minutes"`
		HistorySemantics         string                            `json:"history_semantics"`
		SubjectDimensionsExposed bool                              `json:"subject_dimensions_exposed"`
		RawProviderErrorsExposed bool                              `json:"raw_provider_errors_exposed"`
		LiveExecutionAvailable   bool                              `json:"live_execution_available"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Buckets) != 1 || response.WindowHours != 24 || response.IntervalMinutes != 60 || response.HistorySemantics != "DURABLE_PROVIDER_OUTCOMES_5_MINUTE_STORAGE_HOURLY_VIEW" || response.SubjectDimensionsExposed || response.RawProviderErrorsExposed || response.LiveExecutionAvailable {
		t.Fatalf("unexpected market source history response: %+v", response)
	}
	if strings.Contains(recorder.Body.String(), "provider_request") || strings.Contains(recorder.Body.String(), "symbol") || strings.Contains(recorder.Body.String(), "raw_error") {
		t.Fatalf("history response exposed a prohibited field: %s", recorder.Body.String())
	}
}

func TestMarketSourceHistoryFailsClosedWithoutRawDiagnostics(t *testing.T) {
	handler := &authHandler{markets: fakeMarketIntelligence{historyErr: errors.New("database password leaked")}}
	recorder := httptest.NewRecorder()
	handler.marketSourceHistory(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/markets/source-history", nil))
	if recorder.Code != stdhttp.StatusServiceUnavailable || strings.Contains(recorder.Body.String(), "password") {
		t.Fatalf("history failure did not fail closed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestMarketWatchlistReturnsSavedItemsWithCurrentVenueEvidence(t *testing.T) {
	now := time.Now().UTC()
	handler := &authHandler{markets: fakeMarketIntelligence{watchlist: []marketintelligence.WatchlistItem{{
		ID: "3f6ab43c-8abd-4056-a858-ccb8051a045f", AssetClass: marketintelligence.Crypto,
		Symbol: "BTC", QuoteCurrency: "USD", CreatedAt: now,
	}}}}
	recorder := httptest.NewRecorder()
	handler.listMarketWatchlist(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/markets/watchlist", nil))
	if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected watchlist metadata: status=%d cache=%q", recorder.Code, recorder.Header().Get("Cache-Control"))
	}
	for _, required := range []string{`"market_state":"READY"`, `"symbol":"BTC"`, `"current_price":"60000"`, `"venue":"coinbase_exchange"`, `"max_items":12`, `"provider_write_available":false`, `"order_actions_available":false`, `"live_execution_available":false`} {
		if !strings.Contains(recorder.Body.String(), required) {
			t.Fatalf("watchlist response missing %s: %s", required, recorder.Body.String())
		}
	}
}

func TestMarketWatchlistPreservesSavedItemsWhenProviderFails(t *testing.T) {
	handler := &authHandler{markets: fakeMarketIntelligence{
		watchlist: []marketintelligence.WatchlistItem{{ID: "3f6ab43c-8abd-4056-a858-ccb8051a045f", AssetClass: marketintelligence.Crypto, Symbol: "BTC", QuoteCurrency: "USD", CreatedAt: time.Now().UTC()}},
		err:       errors.New("secret provider diagnostic"),
	}}
	recorder := httptest.NewRecorder()
	handler.listMarketWatchlist(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/markets/watchlist", nil))
	if recorder.Code != stdhttp.StatusOK || !strings.Contains(recorder.Body.String(), `"market_state":"UNAVAILABLE"`) || !strings.Contains(recorder.Body.String(), `"symbol":"BTC"`) || strings.Contains(recorder.Body.String(), "secret provider diagnostic") {
		t.Fatalf("provider failure hid the durable watchlist or leaked diagnostics: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestMarketWatchlistMutationsAreCSRFProtectedAndNonExecutable(t *testing.T) {
	handler := &authHandler{
		markets: fakeMarketIntelligence{},
		cfg:     config.Auth{AllowedOrigins: []string{"https://www.arbion.ai"}},
	}
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/markets/watchlist", strings.NewReader(`{"symbol":"BTC"}`))
	recorder := httptest.NewRecorder()
	handler.createMarketWatchlistItem(recorder, request)
	if recorder.Code != stdhttp.StatusForbidden {
		t.Fatalf("watchlist mutation bypassed CSRF: status=%d", recorder.Code)
	}

	request = httptest.NewRequest(stdhttp.MethodPost, "/api/markets/watchlist", strings.NewReader(`{"symbol":"BTC"}`))
	request.Header.Set("Origin", "https://www.arbion.ai")
	recorder = httptest.NewRecorder()
	handler.createMarketWatchlistItem(recorder, request)
	if recorder.Code != stdhttp.StatusCreated || !strings.Contains(recorder.Body.String(), `"provider_write_available":false`) || !strings.Contains(recorder.Body.String(), `"live_execution_available":false`) {
		t.Fatalf("unexpected watchlist creation response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(stdhttp.MethodDelete, "/api/markets/watchlist/3f6ab43c-8abd-4056-a858-ccb8051a045f", nil)
	request.SetPathValue("id", "3f6ab43c-8abd-4056-a858-ccb8051a045f")
	request.Header.Set("Origin", "https://www.arbion.ai")
	recorder = httptest.NewRecorder()
	handler.deleteMarketWatchlistItem(recorder, request)
	if recorder.Code != stdhttp.StatusNoContent {
		t.Fatalf("unexpected watchlist delete response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestMarketWatchlistErrorsStayBounded(t *testing.T) {
	handler := &authHandler{
		markets: fakeMarketIntelligence{watchErr: marketintelligence.ErrWatchlistConflict},
		cfg:     config.Auth{AllowedOrigins: []string{"https://www.arbion.ai"}},
	}
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/markets/watchlist", strings.NewReader(`{"symbol":"BTC"}`))
	request.Header.Set("Origin", "https://www.arbion.ai")
	recorder := httptest.NewRecorder()
	handler.createMarketWatchlistItem(recorder, request)
	if recorder.Code != stdhttp.StatusConflict || !strings.Contains(recorder.Body.String(), "WATCHLIST_ITEM_EXISTS") {
		t.Fatalf("watchlist conflict was not bounded: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestMarketSourcesExposeOnlyTheCurrentUsersActiveSchwabSource(t *testing.T) {
	broker := &fakeBrokerMarketData{accounts: []financial.FinancialAccount{{ID: "account-1", Provider: "schwab", Status: "active"}}}
	handler := &authHandler{marketSources: marketintelligence.DefaultSources(), marketFinancial: broker}
	recorder := httptest.NewRecorder()
	handler.listMarketSources(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/markets/sources", nil))
	if recorder.Code != stdhttp.StatusOK || !strings.Contains(recorder.Body.String(), `"id":"schwab_broker_market_data"`) {
		t.Fatalf("Schwab source missing: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"enabled":true,"healthy":true`) {
		t.Fatalf("active user source not marked available: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"state":"AWAITING_OBSERVATION"`) {
		t.Fatalf("broker access was mislabeled as a completed provider observation: %s", recorder.Body.String())
	}
}

func TestBrokerMarketRoutesNormalizeReadOnlyAccountScopedObservations(t *testing.T) {
	now := time.Now().UTC()
	realtime, delayed := true, false
	bid, ask, underlying, strike := financial.Decimal("100.10"), financial.Decimal("100.20"), financial.Decimal("100.15"), financial.Decimal("95")
	broker := &fakeBrokerMarketData{
		accounts: []financial.FinancialAccount{{ID: "account-1", Provider: "schwab", BaseCurrency: "USD", Status: "active"}},
		quote:    financial.Quote{Symbol: "SPY", Bid: &bid, Ask: &ask, ProviderTimestamp: now.Add(-time.Second), Realtime: &realtime},
		chain: financial.OptionChain{
			Symbol: "SPY", UnderlyingPrice: &underlying, ProviderTimestamp: now.Add(-time.Second), Delayed: &delayed,
			Contracts: []financial.OptionContract{{Symbol: "SPY   261016P00095000", Underlying: "SPY", PutCall: "PUT", Expiration: "2026-10-16", Strike: strike, Bid: &bid, ProviderTimestamp: now.Add(-time.Second)}},
		},
	}
	handler := &authHandler{marketFinancial: broker}

	quoteRequest := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/account-1/markets/equities/SPY/quote", nil)
	quoteRequest.SetPathValue("id", "account-1")
	quoteRequest.SetPathValue("symbol", "SPY")
	quoteRecorder := httptest.NewRecorder()
	handler.brokerEquityQuote(quoteRecorder, quoteRequest)
	if quoteRecorder.Code != stdhttp.StatusOK || !strings.Contains(quoteRecorder.Body.String(), `"provider":"schwab"`) || !strings.Contains(quoteRecorder.Body.String(), `"live_execution_available":false`) {
		t.Fatalf("broker quote was not normalized safely: status=%d body=%s", quoteRecorder.Code, quoteRecorder.Body.String())
	}

	optionRequest := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/account-1/markets/options?symbol=SPY&contract_type=PUT&strike_count=12", nil)
	optionRequest.SetPathValue("id", "account-1")
	optionRecorder := httptest.NewRecorder()
	handler.brokerOptionChain(optionRecorder, optionRequest)
	if optionRecorder.Code != stdhttp.StatusOK || broker.query.Symbol != "SPY" || broker.query.StrikeCount != 12 || !strings.Contains(optionRecorder.Body.String(), `"quality":"REAL_TIME_CONSOLIDATED"`) {
		t.Fatalf("broker option chain was not normalized safely: status=%d query=%+v body=%s", optionRecorder.Code, broker.query, optionRecorder.Body.String())
	}
}

func TestCryptoPortfolioCombinesHoldingsWithExplicitReadOnlyVenueObservations(t *testing.T) {
	broker := &fakeBrokerMarketData{accounts: []financial.FinancialAccount{{ID: "coinbase-1", Provider: "coinbase", BaseCurrency: "USD", Status: "active"}}}
	handler := &authHandler{marketFinancial: broker, markets: fakeMarketIntelligence{}}
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/coinbase-1/portfolio/crypto", nil)
	request.SetPathValue("id", "coinbase-1")
	recorder := httptest.NewRecorder()

	handler.cryptoPortfolio(recorder, request)

	body := recorder.Body.String()
	if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected portfolio response: status=%d cache=%q body=%s", recorder.Code, recorder.Header().Get("Cache-Control"), body)
	}
	for _, expected := range []string{`"observed_value":{"amount":"30025"`, `"digital_asset_value":{"amount":"30000"`, `"available_quantity":"0.3"`, `"unavailable_to_trade_quantity":"0.2"`, `"pricing_state":"READY"`, `"pricing_basis":"LAST_TRADE"`, `"venue":"coinbase_exchange"`, `"live_execution_available":false`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("portfolio evidence missing %s: %s", expected, body)
		}
	}
}

func TestCryptoPortfolioIncludesCoinbaseUSDCAtExplicitRedemptionReference(t *testing.T) {
	availableBTC := financial.Decimal("0.3")
	availableUSDC := financial.Decimal("5.3263083")
	unavailableUSDC := financial.Decimal("17534.7")
	broker := &fakeBrokerMarketData{
		accounts: []financial.FinancialAccount{{ID: "coinbase-1", Provider: "coinbase", BaseCurrency: "USD", Status: "active"}},
		positions: []financial.Position{
			{InstrumentType: "CRYPTO", Symbol: "BTC", Quantity: "0.5", AvailableQuantity: &availableBTC, Direction: "long"},
			{InstrumentType: "CRYPTO", Symbol: "USDC", Quantity: "17540.0263083", AvailableQuantity: &availableUSDC, UnavailableToTradeQuantity: &unavailableUSDC, Direction: "long"},
		},
	}
	handler := &authHandler{marketFinancial: broker, markets: fakeMarketIntelligence{}}
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/coinbase-1/portfolio/crypto", nil)
	request.SetPathValue("id", "coinbase-1")
	recorder := httptest.NewRecorder()

	handler.cryptoPortfolio(recorder, request)

	body := recorder.Body.String()
	for _, expected := range []string{
		`"observed_value":{"amount":"47565.0263083"`,
		`"digital_asset_value":{"amount":"47540.0263083"`,
		`"symbol":"USDC","quantity":"17540.0263083","available_quantity":"5.3263083","unavailable_to_trade_quantity":"17534.7","unit_price":{"amount":"1","currency":"USD"},"market_value":{"amount":"17540.0263083","currency":"USD"},"pricing_status":"PRICED","valuation_basis":"COINBASE_USDC_USD_REDEMPTION"`,
		`"priced_positions":2`,
		`"pricing_complete":true`,
		`"pricing_basis":"LAST_TRADE_AND_USDC_USD_REDEMPTION"`,
	} {
		if recorder.Code != stdhttp.StatusOK || !strings.Contains(body, expected) {
			t.Fatalf("USDC redemption-reference valuation missing %s: status=%d body=%s", expected, recorder.Code, body)
		}
	}
}

func TestCryptoPortfolioPreservesHoldingsWhenPricingIsUnavailable(t *testing.T) {
	broker := &fakeBrokerMarketData{accounts: []financial.FinancialAccount{{ID: "coinbase-1", Provider: "coinbase", BaseCurrency: "USD", Status: "active"}}}
	handler := &authHandler{marketFinancial: broker, markets: fakeMarketIntelligence{err: errors.New("provider failed")}}
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/coinbase-1/portfolio/crypto", nil)
	request.SetPathValue("id", "coinbase-1")
	recorder := httptest.NewRecorder()

	handler.cryptoPortfolio(recorder, request)

	body := recorder.Body.String()
	if recorder.Code != stdhttp.StatusOK || !strings.Contains(body, `"pricing_state":"UNAVAILABLE"`) || !strings.Contains(body, `"pricing_status":"UNAVAILABLE"`) || !strings.Contains(body, `"quantity":"0.5"`) {
		t.Fatalf("pricing failure hid holdings or fabricated value: status=%d body=%s", recorder.Code, body)
	}
}

func TestCryptoPortfolioRejectsNonCoinbaseAccounts(t *testing.T) {
	broker := &fakeBrokerMarketData{accounts: []financial.FinancialAccount{{ID: "schwab-1", Provider: "schwab", BaseCurrency: "USD", Status: "active"}}}
	handler := &authHandler{marketFinancial: broker, markets: fakeMarketIntelligence{}}
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/schwab-1/portfolio/crypto", nil)
	request.SetPathValue("id", "schwab-1")
	recorder := httptest.NewRecorder()

	handler.cryptoPortfolio(recorder, request)

	if recorder.Code != stdhttp.StatusBadRequest || !strings.Contains(recorder.Body.String(), "PORTFOLIO_PRICING_UNSUPPORTED") {
		t.Fatalf("non-Coinbase account entered crypto view: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestConnectedCryptoCandlesAreOwnerScopedBoundedAndReadOnly(t *testing.T) {
	broker := &fakeBrokerMarketData{accounts: []financial.FinancialAccount{{ID: "coinbase-1", Provider: "coinbase", BaseCurrency: "USD", Status: "active"}}}
	handler := &authHandler{marketFinancial: broker, markets: fakeMarketIntelligence{}}
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/coinbase-1/markets/crypto/BTC/candles", nil)
	request.SetPathValue("id", "coinbase-1")
	request.SetPathValue("symbol", "BTC")
	recorder := httptest.NewRecorder()
	handler.connectedCryptoCandles(recorder, request)
	body := recorder.Body.String()
	for _, expected := range []string{`"granularity_seconds":900`, `"expected_intervals":96`, `"feed":"rest_candles"`, `"chart_semantics":"VENUE_PRICE_MOVEMENT"`, `"live_execution_available":false`} {
		if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" || !strings.Contains(body, expected) {
			t.Fatalf("connected candle boundary missing %s: status=%d body=%s", expected, recorder.Code, body)
		}
	}

	request = httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/coinbase-1/markets/crypto/ETH/candles", nil)
	request.SetPathValue("id", "coinbase-1")
	request.SetPathValue("symbol", "ETH")
	recorder = httptest.NewRecorder()
	handler.connectedCryptoCandles(recorder, request)
	if recorder.Code != stdhttp.StatusNotFound || !strings.Contains(recorder.Body.String(), "CONNECTED_ASSET_NOT_FOUND") {
		t.Fatalf("unconnected asset history was exposed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestConnectedCryptoLiquidityIsOwnerScopedBoundedAndActionFree(t *testing.T) {
	broker := &fakeBrokerMarketData{accounts: []financial.FinancialAccount{{ID: "coinbase-1", Provider: "coinbase", BaseCurrency: "USD", Status: "active"}}}
	handler := &authHandler{marketFinancial: broker, markets: fakeMarketIntelligence{}}
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/coinbase-1/markets/crypto/BTC/liquidity", nil)
	request.SetPathValue("id", "coinbase-1")
	request.SetPathValue("symbol", "BTC")
	recorder := httptest.NewRecorder()
	handler.connectedCryptoLiquidity(recorder, request)
	body := recorder.Body.String()
	for _, expected := range []string{`"depth":10`, `"price":"70186.90"`, `"feed":"advanced_trade_public_product_book"`, `"snapshot_semantics":"SINGLE_VENUE_LIQUIDITY_SNAPSHOT"`, `"order_book_streaming":false`, `"order_actions_available":false`, `"live_execution_available":false`} {
		if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" || !strings.Contains(body, expected) {
			t.Fatalf("connected liquidity boundary missing %s: status=%d body=%s", expected, recorder.Code, body)
		}
	}

	request = httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/coinbase-1/markets/crypto/ETH/liquidity", nil)
	request.SetPathValue("id", "coinbase-1")
	request.SetPathValue("symbol", "ETH")
	recorder = httptest.NewRecorder()
	handler.connectedCryptoLiquidity(recorder, request)
	if recorder.Code != stdhttp.StatusNotFound || !strings.Contains(recorder.Body.String(), "CONNECTED_ASSET_NOT_FOUND") {
		t.Fatalf("unconnected asset book was exposed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestConnectedCryptoTradesAreOwnerScopedBoundedAndInferenceFree(t *testing.T) {
	broker := &fakeBrokerMarketData{accounts: []financial.FinancialAccount{{ID: "coinbase-1", Provider: "coinbase", BaseCurrency: "USD", Status: "active"}}}
	handler := &authHandler{marketFinancial: broker, markets: fakeMarketIntelligence{}}
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/coinbase-1/markets/crypto/BTC/trades", nil)
	request.SetPathValue("id", "coinbase-1")
	request.SetPathValue("symbol", "BTC")
	recorder := httptest.NewRecorder()
	handler.connectedCryptoTrades(recorder, request)
	body := recorder.Body.String()
	for _, expected := range []string{`"limit":25`, `"price":"70187.10"`, `"feed":"advanced_trade_public_market_trades"`, `"snapshot_semantics":"PUBLIC_VENUE_TRADE_TAPE"`, `"trade_streaming":false`, `"order_flow_inference":false`, `"order_actions_available":false`, `"live_execution_available":false`} {
		if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" || !strings.Contains(body, expected) {
			t.Fatalf("connected trade tape missing %s: status=%d body=%s", expected, recorder.Code, body)
		}
	}
}

func TestConnectedCryptoVenueStatsAreOwnerScopedAndReceiptTimed(t *testing.T) {
	broker := &fakeBrokerMarketData{accounts: []financial.FinancialAccount{{ID: "coinbase-1", Provider: "coinbase", BaseCurrency: "USD", Status: "active"}}}
	handler := &authHandler{marketFinancial: broker, markets: fakeMarketIntelligence{}}
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/coinbase-1/markets/crypto/BTC/stats", nil)
	request.SetPathValue("id", "coinbase-1")
	request.SetPathValue("symbol", "BTC")
	recorder := httptest.NewRecorder()
	handler.connectedCryptoVenueStats(recorder, request)
	body := recorder.Body.String()
	for _, expected := range []string{`"open":"72715.34"`, `"volume_30day":"189836.08275489"`, `"feed":"exchange_public_product_stats"`, `"summary_semantics":"ROLLING_SINGLE_VENUE_STATS"`, `"provider_event_time_available":false`, `"timestamp_semantics":"ARBION_RECEIPT_TIME"`, `"performance_claim":false`, `"order_actions_available":false`, `"live_execution_available":false`} {
		if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" || !strings.Contains(body, expected) || strings.Contains(body, "provider_timestamp") {
			t.Fatalf("connected venue stats boundary missing %s: status=%d body=%s", expected, recorder.Code, body)
		}
	}

	request = httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/coinbase-1/markets/crypto/ETH/stats", nil)
	request.SetPathValue("id", "coinbase-1")
	request.SetPathValue("symbol", "ETH")
	recorder = httptest.NewRecorder()
	handler.connectedCryptoVenueStats(recorder, request)
	if recorder.Code != stdhttp.StatusNotFound || !strings.Contains(recorder.Body.String(), "CONNECTED_ASSET_NOT_FOUND") {
		t.Fatalf("unconnected venue stats were exposed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestConnectedTradeFillsAreOwnerScopedNoStoreExecutionEvidence(t *testing.T) {
	broker := &fakeBrokerMarketData{
		accounts: []financial.FinancialAccount{{ID: "coinbase-1", Provider: "coinbase", Status: "active"}},
		fills:    financial.TradeFillPage{Provider: "coinbase", Feed: "advanced_trade_fills", Fills: []financial.TradeFill{{ProductID: "BTC-USD", Side: "BUY", Price: "60123.123456789", Size: "0.00000001", SizeUnit: "BTC"}}, HasMore: true},
	}
	handler := &authHandler{marketFinancial: broker}
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/coinbase-1/activity/fills", nil)
	request.SetPathValue("id", "coinbase-1")
	recorder := httptest.NewRecorder()
	handler.connectedTradeFills(recorder, request)
	body := recorder.Body.String()
	for _, expected := range []string{`"feed":"advanced_trade_fills"`, `"price":"60123.123456789"`, `"history_semantics":"EXTERNAL_EXECUTION_EVIDENCE"`, `"live_execution_available":false`} {
		if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" || !strings.Contains(body, expected) {
			t.Fatalf("trade history boundary missing %s: status=%d body=%s", expected, recorder.Code, body)
		}
	}

	broker.accounts[0].Provider = "schwab"
	recorder = httptest.NewRecorder()
	handler.connectedTradeFills(recorder, request)
	if recorder.Code != stdhttp.StatusBadRequest || !strings.Contains(recorder.Body.String(), "TRADE_HISTORY_UNSUPPORTED") {
		t.Fatalf("non-Coinbase activity entered the connected fill view: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestConnectedOrderHistoryIsOwnerScopedNoStoreAndActionFree(t *testing.T) {
	broker := &fakeBrokerMarketData{
		accounts: []financial.FinancialAccount{{ID: "coinbase-1", Provider: "coinbase", Status: "active"}},
		orders:   financial.OrderHistoryPage{Provider: "coinbase", Feed: "advanced_trade_orders", Orders: []financial.OrderObservation{{ProductID: "BTC-USD", Status: "OPEN", Side: "BUY", CompletionPercentage: "25.000"}}, HasMore: true},
	}
	handler := &authHandler{marketFinancial: broker}
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/coinbase-1/activity/orders", nil)
	request.SetPathValue("id", "coinbase-1")
	recorder := httptest.NewRecorder()
	handler.connectedOrderHistory(recorder, request)
	body := recorder.Body.String()
	for _, expected := range []string{`"feed":"advanced_trade_orders"`, `"completion_percentage":"25.000"`, `"history_semantics":"EXTERNAL_ORDER_STATUS"`, `"order_actions_available":false`, `"live_execution_available":false`} {
		if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" || !strings.Contains(body, expected) {
			t.Fatalf("order monitor boundary missing %s: status=%d body=%s", expected, recorder.Code, body)
		}
	}

	broker.accounts[0].Provider = "schwab"
	recorder = httptest.NewRecorder()
	handler.connectedOrderHistory(recorder, request)
	if recorder.Code != stdhttp.StatusBadRequest || !strings.Contains(recorder.Body.String(), "ORDER_HISTORY_UNSUPPORTED") {
		t.Fatalf("non-Coinbase order history entered the monitor: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestConnectedTradingCostsAreOwnerScopedNoStoreAndOrderActionFree(t *testing.T) {
	broker := &fakeBrokerMarketData{
		accounts: []financial.FinancialAccount{{ID: "coinbase-1", Provider: "coinbase", Status: "active"}},
		costs: financial.TradingCostSummary{
			Provider: "coinbase", Feed: "advanced_trade_transaction_summary", ProductType: "SPOT", PricingTier: "<$10k",
			MakerFeeRate: "0.0020", TakerFeeRate: "0.0030",
			AdvancedTradeVolume: financial.Money{Amount: "1000.123456789", Currency: "USD"},
			AdvancedTradeFees:   financial.Money{Amount: "20.00000001", Currency: "USD"},
			TotalFees:           financial.Money{Amount: "25.00000001", Currency: "USD"},
		},
	}
	handler := &authHandler{marketFinancial: broker}
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/coinbase-1/activity/trading-costs", nil)
	request.SetPathValue("id", "coinbase-1")
	recorder := httptest.NewRecorder()
	handler.connectedTradingCosts(recorder, request)
	body := recorder.Body.String()
	for _, expected := range []string{`"feed":"advanced_trade_transaction_summary"`, `"maker_fee_rate":"0.0020"`, `"advanced_trade_volume":{"amount":"1000.123456789"`, `"summary_semantics":"PROVIDER_FEE_TIER_SNAPSHOT"`, `"order_preview_available":true`, `"order_actions_available":false`, `"live_execution_available":false`} {
		if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" || !strings.Contains(body, expected) {
			t.Fatalf("trading-cost boundary missing %s: status=%d body=%s", expected, recorder.Code, body)
		}
	}

	broker.accounts[0].Provider = "schwab"
	recorder = httptest.NewRecorder()
	handler.connectedTradingCosts(recorder, request)
	if recorder.Code != stdhttp.StatusBadRequest || !strings.Contains(recorder.Body.String(), "TRADING_COSTS_UNSUPPORTED") {
		t.Fatalf("non-Coinbase costs entered the summary: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestMarketObservationRoutesAreNoStoreAndReadOnly(t *testing.T) {
	handler := &authHandler{markets: fakeMarketIntelligence{}}
	for name, testCase := range map[string]struct {
		request *stdhttp.Request
		call    func(stdhttp.ResponseWriter, *stdhttp.Request)
	}{
		"equity":  {httptest.NewRequest(stdhttp.MethodGet, "/api/markets/equities/AAPL/quote", nil), handler.latestEquityQuote},
		"crypto":  {httptest.NewRequest(stdhttp.MethodGet, "/api/markets/crypto?currency=usd&limit=8", nil), handler.topCryptoMarkets},
		"filings": {httptest.NewRequest(stdhttp.MethodGet, "/api/markets/insiders/0000320193?limit=10", nil), handler.recentInsiderFilings},
	} {
		t.Run(name, func(t *testing.T) {
			switch name {
			case "equity":
				testCase.request.SetPathValue("symbol", "AAPL")
			case "filings":
				testCase.request.SetPathValue("cik", "0000320193")
			}
			recorder := httptest.NewRecorder()
			testCase.call(recorder, testCase.request)
			if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("unexpected market response: status=%d cache=%q body=%s", recorder.Code, recorder.Header().Get("Cache-Control"), recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"live_execution_available":false`) {
				t.Fatalf("market boundary missing from response: %s", recorder.Body.String())
			}
		})
	}
}

func TestMarketRoutesFailClosedWithoutConfiguredSource(t *testing.T) {
	handler := &authHandler{markets: fakeMarketIntelligence{err: marketintelligence.ErrNoEligibleSource}}
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/markets/crypto", nil)
	recorder := httptest.NewRecorder()
	handler.topCryptoMarkets(recorder, request)
	if recorder.Code != stdhttp.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "MARKET_SOURCE_UNAVAILABLE") {
		t.Fatalf("unconfigured source did not fail closed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(stdhttp.MethodGet, "/api/markets/crypto?limit=1000", nil)
	recorder = httptest.NewRecorder()
	handler.topCryptoMarkets(recorder, request)
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("unbounded market query was accepted: status=%d", recorder.Code)
	}
}
