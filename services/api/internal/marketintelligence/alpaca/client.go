// Package alpaca implements the read-only Alpaca Market Data adapter.
// It contains no Trading API or broker-order client.
package alpaca

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
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/marketintelligence"
)

const maxResponseBytes = 64 << 10

var (
	ErrInvalidConfiguration = errors.New("invalid Alpaca market data configuration")
	ErrUnauthorized         = errors.New("Alpaca market data authorization failed")
	ErrRateLimited          = errors.New("Alpaca market data rate limited")
	ErrUnavailable          = errors.New("Alpaca market data unavailable")
	ErrInvalidResponse      = errors.New("invalid Alpaca market data response")
)

var symbolPattern = regexp.MustCompile(`^[A-Z][A-Z0-9.\-]{0,14}$`)

type Config struct {
	KeyID         string
	SecretKey     string
	BaseURL       string
	EquityFeed    string
	Timeout       time.Duration
	MaxAge        time.Duration
	MaxFutureSkew time.Duration
}

type Client struct {
	keyID      string
	secretKey  string
	baseURL    *url.URL
	equityFeed string
	quality    marketintelligence.FeedQuality
	venue      string
	freshness  marketintelligence.FreshnessPolicy
	http       *http.Client
}

type latestQuoteResponse struct {
	Symbol string `json:"symbol"`
	Quote  struct {
		AskPrice  json.Number `json:"ap"`
		BidPrice  json.Number `json:"bp"`
		Timestamp time.Time   `json:"t"`
	} `json:"quote"`
}

func New(config Config, httpClient *http.Client) (*Client, error) {
	config.KeyID = strings.TrimSpace(config.KeyID)
	config.SecretKey = strings.TrimSpace(config.SecretKey)
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	config.EquityFeed = strings.ToLower(strings.TrimSpace(config.EquityFeed))
	if config.KeyID == "" || config.SecretKey == "" || config.BaseURL == "" || config.Timeout <= 0 || config.MaxAge <= 0 || config.MaxFutureSkew < 0 || httpClient == nil {
		return nil, ErrInvalidConfiguration
	}
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" || !approvedBaseURL(baseURL) {
		return nil, ErrInvalidConfiguration
	}
	if baseURL.Path != "" {
		return nil, ErrInvalidConfiguration
	}

	quality, venue, ok := feedMetadata(config.EquityFeed)
	if !ok {
		return nil, ErrInvalidConfiguration
	}
	configuredHTTP := *httpClient
	configuredHTTP.Timeout = config.Timeout
	return &Client{
		keyID: config.KeyID, secretKey: config.SecretKey, baseURL: baseURL,
		equityFeed: config.EquityFeed, quality: quality, venue: venue,
		freshness: marketintelligence.FreshnessPolicy{MaxAge: config.MaxAge, MaxFutureSkew: config.MaxFutureSkew},
		http:      &configuredHTTP,
	}, nil
}

func approvedBaseURL(baseURL *url.URL) bool {
	if baseURL.Scheme == "https" {
		return baseURL.Host == "data.alpaca.markets"
	}
	if baseURL.Scheme == "http" {
		host := baseURL.Hostname()
		return host == "localhost" || net.ParseIP(host).IsLoopback()
	}
	return false
}

func feedMetadata(feed string) (marketintelligence.FeedQuality, string, bool) {
	switch feed {
	case "iex":
		return marketintelligence.RealTimeSingleVenue, "IEX", true
	case "sip":
		return marketintelligence.RealTimeConsolidated, "US_CONSOLIDATED", true
	case "delayed_sip":
		return marketintelligence.Delayed, "US_CONSOLIDATED_DELAYED", true
	default:
		return "", "", false
	}
}

func (client *Client) LatestEquityQuote(ctx context.Context, symbol string) (marketintelligence.QuoteObservation, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if !symbolPattern.MatchString(symbol) {
		return marketintelligence.QuoteObservation{}, marketintelligence.ErrInvalidObservation
	}

	endpoint := *client.baseURL
	endpoint.Path = "/v2/stocks/" + url.PathEscape(symbol) + "/quotes/latest"
	query := endpoint.Query()
	query.Set("feed", client.equityFeed)
	query.Set("currency", "USD")
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return marketintelligence.QuoteObservation{}, ErrUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("APCA-API-KEY-ID", client.keyID)
	request.Header.Set("APCA-API-SECRET-KEY", client.secretKey)

	response, err := client.http.Do(request)
	if err != nil {
		return marketintelligence.QuoteObservation{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return marketintelligence.QuoteObservation{}, statusError(response.StatusCode)
	}

	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(payload) > maxResponseBytes {
		return marketintelligence.QuoteObservation{}, ErrInvalidResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var raw latestQuoteResponse
	if err = decoder.Decode(&raw); err != nil {
		return marketintelligence.QuoteObservation{}, ErrInvalidResponse
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return marketintelligence.QuoteObservation{}, ErrInvalidResponse
	}
	if raw.Symbol != symbol || raw.Quote.Timestamp.IsZero() {
		return marketintelligence.QuoteObservation{}, ErrInvalidResponse
	}

	bid := marketintelligence.Decimal(raw.Quote.BidPrice.String())
	ask := marketintelligence.Decimal(raw.Quote.AskPrice.String())
	now := time.Now().UTC()
	providerRequestID := response.Header.Get("APCA-Request-ID")
	if providerRequestID == "" {
		providerRequestID = response.Header.Get("X-Request-ID")
	}
	providerRequestID = safeRequestID(providerRequestID)
	observation := marketintelligence.QuoteObservation{
		Symbol: symbol, AssetClass: marketintelligence.Equity, Currency: "USD", Bid: &bid, Ask: &ask,
		Provenance: marketintelligence.Provenance{
			Provider: "alpaca", ProviderRequestID: providerRequestID, Role: marketintelligence.MarketObservation,
			Feed: client.equityFeed, Quality: client.quality, Venue: client.venue,
			ProviderTimestamp: raw.Quote.Timestamp.UTC(), ReceivedAt: now,
		},
	}
	if err = marketintelligence.ValidateQuote(observation, now, client.freshness); err != nil {
		return marketintelligence.QuoteObservation{}, err
	}
	return observation, nil
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

var _ marketintelligence.EquityQuoteProvider = (*Client)(nil)
