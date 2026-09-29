package coinbase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/financial"
)

const submissionTestProviderID = "40000000-0000-4000-8000-000000000001"

func submissionFixture(now time.Time, side string) (execution.ConfirmedSubmission, execution.Attempt) {
	o := execution.Order{ID: "50000000-0000-4000-8000-000000000001", CreatedAt: now.Add(-time.Second), Request: executionTestOrder().Request}
	o.Request.OwnerID = "60000000-0000-4000-8000-000000000001"
	o.Request.AccountID = "60000000-0000-4000-8000-000000000002"
	o.Request.ConnectionID = "60000000-0000-4000-8000-000000000003"
	o.Request.CapitalBucketID = "60000000-0000-4000-8000-000000000004"
	o.Request.ClientOrderID = "60000000-0000-4000-8000-000000000005"
	o.Request.Side = side
	if side == "SELL" {
		o.Request.MaximumDebitUSD = "0"
	}
	encoded, _ := json.Marshal(o.Request)
	digest := sha256.Sum256(append([]byte("arbion-coinbase-limit-ioc-v1\x00"), encoded...))
	o.RequestDigest = hex.EncodeToString(digest[:])
	p := execution.ProviderPreflight{PortfolioID: executionTestPortfolio, RequestDigest: o.RequestDigest,
		ProductID: "BTC-USD", ProductType: "SPOT", BaseCurrency: "BTC", QuoteCurrency: "USD", Status: "online",
		BaseIncrement: "0.00000001", PriceIncrement: "0.01", BaseMinSize: "0.00001", BaseMaxSize: "100", QuoteMinSize: "1", QuoteMaxSize: "1000000000",
		BestBid: "30000", BestAsk: "30000", PreviewID: "30000000-0000-4000-8000-000000000001",
		PreviewBaseSize: "0.0010", PreviewQuoteSize: "30.00", PreviewFeeUSD: "0.10", PreviewTotalUSD: "30.10", PreviewPrice: "30000.00",
		CashUSD: "1000.10", AvailableCashUSD: "1000.10", TotalBase: "1.00000001", AvailableBase: "1.00000001",
		CanView: true, CanTrade: true, StartedAt: now.Add(-time.Millisecond), CompletedAt: now, QuoteObservedAt: now.Add(-time.Millisecond)}
	if side == "SELL" {
		p.PreviewTotalUSD = "29.90"
	}
	s := execution.ConfirmedSubmission{Order: o, PortfolioID: p.PortfolioID, PreviewID: p.PreviewID, Preflight: p}
	a := execution.Attempt{OrderID: o.ID, ClientOrderID: o.Request.ClientOrderID, RequestDigest: o.RequestDigest,
		AuthorizationID: "60000000-0000-4000-8000-000000000006", CredentialGeneration: 1, ClaimedAt: now, ExpiresAt: now.Add(time.Second)}
	return s, a
}

func submissionIdentityJSON(s execution.ConfirmedSubmission) string {
	return fmt.Sprintf(`{"order_id":%q,"client_order_id":%q,"product_id":%q,"side":%q}`, submissionTestProviderID, s.Order.Request.ClientOrderID, s.Order.Request.ProductID, s.Order.Request.Side)
}

func submissionDetailJSON(s execution.ConfirmedSubmission, created time.Time) string {
	identity := submissionIdentityJSON(s)
	return `{"order":` + strings.TrimSuffix(identity, "}") + fmt.Sprintf(`,"retail_portfolio_id":%q,"product_type":"SPOT","order_type":"LIMIT","time_in_force":"IMMEDIATE_OR_CANCEL","created_time":%q,"size_in_quote":false,"size_inclusive_of_fees":false,"order_configuration":{"sor_limit_ioc":{"base_size":%q,"limit_price":%q}}}}`,
		s.PortfolioID, created.Format(time.RFC3339Nano), s.Order.Request.BaseSize, s.Order.Request.LimitPrice)
}

