package execution

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ownerCommissioningFixture struct {
	terms     OwnerCommissioningTerms
	principal authorization.Principal
	review    OwnerCommissioningReview
	workflow  *OwnerCommissioning
}

func newOwnerCommissioningFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) ownerCommissioningFixture {
	t.Helper()
	r := newExecutionFixture(t, ctx, pool)
	terms := ownerCommissioningTermsFixture()
	terms.Pilot.OwnerID, terms.Pilot.AccountID, terms.Pilot.ConnectionID, terms.Pilot.CapitalBucketID = r.OwnerID, r.AccountID, r.ConnectionID, r.CapitalBucketID
	if err := pool.QueryRow(ctx, `SELECT date_trunc('second',clock_timestamp())-interval '1 second'`).Scan(&terms.EffectiveFrom); err != nil {
		t.Fatal(err)
	}
	terms.EffectiveFrom = terms.EffectiveFrom.UTC()
	terms.Pilot.Limits.ExpiresAt = terms.EffectiveFrom.Add(time.Hour)
	if _, err := pool.Exec(ctx, `UPDATE financial_accounts SET provider_account_id='portfolio:'||id::text WHERE id=$1`, r.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE provider_connections SET encrypted_credential_payload=decode(repeat('11',32),'hex') WHERE id=$1`, r.ConnectionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO auth_totp_factors(user_id,secret_ciphertext,enabled_at) VALUES($1,decode(repeat('22',32),'hex'),clock_timestamp()-interval '1 minute')`, r.OwnerID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status) VALUES($1,'ai','openai','Commissioning fixture','active') RETURNING id::text`, r.OwnerID).Scan(&terms.AIConnectionID); err != nil {
		t.Fatal(err)
	}
	f := ownerCommissioningFixture{terms: terms, principal: authorization.Principal{UserID: r.OwnerID, Entitlement: authorization.EntitlementFounder}}
	var err error
	f.workflow, err = NewOwnerCommissioning(NewPostgresStore(pool), terms, mandateConsentStepUp(pool))
	if err != nil {
		t.Fatal(err)
	}
	f.review, err = f.workflow.Review(ctx, f.principal)
	if err != nil {
		t.Fatal(err)
	}
	// Keep exact immutable history, but do not leave synthetic LIVE inventory for
	// later operational acceptance tests. Register before any Prepare can fail.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := pool.Exec(cleanupCtx, `WITH archived AS (
		 UPDATE automation_mandates m SET execution_mode='SHADOW',status='ARCHIVED',updated_at=clock_timestamp(),
		 current_version=GREATEST(m.current_version,COALESCE((SELECT max(v.version_number) FROM automation_mandate_versions v WHERE v.mandate_id=m.id),0))+1
		 WHERE m.id=$1 AND m.user_id=$2 RETURNING m.*
		) INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary)
		 SELECT id,current_version,user_id,'SYSTEM',(to_jsonb(archived)-ARRAY['id','user_id','current_version','created_at','updated_at'])||'{"execution_capable":false}'::jsonb,
		 '{"change":"synthetic commissioning archived; immutable history preserved"}'::jsonb FROM archived`, f.review.MandateID, r.OwnerID)
		if err != nil {
			t.Errorf("archive synthetic commissioning: %v", err)
		}
	})
	return f
}

func assertCommissioningCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f ownerCommissioningFixture, pilots, mandates, versions, consents, orders, attempts int) {
	t.Helper()
	var got [6]int
	err := pool.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM execution_pilot_allocations WHERE owner_id=$1),
	 (SELECT count(*) FROM automation_mandates WHERE user_id=$1),
	 (SELECT count(*) FROM automation_mandate_versions WHERE mandate_id=$2),
	 (SELECT count(*) FROM execution_mandate_approvals WHERE owner_id=$1),
	 (SELECT count(*) FROM execution_orders WHERE owner_id=$1),
	 (SELECT count(*) FROM execution_dispatch_attempts WHERE owner_id=$1)`, f.principal.UserID, f.review.MandateID).
		Scan(&got[0], &got[1], &got[2], &got[3], &got[4], &got[5])
	if want := [6]int{pilots, mandates, versions, consents, orders, attempts}; err != nil || got != want {
		t.Fatal("commissioning side effects mismatch", got, want, err)
	}
}

