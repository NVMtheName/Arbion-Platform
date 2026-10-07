package execution

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mandateConsentFixture struct {
	request                                   Request
	mandateID, snapshotDigest, aiConnectionID string
	consent                                   MandateConsent
	principal                                 authorization.Principal
}

func mandateConsentStepUp(pool *pgxpool.Pool) stepUpFunc {
	return func(ctx context.Context, owner, _ string) (string, time.Time, error) {
		var verified time.Time
		err := pool.QueryRow(ctx, `UPDATE auth_totp_factors SET updated_at=clock_timestamp(),last_used_step=floor(extract(epoch FROM clock_timestamp())/30) WHERE user_id=$1 RETURNING updated_at`, owner).Scan(&verified)
		return "totp", verified, err
	}
}

func newMandateConsentSetup(t *testing.T, ctx context.Context, pool *pgxpool.Pool, expiry ...time.Time) mandateConsentFixture {
	t.Helper()
	r := newPilotAllocationRequest(t, ctx, pool, "100")
	r.BaseSize, r.LimitPrice, r.FeeAllowanceUSD, r.MaximumDebitUSD = "0.0001", "60000", "0.06", "6.06"
	f := mandateConsentFixture{request: r, principal: authorization.Principal{UserID: r.OwnerID, Entitlement: authorization.EntitlementFounder}}
	var until *time.Time
	if len(expiry) != 0 {
		until = &expiry[0]
	}
	if _, err := pool.Exec(ctx, `UPDATE provider_connections SET encrypted_credential_payload=decode(repeat('11',32),'hex') WHERE id=$1`, r.ConnectionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO auth_totp_factors(user_id,secret_ciphertext,enabled_at) VALUES($1,decode(repeat('22',32),'hex'),clock_timestamp()-interval '1 minute')`, r.OwnerID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO provider_connections(user_id,provider_category,provider_name,display_name,status) VALUES($1,'ai','openai','Consent fixture','active') RETURNING id::text`, r.OwnerID).Scan(&f.aiConnectionID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO automation_mandates(user_id,financial_account_id,capital_bucket_id,ai_provider_connection_id,ai_model_id,automation_type,autonomy_level,execution_mode,status,current_version,strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,effective_from,effective_until)
	 VALUES($1,$2,$3,$4,'gpt-5.4','AI_AUTONOMOUS','FULL_AUTONOMOUS','LIVE','READY',1,
	 '{"profile":"COINBASE_SPOT_PILOT_V1","objective":"Test-only bounded spot decisions","max_proposal_notional":"25"}',
	 '{"max_trades_per_day":2}', '{"symbols":["BTC"]}', '{"symbols":[]}',clock_timestamp()-interval '1 minute',COALESCE($5::timestamptz,clock_timestamp()+interval '2 hours')) RETURNING id::text`, r.OwnerID, r.AccountID, r.CapitalBucketID, f.aiConnectionID, until).Scan(&f.mandateID); err != nil {
		t.Fatal(err)
	}
	// Tests intentionally exercise an otherwise-unwired LIVE policy. Archive its
	// exact current row before later operational-inventory tests run, preserving
	// every immutable consent, execution record and policy version. Register this
	// immediately after creation so even a failed snapshot/approval is isolated.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := pool.Exec(cleanupCtx, `WITH archived AS (
		 UPDATE automation_mandates m SET execution_mode='SHADOW',status='ARCHIVED',updated_at=clock_timestamp(),
		 current_version=GREATEST(m.current_version,COALESCE((SELECT max(v.version_number) FROM automation_mandate_versions v WHERE v.mandate_id=m.id),0))+1
		 WHERE m.id=$1 AND m.user_id=$2 AND m.financial_account_id=$3 AND m.capital_bucket_id=$4 RETURNING m.*
		) INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary)
		 SELECT id,current_version,user_id,'SYSTEM',(to_jsonb(archived)-ARRAY['id','user_id','current_version','created_at','updated_at'])||'{"execution_capable":false}'::jsonb,
		 '{"change":"test fixture archived; immutable execution history preserved"}'::jsonb FROM archived`, f.mandateID, r.OwnerID, r.AccountID, r.CapitalBucketID)
		if err != nil || result.RowsAffected() != 1 {
			t.Errorf("archive synthetic mandate fixture: affected=%d err=%v", result.RowsAffected(), err)
		}
	})
	if err := pool.QueryRow(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot)
	 SELECT m.id,1,m.user_id,'SYSTEM',(to_jsonb(m)-ARRAY['id','user_id','current_version','created_at','updated_at'])||'{"execution_capable":false}'::jsonb FROM automation_mandates m WHERE id=$1
	 RETURNING encode(sha256(convert_to(snapshot::text,'UTF8')),'hex')`, f.mandateID).Scan(&f.snapshotDigest); err != nil {
		t.Fatal(err)
	}
	return f
}

func newMandateConsentFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, expiry ...time.Time) mandateConsentFixture {
	t.Helper()
	f := newMandateConsentSetup(t, ctx, pool, expiry...)
	var err error
	f.consent, err = NewPostgresStore(pool).ApproveMandate(ctx, f.principal, f.request.CapitalBucketID, f.mandateID, 1, f.snapshotDigest, "synthetic", mandateConsentStepUp(pool))
	if err != nil {
		t.Fatal(err)
	}
	f.request.MandateApprovalID = f.consent.ID
	return f
}

func TestPostgresMandateConsentFixtureArchivesWithoutErasingHistory(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	var f mandateConsentFixture
	t.Run("synthetic live policy", func(t *testing.T) {
		f = newMandateConsentFixture(t, ctx, pool)
	})
	if f.consent.ID == "" {
		t.Fatal("fixture did not create consent")
	}
	var archived bool
	var versions int
	err := pool.QueryRow(ctx, `SELECT m.execution_mode='SHADOW' AND m.status='ARCHIVED'
	 AND m.current_version=2 AND execution_mandate_snapshot_matches(m,v.snapshot),
	 (SELECT count(*) FROM automation_mandate_versions WHERE mandate_id=m.id)
	 FROM automation_mandates m JOIN automation_mandate_versions v ON v.mandate_id=m.id AND v.version_number=m.current_version
	 WHERE m.id=$1 AND m.user_id=$2`, f.mandateID, f.request.OwnerID).Scan(&archived, &versions)
	if err != nil || !archived || versions != 2 {
		t.Fatal("synthetic LIVE policy leaked or immutable history changed", archived, versions, err)
	}
	got, err := NewPostgresStore(pool).ReadMandateConsent(ctx, f.request.OwnerID, f.consent.ID)
	if err != nil || !reflect.DeepEqual(got, f.consent) {
		t.Fatal("fixture cleanup rewrote original consent", got, err)
	}
}

