package coinbase

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/financial"
)

func reconciliationFillJSON(s execution.ConfirmedSubmission, at time.Time, id, size, fee string, quote bool) map[string]any {
	return map[string]any{"entry_id": "entry-" + id, "trade_id": "trade-" + id, "order_id": submissionTestProviderID,
		"retail_portfolio_id": s.PortfolioID, "product_id": s.Order.Request.ProductID, "side": s.Order.Request.Side, "trade_type": "FILL",
		"trade_time": at.Format(time.RFC3339Nano), "sequence_timestamp": at.Add(-time.Second).Format(time.RFC3339Nano),
		"price": "30000", "size": size, "commission": fee, "size_in_quote": quote}
}

func reconciliationOrderJSON(s execution.ConfirmedSubmission, claim time.Time, status string, count int, qty, gross, fee string, latest time.Time) map[string]any {
	var body map[string]any
	_ = json.Unmarshal([]byte(submissionDetailJSON(s, claim)), &body)
	o := body["order"].(map[string]any)
	o["status"], o["number_of_fills"], o["filled_size"], o["filled_value"], o["total_fees"] = status, fmt.Sprint(count), qty, gross, fee
	o["pending_cancel"], o["settled"] = false, status == "FILLED" || status == "CANCELLED" || status == "EXPIRED" || status == "FAILED"
	if !latest.IsZero() {
		o["last_fill_time"] = latest.Format(time.RFC3339Nano)
	}
	return body
}

type reconciliationHarness struct {
	adapter *ExecutionAdapter
	key     *financial.Credentials
	s       execution.ConfirmedSubmission
	a       execution.Attempt
	reads   atomic.Int32
}

type reconciliationMutation func(*http.Request, map[string]any, int, int)

func newReconciliationHarness(t *testing.T, side, status string, partial, quote bool, mutate reconciliationMutation) *reconciliationHarness {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	s, a := submissionFixture(now, side)
	a.ProviderOrderID = submissionTestProviderID
	h := &reconciliationHarness{s: s, a: a}
	credentials, signingKey := testCredentials(t)
	credentials.PortfolioID = s.PortfolioID
	h.key = &credentials
	var detailCount, fillCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.reads.Add(1)
		verifyJWT(t, r, signingKey, r.URL.Path)
		if r.Method != http.MethodGet {
			t.Errorf("collector attempted %s", r.Method)
			w.WriteHeader(405)
			return
		}
		var body map[string]any
		first, last := now.Add(10*time.Millisecond), now.Add(20*time.Millisecond)
		switch r.URL.Path {
		case "/api/v3/brokerage/key_permissions":
			body = map[string]any{"can_view": true, "can_trade": false, "can_transfer": false, "portfolio_uuid": s.PortfolioID}
		case executionHistoryPath + submissionTestProviderID:
			detailCount++
			if status == "FAILED" || status == "EXPIRED" {
				body = reconciliationOrderJSON(s, now, status, 0, "0", "0", "0", time.Time{})
			} else if partial {
				body = reconciliationOrderJSON(s, now, status, 1, "0.0004", "12", "0.04", first)
			} else {
				body = reconciliationOrderJSON(s, now, status, 2, "0.001", "30", "0.10", last)
			}
		case executionHistoryPath + "fills":
			fillCount++
			if r.URL.Query().Get("order_ids") != a.ProviderOrderID || len(r.URL.Query()["order_ids"]) != 1 || r.URL.Query().Get("limit") != "100" || r.URL.Query().Has("retail_portfolio_id") {
				t.Error("wrong fill query scope/bound")
			}
			size1, size2 := "0.0004", "0.0006"
			if quote {
				size1, size2 = "12", "18"
			}
			if status == "FAILED" || status == "EXPIRED" {
				body = map[string]any{"fills": []any{}, "cursor": ""}
			} else if partial {
				body = map[string]any{"fills": []any{reconciliationFillJSON(s, first, "1", size1, "0.04", quote)}, "cursor": ""}
			} else if r.URL.Query().Get("cursor") == "" {
				body = map[string]any{"fills": []any{reconciliationFillJSON(s, last, "2", size2, "0.06", quote)}, "cursor": "page-2"}
			} else {
				body = map[string]any{"fills": []any{reconciliationFillJSON(s, first, "1", size1, "0.04", quote)}, "cursor": ""}
			}
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if mutate != nil {
			mutate(r, body, detailCount, fillCount)
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(server.Close)
	c, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return now.Add(time.Second) }
	h.adapter = NewExecutionAdapter(c)
	return h
}

func (h *reconciliationHarness) collect() (execution.BrokerObservation, error) {
	return h.adapter.CollectExecutionObservation(context.Background(), h.key, h.s, h.a)
}

func TestExecutionObservationExactBuySellAndQuoteSize(t *testing.T) {
	for _, side := range []string{"BUY", "SELL"} {
		for _, quote := range []bool{false, true} {
			for _, partial := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s-quote%v-partial%v", side, quote, partial), func(t *testing.T) {
					status := "FILLED"
					if partial {
						status = "OPEN"
					}
					h := newReconciliationHarness(t, side, status, partial, quote, nil)
					obs, err := h.collect()
					if err != nil {
						t.Fatal(err)
					}
					wantCount := 2
					if partial {
						wantCount = 1
					}
					if obs.Status != status || !obs.CompleteFills || len(obs.Fills) != wantCount || obs.FillCount != int64(wantCount) || obs.BrokerIdentity.Side != side {
						t.Fatalf("observation mismatch: %#v", obs)
					}
					if obs.Fills[0].TradeID != "trade-1" || obs.Fills[0].BaseQuantity != "0.0004" || obs.Fills[0].GrossUSD != "12" || obs.Fills[0].FeeUSD != "0.04" {
						t.Fatalf("not exactly normalized/sorted: %#v", obs.Fills)
					}
					for _, f := range obs.Fills {
						p := f.ProviderEvidence
						if p == nil || p.EntryID == "" || p.SizeInQuote != quote || p.FeeCurrency != "USD" || p.FeeCurrencyBasis != "COINBASE_ADVANCED_QUOTE_ASSET_1_91" || !p.SequenceAt.Before(f.TradedAt) {
							t.Fatalf("lost unit/fee/sequence evidence: %#v", p)
						}
					}
					if partial && h.reads.Load() != 4 || !partial && h.reads.Load() != 5 {
						t.Fatal("unexpected read count")
					}
				})
			}
		}
	}
}

