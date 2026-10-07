package execution

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newScheduledProposalInput(t *testing.T, ctx context.Context, pool *pgxpool.Pool, c mandateConsentFixture, r Request) ScheduledProposal {
	t.Helper()
	var anchor, now time.Time
	var model string
	if err := pool.QueryRow(ctx, `SELECT m.effective_from,clock_timestamp(),v.snapshot->>'ai_model_id'
	 FROM automation_mandates m JOIN automation_mandate_versions v ON v.mandate_id=m.id AND v.version_number=$2 WHERE m.id=$1`, c.mandateID, c.consent.MandateVersion).Scan(&anchor, &now, &model); err != nil {
		t.Fatal(err)
	}
	anchor = anchor.UTC()
	if anchor.Nanosecond() != 0 {
		anchor = anchor.Truncate(time.Second).Add(time.Second)
	}
	slot := anchor.Add(now.Sub(anchor) / (30 * time.Minute) * (30 * time.Minute))
	return ScheduledProposal{MandateApprovalID: c.consent.ID, MandateID: c.mandateID, MandateVersion: c.consent.MandateVersion,
		ScheduledFor: slot, SourceMode: "LIVE", AIConnectionID: c.aiConnectionID, AIModelID: model,
		Side: r.Side, BaseSize: r.BaseSize, LimitPrice: r.LimitPrice, FeeAllowanceUSD: r.FeeAllowanceUSD, MaximumDebitUSD: r.MaximumDebitUSD}
}

// A second immediate proposal needs a newly consented immutable version, not a
// second order in the first schedule slot. Account-level attempts and balances
// are deliberately unchanged. A supplied anchor can exercise the real 2m edge.
func renewScheduledMandateFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, c mandateConsentFixture, anchor time.Time) mandateConsentFixture {
	t.Helper()
	var from *time.Time
	if !anchor.IsZero() {
		v := anchor.UTC().Truncate(time.Second)
		from = &v
	}
	var version int
	if err := pool.QueryRow(ctx, `UPDATE automation_mandates SET current_version=current_version+1,updated_at=clock_timestamp(),
	 effective_from=COALESCE($2::timestamptz,date_trunc('second',clock_timestamp())) WHERE id=$1 RETURNING current_version`, c.mandateID, from).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot)
	 SELECT m.id,m.current_version,m.user_id,'SYSTEM',(to_jsonb(m)-ARRAY['id','user_id','current_version','created_at','updated_at'])||'{"execution_capable":false}'::jsonb FROM automation_mandates m WHERE id=$1
	 RETURNING encode(sha256(convert_to(snapshot::text,'UTF8')),'hex')`, c.mandateID).Scan(&c.snapshotDigest); err != nil {
		t.Fatal(err)
	}
	var err error
	c.consent, err = NewPostgresStore(pool).ApproveMandate(ctx, c.principal, c.request.CapitalBucketID, c.mandateID, version, c.snapshotDigest, "synthetic", mandateConsentStepUp(pool))
	if err != nil {
		t.Fatal(err)
	}
	c.request.MandateApprovalID = c.consent.ID
	return c
}

func scheduledSourceDeadline(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderID string) time.Time {
	t.Helper()
	var until time.Time
	if err := pool.QueryRow(ctx, `SELECT expires_at FROM execution_scheduled_proposals WHERE order_id=$1`, orderID).Scan(&until); err != nil {
		t.Fatal(err)
	}
	return until
}

func TestPostgresScheduledProposalConcurrentSlotAndRestart(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c := newMandateConsentFixture(t, ctx, pool)
	p := newScheduledProposalInput(t, ctx, pool, c, c.request)
	type result struct {
		order Order
		err   error
	}
	results := make(chan result, 6)
	var wait sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			o, err := NewPostgresStore(pool).PrepareScheduledProposal(ctx, c.request.OwnerID, p)
			results <- result{o, err}
		}()
	}
	wait.Wait()
	close(results)
	var first Order
	for r := range results {
		if r.err != nil {
			t.Fatal("concurrent exact intake failed", r.err)
		}
		if first.ID == "" {
			first = r.order
		} else if !reflect.DeepEqual(first, r.order) {
			t.Fatal("one slot produced different immutable orders", first, r.order)
		}
	}
	var orders, sources int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_orders WHERE financial_account_id=$1),(SELECT count(*) FROM execution_scheduled_proposals WHERE financial_account_id=$1)`, c.request.AccountID).Scan(&orders, &sources); err != nil || orders != 1 || sources != 1 {
		t.Fatal("slot was not once-only", orders, sources, err)
	}
	replay, err := NewPostgresStore(pool).PrepareScheduledProposal(ctx, c.request.OwnerID, p)
	if err != nil || !reflect.DeepEqual(replay, first) || first.Request.ClientOrderID != scheduledProposalClientID(c.request.CapitalBucketID, c.mandateID, c.consent.MandateVersion, p.ScheduledFor) {
		t.Fatal("restart changed durable client/order identity", replay, err)
	}
	var sqlClient string
	if err := pool.QueryRow(ctx, `SELECT execution_scheduled_client_id($1,$2,$3,$4)::text`, c.request.CapitalBucketID, c.mandateID, c.consent.MandateVersion, p.ScheduledFor).Scan(&sqlClient); err != nil || sqlClient != first.Request.ClientOrderID {
		t.Fatal("SQL and Go source identity disagree", sqlClient, first.Request.ClientOrderID, err)
	}
	for _, query := range []string{
		`UPDATE execution_scheduled_proposals SET expires_at=expires_at+interval '1 second' WHERE order_id=$1`,
		`DELETE FROM execution_scheduled_proposals WHERE order_id=$1`,
		`UPDATE execution_orders SET scheduled_proposal_required=false WHERE id=$1`,
	} {
		if _, err := pool.Exec(ctx, query, first.ID); err == nil {
			t.Fatal("immutable scheduled binding could be rewritten", query)
		}
	}
	if _, err := pool.Exec(ctx, `TRUNCATE execution_scheduled_proposals`); err == nil {
		t.Fatal("immutable sources could be truncated")
	}
}

