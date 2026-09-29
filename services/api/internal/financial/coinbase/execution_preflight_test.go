package coinbase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/financial"
)

const executionTestPortfolio = "10000000-0000-4000-8000-000000000001"

func executionTestOrder() execution.Order {
	return execution.Order{RequestDigest: strings.Repeat("a", 64), Request: execution.Request{
		ProductID: "BTC-USD", Side: "BUY", BaseSize: "0.0010", LimitPrice: "30000.00", FeeAllowanceUSD: "0.10", MaximumDebitUSD: "30.10",
	}}
}

func executionTestFixtures(now time.Time) map[string]string {
	return map[string]string{
		"/api/v3/brokerage/key_permissions":   `{"can_view":true,"can_trade":true,"can_transfer":false,"portfolio_uuid":"` + executionTestPortfolio + `"}`,
		"/api/v3/brokerage/accounts":          `{"accounts":[{"uuid":"20000000-0000-4000-8000-000000000001","currency":"USD","available_balance":{"value":"1000.10","currency":"USD"},"hold":{"value":"0","currency":"USD"},"active":true,"ready":true,"retail_portfolio_id":"` + executionTestPortfolio + `"}],"has_next":true,"cursor":"page-two"}`,
		"/api/v3/brokerage/accounts?page-two": `{"accounts":[{"uuid":"20000000-0000-4000-8000-000000000002","currency":"BTC","available_balance":{"value":"1.00000001","currency":"BTC"},"hold":{"value":"0","currency":"BTC"},"active":true,"ready":true,"retail_portfolio_id":"` + executionTestPortfolio + `"}],"has_next":false,"cursor":""}`,
		"/api/v3/brokerage/products/BTC-USD":  `{"product_id":"BTC-USD","product_type":"SPOT","base_currency_id":"BTC","quote_currency_id":"USD","status":"online","base_increment":"0.00000001","price_increment":"0.01","base_min_size":"0.00001","base_max_size":"100","quote_min_size":"1","quote_max_size":"1000000000","is_disabled":false,"trading_disabled":false,"cancel_only":false,"limit_only":false,"post_only":false,"auction_mode":false,"view_only":false}`,
		"/api/v3/brokerage/product_book":      `{"pricebook":{"product_id":"BTC-USD","bids":[{"price":"29999.99","size":"1"}],"asks":[{"price":"30000.00","size":"1"}],"time":"` + now.Format(time.RFC3339Nano) + `"}}`,
		"/api/v3/brokerage/orders/preview":    `{"preview_id":"30000000-0000-4000-8000-000000000001","base_size":"0.0010","quote_size":"30.00","commission_total":"0.10","order_total":"30.10","est_average_filled_price":"30000.00","errs":[],"warning":[],"is_max":false}`,
	}
}

func executionTestClient(t *testing.T, fixtures map[string]string, expectedOrders ...execution.Order) (*Client, *financial.Credentials, *[]string) {
	t.Helper()
	expected := executionTestOrder()
	if len(expectedOrders) > 0 {
		expected = expectedOrders[0]
	}
	credentials, key := testCredentials(t)
	credentials.PortfolioID = executionTestPortfolio
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verifyJWT(t, r, key, r.URL.Path)
		requests = append(requests, r.Method+" "+r.URL.Path)
		path := r.URL.Path
		switch path {
		case "/api/v3/brokerage/accounts":
			if r.URL.Query().Get("limit") != "250" || r.URL.Query().Get("retail_portfolio_id") != executionTestPortfolio {
				t.Errorf("unbounded or unscoped inventory query: %s", r.URL.RawQuery)
			}
			if cursor := r.URL.Query().Get("cursor"); cursor != "" {
				path += "?" + cursor
			}
		case "/api/v3/brokerage/product_book":
			if r.URL.Query().Get("limit") != "1" || r.URL.Query().Get("product_id") != "BTC-USD" {
				t.Errorf("unexpected book query: %s", r.URL.RawQuery)
			}
		case "/api/v3/brokerage/orders/preview":
			if r.Method != http.MethodPost {
				t.Errorf("wrong preview method: %s", r.Method)
			}
			body, _ := io.ReadAll(r.Body)
			var got any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Error(err)
			}
			want := map[string]any{
				"product_id": expected.Request.ProductID, "side": expected.Request.Side, "retail_portfolio_id": executionTestPortfolio,
				"order_configuration": map[string]any{"sor_limit_ioc": map[string]any{"base_size": expected.Request.BaseSize, "limit_price": expected.Request.LimitPrice}},
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("preview was not the exact portfolio-bound LIMIT_IOC request: %s", body)
			}
		}
		if path != "/api/v3/brokerage/orders/preview" && r.Method != http.MethodGet {
			t.Errorf("unexpected provider write: %s %s", r.Method, path)
		}
		body, ok := fixtures[path]
		if !ok {
			t.Errorf("unexpected endpoint or cursor: %s", path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	client, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }
	return client, &credentials, &requests
}

