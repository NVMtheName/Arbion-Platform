package coinbase

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

type integrationExecutionVault struct {
	request     execution.Request
	credentials financial.Credentials
	generation  int64
}

func (v integrationExecutionVault) RetrieveFinancialVersion(_ context.Context, l credential.Locator) ([]byte, int64, error) {
	if l.Class != credential.Financial || l.UserID != v.request.OwnerID || l.ConnectionID != v.request.ConnectionID {
		return nil, 0, errors.New("incorrect synthetic credential scope")
	}
	raw, err := json.Marshal(v.credentials)
	return raw, v.generation, err
}

type integrationExecutionStepUp struct{ pool *pgxpool.Pool }

func (v integrationExecutionStepUp) VerifyExecutionStepUp(ctx context.Context, owner, _ string) (string, time.Time, error) {
	var verified time.Time
	err := v.pool.QueryRow(ctx, `UPDATE auth_totp_factors SET updated_at=clock_timestamp(),last_used_step=floor(extract(epoch FROM clock_timestamp())/30) WHERE user_id=$1 RETURNING updated_at`, owner).Scan(&verified)
	return "totp", verified, err
}

func setupCoinbaseExecutionIntegration(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	databaseURL := os.Getenv("STRATEGY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("STRATEGY_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return ctx, pool
}

func prepareCoinbaseExecutionIntegration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (execution.Order, int64) {
	t.Helper()
	r := newCoinbaseExecutionIntegrationRequest(t, ctx, pool)
	store := execution.NewPostgresStore(pool)
	order, err := store.Prepare(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := store.ApproveOrder(ctx, authorization.Principal{UserID: r.OwnerID, Entitlement: authorization.EntitlementFounder}, order.ID, order.RequestDigest, "synthetic-only", integrationExecutionStepUp{pool})
	if err != nil {
		t.Fatal(err)
	}
	seedCoinbaseExecutionIntegrationReconciliation(t, ctx, pool, r)
	return order, approval.CredentialGeneration
}

// Infrastructure only: callers may prepare and approve through the owner
// workflow instead of bypassing that boundary in an integration fixture.
func newCoinbaseExecutionIntegrationRequest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) execution.Request {
	t.Helper()
	r := execution.Request{ProductID: "BTC-USD", Side: "BUY", BaseSize: "0.0010", LimitPrice: "30000.00", FeeAllowanceUSD: "0.10", MaximumDebitUSD: "30.10",
		PilotLimits: &execution.OwnerPilotLimits{MaximumOrderUSD: "100000000", ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)}}
	if err := pool.QueryRow(ctx, `INSERT INTO users(external_id) VALUES($1) RETURNING id::text`, fmt.Sprintf("coinbase-send-integration-%d", time.Now().UnixNano())).Scan(&r.OwnerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_entitlements(user_id,entitlement_key) VALUES($1,'founder')`, r.OwnerID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status,encrypted_credential_payload) VALUES($1,'financial','coinbase','Synthetic execution fixture','active',decode(repeat('11',32),'hex')) RETURNING id::text`, r.OwnerID).Scan(&r.ConnectionID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO financial_accounts(user_id,provider_connection_id,provider_name,provider_account_id,display_name,base_currency,status) VALUES($1,$2,'coinbase',$3,'Synthetic execution fixture','USD','active') RETURNING id::text`, r.OwnerID, r.ConnectionID, "fixture:"+r.ConnectionID).Scan(&r.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE financial_accounts SET provider_account_id=$2 WHERE id=$1`, r.AccountID, "portfolio:"+r.AccountID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO capital_buckets(user_id,financial_account_id,name,allocation_type,allocation_value,currency,status) VALUES($1,$2,'Synthetic execution fixture','FIXED_AMOUNT',100,'USD','ACTIVE') RETURNING id::text`, r.OwnerID, r.AccountID).Scan(&r.CapitalBucketID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&r.ClientOrderID); err != nil {
		t.Fatal(err)
	}
	// Only fabricated encrypted bytes and the local synthetic step-up verifier
	// are used. No production credential or authentication service is accessed.
	if _, err := pool.Exec(ctx, `INSERT INTO auth_totp_factors(user_id,secret_ciphertext,enabled_at) VALUES($1,decode(repeat('22',32),'hex'),clock_timestamp()-interval '1 minute')`, r.OwnerID); err != nil {
		t.Fatal(err)
	}
	return r
}

func seedCoinbaseExecutionIntegrationReconciliation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, r execution.Request) {
	t.Helper()
	var reconciliation string
	err := pool.QueryRow(ctx, `INSERT INTO portfolio_reconciliations(user_id,financial_account_id,provider_name,comparison_status,balances_status,positions_status,performance_status,realized_performance_status,autonomy_signal,observed_position_count,performance_position_count,change_count,evidence_hash,observed_at,cash_amount,cash_currency,available_cash_amount,available_cash_currency)
	 VALUES($1,$2,'coinbase','MATCHED','READY','READY','UNAVAILABLE','UNAVAILABLE','CLEAR',1,0,0,decode(repeat('33',32),'hex'),clock_timestamp(),1000.10,'USD',1000.10,'USD') RETURNING id::text`, r.OwnerID, r.AccountID).Scan(&reconciliation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO portfolio_reconciliation_positions(reconciliation_id,user_id,financial_account_id,symbol,instrument_type,direction,quantity,available_quantity,performance_status) VALUES($1,$2,$3,'BTC','CRYPTO','long',1.00000001,1.00000001,'UNAVAILABLE')`, reconciliation, r.OwnerID, r.AccountID); err != nil {
		t.Fatal(err)
	}
}

