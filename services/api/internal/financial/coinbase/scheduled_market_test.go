package coinbase

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/execution"
)

const (
	scheduledMarketProductPath = "/api/v3/brokerage/market/products/BTC-USD"
	scheduledMarketBookPath    = "/api/v3/brokerage/market/product_book?limit=1&product_id=BTC-USD"
)

func scheduledMarketTestScope() ScheduledMarketScope {
	return ScheduledMarketScope{
		OwnerID: "10000000-0000-4000-8000-000000000001", AccountID: "20000000-0000-4000-8000-000000000001",
		ProductID: "BTC-USD", FeeAllowanceUSD: "0.10",
	}
}

func scheduledMarketFixtures(now time.Time) map[string]string {
	fixtures := executionTestFixtures(now)
	return map[string]string{
		scheduledMarketProductPath: fixtures["/api/v3/brokerage/products/BTC-USD"],
		scheduledMarketBookPath:    fixtures["/api/v3/brokerage/product_book"],
	}
}

func scheduledMarketTestCollector(t *testing.T, now time.Time, fixtures map[string]string) (*ScheduledMarketCollector, *[]string) {
	t.Helper()
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) != 0 || r.Method != http.MethodGet || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" ||
			r.Header.Get("Accept") != "application/json" || r.Header.Get("Cache-Control") != "no-cache" || r.URL.User != nil {
			t.Errorf("public evidence request carried authority/body or lacked cache controls: %s %s headers=%v body=%q error=%v", r.Method, r.URL.RequestURI(), r.Header, body, err)
		}
		encoded, ok := fixtures[r.URL.RequestURI()]
		if !ok {
			t.Errorf("unexpected public evidence endpoint or query: %s", r.URL.RequestURI())
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, encoded)
	}))
	t.Cleanup(server.Close)
	c, err := NewScheduledMarketCollector(scheduledMarketTestScope())
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c.base, c.http, c.now = *u, server.Client(), func() time.Time { return now }
	return c, &requests
}

func collectScheduledMarket(c *ScheduledMarketCollector, ctx context.Context) (execution.ScheduledGenerationMarket, error) {
	s := scheduledMarketTestScope()
	return c.CollectScheduledGenerationMarket(ctx, s.OwnerID, s.AccountID, s.ProductID)
}

func requireNoScheduledMarket(t *testing.T, got execution.ScheduledGenerationMarket, err error) {
	t.Helper()
	if err == nil || !reflect.DeepEqual(got, execution.ScheduledGenerationMarket{}) {
		t.Fatalf("failed evidence collection did not return an error and a zero snapshot: %#v %v", got, err)
	}
}