func TestExecutionPreflightReadsCompleteInventoryAndOnlyExactPreview(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	fixtures := executionTestFixtures(now)
	// The exact order is a limit order, so this flag does not prohibit it.
	fixtures["/api/v3/brokerage/products/BTC-USD"] = strings.Replace(fixtures["/api/v3/brokerage/products/BTC-USD"], `"limit_only":false`, `"limit_only":true`, 1)
	c, credentials, requests := executionTestClient(t, fixtures)
	original := *credentials
	got, err := c.CollectExecutionPreflight(context.Background(), credentials, executionTestOrder(), executionTestPortfolio)
	if err != nil {
		t.Fatal(err)
	}
	if got.CashUSD != "1000.1" || got.AvailableCashUSD != "1000.1" || got.TotalBase != "1.00000001" || got.AvailableBase != "1.00000001" ||
		got.RequestDigest != executionTestOrder().RequestDigest || got.PortfolioID != executionTestPortfolio || !got.CanView || !got.CanTrade || got.CanTransfer ||
		got.PriceIncrement != "0.01" || got.PreviewBaseSize != "0.0010" || got.PreviewFeeUSD != "0.10" || got.PreviewTotalUSD != "30.10" ||
		!got.StartedAt.Equal(now) || !got.CompletedAt.Equal(now) || !got.QuoteObservedAt.Equal(now) {
		t.Fatalf("unexpected evidence: %#v", got)
	}
	if !reflect.DeepEqual(*credentials, original) {
		t.Fatal("collector mutated credentials or cached provider permission")
	}
	wantRequests := []string{"GET /api/v3/brokerage/key_permissions", "GET /api/v3/brokerage/accounts", "GET /api/v3/brokerage/accounts", "GET /api/v3/brokerage/products/BTC-USD", "GET /api/v3/brokerage/product_book", "POST /api/v3/brokerage/orders/preview"}
	if !reflect.DeepEqual(*requests, wantRequests) {
		t.Fatalf("unexpected request sequence: %#v", *requests)
	}
}