type submissionHarness struct {
	adapter *ExecutionAdapter
	key     *financial.Credentials
	mu      sync.Mutex
	paths   []string
	ports   map[string]bool
	posts   int
}

func newSubmissionHarness(t *testing.T, s execution.ConfirmedSubmission, now time.Time, mutate func(*http.Request, string) (string, int), postHandler http.HandlerFunc) *submissionHarness {
	t.Helper()
	credentials, signingKey := testCredentials(t)
	credentials.PortfolioID = s.PortfolioID
	h := &submissionHarness{key: &credentials, ports: map[string]bool{}}
	fixtures := executionTestFixtures(now)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verifyJWT(t, r, signingKey, r.URL.Path)
		h.mu.Lock()
		h.paths = append(h.paths, r.Method+" "+r.URL.Path)
		if h.ports[r.RemoteAddr] {
			t.Error("execution transport reused a connection")
		}
		h.ports[r.RemoteAddr] = true
		if r.Method == http.MethodPost {
			h.posts++
		}
		h.mu.Unlock()
		if r.Header.Get("Idempotency-Key") != "" || r.Header.Get("X-Idempotency-Key") != "" || r.ProtoMajor != 1 {
			t.Error("unexpected replay-enabling header or protocol")
		}
		var body string
		switch r.URL.Path {
		case "/api/v3/brokerage/key_permissions":
			body = fixtures[r.URL.Path]
		case "/api/v3/brokerage/accounts":
			path := r.URL.Path
			if r.URL.Query().Get("cursor") != "" {
				path += "?" + r.URL.Query().Get("cursor")
			}
			body = fixtures[path]
		case executionHistoryPath + "batch":
			query := r.URL.Query()
			if query.Get("retail_portfolio_id") != s.PortfolioID || query.Get("limit") != "100" || query.Has("client_order_id") || query.Has("client_order_ids") {
				t.Error("unscoped, unbounded, or undocumented history query")
			}
			if query.Has("order_status") {
				if !reflect.DeepEqual(query["order_status"], []string{"PENDING", "OPEN", "QUEUED", "CANCEL_QUEUED", "EDIT_QUEUED", "UNKNOWN_ORDER_STATUS"}) {
					t.Error("incomplete unresolved-order status set")
				}
				body = `{"orders":[],"has_next":false,"cursor":""}`
			} else {
				body = `{"orders":[` + submissionIdentityJSON(s) + `],"has_next":false,"cursor":""}`
			}
		case executionHistoryPath + submissionTestProviderID:
			body = submissionDetailJSON(s, now)
		case "/api/v3/brokerage/orders":
			if r.Method != http.MethodPost {
				t.Error("orders request must be POST")
			}
			encoded, _ := io.ReadAll(r.Body)
			var got map[string]any
			_ = json.Unmarshal(encoded, &got)
			want := map[string]any{"client_order_id": s.Order.Request.ClientOrderID, "preview_id": s.PreviewID, "product_id": s.Order.Request.ProductID,
				"side": s.Order.Request.Side, "retail_portfolio_id": s.PortfolioID, "order_configuration": map[string]any{"sor_limit_ioc": map[string]any{
					"base_size": s.Order.Request.BaseSize, "limit_price": s.Order.Request.LimitPrice}}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("submission changed exact immutable request: %s", encoded)
			}
			if postHandler != nil {
				postHandler(w, r)
				return
			}
			body = `{"success":true,"success_response":` + submissionIdentityJSON(s) + `}`
		default:
			t.Errorf("unexpected execution endpoint: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		status := http.StatusOK
		if mutate != nil {
			body, status = mutate(r, body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	c, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return now }
	h.adapter = NewExecutionAdapter(c)
	return h
}

func (h *submissionHarness) snapshot() ([]string, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.paths...), h.posts
}

func submitTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestExecutionSubmissionExactBuyAndSell(t *testing.T) {
	for _, side := range []string{"BUY", "SELL"} {
		t.Run(side, func(t *testing.T) {
			now := time.Now().UTC()
			s, _ := submissionFixture(now, side)
			h := newSubmissionHarness(t, s, now, nil, nil)
			original := *h.key
			ack, err := h.adapter.SubmitOnce(submitTestContext(t), h.key, s)
			if err != nil || ack.ProviderOrderID != submissionTestProviderID || ack.ClientOrderID != s.Order.Request.ClientOrderID || ack.Side != side {
				t.Fatalf("exact submission failed: %#v %v", ack, err)
			}
			paths, posts := h.snapshot()
			if posts != 1 || len(paths) != 5 || !reflect.DeepEqual(original, *h.key) {
				t.Fatalf("unexpected requests or mutated credentials: %v", paths)
			}
			if _, ok := any(h.adapter.client).(execution.ConfirmedOrderSender); ok {
				t.Fatal("normal read/preview client gained execution capability")
			}
		})
	}
}

func TestExecutionSubmissionUnsafeReadsCannotPost(t *testing.T) {
	cases := []struct{ name, path, old, replacement string }{
		{"transfer", "key_permissions", `"can_transfer":false`, `"can_transfer":true`},
		{"trade", "key_permissions", `"can_trade":true`, `"can_trade":false`},
		{"scope", "key_permissions", executionTestPortfolio, submissionTestProviderID},
		{"missing view", "key_permissions", `"can_view":true,`, ``},
		{"cash changed", "accounts", `"1000.10"`, `"1000.11"`},
		{"base changed", "accounts", `"1.00000001"`, `"1.00000002"`},
		{"external hold", "accounts", `"value":"0"`, `"value":"1"`},
		{"incomplete orders", "batch", `"has_next":false`, `"has_next":true`},
		{"missing completeness", "batch", `"has_next":false,`, ``},
		{"open order", "batch", `"orders":[]`, `"orders":[{}]`},
		{"requires proof", "batch", `"cursor":""`, `"cursor":"","proof_token_required":true`},
		{"duplicate completion", "batch", `"has_next":false`, `"has_next":true,"HAS_NEXT":false`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			s, _ := submissionFixture(now, "BUY")
			h := newSubmissionHarness(t, s, now, func(r *http.Request, body string) (string, int) {
				if strings.HasSuffix(r.URL.Path, tc.path) {
					body = strings.ReplaceAll(body, tc.old, tc.replacement)
				}
				return body, 200
			}, nil)
			ack, err := h.adapter.SubmitOnce(submitTestContext(t), h.key, s)
			_, posts := h.snapshot()
			if err == nil || ack != (execution.SubmissionAcknowledgement{}) || posts != 0 {
				t.Fatalf("unsafe evidence permitted send: %#v %v posts=%d", ack, err, posts)
			}
		})
	}
}

func TestExecutionSubmissionRequiresBoundedDeadlineAndCurrentProof(t *testing.T) {
	now := time.Now().UTC()
	s, _ := submissionFixture(now, "BUY")
	h := newSubmissionHarness(t, s, now, nil, nil)
	long, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	canceled, stop := context.WithCancel(context.Background())
	stop()
	for _, ctx := range []context.Context{context.Background(), long, canceled} {
		if _, err := h.adapter.SubmitOnce(ctx, h.key, s); err == nil {
			t.Fatal("unbounded or canceled send accepted")
		}
	}
	s.Preflight.QuoteObservedAt = now.Add(-11 * time.Second)
	if _, err := h.adapter.SubmitOnce(submitTestContext(t), h.key, s); err == nil {
		t.Fatal("stale quote accepted")
	}
	paths, _ := h.snapshot()
	if len(paths) != 0 {
		t.Fatalf("invalid authority reached provider: %v", paths)
	}
}

func TestExecutionSubmissionRechecksTimeAfterReads(t *testing.T) {
	now := time.Now().UTC()
	s, _ := submissionFixture(now, "BUY")
	h := newSubmissionHarness(t, s, now, nil, nil)
	var ticks atomic.Int32
	h.adapter.client.now = func() time.Time {
		if ticks.Add(1) >= 6 { // validation + four signed reads consume the old quote
			return now.Add(11 * time.Second)
		}
		return now
	}
	_, err := h.adapter.SubmitOnce(submitTestContext(t), h.key, s)
	_, posts := h.snapshot()
	if err == nil || posts != 0 {
		t.Fatal("read time renewed preflight authority")
	}
}

func TestExecutionSubmissionReportedRejectionAndAmbiguity(t *testing.T) {
	cases := []struct {
		name, response string
		status         int
		rejected       bool
	}{
		{"explicit rejection", `{"success":false,"error_response":{"error":"INSUFFICIENT_FUND","message":"secret must never escape"}}`, 200, true},
		{"missing success", `{"error_response":{"error":"INSUFFICIENT_FUND"}}`, 200, false},
		{"null success", `{"success":null}`, 200, false},
		{"free text rejection", `{"success":false,"error_response":{"error":"secret api key"}}`, 200, false},
		{"conflicting success", `{"success":false,"success_response":{},"error_response":{"error":"INSUFFICIENT_FUND"}}`, 200, false},
		{"conflicting config", `{"success":false,"order_configuration":{"market_market_ioc":{}},"error_response":{"error":"INSUFFICIENT_FUND"}}`, 200, false},
		{"duplicate success", `{"success":true,"SUCCESS":false,"error_response":{"error":"INSUFFICIENT_FUND"}}`, 200, false},
		{"missing identity", `{"success":true,"success_response":{}}`, 200, false},
		{"truncated", `{"success":true`, 200, false},
		{"rate limit", `{"success":false,"error_response":{"error":"RATE_LIMIT"}}`, 429, false},
		{"server error", `{"success":false,"error_response":{"error":"FAILURE"}}`, 503, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			s, _ := submissionFixture(now, "BUY")
			h := newSubmissionHarness(t, s, now, func(r *http.Request, body string) (string, int) {
				if r.Method == http.MethodPost {
					return tc.response, tc.status
				}
				return body, 200
			}, nil)
			ack, err := h.adapter.SubmitOnce(submitTestContext(t), h.key, s)
			var rejection *execution.SubmissionRejectedError
			_, posts := h.snapshot()
			if err == nil || strings.Contains(err.Error(), "secret") || ack != (execution.SubmissionAcknowledgement{}) || errors.As(err, &rejection) != tc.rejected || posts != 1 {
				t.Fatalf("incorrect rejection/unknown classification: %#v %v posts=%d", ack, err, posts)
			}
		})
	}
}

