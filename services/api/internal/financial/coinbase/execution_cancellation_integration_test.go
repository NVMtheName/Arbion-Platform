package coinbase

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/execution"
)

// Cancellation acknowledgement is not finality: a second fill can win the
// race, including when the cancellation response is lost. Only verified order
// history and account balances release the original fences, without retrying.
func TestPostgresCoinbaseCancellationFillRaceAndSettlement(t *testing.T) {
	ctx, pool := setupCoinbaseExecutionIntegration(t)
	for _, loseResponse := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "lost response"}[loseResponse], func(t *testing.T) {
			order, generation := prepareCoinbaseExecutionIntegration(t, ctx, pool)
			r := order.Request
			credentials, key := testCredentials(t)
			credentials.PortfolioID = r.AccountID
			vault := integrationExecutionVault{request: r, credentials: credentials, generation: generation}
			var providerID string
			if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&providerID); err != nil {
				t.Fatal(err)
			}
			fixtures := executionTestFixtures(time.Now().UTC())
			for path, body := range fixtures {
				fixtures[path] = strings.ReplaceAll(body, executionTestPortfolio, r.AccountID)
			}
			submission := execution.ConfirmedSubmission{Order: order, PortfolioID: r.AccountID}
			identity := strings.ReplaceAll(submissionIdentityJSON(submission), submissionTestProviderID, providerID)
			var creates, cancellations, previews, requests atomic.Int32
			var acceptedAt, secondFillAt atomic.Int64
			var final atomic.Bool
			fill := func(second bool) map[string]any {
				at, id, size, fee := acceptedAt.Load(), "one", "0.0004", "0.04"
				if second {
					at, id, size, fee = secondFillAt.Load(), "two", "0.0006", "0.06"
				}
				body := reconciliationFillJSON(submission, time.Unix(0, at).UTC(), id, size, fee, false)
				body["order_id"] = providerID
				return body
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				requests.Add(1)
				verifyJWT(t, req, key, req.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				path := req.URL.Path
				switch path {
				case "/api/v3/brokerage/orders":
					if req.Method != http.MethodPost {
						t.Error("creation must be POST")
					}
					creates.Add(1)
					acceptedAt.Store(time.Now().UTC().Truncate(time.Microsecond).UnixNano())
					_, _ = io.WriteString(w, `{"success":true,"success_response":`+identity+`}`)
					return
				case executionCancellationPath:
					var body struct {
						OrderIDs []string `json:"order_ids"`
					}
					if req.Method != http.MethodPost || json.NewDecoder(req.Body).Decode(&body) != nil || len(body.OrderIDs) != 1 || body.OrderIDs[0] != providerID {
						t.Error("cancellation must target only the exact original order")
					}
					cancellations.Add(1)
					secondFillAt.Store(time.Now().UTC().Truncate(time.Microsecond).UnixNano())
					if loseResponse {
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
							return
						}
						_ = conn.Close()
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"order_id": providerID, "success": true}}})
					return
				case executionHistoryPath + "batch":
					if req.Method != http.MethodGet || !req.URL.Query().Has("order_status") || req.URL.Query().Get("retail_portfolio_id") != r.AccountID {
						t.Error("order scan must be scoped and read only")
					}
					_, _ = io.WriteString(w, `{"orders":[],"has_next":false,"cursor":""}`)
					return
				case executionHistoryPath + providerID:
					if req.Method != http.MethodGet {
						t.Error("detail must be read only")
					}
					at := time.Unix(0, acceptedAt.Load()).UTC()
					body := reconciliationOrderJSON(submission, at, "OPEN", 1, "0.0004", "12", "0.04", at)
					if final.Load() {
						body = reconciliationOrderJSON(submission, at, "FILLED", 2, "0.001", "30", "0.10", time.Unix(0, secondFillAt.Load()).UTC())
					}
					body["order"].(map[string]any)["order_id"] = providerID
					_ = json.NewEncoder(w).Encode(body)
					return
				case executionHistoryPath + "fills":
					query := req.URL.Query()
					if req.Method != http.MethodGet || len(query["order_ids"]) != 1 || query.Get("order_ids") != providerID || query.Get("limit") != "100" || query.Get("cursor") != "" {
						t.Error("fill scan must be bounded to the original order")
					}
					fills := []any{fill(false)}
					if final.Load() {
						fills = append(fills, fill(true))
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"fills": fills, "cursor": ""})
					return
				case "/api/v3/brokerage/orders/preview":
					previews.Add(1)
					if req.Method != http.MethodPost {
						t.Error("preview must be POST")
					}
				case "/api/v3/brokerage/accounts":
					if cursor := req.URL.Query().Get("cursor"); cursor != "" {
						path += "?" + cursor
					}
				}
				if req.URL.Path != "/api/v3/brokerage/orders/preview" && req.Method != http.MethodGet {
					t.Error("unexpected synthetic broker write")
				}
				body, ok := fixtures[path]
				if !ok {
					t.Errorf("unexpected synthetic endpoint %s", path)
					http.NotFound(w, req)
					return
				}
				if final.Load() && req.URL.Path == "/api/v3/brokerage/accounts" {
					body = strings.ReplaceAll(body, "1000.10", "970")
					body = strings.ReplaceAll(body, "1.00000001", "1.00100001")
				}
				_, _ = io.WriteString(w, body)
			}))
			t.Cleanup(server.Close)
			client, err := New(Config{BaseURL: server.URL}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			adapter := NewExecutionAdapter(client)
			store := execution.NewPostgresStore(pool)
			evidence, err := store.CapturePreflight(ctx, r.OwnerID, order.ID, vault, client)
			if err != nil {
				t.Fatal("preflight", err)
			}
			if attempt, err := store.SendConfirmed(ctx, r.OwnerID, order.ID, evidence, vault, adapter); err != nil || attempt.ProviderOrderID != providerID {
				t.Fatal("original synthetic send", err)
			}
			partial, err := store.ReconcileBrokerOrder(ctx, r.OwnerID, order.ID, vault, adapter)
			if err != nil {
				t.Fatal("first partial fill", err)
			}
			assertCoinbaseReconciliation(t, partial, 1, "0.0004", "12", "0.04", "", true)
			cancelled, err := store.CancelBrokerOrder(ctx, r.OwnerID, order.ID, vault, adapter)
			outcome, wantErr := "ACCEPTED", error(nil)
			if loseResponse {
				outcome, wantErr = "UNKNOWN", execution.ErrCancellationUnknown
			}
			if !errors.Is(err, wantErr) || cancelled.Outcome != outcome || cancelled.ProviderOrderID != providerID || cancelled.ReceivedAt == nil || cancellations.Load() != 1 {
				t.Fatal("incorrect durable cancellation result", err, cancelled, cancellations.Load())
			}
			assertCoinbaseReconciliationState(t, ctx, pool, order, partial, 0)
			reads := requests.Load()
			replayed, err := execution.NewPostgresStore(pool).CancelBrokerOrder(ctx, r.OwnerID, order.ID, vault, adapter)
			if !errors.Is(err, wantErr) || !reflect.DeepEqual(replayed, cancelled) || requests.Load() != reads {
				t.Fatal("restart retried cancellation", err)
			}
			final.Store(true)
			terminal, err := execution.NewPostgresStore(pool).ReconcileBrokerOrder(ctx, r.OwnerID, order.ID, vault, adapter)
			if err != nil {
				t.Fatal("fill racing accepted or unknown cancellation", err)
			}
			assertCoinbaseReconciliation(t, terminal, 2, "0.001", "30", "0.1", "FILLED", false)
			assertCoinbaseReconciliationState(t, ctx, pool, order, terminal, 1)
			before, err := store.ReadCapitalReservation(ctx, r.OwnerID, order.ID)
			if err != nil || before.ReleasedAt != nil {
				t.Fatal("capital released before exact account settlement", err)
			}
			settlement, err := store.SettleBrokerAccount(ctx, r.OwnerID, order.ID, vault, adapter)
			if err != nil || settlement.Totals != terminal.Totals || settlement.TerminalStatus != "FILLED" || settlement.OpeningCashUSD != "1000.1" || settlement.OpeningBase != "1.00000001" || settlement.ClosingCashUSD != "970" || settlement.ClosingBase != "1.00100001" {
				t.Fatal("racing fill was not settled exactly", err, settlement)
			}
			after, err := store.ReadCapitalReservation(ctx, r.OwnerID, order.ID)
			if err != nil || after.ReleasedAt == nil || !after.ReleasedAt.Equal(settlement.RecordedAt) {
				t.Fatal("settlement did not release exact reservation", err)
			}
			after.ReleasedAt = nil
			if !reflect.DeepEqual(before, after) {
				t.Fatal("settlement changed immutable reservation history")
			}
			reads = requests.Load()
			saved, err := execution.NewPostgresStore(pool).SettleBrokerAccount(ctx, r.OwnerID, order.ID, vault, adapter)
			if err != nil || !reflect.DeepEqual(saved, settlement) || requests.Load() != reads {
				t.Fatal("restart reread provider or repeated account settlement", err)
			}
			if _, err = store.SendConfirmed(ctx, r.OwnerID, order.ID, evidence, vault, adapter); !errors.Is(err, execution.ErrAlreadyAttempted) {
				t.Fatal("cancellation or settlement allowed resubmission", err)
			}
			if creates.Load() != 1 || cancellations.Load() != 1 || previews.Load() != 1 {
				t.Fatal("duplicate synthetic broker write", creates.Load(), cancellations.Load(), previews.Load())
			}
		})
	}
}
