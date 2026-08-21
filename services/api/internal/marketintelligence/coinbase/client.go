// Package coinbase implements keyless, read-only Coinbase Exchange venue snapshots.
package coinbase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arbion/platform/services/api/internal/marketintelligence"
)

const (
	defaultBaseURL          = "https://api.exchange.coinbase.com"
	defaultAdvancedTradeURL = "https://api.coinbase.com"
	maxResponseSize         = 64 << 10
)

var (
	ErrInvalidConfiguration = errors.New("invalid Coinbase configuration")
	ErrRateLimited          = errors.New("Coinbase rate limited")
	ErrUnavailable          = errors.New("Coinbase unavailable")
	ErrProductUnavailable   = errors.New("Coinbase USD product unavailable")
	ErrInvalidResponse      = errors.New("invalid Coinbase response")
	productPattern          = regexp.MustCompile(`^[A-Z0-9]{2,12}-USD$`)
	unsignedDecimalPattern  = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)
)

type Product struct {
	ID   string
	Name string
}

type Config struct {
	BaseURL              string
	AdvancedTradeBaseURL string
	Products             []Product
	Timeout              time.Duration
	MaxAge               time.Duration
	MaxFutureSkew        time.Duration
}

type Client struct {
	baseURL              *url.URL
	advancedTradeBaseURL *url.URL
	products             []Product
	freshness            marketintelligence.FreshnessPolicy
	http                 *http.Client
}

type tickerResponse struct {
	Price  string    `json:"price"`
	Bid    string    `json:"bid"`
	Ask    string    `json:"ask"`
	Volume string    `json:"volume"`
	Time   time.Time `json:"time"`
}

type bookLevelResponse struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

type productBookResponse struct {
	Pricebook struct {
		ProductID string              `json:"product_id"`
		Bids      []bookLevelResponse `json:"bids"`
		Asks      []bookLevelResponse `json:"asks"`
		Time      time.Time           `json:"time"`
	} `json:"pricebook"`
	Last           string `json:"last"`
	MidMarket      string `json:"mid_market"`
	SpreadBPS      string `json:"spread_bps"`
	SpreadAbsolute string `json:"spread_absolute"`
}

type marketTradesResponse struct {
	Trades []struct {
		ProductID string    `json:"product_id"`
		Price     string    `json:"price"`
		Size      string    `json:"size"`
		Time      time.Time `json:"time"`
		Side      string    `json:"side"`
		Exchange  string    `json:"exchange"`
	} `json:"trades"`
	BestBid string `json:"best_bid"`
	BestAsk string `json:"best_ask"`
}

func DefaultProducts() []Product {
	return []Product{
		{ID: "BTC-USD", Name: "Bitcoin"},
		{ID: "ETH-USD", Name: "Ethereum"},
		{ID: "SOL-USD", Name: "Solana"},
		{ID: "XRP-USD", Name: "XRP"},
		{ID: "DOGE-USD", Name: "Dogecoin"},
		{ID: "ADA-USD", Name: "Cardano"},
		{ID: "AVAX-USD", Name: "Avalanche"},
		{ID: "LINK-USD", Name: "Chainlink"},
	}
}