func TestExecutionObservationCancelExpiryAndReject(t *testing.T) {
	for _, status := range []string{"CANCELLED", "EXPIRED", "FAILED"} {
		t.Run(status, func(t *testing.T) {
			h := newReconciliationHarness(t, "BUY", status, true, false, nil)
			obs, err := h.collect()
			if err != nil {
				t.Fatal(err)
			}
			want := status
			if want == "FAILED" {
				want = "REJECTED"
			}
			if obs.Status != want || !obs.CompleteFills {
				t.Fatalf("bad status: %#v", obs)
			}
			if status == "CANCELLED" && (obs.BaseQuantity != "0.0004" || obs.FillCount != 1) {
				t.Fatal("cancel lost partial fill")
			}
		})
	}
}

func TestExecutionObservationFailsClosed(t *testing.T) {
	type mutation struct {
		name   string
		mutate reconciliationMutation
	}
	cases := []mutation{}
	for _, field := range []string{"status", "number_of_fills", "filled_size", "filled_value", "total_fees", "pending_cancel", "settled", "last_fill_time"} {
		field := field
		cases = append(cases, mutation{"missing order " + field, func(r *http.Request, b map[string]any, _, _ int) {
			if strings.HasSuffix(r.URL.Path, submissionTestProviderID) {
				delete(b["order"].(map[string]any), field)
			}
		}})
	}
	for _, tc := range []struct {
		field string
		value any
	}{
		{"status", "UNKNOWN_ORDER_STATUS"}, {"status", "FAILED"}, {"settled", false}, {"pending_cancel", true},
		{"filled_size", "0.0009"}, {"filled_value", "29.99"}, {"total_fees", "0.09"}, {"number_of_fills", "1"},
		{"last_fill_time", "2020-01-01T00:00:00Z"}, {"number_of_fills", "2e0"}, {"filled_value", "3e1"},
		{"retail_portfolio_id", submissionTestProviderID}, {"side", "SELL"}, {"product_type", "FUTURE"},
	} {
		tc := tc
		cases = append(cases, mutation{"order " + tc.field + fmt.Sprint(tc.value), func(r *http.Request, b map[string]any, _, _ int) {
			if strings.HasSuffix(r.URL.Path, submissionTestProviderID) {
				b["order"].(map[string]any)[tc.field] = tc.value
			}
		}})
	}
	for _, tc := range []struct {
		field string
		value any
	}{
		{"entry_id", ""}, {"trade_id", "bad space"}, {"trade_type", "REVERSAL"}, {"retail_portfolio_id", submissionTestProviderID},
		{"order_id", executionTestPortfolio}, {"product_id", "ETH-USD"}, {"side", "SELL"}, {"product_type", "FUTURE"},
		{"commission", "-0.01"}, {"commission_currency", "BTC"}, {"fee_currency", "EUR"}, {"commission_detail_total", map[string]any{"total_commission": "0.05"}},
		{"size", "0.0000000000000000001"}, {"price", "0"}, {"price", "30001"}, {"size_in_quote", nil},
		{"trade_time", "2020-01-01T00:00:00Z"}, {"trade_time", "2999-01-01T00:00:00Z"}, {"sequence_timestamp", "0001-01-01T00:00:00Z"},
		{"sequence_timestamp", "2999-01-01T00:00:00Z"}, {"future_legs", []any{map[string]any{"product_id": "BTC-PERP"}}},
	} {
		tc := tc
		cases = append(cases, mutation{"fill " + tc.field + fmt.Sprint(tc.value), func(r *http.Request, b map[string]any, _, _ int) {
			if strings.HasSuffix(r.URL.Path, "fills") {
				b["fills"].([]any)[0].(map[string]any)[tc.field] = tc.value
			}
		}})
	}
	for _, tc := range []struct {
		field string
		value any
	}{{"portfolio_uuid", submissionTestProviderID}, {"can_transfer", true}, {"can_view", false}} {
		tc := tc
		cases = append(cases, mutation{"permission " + tc.field, func(r *http.Request, b map[string]any, _, _ int) {
			if strings.HasSuffix(r.URL.Path, "key_permissions") {
				b[tc.field] = tc.value
			}
		}})
	}
	cases = append(cases,
		mutation{"missing cursor", func(r *http.Request, b map[string]any, _, _ int) {
			if strings.HasSuffix(r.URL.Path, "fills") {
				delete(b, "cursor")
			}
		}},
		mutation{"missing fills", func(r *http.Request, b map[string]any, _, _ int) {
			if strings.HasSuffix(r.URL.Path, "fills") {
				delete(b, "fills")
			}
		}},
		mutation{"null fills", func(r *http.Request, b map[string]any, _, _ int) {
			if strings.HasSuffix(r.URL.Path, "fills") {
				b["fills"] = nil
			}
		}},
		mutation{"requires proof", func(r *http.Request, b map[string]any, _, _ int) {
			if strings.HasSuffix(r.URL.Path, "fills") {
				b["proof_token_required"] = true
			}
		}},
		mutation{"order requires proof", func(r *http.Request, b map[string]any, _, _ int) {
			if strings.HasSuffix(r.URL.Path, submissionTestProviderID) {
				b["proof_token_required"] = true
			}
		}},
		mutation{"cursor loop", func(r *http.Request, b map[string]any, _, _ int) {
			if strings.HasSuffix(r.URL.Path, "fills") {
				b["cursor"] = "page-2"
			}
		}},
		mutation{"empty continuation", func(r *http.Request, b map[string]any, _, _ int) {
			if strings.HasSuffix(r.URL.Path, "fills") {
				b["fills"] = []any{}
			}
		}},
		mutation{"duplicate trade", func(r *http.Request, b map[string]any, _, _ int) {
			if strings.HasSuffix(r.URL.Path, "fills") {
				b["fills"].([]any)[0].(map[string]any)["trade_id"] = "trade-same"
			}
		}},
		mutation{"duplicate entry", func(r *http.Request, b map[string]any, _, _ int) {
			if strings.HasSuffix(r.URL.Path, "fills") {
				b["fills"].([]any)[0].(map[string]any)["entry_id"] = "entry-same"
			}
		}},
		mutation{"fraction cannot terminate", func(r *http.Request, b map[string]any, _, _ int) {
			if strings.HasSuffix(r.URL.Path, "fills") {
				f := b["fills"].([]any)[0].(map[string]any)
				f["size_in_quote"], f["size"], f["price"] = true, "1", "3"
			}
		}},
	)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newReconciliationHarness(t, "BUY", "FILLED", false, false, tc.mutate)
			obs, err := h.collect()
			if err == nil || !reflect.DeepEqual(obs, execution.BrokerObservation{}) {
				t.Fatalf("invalid evidence accepted: %#v %v", obs, err)
			}
		})
	}
}