func TestScheduledMarketCollectorUsesOnlyFreshPublicEvidence(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	fixtures := scheduledMarketFixtures(now)
	fixtures[scheduledMarketProductPath] = strings.Replace(fixtures[scheduledMarketProductPath], `"limit_only":false`, `"limit_only":true`, 1)
	c, requests := scheduledMarketTestCollector(t, now, fixtures)
	// A shared client must not add cookies or relax the collector's own redirect policy.
	c.http.Jar, _ = cookiejar.New(nil)
	c.http.Jar.SetCookies(&c.base, []*http.Cookie{{Name: "session", Value: "synthetic-no-authority"}})
	c.http.CheckRedirect = func(*http.Request, []*http.Request) error { return nil }
	got, err := collectScheduledMarket(c, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := execution.ScheduledGenerationMarket{
		ProductID: "BTC-USD", ProductType: "SPOT", BaseCurrency: "BTC", QuoteCurrency: "USD", Status: "online",
		BaseIncrement: "0.00000001", PriceIncrement: "0.01", BaseMinSize: "0.00001", BaseMaxSize: "100", QuoteMinSize: "1", QuoteMaxSize: "1000000000",
		BestBid: "29999.99", BestAsk: "30000.00", FeeAllowanceUSD: "0.10", StartedAt: now, CompletedAt: now, ObservedAt: now,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalized public market evidence: got %#v want %#v", got, want)
	}
	next := now.Add(time.Second)
	c.now = func() time.Time { return next }
	fixtures[scheduledMarketProductPath] = strings.Replace(fixtures[scheduledMarketProductPath], `"base_max_size":"100"`, `"base_max_size":"50"`, 1)
	fixtures[scheduledMarketBookPath] = strings.NewReplacer("29999.99", "29998.99", "30000.00", "29999.00", now.Format(time.RFC3339Nano), next.Format(time.RFC3339Nano)).Replace(fixtures[scheduledMarketBookPath])
	got, err = collectScheduledMarket(c, context.Background())
	if err != nil || got.BaseMaxSize != "50" || got.BestBid != "29998.99" || got.BestAsk != "29999.00" ||
		!got.StartedAt.Equal(next) || !got.CompletedAt.Equal(next) || !got.ObservedAt.Equal(next) {
		t.Fatalf("second collection did not independently read fresh product and book: %#v %v", got, err)
	}
	wantRequests := []string{"GET " + scheduledMarketProductPath, "GET " + scheduledMarketBookPath, "GET " + scheduledMarketProductPath, "GET " + scheduledMarketBookPath}
	if !reflect.DeepEqual(*requests, wantRequests) {
		t.Fatalf("unexpected public evidence request sequence: %#v", *requests)
	}
}

func TestScheduledMarketCollectorConstructorRejectsInvalidScope(t *testing.T) {
	for _, field := range []string{"owner", "account", "product", "fee"} {
		values := map[string][]string{
			"owner":   {"", "not-a-uuid", "00000000-0000-0000-0000-000000000000", "10000000-0000-4000-8000-00000000000A"},
			"account": {"", "not-a-uuid", "00000000-0000-0000-0000-000000000000"},
			"product": {"", "btc-USD", "BTC-usd", "USD-USD", "BTC-EUR", "BTC-USD/../orders", "BTC-USD?limit=2", "BTC--USD", "1BTC-USD", "ABCDEFGHIJKLMNOPQ-USD"},
			"fee":     {"", " ", "-0.01", "+1", "01", ".1", "1.", "1e-2", "NaN", "Infinity", "0.0000000000000000001", "1000000000000000000"},
		}[field]
		for _, value := range values {
			t.Run(field+"="+value, func(t *testing.T) {
				scope := scheduledMarketTestScope()
				switch field {
				case "owner":
					scope.OwnerID = value
				case "account":
					scope.AccountID = value
				case "product":
					scope.ProductID = value
				case "fee":
					scope.FeeAllowanceUSD = value
				}
				if c, err := NewScheduledMarketCollector(scope); err == nil || c != nil {
					t.Fatalf("invalid server scope constructed a collector: %#v %v", c, err)
				}
			})
		}
	}
	for _, fee := range []string{"0", "0.00", "0.000000000000000001", "999999999999999999.999999999999999999"} {
		scope := scheduledMarketTestScope()
		scope.FeeAllowanceUSD = fee
		c, err := NewScheduledMarketCollector(scope)
		if err != nil || c == nil || c.scope.FeeAllowanceUSD != fee {
			t.Fatalf("explicit nonnegative decimal fee was not retained: %#v %v", c, err)
		}
	}
}

func TestScheduledMarketCollectorOwnsProductionTransportAndCopiesScope(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	oldTransport, oldClient := http.DefaultTransport, http.DefaultClient
	t.Cleanup(func() { http.DefaultTransport, http.DefaultClient = oldTransport, oldClient })
	trap := &submissionRetryTransport{}
	http.DefaultTransport, http.DefaultClient = trap, &http.Client{Transport: trap}
	scope := scheduledMarketTestScope()
	c, err := NewScheduledMarketCollector(scope)
	if err != nil {
		t.Fatal(err)
	}
	scope.OwnerID, scope.AccountID, scope.ProductID, scope.FeeAllowanceUSD = "changed", "changed", "ETH-USD", "999"
	if c.scope != scheduledMarketTestScope() || c.base.String() != "https://api.coinbase.com" || c.http == http.DefaultClient ||
		c.http.Jar != nil || c.http.Timeout != 10*time.Second || c.now == nil || c.http.CheckRedirect == nil {
		t.Fatal("collector copied authority incorrectly or inherited external destination/client configuration")
	}
	assertProductionTransport(t, c.http.Transport)
	if err := c.http.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatal("production collector permits redirects")
	}
	other, err := NewScheduledMarketCollector(scheduledMarketTestScope())
	if err != nil {
		t.Fatal(err)
	}
	if other.http == c.http || other.http.Transport == c.http.Transport ||
		other.http.Transport.(*http.Transport).TLSClientConfig == c.http.Transport.(*http.Transport).TLSClientConfig || trap.calls != 0 {
		t.Fatal("collector construction performed I/O or shared mutable transport/TLS state")
	}
}