func TestExecutionPreflightRejectsIncompleteOrUnsafeProviderEvidence(t *testing.T) {
	const permissions = "/api/v3/brokerage/key_permissions"
	const accounts = "/api/v3/brokerage/accounts"
	const secondPage = "/api/v3/brokerage/accounts?page-two"
	const product = "/api/v3/brokerage/products/BTC-USD"
	const book = "/api/v3/brokerage/product_book"
	const preview = "/api/v3/brokerage/orders/preview"
	now := time.Unix(1_800_000_000, 0).UTC()
	cases := []struct{ name, path, old, replacement string }{
		{"missing trade permission", permissions, `"can_trade":true,`, ``},
		{"missing transfer permission", permissions, `"can_transfer":false,`, ``},
		{"null view permission", permissions, `"can_view":true`, `"can_view":null`},
		{"view disabled", permissions, `"can_view":true`, `"can_view":false`},
		{"trade disabled", permissions, `"can_trade":true`, `"can_trade":false`},
		{"transfer enabled", permissions, `"can_transfer":false`, `"can_transfer":true`},
		{"foreign key portfolio", permissions, executionTestPortfolio, "10000000-0000-4000-8000-000000000099"},
		{"duplicate permission", permissions, `"can_transfer":false`, `"can_transfer":true,"can_transfer":false`},
		{"case alias duplicate permission", permissions, `"can_transfer":false`, `"CAN_TRANSFER":true,"can_transfer":false`},
		{"unicode alias duplicate permission", permissions, `"can_transfer":false`, `"can_transfer":true,"can_tran\u017ffer":false`},
		{"missing account completion", accounts, `"has_next":true,`, ``},
		{"missing cursor", accounts, `,"cursor":"page-two"`, ``},
		{"cycling cursor", secondPage, `"has_next":false,"cursor":""`, `"has_next":true,"cursor":"page-two"`},
		{"foreign account portfolio", accounts, executionTestPortfolio, "10000000-0000-4000-8000-000000000099"},
		{"inactive account", accounts, `"active":true`, `"active":false`},
		{"unready account", accounts, `"ready":true`, `"ready":false`},
		{"missing ready", accounts, `"ready":true,`, ``},
		{"missing hold", accounts, `"hold":{"value":"0","currency":"USD"},`, ``},
		{"nonzero hold", accounts, `"hold":{"value":"0"`, `"hold":{"value":"0.01"`},
		{"currency mismatch", accounts, `"value":"1000.10","currency":"USD"`, `"value":"1000.10","currency":"BTC"`},
		{"negative availability", accounts, `"1000.10"`, `"-1000.10"`},
		{"exponent availability", accounts, `"1000.10"`, `1e3`},
		{"missing availability", accounts, `"value":"1000.10",`, ``},
		{"null availability", accounts, `"1000.10"`, `null`},
		{"foreign nonzero asset", secondPage, `"BTC"`, `"ETH"`},
		{"duplicate account identity", secondPage, "20000000-0000-4000-8000-000000000002", "20000000-0000-4000-8000-000000000001"},
		{"missing price increment", product, `"price_increment":"0.01",`, ``},
		{"zero price increment", product, `"price_increment":"0.01"`, `"price_increment":"0"`},
		{"missing disabled flag", product, `"is_disabled":false,`, ``},
		{"missing limit only flag", product, `"limit_only":false,`, ``},
		{"null disabled flag", product, `"is_disabled":false`, `"is_disabled":null`},
		{"unicode alias disabled flag", product, `"is_disabled":false`, `"is_disabled":true,"i\u017f_disabled":false`},
		{"disabled product", product, `"trading_disabled":false`, `"trading_disabled":true`},
		{"auction product", product, `"auction_mode":false`, `"auction_mode":true`},
		{"post only product", product, `"post_only":false`, `"post_only":true`},
		{"view only product", product, `"view_only":false`, `"view_only":true`},
		{"cancel only product", product, `"cancel_only":false`, `"cancel_only":true`},
		{"offline product", product, `"online"`, `"offline"`},
		{"wrong product type", product, `"SPOT"`, `"FUTURE"`},
		{"wrong product", product, `"BTC-USD"`, `"ETH-USD"`},
		{"wrong book product", book, `"BTC-USD"`, `"ETH-USD"`},
		{"stale book", book, now.Format(time.RFC3339Nano), now.Add(-11 * time.Second).Format(time.RFC3339Nano)},
		{"future book", book, now.Format(time.RFC3339Nano), now.Add(time.Second).Format(time.RFC3339Nano)},
		{"missing book time", book, `,"time":"` + now.Format(time.RFC3339Nano) + `"`, ``},
		{"empty book", book, `[{"price":"29999.99","size":"1"}]`, `[]`},
		{"zero book size", book, `"size":"1"`, `"size":"0"`},
		{"crossed book", book, `"29999.99"`, `"30001"`},
		{"preview errors", preview, `"errs":[]`, `"errs":["UNKNOWN"]`},
		{"preview warnings", preview, `"warning":[]`, `"warning":["UNKNOWN"]`},
		{"missing preview errors", preview, `"errs":[],`, ``},
		{"null preview warnings", preview, `"warning":[]`, `"warning":null`},
		{"missing max flag", preview, `,"is_max":false`, ``},
		{"max preview", preview, `"is_max":false`, `"is_max":true`},
		{"changed size preview", preview, `"base_size":"0.0010"`, `"base_size":"0.0020"`},
		{"missing fee", preview, `"commission_total":"0.10",`, ``},
		{"excess fee", preview, `"commission_total":"0.10"`, `"commission_total":"0.10000001"`},
		{"zero quote", preview, `"quote_size":"30.00"`, `"quote_size":"0"`},
		{"invalid preview ID", preview, `"30000000-0000-4000-8000-000000000001"`, `""`},
		{"leveraged preview", preview, `"is_max":false`, `"is_max":false,"leverage":"2"`},
		{"null leverage", preview, `"is_max":false`, `"is_max":false,"leverage":null`},
		{"margin preview", preview, `"is_max":false`, `"is_max":false,"margin_ratio":"0.1"`},
		{"null margin", preview, `"is_max":false`, `"is_max":false,"margin_ratio":null`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixtures := executionTestFixtures(now)
			if !strings.Contains(fixtures[tc.path], tc.old) {
				t.Fatal("invalid mutation fixture")
			}
			fixtures[tc.path] = strings.ReplaceAll(fixtures[tc.path], tc.old, tc.replacement)
			c, credentials, _ := executionTestClient(t, fixtures)
			got, err := c.CollectExecutionPreflight(context.Background(), credentials, executionTestOrder(), executionTestPortfolio)
			var providerErr *financial.ProviderError
			if !errors.As(err, &providerErr) || !reflect.DeepEqual(got, execution.ProviderPreflight{}) {
				t.Fatalf("unsafe evidence did not fail closed: %#v %v", got, err)
			}
		})
	}
}