func TestExecutionLostAcknowledgementRecoveredReadOnlyWithoutRetry(t *testing.T) {
	now := time.Now().UTC()
	s, attempt := submissionFixture(now, "BUY")
	h := newSubmissionHarness(t, s, now, nil, func(w http.ResponseWriter, _ *http.Request) {
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = connection.Close() // accepted request, lost response
	})
	if ack, err := h.adapter.SubmitOnce(submitTestContext(t), h.key, s); err == nil || ack != (execution.SubmissionAcknowledgement{}) {
		t.Fatal("lost acknowledgement reported success")
	}
	// Recovery still uses the original identity after the submission proof expires.
	h.adapter.client.now = func() time.Time { return now.Add(time.Hour) }
	ack, err := NewExecutionAdapter(h.adapter.client).LookupSubmission(context.Background(), h.key, s, attempt)
	if err != nil || ack.ProviderOrderID != submissionTestProviderID || ack.ClientOrderID != attempt.ClientOrderID {
		t.Fatalf("lost acknowledgement not recovered: %#v %v", ack, err)
	}
	paths, posts := h.snapshot()
	if posts != 1 || len(paths) != 8 {
		t.Fatalf("recovery sent again: %v", paths)
	}
}

func TestExecutionRecoveryFailsClosedForIncompleteOrMismatchedEvidence(t *testing.T) {
	cases := []struct{ name, path, old, replacement string }{
		{"missing order", "batch", `"orders":[IDENTITY]`, `"orders":[]`},
		{"duplicate identity", "batch", `"orders":[IDENTITY]`, `"orders":[IDENTITY,IDENTITY]`},
		{"different IDs same client", "batch", `"orders":[IDENTITY]`, `"orders":[IDENTITY,SECOND]`},
		{"incomplete pages", "batch", `"has_next":false,"cursor":""`, `"has_next":true,"cursor":"loop"`},
		{"missing orders", "batch", `"orders":[IDENTITY],`, ``},
		{"null cursor", "batch", `"cursor":""`, `"cursor":null`},
		{"foreign portfolio", submissionTestProviderID, executionTestPortfolio, "10000000-0000-4000-8000-000000000002"},
		{"different size", submissionTestProviderID, `"base_size":"0.0010"`, `"base_size":"0.002"`},
		{"different limit", submissionTestProviderID, `"limit_price":"30000.00"`, `"limit_price":"30000.01"`},
		{"market fallback", submissionTestProviderID, `"sor_limit_ioc"`, `"market_market_ioc"`},
		{"additional config", submissionTestProviderID, `"sor_limit_ioc":`, `"market_market_ioc":{},"sor_limit_ioc":`},
		{"quote size", submissionTestProviderID, `"base_size":"0.0010"`, `"quote_size":"30"`},
		{"size in quote", submissionTestProviderID, `"size_in_quote":false`, `"size_in_quote":true`},
		{"inclusive fees", submissionTestProviderID, `"size_inclusive_of_fees":false`, `"size_inclusive_of_fees":true`},
		{"missing quote flag", submissionTestProviderID, `"size_in_quote":false,`, ``},
		{"wrong product type", submissionTestProviderID, `"SPOT"`, `"FUTURE"`},
		{"wrong time in force", submissionTestProviderID, `"IMMEDIATE_OR_CANCEL"`, `"GOOD_UNTIL_CANCELLED"`},
		{"attached order", submissionTestProviderID, `"size_in_quote":false`, `"size_in_quote":false,"attached_order_configuration":{"trigger_bracket_gtc":{}}`},
		{"margin", submissionTestProviderID, `"size_in_quote":false`, `"size_in_quote":false,"margin_type":"CROSS"`},
		{"leverage", submissionTestProviderID, `"size_in_quote":false`, `"size_in_quote":false,"leverage":"2"`},
		{"liquidation", submissionTestProviderID, `"size_in_quote":false`, `"size_in_quote":false,"is_liquidation":true`},
		{"duplicate field", submissionTestProviderID, `"size_in_quote":false`, `"size_in_quote":true,"SIZE_IN_QUOTE":false`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			s, attempt := submissionFixture(now, "BUY")
			replacer := strings.NewReplacer("IDENTITY", submissionIdentityJSON(s), "SECOND", strings.Replace(submissionIdentityJSON(s), submissionTestProviderID, "40000000-0000-4000-8000-000000000002", 1))
			h := newSubmissionHarness(t, s, now, func(r *http.Request, body string) (string, int) {
				if strings.HasSuffix(r.URL.Path, tc.path) {
					body = strings.ReplaceAll(body, replacer.Replace(tc.old), replacer.Replace(tc.replacement))
				}
				return body, 200
			}, nil)
			ack, err := h.adapter.LookupSubmission(context.Background(), h.key, s, attempt)
			_, posts := h.snapshot()
			if err == nil || ack != (execution.SubmissionAcknowledgement{}) || posts != 0 {
				t.Fatalf("unsafe recovery accepted: %#v %v posts=%d", ack, err, posts)
			}
		})
	}
}

