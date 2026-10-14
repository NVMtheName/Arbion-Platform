package execution

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPostgresScheduledWorkerStoreConcurrentAdmissionAndRestart(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "ABSTAIN")
	type result struct {
		work scheduledWorkerWork
		err  error
	}
	results := make(chan result, 6)
	var group sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			w, err := NewPostgresStore(pool).claimScheduledWork(ctx, f.scope)
			results <- result{w, err}
		}()
	}
	group.Wait()
	close(results)
	admitted := 0
	for got := range results {
		if got.err != nil || !got.work.ScheduledFor.Equal(f.slot.ScheduledFor) || got.work.OrderID != "" {
			t.Fatal("incorrect admission or DB-selected slot", got.work, got.err)
		}
		if got.work.Admitted {
			admitted++
		}
	}
	if admitted != 1 {
		t.Fatal("concurrent admission was not once-only", admitted)
	}
	w, err := NewPostgresStore(pool).claimScheduledWork(ctx, f.scope)
	if err != nil || w.Admitted || !w.ScheduledFor.Equal(f.slot.ScheduledFor) {
		t.Fatal("restart re-admitted slot", w, err)
	}
	var runs, claims, orders, attempts int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_scheduled_runs WHERE owner_id=$1),
	 (SELECT count(*) FROM execution_generation_claims WHERE owner_id=$1),
	 (SELECT count(*) FROM execution_orders WHERE owner_id=$1),(SELECT count(*) FROM execution_dispatch_attempts WHERE owner_id=$1)`, f.c.request.OwnerID).Scan(&runs, &claims, &orders, &attempts); err != nil || runs != 1 || claims != 0 || orders != 0 || attempts != 0 {
		t.Fatal("admission created financial/model state", runs, claims, orders, attempts, err)
	}
	// Consent renewal is not part of the slot identity and cannot reroll it.
	renewed, err := NewPostgresStore(pool).ApproveMandate(ctx, f.c.principal, f.c.request.CapitalBucketID, f.c.mandateID,
		f.scope.MandateVersion, f.c.snapshotDigest, "synthetic", mandateConsentStepUp(pool))
	if err != nil {
		t.Fatal(err)
	}
	scope := f.scope
	scope.MandateApprovalID = renewed.ID
	w, err = NewPostgresStore(pool).claimScheduledWork(ctx, scope)
	if err != nil || w.Admitted || !w.ScheduledFor.Equal(f.slot.ScheduledFor) {
		t.Fatal("renewed consent re-admitted same slot", w, err)
	}
}

func TestPostgresScheduledWorkerStoreLostCommitNeverAdmits(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "ABSTAIN")
	w, err := NewPostgresStore(lostCommitDB{pool}).claimScheduledWork(ctx, f.scope)
	if !errors.Is(err, ErrCommitUnknown) || w.Admitted || !w.ScheduledFor.Equal(f.slot.ScheduledFor) {
		t.Fatal("ambiguous commit permitted callback", w, err)
	}
	w, err = NewPostgresStore(pool).claimScheduledWork(ctx, f.scope)
	if err != nil || w.Admitted || !w.ScheduledFor.Equal(f.slot.ScheduledFor) {
		t.Fatal("restart retried ambiguous admission", w, err)
	}
}

func TestPostgresScheduledWorkerStoreScopeAndCurrentControlDenials(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "ABSTAIN")
	s := NewPostgresStore(pool)
	if w, err := s.claimScheduledWork(ctx, f.scope); err != nil || !w.Admitted {
		t.Fatal(w, err)
	}
	for name, mutate := range map[string]func(*ScheduledScope){
		"product": func(s *ScheduledScope) { s.Owner.ProductID = "ETH-USD" },
		"limit":   func(s *ScheduledScope) { s.Owner.PilotLimits.MaximumOrderUSD = "24" },
		"expiry": func(s *ScheduledScope) {
			s.Owner.PilotLimits.ExpiresAt = s.Owner.PilotLimits.ExpiresAt.Add(-time.Minute)
		},
		"version": func(s *ScheduledScope) { s.MandateVersion++ },
	} {
		t.Run(name, func(t *testing.T) {
			scope := f.scope
			mutate(&scope)
			if w, err := s.claimScheduledWork(ctx, scope); err == nil || w.Admitted {
				t.Fatal("changed replay scope accepted", w, err)
			}
		})
	}
	if err := s.RevokeMandateConsent(ctx, f.scope.Owner.OwnerID, f.scope.MandateApprovalID); err != nil {
		t.Fatal(err)
	}
	if w, err := s.claimScheduledWork(ctx, f.scope); err == nil || w.Admitted {
		t.Fatal("revoked authority admitted work", w, err)
	}
}

func TestPostgresScheduledWorkerStoreSkipsMissedSlots(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "ABSTAIN")
	var now time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	c := renewScheduledMandateFixture(t, ctx, pool, f.c, now.Add(-5*time.Minute))
	scope := f.scope
	scope.MandateApprovalID, scope.MandateVersion = c.consent.ID, c.consent.MandateVersion
	w, err := NewPostgresStore(pool).claimScheduledWork(ctx, scope)
	if err != nil || w.Admitted || !w.ScheduledFor.IsZero() || w.OrderID != "" {
		t.Fatal("old slot was caught up or regenerated", w, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_scheduled_runs WHERE owner_id=$1`, scope.Owner.OwnerID).Scan(&count); err != nil || count != 0 {
		t.Fatal("missed slot created admission", count, err)
	}
}

