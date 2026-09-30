package coinbase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/platform/config"
	platformhttp "github.com/arbion/platform/services/api/internal/platform/http"
	"github.com/redis/go-redis/v9"
)

// Exercise the authenticated-owner command boundary with synthetic dependencies
// and a loopback broker only. No production route, credential or API is used.
func TestPostgresCoinbaseOwnerWorkflowCancellationSettlementAndRestart(t *testing.T) {
	ctx, pool := setupCoinbaseExecutionIntegration(t)
	r := newCoinbaseExecutionIntegrationRequest(t, ctx, pool)
	// The existing provider fixture has no login fields. Add synthetic values so
	// the real auth store, not a fake principal, authenticates every HTTP command.
	if _, err := pool.Exec(ctx, `UPDATE users SET email=$2,normalized_email=$2,password_hash='synthetic-not-a-login-hash' WHERE id=$1`, r.OwnerID, r.OwnerID+"@example.invalid"); err != nil {
		t.Fatal(err)
	}
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	sessions := auth.NewRedisStore(redisClient)
	authService := auth.NewService(auth.NewPostgresStore(pool), sessions, sessions, nil, time.Hour)
	token, _, err := sessions.Create(ctx, r.OwnerID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Auth{SessionCookie: "synthetic-session", SessionTTL: time.Hour, AllowedOrigins: []string{"https://owner.example"}}
	credentials, key := testCredentials(t)
	credentials.PortfolioID = r.AccountID
	var generation int64
	var providerID string
	if err := pool.QueryRow(ctx, `SELECT credential_generation,gen_random_uuid()::text FROM provider_connections WHERE id=$1`, r.ConnectionID).Scan(&generation, &providerID); err != nil {
		t.Fatal(err)
	}
	vault := integrationExecutionVault{request: r, credentials: credentials, generation: generation}
	fixtures := executionTestFixtures(time.Now().UTC())
	for path, body := range fixtures {
		fixtures[path] = strings.ReplaceAll(body, executionTestPortfolio, r.AccountID)
	}
	var prepared atomic.Value // Private immutable order, published before any I/O.
	var creates, previews, cancels, requests atomic.Int32
	var acceptedAt, secondFillAt atomic.Int64
	var final atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		verifyJWT(t, req, key, req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		path := req.URL.Path
		sub := execution.ConfirmedSubmission{Order: prepared.Load().(execution.Order), PortfolioID: r.AccountID}
		switch path {
		case "/api/v3/brokerage/orders":
			var command struct {
				ClientID  string `json:"client_order_id"`
				ProductID string `json:"product_id"`
				Side      string `json:"side"`
			}
			if req.Method != http.MethodPost || json.NewDecoder(req.Body).Decode(&command) != nil || command.ClientID != sub.Order.Request.ClientOrderID || command.ClientID == r.ClientOrderID || command.ProductID != r.ProductID || command.Side != r.Side {
				t.Error("owner workflow changed private broker identity or terms")
			}
			creates.Add(1)
			acceptedAt.Store(time.Now().UTC().Truncate(time.Microsecond).UnixNano())
			identity := strings.ReplaceAll(submissionIdentityJSON(sub), submissionTestProviderID, providerID)
			_, _ = io.WriteString(w, `{"success":true,"success_response":`+identity+`}`)
			return
		case executionCancellationPath:
			var command struct {
				OrderIDs []string `json:"order_ids"`
			}
			if req.Method != http.MethodPost || json.NewDecoder(req.Body).Decode(&command) != nil || len(command.OrderIDs) != 1 || command.OrderIDs[0] != providerID {
				t.Error("owner cancellation did not retain the exact original broker identity")
			}
			cancels.Add(1)
			secondFillAt.Store(time.Now().UTC().Truncate(time.Microsecond).UnixNano())
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"order_id": providerID, "success": true}}})
			return
		case executionHistoryPath + "batch":
			if req.Method != http.MethodGet || !req.URL.Query().Has("order_status") || req.URL.Query().Get("retail_portfolio_id") != r.AccountID {
				t.Error("owner settlement requested an unscoped or writable order scan")
			}
			_, _ = io.WriteString(w, `{"orders":[],"has_next":false,"cursor":""}`)
			return
		case executionHistoryPath + providerID:
			at := time.Unix(0, acceptedAt.Load()).UTC()
			body := reconciliationOrderJSON(sub, at, "OPEN", 1, "0.0004", "12", "0.04", at)
			if final.Load() {
				body = reconciliationOrderJSON(sub, at, "FILLED", 2, "0.001", "30", "0.10", time.Unix(0, secondFillAt.Load()).UTC())
			}
			if req.Method != http.MethodGet {
				t.Error("owner reconciliation attempted a write")
			}
			body["order"].(map[string]any)["order_id"] = providerID
			_ = json.NewEncoder(w).Encode(body)
			return
		case executionHistoryPath + "fills":
			q := req.URL.Query()
			if req.Method != http.MethodGet || len(q["order_ids"]) != 1 || q.Get("order_ids") != providerID || q.Get("limit") != "100" || q.Get("cursor") != "" {
				t.Error("owner reconciliation requested unbounded or foreign fills")
			}
			first := reconciliationFillJSON(sub, time.Unix(0, acceptedAt.Load()).UTC(), "one", "0.0004", "0.04", false)
			first["order_id"] = providerID
			fills := []any{first}
			if final.Load() {
				second := reconciliationFillJSON(sub, time.Unix(0, secondFillAt.Load()).UTC(), "two", "0.0006", "0.06", false)
				second["order_id"] = providerID
				fills = append(fills, second)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"fills": fills, "cursor": ""})
			return
		case "/api/v3/brokerage/orders/preview":
			previews.Add(1)
			if req.Method != http.MethodPost {
				t.Error("preview was not POST")
			}
		case "/api/v3/brokerage/accounts":
			if cursor := req.URL.Query().Get("cursor"); cursor != "" {
				path += "?" + cursor
			}
		}
		if req.URL.Path != "/api/v3/brokerage/orders/preview" && req.Method != http.MethodGet {
			t.Error("unexpected owner workflow broker write")
		}
		body, ok := fixtures[path]
		if !ok {
			t.Errorf("unexpected synthetic endpoint %s", path)
			http.NotFound(w, req)
			return
		}
		if final.Load() && req.URL.Path == "/api/v3/brokerage/accounts" {
			body = strings.ReplaceAll(strings.ReplaceAll(body, "1000.10", "970"), "1.00000001", "1.00100001")
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	client, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	adapter := NewExecutionAdapter(client)
	scope := execution.OwnerScope{OwnerID: r.OwnerID, AccountID: r.AccountID, ConnectionID: r.ConnectionID, CapitalBucketID: r.CapitalBucketID, ProductID: r.ProductID}
	deps := execution.OwnerWorkflowDependencies{StepUp: integrationExecutionStepUp{pool}, Vault: vault, Preflight: client, Sender: adapter, Lookup: adapter, Observation: adapter, Cancellation: adapter, Settlement: adapter}
	restart := func() *ownerWorkflowHTTP {
		t.Helper()
		w, err := execution.NewOwnerWorkflow(execution.NewPostgresStore(pool), scope, deps)
		if err != nil {
			t.Fatal(err)
		}
		h := &ownerWorkflowHTTP{t: t, handler: platformhttp.NewOwnerExecutionHandler(cfg, authService, w), cookie: &http.Cookie{Name: cfg.SessionCookie, Value: token}}
		response := h.request(ctx, http.MethodGet, "/api/personal-execution/context", nil)
		var view struct {
			Available      bool   `json:"available"`
			ProductID      string `json:"product_id"`
			AccountLabel   string `json:"account_label"`
			SessionBinding string `json:"session_binding"`
		}
		if response.Code != http.StatusOK || h.decode(response, &view) != nil || !view.Available || view.ProductID != r.ProductID || view.AccountLabel != "Dedicated Coinbase portfolio" || len(view.SessionBinding) != 64 || strings.Contains(response.Body.String(), token) {
			t.Fatal("authenticated fixed owner context was unavailable or exposed the session")
		}
		h.binding = view.SessionBinding
		return h
	}
	w := restart()
	if requests.Load() != 0 {
		t.Fatal("owner startup/context contacted the broker")
	}
	// Context is read-only. An old screen cannot cross a session replacement,
	// and neither an absent nor revoked session may read even the saved order.
	originalCookie, originalBinding := w.cookie, w.binding
	w.cookie = nil
	if response := w.request(ctx, http.MethodGet, "/api/personal-execution/context", nil); response.Code != http.StatusUnauthorized {
		t.Fatal("anonymous owner context accepted")
	}
	replacementToken, _, err := sessions.Create(ctx, r.OwnerID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	w.cookie = &http.Cookie{Name: cfg.SessionCookie, Value: replacementToken}
	if _, err := w.order(ctx, http.MethodPost, "", struct{}{}); !ownerWorkflowHTTPErrorIs(err, http.StatusUnauthorized, "execution_session_changed") {
		t.Fatal("stale screen binding survived same-owner session replacement", err)
	}
	w.cookie, w.binding = originalCookie, originalBinding
	command := execution.OwnerPrepareCommand{RequestKey: r.ClientOrderID, Side: r.Side, BaseSize: r.BaseSize, LimitPrice: r.LimitPrice, FeeAllowanceUSD: r.FeeAllowanceUSD, MaximumDebitUSD: r.MaximumDebitUSD}
	o, err := w.order(ctx, http.MethodPost, "", command)
	if err != nil || o.State != "PREPARED" || o.Attempted || o.AccountHeld || o.CapitalHeld {
		t.Fatal("owner prepare", err, o.State)
	}
	if replay, err := restart().order(ctx, http.MethodPost, "", command); err != nil || !reflect.DeepEqual(replay, o) {
		t.Fatal("owner request-key replay changed prepared order", err)
	}
	changed := command
	changed.BaseSize = "0.0009"
	if _, err := w.order(ctx, http.MethodPost, "", changed); !ownerWorkflowHTTPErrorIs(err, http.StatusConflict, "EXECUTION_REVIEW_REQUIRED") {
		t.Fatal("owner request key accepted changed terms", err)
	}
	// Only the synthetic broker fixture reads private correlation from storage;
	// the owner workflow never receives or returns it as a command parameter.
	var private execution.Order
	var requestJSON []byte
	if err := pool.QueryRow(ctx, `SELECT id::text,request_digest,created_at,request FROM execution_orders WHERE id=$1 AND owner_id=$2`, o.ID, r.OwnerID).Scan(&private.ID, &private.RequestDigest, &private.CreatedAt, &requestJSON); err != nil || json.Unmarshal(requestJSON, &private.Request) != nil {
		t.Fatal("read synthetic immutable request", err)
	}
	prepared.Store(private)
	assertSafe := func(value any) {
		t.Helper()
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{`"owner_id"`, `"account_id"`, `"connection_id"`, `"capital_bucket_id"`, `"client_order_id"`, `"provider_order_id"`, `"portfolio_id"`, `"preview_id"`, `"credential_generation"`, `"authorization_id"`, `"preflight"`, "PRIVATE KEY", credentials.APIKeyName, providerID, private.Request.ClientOrderID, r.OwnerID, r.AccountID, r.ConnectionID, r.CapitalBucketID} {
			if strings.Contains(string(body), forbidden) {
				t.Fatal("owner JSON exposed a private field or provider identity")
			}
		}
	}
	assertSafe(o)
	o, err = w.order(ctx, http.MethodPost, o.ID+"/approve", execution.OwnerApproveCommand{ExpectedDigest: o.RequestDigest, MFACode: "synthetic-only"})
	if err != nil || o.State != "APPROVED" || o.ApprovalStatus != "RECORDED" {
		t.Fatal("owner approval", err, o.State)
	}
	assertSafe(o)
	seedCoinbaseExecutionIntegrationReconciliation(t, ctx, pool, private.Request)
	captured, err := w.capture(ctx, o.ID)
	if err != nil || captured.EvidenceID == "" || captured.Order.ID != o.ID {
		t.Fatal("owner capture", err)
	}
	assertSafe(captured)
	send := execution.OwnerSendCommand{EvidenceID: captured.EvidenceID}
	o, err = w.order(ctx, http.MethodPost, o.ID+"/send", send)
	if err != nil || o.State != "BROKER_ACKNOWLEDGED" || !o.AccountHeld || !o.CapitalHeld || o.FillCount != 0 {
		t.Fatal("owner send", err, o.State)
	}
	assertSafe(o)
	beforeRecovery := requests.Load()
	if replay, err := restart().order(ctx, http.MethodPost, o.ID+"/recover", struct{}{}); err != nil || !reflect.DeepEqual(replay, o) || requests.Load() != beforeRecovery {
		t.Fatal("owner recovery replay changed acknowledgement or contacted broker", err)
	}
	o, err = w.order(ctx, http.MethodPost, o.ID+"/reconcile", struct{}{})
	if err != nil || o.State != "PARTIALLY_FILLED" || o.FillCount != 1 || o.BaseFilled != "0.0004" || o.GrossUSD != "12" || o.FeeUSD != "0.04" || !o.AccountHeld || !o.CapitalHeld {
		t.Fatal("owner partial reconciliation", err, o.State)
	}
	assertSafe(o)
	o, err = w.order(ctx, http.MethodPost, o.ID+"/cancel", struct{}{})
	if err != nil || o.State != "PARTIALLY_FILLED" || o.CancellationStatus != "ACCEPTED" || !o.AccountHeld || !o.CapitalHeld || o.TerminalStatus != "" || o.Accounting != nil {
		t.Fatal("cancellation result implied finality or released capital", err, o.State)
	}
	assertSafe(o)
	w = restart()
	beforeReads := requests.Load()
	if replay, err := w.order(ctx, http.MethodGet, o.ID, nil); err != nil || !reflect.DeepEqual(replay, o) {
		t.Fatal("owner restart lost cancellation state", err)
	}
	if replay, err := w.order(ctx, http.MethodPost, o.ID+"/cancel", struct{}{}); err != nil || !reflect.DeepEqual(replay, o) {
		t.Fatal("owner restart repeated cancellation", err)
	}
	if _, err := w.order(ctx, http.MethodPost, o.ID+"/send", send); !ownerWorkflowHTTPErrorIs(err, http.StatusConflict, "EXECUTION_ALREADY_ATTEMPTED") {
		t.Fatal("owner restart authorized another send", err)
	}
	if _, err := w.capture(ctx, o.ID); !ownerWorkflowHTTPErrorIs(err, http.StatusConflict, "EXECUTION_ALREADY_ATTEMPTED") || requests.Load() != beforeReads {
		t.Fatal("owner restart refreshed preview or contacted broker", err)
	}
	final.Store(true)
	o, err = w.order(ctx, http.MethodPost, o.ID+"/reconcile", struct{}{})
	if err != nil || o.State != "AWAITING_ACCOUNT_SETTLEMENT" || o.TerminalStatus != "FILLED" || o.FillCount != 2 || o.BaseFilled != "0.001" || o.GrossUSD != "30" || o.FeeUSD != "0.1" || o.AccountHeld || !o.CapitalHeld {
		t.Fatal("owner final reconciliation", err, o.State)
	}
	assertSafe(o)
	o, err = w.order(ctx, http.MethodPost, o.ID+"/settle", struct{}{})
	if err != nil || o.State != "SETTLED" || o.AccountHeld || o.CapitalHeld || o.AccountBlocked || o.CancellationStatus != "ACCEPTED" || o.Accounting == nil || o.Accounting.OpeningCashUSD != "1000.1" || o.Accounting.OpeningBase != "1.00000001" || o.Accounting.ClosingCashUSD != "970" || o.Accounting.ClosingBase != "1.00100001" {
		t.Fatal("owner exact account settlement", err, o.State)
	}
	assertSafe(o)
	w = restart()
	beforeReads = requests.Load()
	if replay, err := w.order(ctx, http.MethodGet, o.ID, nil); err != nil || !reflect.DeepEqual(replay, o) {
		t.Fatal("owner restart changed settled accounting", err)
	}
	if replay, err := w.order(ctx, http.MethodPost, o.ID+"/settle", struct{}{}); err != nil || !reflect.DeepEqual(replay, o) || requests.Load() != beforeReads {
		t.Fatal("owner restart repeated settlement collection", err)
	}
	if err := sessions.Delete(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := w.order(ctx, http.MethodGet, o.ID, nil); !ownerWorkflowHTTPErrorIs(err, http.StatusUnauthorized, "unauthenticated") || requests.Load() != beforeReads {
		t.Fatal("revoked session retained owner status access or contacted broker", err)
	}
	var attempts, fills, cancellations, receipts, settlements, released int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_dispatch_attempts WHERE order_id=$1),
	 (SELECT count(*) FROM execution_fills WHERE order_id=$1),(SELECT count(*) FROM execution_cancellation_attempts WHERE order_id=$1),
	 (SELECT count(*) FROM execution_cancellation_receipts WHERE order_id=$1),(SELECT count(*) FROM execution_account_settlements WHERE order_id=$1),
	 (SELECT count(*) FROM execution_capital_reservations WHERE order_id=$1 AND released_at IS NOT NULL)`, o.ID).Scan(&attempts, &fills, &cancellations, &receipts, &settlements, &released); err != nil || attempts != 1 || fills != 2 || cancellations != 1 || receipts != 1 || settlements != 1 || released != 1 || creates.Load() != 1 || previews.Load() != 1 || cancels.Load() != 1 {
		t.Fatal("owner workflow duplicated operation or accounting history", err, attempts, fills, cancellations, receipts, settlements, released, creates.Load(), previews.Load(), cancels.Load())
	}
}

// These helpers only encode/decode the public protocol; the concrete handler,
// authentication service, durable workflow and provider adapters remain real.
type ownerWorkflowHTTP struct {
	t       *testing.T
	handler http.Handler
	cookie  *http.Cookie
	binding string
}

type ownerWorkflowHTTPError struct {
	status int
	code   string
}

func (e *ownerWorkflowHTTPError) Error() string { return fmt.Sprintf("HTTP %d %s", e.status, e.code) }

func ownerWorkflowHTTPErrorIs(err error, status int, code string) bool {
	var response *ownerWorkflowHTTPError
	return errors.As(err, &response) && response.status == status && response.code == code
}

func (h *ownerWorkflowHTTP) request(ctx context.Context, method, path string, command any) *httptest.ResponseRecorder {
	h.t.Helper()
	var body []byte
	if command != nil {
		var err error
		body, err = json.Marshal(command)
		if err != nil {
			h.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body)).WithContext(ctx)
	if h.cookie != nil {
		req.AddCookie(h.cookie)
	}
	req.Header.Set("Origin", "https://owner.example")
	req.Header.Set("Content-Type", "application/json")
	if h.binding != "" {
		req.Header.Set("X-Arbion-Execution-Session", h.binding)
	}
	response := httptest.NewRecorder()
	h.handler.ServeHTTP(response, req)
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		h.t.Fatal("owner response lost private response headers")
	}
	return response
}

func (h *ownerWorkflowHTTP) decode(response *httptest.ResponseRecorder, out any) error {
	h.t.Helper()
	if response.Code != http.StatusOK {
		var failure struct {
			Error struct{ Code, Message string }
		}
		if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil || failure.Error.Code == "" {
			h.t.Fatal("owner error lost its bounded protocol", err)
		}
		return &ownerWorkflowHTTPError{status: response.Code, code: failure.Error.Code}
	}
	decoder := json.NewDecoder(bytes.NewReader(response.Body.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		h.t.Fatal("owner response widened beyond the public DTO", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		h.t.Fatal("owner response contained multiple JSON values")
	}
	return nil
}

func (h *ownerWorkflowHTTP) order(ctx context.Context, method, suffix string, command any) (execution.OwnerOrder, error) {
	path := "/api/personal-execution/orders"
	if suffix != "" {
		path += "/" + suffix
	}
	var body struct {
		Order execution.OwnerOrder `json:"order"`
	}
	err := h.decode(h.request(ctx, method, path, command), &body)
	return body.Order, err
}

func (h *ownerWorkflowHTTP) capture(ctx context.Context, id string) (execution.OwnerPreflight, error) {
	var proof execution.OwnerPreflight
	err := h.decode(h.request(ctx, http.MethodPost, "/api/personal-execution/orders/"+id+"/preflight", struct{}{}), &proof)
	return proof, err
}
