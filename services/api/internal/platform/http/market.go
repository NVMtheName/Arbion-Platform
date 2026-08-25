package http

import (
	"context"
	"errors"
	"math/big"
	stdhttp "net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/marketintelligence"
)

type MarketIntelligence interface {
	Sources() []marketintelligence.Source
	SourceHealthHistory(context.Context) (marketintelligence.HealthHistory, error)
	ListWatchlist(context.Context, string) ([]marketintelligence.WatchlistItem, error)
	CreateWatchlistItem(context.Context, string, string) (marketintelligence.WatchlistItem, error)
	DeleteWatchlistItem(context.Context, string, string) error
	LatestEquityQuote(context.Context, string) (marketintelligence.QuoteObservation, bool, error)
	TopCryptoMarkets(context.Context, string, int) ([]marketintelligence.CryptoMarketObservation, bool, error)
	CryptoMarkets(context.Context, string, []string) (marketintelligence.CryptoMarketBatch, bool, error)
	RecentCryptoCandles(context.Context, string, string, int, int) (marketintelligence.CryptoCandleSeries, bool, error)
	CryptoLiquidity(context.Context, string, string, int) (marketintelligence.CryptoLiquiditySnapshot, bool, error)
	RecentCryptoTrades(context.Context, string, string, int) (marketintelligence.CryptoTradeTape, bool, error)
	CryptoVenueStats(context.Context, string, string) (marketintelligence.CryptoVenueStats, bool, error)
	RecentInsiderFilings(context.Context, string, int) ([]marketintelligence.InsiderFilingObservation, bool, error)
}

func (h *authHandler) marketSourceHistory(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.markets == nil {
		writeError(writer, stdhttp.StatusServiceUnavailable, "MARKET_HISTORY_UNAVAILABLE", "Durable market-source history is temporarily unavailable.")
		return
	}
	history, err := h.markets.SourceHealthHistory(request.Context())
	if err != nil {
		writeError(writer, stdhttp.StatusServiceUnavailable, "MARKET_HISTORY_UNAVAILABLE", "Durable market-source history is temporarily unavailable.")
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"buckets":                     history.Buckets,
		"window_started_at":           history.WindowStartedAt,
		"window_ended_at":             history.WindowEndedAt,
		"window_hours":                history.WindowHours,
		"interval_minutes":            history.IntervalMinutes,
		"history_semantics":           "DURABLE_PROVIDER_OUTCOMES_5_MINUTE_STORAGE_HOURLY_VIEW",
		"subject_dimensions_exposed":  false,
		"raw_provider_errors_exposed": false,
		"live_execution_available":    false,
	})
}

type BrokerMarketData interface {
	ListAccounts(context.Context, authorization.Principal) ([]financial.FinancialAccount, error)
	GetAccount(context.Context, authorization.Principal, string) (financial.FinancialAccount, error)
	GetBalances(context.Context, authorization.Principal, string) (financial.Balances, error)
	GetPositions(context.Context, authorization.Principal, string) ([]financial.Position, error)
	GetTradeFills(context.Context, authorization.Principal, string) (financial.TradeFillPage, error)
	GetOrderHistory(context.Context, authorization.Principal, string) (financial.OrderHistoryPage, error)
	GetTradingCostSummary(context.Context, authorization.Principal, string) (financial.TradingCostSummary, error)
	GetQuote(context.Context, authorization.Principal, string, string) (financial.Quote, error)
	GetOptionChain(context.Context, authorization.Principal, string, financial.OptionChainRequest) (financial.OptionChain, error)
}

const (
	maxPortfolioPositions = 250
	maxPricedCryptoAssets = 32

	valuationBasisVenueLastTrade       = "VENUE_LAST_TRADE"
	valuationBasisUSDCUSDRedemption    = "COINBASE_USDC_USD_REDEMPTION"
	pricingBasisLastTrade              = "LAST_TRADE"
	pricingBasisUSDCUSDRedemption      = "USDC_USD_REDEMPTION"
	pricingBasisLastTradeAndRedemption = "LAST_TRADE_AND_USDC_USD_REDEMPTION"
)