func TestPostgresMandateConsentExactRecoveryRevocationAndImmutable(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newMandateConsentFixture(t, ctx, pool)
	s := NewPostgresStore(pool)
	a := f.consent
	if a.ID == "" || a.OwnerID != f.request.OwnerID || a.AccountID != f.request.AccountID || a.ConnectionID != f.request.ConnectionID || a.CapitalBucketID != f.request.CapitalBucketID || a.MandateID != f.mandateID || a.MandateVersion != 1 || a.SnapshotDigest != f.snapshotDigest || a.ExpiresAt.Sub(a.ApprovedAt) > 24*time.Hour {
		t.Fatal("incorrect immutable consent", a)
	}
	var until time.Time
	if err := pool.QueryRow(ctx, `SELECT check_execution_mandate_consent($1,$2,$3)`, a.ID, a.OwnerID, a.CapitalBucketID).Scan(&until); err != nil || !until.Equal(a.ExpiresAt) {
		t.Fatal("valid current consent denied", err)
	}
	for _, query := range []string{`UPDATE execution_mandate_approvals SET expires_at=expires_at+interval '1 second' WHERE id=$1`, `DELETE FROM execution_mandate_approvals WHERE id=$1`} {
		if _, err := pool.Exec(ctx, query, a.ID); err == nil {
			t.Fatal("mutable consent")
		}
	}
	for _, table := range []string{"execution_mandate_approvals", "execution_mandate_revocations", "automation_mandate_versions"} {
		if _, err := pool.Exec(ctx, "TRUNCATE "+table+" CASCADE"); err == nil {
			t.Fatal("immutable history truncatable", table)
		}
	}
	if err := s.RevokeMandateConsent(ctx, a.OwnerID, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeMandateConsent(ctx, a.OwnerID, a.ID); err != nil {
		t.Fatal("revocation not idempotent", err)
	}
	if err := pool.QueryRow(ctx, `SELECT check_execution_mandate_consent($1,$2,$3)`, a.ID, a.OwnerID, a.CapitalBucketID).Scan(&until); err == nil {
		t.Fatal("revoked consent authorized")
	}
	got, err := s.ReadMandateConsent(ctx, a.OwnerID, a.ID)
	if err != nil || !reflect.DeepEqual(got, a) {
		t.Fatal("historical consent recovery changed", got, err)
	}
}

func TestPostgresMandateConsentDigestBeforeMFAAndUnknownCommit(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newMandateConsentSetup(t, ctx, pool)
	called := false
	verifier := stepUpFunc(func(context.Context, string, string) (string, time.Time, error) {
		called = true
		return "totp", time.Time{}, nil
	})
	if _, err := NewPostgresStore(pool).ApproveMandate(ctx, f.principal, f.request.CapitalBucketID, f.mandateID, 1, strings.Repeat("0", 64), "synthetic", verifier); !errors.Is(err, ErrConflict) || called {
		t.Fatal("wrong snapshot consumed MFA", err, called)
	}
	a, err := NewPostgresStore(lostCommitDB{pool}).ApproveMandate(ctx, f.principal, f.request.CapitalBucketID, f.mandateID, 1, f.snapshotDigest, "synthetic", mandateConsentStepUp(pool))
	if !errors.Is(err, ErrCommitUnknown) || a.ID == "" {
		t.Fatal("ambiguous commit lost exact recovery identity", a, err)
	}
	got, err := NewPostgresStore(pool).ReadMandateConsent(ctx, a.OwnerID, a.ID)
	if err != nil || !reflect.DeepEqual(got, a) {
		t.Fatal("ambiguous consent not recoverable", got, err)
	}
}

func TestPostgresMandateConsentRejectsCurrentDrift(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for name, mutate := range map[string]string{
		"version":               `UPDATE automation_mandates SET current_version=2 WHERE id=$1`,
		"state":                 `UPDATE automation_mandates SET status='PAUSED' WHERE id=$1`,
		"terms without version": `UPDATE automation_mandates SET risk_parameters='{"max_trades_per_day":48}' WHERE id=$1`,
		"expiry":                `UPDATE automation_mandates SET effective_until=clock_timestamp()-interval '1 second' WHERE id=$1`,
		"AI key disabled":       `UPDATE provider_connections SET status='disabled' WHERE id=(SELECT ai_provider_connection_id FROM automation_mandates WHERE id=$1)`,
		"MFA replaced":          `UPDATE auth_totp_factors SET enabled_at=clock_timestamp() WHERE user_id=(SELECT user_id FROM automation_mandates WHERE id=$1)`,
		"financial generation":  `UPDATE provider_connections SET encrypted_credential_payload=decode(repeat('44',32),'hex') WHERE id=(SELECT provider_connection_id FROM execution_pilot_allocations WHERE capital_bucket_id=(SELECT capital_bucket_id FROM automation_mandates WHERE id=$1))`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newMandateConsentFixture(t, ctx, pool)
			if _, err := pool.Exec(ctx, mutate, f.mandateID); err != nil {
				t.Fatal(err)
			}
			var until time.Time
			if err := pool.QueryRow(ctx, `SELECT check_execution_mandate_consent($1,$2,$3)`, f.consent.ID, f.request.OwnerID, f.request.CapitalBucketID).Scan(&until); err == nil {
				t.Fatal("changed current controls authorized")
			}
		})
	}
}