func New(config Config, httpClient *http.Client) (*Client, error) {
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	config.AdvancedTradeBaseURL = strings.TrimRight(strings.TrimSpace(config.AdvancedTradeBaseURL), "/")
	if config.AdvancedTradeBaseURL == "" {
		config.AdvancedTradeBaseURL = defaultAdvancedTradeURL
	}
	if len(config.Products) == 0 {
		config.Products = DefaultProducts()
	}
	if httpClient == nil || config.Timeout <= 0 || config.MaxAge <= 0 || config.MaxFutureSkew < 0 || len(config.Products) > 32 {
		return nil, ErrInvalidConfiguration
	}
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" || !approvedBaseURL(baseURL, defaultBaseURL) {
		return nil, ErrInvalidConfiguration
	}
	advancedTradeBaseURL, err := url.Parse(config.AdvancedTradeBaseURL)
	if err != nil || advancedTradeBaseURL.User != nil || advancedTradeBaseURL.RawQuery != "" || advancedTradeBaseURL.Fragment != "" || !approvedBaseURL(advancedTradeBaseURL, defaultAdvancedTradeURL) {
		return nil, ErrInvalidConfiguration
	}
	products := make([]Product, len(config.Products))
	seen := make(map[string]struct{}, len(config.Products))
	for index, product := range config.Products {
		product.ID = strings.ToUpper(strings.TrimSpace(product.ID))
		product.Name = strings.TrimSpace(product.Name)
		if !productPattern.MatchString(product.ID) || product.Name == "" || len(product.Name) > 128 {
			return nil, ErrInvalidConfiguration
		}
		if _, exists := seen[product.ID]; exists {
			return nil, ErrInvalidConfiguration
		}
		seen[product.ID] = struct{}{}
		products[index] = product
	}
	configuredHTTP := *httpClient
	configuredHTTP.Timeout = config.Timeout
	return &Client{
		baseURL:              baseURL,
		advancedTradeBaseURL: advancedTradeBaseURL,
		products:             products,
		freshness: marketintelligence.FreshnessPolicy{
			MaxAge: config.MaxAge, MaxFutureSkew: config.MaxFutureSkew,
		},
		http: &configuredHTTP,
	}, nil
}

func approvedBaseURL(baseURL *url.URL, productionURL string) bool {
	if baseURL.String() == productionURL {
		return true
	}
	if baseURL.Scheme != "http" || baseURL.Path != "" {
		return false
	}
	host := baseURL.Hostname()
	return host == "localhost" || net.ParseIP(host).IsLoopback()
}

