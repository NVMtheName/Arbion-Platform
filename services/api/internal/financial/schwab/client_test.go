package schwab

import (
	"context"
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