func TestScheduledMarketCollectorRejectsMismatchedScopeBeforeAnyRequest(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	c, requests := scheduledMarketTestCollector(t, now, scheduledMarketFixtures(now))
	for _, mutate := range []func(*ScheduledMarketScope){
		func(s *ScheduledMarketScope) { s.OwnerID = "10000000-0000-4000-8000-000000000002" },
		func(s *ScheduledMarketScope) { s.AccountID = "20000000-0000-4000-8000-000000000002" },
		func(s *ScheduledMarketScope) { s.ProductID = "ETH-USD" },
		func(s *ScheduledMarketScope) { s.OwnerID = "" },
		func(s *ScheduledMarketScope) { s.AccountID = "" },
		func(s *ScheduledMarketScope) { s.ProductID = "BTC-USD/../orders" },
	} {
		s := scheduledMarketTestScope()
		mutate(&s)
		got, err := c.CollectScheduledGenerationMarket(context.Background(), s.OwnerID, s.AccountID, s.ProductID)
		requireNoScheduledMarket(t, got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := collectScheduledMarket(c, ctx)
	requireNoScheduledMarket(t, got, err)
	if len(*requests) != 0 {
		t.Fatalf("mismatched scope or canceled caller caused provider I/O: %#v", *requests)
	}
}

func TestScheduledMarketCollectorRequiresEveryProductField(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(scheduledMarketFixtures(now)[scheduledMarketProductPath]), &fields); err != nil {
		t.Fatal(err)
	}
	for field := range fields {
		for _, variant := range []string{"missing", "null", "wrong type"} {
			t.Run(field+"/"+variant, func(t *testing.T) {
				fixtures := scheduledMarketFixtures(now)
				var product map[string]json.RawMessage
				if err := json.Unmarshal([]byte(fixtures[scheduledMarketProductPath]), &product); err != nil {
					t.Fatal(err)
				}
				switch variant {
				case "missing":
					delete(product, field)
				case "null":
					product[field] = json.RawMessage(`null`)
				case "wrong type":
					product[field] = json.RawMessage(`[]`)
				}
				body, err := json.Marshal(product)
				if err != nil {
					t.Fatal(err)
				}
				fixtures[scheduledMarketProductPath] = string(body)
				c, _ := scheduledMarketTestCollector(t, now, fixtures)
				got, err := collectScheduledMarket(c, context.Background())
				requireNoScheduledMarket(t, got, err)
			})
		}
	}
}

func TestScheduledMarketCollectorRejectsUnsafeProductAndBook(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	cases := []struct{ name, path, old, replacement string }{
		{"wrong product", scheduledMarketProductPath, `"product_id":"BTC-USD"`, `"product_id":"ETH-USD"`},
		{"wrong type", scheduledMarketProductPath, `"SPOT"`, `"FUTURE"`},
		{"wrong base", scheduledMarketProductPath, `"base_currency_id":"BTC"`, `"base_currency_id":"ETH"`},
		{"wrong quote", scheduledMarketProductPath, `"quote_currency_id":"USD"`, `"quote_currency_id":"EUR"`},
		{"offline", scheduledMarketProductPath, `"online"`, `"offline"`},
		{"reversed base sizes", scheduledMarketProductPath, `"base_min_size":"0.00001"`, `"base_min_size":"101"`},
		{"reversed quote sizes", scheduledMarketProductPath, `"quote_min_size":"1"`, `"quote_min_size":"1000000001"`},
		{"missing pricebook", scheduledMarketBookPath, scheduledMarketFixtures(now)[scheduledMarketBookPath], `{}`},
		{"null pricebook", scheduledMarketBookPath, scheduledMarketFixtures(now)[scheduledMarketBookPath], `{"pricebook":null}`},
		{"wrong book product", scheduledMarketBookPath, `"BTC-USD"`, `"ETH-USD"`},
		{"missing book product", scheduledMarketBookPath, `"product_id":"BTC-USD",`, ``},
		{"null book product", scheduledMarketBookPath, `"BTC-USD"`, `null`},
		{"missing book time", scheduledMarketBookPath, `,"time":"` + now.Format(time.RFC3339Nano) + `"`, ``},
		{"null book time", scheduledMarketBookPath, `"time":"` + now.Format(time.RFC3339Nano) + `"`, `"time":null`},
		{"invalid book time", scheduledMarketBookPath, now.Format(time.RFC3339Nano), `not-a-time`},
		{"zero book time", scheduledMarketBookPath, now.Format(time.RFC3339Nano), time.Time{}.Format(time.RFC3339Nano)},
		{"stale book", scheduledMarketBookPath, now.Format(time.RFC3339Nano), now.Add(-10*time.Second - time.Nanosecond).Format(time.RFC3339Nano)},
		{"future book", scheduledMarketBookPath, now.Format(time.RFC3339Nano), now.Add(time.Nanosecond).Format(time.RFC3339Nano)},
		{"crossed book", scheduledMarketBookPath, `"29999.99"`, `"30000.01"`},
		{"off tick bid", scheduledMarketBookPath, `"29999.99"`, `"29999.999"`},
		{"off tick ask", scheduledMarketBookPath, `"30000.00"`, `"30000.001"`},
		{"duplicate product field", scheduledMarketProductPath, `"is_disabled":false`, `"is_disabled":true,"is_disabled":false`},
		{"case duplicate product field", scheduledMarketProductPath, `"is_disabled":false`, `"IS_DISABLED":true,"is_disabled":false`},
		{"unicode duplicate product field", scheduledMarketProductPath, `"is_disabled":false`, `"is_disabled":true,"i\u017f_disabled":false`},
		{"duplicate nested book price", scheduledMarketBookPath, `"price":"29999.99"`, `"price":"1","price":"29999.99"`},
		{"case duplicate nested book price", scheduledMarketBookPath, `"price":"29999.99"`, `"PRICE":"1","price":"29999.99"`},
	}
	for _, flag := range []string{"is_disabled", "trading_disabled", "cancel_only", "post_only", "auction_mode", "view_only"} {
		cases = append(cases, struct{ name, path, old, replacement string }{flag, scheduledMarketProductPath, `"` + flag + `":false`, `"` + flag + `":true`})
	}
	for _, field := range []string{"base_increment", "price_increment", "base_min_size", "base_max_size", "quote_min_size", "quote_max_size"} {
		var product map[string]json.RawMessage
		if err := json.Unmarshal([]byte(scheduledMarketFixtures(now)[scheduledMarketProductPath]), &product); err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{`"0"`, `"-1"`, `"1e-2"`, `1e-2`, `"01"`, `"0.0000000000000000001"`} {
			cases = append(cases, struct{ name, path, old, replacement string }{field + "=" + value, scheduledMarketProductPath, `"` + field + `":` + string(product[field]), `"` + field + `":` + value})
		}
	}
	for _, side := range []string{"bids", "asks"} {
		price := "29999.99"
		if side == "asks" {
			price = "30000.00"
		}
		original := `"` + side + `":[{"price":"` + price + `","size":"1"}]`
		for _, value := range []string{`null`, `[]`, `[null]`, `[{"price":"` + price + `","size":"1"},{"price":"` + price + `","size":"1"}]`,
			`[{"size":"1"}]`, `[{"price":null,"size":"1"}]`, `[{"price":"0","size":"1"}]`, `[{"price":"-1","size":"1"}]`,
			`[{"price":"` + price + `"}]`, `[{"price":"` + price + `","size":null}]`, `[{"price":"` + price + `","size":"0"}]`, `[{"price":"` + price + `","size":"-1"}]`} {
			cases = append(cases, struct{ name, path, old, replacement string }{side + "=" + value, scheduledMarketBookPath, original, `"` + side + `":` + value})
		}
		cases = append(cases, struct{ name, path, old, replacement string }{side + " missing", scheduledMarketBookPath, original + ",", ""})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixtures := scheduledMarketFixtures(now)
			if !strings.Contains(fixtures[tc.path], tc.old) {
				t.Fatal("invalid mutation fixture")
			}
			fixtures[tc.path] = strings.Replace(fixtures[tc.path], tc.old, tc.replacement, 1)
			c, _ := scheduledMarketTestCollector(t, now, fixtures)
			got, err := collectScheduledMarket(c, context.Background())
			requireNoScheduledMarket(t, got, err)
		})
	}
}

