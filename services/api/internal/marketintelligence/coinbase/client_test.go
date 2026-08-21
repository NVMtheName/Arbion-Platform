package coinbase

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/marketintelligence"
)

func TestClientReturnsKeylessVenueTickerWithProvenance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/products/BTC-USD/ticker" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Cache-Control") != "no-cache" || request.Header.Get("Authorization") != "" {
			t.Fatal("Coinbase public ticker must bypass cache without sending credentials")
		}
		writer.Header().Set("CB-Request-ID", "coinbase-request-1")
		_, _ = writer.Write([]byte(`{"price":"70187.123456789012345678","bid":"70186.9","ask":"70187.2","volume":"12345.67890123","time":"` + time.Now().UTC().Format(time.RFC3339Nano) + `"}`))
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL, Products: []Product{{ID: "BTC-USD", Name: "Bitcoin"}},
		Timeout: time.Second, MaxAge: 24 * time.Hour, MaxFutureSkew: time.Minute,
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	markets, err := client.TopCryptoMarkets(t.Context(), "USD", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(markets) != 1 || markets[0].CurrentPrice != "70187.123456789012345678" || markets[0].Volume24HUnit != "BTC" {
		t.Fatalf("market precision or identity changed: %+v", markets)
	}
	if markets[0].Provenance.Provider != "coinbase" || markets[0].Provenance.Quality != marketintelligence.RealTimeSingleVenue || markets[0].Provenance.Venue != "coinbase_exchange" {
		t.Fatalf("venue provenance missing: %+v", markets[0].Provenance)
	}
}

func TestClientRejectsInvalidConfigurationAndQueries(t *testing.T) {
	if _, err := New(Config{BaseURL: "https://example.com", Timeout: time.Second, MaxAge: time.Minute}, http.DefaultClient); err == nil {
		t.Fatal("unapproved Coinbase host accepted")
	}
	client, err := New(Config{
		BaseURL: "http://127.0.0.1:8080", Products: []Product{{ID: "BTC-USD", Name: "Bitcoin"}},
		Timeout: time.Second, MaxAge: time.Minute,
	}, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.TopCryptoMarkets(t.Context(), "eur", 1); err == nil {
		t.Fatal("unsupported quote currency accepted")
	}
}
