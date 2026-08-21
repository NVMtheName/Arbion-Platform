package coinbase

import (
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestClientReturnsPortfolioTickersWithExplicitPartialCoverage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/products/BTC-USD/ticker":
			_, _ = writer.Write([]byte(`{"price":"70187.12","bid":"70186.90","ask":"70187.20","volume":"12.5","time":"` + time.Now().UTC().Format(time.RFC3339Nano) + `"}`))
		case "/products/RARE-USD/ticker":
			writer.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected request path: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL, Products: []Product{{ID: "BTC-USD", Name: "Bitcoin"}},
		Timeout: time.Second, MaxAge: time.Hour, MaxFutureSkew: time.Minute,
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	batch, err := client.CryptoMarkets(t.Context(), "USD", []string{"BTC", "RARE"})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Markets) != 1 || batch.Markets[0].Name != "Bitcoin" || batch.Markets[0].CurrentPrice != "70187.12" {
		t.Fatalf("portfolio ticker was not normalized: %+v", batch)
	}
	if len(batch.UnavailableSymbols) != 1 || batch.UnavailableSymbols[0] != "RARE" {
		t.Fatalf("missing product coverage was hidden: %+v", batch)
	}
	if _, err = client.CryptoMarkets(t.Context(), "USD", []string{"BTC-USD"}); err == nil {
		t.Fatal("invalid portfolio symbol accepted")
	}
}

func TestClientReturnsBoundedCandlesInAscendingOrderAndPreservesGaps(t *testing.T) {
	now := time.Now().UTC().Truncate(15 * time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/products/BTC-USD/candles" || request.URL.Query().Get("granularity") != "900" {
			t.Fatalf("unexpected candle request: %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		if request.Header.Get("Authorization") != "" || request.Header.Get("Cache-Control") != "no-cache" {
			t.Fatal("public candles must not send credentials and must bypass intermediary cache")
		}
		writer.Header().Set("CB-Request-ID", "candles-1")
		_, _ = writer.Write([]byte(`[[` +
			strconv.FormatInt(now.Add(-15*time.Minute).Unix(), 10) + `,70100,70300,70180,70250,2.5],[` +
			strconv.FormatInt(now.Add(-45*time.Minute).Unix(), 10) + `,70000.000000000000000001,70200,70025,70180,1.234567890123456789]]`))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Products: []Product{{ID: "BTC-USD", Name: "Bitcoin"}}, Timeout: time.Second, MaxAge: time.Hour, MaxFutureSkew: time.Minute}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	series, err := client.RecentCryptoCandles(t.Context(), "btc", "USD", 900, 96)
	if err != nil {
		t.Fatal(err)
	}
	if len(series.Candles) != 2 || !series.Candles[0].Start.Equal(now.Add(-45*time.Minute)) || series.Candles[0].Low != "70000.000000000000000001" || series.Candles[0].Volume != "1.234567890123456789" {
		t.Fatalf("candle ordering, gap, or precision changed: %+v", series)
	}
	if series.Provenance.Feed != "rest_candles" || series.Provenance.ProviderRequestID != "candles-1" || series.ExpectedIntervals != 96 {
		t.Fatalf("candle provenance or bounds missing: %+v", series)
	}
	if _, err = client.RecentCryptoCandles(t.Context(), "BTC-USD", "USD", 900, 96); err == nil {
		t.Fatal("invalid candle symbol accepted")
	}
}

func TestClientReturnsKeylessBoundedAdvancedTradeBook(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/v3/brokerage/market/product_book" || request.URL.Query().Get("product_id") != "BTC-USD" || request.URL.Query().Get("limit") != "10" {
			t.Fatalf("unexpected book request: %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		if request.Header.Get("Authorization") != "" || request.Header.Get("Cache-Control") != "no-cache" {
			t.Fatal("public product book must not send credentials and must bypass intermediary cache")
		}
		writer.Header().Set("CB-Request-ID", "book-1")
		_, _ = writer.Write([]byte(`{"pricebook":{"product_id":"BTC-USD","bids":[{"price":"70186.80","size":"0.5"},{"price":"70186.900000000000000001","size":"0.12500000"}],"asks":[{"price":"70187.3","size":"0.75"},{"price":"70187.200000000000000001","size":"0.25000000"}],"time":"` + now.Format(time.RFC3339Nano) + `"},"last":"70187.10","mid_market":"70187.050000000000000001","spread_bps":"0.042743","spread_absolute":"0.300000000000000000"}`))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, AdvancedTradeBaseURL: server.URL, Products: []Product{{ID: "BTC-USD", Name: "Bitcoin"}}, Timeout: time.Second, MaxAge: time.Minute, MaxFutureSkew: time.Minute}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := client.CryptoLiquidity(t.Context(), "btc", "USD", 10)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ProductID != "BTC-USD" || snapshot.Bids[0].Price != "70186.900000000000000001" || snapshot.Asks[0].Size != "0.25000000" || snapshot.SpreadAbsolute != "0.300000000000000000" {
		t.Fatalf("book order or precision changed: %+v", snapshot)
	}
	if snapshot.Provenance.ProviderRequestID != "book-1" || snapshot.Provenance.Feed != "advanced_trade_public_product_book" || snapshot.Provenance.Venue != "coinbase_advanced_trade" {
		t.Fatalf("book provenance missing: %+v", snapshot.Provenance)
	}
	if _, err = client.CryptoLiquidity(t.Context(), "BTC", "USD", 50); err == nil {
		t.Fatal("unbounded book depth accepted")
	}
}