// This is the public control-plane path with the real Coinbase adapter and a
// local HTTP broker. The broker accepts exactly one request but loses its HTTP
// acknowledgement; a fresh store may only look up that original identity.
func TestPostgresCoinbaseLostAcknowledgementRecoveryNeverResubmits(t *testing.T) {
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
	var submissions, previews, lookups, details, requests atomic.Int32
	var acceptedAt atomic.Int64
	var historyVisible atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		verifyJWT(t, req, key, req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		path := req.URL.Path
		switch path {
		case "/api/v3/brokerage/orders":
			if req.Method != http.MethodPost {
				t.Errorf("unexpected submission method: %s", req.Method)
				http.Error(w, "method", http.StatusMethodNotAllowed)
				return
			}
			submissions.Add(1)
			body, err := io.ReadAll(req.Body)
			var got any
			if err != nil || json.Unmarshal(body, &got) != nil {
				t.Error("invalid synthetic submission JSON")
				http.Error(w, "body", http.StatusBadRequest)
				return
			}
			want := map[string]any{"client_order_id": r.ClientOrderID, "preview_id": "30000000-0000-4000-8000-000000000001", "product_id": r.ProductID, "side": r.Side, "retail_portfolio_id": r.AccountID,
				"order_configuration": map[string]any{"sor_limit_ioc": map[string]any{"base_size": r.BaseSize, "limit_price": r.LimitPrice}}}
			if !reflect.DeepEqual(got, want) {
				t.Error("submission changed immutable order terms")
			}
			acceptedAt.Store(time.Now().UTC().UnixNano())
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close() // The broker accepted it; the client sees only EOF.
			return
		case "/api/v3/brokerage/orders/historical/batch":
			if req.Method != http.MethodGet || req.URL.Query().Get("retail_portfolio_id") != r.AccountID || req.URL.Query().Get("limit") != "100" {
				t.Error("history read is not bounded and portfolio-scoped")
			}
			if len(req.URL.Query()["order_status"]) > 0 {
				_, _ = io.WriteString(w, `{"orders":[],"has_next":false,"cursor":""}`)
				return
			}
			lookups.Add(1)
			if !historyVisible.Load() {
				_, _ = io.WriteString(w, `{"orders":[],"has_next":false,"cursor":""}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"orders": []any{map[string]any{"order_id": providerID, "client_order_id": r.ClientOrderID, "product_id": r.ProductID, "side": r.Side}}, "has_next": false, "cursor": ""})
			return
		case "/api/v3/brokerage/orders/historical/" + providerID:
			details.Add(1)
			if req.Method != http.MethodGet || acceptedAt.Load() == 0 {
				t.Error("unexpected detail lookup")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"order": map[string]any{"order_id": providerID, "client_order_id": r.ClientOrderID, "product_id": r.ProductID, "side": r.Side, "retail_portfolio_id": r.AccountID,
				"product_type": "SPOT", "order_type": "LIMIT", "time_in_force": "IMMEDIATE_OR_CANCEL", "created_time": time.Unix(0, acceptedAt.Load()).UTC().Format(time.RFC3339Nano), "size_in_quote": false, "size_inclusive_of_fees": false,
				"order_configuration": map[string]any{"sor_limit_ioc": map[string]any{"base_size": r.BaseSize, "limit_price": r.LimitPrice}}}})
			return
		case "/api/v3/brokerage/orders/preview":
			previews.Add(1)
			if req.Method != http.MethodPost {
				t.Error("unexpected preview method")
			}
		case "/api/v3/brokerage/accounts":
			if req.URL.Query().Get("retail_portfolio_id") != r.AccountID || req.URL.Query().Get("limit") != "250" {
				t.Error("inventory read is not bounded and portfolio-scoped")
			}
			if cursor := req.URL.Query().Get("cursor"); cursor != "" {
				path += "?" + cursor
			}
		}
		if req.URL.Path != "/api/v3/brokerage/orders/preview" && req.Method != http.MethodGet {
			t.Error("unexpected provider write")
		}
		body, ok := fixtures[path]
		if !ok {
			t.Errorf("unexpected local broker endpoint: %s", path)
			http.NotFound(w, req)
			return
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
		t.Fatal("capture through local Coinbase preview", err)
	}
	if _, err = store.SendConfirmed(ctx, r.OwnerID, order.ID, evidence, vault, adapter); !errors.Is(err, execution.ErrSubmissionUnknown) {
		t.Fatal("lost HTTP acknowledgement must remain unknown", err)
	}
	original := assertCoinbaseExecutionHeld(t, ctx, pool, order, "")
	if submissions.Load() != 1 || previews.Load() != 1 {
		t.Fatal("unexpected write count", submissions.Load(), previews.Load())
	}
	// Reconstruct the store as after restart. Empty broker history neither
	// releases held capital nor permits sending the same client identity again.
	restarted := execution.NewPostgresStore(pool)
	if _, err = restarted.RecoverSubmission(ctx, r.OwnerID, order.ID, vault, adapter); !errors.Is(err, execution.ErrSubmissionUnknown) {
		t.Fatal("empty history must remain unknown", err)
	}
	assertCoinbaseExecutionHeld(t, ctx, pool, order, "")
	if _, err = restarted.SendConfirmed(ctx, r.OwnerID, order.ID, evidence, vault, adapter); !errors.Is(err, execution.ErrAlreadyAttempted) {
		t.Fatal("unknown attempt became retry authority", err)
	}
	if submissions.Load() != 1 || details.Load() != 0 || lookups.Load() != 1 {
		t.Fatal("missing history caused a write or guessed detail lookup")
	}
	// Revocation prevents sending, but must not prevent read-only correlation
	// of the broker request already attempted under the original approval.
	if err = restarted.RevokeOwnerApproval(ctx, r.OwnerID, order.ID); err != nil {
		t.Fatal(err)
	}
	historyVisible.Store(true)
	recovered, err := restarted.RecoverSubmission(ctx, r.OwnerID, order.ID, vault, adapter)
	if err != nil || recovered.ProviderOrderID != providerID || recovered.OrderID != original.OrderID || recovered.ClientOrderID != original.ClientOrderID || recovered.RequestDigest != original.RequestDigest || recovered.AuthorizationID != original.AuthorizationID || !recovered.ClaimedAt.Equal(original.ClaimedAt) {
		t.Fatal("read-only recovery changed original identity", err, recovered)
	}
	assertCoinbaseExecutionHeld(t, ctx, pool, order, providerID)
	before := requests.Load()
	again, err := execution.NewPostgresStore(pool).RecoverSubmission(ctx, r.OwnerID, order.ID, vault, adapter)
	if err != nil || again.ProviderOrderID != providerID || requests.Load() != before {
		t.Fatal("durable acknowledgement replay made another provider call", err)
	}
	if _, err = restarted.SendConfirmed(ctx, r.OwnerID, order.ID, evidence, vault, adapter); !errors.Is(err, execution.ErrAlreadyAttempted) {
		t.Fatal("acknowledgement became resend authority", err)
	}
	if submissions.Load() != 1 || previews.Load() != 1 || details.Load() != 1 || lookups.Load() != 2 {
		t.Fatal("recovery was not bounded read-only identity correlation", submissions.Load(), previews.Load(), details.Load(), lookups.Load())
	}
}

func assertCoinbaseExecutionHeld(t *testing.T, ctx context.Context, pool *pgxpool.Pool, order execution.Order, providerID string) execution.Attempt {
	t.Helper()
	store := execution.NewPostgresStore(pool)
	r := order.Request
	attempt, err := store.ReadAttempt(ctx, r.OwnerID, order.ID)
	if err != nil || attempt.OrderID != order.ID || attempt.ClientOrderID != r.ClientOrderID || attempt.RequestDigest != order.RequestDigest || attempt.ProviderOrderID != providerID {
		t.Fatal("wrong durable attempt", err, attempt)
	}
	var accountHolds, fills, terminals int
	err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_account_holds WHERE order_id=$1 AND owner_id=$2 AND financial_account_id=$3),(SELECT count(*) FROM execution_fills WHERE order_id=$1),(SELECT count(*) FROM execution_order_terminals WHERE order_id=$1)`, order.ID, r.OwnerID, r.AccountID).Scan(&accountHolds, &fills, &terminals)
	if err != nil || accountHolds != 1 || fills != 0 || terminals != 0 {
		t.Fatal("submission/recovery released slot or manufactured settlement", err, accountHolds, fills, terminals)
	}
	reservation, err := store.ReadCapitalReservation(ctx, r.OwnerID, order.ID)
	if err != nil || reservation.OrderID != order.ID || reservation.AccountID != r.AccountID || reservation.CapitalBucketID != r.CapitalBucketID || reservation.ResourceType != "CASH" || reservation.Asset != "USD" || compareDecimal(financial.Decimal(reservation.Quantity), financial.Decimal(r.MaximumDebitUSD)) != 0 {
		t.Fatal("submission/recovery changed exact capital reservation", err, reservation)
	}
	return attempt
}