func TestExecutionPreflightAcceptsZeroForeignInventoryAndExplicitSpotFields(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	fixtures := executionTestFixtures(now)
	fixtures["/api/v3/brokerage/accounts?page-two"] = strings.ReplaceAll(strings.ReplaceAll(fixtures["/api/v3/brokerage/accounts?page-two"], `"BTC"`, `"ETH"`), `"1.00000001"`, `0`)
	fixtures["/api/v3/brokerage/orders/preview"] = strings.Replace(fixtures["/api/v3/brokerage/orders/preview"], `"is_max":false`, `"is_max":false,"leverage":"1.0","margin_ratio":0`, 1)
	c, credentials, _ := executionTestClient(t, fixtures)
	got, err := c.CollectExecutionPreflight(context.Background(), credentials, executionTestOrder(), executionTestPortfolio)
	if err != nil || got.AvailableBase != "0" || got.TotalBase != "0" {
		t.Fatalf("fully observed zero foreign inventory was not accepted: %#v %v", got, err)
	}
}

func TestExecutionPreflightSellRemainsExactBaseSizedLimitIOC(t *testing.T) {
	fixtures := executionTestFixtures(time.Unix(1_800_000_000, 0).UTC())
	fixtures["/api/v3/brokerage/orders/preview"] = strings.Replace(fixtures["/api/v3/brokerage/orders/preview"], `"order_total":"30.10"`, `"order_total":"29.90"`, 1)
	order := executionTestOrder()
	order.Request.Side, order.Request.LimitPrice, order.Request.MaximumDebitUSD = "SELL", "29999.99", "0"
	c, credentials, _ := executionTestClient(t, fixtures, order)
	got, err := c.CollectExecutionPreflight(context.Background(), credentials, order, executionTestPortfolio)
	if err != nil || got.PreviewTotalUSD != "29.90" || got.PreviewBaseSize != "0.0010" {
		t.Fatalf("exact sell preview rejected: %#v %v", got, err)
	}
}

func TestExecutionPreflightRejectsPaginationLimitWithoutPartialEvidence(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	fixtures := executionTestFixtures(now)
	first := fixtures["/api/v3/brokerage/accounts"]
	fixtures["/api/v3/brokerage/accounts"] = strings.Replace(first, "page-two", "page-1", 1)
	for i := 1; i < maxPages; i++ {
		body := strings.Replace(first, "20000000-0000-4000-8000-000000000001", fmt.Sprintf("20000000-0000-4000-8000-%012d", i+1), 1)
		fixtures[fmt.Sprintf("/api/v3/brokerage/accounts?page-%d", i)] = strings.Replace(body, "page-two", fmt.Sprintf("page-%d", i+1), 1)
	}
	c, credentials, requests := executionTestClient(t, fixtures)
	got, err := c.CollectExecutionPreflight(context.Background(), credentials, executionTestOrder(), executionTestPortfolio)
	if err == nil || !reflect.DeepEqual(got, execution.ProviderPreflight{}) || len(*requests) != 1+maxPages {
		t.Fatalf("incomplete pagination escaped bound: %#v %v %d", got, err, len(*requests))
	}
}

