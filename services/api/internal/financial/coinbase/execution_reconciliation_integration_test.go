package coinbase

import (
	"context"
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
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Exercise the public control plane and real Coinbase adapter against synthetic
// HTTP evidence. Partial fills are durable facts, not terminal settlement. Only
// a stable complete terminal scan releases the account slot; capital stays held.
func TestPostgresCoinbasePartialFillTerminalAndRestartReconciliation(t *testing.T) {
	ctx, pool := setupCoinbaseExecutionIntegration(t)
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
	identityJSON := strings.ReplaceAll(submissionIdentityJSON(submission), submissionTestProviderID, providerID)
	var posts, previews, detailReads, fillReads, requests atomic.Int32
	var acceptedAt atomic.Int64
	// 0: first OPEN partial; 1: incomplete pagination; 2: changing bracket;
	// 3: complete stable CANCELLED after a second partial fill;
	// 4: matching settled cash and position inventory, still no broker writes.
	var phase atomic.Int32
	fill := func(number int) map[string]any {
		stamp := time.Unix(0, acceptedAt.Load()).UTC().Add(time.Duration(number-1) * time.Microsecond).Format(time.RFC3339Nano)
		entry, trade := "entry-one", "trade-one"
		if number == 2 {
			entry, trade = "entry-two", "trade-two"
		}
		return map[string]any{"entry_id": entry, "trade_id": trade, "order_id": providerID, "retail_portfolio_id": r.AccountID, "product_id": r.ProductID, "product_type": "SPOT", "side": r.Side, "trade_type": "FILL",
			"trade_time": stamp, "sequence_timestamp": stamp, "price": "30000.00", "size": "0.0004", "commission": "0.04", "size_in_quote": false}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		verifyJWT(t, req, key, req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		path := req.URL.Path
		switch path {
		case "/api/v3/brokerage/orders":
			if req.Method != http.MethodPost {
				t.Error("unexpected submission method")
			}
			posts.Add(1)
			acceptedAt.Store(time.Now().UTC().Truncate(time.Microsecond).UnixNano())
			_, _ = io.WriteString(w, `{"success":true,"success_response":`+identityJSON+`}`)
			return
		case executionHistoryPath + "batch":
			if req.Method != http.MethodGet || !req.URL.Query().Has("order_status") || req.URL.Query().Get("retail_portfolio_id") != r.AccountID {
				t.Error("unexpected pre-send order scan")
			}
			_, _ = io.WriteString(w, `{"orders":[],"has_next":false,"cursor":""}`)
			return
		case executionHistoryPath + providerID:
			if req.Method != http.MethodGet {
				t.Error("order observation must be GET")
			}
			read := detailReads.Add(1)
			var response map[string]map[string]any
			body := strings.ReplaceAll(submissionDetailJSON(submission, time.Unix(0, acceptedAt.Load()).UTC()), submissionTestProviderID, providerID)
			if err := json.Unmarshal([]byte(body), &response); err != nil {
				t.Error(err)
				return
			}
			snapshot := response["order"]
			snapshot["status"], snapshot["pending_cancel"], snapshot["settled"] = "CANCELLED", false, true
			snapshot["number_of_fills"], snapshot["filled_size"], snapshot["filled_value"], snapshot["total_fees"] = "2", "0.0008", "24", "0.08"
			last := time.Unix(0, acceptedAt.Load()).UTC().Add(time.Microsecond)
			if phase.Load() == 0 {
				snapshot["status"], snapshot["settled"] = "OPEN", false
				snapshot["number_of_fills"], snapshot["filled_size"], snapshot["filled_value"], snapshot["total_fees"] = "1", "0.0004", "12", "0.04"
				last = last.Add(-time.Microsecond)
			} else if phase.Load() == 2 && read%2 == 1 {
				snapshot["status"], snapshot["settled"] = "OPEN", false
			}
			snapshot["last_fill_time"] = last.Format(time.RFC3339Nano)
			_ = json.NewEncoder(w).Encode(response)
			return
		case executionHistoryPath + "fills":
			fillReads.Add(1)
			query := req.URL.Query()
			if req.Method != http.MethodGet || len(query["order_ids"]) != 1 || query.Get("order_ids") != providerID || query.Get("limit") != "100" {
				t.Error("fill lookup is not a bounded exact-order GET")
			}
			page := map[string]any{"fills": []any{fill(1)}, "cursor": ""}
			if phase.Load() != 0 {
				if query.Get("cursor") == "page-two" {
					page["fills"] = []any{fill(2)}
				} else if query.Get("cursor") != "" {
					t.Error("unexpected fill cursor")
				} else {
					page["cursor"] = "page-two"
				}
				if phase.Load() == 1 {
					// Complete-looking totals cannot compensate for a cursor loop.
					page["cursor"] = "page-two"
				}
			}
			_ = json.NewEncoder(w).Encode(page)
			return
		case "/api/v3/brokerage/orders/preview":
			previews.Add(1)
			if req.Method != http.MethodPost {
				t.Error("unexpected preview method")
			}
		case "/api/v3/brokerage/accounts":
			if cursor := req.URL.Query().Get("cursor"); cursor != "" {
				path += "?" + cursor
			}
		}
		if req.URL.Path != "/api/v3/brokerage/orders/preview" && req.Method != http.MethodGet {
			t.Error("observation made an unexpected write")
		}
		body, ok := fixtures[path]
		if !ok {
			t.Errorf("unexpected synthetic endpoint: %s", path)
			http.NotFound(w, req)
			return
		}
		if phase.Load() == 4 && req.URL.Path == "/api/v3/brokerage/accounts" {
			body = strings.ReplaceAll(body, "1000.10", "976.02")
			body = strings.ReplaceAll(body, "1.00000001", "1.00080001")
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
		t.Fatal("capture synthetic provider evidence", err)
	}
	attempt, err := store.SendConfirmed(ctx, r.OwnerID, order.ID, evidence, vault, adapter)
	if err != nil || attempt.ProviderOrderID != providerID {
		t.Fatal("submit exact synthetic order", err)
	}
	assertCoinbaseExecutionHeld(t, ctx, pool, order, providerID)
	partial, err := store.ReconcileBrokerOrder(ctx, r.OwnerID, order.ID, vault, adapter)
	if err != nil {
		t.Fatal("complete OPEN partial fill scan", err)
	}
	assertCoinbaseReconciliation(t, partial, 1, "0.0004", "12", "0.04", "", true)
	assertCoinbaseReconciliationState(t, ctx, pool, order, partial, 0)
	for _, bad := range []struct {
		name  string
		phase int32
	}{{"incomplete pagination", 1}, {"changing bracket", 2}} {
		t.Run(bad.name, func(t *testing.T) {
			phase.Store(bad.phase)
			detailReads.Store(0)
			if _, err := store.ReconcileBrokerOrder(ctx, r.OwnerID, order.ID, vault, adapter); !errors.Is(err, execution.ErrUnreconciled) {
				t.Fatal("incomplete or changing evidence permitted settlement", err)
			}
			assertCoinbaseReconciliationState(t, ctx, pool, order, partial, 0)
		})
	}
	phase.Store(3)
	detailReads.Store(0)
	terminal, err := execution.NewPostgresStore(pool).ReconcileBrokerOrder(ctx, r.OwnerID, order.ID, vault, adapter)
	if err != nil {
		t.Fatal("complete stable terminal scan", err)
	}
	assertCoinbaseReconciliation(t, terminal, 2, "0.0008", "24", "0.08", "CANCELLED", false)
	assertCoinbaseReconciliationState(t, ctx, pool, order, terminal, 1)
	var completed time.Time
	var completionBasis string
	if err = pool.QueryRow(ctx, `SELECT completed_at,payload->>'CompletionTimeBasis' FROM execution_order_terminals WHERE order_id=$1`, order.ID).Scan(&completed, &completionBasis); err != nil || completionBasis != "OBSERVED_TERMINAL_STATUS" {
		t.Fatal("terminal invented a broker completion timestamp", err, completionBasis)
	}
	// Recreate the store and replay full broker history. No economic fact may
	// be counted twice and the first terminal observation must remain immutable.
	replayed, err := execution.NewPostgresStore(pool).ReconcileBrokerOrder(ctx, r.OwnerID, order.ID, vault, adapter)
	if err != nil || replayed != terminal {
		t.Fatal("restart/full history replay changed exact totals", err, replayed)
	}
	assertCoinbaseReconciliationState(t, ctx, pool, order, terminal, 1)
	var again time.Time
	if err = pool.QueryRow(ctx, `SELECT completed_at FROM execution_order_terminals WHERE order_id=$1`, order.ID).Scan(&again); err != nil || !again.Equal(completed) {
		t.Fatal("replay changed immutable first terminal observation", err)
	}
	if _, err = store.SendConfirmed(ctx, r.OwnerID, order.ID, evidence, vault, adapter); !errors.Is(err, execution.ErrAlreadyAttempted) {
		t.Fatal("terminal permitted repeat submission", err)
	}
	if posts.Load() != 1 || previews.Load() != 1 || fillReads.Load() != 9 {
		t.Fatal("reconciliation wrote, refreshed preview, or did not traverse fills", posts.Load(), previews.Load(), fillReads.Load())
	}
	// Matching final order fills are necessary but do not themselves prove
	// account balances. Only the separate complete zero-hold inventory proof
	// may record settlement and release the original internal capital fence.
	before, err := store.ReadCapitalReservation(ctx, r.OwnerID, order.ID)
	if err != nil || before.ReleasedAt != nil {
		t.Fatal("capital released before account settlement", err)
	}
	phase.Store(4)
	settlement, err := store.SettleBrokerAccount(ctx, r.OwnerID, order.ID, vault, adapter)
	if err != nil || settlement.OrderID != order.ID || settlement.ProviderOrderID != providerID || settlement.PortfolioID != r.AccountID || settlement.TerminalStatus != "CANCELLED" || settlement.Totals != terminal.Totals || settlement.OpeningCashUSD != "1000.1" || settlement.OpeningBase != "1.00000001" || settlement.ClosingCashUSD != "976.02" || settlement.ClosingBase != "1.00080001" {
		t.Fatal("complete synthetic account proof did not settle exactly", err, settlement)
	}
	after, err := store.ReadCapitalReservation(ctx, r.OwnerID, order.ID)
	if err != nil || after.ReleasedAt == nil || !after.ReleasedAt.Equal(settlement.RecordedAt) {
		t.Fatal("settlement receipt did not release its exact reservation", err, after)
	}
	after.ReleasedAt = nil
	if !reflect.DeepEqual(after, before) {
		t.Fatal("settlement erased or changed original reservation history")
	}
	var receipts int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_account_settlements WHERE order_id=$1`, order.ID).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatal("settlement did not persist one immutable receipt", err, receipts)
	}
	readsBeforeReplay := requests.Load()
	replayedSettlement, err := execution.NewPostgresStore(pool).SettleBrokerAccount(ctx, r.OwnerID, order.ID, vault, adapter)
	if err != nil || !reflect.DeepEqual(replayedSettlement, settlement) || requests.Load() != readsBeforeReplay {
		t.Fatal("restart reapplied settlement or reread provider", err)
	}
	if posts.Load() != 1 || previews.Load() != 1 {
		t.Fatal("account settlement submitted or refreshed a preview")
	}
}

func assertCoinbaseReconciliation(t *testing.T, got execution.Reconciliation, count int64, base, gross, fee, status string, held bool) {
	t.Helper()
	if got.FillCount != count || got.BaseQuantity != base || got.GrossUSD != gross || got.FeeUSD != fee || got.TerminalStatus != status || got.AccountHeld != held || got.AccountBlocked {
		t.Fatal("incorrect exact reconciliation", got)
	}
}

func assertCoinbaseReconciliationState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, order execution.Order, want execution.Reconciliation, terminalCount int) {
	t.Helper()
	store := execution.NewPostgresStore(pool)
	got, err := store.ReadReconciliation(ctx, order.Request.OwnerID, order.ID)
	if err != nil || got != want {
		t.Fatal("saved reconciliation changed", err, got)
	}
	reservation, err := store.ReadCapitalReservation(ctx, order.Request.OwnerID, order.ID)
	if err != nil || reservation.ReleasedAt != nil || reservation.ResourceType != "CASH" || reservation.Asset != "USD" || compareDecimal(financial.Decimal(reservation.Quantity), financial.Decimal(order.Request.MaximumDebitUSD)) != 0 {
		t.Fatal("order reconciliation released or changed reserved cash", err, reservation)
	}
	var terminals, exactEvidence int
	err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_order_terminals WHERE order_id=$1),(SELECT count(*) FROM execution_fills WHERE order_id=$1 AND payload->'ProviderEvidence'->>'EntryID' IN ('entry-one','entry-two') AND payload->'ProviderEvidence'->>'FeeCurrency'='USD' AND payload->'ProviderEvidence'->>'FeeCurrencyBasis'='COINBASE_ADVANCED_QUOTE_ASSET_1_91')`, order.ID).Scan(&terminals, &exactEvidence)
	if err != nil || terminals != terminalCount || int64(exactEvidence) != want.FillCount {
		t.Fatal("missing immutable source fill evidence or unexpected terminal", err, terminals, exactEvidence)
	}
}
