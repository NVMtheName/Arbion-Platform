package schwab

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

func TestAuthorizationURLAndAccountNormalization(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-secret" {
			t.Errorf("missing bearer authorization")
		}
		switch r.URL.Path {
		case "/accounts/accountNumbers":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"accountNumber":"1234564821","hashValue":"opaque-one"},{"accountNumber":"9184","hashValue":"opaque-two"}]`))
		case "/accounts/opaque-one":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"securitiesAccount":{"type":"MARGIN","currentBalances":{"cashBalance": "1000.00000001","availableFunds":"900.12","buyingPower":"1800.2400","liquidationValue":"2500.99","equity":"2400.88"},"positions":[{"longQuantity":"0.123456789012345678","shortQuantity":"0.0","marketValue":"12.345678901","averagePrice":"99.0001","instrument":{"assetType":"COLLECTIVE_INVESTMENT","symbol":"FUND","cusip":"sensitive-id"}},{"longQuantity":0.0,"shortQuantity":2.500,"marketValue":"20.00","averagePrice":"8.00","instrument":{"assetType":"EQUITY","symbol":"SHORT","cusip":"second-id"}},{"longQuantity":0.00,"shortQuantity":0.000,"marketValue":"0","averagePrice":"0","instrument":{"assetType":"EQUITY","symbol":"ZERO","cusip":"zero-id"}}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New(Config{ClientID: "client", RedirectURI: "https://arbion.example/callback", AuthorizationURL: srv.URL + "/authorize", TraderBaseURL: srv.URL}, srv.Client())
	u, e := c.AuthorizationURL(strings.Repeat("x", 43))
	if e != nil || !strings.Contains(u, "state=") {
		t.Fatalf("authorization URL: %v %s", e, u)
	}
	cr := &financial.Credentials{AccessToken: "access-secret", AccessExpiresAt: time.Now().Add(time.Hour)}
	accounts, e := c.ListAccounts(context.Background(), cr)
	if e != nil || len(accounts) != 2 {
		t.Fatalf("accounts: %v %#v", e, accounts)
	}
	if accounts[0].ProviderAccountID != "opaque-one" || accounts[0].MaskedIdentifier != "••••4821" {
		t.Fatalf("unsafe normalization: %#v", accounts[0])
	}
	b, e := c.GetBalances(context.Background(), cr, "opaque-one")
	if e != nil || b.Cash.Amount != "1000.00000001" || b.BuyingPower.Amount != "1800.2400" {
		t.Fatalf("balances: %v %#v", e, b)
	}
	p, e := c.GetPositions(context.Background(), cr, "opaque-one")
	if e != nil || len(p) != 2 || p[0].Quantity != "0.123456789012345678" || p[0].Direction != "long" || p[0].InstrumentType != "COLLECTIVE_INVESTMENT" || p[1].Quantity != "2.500" || p[1].Direction != "short" {
		t.Fatalf("positions: %v %#v", e, p)
	}
	caps, e := c.GetCapabilities(context.Background(), cr, "opaque-one")
	if e != nil || caps["margin"] != financial.Supported || caps["options"] != financial.Unknown {
		t.Fatalf("capabilities: %v %#v", e, caps)
	}
}

func TestNonZeroQuantityParsing(t *testing.T) {
	for _, test := range []struct {
		name  string
		value decimal
		want  bool
	}{
		{name: "missing", value: "", want: false},
		{name: "integer zero", value: "0", want: false},
		{name: "decimal zero", value: "0.0", want: false},
		{name: "negative decimal zero", value: "-0.000", want: false},
		{name: "positive", value: "0.0001", want: true},
		{name: "negative", value: "-2.5", want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := nonZero(test.value)
			if err != nil || got != test.want {
				t.Fatalf("nonZero(%q) = %v, %v; want %v", test.value, got, err, test.want)
			}
		})
	}
	if _, err := nonZero("not-a-decimal"); err == nil {
		t.Fatal("expected malformed quantity to fail closed")
	}
}

func TestRefreshPreservesRotatedPrecisionAndSecretsStayInHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" || r.Method != "POST" {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.URL.RawQuery, "refresh-secret") {
			t.Fatal("refresh token in URL")
		}
		_ = r.ParseForm()
		if r.Form.Get("refresh_token") != "refresh-secret" {
			t.Fatal("refresh token not in form")
		}
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","token_type":"Bearer","expires_in":1800}`))
	}))
	defer srv.Close()
	c := New(Config{ClientID: "id", ClientSecret: "secret", TokenURL: srv.URL + "/token"}, srv.Client())
	cr := &financial.Credentials{AccessToken: "old", RefreshToken: "refresh-secret"}
	if e := c.RefreshAuthorization(context.Background(), cr); e != nil {
		t.Fatal(e)
	}
	if cr.AccessToken != "new-access" || cr.RefreshToken != "new-refresh" {
		t.Fatalf("refresh not rotated: %#v", cr)
	}
}

