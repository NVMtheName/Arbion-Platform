package coinbase

import (
	"context"
	"encoding/json"
	"fmt"
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

type settlementTestReads struct {
	Details, Fills, AccountPass, AccountPages, OpenOrders int
}

type settlementTestMutation func(*http.Request, map[string]any, settlementTestReads)

type settlementTestHarness struct {
	adapter *ExecutionAdapter
	key     *financial.Credentials
	s       execution.ConfirmedSubmission
	a       execution.Attempt
	clock   atomic.Int64
	mu      sync.Mutex
	reads   settlementTestReads
	paths   []string
}

func newSettlementTestHarness(t *testing.T, side, status string, partial bool, mutate settlementTestMutation) *settlementTestHarness {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	s, a := submissionFixture(now, side)
	a.ProviderOrderID = submissionTestProviderID
	h := &settlementTestHarness{s: s, a: a}
	credentials, signingKey := testCredentials(t)
	credentials.PortfolioID = s.PortfolioID
	h.key = &credentials
	zero := status == "FAILED" || status == "EXPIRED"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verifyJWT(t, r, signingKey, r.URL.Path)
		h.mu.Lock()
		h.paths = append(h.paths, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case executionHistoryPath + submissionTestProviderID:
			h.reads.Details++
		case executionHistoryPath + "fills":
			h.reads.Fills++
		case executionHistoryPath + "batch":
			h.reads.OpenOrders++
		case "/api/v3/brokerage/accounts":
			h.reads.AccountPages++
			if r.URL.Query().Get("cursor") == "" {
				h.reads.AccountPass++
			}
		}
		reads := h.reads
		h.mu.Unlock()
		if r.Method != http.MethodGet {
			t.Errorf("settlement attempted a provider write: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		first, last := now.Add(10*time.Millisecond), now.Add(20*time.Millisecond)
		var body map[string]any
		switch r.URL.Path {
		case "/api/v3/brokerage/key_permissions":
			// Settlement must remain available with a view-only recovery key.
			body = map[string]any{"can_view": true, "can_trade": false, "can_transfer": false, "portfolio_uuid": s.PortfolioID}
		case executionHistoryPath + submissionTestProviderID:
			if zero {
				body = reconciliationOrderJSON(s, now, status, 0, "0", "0", "0", time.Time{})
			} else if partial {
				body = reconciliationOrderJSON(s, now, status, 1, "0.0004", "12", "0.04", first)
			} else {
				body = reconciliationOrderJSON(s, now, status, 2, "0.001", "30", "0.10", last)
			}
		case executionHistoryPath + "fills":
			if r.URL.Query().Get("order_ids") != a.ProviderOrderID || r.URL.Query().Get("limit") != "100" {
				t.Error("settlement fill read lost exact order scope")
			}
			if zero {
				body = map[string]any{"fills": []any{}, "cursor": ""}
			} else if partial {
				body = map[string]any{"fills": []any{reconciliationFillJSON(s, first, "1", "0.0004", "0.04", false)}, "cursor": ""}
			} else if r.URL.Query().Get("cursor") == "" {
				body = map[string]any{"fills": []any{reconciliationFillJSON(s, last, "2", "0.0006", "0.06", false)}, "cursor": "page-2"}
			} else {
				body = map[string]any{"fills": []any{reconciliationFillJSON(s, first, "1", "0.0004", "0.04", false)}, "cursor": ""}
			}
		case executionHistoryPath + "batch":
			q := r.URL.Query()
			if q.Get("retail_portfolio_id") != s.PortfolioID || q.Get("limit") != "100" || q.Has("client_order_id") || q.Has("client_order_ids") || q.Has("cursor") ||
				!reflect.DeepEqual(q["order_status"], []string{"PENDING", "OPEN", "QUEUED", "CANCEL_QUEUED", "EDIT_QUEUED", "UNKNOWN_ORDER_STATUS"}) {
				t.Error("settlement unresolved-order guard lost exact scope/status coverage")
			}
			body = map[string]any{"orders": []any{}, "has_next": false, "cursor": ""}
		case "/api/v3/brokerage/accounts":
			if r.URL.Query().Get("limit") != "250" || r.URL.Query().Get("retail_portfolio_id") != s.PortfolioID {
				t.Error("settlement account read lost scope/bound")
			}
			cash, base := "970.00", "1.00100001"
			if side == "SELL" {
				cash, base = "1030.00", "0.99900001"
			}
			if partial {
				cash, base = "988.06", "1.00040001"
				if side == "SELL" {
					cash, base = "1012.06", "0.99960001"
				}
			}
			if zero {
				cash, base = "1000.10", "1.00000001"
			}
			currency, amount, id, next, cursor := "USD", cash, "20000000-0000-4000-8000-000000000001", true, "page-2"
			if r.URL.Query().Get("cursor") != "" {
				currency, amount, id, next, cursor = "BTC", base, "20000000-0000-4000-8000-000000000002", false, ""
			}
			account := map[string]any{"uuid": id, "currency": currency, "available_balance": map[string]any{"value": amount, "currency": currency},
				"hold": map[string]any{"value": "0", "currency": currency}, "active": true, "ready": true, "retail_portfolio_id": s.PortfolioID}
			body = map[string]any{"accounts": []any{account}, "has_next": next, "cursor": cursor}
		default:
			t.Errorf("unexpected settlement endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if mutate != nil {
			mutate(r, body, reads)
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(server.Close)
	c, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return now.Add(time.Second + time.Duration(h.clock.Add(1))*time.Millisecond) }
	h.adapter = NewExecutionAdapter(c)
	return h
}

func (h *settlementTestHarness) snapshot() (settlementTestReads, []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.reads, append([]string(nil), h.paths...)
}

func (h *settlementTestHarness) collect() (execution.AccountSettlementEvidence, error) {
	return h.adapter.CollectAccountSettlement(context.Background(), h.key, h.s, h.a)
}

func TestExecutionSettlementStableTerminalBuySellAndPartialOrZeroFill(t *testing.T) {
	for _, side := range []string{"BUY", "SELL"} {
		for _, status := range []string{"FILLED", "CANCELLED", "EXPIRED", "FAILED"} {
			t.Run(side+"-"+status, func(t *testing.T) {
				h := newSettlementTestHarness(t, side, status, status == "CANCELLED", nil)
				original := *h.key
				e, err := h.collect()
				if err != nil {
					t.Fatal(err)
				}
				wantStatus, count, cash, base := status, int64(2), "970", "1.00100001"
				if side == "SELL" {
					cash, base = "1030", "0.99900001"
				}
				if status == "CANCELLED" {
					count, cash, base = 1, "988.06", "1.00040001"
					if side == "SELL" {
						cash, base = "1012.06", "0.99960001"
					}
				}
				if status == "EXPIRED" || status == "FAILED" {
					count, cash, base = 0, "1000.1", "1.00000001"
				}
				if status == "FAILED" {
					wantStatus = "REJECTED"
				}
				if e.PortfolioID != h.s.PortfolioID || !e.Complete || !e.NoOpenOrders || e.Observation.Status != wantStatus || e.Observation.FillCount != count ||
					e.CashUSD != cash || e.AvailableCashUSD != cash || e.TotalBase != base || e.AvailableBase != base ||
					e.StartedAt.After(e.Observation.StartedAt) || !e.CompletedAt.After(e.Observation.ObservedAt) || !reflect.DeepEqual(original, *h.key) {
					t.Fatalf("invalid or mutated settlement evidence: %#v", e)
				}
				reads, paths := h.snapshot()
				if reads.AccountPass != 2 || reads.AccountPages != 4 || reads.OpenOrders != 2 || reads.Details != 2 || paths[len(paths)-1] != "GET "+executionHistoryPath+"batch" {
					t.Fatalf("missing stable account/order brackets: %#v %v", reads, paths)
				}
				wantSuffix := []string{"GET " + executionHistoryPath + "batch", "GET /api/v3/brokerage/accounts", "GET /api/v3/brokerage/accounts", "GET /api/v3/brokerage/accounts", "GET /api/v3/brokerage/accounts", "GET " + executionHistoryPath + "batch"}
				if !reflect.DeepEqual(paths[len(paths)-len(wantSuffix):], wantSuffix) {
					t.Fatalf("account reads were not bracketed: %v", paths)
				}
			})
		}
	}
}

func TestExecutionSettlementRejectsUnreadyUnstableOrIncompleteAccountEvidence(t *testing.T) {
	type mutation struct {
		name   string
		mutate settlementTestMutation
	}
	cases := []mutation{}
	for _, tc := range []struct {
		field string
		value any
	}{{"retail_portfolio_id", submissionTestProviderID}, {"active", false}, {"ready", false}, {"uuid", nil}, {"currency", "ETH"}} {
		tc := tc
		cases = append(cases, mutation{"account " + tc.field, func(r *http.Request, b map[string]any, _ settlementTestReads) {
			if strings.HasSuffix(r.URL.Path, "accounts") {
				b["accounts"].([]any)[0].(map[string]any)[tc.field] = tc.value
			}
		}})
	}
	for _, field := range []string{"accounts", "has_next", "cursor"} {
		field := field
		cases = append(cases, mutation{"missing " + field, func(r *http.Request, b map[string]any, _ settlementTestReads) {
			if strings.HasSuffix(r.URL.Path, "accounts") {
				delete(b, field)
			}
		}})
	}
	cases = append(cases,
		mutation{"external hold", func(r *http.Request, b map[string]any, _ settlementTestReads) {
			if strings.HasSuffix(r.URL.Path, "accounts") {
				b["accounts"].([]any)[0].(map[string]any)["hold"].(map[string]any)["value"] = "0.01"
			}
		}},
		mutation{"wrong balance currency", func(r *http.Request, b map[string]any, _ settlementTestReads) {
			if strings.HasSuffix(r.URL.Path, "accounts") {
				b["accounts"].([]any)[0].(map[string]any)["available_balance"].(map[string]any)["currency"] = "EUR"
			}
		}},
		mutation{"cash changes between traversals", func(r *http.Request, b map[string]any, state settlementTestReads) {
			if strings.HasSuffix(r.URL.Path, "accounts") && state.AccountPass == 2 && r.URL.Query().Get("cursor") == "" {
				b["accounts"].([]any)[0].(map[string]any)["available_balance"].(map[string]any)["value"] = "969.99"
			}
		}},
		mutation{"base changes between traversals", func(r *http.Request, b map[string]any, state settlementTestReads) {
			if strings.HasSuffix(r.URL.Path, "accounts") && state.AccountPass == 2 && r.URL.Query().Get("cursor") != "" {
				b["accounts"].([]any)[0].(map[string]any)["available_balance"].(map[string]any)["value"] = "1.00100002"
			}
		}},
		mutation{"stable unrelated cash change", func(r *http.Request, b map[string]any, _ settlementTestReads) {
			if strings.HasSuffix(r.URL.Path, "accounts") && r.URL.Query().Get("cursor") == "" {
				b["accounts"].([]any)[0].(map[string]any)["available_balance"].(map[string]any)["value"] = "971"
			}
		}},
		mutation{"stable unrelated inventory change", func(r *http.Request, b map[string]any, _ settlementTestReads) {
			if strings.HasSuffix(r.URL.Path, "accounts") && r.URL.Query().Get("cursor") != "" {
				b["accounts"].([]any)[0].(map[string]any)["available_balance"].(map[string]any)["value"] = "1.00100002"
			}
		}},
		mutation{"missing continuation", func(r *http.Request, b map[string]any, _ settlementTestReads) {
			if strings.HasSuffix(r.URL.Path, "accounts") && r.URL.Query().Get("cursor") != "" {
				b["accounts"] = []any{}
				b["has_next"], b["cursor"] = true, "page-3"
			}
		}},
		mutation{"cursor loop", func(r *http.Request, b map[string]any, _ settlementTestReads) {
			if strings.HasSuffix(r.URL.Path, "accounts") {
				b["has_next"], b["cursor"] = true, "page-2"
			}
		}},
	)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newSettlementTestHarness(t, "BUY", "FILLED", false, tc.mutate)
			e, err := h.collect()
			if err == nil || !reflect.DeepEqual(e, execution.AccountSettlementEvidence{}) {
				t.Fatalf("unsafe account evidence escaped: %#v %v", e, err)
			}
		})
	}
}

func TestExecutionSettlementRequiresBothCompleteEmptyUnresolvedOrderReads(t *testing.T) {
	for _, pass := range []int{1, 2} {
		for _, problem := range []string{"open", "missing orders", "missing has_next", "missing cursor", "incomplete", "proof required"} {
			t.Run(fmt.Sprintf("pass%d-%s", pass, problem), func(t *testing.T) {
				h := newSettlementTestHarness(t, "BUY", "FILLED", false, func(r *http.Request, b map[string]any, state settlementTestReads) {
					if !strings.HasSuffix(r.URL.Path, "batch") || state.OpenOrders != pass {
						return
					}
					switch problem {
					case "open":
						b["orders"] = []any{map[string]any{"order_id": submissionTestProviderID}}
					case "missing orders":
						delete(b, "orders")
					case "missing has_next":
						delete(b, "has_next")
					case "missing cursor":
						delete(b, "cursor")
					case "incomplete":
						b["has_next"], b["cursor"] = true, "more"
					case "proof required":
						b["proof_token_required"] = true
					}
				})
				e, err := h.collect()
				if err == nil || !reflect.DeepEqual(e, execution.AccountSettlementEvidence{}) {
					t.Fatalf("unsafe unresolved orders escaped: %#v %v", e, err)
				}
				reads, _ := h.snapshot()
				if pass == 1 && reads.AccountPages != 0 {
					t.Fatal("account collection continued after known unresolved order")
				}
			})
		}
	}
}

func TestExecutionSettlementRequiresFreshScopedPermissionAndTerminalObservation(t *testing.T) {
	for _, status := range []string{"PENDING", "OPEN", "QUEUED", "CANCEL_QUEUED", "EDIT_QUEUED", "UNKNOWN_ORDER_STATUS"} {
		t.Run(status, func(t *testing.T) {
			h := newSettlementTestHarness(t, "BUY", status, true, nil)
			if e, err := h.collect(); err == nil || !reflect.DeepEqual(e, execution.AccountSettlementEvidence{}) {
				t.Fatalf("nonterminal order accepted: %#v %v", e, err)
			}
			reads, _ := h.snapshot()
			if reads.AccountPages != 0 || reads.OpenOrders != 0 {
				t.Fatal("nonterminal observation reached account settlement reads")
			}
		})
	}
	for _, field := range []string{"can_view", "can_transfer", "portfolio_uuid"} {
		t.Run(field, func(t *testing.T) {
			h := newSettlementTestHarness(t, "BUY", "FILLED", false, func(r *http.Request, b map[string]any, _ settlementTestReads) {
				if strings.HasSuffix(r.URL.Path, "key_permissions") {
					switch field {
					case "can_view":
						b[field] = false
					case "can_transfer":
						b[field] = true
					default:
						b[field] = submissionTestProviderID
					}
				}
			})
			if e, err := h.collect(); err == nil || !reflect.DeepEqual(e, execution.AccountSettlementEvidence{}) {
				t.Fatalf("unsafe recovery permission accepted: %#v %v", e, err)
			}
			reads, _ := h.snapshot()
			if reads.Details != 0 || reads.AccountPages != 0 {
				t.Fatal("unsafe key reached account/order reads")
			}
		})
	}
}

func TestExecutionSettlementNoUsableEvidenceOnCanceledOrExpiredOperation(t *testing.T) {
	t.Run("parent cancellation bounds in-flight account read", func(t *testing.T) {
		h := newSettlementTestHarness(t, "BUY", "FILLED", false, func(r *http.Request, _ map[string]any, _ settlementTestReads) {
			if strings.HasSuffix(r.URL.Path, "accounts") {
				<-r.Context().Done()
			}
		})
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		started := time.Now()
		e, err := h.adapter.CollectAccountSettlement(ctx, h.key, h.s, h.a)
		if err == nil || !reflect.DeepEqual(e, execution.AccountSettlementEvidence{}) || time.Since(started) > time.Second {
			t.Fatalf("cancellation did not bound all reads: %#v %v", e, err)
		}
	})
	t.Run("total operation exceeds 15 seconds", func(t *testing.T) {
		var h *settlementTestHarness
		h = newSettlementTestHarness(t, "BUY", "FILLED", false, func(r *http.Request, _ map[string]any, state settlementTestReads) {
			if strings.HasSuffix(r.URL.Path, "batch") && state.OpenOrders == 2 {
				h.clock.Store(16000)
			}
		})
		e, err := h.collect()
		if err == nil || !reflect.DeepEqual(e, execution.AccountSettlementEvidence{}) {
			t.Fatalf("expired operation returned settlement proof: %#v %v", e, err)
		}
	})
}