func TestPostgresScheduledWorkerStoreImmutableDirectSQL(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "ABSTAIN")
	s := NewPostgresStore(pool)
	if w, err := s.claimScheduledWork(ctx, f.scope); err != nil || !w.Admitted {
		t.Fatal(w, err)
	}
	for _, query := range []string{
		`UPDATE execution_scheduled_runs SET expires_at=expires_at+interval '1 second' WHERE owner_id=$1`,
		`DELETE FROM execution_scheduled_runs WHERE owner_id=$1`,
		`INSERT INTO execution_scheduled_runs(owner_id,financial_account_id,provider_connection_id,capital_bucket_id,mandate_approval_id,mandate_id,mandate_version,scheduled_for,expires_at)
		 SELECT owner_id,financial_account_id,provider_connection_id,capital_bucket_id,mandate_approval_id,mandate_id,mandate_version,scheduled_for-interval '30 minutes',expires_at FROM execution_scheduled_runs WHERE owner_id=$1`,
		`INSERT INTO execution_scheduled_runs(owner_id,financial_account_id,provider_connection_id,capital_bucket_id,mandate_approval_id,mandate_id,mandate_version,scheduled_for,expires_at)
		 SELECT owner_id,financial_account_id,provider_connection_id,capital_bucket_id,mandate_approval_id,mandate_id,mandate_version,scheduled_for+interval '30 minutes',expires_at FROM execution_scheduled_runs WHERE owner_id=$1`,
	} {
		if _, err := pool.Exec(ctx, query, f.scope.Owner.OwnerID); err == nil {
			t.Fatal("direct SQL bypassed immutable/current admission guard", query)
		}
	}
	if _, err := pool.Exec(ctx, `TRUNCATE execution_scheduled_runs`); err == nil {
		t.Fatal("admission evidence could be truncated")
	}
}