func TestExecutionObservationChangingSnapshotsCannotSettle(t *testing.T) {
	for _, field := range []string{"status", "filled_size", "filled_value", "total_fees", "number_of_fills", "pending_cancel", "settled", "last_fill_time", "created_time"} {
		t.Run(field, func(t *testing.T) {
			h := newReconciliationHarness(t, "BUY", "OPEN", true, false, func(r *http.Request, b map[string]any, detail, _ int) {
				if !strings.HasSuffix(r.URL.Path, submissionTestProviderID) || detail != 2 {
					return
				}
				o := b["order"].(map[string]any)
				switch field {
				case "status":
					o[field] = "CANCEL_QUEUED"
				case "filled_size":
					o[field] = "0.0005"
				case "filled_value":
					o[field] = "15"
				case "total_fees":
					o[field] = "0.05"
				case "number_of_fills":
					o[field] = "2"
				case "pending_cancel", "settled":
					o[field] = true
				default:
					at, _ := time.Parse(time.RFC3339Nano, o[field].(string))
					o[field] = at.Add(time.Millisecond).Format(time.RFC3339Nano)
				}
			})
			if _, err := h.collect(); err == nil {
				t.Fatal("changing snapshot accepted")
			}
		})
	}
}

func TestExecutionObservationCancelFillRaceCannotDeclareCompletion(t *testing.T) {
	h := newReconciliationHarness(t, "BUY", "OPEN", true, false, func(r *http.Request, b map[string]any, detail, _ int) {
		if !strings.HasSuffix(r.URL.Path, submissionTestProviderID) || detail != 2 {
			return
		}
		o := b["order"].(map[string]any)
		o["status"], o["settled"] = "CANCELLED", true
		o["number_of_fills"], o["filled_size"], o["filled_value"], o["total_fees"] = "2", "0.001", "30", "0.10"
	})
	obs, err := h.collect()
	if err == nil || !reflect.DeepEqual(obs, execution.BrokerObservation{}) {
		t.Fatal("late fill during cancellation became false terminal evidence")
	}
}

