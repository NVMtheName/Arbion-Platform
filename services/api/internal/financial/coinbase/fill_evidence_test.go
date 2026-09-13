package coinbase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

const exactFillJSON = `{"entry_id":"entry-a","trade_id":"trade-a","order_id":"order-a","retail_portfolio_id":"portfolio-123","trade_time":"2026-09-01T14:59:00.123456789Z","trade_type":"FILL","price":"60123.123456789","size":"0.000000010000","commission":"0.00000001","product_id":"BTC-USD","sequence_timestamp":"2026-09-01T14:59:01.987654321Z","liquidity_indicator":"MAKER","size_in_quote":false,"side":"BUY"}`

func evidenceClient(t *testing.T, respond func(*http.Request) (int, string)) (*Client, financial.Credentials, *atomic.Int32) {
	t.Helper()
	credentials, key := testCredentials(t)
	credentials.PortfolioID = "portfolio-123"
	count := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verifyJWT(t, r, key, r.URL.Path)
		if r.Method != http.MethodGet {
			t.Error("evidence attempted a non-GET request")
			w.WriteHeader(405)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v3/brokerage/key_permissions" {
			_, _ = w.Write([]byte(`{"can_view":true,"can_trade":false,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`))
			return
		}
		if r.URL.Path != "/api/v3/brokerage/orders/historical/fills" {
			t.Error("unexpected endpoint")
			w.WriteHeader(404)
			return
		}
		count.Add(1)
		code, body := respond(r)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	c, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return time.Date(2026, 9, 1, 15, 0, 0, 0, time.UTC) }
	return c, credentials, count
}

func evidenceWindow() financial.FillEvidenceWindow {
	end := time.Date(2026, 9, 1, 15, 0, 0, 0, time.UTC)
	return financial.FillEvidenceWindow{From: end.Add(-time.Hour), To: end}
}

func TestPrivateFillReaderPaginatesAndDeduplicatesWithoutLeaking(t *testing.T) {
	c, credentials, count := evidenceClient(t, func(r *http.Request) (int, string) {
		q := r.URL.Query()
		if q.Get("limit") != "100" || q.Get("product_types") != "SPOT" || q.Has("sort_by") || q.Get("start_sequence_timestamp") != "2026-09-01T14:00:00Z" || q.Get("end_sequence_timestamp") != "2026-09-01T15:00:00Z" {
			t.Error("unsafe history query", q)
		}
		if q.Get("cursor") == "" {
			return 200, `{"fills":[` + exactFillJSON + `],"cursor":"next-private"}`
		}
		if q.Get("cursor") != "next-private" {
			t.Error("cursor changed")
		}
		second := strings.ReplaceAll(exactFillJSON, "entry-a", "entry-b")
		return 200, `{"fills":[` + exactFillJSON + `,` + second + `],"cursor":""}`
	})
	scan, err := c.ReadFillEvidence(context.Background(), &credentials, "portfolio:portfolio-123", evidenceWindow())
	if err != nil || count.Load() != 2 || scan.Pages != 2 || !scan.PaginationExhausted || scan.Page.HasMore || len(scan.Page.Fills) != 2 {
		t.Fatalf("bad scan %+v %v", scan, err)
	}
	f := scan.Page.Fills[0]
	if f.Fill.Size != "0.00000001" || f.Fill.Price != "60123.123456789" || f.Fill.TradeTime.Nanosecond() != 123456789 || f.SequenceTime.Nanosecond() != 987654321 {
		t.Fatal("precision lost", f)
	}
	encoded, err := json.Marshal(scan)
	if err != nil || string(encoded) != "{}" {
		t.Fatal("private scan escaped", string(encoded), err)
	}
}

func TestPrivateFillReaderFailsClosedOnIncompleteOrConflictingPages(t *testing.T) {
	for _, name := range []string{"loop", "conflict", "foreign portfolio", "missing identity", "missing units", "missing fills", "proof required", "late error", "outside window", "future sequence"} {
		t.Run(name, func(t *testing.T) {
			c, credentials, _ := evidenceClient(t, func(r *http.Request) (int, string) {
				if r.URL.Query().Get("cursor") == "" {
					return 200, `{"fills":[` + exactFillJSON + `],"cursor":"next"}`
				}
				fill := exactFillJSON
				cursor := ""
				switch name {
				case "loop":
					cursor = "next"
				case "conflict":
					fill = strings.ReplaceAll(fill, `"commission":"0.00000001"`, `"commission":"0.00000002"`)
				case "foreign portfolio":
					fill = strings.ReplaceAll(fill, "portfolio-123", "portfolio-other")
				case "missing identity":
					fill = strings.ReplaceAll(fill, `"entry_id":"entry-a",`, "")
				case "missing units":
					fill = strings.ReplaceAll(fill, `"size_in_quote":false,`, "")
				case "missing fills":
					return 200, `{}`
				case "proof required":
					return 200, `{"fills":[],"proof_token_required":true}`
				case "late error":
					return 503, `{"error":"private-response-not-for-owner"}`
				case "outside window":
					fill = strings.ReplaceAll(fill, "14:59:00.123456789", "13:59:00.123456789")
				case "future sequence":
					fill = strings.ReplaceAll(fill, "14:59:01.987654321", "15:59:01.987654321")
				}
				return 200, `{"fills":[` + fill + `],"cursor":"` + cursor + `"}`
			})
			scan, err := c.ReadFillEvidence(context.Background(), &credentials, "portfolio:portfolio-123", evidenceWindow())
			if err == nil || scan.Pages != 0 || len(scan.Page.Fills) != 0 || strings.Contains(err.Error(), "private-response") {
				t.Fatal("invalid page returned usable evidence", scan, err)
			}
		})
	}
}