func TestPostgresScheduledProposalExactReplayRejectsChangedEconomicsAndSource(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c := newMandateConsentFixture(t, ctx, pool)
	p := newScheduledProposalInput(t, ctx, pool, c, c.request)
	s := NewPostgresStore(pool)
	first, err := s.PrepareScheduledProposal(ctx, c.request.OwnerID, p)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ScheduledProposal){
		"terms":            func(p *ScheduledProposal) { p.LimitPrice = "59999" },
		"decimal spelling": func(p *ScheduledProposal) { p.LimitPrice = "60000.0" },
		"model":            func(p *ScheduledProposal) { p.AIModelID = "another-model" },
		"AI connection":    func(p *ScheduledProposal) { p.AIConnectionID = c.request.ConnectionID },
		"mode":             func(p *ScheduledProposal) { p.SourceMode = "SHADOW" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := p
			mutate(&changed)
			if _, err := s.PrepareScheduledProposal(ctx, c.request.OwnerID, changed); !errors.Is(err, ErrConflict) {
				t.Fatal("changed same-slot proposal did not conflict", err)
			}
		})
	}
	secondConsent, err := s.ApproveMandate(ctx, c.principal, c.request.CapitalBucketID, c.mandateID, c.consent.MandateVersion, c.snapshotDigest, "synthetic", mandateConsentStepUp(pool))
	if err != nil {
		t.Fatal(err)
	}
	changed := p
	changed.MandateApprovalID = secondConsent.ID
	if _, err := s.PrepareScheduledProposal(ctx, c.request.OwnerID, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("new consent replaced an existing scheduled proposal", err)
	}
	got, err := s.PrepareScheduledProposal(ctx, c.request.OwnerID, p)
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Fatal("conflict changed original receipt", got, err)
	}
}