func TestExecutionRecoveryReadOnlyPermissionAndDirectID(t *testing.T) {
	now := time.Now().UTC()
	s, attempt := submissionFixture(now, "BUY")
	attempt.ProviderOrderID = submissionTestProviderID
	h := newSubmissionHarness(t, s, now, func(r *http.Request, body string) (string, int) {
		if strings.HasSuffix(r.URL.Path, "key_permissions") {
			body = strings.Replace(body, `"can_trade":true`, `"can_trade":false`, 1)
		}
		return body, 200
	}, nil)
	ack, err := h.adapter.LookupSubmission(context.Background(), h.key, s, attempt)
	paths, posts := h.snapshot()
	if err != nil || ack.ProviderOrderID != submissionTestProviderID || posts != 0 || len(paths) != 2 {
		t.Fatalf("read-only direct identity lookup failed: %#v %v %v", ack, err, paths)
	}
}

type submissionRetryTransport struct{ calls int }

func (r *submissionRetryTransport) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls++
	return nil, errors.New("unsafe wrapper must not be called")
}

func TestExecutionSubmissionRejectsWrappedTransportAndRedirects(t *testing.T) {
	now := time.Now().UTC()
	s, _ := submissionFixture(now, "BUY")
	h := newSubmissionHarness(t, s, now, nil, nil)
	wrapped := &submissionRetryTransport{}
	h.adapter.client.http.Transport = wrapped
	if _, err := h.adapter.SubmitOnce(submitTestContext(t), h.key, s); err == nil || wrapped.calls != 0 {
		t.Fatal("custom retry wrapper accepted")
	}
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	defer target.Close()
	h = newSubmissionHarness(t, s, now, nil, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	})
	ack, err := h.adapter.SubmitOnce(submitTestContext(t), h.key, s)
	_, posts := h.snapshot()
	if err == nil || ack != (execution.SubmissionAcknowledgement{}) || posts != 1 || redirected.Load() != 0 {
		t.Fatal("redirect was followed or replayed")
	}
}