func TestPrivateFillReaderHardCapsAndRejectsBadAccountBeforeRead(t *testing.T) {
	n := 0
	c, credentials, count := evidenceClient(t, func(r *http.Request) (int, string) {
		n++
		return 200, fmt.Sprintf(`{"fills":[],"cursor":"page-%d"}`, n)
	})
	scan, err := c.ReadFillEvidence(context.Background(), &credentials, "portfolio:portfolio-123", evidenceWindow())
	if err != nil || count.Load() != 10 || scan.Pages != 10 || scan.PaginationExhausted || !scan.Page.HasMore {
		t.Fatal("pagination cap/completeness wrong", scan, err)
	}
	if _, err := c.ReadFillEvidence(context.Background(), &credentials, "portfolio:other", evidenceWindow()); err == nil || count.Load() != 10 {
		t.Fatal("foreign account fetched history", err)
	}
	w := evidenceWindow()
	w.From = w.To.Add(-25 * time.Hour)
	if _, err := c.ReadFillEvidence(context.Background(), &credentials, "portfolio:portfolio-123", w); err == nil || count.Load() != 10 {
		t.Fatal("unbounded history read")
	}
}

func TestDisplayHistoryRetainsPrivateEvidenceOnlyWhenExact(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			c, credentials, count := evidenceClient(t, func(r *http.Request) (int, string) {
				fill := exactFillJSON
				if !valid {
					fill = strings.ReplaceAll(fill, `"entry_id":"entry-a",`, "")
				}
				return 200, `{"fills":[` + fill + `],"cursor":""}`
			})
			page, err := c.GetTradeFills(context.Background(), &credentials, "portfolio:portfolio-123", 50)
			if err != nil || count.Load() != 1 || len(page.Fills) != 1 || (page.Evidence != nil) != valid {
				t.Fatal("display was disrupted or unsafe evidence captured", page, err)
			}
			encoded, _ := json.Marshal(page)
			if strings.Contains(string(encoded), "entry-a") || strings.Contains(string(encoded), "reference") || strings.Contains(string(encoded), "portfolio") {
				t.Fatal("private evidence leaked", string(encoded))
			}
		})
	}
}

func TestDisplayFillHistoryRejectsForeignPortfolio(t *testing.T) {
	c, credentials, _ := evidenceClient(t, func(*http.Request) (int, string) {
		return 200, `{"fills":[` + strings.ReplaceAll(exactFillJSON, "portfolio-123", "another-portfolio") + `],"cursor":""}`
	})
	page, err := c.GetTradeFills(context.Background(), &credentials, "portfolio:portfolio-123", 50)
	if err == nil || len(page.Fills) != 0 || page.Evidence != nil {
		t.Fatal("foreign portfolio exposed", page, err)
	}
}

func TestPrivateFillReaderPreservesQuoteSizeAndUnknownCommissionUnit(t *testing.T) {
	c, credentials, _ := evidenceClient(t, func(*http.Request) (int, string) {
		return 200, `{"fills":[` + strings.ReplaceAll(exactFillJSON, `"size_in_quote":false`, `"size_in_quote":true`) + `],"cursor":""}`
	})
	scan, err := c.ReadFillEvidence(context.Background(), &credentials, "portfolio:portfolio-123", evidenceWindow())
	if err != nil || len(scan.Page.Fills) != 1 {
		t.Fatal("valid quote-size fill rejected", err)
	}
	e := scan.Page.Fills[0]
	if e.Fill.SizeUnit != "USD" || e.Fill.Size != "0.00000001" || e.Fill.Commission.Currency != "" || e.CommissionCurrencyStatus != "UNAVAILABLE" {
		t.Fatal("quantity or fee currency inferred", e)
	}
}