func TestScheduledMarketCollectorRejectsMalformedAndOversizedBodies(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	for _, path := range []string{scheduledMarketProductPath, scheduledMarketBookPath} {
		for _, variant := range []string{"empty", "null", "array", "invalid", "trailing JSON", "trailing junk", "oversized whitespace"} {
			t.Run(path+"/"+variant, func(t *testing.T) {
				fixtures := scheduledMarketFixtures(now)
				switch variant {
				case "empty":
					fixtures[path] = ""
				case "null":
					fixtures[path] = "null"
				case "array":
					fixtures[path] = "[]"
				case "invalid":
					fixtures[path] = "{"
				case "trailing JSON":
					fixtures[path] += " {}"
				case "trailing junk":
					fixtures[path] += " unexpected"
				case "oversized whitespace":
					fixtures[path] += strings.Repeat(" ", maxBodyBytes)
				}
				c, _ := scheduledMarketTestCollector(t, now, fixtures)
				got, err := collectScheduledMarket(c, context.Background())
				requireNoScheduledMarket(t, got, err)
			})
		}
	}
}

func TestScheduledMarketRequestRestrictsExactAllowlistedPaths(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	c, requests := scheduledMarketTestCollector(t, now, scheduledMarketFixtures(now))
	for _, path := range []string{
		"/api/v3/brokerage/accounts", "/api/v3/brokerage/key_permissions", "/api/v3/brokerage/products/BTC-USD", "/api/v3/brokerage/orders/preview", "/api/v3/brokerage/orders", "/api/v3/brokerage/orders/batch_cancel",
		"/api/v3/brokerage/market/products/ETH-USD", scheduledMarketProductPath + "/extra", scheduledMarketProductPath + "?owner_id=private", scheduledMarketProductPath + "#fragment",
		"/api/v3/brokerage/market/product_book", scheduledMarketBookPath + "&owner_id=private", scheduledMarketBookPath + "&limit=2",
		"/api/v3/brokerage/market/product_book?limit=2&product_id=BTC-USD", "/api/v3/brokerage/market/product_book?limit=1&product_id=ETH-USD",
		"/api/v3/brokerage/market/products/BTC%2DUSD", "https://api.coinbase.com" + scheduledMarketProductPath, "//example.invalid" + scheduledMarketProductPath,
	} {
		t.Run(path, func(t *testing.T) {
			var output executionProduct
			if err := c.publicMarketRequest(context.Background(), path, &output); err == nil || !reflect.DeepEqual(output, executionProduct{}) {
				t.Fatalf("request helper accepted an unallowlisted route: %s %#v %v", path, output, err)
			}
		})
	}
	if len(*requests) != 0 {
		t.Fatalf("unallowlisted helper input performed provider I/O: %#v", *requests)
	}
}