type cryptoPortfolioPosition struct {
	Symbol                     string                         `json:"symbol"`
	Quantity                   financial.Decimal              `json:"quantity"`
	AvailableQuantity          *financial.Decimal             `json:"available_quantity,omitempty"`
	UnavailableToTradeQuantity *financial.Decimal             `json:"unavailable_to_trade_quantity,omitempty"`
	UnitPrice                  *financial.Money               `json:"unit_price,omitempty"`
	Bid                        *financial.Money               `json:"bid,omitempty"`
	Ask                        *financial.Money               `json:"ask,omitempty"`
	MarketValue                *financial.Money               `json:"market_value,omitempty"`
	ChangeAmount24H            *financial.Money               `json:"change_amount_24h,omitempty"`
	ChangePercent24H           *financial.Decimal             `json:"change_percent_24h,omitempty"`
	PositionChange24H          *financial.Money               `json:"position_change_24h,omitempty"`
	PricingStatus              string                         `json:"pricing_status"`
	CostBasisStatus            string                         `json:"cost_basis_status"`
	ValuationBasis             string                         `json:"valuation_basis,omitempty"`
	Provenance                 *marketintelligence.Provenance `json:"provenance,omitempty"`
}

type cryptoPortfolioSnapshot struct {
	Account           financial.FinancialAccount `json:"account"`
	Balances          financial.Balances         `json:"balances"`
	PortfolioState    string                     `json:"portfolio_state"`
	BalanceState      string                     `json:"balance_state"`
	HoldingsState     string                     `json:"holdings_state"`
	ObservedValue     *financial.Money           `json:"observed_value,omitempty"`
	DigitalAssetValue *financial.Money           `json:"digital_asset_value,omitempty"`
	Positions         []cryptoPortfolioPosition  `json:"positions"`
	PricedPositions   int                        `json:"priced_positions"`
	TotalPositions    int                        `json:"total_positions"`
	PricingComplete   bool                       `json:"pricing_complete"`
	PricingState      string                     `json:"pricing_state"`
	PricingBasis      string                     `json:"pricing_basis"`
	PricingMessage    string                     `json:"pricing_message"`
	PricingAsOf       *time.Time                 `json:"pricing_as_of,omitempty"`
	MarketDataCached  bool                       `json:"market_data_cached"`
}

func (h *authHandler) listMarketSources(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	sources := h.marketSources
	if h.markets != nil {
		sources = h.markets.Sources()
	}
	sources = append([]marketintelligence.Source(nil), sources...)
	if h.marketFinancial != nil {
		accounts, err := h.marketFinancial.ListAccounts(request.Context(), principal(request))
		if err == nil {
			for _, account := range accounts {
				if account.Provider == "schwab" && strings.EqualFold(account.Status, "active") {
					setMarketSourceAvailable(sources, "schwab_broker_market_data")
					break
				}
			}
		}
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"sources":                  sources,
		"status_generated_at":      time.Now().UTC(),
		"status_semantics":         "PROCESS_LOCAL_TIME_BOUNDED_PROVIDER_VERIFICATION",
		"request_usage_semantics":  "PROCESS_LOCAL_BOUNDED_AGGREGATES",
		"provider_quota_exposed":   false,
		"provider_errors_exposed":  false,
		"live_execution_available": false,
	})
}

func setMarketSourceAvailable(sources []marketintelligence.Source, sourceID string) {
	for index := range sources {
		if sources[index].ID == sourceID {
			sources[index].Enabled = true
			sources[index].Healthy = true
			for statusIndex := range sources[index].CapabilityStatus {
				sources[index].CapabilityStatus[statusIndex].Enabled = true
				sources[index].CapabilityStatus[statusIndex].State = marketintelligence.AwaitingObservation
			}
			return
		}
	}
}

