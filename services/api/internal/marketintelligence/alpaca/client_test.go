package alpaca

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/marketintelligence"
)

func config(baseURL string) Config {
	return Config{KeyID: "test-key-id", SecretKey: "test-secret", BaseURL: baseURL, EquityFeed: "iex", Timeout: time.Second, MaxAge: time.Minute, MaxFutureSkew: time.Second}
}

func TestLatestEquityQuoteNormalizesExactDataWithoutLeakingKeys(t *testing.T) {
	now := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v2/stocks/AAPL/quotes/latest" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.URL.Query().Get("feed") != "iex" || request.URL.Query().Get("currency") != "USD" {
			t.Fatalf("unexpected query: %s", request.URL.RawQuery)
		}
		if request.Header.Get("APCA-API-KEY-ID") != "test-key-id" || request.Header.Get("APCA-API-SECRET-KEY") != "test-secret" {
			t.Fatal("missing authentication headers")
		}
		if strings.Contains(request.URL.RawQuery, "test-key") || strings.Contains(request.URL.RawQuery, "test-secret") {
			t.Fatal("credential leaked into URL")
		}
		writer.Header().Set("APCA-Request-ID", "request-123")
		_, _ = writer.Write([]byte(`{"symbol":"AAPL","quote":{"ap":226.130000000000000002,"bp":226.120000000000000001,"t":"` + now + `"}}`))
	}))
	defer server.Close()

	client, err := New(config(server.URL), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	quote, err := client.LatestEquityQuote(t.Context(), "aapl")
	if err != nil {
		t.Fatal(err)
	}
	if quote.Symbol != "AAPL" || quote.Bid == nil || string(*quote.Bid) != "226.120000000000000001" || quote.Ask == nil || string(*quote.Ask) != "226.130000000000000002" {
		t.Fatalf("precision or identity changed: %+v", quote)
	}
	if quote.Provenance.Quality != marketintelligence.RealTimeSingleVenue || quote.Provenance.Venue != "IEX" || quote.Provenance.ProviderRequestID != "request-123" {
		t.Fatalf("provenance missing: %+v", quote.Provenance)
	}
}

func TestNewFailsClosedForUnsafeConfiguration(t *testing.T) {
	for _, test := range []Config{
		{},
		{KeyID: "key", SecretKey: "", BaseURL: "https://data.alpaca.markets", EquityFeed: "iex", Timeout: time.Second, MaxAge: time.Minute},
		{KeyID: "key", SecretKey: "secret", BaseURL: "http://example.com", EquityFeed: "iex", Timeout: time.Second, MaxAge: time.Minute},
		{KeyID: "key", SecretKey: "secret", BaseURL: "https://example.com", EquityFeed: "iex", Timeout: time.Second, MaxAge: time.Minute},
		{KeyID: "key", SecretKey: "secret", BaseURL: "https://data.alpaca.markets/path", EquityFeed: "iex", Timeout: time.Second, MaxAge: time.Minute},
		{KeyID: "key", SecretKey: "secret", BaseURL: "https://data.alpaca.markets", EquityFeed: "best", Timeout: time.Second, MaxAge: time.Minute},
	} {
		if _, err := New(test, http.DefaultClient); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("unsafe configuration accepted: %+v err=%v", test, err)
		}
	}
}

func TestLatestEquityQuoteClassifiesProviderFailures(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrUnauthorized},
		{http.StatusForbidden, ErrUnauthorized},
		{http.StatusTooManyRequests, ErrRateLimited},
		{http.StatusInternalServerError, ErrUnavailable},
	}
	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(`{"secret":"provider body must not escape"}`))
			}))
			defer server.Close()
			client, err := New(config(server.URL), server.Client())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.LatestEquityQuote(t.Context(), "AAPL"); !errors.Is(err, test.want) || strings.Contains(err.Error(), "provider body") {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestLatestEquityQuoteRejectsMalformedStaleAndOversizedResponses(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name string
		body string
		want error
	}{
		{name: "exponent price", body: `{"symbol":"AAPL","quote":{"ap":2.26e2,"bp":226.1,"t":"` + now.Format(time.RFC3339Nano) + `"}}`, want: marketintelligence.ErrInvalidObservation},
		{name: "stale", body: `{"symbol":"AAPL","quote":{"ap":226.2,"bp":226.1,"t":"` + now.Add(-2*time.Minute).Format(time.RFC3339Nano) + `"}}`, want: marketintelligence.ErrStaleObservation},
		{name: "wrong symbol", body: `{"symbol":"TSLA","quote":{"ap":226.2,"bp":226.1,"t":"` + now.Format(time.RFC3339Nano) + `"}}`, want: ErrInvalidResponse},
		{name: "oversized", body: strings.Repeat("x", maxResponseBytes+1), want: ErrInvalidResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { _, _ = writer.Write([]byte(test.body)) }))
			defer server.Close()
			client, err := New(config(server.URL), server.Client())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.LatestEquityQuote(t.Context(), "AAPL"); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}
