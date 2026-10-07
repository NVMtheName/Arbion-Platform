package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/platform/config"
	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"
)

// Only the one MFA proof is synthetic. The cookie/session authentication,
// current founder lookup, domain workflow, transactions and SQL guards are real.
type commissioningPostgresStepUp struct {
	pool  *pgxpool.Pool
	calls atomic.Int64
}

func (s *commissioningPostgresStepUp) VerifyExecutionStepUp(ctx context.Context, owner, code string) (string, time.Time, error) {
	s.calls.Add(1)
	if code != "654321" {
		return "", time.Time{}, errors.New("unexpected synthetic MFA code")
	}
	var verified time.Time
	err := s.pool.QueryRow(ctx, `UPDATE auth_totp_factors SET updated_at=clock_timestamp(),
	 last_used_step=floor(extract(epoch FROM clock_timestamp())/30) WHERE user_id=$1 RETURNING updated_at`, owner).Scan(&verified)
	return "totp", verified, err
}

func setupCommissioningHTTPPostgres(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("STRATEGY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("STRATEGY_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	db, err := sql.Open("pgx", url)
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
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return ctx, pool
}

func newCommissioningHTTPPostgresTerms(t *testing.T, ctx context.Context, pool *pgxpool.Pool) execution.OwnerCommissioningTerms {
	t.Helper()
	terms := execution.OwnerCommissioningTerms{
		Pilot:     execution.PilotAllocation{ProductID: "BTC-USD", InitialCashUSD: "100", Limits: execution.OwnerPilotLimits{MaximumOrderUSD: "25"}},
		AIModelID: "synthetic-model", Objective: "Synthetic bounded spot decisions", IntervalMinutes: 30, MaxTradesPerDay: 2,
		MaxCapitalDeployedUSD: "100", MaxSinglePositionUSD: "25", MinimumCashReserveUSD: "0",
	}
	email := fmt.Sprintf("commissioning-http-%d@example.test", time.Now().UnixNano())
	user, err := auth.NewPostgresStore(pool).Create(ctx, email, email, "synthetic-not-a-login-password", "Commissioning fixture", "active")
	if err != nil {
		t.Fatal(err)
	}
	terms.Pilot.OwnerID = user.ID
	if _, err = pool.Exec(ctx, `INSERT INTO user_entitlements(user_id,entitlement_key) VALUES($1,'founder')`, user.ID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status,encrypted_credential_payload)
	 VALUES($1,'financial','coinbase','Synthetic commissioning account','active',decode(repeat('11',32),'hex')) RETURNING id::text`, user.ID).Scan(&terms.Pilot.ConnectionID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO financial_accounts(user_id,provider_connection_id,provider_name,provider_account_id,display_name,base_currency,status)
	 VALUES($1,$2,'coinbase','portfolio:'||gen_random_uuid()::text,'Synthetic commissioning account','USD','active') RETURNING id::text`, user.ID, terms.Pilot.ConnectionID).Scan(&terms.Pilot.AccountID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO capital_buckets(user_id,financial_account_id,name,allocation_type,allocation_value,currency,status)
	 VALUES($1,$2,'Synthetic commissioning bucket','FIXED_AMOUNT',100,'USD','ACTIVE') RETURNING id::text`, user.ID, terms.Pilot.AccountID).Scan(&terms.Pilot.CapitalBucketID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status)
	 VALUES($1,'ai','openai','Synthetic commissioning AI','active') RETURNING id::text`, user.ID).Scan(&terms.AIConnectionID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO auth_totp_factors(user_id,secret_ciphertext,enabled_at)
	 VALUES($1,decode(repeat('22',32),'hex'),clock_timestamp()-interval '1 minute')`, user.ID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT date_trunc('second',clock_timestamp())-interval '1 second'`).Scan(&terms.EffectiveFrom); err != nil {
		t.Fatal(err)
	}
	terms.EffectiveFrom = terms.EffectiveFrom.UTC()
	terms.Pilot.Limits.ExpiresAt = terms.EffectiveFrom.Add(time.Hour)
	return terms
}

// Discard successful response bytes to model a browser losing a response after
// the server commits. Recovery must use GET, not a new setup or MFA operation.
type lostCommissioningHTTPResponse struct {
	header stdhttp.Header
	status int
}

func (w *lostCommissioningHTTPResponse) Header() stdhttp.Header { return w.header }
func (w *lostCommissioningHTTPResponse) WriteHeader(status int) { w.status = status }
func (w *lostCommissioningHTTPResponse) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = stdhttp.StatusOK
	}
	return len(p), nil
}