func (h *authHandler) latestEquityQuote(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.markets == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	quote, cached, err := h.markets.LatestEquityQuote(request.Context(), request.PathValue("symbol"))
	if err != nil {
		h.marketUnavailable(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{"quote": quote, "cached": cached, "live_execution_available": false})
}

func (h *authHandler) topCryptoMarkets(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.markets == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	currency := request.URL.Query().Get("currency")
	if currency == "" {
		currency = "usd"
	}
	limit, ok := boundedMarketLimit(request.URL.Query().Get("limit"), 8)
	if !ok {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_MARKET_QUERY", "The market query is invalid.")
		return
	}
	markets, cached, err := h.markets.TopCryptoMarkets(request.Context(), currency, limit)
	if err != nil {
		h.marketUnavailable(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{"markets": markets, "cached": cached, "live_execution_available": false})
}

func (h *authHandler) cryptoPortfolio(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.marketFinancial == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	account, err := h.marketFinancial.GetAccount(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		h.financialError(writer, err)
		return
	}
	if account.Provider != "coinbase" {
		writeError(writer, stdhttp.StatusBadRequest, "PORTFOLIO_PRICING_UNSUPPORTED", "This account does not use the crypto portfolio view.")
		return
	}

	var (
		balances     financial.Balances
		positions    []financial.Position
		balancesErr  error
		positionsErr error
		wait         sync.WaitGroup
	)
	p := principal(request)
	wait.Add(2)
	go func() {
		defer wait.Done()
		balances, balancesErr = h.marketFinancial.GetBalances(request.Context(), p, account.ID)
	}()
	go func() {
		defer wait.Done()
		positions, positionsErr = h.marketFinancial.GetPositions(request.Context(), p, account.ID)
	}()
	wait.Wait()
	if (balancesErr != nil && positionsErr != nil) || providerCredentialError(balancesErr) || providerCredentialError(positionsErr) {
		if balancesErr != nil {
			h.financialError(writer, balancesErr)
		} else {
			h.financialError(writer, positionsErr)
		}
		return
	}
	if len(positions) > maxPortfolioPositions {
		writeError(writer, stdhttp.StatusUnprocessableEntity, "PORTFOLIO_TOO_LARGE", "The portfolio contains too many positions for this view.")
		return
	}

	symbols := make([]string, 0, len(positions))
	seen := make(map[string]struct{}, len(positions))
	for _, position := range positions {
		if !strings.EqualFold(position.InstrumentType, "CRYPTO") {
			continue
		}
		symbol := strings.ToUpper(strings.TrimSpace(position.Symbol))
		if isUSDCUSDRedemptionReference(symbol, account.BaseCurrency) {
			continue
		}
		if _, exists := seen[symbol]; exists {
			continue
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	requested := symbols
	if len(requested) > maxPricedCryptoAssets {
		requested = requested[:maxPricedCryptoAssets]
	}

	batch := marketintelligence.CryptoMarketBatch{}
	cached := false
	portfolioState := "READY"
	balanceState := "READY"
	holdingsState := "READY"
	pricingMessage := "Last-trade observations from Coinbase Exchange; values are informational and non-executable."
	pricingState := "READY"
	if balancesErr != nil {
		portfolioState = "PARTIAL"
		balanceState = "UNAVAILABLE"
	}
	if positionsErr != nil {
		portfolioState = "PARTIAL"
		holdingsState = "UNAVAILABLE"
		pricingState = "UNAVAILABLE"
		pricingMessage = "Coinbase balances are available, but its holdings feed is temporarily unavailable. The connection remains active and no holdings were substituted."
	}
	if holdingsState == "READY" && len(requested) > 0 {
		if h.markets == nil {
			pricingState = "UNAVAILABLE"
			pricingMessage = "Portfolio holdings are available, but the approved market source is not configured. No values were substituted."
		} else {
			batch, cached, err = h.markets.CryptoMarkets(request.Context(), account.BaseCurrency, requested)
			if err != nil {
				pricingState = "UNAVAILABLE"
				pricingMessage = "Portfolio holdings are available, but Coinbase Exchange pricing is temporarily unavailable. No values were substituted."
				batch = marketintelligence.CryptoMarketBatch{}
			}
		}
	}

	observations := make(map[string]marketintelligence.CryptoMarketObservation, len(batch.Markets))
	for _, observation := range batch.Markets {
		observations[strings.ToUpper(observation.Symbol)] = observation
	}
	venueStats := portfolioVenueStats(request.Context(), h.markets, requested)
	view := make([]cryptoPortfolioPosition, 0, len(positions))
	assetValue := new(big.Rat)
	priced := 0
	venueValued := false
	redemptionValued := false
	var pricingAsOf *time.Time
	for _, position := range positions {
		symbol := strings.ToUpper(strings.TrimSpace(position.Symbol))
		item := cryptoPortfolioPosition{
			Symbol:                     symbol,
			Quantity:                   position.Quantity,
			AvailableQuantity:          position.AvailableQuantity,
			UnavailableToTradeQuantity: position.UnavailableToTradeQuantity,
			PricingStatus:              "UNAVAILABLE",
			CostBasisStatus:            "UNAVAILABLE_FROM_PROVIDER",
		}
		if isUSDCUSDRedemptionReference(symbol, account.BaseCurrency) {
			value, rational, valueErr := observedMarketValue(position.Quantity, "1", account.BaseCurrency)
			if valueErr == nil {
				item.UnitPrice = &financial.Money{Amount: "1", Currency: strings.ToUpper(account.BaseCurrency)}
				item.MarketValue = value
				item.PricingStatus = "PRICED"
				item.ValuationBasis = valuationBasisUSDCUSDRedemption
				zero := financial.Decimal("0")
				item.ChangeAmount24H = &financial.Money{Amount: zero, Currency: strings.ToUpper(account.BaseCurrency)}
				item.ChangePercent24H = &zero
				item.PositionChange24H = &financial.Money{Amount: zero, Currency: strings.ToUpper(account.BaseCurrency)}
				assetValue.Add(assetValue, rational)
				priced++
				redemptionValued = true
			}
			view = append(view, item)
			continue
		}
		observation, found := observations[symbol]
		if found {
			value, rational, valueErr := observedMarketValue(position.Quantity, observation.CurrentPrice, account.BaseCurrency)
			if valueErr == nil {
				item.UnitPrice = marketMoney(observation.CurrentPrice, observation.Currency)
				item.Bid = marketMoneyPointer(observation.Bid, observation.Currency)
				item.Ask = marketMoneyPointer(observation.Ask, observation.Currency)
				item.MarketValue = value
				item.PricingStatus = "PRICED"
				item.ValuationBasis = valuationBasisVenueLastTrade
				provenance := observation.Provenance
				item.Provenance = &provenance
				if stats, available := venueStats[symbol]; available {
					item.ChangeAmount24H, item.ChangePercent24H, item.PositionChange24H = cryptoPositionChange(observation.CurrentPrice, stats.Open, position.Quantity, account.BaseCurrency)
				}
				assetValue.Add(assetValue, rational)
				priced++
				venueValued = true
				providerTime := observation.Provenance.ProviderTimestamp.UTC()
				if pricingAsOf == nil || providerTime.Before(*pricingAsOf) {
					pricingAsOf = &providerTime
				}
			}
		}
		view = append(view, item)
	}
	sort.SliceStable(view, func(left, right int) bool {
		leftValue := moneyRational(view[left].MarketValue)
		rightValue := moneyRational(view[right].MarketValue)
		if comparison := leftValue.Cmp(rightValue); comparison != 0 {
			return comparison > 0
		}
		return view[left].Symbol < view[right].Symbol
	})

	complete := holdingsState == "READY" && priced == len(positions)
	if !complete && pricingState == "READY" {
		pricingState = "PARTIAL"
		pricingMessage = "Some assets do not have an approved Coinbase Exchange USD ticker. They remain visible without an estimated value."
	}
	if len(positions) > maxPricedCryptoAssets && pricingState != "UNAVAILABLE" {
		pricingState = "PARTIAL"
		pricingMessage = "Pricing is limited to 32 assets per refresh. Remaining holdings stay visible without an estimated value."
	}
	pricingBasis := pricingBasisLastTrade
	if redemptionValued {
		pricingBasis = pricingBasisUSDCUSDRedemption
		if venueValued {
			pricingBasis = pricingBasisLastTradeAndRedemption
		}
		if pricingState == "READY" {
			pricingMessage = "Coinbase Exchange last trades are combined with Coinbase's 1:1 USDC-to-USD redemption reference; values are informational and non-executable."
		} else {
			pricingMessage = "Coinbase's 1:1 USDC-to-USD redemption reference is included. " + pricingMessage
		}
	}
	if balanceState == "UNAVAILABLE" {
		pricingMessage = "Coinbase holdings are available, but its balance feed is temporarily unavailable; aggregate portfolio value omits cash. " + pricingMessage
	}

	var digitalAssetValue *financial.Money
	if holdingsState == "READY" {
		digitalAssetValue = &financial.Money{Amount: financial.Decimal(formatObservedMoney(assetValue)), Currency: account.BaseCurrency}
	}
	var observedValueMoney *financial.Money
	if holdingsState == "READY" && balanceState == "READY" {
		observedValue := new(big.Rat).Set(assetValue)
		if balances.Cash != nil && strings.EqualFold(balances.Cash.Currency, account.BaseCurrency) {
			if cash, ok := new(big.Rat).SetString(string(balances.Cash.Amount)); ok && cash.Sign() >= 0 {
				observedValue.Add(observedValue, cash)
			}
		}
		observedValueMoney = &financial.Money{Amount: financial.Decimal(formatObservedMoney(observedValue)), Currency: account.BaseCurrency}
	}
	snapshot := cryptoPortfolioSnapshot{
		Account: account, Balances: balances,
		PortfolioState:    portfolioState,
		BalanceState:      balanceState,
		HoldingsState:     holdingsState,
		ObservedValue:     observedValueMoney,
		DigitalAssetValue: digitalAssetValue, Positions: view,
		PricedPositions: priced, TotalPositions: len(positions), PricingComplete: complete,
		PricingState: pricingState, PricingBasis: pricingBasis, PricingMessage: pricingMessage,
		PricingAsOf: pricingAsOf, MarketDataCached: cached,
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{"portfolio": snapshot, "live_execution_available": false})
}

func portfolioVenueStats(ctx context.Context, markets MarketIntelligence, symbols []string) map[string]marketintelligence.CryptoVenueStats {
	result := make(map[string]marketintelligence.CryptoVenueStats, len(symbols))
	if markets == nil || len(symbols) == 0 {
		return result
	}
	type observation struct {
		symbol string
		stats  marketintelligence.CryptoVenueStats
		valid  bool
	}
	observations := make([]observation, len(symbols))
	semaphore := make(chan struct{}, 4)
	var wait sync.WaitGroup
	for index, symbol := range symbols {
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			stats, _, err := markets.CryptoVenueStats(ctx, symbol, "USD")
			if err == nil && strings.EqualFold(stats.Symbol, symbol) && strings.EqualFold(stats.Currency, "USD") {
				observations[index] = observation{symbol: symbol, stats: stats, valid: true}
			}
		}()
	}
	wait.Wait()
	for _, item := range observations {
		if item.valid {
			result[item.symbol] = item.stats
		}
	}
	return result
}

func cryptoPositionChange(current, open marketintelligence.Decimal, quantity financial.Decimal, currency string) (*financial.Money, *financial.Decimal, *financial.Money) {
	currentValue, currentOK := new(big.Rat).SetString(string(current))
	openValue, openOK := new(big.Rat).SetString(string(open))
	quantityValue, quantityOK := new(big.Rat).SetString(string(quantity))
	if !currentOK || !openOK || !quantityOK || currentValue.Sign() < 0 || openValue.Sign() <= 0 || quantityValue.Sign() < 0 {
		return nil, nil, nil
	}
	change := new(big.Rat).Sub(currentValue, openValue)
	percent := new(big.Rat).Mul(new(big.Rat).Quo(new(big.Rat).Set(change), openValue), big.NewRat(100, 1))
	positionChange := new(big.Rat).Mul(new(big.Rat).Set(change), quantityValue)
	normalizedCurrency := strings.ToUpper(currency)
	changeMoney := &financial.Money{Amount: financial.Decimal(formatObservedMoney(change)), Currency: normalizedCurrency}
	changePercent := financial.Decimal(formatObservedMoney(percent))
	positionMoney := &financial.Money{Amount: financial.Decimal(formatObservedMoney(positionChange)), Currency: normalizedCurrency}
	return changeMoney, &changePercent, positionMoney
}

func providerCredentialError(err error) bool {
	if err == nil {
		return false
	}
	var providerError *financial.ProviderError
	return errors.As(err, &providerError) && (providerError.Code == financial.AuthorizationFailed || providerError.Code == financial.PermissionDenied || providerError.Code == financial.InvalidCredentialFormat)
}

func isUSDCUSDRedemptionReference(symbol, currency string) bool {
	return strings.EqualFold(strings.TrimSpace(symbol), "USDC") && strings.EqualFold(strings.TrimSpace(currency), "USD")
}

func (h *authHandler) connectedCryptoCandles(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.marketFinancial == nil || h.markets == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	account, err := h.marketFinancial.GetAccount(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		h.financialError(writer, err)
		return
	}
	if account.Provider != "coinbase" {
		writeError(writer, stdhttp.StatusBadRequest, "CRYPTO_HISTORY_UNSUPPORTED", "This account does not use the connected crypto history view.")
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(request.PathValue("symbol")))
	if !validConnectedCryptoSymbol(symbol) {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_MARKET_QUERY", "The market query is invalid.")
		return
	}
	positions, err := h.marketFinancial.GetPositions(request.Context(), principal(request), account.ID)
	if err != nil {
		h.financialError(writer, err)
		return
	}
	connected := false
	for _, position := range positions {
		if strings.EqualFold(position.InstrumentType, "CRYPTO") && strings.EqualFold(strings.TrimSpace(position.Symbol), symbol) {
			connected = true
			break
		}
	}
	if !connected {
		writeError(writer, stdhttp.StatusNotFound, "CONNECTED_ASSET_NOT_FOUND", "This asset is not present in the connected account.")
		return
	}
	series, cached, err := h.markets.RecentCryptoCandles(request.Context(), symbol, account.BaseCurrency, 900, 96)
	if err != nil {
		h.marketUnavailable(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"history": series, "cached": cached,
		"chart_semantics": "VENUE_PRICE_MOVEMENT", "live_execution_available": false,
	})
}

func (h *authHandler) connectedCryptoLiquidity(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.marketFinancial == nil || h.markets == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	account, err := h.marketFinancial.GetAccount(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		h.financialError(writer, err)
		return
	}
	if account.Provider != "coinbase" {
		writeError(writer, stdhttp.StatusBadRequest, "CRYPTO_LIQUIDITY_UNSUPPORTED", "This account does not use the connected crypto liquidity view.")
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(request.PathValue("symbol")))
	if !validConnectedCryptoSymbol(symbol) {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_MARKET_QUERY", "The market query is invalid.")
		return
	}
	positions, err := h.marketFinancial.GetPositions(request.Context(), principal(request), account.ID)
	if err != nil {
		h.financialError(writer, err)
		return
	}
	connected := false
	for _, position := range positions {
		if strings.EqualFold(position.InstrumentType, "CRYPTO") && strings.EqualFold(strings.TrimSpace(position.Symbol), symbol) {
			connected = true
			break
		}
	}
	if !connected {
		writeError(writer, stdhttp.StatusNotFound, "CONNECTED_ASSET_NOT_FOUND", "This asset is not present in the connected account.")
		return
	}
	snapshot, cached, err := h.markets.CryptoLiquidity(request.Context(), symbol, account.BaseCurrency, 10)
	if err != nil {
		h.marketUnavailable(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"liquidity": snapshot, "cached": cached,
		"snapshot_semantics": "SINGLE_VENUE_LIQUIDITY_SNAPSHOT", "order_book_streaming": false,
		"order_actions_available": false, "live_execution_available": false,
	})
}

func (h *authHandler) connectedCryptoTrades(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.marketFinancial == nil || h.markets == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	account, err := h.marketFinancial.GetAccount(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		h.financialError(writer, err)
		return
	}
	if account.Provider != "coinbase" {
		writeError(writer, stdhttp.StatusBadRequest, "CRYPTO_TRADES_UNSUPPORTED", "This account does not use the connected public trade-tape view.")
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(request.PathValue("symbol")))
	if !validConnectedCryptoSymbol(symbol) {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_MARKET_QUERY", "The market query is invalid.")
		return
	}
	positions, err := h.marketFinancial.GetPositions(request.Context(), principal(request), account.ID)
	if err != nil {
		h.financialError(writer, err)
		return
	}
	connected := false
	for _, position := range positions {
		if strings.EqualFold(position.InstrumentType, "CRYPTO") && strings.EqualFold(strings.TrimSpace(position.Symbol), symbol) {
			connected = true
			break
		}
	}
	if !connected {
		writeError(writer, stdhttp.StatusNotFound, "CONNECTED_ASSET_NOT_FOUND", "This asset is not present in the connected account.")
		return
	}
	tape, cached, err := h.markets.RecentCryptoTrades(request.Context(), symbol, account.BaseCurrency, 25)
	if err != nil {
		h.marketUnavailable(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"market_trades": tape, "cached": cached, "snapshot_semantics": "PUBLIC_VENUE_TRADE_TAPE",
		"trade_streaming": false, "order_flow_inference": false, "order_actions_available": false, "live_execution_available": false,
	})
}

func (h *authHandler) connectedCryptoVenueStats(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.marketFinancial == nil || h.markets == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	account, err := h.marketFinancial.GetAccount(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		h.financialError(writer, err)
		return
	}
	if account.Provider != "coinbase" {
		writeError(writer, stdhttp.StatusBadRequest, "CRYPTO_VENUE_STATS_UNSUPPORTED", "This account does not use the connected public venue-statistics view.")
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(request.PathValue("symbol")))
	if !validConnectedCryptoSymbol(symbol) {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_MARKET_QUERY", "The market query is invalid.")
		return
	}
	positions, err := h.marketFinancial.GetPositions(request.Context(), principal(request), account.ID)
	if err != nil {
		h.financialError(writer, err)
		return
	}
	connected := false
	for _, position := range positions {
		if strings.EqualFold(position.InstrumentType, "CRYPTO") && strings.EqualFold(strings.TrimSpace(position.Symbol), symbol) {
			connected = true
			break
		}
	}
	if !connected {
		writeError(writer, stdhttp.StatusNotFound, "CONNECTED_ASSET_NOT_FOUND", "This asset is not present in the connected account.")
		return
	}
	stats, cached, err := h.markets.CryptoVenueStats(request.Context(), symbol, account.BaseCurrency)
	if err != nil {
		h.marketUnavailable(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"venue_stats": stats, "cached": cached, "summary_semantics": "ROLLING_SINGLE_VENUE_STATS",
		"provider_event_time_available": false, "timestamp_semantics": "ARBION_RECEIPT_TIME",
		"performance_claim": false, "order_actions_available": false, "live_execution_available": false,
	})
}

func (h *authHandler) connectedTradeFills(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.marketFinancial == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	account, err := h.marketFinancial.GetAccount(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		h.financialError(writer, err)
		return
	}
	if account.Provider != "coinbase" {
		writeError(writer, stdhttp.StatusBadRequest, "TRADE_HISTORY_UNSUPPORTED", "This account does not use the connected execution history view.")
		return
	}
	activity, err := h.marketFinancial.GetTradeFills(request.Context(), principal(request), account.ID)
	if err != nil {
		h.financialError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"activity": activity, "history_semantics": "EXTERNAL_EXECUTION_EVIDENCE", "live_execution_available": false,
	})
}

func (h *authHandler) connectedOrderHistory(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.marketFinancial == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	account, err := h.marketFinancial.GetAccount(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		h.financialError(writer, err)
		return
	}
	if account.Provider != "coinbase" {
		writeError(writer, stdhttp.StatusBadRequest, "ORDER_HISTORY_UNSUPPORTED", "This account does not use the connected order monitor.")
		return
	}
	history, err := h.marketFinancial.GetOrderHistory(request.Context(), principal(request), account.ID)
	if err != nil {
		h.financialError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"orders": history, "history_semantics": "EXTERNAL_ORDER_STATUS", "order_actions_available": false, "live_execution_available": false,
	})
}

func (h *authHandler) connectedTradingCosts(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.marketFinancial == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	account, err := h.marketFinancial.GetAccount(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		h.financialError(writer, err)
		return
	}
	if account.Provider != "coinbase" {
		writeError(writer, stdhttp.StatusBadRequest, "TRADING_COSTS_UNSUPPORTED", "This account does not use the connected trading-cost view.")
		return
	}
	summary, err := h.marketFinancial.GetTradingCostSummary(request.Context(), principal(request), account.ID)
	if err != nil {
		h.financialError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"trading_costs": summary, "summary_semantics": "PROVIDER_FEE_TIER_SNAPSHOT",
		"order_preview_available": true, "order_actions_available": false, "live_execution_available": false,
	})
}

