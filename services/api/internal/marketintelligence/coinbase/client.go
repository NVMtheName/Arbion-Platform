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
	"strings"
	"sync"
	"time"

	"github.com/arbion/platform/services/api/internal/marketintelligence"
)

const (
	defaultBaseURL  = "https://api.exchange.coinbase.com"
	maxResponseSize = 64 << 10
)

var (
	ErrInvalidConfiguration = errors.New("invalid Coinbase configuration")
	ErrRateLimited          = errors.New("Coinbase rate limited")
	ErrUnavailable          = errors.New("Coinbase unavailable")
	ErrInvalidResponse      = errors.New("invalid Coinbase response")
	productPattern          = regexp.MustCompile(`^[A-Z0-9]{2,12}-USD$`)
	unsignedDecimalPattern  = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)
)

type Product struct {
	ID   string
	Name string
}

type Config struct {
	BaseURL       string
	Products      []Product
	Timeout       time.Duration
	MaxAge        time.Duration
	MaxFutureSkew time.Duration
}

type Client struct {
	baseURL   *url.URL
	products  []Product
	freshness marketintelligence.FreshnessPolicy
	http      *http.Client
}

type tickerResponse struct {
	Price  string    `json:"price"`
	Bid    string    `json:"bid"`
	Ask    string    `json:"ask"`
	Volume string    `json:"volume"`
	Time   time.Time `json:"time"`
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
	if len(config.Products) == 0 {
		config.Products = DefaultProducts()
	}
	if httpClient == nil || config.Timeout <= 0 || config.MaxAge <= 0 || config.MaxFutureSkew < 0 || len(config.Products) > 32 {
		return nil, ErrInvalidConfiguration
	}
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" || !approvedBaseURL(baseURL) {
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
		baseURL:  baseURL,
		products: products,
		freshness: marketintelligence.FreshnessPolicy{
			MaxAge: config.MaxAge, MaxFutureSkew: config.MaxFutureSkew,
		},
		http: &configuredHTTP,
	}, nil
}

func approvedBaseURL(baseURL *url.URL) bool {
	if baseURL.String() == defaultBaseURL {
		return true
	}
	if baseURL.Scheme != "http" || baseURL.Path != "" {
		return false
	}
	host := baseURL.Hostname()
	return host == "localhost" || net.ParseIP(host).IsLoopback()
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
		if response.StatusCode == http.StatusTooManyRequests {
			return marketintelligence.CryptoMarketObservation{}, ErrRateLimited
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