func prepareOwnerCommissioning(t *testing.T, ctx context.Context, f ownerCommissioningFixture) OwnerCommissioningReceipt {
	t.Helper()
	r, err := f.workflow.Prepare(ctx, f.principal, f.review.TermsDigest)
	if err != nil {
		t.Fatal(err)
	}
	if r.OwnerCommissioningReview != f.review || len(r.SnapshotDigest) != 64 {
		t.Fatal("prepared receipt changed reviewed terms", r)
	}
	return r
}

func TestPostgresOwnerCommissioningAtomicPrepareSeparateConsentAndJoinedLifecycle(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newOwnerCommissioningFixture(t, ctx, pool)
	assertCommissioningCounts(t, ctx, pool, f, 0, 0, 0, 0, 0, 0)
	if _, err := f.workflow.Read(ctx, f.principal); !errors.Is(err, ErrNotFound) {
		t.Fatal("review created a receipt", err)
	}
	r := prepareOwnerCommissioning(t, ctx, f)
	assertCommissioningCounts(t, ctx, pool, f, 1, 1, 1, 0, 0, 0)
	a, err := f.workflow.Approve(ctx, f.principal, r.TermsDigest, r.SnapshotDigest, "synthetic")
	if err != nil || a.MandateID != r.MandateID || a.MandateVersion != 1 || a.SnapshotDigest != r.SnapshotDigest || !a.ExpiresAt.Equal(f.terms.Pilot.Limits.ExpiresAt) {
		t.Fatal("fresh separate consent mismatch", a, err)
	}
	assertCommissioningCounts(t, ctx, pool, f, 1, 1, 1, 1, 0, 0)
	read, err := f.workflow.ReadConsent(ctx, f.principal, a.ID)
	if err != nil || !reflect.DeepEqual(read, a) {
		t.Fatal("exact consent recovery failed", read, err)
	}
	// Join the new commissioning entry to the already-tested intake, mock-only
	// send, reconciliation and settlement helpers. This never contacts a broker.
	request := Request{OwnerID: f.terms.Pilot.OwnerID, AccountID: f.terms.Pilot.AccountID, ConnectionID: f.terms.Pilot.ConnectionID,
		CapitalBucketID: f.terms.Pilot.CapitalBucketID, ProductID: f.terms.Pilot.ProductID, Side: "BUY", BaseSize: "0.0001", LimitPrice: "60000",
		FeeAllowanceUSD: "0.06", MaximumDebitUSD: "6.06", PilotLimits: &f.terms.Pilot.Limits, MandateApprovalID: a.ID}
	c := mandateConsentFixture{request: request, mandateID: r.MandateID, snapshotDigest: r.SnapshotDigest, aiConnectionID: f.terms.AIConnectionID, consent: a, principal: f.principal}
	order := newMandateSendOrder(t, ctx, pool, c, "BUY")
	sendMandateOrder(t, ctx, pool, c, order)
	settleMandateOrder(t, ctx, pool, order)
	assertPilotAllocationBalance(t, ctx, pool, request, "93.94", "0.0001", 1, false)
	assertCommissioningCounts(t, ctx, pool, f, 1, 1, 1, 1, 1, 1)
	if err := f.workflow.Revoke(ctx, f.principal, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.workflow.Revoke(ctx, f.principal, a.ID); err != nil {
		t.Fatal("revocation is not idempotent", err)
	}
	if got, err := f.workflow.ReadConsent(ctx, f.principal, a.ID); err != nil || !reflect.DeepEqual(got, a) {
		t.Fatal("revocation changed immutable consent", got, err)
	}
	var until time.Time
	if err := pool.QueryRow(ctx, `SELECT check_execution_mandate_consent($1,$2,$3)`, a.ID, f.principal.UserID, f.terms.Pilot.CapitalBucketID).Scan(&until); err == nil {
		t.Fatal("revoked consent remained valid")
	}
}

func TestPostgresOwnerCommissioningConcurrentPrepareAndLostCommit(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newOwnerCommissioningFixture(t, ctx, pool)
	type result struct {
		receipt OwnerCommissioningReceipt
		err     error
	}
	results := make(chan result, 6)
	var wg sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, err := NewOwnerCommissioning(NewPostgresStore(pool), f.terms, ownerNoIO{})
			if err != nil {
				results <- result{err: err}
				return
			}
			r, err := w.Prepare(ctx, f.principal, f.review.TermsDigest)
			results <- result{r, err}
		}()
	}
	wg.Wait()
	close(results)
	var first OwnerCommissioningReceipt
	for got := range results {
		if got.err != nil {
			t.Fatal(got.err)
		}
		if first.MandateID == "" {
			first = got.receipt
		}
		if got.receipt != first {
			t.Fatal("concurrent preparation changed receipt", got.receipt, first)
		}
	}
	assertCommissioningCounts(t, ctx, pool, f, 1, 1, 1, 0, 0, 0)
	t.Run("lost commit", func(t *testing.T) {
		f := newOwnerCommissioningFixture(t, ctx, pool)
		w, err := NewOwnerCommissioning(NewPostgresStore(lostCommitDB{pool}), f.terms, ownerNoIO{})
		if err != nil {
			t.Fatal(err)
		}
		original, err := w.Prepare(ctx, f.principal, f.review.TermsDigest)
		if !errors.Is(err, ErrCommitUnknown) || original.MandateID != f.review.MandateID || original.SnapshotDigest == "" {
			t.Fatal("ambiguous commit lost stable receipt", original, err)
		}
		got := prepareOwnerCommissioning(t, ctx, f)
		if got != original {
			t.Fatal("commit replay replaced original", got, original)
		}
		if got, err := f.workflow.Read(ctx, f.principal); err != nil || got != original {
			t.Fatal("read after lost commit failed", got, err)
		}
		assertCommissioningCounts(t, ctx, pool, f, 1, 1, 1, 0, 0, 0)
	})
}

