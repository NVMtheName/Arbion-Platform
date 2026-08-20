package coingecko

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
	return Config{APIKey: "demo-key", Tier: "demo", BaseURL: baseURL, Timeout: time.Second, MaxAge: 2 * time.Minute, MaxFutureSkew: time.Second}
}

func TestTopCryptoMarketsUsesKeyedHeaderAndNormalizesReferenceData(t *testing.T) {
	now := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/coins/markets" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("vs_currency") != "usd" || query.Get("order") != "market_cap_desc" || query.Get("per_page") != "2" || query.Get("page") != "1" || query.Get("sparkline") != "false" || query.Get("price_change_percentage") != "24h" {
			t.Fatalf("unexpected query: %s", request.URL.RawQuery)
		}
		if request.Header.Get("x-cg-demo-api-key") != "demo-key" || strings.Contains(request.URL.RawQuery, "demo-key") {
			t.Fatal("CoinGecko key missing from header or leaked into URL")
		}
		writer.Header().Set("X-Request-ID", "cg-request-1")
		_, _ = writer.Write([]byte(`[
			{"id":"bitcoin","symbol":"btc","name":"Bitcoin","current_price":70187.123456789012345678,"market_cap":1381651251183,"market_cap_rank":1,"total_volume":20154184933,"price_change_percentage_24h":3.12502,"last_updated":"` + now + `"},
			{"id":"tiny-token","symbol":"tiny","name":"Tiny Token","current_price":1.25e-8,"market_cap":null,"market_cap_rank":2,"total_volume":100,"price_change_percentage_24h":-4.25,"last_updated":"` + now + `"}
		]`))
	}))
	defer server.Close()

	client, err := New(config(server.URL), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	markets, err := client.TopCryptoMarkets(t.Context(), "USD", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(markets) != 2 || string(markets[0].CurrentPrice) != "70187.123456789012345678" || string(markets[1].CurrentPrice) != "0.0000000125" {
		t.Fatalf("market precision changed: %+v", markets)
	}
	if markets[1].ChangePercent24H == nil || string(*markets[1].ChangePercent24H) != "-4.25" || markets[1].MarketCap != nil {
		t.Fatalf("nullable or signed values changed: %+v", markets[1])
	}
	if markets[0].Provenance.Quality != marketintelligence.AggregatedReference || markets[0].Provenance.ProviderRequestID != "cg-request-1" {
		t.Fatalf("reference provenance missing: %+v", markets[0].Provenance)
	}
}

func TestNewRequiresKeyedFixedTierConfiguration(t *testing.T) {
	for _, test := range []Config{
		{},
		{APIKey: "key", Tier: "keyless", Timeout: time.Second, MaxAge: time.Minute},
		{APIKey: "key", Tier: "demo", BaseURL: "https://example.com/api/v3", Timeout: time.Second, MaxAge: time.Minute},
		{APIKey: "key", Tier: "demo", BaseURL: "http://example.com", Timeout: time.Second, MaxAge: time.Minute},
	} {
		if _, err := New(test, http.DefaultClient); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("unsafe configuration accepted: %+v err=%v", test, err)
		}
	}
}

func TestTopCryptoMarketsRejectsInvalidInputsAndResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`[]`))
	}))
	defer server.Close()
	client, err := New(config(server.URL), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.TopCryptoMarkets(t.Context(), "US D", 10); !errors.Is(err, marketintelligence.ErrInvalidObservation) {
		t.Fatalf("invalid currency accepted: %v", err)
	}
	if _, err = client.TopCryptoMarkets(t.Context(), "USD", 101); !errors.Is(err, marketintelligence.ErrInvalidObservation) {
		t.Fatalf("invalid limit accepted: %v", err)
	}
	if _, err = client.TopCryptoMarkets(t.Context(), "USD", 10); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("empty response accepted: %v", err)
	}
}

func TestTopCryptoMarketsClassifiesFailuresAndRejectsStaleData(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, want: ErrUnauthorized},
		{name: "rate limited", status: http.StatusTooManyRequests, want: ErrRateLimited},
		{name: "unavailable", status: http.StatusBadGateway, want: ErrUnavailable},
		{name: "stale", status: http.StatusOK, body: `[{"id":"bitcoin","symbol":"btc","name":"Bitcoin","current_price":1,"market_cap_rank":1,"last_updated":"` + time.Now().UTC().Add(-3*time.Minute).Format(time.RFC3339Nano) + `"}]`, want: marketintelligence.ErrStaleObservation},
		{name: "missing price", status: http.StatusOK, body: `[{"id":"bitcoin","symbol":"btc","name":"Bitcoin","current_price":null,"market_cap_rank":1,"last_updated":"` + time.Now().UTC().Format(time.RFC3339Nano) + `"}]`, want: ErrInvalidResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			client, err := New(config(server.URL), server.Client())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.TopCryptoMarkets(t.Context(), "USD", 1); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}