func TestExecutionPreflightRejectsUnsafeInputBeforeProviderCall(t *testing.T) {
	mutations := []func(*execution.Order, *financial.Credentials){
		func(o *execution.Order, _ *financial.Credentials) { o.Request.ProductID = "BTC-USD/../orders" },
		func(o *execution.Order, _ *financial.Credentials) { o.Request.Side = "buy" },
		func(o *execution.Order, _ *financial.Credentials) { o.Request.BaseSize = "1e3" },
		func(o *execution.Order, _ *financial.Credentials) { o.Request.LimitPrice = "0" },
		func(o *execution.Order, _ *financial.Credentials) { o.Request.FeeAllowanceUSD = "-1" },
		func(o *execution.Order, _ *financial.Credentials) { o.RequestDigest = "" },
		func(_ *execution.Order, c *financial.Credentials) { c.PortfolioID = "" },
	}
	for i, mutate := range mutations {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			c, credentials, requests := executionTestClient(t, nil)
			o := executionTestOrder()
			mutate(&o, credentials)
			if _, err := c.CollectExecutionPreflight(context.Background(), credentials, o, executionTestPortfolio); err == nil || len(*requests) != 0 {
				t.Fatalf("invalid request reached provider: %v %#v", err, *requests)
			}
		})
	}
}

func TestExecutionPreflightHTTPRejectsRedirectsAndMalformedBodies(t *testing.T) {
	for _, body := range []string{
		`{"ok":true,"ok":false}`, `{"nested":{"a":1,"a":2}}`, `{"nested":{"a":1,"A":2}}`, `{"a":1} {"b":2}`,
		`{"a":1}` + strings.Repeat(" ", maxBodyBytes), strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34),
	} {
		t.Run(fmt.Sprintf("body-%d-%d", len(body), strings.Count(body, "[")), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			c, _ := New(Config{BaseURL: server.URL}, server.Client())
			credentials, _ := testCredentials(t)
			var out map[string]any
			if err := c.executionRequest(context.Background(), &credentials, http.MethodGet, "/api/v3/brokerage/key_permissions", nil, &out); err == nil {
				t.Fatal("unsafe response body accepted")
			}
		})
	}
	t.Run("redirect never followed including same origin", func(t *testing.T) {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			http.Redirect(w, r, "/redirect-target", http.StatusTemporaryRedirect)
		}))
		defer server.Close()
		client := server.Client()
		originalRedirectCalled := false
		client.CheckRedirect = func(*http.Request, []*http.Request) error { originalRedirectCalled = true; return nil }
		c, _ := New(Config{BaseURL: server.URL}, client)
		credentials, _ := testCredentials(t)
		var out map[string]any
		if err := c.executionRequest(context.Background(), &credentials, http.MethodGet, "/api/v3/brokerage/key_permissions", nil, &out); err == nil || requests != 1 || originalRedirectCalled {
			t.Fatalf("redirect policy failed: %v calls=%d original=%v", err, requests, originalRedirectCalled)
		}
		if client.CheckRedirect == nil {
			t.Fatal("original shared client's redirect policy mutated")
		}
	})
}

func TestExecutionPreflightHTTPHasNoSubmissionOrCancellationRoute(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	c, _ := New(Config{BaseURL: server.URL}, server.Client())
	credentials, _ := testCredentials(t)
	for _, path := range []string{"/api/v3/brokerage/orders", "/api/v3/brokerage/orders/batch_cancel", "/api/v3/brokerage/orders/edit", "https://example.invalid/api/v3/brokerage/orders/preview"} {
		if err := c.executionRequest(context.Background(), &credentials, http.MethodPost, path, struct{}{}, &struct{}{}); err == nil {
			t.Fatalf("write endpoint accepted: %s", path)
		}
	}
	if requests != 0 {
		t.Fatalf("forbidden endpoints reached transport: %d", requests)
	}
}