func TestPostgresScheduledProposalLostPrepareCommitRecoversWithoutNewIdentity(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c := newMandateConsentFixture(t, ctx, pool)
	p := newScheduledProposalInput(t, ctx, pool, c, c.request)
	if _, err := NewPostgresStore(lostCommitDB{pool}).PrepareScheduledProposal(ctx, c.request.OwnerID, p); !errors.Is(err, ErrCommitUnknown) {
		t.Fatal("ambiguous prepare commit treated as certain", err)
	}
	s := NewPostgresStore(pool)
	first, err := s.PrepareScheduledProposal(ctx, c.request.OwnerID, p)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.PrepareScheduledProposal(ctx, c.request.OwnerID, p)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("lost prepare commit created replacement identity", second, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_orders WHERE financial_account_id=$1`, c.request.AccountID).Scan(&count); err != nil || count != 1 {
		t.Fatal("lost prepare duplicated order", count, err)
	}
}

func TestPostgresScheduledProposalRejectsInvalidFreshSources(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for name, mutate := range map[string]func(*ScheduledProposal){
		"wrong model":         func(p *ScheduledProposal) { p.AIModelID = "not-pinned" },
		"wrong AI connection": func(p *ScheduledProposal) { p.AIConnectionID = "11111111-1111-4111-8111-111111111111" },
		"Paper":               func(p *ScheduledProposal) { p.SourceMode = "PAPER" },
		"Shadow":              func(p *ScheduledProposal) { p.SourceMode = "SHADOW" },
		"off grid":            func(p *ScheduledProposal) { p.ScheduledFor = p.ScheduledFor.Add(time.Second) },
		"future slot":         func(p *ScheduledProposal) { p.ScheduledFor = p.ScheduledFor.Add(30 * time.Minute) },
		"backfill":            func(p *ScheduledProposal) { p.ScheduledFor = p.ScheduledFor.Add(-30 * time.Minute) },
		"subsecond":           func(p *ScheduledProposal) { p.ScheduledFor = p.ScheduledFor.Add(time.Nanosecond) },
		"version":             func(p *ScheduledProposal) { p.MandateVersion++ },
	} {
		t.Run(name, func(t *testing.T) {
			c := newMandateConsentFixture(t, ctx, pool)
			p := newScheduledProposalInput(t, ctx, pool, c, c.request)
			mutate(&p)
			if _, err := NewPostgresStore(pool).PrepareScheduledProposal(ctx, c.request.OwnerID, p); err == nil {
				t.Fatal("invalid fresh source admitted")
			}
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_orders WHERE financial_account_id=$1`, c.request.AccountID).Scan(&count); err != nil || count != 0 {
				t.Fatal("rejected source persisted order", count, err)
			}
		})
	}
	c := newMandateConsentFixture(t, ctx, pool)
	other := newExecutionFixture(t, ctx, pool)
	p := newScheduledProposalInput(t, ctx, pool, c, c.request)
	if _, err := NewPostgresStore(pool).PrepareScheduledProposal(ctx, other.OwnerID, p); err == nil {
		t.Fatal("foreign owner admitted source")
	}
}

func TestPostgresScheduledProposalExpiredReplayIsReadOnlyAndCannotSend(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c := newMandateConsentFixture(t, ctx, pool)
	var now time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	c = renewScheduledMandateFixture(t, ctx, pool, c, now.Add(-117*time.Second))
	p := newScheduledProposalInput(t, ctx, pool, c, c.request)
	f := newMandateSendOrder(t, ctx, pool, c, "BUY")
	until := scheduledSourceDeadline(t, ctx, pool, f.order.ID)
	waitPastPilotDeadline(t, ctx, pool, until)
	s := NewPostgresStore(pool)
	replay, err := s.PrepareScheduledProposal(ctx, c.request.OwnerID, p)
	if err != nil || !reflect.DeepEqual(replay, f.order) {
		t.Fatal("expired exact source not recoverable", replay, err)
	}
	called := false
	if _, err := s.SendAutonomous(ctx, c.request.OwnerID, replay.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		called = true
		return f.ack, nil
	})); err == nil || called {
		t.Fatal("expired source reached sender", err, called)
	}
	if _, err := s.ReadAttempt(ctx, c.request.OwnerID, replay.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired source persisted attempt", err)
	}
}