func TestPostgresMandateConsentMFAIsNotReusableAcrossApprovalKinds(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, manualFirst := range []bool{false, true} {
		f := newMandateConsentSetup(t, ctx, pool)
		s := NewPostgresStore(pool)
		o, err := s.Prepare(ctx, f.request)
		if err != nil {
			t.Fatal(err)
		}
		method, verified, err := mandateConsentStepUp(pool)(ctx, f.request.OwnerID, "synthetic")
		if err != nil {
			t.Fatal(err)
		}
		proof := stepUpFunc(func(context.Context, string, string) (string, time.Time, error) { return method, verified, nil })
		manual := func() error {
			_, err := s.ApproveOrder(ctx, f.principal, o.ID, o.RequestDigest, "synthetic", proof)
			return err
		}
		mandate := func() error {
			_, err := s.ApproveMandate(ctx, f.principal, f.request.CapitalBucketID, f.mandateID, 1, f.snapshotDigest, "synthetic", proof)
			return err
		}
		first, second := mandate, manual
		if manualFirst {
			first, second = manual, mandate
		}
		if err := first(); err != nil {
			t.Fatal("first fresh verification refused", err)
		}
		if err := second(); err == nil {
			t.Fatal("one fresh MFA authorized two authority kinds")
		}
	}
}