func TestExecutionObservationPageBoundAndCanceledContext(t *testing.T) {
	t.Run("bounded pagination", func(t *testing.T) {
		h := newReconciliationHarness(t, "BUY", "OPEN", true, false, func(r *http.Request, b map[string]any, _, page int) {
			if strings.HasSuffix(r.URL.Path, "fills") {
				b["cursor"] = fmt.Sprintf("page-%d", page)
				f := b["fills"].([]any)[0].(map[string]any)
				f["trade_id"], f["entry_id"] = fmt.Sprintf("trade-%d", page), fmt.Sprintf("entry-%d", page)
			}
		})
		if _, err := h.collect(); err == nil || h.reads.Load() != 12 {
			t.Fatalf("scan did not stop at ten pages: %d %v", h.reads.Load(), err)
		}
	})
	t.Run("canceled context", func(t *testing.T) {
		h := newReconciliationHarness(t, "BUY", "OPEN", true, false, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := h.adapter.CollectExecutionObservation(ctx, h.key, h.s, h.a); err == nil || h.reads.Load() != 0 {
			t.Fatal("canceled collection read provider")
		}
	})
}

func TestExactExecutionAmountDoesNotRound(t *testing.T) {
	for _, tc := range []struct {
		input string
		scale int
		want  string
	}{{"0", 18, "0"}, {"1/3", 18, ""}, {"1/1000000000000000000", 18, "0.000000000000000001"}, {"1/10000000000000000000", 18, ""}, {"30000", 36, "30000"}} {
		n, _ := new(big.Rat).SetString(tc.input)
		if got := exactExecutionAmount(n, tc.scale); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.input, got, tc.want)
		}
	}
}
