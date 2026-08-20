// Package coingecko implements the read-only CoinGecko reference-data adapter.
package coingecko

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/marketintelligence"
)

const maxResponseBytes = 1 << 20

var (
	ErrInvalidConfiguration = errors.New("invalid CoinGecko configuration")
	ErrUnauthorized         = errors.New("CoinGecko authorization failed")
	ErrRateLimited          = errors.New("CoinGecko rate limited")
	ErrUnavailable          = errors.New("CoinGecko unavailable")
	ErrInvalidResponse      = errors.New("invalid CoinGecko response")
)

var currencyPattern = regexp.MustCompile(`^[a-z][a-z0-9]{1,11}$`)

type Config struct {
	APIKey        string
	Tier          string
	BaseURL       string
	Timeout       time.Duration
	MaxAge        time.Duration
	MaxFutureSkew time.Duration
}

type Client struct {
	apiKey    string
	header    string
	baseURL   *url.URL
	freshness marketintelligence.FreshnessPolicy
	http      *http.Client
}

type rawMarket struct {
	ID                       string       `json:"id"`
	Symbol                   string       `json:"symbol"`
	Name                     string       `json:"name"`
	CurrentPrice             *json.Number `json:"current_price"`
	MarketCap                *json.Number `json:"market_cap"`
	MarketCapRank            *int         `json:"market_cap_rank"`
	TotalVolume              *json.Number `json:"total_volume"`
	PriceChangePercentage24H *json.Number `json:"price_change_percentage_24h"`
	LastUpdated              time.Time    `json:"last_updated"`
}

func New(config Config, httpClient *http.Client) (*Client, error) {
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.Tier = strings.ToLower(strings.TrimSpace(config.Tier))
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.APIKey == "" || config.Timeout <= 0 || config.MaxAge <= 0 || config.MaxFutureSkew < 0 || httpClient == nil {
		return nil, ErrInvalidConfiguration
	}
	header, expectedURL, ok := tierMetadata(config.Tier)
	if !ok {
		return nil, ErrInvalidConfiguration
	}
	if config.BaseURL == "" {
		config.BaseURL = expectedURL
	}
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" || !approvedBaseURL(baseURL, expectedURL) {
		return nil, ErrInvalidConfiguration
	}

	configuredHTTP := *httpClient
	configuredHTTP.Timeout = config.Timeout
	return &Client{
		apiKey: config.APIKey, header: header, baseURL: baseURL,
		freshness: marketintelligence.FreshnessPolicy{MaxAge: config.MaxAge, MaxFutureSkew: config.MaxFutureSkew},
		http:      &configuredHTTP,
	}, nil
}

func tierMetadata(tier string) (header string, baseURL string, ok bool) {
	switch tier {
	case "demo":
		return "x-cg-demo-api-key", "https://api.coingecko.com/api/v3", true
	case "pro":
		return "x-cg-pro-api-key", "https://pro-api.coingecko.com/api/v3", true
	default:
		return "", "", false
	}
}

func approvedBaseURL(baseURL *url.URL, expected string) bool {
	if baseURL.String() == expected {
		return true
	}
	if baseURL.Scheme != "http" || baseURL.Path != "" {
		return false
	}
	host := baseURL.Hostname()
	return host == "localhost" || net.ParseIP(host).IsLoopback()
}

func (client *Client) TopCryptoMarkets(ctx context.Context, currency string, limit int) ([]marketintelligence.CryptoMarketObservation, error) {
	currency = strings.ToLower(strings.TrimSpace(currency))
	if !currencyPattern.MatchString(currency) || limit < 1 || limit > 100 {
		return nil, marketintelligence.ErrInvalidObservation
	}

	endpoint := *client.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/coins/markets"
	query := endpoint.Query()
	query.Set("vs_currency", currency)
	query.Set("order", "market_cap_desc")
	query.Set("per_page", strconv.Itoa(limit))
	query.Set("page", "1")
	query.Set("sparkline", "false")
	query.Set("price_change_percentage", "24h")
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set(client.header, client.apiKey)

	response, err := client.http.Do(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, statusError(response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(payload) > maxResponseBytes {
		return nil, ErrInvalidResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var raw []rawMarket
	if err = decoder.Decode(&raw); err != nil || len(raw) == 0 || len(raw) > limit {
		return nil, ErrInvalidResponse
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, ErrInvalidResponse
	}

	now := time.Now().UTC()
	requestID := safeRequestID(response.Header.Get("X-Request-ID"))
	result := make([]marketintelligence.CryptoMarketObservation, 0, len(raw))
	for _, item := range raw {
		observation, normalizeErr := client.normalize(item, currency, requestID, now)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		result = append(result, observation)
	}
	return result, nil
}

func (client *Client) normalize(raw rawMarket, currency, requestID string, now time.Time) (marketintelligence.CryptoMarketObservation, error) {
	price, ok := requiredDecimal(raw.CurrentPrice, false)
	if !ok {
		return marketintelligence.CryptoMarketObservation{}, ErrInvalidResponse
	}
	marketCap, ok := optionalDecimal(raw.MarketCap, false)
	if !ok {
		return marketintelligence.CryptoMarketObservation{}, ErrInvalidResponse
	}
	volume, ok := optionalDecimal(raw.TotalVolume, false)
	if !ok {
		return marketintelligence.CryptoMarketObservation{}, ErrInvalidResponse
	}
	change, ok := optionalDecimal(raw.PriceChangePercentage24H, true)
	if !ok {
		return marketintelligence.CryptoMarketObservation{}, ErrInvalidResponse
	}
	observation := marketintelligence.CryptoMarketObservation{
		ID: strings.TrimSpace(raw.ID), Symbol: strings.ToUpper(strings.TrimSpace(raw.Symbol)), Name: strings.TrimSpace(raw.Name),
		Currency: strings.ToUpper(currency), CurrentPrice: price, MarketCap: marketCap, MarketCapRank: raw.MarketCapRank,
		Volume24H: volume, ChangePercent24H: change,
		Provenance: marketintelligence.Provenance{
			Provider: "coingecko", ProviderRequestID: requestID, Role: marketintelligence.ReferenceData,
			Feed: "rest", Quality: marketintelligence.AggregatedReference,
			ProviderTimestamp: raw.LastUpdated.UTC(), ReceivedAt: now,
		},
	}
	if err := marketintelligence.ValidateCryptoMarket(observation, now, client.freshness); err != nil {
		return marketintelligence.CryptoMarketObservation{}, err
	}
	return observation, nil
}

func requiredDecimal(value *json.Number, signed bool) (marketintelligence.Decimal, bool) {
	if value == nil {
		return "", false
	}
	result, ok := normalizeDecimal(value.String(), signed)
	return marketintelligence.Decimal(result), ok
}

func optionalDecimal(value *json.Number, signed bool) (*marketintelligence.Decimal, bool) {
	if value == nil {
		return nil, true
	}
	result, ok := requiredDecimal(value, signed)
	if !ok {
		return nil, false
	}
	return &result, true
}

func statusError(status int) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return ErrUnavailable
	}
}

func safeRequestID(value string) string {
	value = strings.TrimSpace(value)
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