func TestPostgresScheduledProposalExpiryAfterClaimClosesNoSend(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c := newMandateConsentFixture(t, ctx, pool)
	var now time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	c = renewScheduledMandateFixture(t, ctx, pool, c, now.Add(-117*time.Second))
	f := newMandateSendOrder(t, ctx, pool, c, "BUY")
	until := scheduledSourceDeadline(t, ctx, pool, f.order.ID)
	db := &sendBoundaryDB{Database: pool, beforeBegin: func(callCtx context.Context, n int) error {
		if n == 3 {
			waitPastPilotDeadline(t, callCtx, pool, until)
		}
		return nil
	}}
	called := false
	_, err := NewPostgresStore(db).SendAutonomous(ctx, c.request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		called = true
		return f.ack, nil
	}))
	if !errors.Is(err, ErrNotAuthorized) || !errors.Is(err, ErrSubmissionNotSent) || called {
		t.Fatal("expired claimed source reached callback", err, called)
	}
	assertNoSendClosed(t, ctx, pool, f)
}

func TestPostgresScheduledProposalBindingIsMandatory(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c := newMandateConsentFixture(t, ctx, pool)
	if _, err := NewPostgresStore(pool).Prepare(ctx, c.request); err == nil {
		t.Fatal("autonomous order persisted without durable scheduled source")
	}
	digest, err := requestDigest(c.request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(c.request)
	if err != nil {
		t.Fatal(err)
	}
	// Even an explicit false marker from a SQL caller is overwritten by the
	// server guard; the deferred counterpart check must abort this transaction.
	if _, err := pool.Exec(ctx, `INSERT INTO execution_orders(owner_id,financial_account_id,provider_connection_id,capital_bucket_id,client_order_id,request_digest,request,scheduled_proposal_required)
	 VALUES($1,$2,$3,$4,$5,$6,$7,false)`, c.request.OwnerID, c.request.AccountID, c.request.ConnectionID, c.request.CapitalBucketID, c.request.ClientOrderID, digest, body); err == nil {
		t.Fatal("direct SQL autonomous order committed without its source")
	}
	var orders int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_orders WHERE financial_account_id=$1`, c.request.AccountID).Scan(&orders); err != nil || orders != 0 {
		t.Fatal("unbound autonomous order escaped transaction", orders, err)
	}
}

func TestPostgresScheduledProposalRequiresEnabledImmutableCadence(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for name, schedule := range map[string]string{
		"disabled":                  `{"enabled":false,"interval_minutes":30,"session":"CONTINUOUS"}`,
		"unsupported short cadence": `{"enabled":true,"interval_minutes":29,"session":"CONTINUOUS"}`,
		"unsupported session":       `{"enabled":true,"interval_minutes":30,"session":"REGULAR"}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := newMandateConsentFixture(t, ctx, pool)
			if _, err := pool.Exec(ctx, `UPDATE automation_mandates SET schedule_conditions=$2 WHERE id=$1`, c.mandateID, schedule); err != nil {
				t.Fatal(err)
			}
			c = renewScheduledMandateFixture(t, ctx, pool, c, time.Time{})
			p := newScheduledProposalInput(t, ctx, pool, c, c.request)
			if _, err := NewPostgresStore(pool).PrepareScheduledProposal(ctx, c.request.OwnerID, p); err == nil {
				t.Fatal("unsupported current schedule admitted source")
			}
		})
	}
	c := newMandateConsentFixture(t, ctx, pool)
	p := newScheduledProposalInput(t, ctx, pool, c, c.request)
	if _, err := pool.Exec(ctx, `UPDATE automation_mandates SET schedule_conditions='{"enabled":true,"interval_minutes":60,"session":"CONTINUOUS"}' WHERE id=$1`, c.mandateID); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPostgresStore(pool).PrepareScheduledProposal(ctx, c.request.OwnerID, p); err == nil {
		t.Fatal("mutable schedule drift bypassed immutable version")
	}
}