func TestPostgresOwnerCommissioningHTTPAuthenticatedRecoveryAndRevocation(t *testing.T) {
	ctx, pool := setupCommissioningHTTPPostgres(t)
	terms := newCommissioningHTTPPostgresTerms(t, ctx, pool)
	stepUp := &commissioningPostgresStepUp{pool: pool}
	workflow, err := execution.NewOwnerCommissioning(execution.NewPostgresStore(pool), terms, stepUp)
	if err != nil {
		t.Fatal(err)
	}
	review, err := workflow.Review(ctx, authorization.Principal{UserID: terms.Pilot.OwnerID, Entitlement: authorization.EntitlementFounder})
	if err != nil {
		t.Fatal(err)
	}
	// Preserve all history but archive this test's synthetic LIVE mandate, so
	// later operational acceptance tests do not see active fixture inventory.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := pool.Exec(cleanupCtx, `WITH archived AS (
		 UPDATE automation_mandates m SET execution_mode='SHADOW',status='ARCHIVED',updated_at=clock_timestamp(),
		 current_version=GREATEST(m.current_version,COALESCE((SELECT max(v.version_number) FROM automation_mandate_versions v WHERE v.mandate_id=m.id),0))+1
		 WHERE m.id=$1 AND m.user_id=$2 RETURNING m.*
		) INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary)
		 SELECT id,current_version,user_id,'SYSTEM',(to_jsonb(archived)-ARRAY['id','user_id','current_version','created_at','updated_at'])||'{"execution_capable":false}'::jsonb,
		 '{"change":"synthetic HTTP commissioning archived; immutable history preserved"}'::jsonb FROM archived`, review.MandateID, terms.Pilot.OwnerID)
		if err != nil {
			t.Errorf("archive synthetic HTTP commissioning: %v", err)
		}
	})
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	sessions := auth.NewRedisStore(client)
	service := auth.NewService(auth.NewPostgresStore(pool), sessions, sessions, auditSink{}, time.Hour)
	token, _, err := sessions.Create(ctx, terms.Pilot.OwnerID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"https://owner.example"}}
	cookie := &stdhttp.Cookie{Name: cfg.SessionCookie, Value: token}
	handler := NewOwnerCommissioningHandler(cfg, service, workflow)
	assertCounts := func(pilots, mandates, consents int) {
		t.Helper()
		var got [7]int
		err := pool.QueryRow(ctx, `SELECT
		 (SELECT count(*) FROM execution_pilot_allocations WHERE owner_id=$1),
		 (SELECT count(*) FROM automation_mandates WHERE user_id=$1),
		 (SELECT count(*) FROM automation_mandate_versions WHERE mandate_id=$2),
		 (SELECT count(*) FROM execution_mandate_approvals WHERE owner_id=$1),
		 (SELECT count(*) FROM execution_orders WHERE owner_id=$1),
		 (SELECT count(*) FROM execution_dispatch_attempts WHERE owner_id=$1),
		 (SELECT count(*) FROM execution_scheduled_proposals WHERE owner_id=$1)`, terms.Pilot.OwnerID, review.MandateID).
			Scan(&got[0], &got[1], &got[2], &got[3], &got[4], &got[5], &got[6])
		if want := [7]int{pilots, mandates, mandates, consents, 0, 0, 0}; err != nil || got != want {
			t.Fatal("HTTP commissioning persisted unexpected state", got, want, err)
		}
	}
	assertCounts(0, 0, 0)
	assertOwnerExecutionResponse(t, commissioningHTTPRequest(handler, nil, "GET", "/context", "", ""), stdhttp.StatusUnauthorized)
	contextResponse := commissioningHTTPRequest(handler, cookie, "GET", "/context", "", "")
	assertOwnerExecutionResponse(t, contextResponse, stdhttp.StatusOK)
	var view struct {
		Available bool                   `json:"available"`
		Binding   string                 `json:"session_binding"`
		Review    ownerCommissioningView `json:"review"`
	}
	if err = json.Unmarshal(contextResponse.Body.Bytes(), &view); err != nil || !view.Available || view.Binding == "" || view.Review != commissioningView(review) {
		t.Fatal("real authentication did not recover exact commissioning context", err, contextResponse.Body.String())
	}
	assertCounts(0, 0, 0)
	loseResponse := func(path, body string) {
		t.Helper()
		r := httptest.NewRequest(stdhttp.MethodPost, ownerCommissioningRoot+path, strings.NewReader(body)).WithContext(ctx)
		r.AddCookie(cookie)
		r.Header.Set("Origin", "https://owner.example")
		r.Header.Set("X-Arbion-Execution-Session", view.Binding)
		w := &lostCommissioningHTTPResponse{header: make(stdhttp.Header)}
		handler.ServeHTTP(w, r)
		if w.status != stdhttp.StatusOK {
			t.Fatalf("write did not commit before simulated response loss: %s status=%d", path, w.status)
		}
	}
	loseResponse("/prepare", `{"expected_terms_digest":"`+view.Review.TermsDigest+`"}`)
	assertCounts(1, 1, 0)
	if stepUp.calls.Load() != 0 {
		t.Fatal("preparation consumed MFA")
	}
	receiptResponse := commissioningHTTPRequest(handler, cookie, "GET", "/receipt", "", "", view.Binding)
	assertOwnerExecutionResponse(t, receiptResponse, stdhttp.StatusOK)
	var receipt struct {
		Receipt struct {
			ownerCommissioningView
			SnapshotDigest string `json:"snapshot_digest"`
		} `json:"receipt"`
	}
	if err = json.Unmarshal(receiptResponse.Body.Bytes(), &receipt); err != nil || receipt.Receipt.ownerCommissioningView != view.Review || len(receipt.Receipt.SnapshotDigest) != 64 {
		t.Fatal("saved receipt recovery changed prepared terms", err)
	}
	approveBody := `{"expected_terms_digest":"` + view.Review.TermsDigest + `","expected_snapshot_digest":"` + receipt.Receipt.SnapshotDigest + `","mfa_code":"654321"}`
	loseResponse("/approve", approveBody)
	assertCounts(1, 1, 1)
	if stepUp.calls.Load() != 1 {
		t.Fatal("separate approval did not consume exactly one MFA proof")
	}
	getConsent := func(h stdhttp.Handler) execution.OwnerCommissioningConsent {
		t.Helper()
		response := commissioningHTTPRequest(h, cookie, "GET", "/consent", "", "", view.Binding)
		assertOwnerExecutionResponse(t, response, stdhttp.StatusOK)
		var body struct {
			Consent execution.OwnerCommissioningConsent `json:"consent"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Consent.ID == "" || body.Consent.SnapshotDigest != receipt.Receipt.SnapshotDigest {
			t.Fatal("server-derived consent recovery failed", err)
		}
		for _, private := range []string{terms.Pilot.OwnerID, terms.Pilot.AccountID, terms.Pilot.ConnectionID, terms.Pilot.CapitalBucketID, terms.AIConnectionID, "mfa_code", "credential_generation"} {
			if strings.Contains(response.Body.String(), private) {
				t.Fatal("consent response disclosed internal authority fields")
			}
		}
		return body.Consent
	}
	first := getConsent(handler)
	if first.RevokedAt != nil || !first.ExpiresAt.Equal(terms.Pilot.Limits.ExpiresAt) {
		t.Fatal("initial receipt has wrong immutable deadline or revocation")
	}
	// A new domain/store and handler must recover without retaining response IDs.
	restartedWorkflow, err := execution.NewOwnerCommissioning(execution.NewPostgresStore(pool), terms, stepUp)
	if err != nil {
		t.Fatal(err)
	}
	restartedService := auth.NewService(auth.NewPostgresStore(pool), auth.NewRedisStore(client), sessions, auditSink{}, time.Hour)
	handler = NewOwnerCommissioningHandler(cfg, restartedService, restartedWorkflow)
	if got := getConsent(handler); !reflect.DeepEqual(got, first) {
		t.Fatal("handler restart changed immutable consent", got, first)
	}
	assertOwnerExecutionResponse(t, commissioningHTTPRequest(handler, cookie, "POST", "/approve", "https://owner.example", approveBody, view.Binding), stdhttp.StatusOK)
	if got := getConsent(handler); !reflect.DeepEqual(got, first) || stepUp.calls.Load() != 1 {
		t.Fatal("approval replay renewed consent or consumed MFA")
	}
	assertOwnerExecutionResponse(t, commissioningHTTPRequest(handler, cookie, "POST", "/revoke", "https://owner.example", `{}`, view.Binding), stdhttp.StatusOK)
	revoked := getConsent(handler)
	if revoked.RevokedAt == nil || revoked.ID != first.ID || !revoked.ApprovedAt.Equal(first.ApprovedAt) || !revoked.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatal("revocation changed immutable receipt or failed to expose revocation")
	}
	assertOwnerExecutionResponse(t, commissioningHTTPRequest(handler, cookie, "POST", "/approve", "https://owner.example", approveBody, view.Binding), stdhttp.StatusOK)
	if got := getConsent(handler); !reflect.DeepEqual(got, revoked) || stepUp.calls.Load() != 1 {
		t.Fatal("post-revocation replay reactivated consent or consumed MFA")
	}
	assertCounts(1, 1, 1)
	var revocations int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_mandate_revocations WHERE approval_id=$1`, first.ID).Scan(&revocations); err != nil || revocations != 1 {
		t.Fatal("revocation was not durable", revocations, err)
	}
	// The cookie carries no durable entitlement grant: each request loads the
	// current founder status from PostgreSQL before reaching the domain workflow.
	if _, err = pool.Exec(ctx, `UPDATE user_entitlements SET status='revoked' WHERE user_id=$1 AND entitlement_key='founder'`, terms.Pilot.OwnerID); err != nil {
		t.Fatal(err)
	}
	assertOwnerExecutionResponse(t, commissioningHTTPRequest(handler, cookie, "GET", "/context", "", ""), stdhttp.StatusForbidden)
	assertCounts(1, 1, 1)
}