func TestExecutionSubmissionTimeoutCannotRetry(t *testing.T) {
	now := time.Now().UTC()
	s, _ := submissionFixture(now, "BUY")
	started, finish := make(chan struct{}), make(chan struct{})
	h := newSubmissionHarness(t, s, now, nil, func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-finish
		_, _ = w.Write([]byte(`{"success":true,"success_response":` + submissionIdentityJSON(s) + `}`))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { <-started; cancel() }()
	ack, err := h.adapter.SubmitOnce(ctx, h.key, s)
	close(finish)
	_, posts := h.snapshot()
	if err == nil || ack != (execution.SubmissionAcknowledgement{}) || posts != 1 {
		t.Fatalf("timeout retried or acknowledged: %#v %v posts=%d", ack, err, posts)
	}
}

func TestExecutionTransportOverridesExplicitHTTP2AndPreservesOriginal(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 1 {
			t.Error("execution request negotiated HTTP/2")
		}
		_, _ = w.Write([]byte(`{"orders":[],"has_next":false,"cursor":""}`))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	original := server.Client().Transport.(*http.Transport)
	original.ForceAttemptHTTP2 = true
	original.Protocols = new(http.Protocols)
	original.Protocols.SetHTTP2(true)
	original.Protocols.SetHTTP1(true)
	c, err := New(Config{BaseURL: server.URL}, &http.Client{Transport: original})
	if err != nil {
		t.Fatal(err)
	}
	isolated, done, err := NewExecutionAdapter(c).isolatedClient()
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	key, _ := testCredentials(t)
	var p executionHistoryPage
	if err := isolated.submissionRequest(context.Background(), &key, http.MethodGet, executionHistoryPath+"batch", nil, &p); err != nil {
		t.Fatal(err)
	}
	if !original.ForceAttemptHTTP2 || !original.Protocols.HTTP2() || original.DisableKeepAlives {
		t.Fatal("execution modified original read/preview transport")
	}
}

func TestExecutionRecoveryCompletePaginationAndTimeBounds(t *testing.T) {
	t.Run("complete second page", func(t *testing.T) {
		now := time.Now().UTC()
		s, attempt := submissionFixture(now, "BUY")
		h := newSubmissionHarness(t, s, now, func(r *http.Request, body string) (string, int) {
			if r.URL.Path == executionHistoryPath+"batch" && r.URL.Query().Get("cursor") == "" {
				other := strings.ReplaceAll(submissionIdentityJSON(s), submissionTestProviderID, "40000000-0000-4000-8000-000000000002")
				other = strings.ReplaceAll(other, s.Order.Request.ClientOrderID, "external-order")
				return `{"orders":[` + other + `],"has_next":true,"cursor":"next"}`, 200
			}
			return body, 200
		}, nil)
		ack, err := h.adapter.LookupSubmission(context.Background(), h.key, s, attempt)
		paths, posts := h.snapshot()
		if err != nil || ack.ProviderOrderID != submissionTestProviderID || len(paths) != 4 || posts != 0 {
			t.Fatalf("complete paginated lookup failed: %#v %v %v", ack, err, paths)
		}
	})
	for _, offset := range []time.Duration{-time.Nanosecond, time.Nanosecond} {
		t.Run(offset.String(), func(t *testing.T) {
			now := time.Now().UTC()
			s, attempt := submissionFixture(now, "BUY")
			h := newSubmissionHarness(t, s, now, func(r *http.Request, body string) (string, int) {
				if r.URL.Path == executionHistoryPath+submissionTestProviderID {
					return submissionDetailJSON(s, now.Add(offset)), 200
				}
				return body, 200
			}, nil)
			if ack, err := h.adapter.LookupSubmission(context.Background(), h.key, s, attempt); err == nil || ack != (execution.SubmissionAcknowledgement{}) {
				t.Fatal("out-of-bounds broker creation timestamp accepted")
			}
		})
	}
}

func TestExecutionRecoveryAuthorizationAndHTTPFailuresCannotSend(t *testing.T) {
	for _, failure := range []string{"no view", "transfer", "foreign scope", "missing detail", "history unavailable", "oversized detail"} {
		t.Run(failure, func(t *testing.T) {
			now := time.Now().UTC()
			s, attempt := submissionFixture(now, "BUY")
			h := newSubmissionHarness(t, s, now, func(r *http.Request, body string) (string, int) {
				if strings.HasSuffix(r.URL.Path, "key_permissions") {
					switch failure {
					case "no view":
						body = strings.Replace(body, `"can_view":true`, `"can_view":false`, 1)
					case "transfer":
						body = strings.Replace(body, `"can_transfer":false`, `"can_transfer":true`, 1)
					case "foreign scope":
						body = strings.Replace(body, s.PortfolioID, submissionTestProviderID, 1)
					}
				}
				if strings.HasSuffix(r.URL.Path, submissionTestProviderID) && failure == "missing detail" {
					return `{"message":"secret"}`, 404
				}
				if strings.HasSuffix(r.URL.Path, submissionTestProviderID) && failure == "oversized detail" {
					return body + strings.Repeat(" ", maxBodyBytes), 200
				}
				if strings.HasSuffix(r.URL.Path, "batch") && failure == "history unavailable" {
					return `{"message":"secret"}`, 503
				}
				return body, 200
			}, nil)
			ack, err := h.adapter.LookupSubmission(context.Background(), h.key, s, attempt)
			_, posts := h.snapshot()
			if err == nil || strings.Contains(err.Error(), "secret") || ack != (execution.SubmissionAcknowledgement{}) || posts != 0 {
				t.Fatalf("unsafe recovery performed: %#v %v posts=%d", ack, err, posts)
			}
		})
	}
}
