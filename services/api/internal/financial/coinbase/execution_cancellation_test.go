package coinbase

import (
	"context"
	"encoding/json"
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

type cancellationHarness struct {
	adapter *ExecutionAdapter
	key     *financial.Credentials
	s       execution.ConfirmedSubmission
	a       execution.Attempt
	mu      sync.Mutex
	paths   []string
	ports   map[string]bool
	posts   int
}

func newCancellationHarness(t *testing.T, side, status string, mutate func(*http.Request, string) (string, int), postHandler http.HandlerFunc) *cancellationHarness {
	t.Helper()
	now := time.Now().UTC()
	s, a := submissionFixture(now, side)
	a.ProviderOrderID = submissionTestProviderID
	credentials, signingKey := testCredentials(t)
	credentials.PortfolioID = s.PortfolioID
	h := &cancellationHarness{key: &credentials, s: s, a: a, ports: map[string]bool{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verifyJWT(t, r, signingKey, r.URL.Path)
		h.mu.Lock()
		h.paths = append(h.paths, r.Method+" "+r.URL.Path)
		if h.ports[r.RemoteAddr] {
			t.Error("cancellation reused a connection")
		}
		h.ports[r.RemoteAddr] = true
		if r.Method == http.MethodPost {
			h.posts++
		}
		h.mu.Unlock()
		if r.ProtoMajor != 1 || r.Header.Get("Idempotency-Key") != "" || r.Header.Get("X-Idempotency-Key") != "" {
			t.Error("cancellation enabled transport replay")
		}
		if r.URL.RawQuery != "" {
			t.Error("cancellation used unexpected query parameters")
		}
		var body string
		switch r.URL.Path {
		case "/api/v3/brokerage/key_permissions":
			body = `{"can_view":true,"can_trade":true,"can_transfer":false,"portfolio_uuid":"` + s.PortfolioID + `"}`
		case executionHistoryPath + submissionTestProviderID:
			encoded, _ := json.Marshal(reconciliationOrderJSON(s, now, status, 0, "0", "0", "0", time.Time{}))
			body = string(encoded)
		case executionCancellationPath:
			if r.Method != http.MethodPost {
				t.Error("cancellation must be POST")
			}
			encoded, _ := io.ReadAll(r.Body)
			var got map[string]any
			if json.Unmarshal(encoded, &got) != nil || !reflect.DeepEqual(got, map[string]any{"order_ids": []any{submissionTestProviderID}}) {
				t.Errorf("cancellation changed the exact singleton target: %s", encoded)
			}
			if postHandler != nil {
				postHandler(w, r)
				return
			}
			body = `{"results":[{"success":true,"failure_reason":"UNKNOWN_CANCEL_FAILURE_REASON","order_id":"` + submissionTestProviderID + `"}]}`
		default:
			t.Errorf("unexpected cancellation endpoint: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path != executionCancellationPath && r.Method != http.MethodGet {
			t.Error("cancellation attempted another provider write")
		}
		code := http.StatusOK
		if mutate != nil {
			body, code = mutate(r, body)
		}
		w.WriteHeader(code)
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

func (h *cancellationHarness) snapshot() ([]string, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.paths...), h.posts
}

func (h *cancellationHarness) cancel(t *testing.T) (execution.CancellationAcknowledgement, error) {
	t.Helper()
	return h.adapter.CancelOnce(submitTestContext(t), h.key, h.s, h.a)
}

func TestExecutionCancellationExactOriginalOrderOnly(t *testing.T) {
	for _, side := range []string{"BUY", "SELL"} {
		for _, status := range []string{"PENDING", "OPEN", "QUEUED", "EDIT_QUEUED"} {
			t.Run(side+"-"+status, func(t *testing.T) {
				h := newCancellationHarness(t, side, status, nil, nil)
				original := *h.key
				// The old send preview/approval is not a refreshed send grant;
				// only archived identity validation is appropriate to cancel.
				h.adapter.client.now = func() time.Time { return h.a.ClaimedAt.Add(time.Hour) }
				ack, err := h.cancel(t)
				paths, posts := h.snapshot()
				want := []string{"GET /api/v3/brokerage/key_permissions", "GET " + executionHistoryPath + submissionTestProviderID, "POST " + executionCancellationPath}
				if err != nil || !ack.Accepted || ack.ProviderOrderID != h.a.ProviderOrderID || posts != 1 || !reflect.DeepEqual(paths, want) || !reflect.DeepEqual(original, *h.key) {
					t.Fatalf("invalid exact cancellation: %#v %v %v", ack, err, paths)
				}
				if _, ok := any(h.adapter.client).(execution.CancellationSender); ok {
					t.Fatal("existing runtime financial Client acquired cancellation capability")
				}
			})
		}
	}
}

func TestExecutionCancellationUnsafePermissionOrOrderCannotPost(t *testing.T) {
	cases := []struct{ name, path, old, replacement string }{
		{"view revoked", "key_permissions", `"can_view":true`, `"can_view":false`},
		{"trade revoked", "key_permissions", `"can_trade":true`, `"can_trade":false`},
		{"transfer enabled", "key_permissions", `"can_transfer":false`, `"can_transfer":true`},
		{"missing trade permission", "key_permissions", `"can_trade":true,`, ``},
		{"foreign key portfolio", "key_permissions", executionTestPortfolio, submissionTestProviderID},
		{"foreign detail portfolio", submissionTestProviderID, executionTestPortfolio, submissionTestProviderID},
		{"wrong order ID", submissionTestProviderID, `"order_id":"` + submissionTestProviderID + `"`, `"order_id":"` + executionTestPortfolio + `"`},
		{"wrong client identity", submissionTestProviderID, `"client_order_id":"60000000-0000-4000-8000-000000000005"`, `"client_order_id":"60000000-0000-4000-8000-000000000009"`},
		{"wrong side", submissionTestProviderID, `"side":"BUY"`, `"side":"SELL"`},
		{"wrong product", submissionTestProviderID, `"product_id":"BTC-USD"`, `"product_id":"ETH-USD"`},
		{"changed size", submissionTestProviderID, `"base_size":"0.0010"`, `"base_size":"0.0020"`},
		{"changed limit", submissionTestProviderID, `"limit_price":"30000.00"`, `"limit_price":"30001"`},
		{"market order", submissionTestProviderID, `"sor_limit_ioc"`, `"market_market_ioc"`},
		{"wrong time in force", submissionTestProviderID, `"IMMEDIATE_OR_CANCEL"`, `"GOOD_UNTIL_CANCELLED"`},
		{"wrong product type", submissionTestProviderID, `"SPOT"`, `"FUTURE"`},
		{"pending cancel", submissionTestProviderID, `"pending_cancel":false`, `"pending_cancel":true`},
		{"missing pending cancel", submissionTestProviderID, `"pending_cancel":false,`, ``},
		{"missing settled", submissionTestProviderID, `"settled":false,`, ``},
		{"settled contradiction", submissionTestProviderID, `"settled":false`, `"settled":true`},
		{"missing status", submissionTestProviderID, `"status":"OPEN",`, ``},
		{"null status", submissionTestProviderID, `"status":"OPEN"`, `"status":null`},
		{"duplicate status", submissionTestProviderID, `"status":"OPEN"`, `"status":"FILLED","STATUS":"OPEN"`},
	}
	for _, status := range []string{"FILLED", "CANCELLED", "EXPIRED", "FAILED", "REJECTED", "CANCEL_QUEUED", "UNKNOWN_ORDER_STATUS", ""} {
		cases = append(cases, struct{ name, path, old, replacement string }{"status " + status, submissionTestProviderID, `"status":"OPEN"`, `"status":"` + status + `"`})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newCancellationHarness(t, "BUY", "OPEN", func(r *http.Request, body string) (string, int) {
				if strings.HasSuffix(r.URL.Path, tc.path) {
					if !strings.Contains(body, tc.old) {
						t.Error("invalid cancellation mutation fixture")
					}
					body = strings.ReplaceAll(body, tc.old, tc.replacement)
				}
				return body, 200
			}, nil)
			ack, err := h.cancel(t)
			_, posts := h.snapshot()
			if err == nil || ack != (execution.CancellationAcknowledgement{}) || posts != 0 {
				t.Fatalf("unsafe cancellation posted: %#v %v posts=%d", ack, err, posts)
			}
		})
	}
}

func TestExecutionCancellationResponseIsOnlyCorrelatedAcceptance(t *testing.T) {
	cases := []struct {
		name, result string
		accepted     bool
	}{
		{"accepted omitted reason", `{"success":true,"order_id":"ID"}`, true},
		{"accepted empty reason", `{"success":true,"order_id":"ID","failure_reason":""}`, true},
		{"accepted default reason", `{"success":true,"order_id":"ID","failure_reason":"UNKNOWN_CANCEL_FAILURE_REASON"}`, true},
		{"rejected", `{"success":false,"order_id":"ID","failure_reason":"ORDER_NOT_FOUND"}`, false},
		{"already filled rejection is not terminal evidence", `{"success":false,"order_id":"ID","failure_reason":"ORDER_ALREADY_FILLED"}`, false},
		{"rejected no reason", `{"success":false,"order_id":"ID"}`, false},
		{"rejected default reason", `{"success":false,"order_id":"ID","failure_reason":"UNKNOWN_CANCEL_FAILURE_REASON"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newCancellationHarness(t, "SELL", "OPEN", func(r *http.Request, body string) (string, int) {
				if r.Method == http.MethodPost {
					body = `{"results":[` + strings.ReplaceAll(tc.result, "ID", submissionTestProviderID) + `]}`
				}
				return body, 200
			}, nil)
			ack, err := h.cancel(t)
			_, posts := h.snapshot()
			if err != nil || ack.ProviderOrderID != submissionTestProviderID || ack.Accepted != tc.accepted || posts != 1 {
				t.Fatalf("incorrect acknowledgement only: %#v %v", ack, err)
			}
		})
	}
}

func TestExecutionCancellationMalformedOrConflictingAcknowledgementIsUnknown(t *testing.T) {
	cases := []string{
		`{}`, `{"results":null}`, `{"results":[]}`, `{"results":[null]}`,
		`{"results":[{"success":true,"order_id":"ID"},{"success":true,"order_id":"ID"}]}`,
		`{"results":[{"success":true,"order_id":"wrong"}]}`,
		`{"results":[{"success":true}]}`, `{"results":[{"order_id":"ID"}]}`,
		`{"results":[{"success":null,"order_id":"ID"}]}`, `{"results":[{"success":"true","order_id":"ID"}]}`,
		`{"results":[{"success":false,"SUCCESS":true,"order_id":"ID"}]}`,
		`{"results":[{"success":false,"\u017fuccess":true,"order_id":"ID"}]}`,
		`{"results":[{"success":true,"order_id":"ID","failure_reason":"ORDER_NOT_FOUND"}]}`,
		`{"results":[{"success":false,"order_id":"ID","failure_reason":"secret provider text"}]}`,
		`{"results":[{"success":true,"order_id":"ID","failure_reason":null}]}`,
		`{"results":[{"success":true,"order_id":"ID","failure_reason":123}]}`,
		`{"results":[{"success":true,"order_id":"ID","failure_reason":{}}]}`,
		`{"results":[{"success":true,"order_id":"ID","failure_reason":"","failure_reason":""}]}`,
		`{"results":[{"success":true,"order_id":"ID","order_id":"ID"}]}`,
		`{"results":[{"success":true,"order_id":"ID"}],"RESULTS":[]}`,
		`{"results":[{"success":true,"order_id":"ID"}]} {}`,
		`{"results":[{"success":true,"order_id":"ID"}`, // truncated
		`{"results":[{"success":true,"order_id":"ID"}]}` + strings.Repeat(" ", maxBodyBytes),
	}
	for i, response := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			h := newCancellationHarness(t, "BUY", "OPEN", func(r *http.Request, body string) (string, int) {
				if r.Method == http.MethodPost {
					body = strings.ReplaceAll(response, "ID", submissionTestProviderID)
				}
				return body, 200
			}, nil)
			ack, err := h.cancel(t)
			_, posts := h.snapshot()
			if err == nil || strings.Contains(err.Error(), "secret") || ack != (execution.CancellationAcknowledgement{}) || posts != 1 {
				t.Fatalf("ambiguous acknowledgement accepted: %#v %v posts=%d", ack, err, posts)
			}
		})
	}
}

func TestExecutionCancellationInvalidTargetOrDeadlineNeverCallsProvider(t *testing.T) {
	for _, bad := range []string{"missing ID", "wrong attempt", "wrong portfolio", "bad digest", "no deadline", "long deadline", "canceled"} {
		t.Run(bad, func(t *testing.T) {
			h := newCancellationHarness(t, "BUY", "OPEN", nil, nil)
			ctx := submitTestContext(t)
			switch bad {
			case "missing ID":
				h.a.ProviderOrderID = ""
			case "wrong attempt":
				h.a.ClientOrderID = executionTestPortfolio
			case "wrong portfolio":
				h.key.PortfolioID = submissionTestProviderID
			case "bad digest":
				h.s.Order.RequestDigest = strings.Repeat("0", 64)
			case "no deadline":
				ctx = context.Background()
			case "long deadline":
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(context.Background(), 6*time.Second)
				defer cancel()
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
				cancel()
			}
			ack, err := h.adapter.CancelOnce(ctx, h.key, h.s, h.a)
			paths, posts := h.snapshot()
			if err == nil || ack != (execution.CancellationAcknowledgement{}) || posts != 0 || len(paths) != 0 {
				t.Fatalf("unsafe target/deadline reached provider: %#v %v %v", ack, err, paths)
			}
		})
	}
}

func TestExecutionCancellationNoRedirectRetryOrDetachedSend(t *testing.T) {
	t.Run("custom retry transport rejected", func(t *testing.T) {
		h := newCancellationHarness(t, "BUY", "OPEN", nil, nil)
		wrapped := &submissionRetryTransport{}
		h.adapter.client.http.Transport = wrapped
		if _, err := h.cancel(t); err == nil || wrapped.calls != 0 {
			t.Fatal("custom retry transport reached cancellation")
		}
	})
	t.Run("redirect not followed", func(t *testing.T) {
		var redirected atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
		defer target.Close()
		h := newCancellationHarness(t, "BUY", "OPEN", nil, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
		})
		ack, err := h.cancel(t)
		_, posts := h.snapshot()
		if err == nil || ack != (execution.CancellationAcknowledgement{}) || posts != 1 || redirected.Load() != 0 {
			t.Fatal("cancellation redirect followed or acknowledged")
		}
	})
	t.Run("lost response is not retried", func(t *testing.T) {
		h := newCancellationHarness(t, "BUY", "OPEN", nil, func(w http.ResponseWriter, _ *http.Request) {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = connection.Close()
		})
		ack, err := h.cancel(t)
		_, posts := h.snapshot()
		if err == nil || ack != (execution.CancellationAcknowledgement{}) || posts != 1 {
			t.Fatalf("lost cancellation response was retried/accepted: %#v %v %d", ack, err, posts)
		}
	})
	t.Run("canceled in-flight post is unknown", func(t *testing.T) {
		started, finish := make(chan struct{}), make(chan struct{})
		h := newCancellationHarness(t, "BUY", "OPEN", nil, func(w http.ResponseWriter, _ *http.Request) {
			close(started)
			<-finish
			_, _ = w.Write([]byte(`{"results":[{"success":true,"order_id":"` + submissionTestProviderID + `"}]}`))
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		go func() { <-started; cancel() }()
		ack, err := h.adapter.CancelOnce(ctx, h.key, h.s, h.a)
		close(finish)
		_, posts := h.snapshot()
		if err == nil || ack != (execution.CancellationAcknowledgement{}) || posts != 1 {
			t.Fatal("canceled request retried or acknowledged")
		}
	})
}

func TestExecutionCancellationDeadlineDuringReadsCannotPost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h := newCancellationHarness(t, "BUY", "OPEN", func(r *http.Request, body string) (string, int) {
		if r.URL.Path == executionHistoryPath+submissionTestProviderID {
			cancel()
		}
		return body, 200
	}, nil)
	ack, err := h.adapter.CancelOnce(ctx, h.key, h.s, h.a)
	_, posts := h.snapshot()
	if err == nil || ack != (execution.CancellationAcknowledgement{}) || posts != 0 {
		t.Fatal("cancellation posted after read consumed context")
	}
}

func TestExecutionCancellationHTTPFailuresNeverRetryOrLeakProviderBody(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			h := newCancellationHarness(t, "BUY", "OPEN", func(r *http.Request, body string) (string, int) {
				if r.Method == http.MethodPost {
					return `{"message":"secret provider body"}`, status
				}
				return body, http.StatusOK
			}, nil)
			ack, err := h.cancel(t)
			_, posts := h.snapshot()
			if err == nil || strings.Contains(err.Error(), "secret") || ack != (execution.CancellationAcknowledgement{}) || posts != 1 {
				t.Fatalf("HTTP failure retried, acknowledged or leaked: %#v %v posts=%d", ack, err, posts)
			}
		})
	}
}