func TestScheduledMarketCollectorFreshnessIsCheckedAtCompletion(t *testing.T) {
	started := time.Unix(1_800_000_000, 0).UTC()
	cases := []struct {
		name      string
		duration  time.Duration
		age       time.Duration
		wantError bool
	}{
		{"exact limits", 30 * time.Second, 10 * time.Second, false},
		{"aggregate over limit", 30*time.Second + time.Nanosecond, 0, true},
		{"quote stale at completion", 10*time.Second + time.Nanosecond, 10*time.Second + time.Nanosecond, true},
		{"backward clock", -time.Nanosecond, 0, true},
		{"future quote at completion", time.Second, -time.Nanosecond, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			completed := started.Add(tc.duration)
			fixtures := scheduledMarketFixtures(completed.Add(-tc.age))
			var current atomic.Int64
			current.Store(started.UnixNano())
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.RequestURI() == scheduledMarketBookPath {
					current.Store(completed.UnixNano())
				}
				_, _ = io.WriteString(w, fixtures[r.URL.RequestURI()])
			}))
			defer server.Close()
			c, err := NewScheduledMarketCollector(scheduledMarketTestScope())
			if err != nil {
				t.Fatal(err)
			}
			u, _ := url.Parse(server.URL)
			c.base, c.http, c.now = *u, server.Client(), func() time.Time { return time.Unix(0, current.Load()).UTC() }
			got, err := collectScheduledMarket(c, context.Background())
			if tc.wantError {
				requireNoScheduledMarket(t, got, err)
			} else if err != nil || !got.StartedAt.Equal(started) || !got.CompletedAt.Equal(completed) || !got.ObservedAt.Equal(completed.Add(-tc.age)) {
				t.Fatalf("bounded completed evidence was not accepted: %#v %v", got, err)
			}
		})
	}
}