func validConnectedCryptoSymbol(symbol string) bool {
	if len(symbol) == 0 || len(symbol) > 12 {
		return false
	}
	for _, character := range symbol {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func marketMoney(value marketintelligence.Decimal, currency string) *financial.Money {
	return &financial.Money{Amount: financial.Decimal(value), Currency: strings.ToUpper(currency)}
}

func marketMoneyPointer(value *marketintelligence.Decimal, currency string) *financial.Money {
	if value == nil {
		return nil
	}
	return marketMoney(*value, currency)
}

func observedMarketValue(quantity financial.Decimal, price marketintelligence.Decimal, currency string) (*financial.Money, *big.Rat, error) {
	quantityValue, quantityOK := new(big.Rat).SetString(string(quantity))
	priceValue, priceOK := new(big.Rat).SetString(string(price))
	if !quantityOK || !priceOK || quantityValue.Sign() < 0 || priceValue.Sign() < 0 {
		return nil, nil, marketintelligence.ErrInvalidObservation
	}
	value := new(big.Rat).Mul(quantityValue, priceValue)
	return &financial.Money{Amount: financial.Decimal(formatObservedMoney(value)), Currency: strings.ToUpper(currency)}, value, nil
}

func formatObservedMoney(value *big.Rat) string {
	formatted := value.FloatString(8)
	formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	if formatted == "" || formatted == "-0" {
		return "0"
	}
	return formatted
}

func moneyRational(value *financial.Money) *big.Rat {
	if value == nil {
		return new(big.Rat)
	}
	rational, ok := new(big.Rat).SetString(string(value.Amount))
	if !ok {
		return new(big.Rat)
	}
	return rational
}

func (h *authHandler) recentInsiderFilings(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.markets == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	limit, ok := boundedMarketLimit(request.URL.Query().Get("limit"), 10)
	if !ok {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_MARKET_QUERY", "The market query is invalid.")
		return
	}
	filings, cached, err := h.markets.RecentInsiderFilings(request.Context(), request.PathValue("cik"), limit)
	if err != nil {
		h.marketUnavailable(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{"filings": filings, "cached": cached, "live_execution_available": false})
}

func (h *authHandler) brokerEquityQuote(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.marketFinancial == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	account, err := h.marketFinancial.GetAccount(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		h.financialError(writer, err)
		return
	}
	quote, err := h.marketFinancial.GetQuote(request.Context(), principal(request), account.ID, request.PathValue("symbol"))
	if err != nil {
		h.financialError(writer, err)
		return
	}
	observation, err := marketintelligence.NormalizeBrokerQuote(account.Provider, account.BaseCurrency, quote, time.Now().UTC(), brokerFreshnessPolicy())
	if err != nil {
		h.marketUnavailable(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{"quote": observation, "live_execution_available": false})
}

func (h *authHandler) brokerOptionChain(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.marketFinancial == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	query, ok := brokerOptionQuery(request, time.Now().UTC())
	if !ok {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_MARKET_QUERY", "The option-chain query is invalid.")
		return
	}
	account, err := h.marketFinancial.GetAccount(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		h.financialError(writer, err)
		return
	}
	chain, err := h.marketFinancial.GetOptionChain(request.Context(), principal(request), account.ID, query)
	if err != nil {
		h.financialError(writer, err)
		return
	}
	observation, err := marketintelligence.NormalizeBrokerOptionChain(account.Provider, chain, time.Now().UTC(), brokerFreshnessPolicy())
	if err != nil {
		h.marketUnavailable(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{"chain": observation, "live_execution_available": false})
}

func brokerOptionQuery(request *stdhttp.Request, now time.Time) (financial.OptionChainRequest, bool) {
	query := request.URL.Query()
	symbol := strings.ToUpper(strings.TrimSpace(query.Get("symbol")))
	contractType := strings.ToUpper(strings.TrimSpace(query.Get("contract_type")))
	if contractType == "" {
		contractType = "PUT"
	}
	strikeCount, ok := boundedMarketLimit(query.Get("strike_count"), 12)
	if !ok || strikeCount > 25 || symbol == "" || (contractType != "PUT" && contractType != "CALL") {
		return financial.OptionChainRequest{}, false
	}
	fromDate, toDate := startOfUTCDay(now), startOfUTCDay(now).AddDate(0, 0, 60)
	if value := strings.TrimSpace(query.Get("from_date")); value != "" {
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return financial.OptionChainRequest{}, false
		}
		fromDate = parsed
	}
	if value := strings.TrimSpace(query.Get("to_date")); value != "" {
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return financial.OptionChainRequest{}, false
		}
		toDate = parsed
	}
	if toDate.Before(fromDate) || toDate.Sub(fromDate) > 90*24*time.Hour || fromDate.Before(startOfUTCDay(now).AddDate(0, 0, -1)) {
		return financial.OptionChainRequest{}, false
	}
	return financial.OptionChainRequest{Symbol: symbol, ContractType: contractType, StrikeCount: strikeCount, FromDate: fromDate, ToDate: toDate}, true
}

func startOfUTCDay(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func brokerFreshnessPolicy() marketintelligence.FreshnessPolicy {
	return marketintelligence.FreshnessPolicy{MaxAge: 120 * time.Hour, MaxFutureSkew: 2 * time.Minute}
}

func (h *authHandler) marketUnavailable(writer stdhttp.ResponseWriter, err error) {
	if errors.Is(err, marketintelligence.ErrInstrumentUnavailable) {
		writeError(writer, stdhttp.StatusNotFound, "MARKET_INSTRUMENT_UNAVAILABLE", "No approved market history is available for this asset.")
		return
	}
	if errors.Is(err, marketintelligence.ErrInvalidObservation) {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_MARKET_QUERY", "The market query is invalid.")
		return
	}
	if errors.Is(err, marketintelligence.ErrNoEligibleSource) {
		writeError(writer, stdhttp.StatusServiceUnavailable, "MARKET_SOURCE_UNAVAILABLE", "The requested market source is not configured.")
		return
	}
	writeError(writer, stdhttp.StatusBadGateway, "MARKET_DATA_UNAVAILABLE", "The market provider is temporarily unavailable.")
}

func boundedMarketLimit(value string, fallback int) (int, bool) {
	if value == "" {
		return fallback, true
	}
	limit, err := strconv.Atoi(value)
	return limit, err == nil && limit >= 1 && limit <= 100
}