func TestPostgresMandateConsentRevocationAndEditSerialize(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newMandateConsentFixture(t, ctx, pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(tx)
	var until time.Time
	if err = tx.QueryRow(ctx, `SELECT check_execution_mandate_consent($1,$2,$3)`, f.consent.ID, f.request.OwnerID, f.request.CapitalBucketID).Scan(&until); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- NewPostgresStore(pool).RevokeMandateConsent(ctx, f.request.OwnerID, f.consent.ID) }()
	select {
	case err := <-done:
		t.Fatal("revocation bypassed authority lock", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	// An edit already in progress must be observed after the mandate lock wait.
	g := newMandateConsentFixture(t, ctx, pool)
	edit, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(edit)
	if _, err = edit.Exec(ctx, `UPDATE automation_mandates SET status='PAUSED' WHERE id=$1`, g.mandateID); err != nil {
		t.Fatal(err)
	}
	go func() {
		var out time.Time
		done <- pool.QueryRow(ctx, `SELECT check_execution_mandate_consent($1,$2,$3)`, g.consent.ID, g.request.OwnerID, g.request.CapitalBucketID).Scan(&out)
	}()
	select {
	case err := <-done:
		t.Fatal("authority bypassed mandate edit lock", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err = edit.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("authority accepted state predating lock wait")
	}
}

func TestPostgresMandateConsentConcurrentMFAConsumption(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newMandateConsentSetup(t, ctx, pool)
	s := NewPostgresStore(pool)
	o, err := s.Prepare(ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	method, verified, err := mandateConsentStepUp(pool)(ctx, f.request.OwnerID, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	proof := stepUpFunc(func(context.Context, string, string) (string, time.Time, error) { return method, verified, nil })
	start, results := make(chan struct{}), make(chan error, 2)
	go func() {
		<-start
		_, err := s.ApproveOrder(ctx, f.principal, o.ID, o.RequestDigest, "synthetic", proof)
		results <- err
	}()
	go func() {
		<-start
		_, err := s.ApproveMandate(ctx, f.principal, f.request.CapitalBucketID, f.mandateID, 1, f.snapshotDigest, "synthetic", proof)
		results <- err
	}()
	close(start)
	successes := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			successes++
		}
	}
	var receipts int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_owner_approvals WHERE owner_id=$1)+(SELECT count(*) FROM execution_mandate_approvals WHERE owner_id=$1)`, f.request.OwnerID).Scan(&receipts); err != nil || receipts != 1 || successes != 1 {
		t.Fatal("concurrent proof reuse did not have one winner", successes, receipts, err)
	}
}

func TestPostgresMandateConsentRequiresVerifiedTOTPStep(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, manual := range []bool{false, true} {
		f := newMandateConsentSetup(t, ctx, pool)
		s := NewPostgresStore(pool)
		o, err := s.Prepare(ctx, f.request)
		if err != nil {
			t.Fatal(err)
		}
		proof := stepUpFunc(func(ctx context.Context, owner, _ string) (string, time.Time, error) {
			var verified time.Time
			err := pool.QueryRow(ctx, `UPDATE auth_totp_factors SET updated_at=clock_timestamp(),last_used_step=-1 WHERE user_id=$1 RETURNING updated_at`, owner).Scan(&verified)
			return "totp", verified, err
		})
		if manual {
			_, err = s.ApproveOrder(ctx, f.principal, o.ID, o.RequestDigest, "synthetic", proof)
		} else {
			_, err = s.ApproveMandate(ctx, f.principal, f.request.CapitalBucketID, f.mandateID, 1, f.snapshotDigest, "synthetic", proof)
		}
		if err == nil {
			t.Fatal("enabled but never verified factor approved authority", manual)
		}
	}
}

func TestPostgresMandateConsentDirectSQLScopeExpiryAndAIExpiry(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, kind := range []string{"foreign account", "foreign connection", "foreign digest", "mandate expiry", "24 hour expiry", "AI expiry"} {
		t.Run(kind, func(t *testing.T) {
			f := newMandateConsentFixture(t, ctx, pool)
			a := f.consent
			_, verified, err := mandateConsentStepUp(pool)(ctx, a.OwnerID, "synthetic")
			if err != nil {
				t.Fatal(err)
			}
			account, connection, digest, until := a.AccountID, a.ConnectionID, a.SnapshotDigest, a.ExpiresAt
			switch kind {
			case "foreign account":
				account = newExecutionFixture(t, ctx, pool).AccountID
			case "foreign connection":
				connection = f.aiConnectionID
			case "foreign digest":
				digest = strings.Repeat("0", 64)
			case "mandate expiry":
				until = a.ExpiresAt.Add(time.Hour)
			case "24 hour expiry":
				until = a.ApprovedAt.Add(25 * time.Hour)
			case "AI expiry":
				if _, err = pool.Exec(ctx, `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()+interval '1 hour' WHERE id=$1`, f.aiConnectionID); err != nil {
					t.Fatal(err)
				}
			}
			_, err = pool.Exec(ctx, `INSERT INTO execution_mandate_approvals(owner_id,financial_account_id,provider_connection_id,capital_bucket_id,mandate_id,mandate_version,snapshot_digest,credential_generation,mfa_method,mfa_verified_at,mfa_enabled_at,approved_at,expires_at)
			 SELECT owner_id,$2,$3,capital_bucket_id,mandate_id,mandate_version,$4,credential_generation,'totp',$5,mfa_enabled_at,clock_timestamp(),$6 FROM execution_mandate_approvals WHERE id=$1`, a.ID, account, connection, digest, verified, until)
			if err == nil {
				t.Fatal("direct SQL bypassed consent binding", kind)
			}
		})
	}
	f := newMandateConsentFixture(t, ctx, pool)
	var aiExpiry, until time.Time
	if err := pool.QueryRow(ctx, `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()+interval '1 hour' WHERE id=$1 RETURNING authorization_expires_at`, f.aiConnectionID).Scan(&aiExpiry); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT check_execution_mandate_consent($1,$2,$3)`, f.consent.ID, f.request.OwnerID, f.request.CapitalBucketID).Scan(&until); err != nil || !until.Equal(aiExpiry) {
		t.Fatal("current authority outlived AI connection", until, aiExpiry, err)
	}
}