func TestScheduledMarketCollectorProviderFailuresDoNotLeakPartialEvidence(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	for _, failPath := range []string{scheduledMarketProductPath, scheduledMarketBookPath} {
		for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusNoContent, http.StatusFound} {
			t.Run(failPath+"/"+http.StatusText(status), func(t *testing.T) {
				fixtures := scheduledMarketFixtures(now)
				var redirectCalls atomic.Int32
				redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirectCalls.Add(1) }))
				defer redirectTarget.Close()
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.URL.RequestURI() == failPath {
						w.Header().Set("Location", redirectTarget.URL)
						w.WriteHeader(status)
						return
					}
					_, _ = io.WriteString(w, fixtures[r.URL.RequestURI()])
				}))
				defer server.Close()
				c, err := NewScheduledMarketCollector(scheduledMarketTestScope())
				if err != nil {
					t.Fatal(err)
				}
				u, _ := url.Parse(server.URL)
				c.base, c.http, c.now = *u, server.Client(), func() time.Time { return now }
				c.http.CheckRedirect = func(*http.Request, []*http.Request) error { return nil }
				got, err := collectScheduledMarket(c, context.Background())
				requireNoScheduledMarket(t, got, err)
				wantCalls := int32(1)
				if failPath == scheduledMarketBookPath {
					wantCalls = 2
				}
				if calls.Load() != wantCalls || redirectCalls.Load() != 0 {
					t.Fatalf("failed public evidence request retried, continued, or redirected: requests=%d redirects=%d", calls.Load(), redirectCalls.Load())
				}
			})
		}
	}
}

func TestScheduledMarketCollectorCancellationAndTransportFailure(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	fixtures := scheduledMarketFixtures(now)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		cancel()
		_, _ = io.WriteString(w, fixtures[r.URL.RequestURI()])
	}))
	c, err := NewScheduledMarketCollector(scheduledMarketTestScope())
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(server.URL)
	c.base, c.http, c.now = *u, server.Client(), func() time.Time { return now }
	got, err := collectScheduledMarket(c, ctx)
	requireNoScheduledMarket(t, got, err)
	if calls.Load() != 1 {
		t.Fatalf("cancellation permitted another provider request: %d", calls.Load())
	}
	server.Close()
	got, err = collectScheduledMarket(c, context.Background())
	requireNoScheduledMarket(t, got, err)
}