func TestPostgresScheduledWorkerStoreExpiredPreparedIsHistoryNotRetry(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "ABSTAIN")
	s := NewPostgresStore(pool)
	p := newScheduledProposalInput(t, ctx, pool, f.c, f.c.request)
	order, err := s.PrepareScheduledProposal(ctx, f.c.request.OwnerID, p)
	if err != nil {
		t.Fatal(err)
	}
	if w, err := s.claimScheduledWork(ctx, f.scope); err != nil || w.OrderID != order.ID || w.Admitted {
		t.Fatal("current prepared source was retried or skipped", w, err)
	}
	// Construct elapsed history inside a rollback-only transaction, retaining
	// the exact source/client/digest/snapshot relationships. No production clock
	// injection or sleep is introduced and no immutable evidence edit commits.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(tx)
	directInsert := func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO execution_scheduled_runs(owner_id,financial_account_id,provider_connection_id,
		 capital_bucket_id,mandate_approval_id,mandate_id,mandate_version,scheduled_for,expires_at)
		 SELECT $1,$2,$3,$4,$5,$6,$7,$8,execution_scheduled_proposal_deadline($5,$1,$4,$6,$7,$8,$9,$10)`,
			f.scope.Owner.OwnerID, f.scope.Owner.AccountID, f.scope.Owner.ConnectionID, f.scope.Owner.CapitalBucketID,
			f.scope.MandateApprovalID, f.scope.MandateID, f.scope.MandateVersion, f.slot.ScheduledFor, f.c.aiConnectionID, "gpt-5.6-sol")
		return err
	}
	checkDirect := func(wantAllowed bool) {
		t.Helper()
		probe, err := tx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = directInsert(probe)
		_ = probe.Rollback(ctx)
		if (err == nil) != wantAllowed {
			t.Fatal("direct SQL history admission differed", wantAllowed, err)
		}
	}
	checkDirect(false)
	for _, table := range []string{"automation_mandates", "automation_mandate_versions", "execution_mandate_approvals", "execution_orders", "execution_scheduled_proposals"} {
		if _, err := tx.Exec(ctx, `ALTER TABLE `+table+` DISABLE TRIGGER USER`); err != nil {
			t.Fatal(err)
		}
	}
	oldSlot := f.slot.ScheduledFor.Add(-30 * time.Minute)
	order.Request.ClientOrderID = scheduledProposalClientID(f.scope.Owner.CapitalBucketID, f.scope.MandateID, f.scope.MandateVersion, oldSlot)
	digest, err := requestDigest(order.Request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(order.Request)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		query string
		args  []any
	}{
		{`UPDATE automation_mandates SET effective_from=$2 WHERE id=$1`, []any{f.scope.MandateID, oldSlot}},
		{`UPDATE automation_mandate_versions SET snapshot=jsonb_set(snapshot,'{effective_from}',to_jsonb($3::timestamptz)) WHERE mandate_id=$1 AND version_number=$2`, []any{f.scope.MandateID, f.scope.MandateVersion, oldSlot}},
		{`UPDATE execution_mandate_approvals a SET snapshot_digest=encode(sha256(convert_to(v.snapshot::text,'UTF8')),'hex') FROM automation_mandate_versions v WHERE a.id=$1 AND v.mandate_id=a.mandate_id AND v.version_number=a.mandate_version`, []any{f.scope.MandateApprovalID}},
		{`UPDATE execution_orders SET client_order_id=$2,request_digest=$3,request=$4,created_at=$5 WHERE id=$1`, []any{order.ID, order.Request.ClientOrderID, digest, body, oldSlot}},
		{`UPDATE execution_scheduled_proposals SET client_order_id=$2,request_digest=$3,scheduled_for=$4,created_at=$4,expires_at=$4::timestamptz+interval '2 minutes' WHERE order_id=$1`, []any{order.ID, order.Request.ClientOrderID, digest, oldSlot}},
	} {
		if _, err := tx.Exec(ctx, change.query, change.args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"automation_mandates", "automation_mandate_versions", "execution_mandate_approvals", "execution_orders", "execution_scheduled_proposals"} {
		if _, err := tx.Exec(ctx, `ALTER TABLE `+table+` ENABLE TRIGGER USER`); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := NewPostgresStore(tx).ReadOwnerOrder(ctx, f.scope.Owner.OwnerID, order.ID); err != nil || got.State != "PREPARED" || got.Attempted || got.AccountHeld || got.CapitalHeld {
		t.Fatal("elapsed source did not retain inert validated terms", got, err)
	}
	checkDirect(true)
	w, err := NewPostgresStore(tx).claimScheduledWork(ctx, f.scope)
	if err != nil || !w.Admitted || w.OrderID != "" || !w.ScheduledFor.Equal(f.slot.ScheduledFor) {
		t.Fatal("safe expired prepared source permanently blocked a later slot", w, err)
	}
	var closures, attempts int
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_account_settlements WHERE order_id=$1)+(SELECT count(*) FROM execution_no_send_resolutions WHERE order_id=$1),
	 (SELECT count(*) FROM execution_dispatch_attempts WHERE order_id=$1)`, order.ID).Scan(&closures, &attempts); err != nil || closures != 0 || attempts != 0 {
		t.Fatal("expired source fabricated a closure or submission", closures, attempts, err)
	}
}