// CryptoLiquidity returns a fixed-depth, credential-free Coinbase Advanced
// Trade public book. It cannot preview or submit an order.
func (client *Client) CryptoLiquidity(ctx context.Context, symbol, currency string, depth int) (marketintelligence.CryptoLiquiditySnapshot, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if !validAssetSymbol(symbol) || !strings.EqualFold(strings.TrimSpace(currency), "usd") || depth != 10 {
		return marketintelligence.CryptoLiquiditySnapshot{}, marketintelligence.ErrInvalidObservation
	}
	productID := symbol + "-USD"
	endpoint := *client.advancedTradeBaseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/api/v3/brokerage/market/product_book"
	query := endpoint.Query()
	query.Set("product_id", productID)
	query.Set("limit", strconv.Itoa(depth))
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return marketintelligence.CryptoLiquiditySnapshot{}, ErrUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-cache")
	response, err := client.http.Do(request)
	if err != nil {
		return marketintelligence.CryptoLiquiditySnapshot{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		if response.StatusCode == http.StatusTooManyRequests {
			return marketintelligence.CryptoLiquiditySnapshot{}, ErrRateLimited
		}
		if response.StatusCode == http.StatusNotFound {
			return marketintelligence.CryptoLiquiditySnapshot{}, marketintelligence.ErrInstrumentUnavailable
		}
		return marketintelligence.CryptoLiquiditySnapshot{}, ErrUnavailable
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil || len(payload) > maxResponseSize {
		return marketintelligence.CryptoLiquiditySnapshot{}, ErrInvalidResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var raw productBookResponse
	if err = decoder.Decode(&raw); err != nil || len(raw.Pricebook.Bids) == 0 || len(raw.Pricebook.Bids) > depth || len(raw.Pricebook.Asks) == 0 || len(raw.Pricebook.Asks) > depth {
		return marketintelligence.CryptoLiquiditySnapshot{}, ErrInvalidResponse
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return marketintelligence.CryptoLiquiditySnapshot{}, ErrInvalidResponse
	}
	if strings.ToUpper(strings.TrimSpace(raw.Pricebook.ProductID)) != productID || raw.Pricebook.Time.IsZero() {
		return marketintelligence.CryptoLiquiditySnapshot{}, ErrInvalidResponse
	}
	bids, ok := normalizeBookLevels(raw.Pricebook.Bids, true)
	if !ok {
		return marketintelligence.CryptoLiquiditySnapshot{}, ErrInvalidResponse
	}
	asks, ok := normalizeBookLevels(raw.Pricebook.Asks, false)
	if !ok {
		return marketintelligence.CryptoLiquiditySnapshot{}, ErrInvalidResponse
	}
	last, lastOK := decimal(raw.Last)
	midMarket, midOK := decimal(raw.MidMarket)
	spreadBPS, bpsOK := decimal(raw.SpreadBPS)
	spreadAbsolute, spreadOK := decimal(raw.SpreadAbsolute)
	if !lastOK || !midOK || !bpsOK || !spreadOK {
		return marketintelligence.CryptoLiquiditySnapshot{}, ErrInvalidResponse
	}
	now := time.Now().UTC()
	snapshot := marketintelligence.CryptoLiquiditySnapshot{
		Symbol: symbol, Currency: "USD", ProductID: productID, Depth: depth,
		Bids: bids, Asks: asks, Last: last, MidMarket: midMarket, SpreadBPS: spreadBPS, SpreadAbsolute: spreadAbsolute,
		Provenance: marketintelligence.Provenance{
			Provider: "coinbase", ProviderRequestID: requestID(response), Role: marketintelligence.MarketObservation,
			Feed: "advanced_trade_public_product_book", Quality: marketintelligence.RealTimeSingleVenue, Venue: "coinbase_advanced_trade",
			ProviderTimestamp: raw.Pricebook.Time.UTC(), ReceivedAt: now,
		},
	}
	liquidityFreshness := client.freshness
	if liquidityFreshness.MaxAge > 30*time.Second {
		liquidityFreshness.MaxAge = 30 * time.Second
	}
	if err := marketintelligence.ValidateCryptoLiquidity(snapshot, now, liquidityFreshness); err != nil {
		return marketintelligence.CryptoLiquiditySnapshot{}, err
	}
	return snapshot, nil
}

func normalizeBookLevels(raw []bookLevelResponse, descending bool) ([]marketintelligence.CryptoBookLevel, bool) {
	levels := make([]marketintelligence.CryptoBookLevel, 0, len(raw))
	for _, value := range raw {
		price, priceOK := decimal(value.Price)
		size, sizeOK := decimal(value.Size)
		if !priceOK || !sizeOK {
			return nil, false
		}
		levels = append(levels, marketintelligence.CryptoBookLevel{Price: price, Size: size})
	}
	sort.Slice(levels, func(left, right int) bool {
		leftValue, _ := new(big.Rat).SetString(string(levels[left].Price))
		rightValue, _ := new(big.Rat).SetString(string(levels[right].Price))
		if descending {
			return leftValue.Cmp(rightValue) > 0
		}
		return leftValue.Cmp(rightValue) < 0
	})
	return levels, true
}

// RecentCryptoTrades returns a fixed-size, credential-free public tape. Trade
// IDs and provider bid/ask copies are discarded during normalization.
func (client *Client) RecentCryptoTrades(ctx context.Context, symbol, currency string, limit int) (marketintelligence.CryptoTradeTape, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if !validAssetSymbol(symbol) || !strings.EqualFold(strings.TrimSpace(currency), "usd") || limit != 25 {
		return marketintelligence.CryptoTradeTape{}, marketintelligence.ErrInvalidObservation
	}
	productID := symbol + "-USD"
	endpoint := *client.advancedTradeBaseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/api/v3/brokerage/market/products/" + url.PathEscape(productID) + "/ticker"
	query := endpoint.Query()
	query.Set("limit", strconv.Itoa(limit))
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return marketintelligence.CryptoTradeTape{}, ErrUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-cache")
	response, err := client.http.Do(request)
	if err != nil {
		return marketintelligence.CryptoTradeTape{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		if response.StatusCode == http.StatusTooManyRequests {
			return marketintelligence.CryptoTradeTape{}, ErrRateLimited
		}
		if response.StatusCode == http.StatusNotFound {
			return marketintelligence.CryptoTradeTape{}, marketintelligence.ErrInstrumentUnavailable
		}
		return marketintelligence.CryptoTradeTape{}, ErrUnavailable
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil || len(payload) > maxResponseSize {
		return marketintelligence.CryptoTradeTape{}, ErrInvalidResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var raw marketTradesResponse
	if err = decoder.Decode(&raw); err != nil || len(raw.Trades) == 0 || len(raw.Trades) > limit {
		return marketintelligence.CryptoTradeTape{}, ErrInvalidResponse
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return marketintelligence.CryptoTradeTape{}, ErrInvalidResponse
	}
	bestBid, bidOK := decimal(raw.BestBid)
	bestAsk, askOK := decimal(raw.BestAsk)
	if !bidOK || !askOK {
		return marketintelligence.CryptoTradeTape{}, ErrInvalidResponse
	}
	trades := make([]marketintelligence.CryptoTradeObservation, 0, len(raw.Trades))
	for _, value := range raw.Trades {
		price, priceOK := decimal(value.Price)
		size, sizeOK := decimal(value.Size)
		side := strings.ToUpper(strings.TrimSpace(value.Side))
		if !priceOK || !sizeOK || value.Time.IsZero() || strings.ToUpper(strings.TrimSpace(value.ProductID)) != productID ||
			(side != "BUY" && side != "SELL") || !strings.EqualFold(strings.TrimSpace(value.Exchange), "coinbase") {
			return marketintelligence.CryptoTradeTape{}, ErrInvalidResponse
		}
		trades = append(trades, marketintelligence.CryptoTradeObservation{Price: price, Size: size, Time: value.Time.UTC(), Side: side})
	}
	sort.SliceStable(trades, func(left, right int) bool { return trades[left].Time.After(trades[right].Time) })
	now := time.Now().UTC()
	tape := marketintelligence.CryptoTradeTape{
		Symbol: symbol, Currency: "USD", ProductID: productID, Limit: limit, Trades: trades, BestBid: bestBid, BestAsk: bestAsk,
		Provenance: marketintelligence.Provenance{
			Provider: "coinbase", ProviderRequestID: requestID(response), Role: marketintelligence.MarketObservation,
			Feed: "advanced_trade_public_market_trades", Quality: marketintelligence.RealTimeSingleVenue, Venue: "coinbase_advanced_trade",
			ProviderTimestamp: trades[0].Time, ReceivedAt: now,
		},
	}
	tradeFreshness := client.freshness
	if tradeFreshness.MaxAge > 30*time.Second {
		tradeFreshness.MaxAge = 30 * time.Second
	}
	if err := marketintelligence.ValidateCryptoTradeTape(tape, now, tradeFreshness); err != nil {
		return marketintelligence.CryptoTradeTape{}, err
	}
	return tape, nil
}

func (client *Client) TopCryptoMarkets(ctx context.Context, currency string, limit int) ([]marketintelligence.CryptoMarketObservation, error) {
	if !strings.EqualFold(strings.TrimSpace(currency), "usd") || limit < 1 || limit > 100 {
		return nil, marketintelligence.ErrInvalidObservation
	}
	if limit > len(client.products) {
		limit = len(client.products)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([]marketintelligence.CryptoMarketObservation, limit)
	errorsFound := make(chan error, limit)
	semaphore := make(chan struct{}, 4)
	var wait sync.WaitGroup
	for index, product := range client.products[:limit] {
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			observation, err := client.fetchTicker(ctx, product)
			if err != nil {
				select {
				case errorsFound <- err:
				default:
				}
				cancel()
				return
			}
			results[index] = observation
		}()
	}
	wait.Wait()
	select {
	case err := <-errorsFound:
		return nil, err
	default:
		return results, nil
	}
}

func (client *Client) CryptoMarkets(ctx context.Context, currency string, symbols []string) (marketintelligence.CryptoMarketBatch, error) {
	if !strings.EqualFold(strings.TrimSpace(currency), "usd") || len(symbols) == 0 || len(symbols) > 32 {
		return marketintelligence.CryptoMarketBatch{}, marketintelligence.ErrInvalidObservation
	}
	products := make([]Product, 0, len(symbols))
	seen := make(map[string]struct{}, len(symbols))
	for _, value := range symbols {
		symbol := strings.ToUpper(strings.TrimSpace(value))
		if !validAssetSymbol(symbol) {
			return marketintelligence.CryptoMarketBatch{}, marketintelligence.ErrInvalidObservation
		}
		if _, exists := seen[symbol]; exists {
			continue
		}
		seen[symbol] = struct{}{}
		products = append(products, Product{ID: symbol + "-USD", Name: client.productName(symbol)})
	}
	if len(products) == 0 {
		return marketintelligence.CryptoMarketBatch{}, marketintelligence.ErrInvalidObservation
	}

	type result struct {
		observation marketintelligence.CryptoMarketObservation
		unavailable bool
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([]result, len(products))
	errorsFound := make(chan error, len(products))
	semaphore := make(chan struct{}, 4)
	var wait sync.WaitGroup
	for index, product := range products {
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			observation, err := client.fetchTicker(ctx, product)
			if errors.Is(err, ErrProductUnavailable) {
				results[index].unavailable = true
				return
			}
			if err != nil {
				select {
				case errorsFound <- err:
				default:
				}
				cancel()
				return
			}
			results[index].observation = observation
		}()
	}
	wait.Wait()
	select {
	case err := <-errorsFound:
		return marketintelligence.CryptoMarketBatch{}, err
	default:
	}
	batch := marketintelligence.CryptoMarketBatch{
		Markets:            make([]marketintelligence.CryptoMarketObservation, 0, len(results)),
		UnavailableSymbols: make([]string, 0),
	}
	for index, value := range results {
		if value.unavailable {
			batch.UnavailableSymbols = append(batch.UnavailableSymbols, strings.TrimSuffix(products[index].ID, "-USD"))
			continue
		}
		batch.Markets = append(batch.Markets, value.observation)
	}
	return batch, nil
}

// RecentCryptoCandles returns a bounded venue history. Coinbase documents that
// intervals without ticks may be absent, so this method sorts and trims the
// provider response but never fills a missing bucket.
func (client *Client) RecentCryptoCandles(ctx context.Context, symbol, currency string, granularitySeconds, limit int) (marketintelligence.CryptoCandleSeries, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if !validAssetSymbol(symbol) || !strings.EqualFold(strings.TrimSpace(currency), "usd") || limit < 1 || limit > 300 || !validGranularity(granularitySeconds) {
		return marketintelligence.CryptoCandleSeries{}, marketintelligence.ErrInvalidObservation
	}
	productID := symbol + "-USD"
	end := time.Now().UTC()
	start := end.Add(-time.Duration(granularitySeconds*limit) * time.Second)
	endpoint := *client.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/products/" + url.PathEscape(productID) + "/candles"
	query := endpoint.Query()
	query.Set("start", start.Format(time.RFC3339))
	query.Set("end", end.Format(time.RFC3339))
	query.Set("granularity", strconv.Itoa(granularitySeconds))
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return marketintelligence.CryptoCandleSeries{}, ErrUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-cache")
	response, err := client.http.Do(request)
	if err != nil {
		return marketintelligence.CryptoCandleSeries{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		if response.StatusCode == http.StatusTooManyRequests {
			return marketintelligence.CryptoCandleSeries{}, ErrRateLimited
		}
		if response.StatusCode == http.StatusNotFound {
			return marketintelligence.CryptoCandleSeries{}, marketintelligence.ErrInstrumentUnavailable
		}
		return marketintelligence.CryptoCandleSeries{}, ErrUnavailable
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil || len(payload) > maxResponseSize {
		return marketintelligence.CryptoCandleSeries{}, ErrInvalidResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var rows [][]json.Number
	if err = decoder.Decode(&rows); err != nil || len(rows) == 0 || len(rows) > 300 {
		return marketintelligence.CryptoCandleSeries{}, ErrInvalidResponse
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return marketintelligence.CryptoCandleSeries{}, ErrInvalidResponse
	}
	candles := make([]marketintelligence.CryptoCandle, 0, len(rows))
	for _, row := range rows {
		if len(row) != 6 {
			return marketintelligence.CryptoCandleSeries{}, ErrInvalidResponse
		}
		epoch, epochErr := strconv.ParseInt(string(row[0]), 10, 64)
		low, lowOK := decimal(string(row[1]))
		high, highOK := decimal(string(row[2]))
		open, openOK := decimal(string(row[3]))
		closeValue, closeOK := decimal(string(row[4]))
		volume, volumeOK := decimal(string(row[5]))
		if epochErr != nil || !lowOK || !highOK || !openOK || !closeOK || !volumeOK {
			return marketintelligence.CryptoCandleSeries{}, ErrInvalidResponse
		}
		bucket := time.Unix(epoch, 0).UTC()
		if bucket.Before(start.Add(-time.Second)) || bucket.After(end.Add(time.Duration(granularitySeconds)*time.Second)) {
			continue
		}
		candles = append(candles, marketintelligence.CryptoCandle{Start: bucket, Low: low, High: high, Open: open, Close: closeValue, Volume: volume})
	}
	sort.Slice(candles, func(left, right int) bool { return candles[left].Start.Before(candles[right].Start) })
	if len(candles) > limit {
		candles = candles[len(candles)-limit:]
	}
	if len(candles) == 0 {
		return marketintelligence.CryptoCandleSeries{}, marketintelligence.ErrInstrumentUnavailable
	}
	now := time.Now().UTC()
	series := marketintelligence.CryptoCandleSeries{
		Symbol: symbol, Currency: "USD", GranularitySeconds: granularitySeconds, ExpectedIntervals: limit, Candles: candles,
		Provenance: marketintelligence.Provenance{
			Provider: "coinbase", ProviderRequestID: requestID(response), Role: marketintelligence.MarketObservation,
			Feed: "rest_candles", Quality: marketintelligence.RealTimeSingleVenue, Venue: "coinbase_exchange",
			ProviderTimestamp: candles[len(candles)-1].Start, ReceivedAt: now,
		},
	}
	historyPolicy := marketintelligence.FreshnessPolicy{
		MaxAge:        time.Duration(granularitySeconds*(limit+1)) * time.Second,
		MaxFutureSkew: client.freshness.MaxFutureSkew,
	}
	if err := marketintelligence.ValidateCryptoCandleSeries(series, now, historyPolicy); err != nil {
		return marketintelligence.CryptoCandleSeries{}, err
	}
	return series, nil
}

func validGranularity(value int) bool {
	switch value {
	case 60, 300, 900, 3600, 21600, 86400:
		return true
	default:
		return false
	}
}

func validAssetSymbol(symbol string) bool {
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

func (client *Client) productName(symbol string) string {
	for _, product := range client.products {
		if strings.TrimSuffix(product.ID, "-USD") == symbol {
			return product.Name
		}
	}
	return symbol
}

func (client *Client) fetchTicker(ctx context.Context, product Product) (marketintelligence.CryptoMarketObservation, error) {
	endpoint := *client.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/products/" + url.PathEscape(product.ID) + "/ticker"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return marketintelligence.CryptoMarketObservation{}, ErrUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-cache")
	response, err := client.http.Do(request)
	if err != nil {
		return marketintelligence.CryptoMarketObservation{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		if response.StatusCode == http.StatusTooManyRequests {
			return marketintelligence.CryptoMarketObservation{}, ErrRateLimited
		}
		if response.StatusCode == http.StatusNotFound {
			return marketintelligence.CryptoMarketObservation{}, ErrProductUnavailable
		}
		return marketintelligence.CryptoMarketObservation{}, ErrUnavailable
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil || len(payload) > maxResponseSize {
		return marketintelligence.CryptoMarketObservation{}, ErrInvalidResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var raw tickerResponse
	if err = decoder.Decode(&raw); err != nil {
		return marketintelligence.CryptoMarketObservation{}, ErrInvalidResponse
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return marketintelligence.CryptoMarketObservation{}, ErrInvalidResponse
	}
	price, ok := decimal(raw.Price)
	if !ok {
		return marketintelligence.CryptoMarketObservation{}, ErrInvalidResponse
	}
	bid, ok := optionalDecimal(raw.Bid)
	if !ok {
		return marketintelligence.CryptoMarketObservation{}, ErrInvalidResponse
	}
	ask, ok := optionalDecimal(raw.Ask)
	if !ok {
		return marketintelligence.CryptoMarketObservation{}, ErrInvalidResponse
	}
	volume, ok := optionalDecimal(raw.Volume)
	if !ok || raw.Time.IsZero() {
		return marketintelligence.CryptoMarketObservation{}, ErrInvalidResponse
	}
	baseSymbol := strings.TrimSuffix(product.ID, "-USD")
	now := time.Now().UTC()
	observation := marketintelligence.CryptoMarketObservation{
		ID: product.ID, Symbol: baseSymbol, Name: product.Name, Currency: "USD",
		CurrentPrice: price, Bid: bid, Ask: ask, Volume24H: volume, Volume24HUnit: baseSymbol,
		Provenance: marketintelligence.Provenance{
			Provider: "coinbase", ProviderRequestID: requestID(response), Role: marketintelligence.MarketObservation,
			Feed: "rest_ticker", Quality: marketintelligence.RealTimeSingleVenue, Venue: "coinbase_exchange",
			ProviderTimestamp: raw.Time.UTC(), ReceivedAt: now,
		},
	}
	if err := marketintelligence.ValidateCryptoMarket(observation, now, client.freshness); err != nil {
		return marketintelligence.CryptoMarketObservation{}, err
	}
	return observation, nil
}

func decimal(value string) (marketintelligence.Decimal, bool) {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 128 || !unsignedDecimalPattern.MatchString(value) {
		return "", false
	}
	parsed, ok := new(big.Rat).SetString(value)
	return marketintelligence.Decimal(value), ok && parsed.Sign() >= 0
}

func optionalDecimal(value string) (*marketintelligence.Decimal, bool) {
	if strings.TrimSpace(value) == "" {
		return nil, true
	}
	result, ok := decimal(value)
	return &result, ok
}

func requestID(response *http.Response) string {
	value := strings.TrimSpace(response.Header.Get("CB-Request-ID"))
	if value == "" {
		value = strings.TrimSpace(response.Header.Get("X-Request-ID"))
	}
	if len(value) > 128 {
		return ""
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return ""
		}
	}
	return value
}

var _ marketintelligence.CryptoMarketProvider = (*Client)(nil)
var _ marketintelligence.CryptoAssetMarketProvider = (*Client)(nil)
var _ marketintelligence.CryptoCandleProvider = (*Client)(nil)
var _ marketintelligence.CryptoLiquidityProvider = (*Client)(nil)
var _ marketintelligence.CryptoTradeProvider = (*Client)(nil)