func TestPostgresOwnerCommissioningChangedTermsAndStaleReviewAreDeniedBeforeMFA(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newOwnerCommissioningFixture(t, ctx, pool)
	if _, err := f.workflow.Prepare(ctx, f.principal, strings.Repeat("0", 64)); !errors.Is(err, ErrConflict) {
		t.Fatal("stale review prepared terms", err)
	}
	assertCommissioningCounts(t, ctx, pool, f, 0, 0, 0, 0, 0, 0)
	r := prepareOwnerCommissioning(t, ctx, f)
	for name, mutate := range changedCommissioningTerms() {
		if name == "owner" || name == "account" {
			continue
		} // Different identity denied separately, never relabeled.
		t.Run(name, func(t *testing.T) {
			changed := f.terms
			mutate(&changed)
			w, err := NewOwnerCommissioning(NewPostgresStore(pool), changed, ownerNoIO{})
			if err != nil {
				t.Fatal(err)
			}
			review, err := w.Review(ctx, f.principal)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = w.Prepare(ctx, f.principal, review.TermsDigest); !errors.Is(err, ErrConflict) {
				t.Fatal("changed terms replaced original", err)
			}
			if _, err = w.Read(ctx, f.principal); !errors.Is(err, ErrConflict) {
				t.Fatal("changed terms read original as new agreement", err)
			}
		})
	}
	w, err := NewOwnerCommissioning(NewPostgresStore(pool), f.terms, ownerNoIO{})
	if err != nil {
		t.Fatal(err)
	}
	for _, digests := range [][2]string{{"stale", r.SnapshotDigest}, {r.TermsDigest, "stale"}} {
		if _, err := w.Approve(ctx, f.principal, digests[0], digests[1], "synthetic"); !errors.Is(err, ErrConflict) {
			t.Fatal("stale review consumed MFA", err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE user_entitlements SET status='revoked' WHERE user_id=$1`, f.principal.UserID); err != nil {
		t.Fatal(err)
	}
	for name, call := range commissioningCommands(w, f.principal) {
		t.Run("revoked founder/"+name, func(t *testing.T) {
			if err := call(ctx); !errors.Is(err, ErrNotAuthorized) {
				t.Fatal("revoked founder retained access", err)
			}
		})
	}
	assertCommissioningCounts(t, ctx, pool, f, 1, 1, 1, 0, 0, 0)
}

func TestPostgresOwnerCommissioningFailedCurrentControlsLeaveNoPartialSetup(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, future := range []bool{false, true} {
		t.Run("outside current pilot window", func(t *testing.T) {
			f := newOwnerCommissioningFixture(t, ctx, pool)
			terms := f.terms
			if future {
				terms.EffectiveFrom = terms.EffectiveFrom.Add(time.Hour)
				terms.Pilot.Limits.ExpiresAt = terms.Pilot.Limits.ExpiresAt.Add(time.Hour)
			} else {
				terms.EffectiveFrom = terms.EffectiveFrom.Add(-2 * time.Hour)
				terms.Pilot.Limits.ExpiresAt = terms.Pilot.Limits.ExpiresAt.Add(-2 * time.Hour)
			}
			w, err := NewOwnerCommissioning(NewPostgresStore(pool), terms, ownerNoIO{})
			if err != nil {
				t.Fatal(err)
			}
			review, err := w.Review(ctx, f.principal)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Prepare(ctx, f.principal, review.TermsDigest); err == nil {
				t.Fatal("out-of-window pilot commissioned")
			}
			assertCommissioningCounts(t, ctx, pool, f, 0, 0, 0, 0, 0, 0)
		})
	}
	for name, query := range map[string]string{
		"financial disabled": `UPDATE provider_connections SET status='disabled' WHERE id=$1`,
		"financial expired":  `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`,
		"credentials absent": `UPDATE provider_connections SET encrypted_credential_payload=NULL WHERE id=$1`,
		"AI disabled":        `UPDATE provider_connections SET status='disabled' WHERE id=$2`,
		"AI expired":         `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()-interval '1 second' WHERE id=$2`,
		"account inactive":   `UPDATE financial_accounts SET status='disabled' WHERE id=$3`,
		"bucket protected":   `UPDATE capital_buckets SET protected_amount=1 WHERE id=$4`,
		"owner revoked":      `UPDATE user_entitlements SET status='revoked' WHERE user_id=$5`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newOwnerCommissioningFixture(t, ctx, pool)
			// A typed CTE avoids unused parameter inference in the individual update.
			query = `WITH params AS (SELECT $1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid) ` + query
			if _, err := pool.Exec(ctx, query, f.terms.Pilot.ConnectionID, f.terms.AIConnectionID, f.terms.Pilot.AccountID, f.terms.Pilot.CapitalBucketID, f.principal.UserID); err != nil {
				t.Fatal(err)
			}
			if _, err := f.workflow.Prepare(ctx, f.principal, f.review.TermsDigest); err == nil {
				t.Fatal("unavailable controls commissioned policy")
			}
			assertCommissioningCounts(t, ctx, pool, f, 0, 0, 0, 0, 0, 0)
		})
	}
	for _, scope := range []string{"USER", "ACCOUNT", "AUTOMATION"} {
		t.Run("stop/"+scope, func(t *testing.T) {
			f := newOwnerCommissioningFixture(t, ctx, pool)
			id := newSendStopID(t, ctx, pool)
			scopeID := f.principal.UserID
			if scope == "ACCOUNT" {
				scopeID = f.terms.Pilot.AccountID
			}
			if scope == "AUTOMATION" {
				scopeID = f.review.MandateID
			}
			if _, err := pool.Exec(ctx, `INSERT INTO risk_circuit_breakers(id,scope,scope_id,state,reason,source) VALUES($1,$2,$3,'OPEN','synthetic commissioning stop','SYSTEM')`, id, scope, scopeID); err != nil {
				t.Fatal(err)
			}
			if _, err := f.workflow.Prepare(ctx, f.principal, f.review.TermsDigest); err == nil {
				t.Fatal("open breaker commissioned policy")
			}
			assertCommissioningCounts(t, ctx, pool, f, 0, 0, 0, 0, 0, 0)
		})
	}
}

func TestPostgresOwnerCommissioningOriginalReceiptSurvivesCurrentPolicyDrift(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for name, query := range map[string]string{
		"version and status": `UPDATE automation_mandates SET current_version=2,status='PAUSED' WHERE id=$1`,
		"expired":            `UPDATE automation_mandates SET effective_until=effective_from+interval '0.1 second' WHERE id=$1`,
		"different account and bucket": `WITH account AS (
		 INSERT INTO financial_accounts(user_id,provider_connection_id,provider_name,provider_account_id,display_name)
		 SELECT m.user_id,a.provider_connection_id,'coinbase','alternate:'||gen_random_uuid()::text,'Synthetic replacement'
		 FROM automation_mandates m JOIN financial_accounts a ON a.id=m.financial_account_id WHERE m.id=$1 RETURNING id,user_id
		), bucket AS (
		 INSERT INTO capital_buckets(user_id,financial_account_id,name,allocation_type,allocation_value,currency,status)
		 SELECT user_id,id,'Synthetic replacement','FIXED_AMOUNT',100,'USD','ACTIVE' FROM account RETURNING id,financial_account_id
		) UPDATE automation_mandates SET financial_account_id=bucket.financial_account_id,capital_bucket_id=bucket.id FROM bucket WHERE automation_mandates.id=$1`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newOwnerCommissioningFixture(t, ctx, pool)
			original := prepareOwnerCommissioning(t, ctx, f)
			if _, err := pool.Exec(ctx, query, original.MandateID); err != nil {
				t.Fatal(err)
			}
			w, err := NewOwnerCommissioning(NewPostgresStore(pool), f.terms, ownerNoIO{})
			if err != nil {
				t.Fatal(err)
			}
			if got, err := w.Read(ctx, f.principal); err != nil || got != original {
				t.Fatal("current drift erased immutable receipt", got, err)
			}
			if got, err := w.Prepare(ctx, f.principal, original.TermsDigest); err != nil || got != original {
				t.Fatal("exact historical retry failed", got, err)
			}
			if _, err := w.Approve(ctx, f.principal, original.TermsDigest, original.SnapshotDigest, "synthetic"); err == nil {
				t.Fatal("historical receipt authorized drifted policy")
			}
			assertCommissioningCounts(t, ctx, pool, f, 1, 1, 1, 0, 0, 0)
		})
	}
}

func waitCommissioningBlocked(t *testing.T, ctx context.Context, pool *pgxpool.Pool, pid uint32) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, int64(pid)).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("commissioning did not reach the held database lock")
}

func TestPostgresOwnerCommissioningWaitRechecksExpiredFinancialAuthorization(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newOwnerCommissioningFixture(t, ctx, pool)
	// This AI row lock deliberately does not take the lifecycle advisory lock.
	// Prepare passes its initial financial check, then waits here while the
	// financial authorization expires; its final check must roll everything back.
	if _, err := pool.Exec(ctx, `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()+interval '3 seconds' WHERE id=$1`, f.terms.Pilot.ConnectionID); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, `SELECT 1 FROM provider_connections WHERE id=$1 FOR UPDATE`, f.terms.AIConnectionID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := f.workflow.Prepare(ctx, f.principal, f.review.TermsDigest); done <- err }()
	waitCommissioningBlocked(t, ctx, pool, tx.Conn().PgConn().PID())
	if _, err := pool.Exec(ctx, `SELECT pg_sleep(GREATEST(0,extract(epoch FROM authorization_expires_at-clock_timestamp()))+0.05) FROM provider_connections WHERE id=$1`, f.terms.Pilot.ConnectionID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("expired financial authorization crossed the final wait", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("commissioning deadlocked after AI lock release")
	}
	assertCommissioningCounts(t, ctx, pool, f, 0, 0, 0, 0, 0, 0)
}

func TestPostgresOwnerCommissioningContendedRevocationRollsBack(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, kind := range []string{"founder", "AI connection"} {
		t.Run(kind, func(t *testing.T) {
			f := newOwnerCommissioningFixture(t, ctx, pool)
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer rollback(tx)
			query, id := `UPDATE user_entitlements SET status='revoked' WHERE user_id=$1`, f.principal.UserID
			if kind == "AI connection" {
				query, id = `UPDATE provider_connections SET status='disabled' WHERE id=$1`, f.terms.AIConnectionID
			}
			if _, err = tx.Exec(ctx, query, id); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, err := f.workflow.Prepare(ctx, f.principal, f.review.TermsDigest); done <- err }()
			waitCommissioningBlocked(t, ctx, pool, tx.Conn().PgConn().PID())
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, ErrNotAuthorized) {
					t.Fatal("committed revocation did not deny setup", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("commissioning deadlocked with revocation")
			}
			assertCommissioningCounts(t, ctx, pool, f, 0, 0, 0, 0, 0, 0)
		})
	}
}