func TestReadOnlyQuoteAndStandardOptionChainNormalization(t *testing.T) {
	quoteTime := int64(1767268800000)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer market-access" || r.Method != http.MethodGet {
			t.Fatal("market data request was not a bearer-authenticated GET")
		}
		switch r.URL.Path {
		case "/AAPL/quotes":
			if r.URL.Query().Get("fields") != "quote,reference" {
				t.Fatalf("unexpected quote fields: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"AAPL":{"assetMainType":"EQUITY","symbol":"AAPL","quote":{"bidPrice":"199.90000001","askPrice":200.10,"mark":"200.000000005","lastPrice":199.99,"quoteTime":1767268800000}}}`))
		case "/chains":
			q := r.URL.Query()
			if q.Get("symbol") != "AAPL" || q.Get("contractType") != "PUT" || q.Get("strikeCount") != "50" || q.Get("includeUnderlyingQuote") != "true" || q.Get("strategy") != "SINGLE" || q.Get("fromDate") != "2026-01-21" || q.Get("toDate") != "2026-03-02" {
				t.Fatalf("unexpected chain query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"symbol":"AAPL","status":"SUCCESS","underlyingPrice":"200.000000005","underlying":{"quoteTime":1767268800000},"putExpDateMap":{"2026-01-31:30":{"190.0":[{"putCall":"PUT","symbol":"AAPL  260131P00190000","bidPrice":"1.2500000001","askPrice":1.35,"markPrice":1.30,"volatility":"21.123456789","delta":"-0.300000001","quoteTimeInLong":1767268800000,"openInterest":123,"totalVolume":7,"strikePrice":"190.0000000000","expirationDate":"2026-01-31","multiplier":100.0,"isMini":false,"isNonStandard":false},{"putCall":"PUT","symbol":"NONSTANDARD","bidPrice":9,"delta":-0.3,"strikePrice":190,"expirationDate":"2026-01-31","multiplier":10.0,"isNonStandard":true}],"195.0":{"putCall":"PUT","symbol":"AAPL  260131P00195000","bidPrice":2.5,"askPrice":2.7,"delta":-0.4,"strikePrice":195,"expirationDate":"2026-01-31","multiplier":100.0}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New(Config{MarketDataBaseURL: srv.URL}, srv.Client())
	credentials := &financial.Credentials{AccessToken: "market-access"}
	quote, err := c.GetQuote(context.Background(), credentials, "aapl")
	if err != nil || quote.Symbol != "AAPL" || quote.Bid == nil || *quote.Bid != "199.90000001" || quote.Mark == nil || *quote.Mark != "200.000000005" || !quote.ProviderTimestamp.Equal(time.UnixMilli(quoteTime).UTC()) {
		t.Fatalf("quote normalization failed: %#v %v", quote, err)
	}
	chain, err := c.GetOptionChain(context.Background(), credentials, financial.OptionChainRequest{Symbol: "aapl", ContractType: "put", StrikeCount: 50, FromDate: time.Date(2026, 1, 21, 12, 0, 0, 0, time.UTC), ToDate: time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)})
	if err != nil || len(chain.Contracts) != 2 || chain.UnderlyingPrice == nil || *chain.UnderlyingPrice != "200.000000005" {
		t.Fatalf("chain normalization failed: %#v %v (%v)", chain, err, errors.Unwrap(err))
	}
	if chain.Contracts[0].Bid == nil || *chain.Contracts[0].Bid != "1.2500000001" || chain.Contracts[0].Delta == nil || *chain.Contracts[0].Delta != "-0.300000001" || chain.Contracts[0].OpenInterest == nil || *chain.Contracts[0].OpenInterest != 123 {
		t.Fatalf("contract precision changed: %#v", chain.Contracts[0])
	}
}

func TestMarketDataMalformedDecimalFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"AAPL":{"assetMainType":"EQUITY","symbol":"AAPL","quote":{"bidPrice":"not-a-decimal"}}}`))
	}))
	defer srv.Close()
	_, err := New(Config{MarketDataBaseURL: srv.URL}, srv.Client()).GetQuote(context.Background(), &financial.Credentials{AccessToken: "access"}, "AAPL")
	var providerError *financial.ProviderError
	if !errors.As(err, &providerError) || providerError.Code != financial.InvalidProviderResponse {
		t.Fatalf("malformed market data did not fail closed: %v", err)
	}
}

func TestProviderIntegerRequiresAnExactNonNegativeWholeNumber(t *testing.T) {
	for input, want := range map[decimal]int{
		"0":      0,
		"7":      7,
		"100.0":  100,
		"42.000": 42,
	} {
		got, err := providerInt(input)
		if err != nil || got == nil || *got != want {
			t.Fatalf("providerInt(%q) = %v, %v; want %d", input, got, err, want)
		}
	}
	for _, input := range []decimal{"-1", "1.5", "not-a-number", "999999999999999999999999999999999999"} {
		if got, err := providerInt(input); err == nil || got != nil {
			t.Fatalf("providerInt(%q) = %v, %v; want fail closed", input, got, err)
		}
	}
}
