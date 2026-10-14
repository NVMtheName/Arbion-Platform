package execution

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/neural"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests exercise real PostgreSQL admission and existing control-plane
// transitions. Market, model, credentials and every broker adapter are synthetic.
// They do not establish provider compatibility or trading profitability.
func scheduledWorkerForFixture(t *testing.T, f *scheduledWorkflowFixture, pool *pgxpool.Pool) *ScheduledWorker {
	t.Helper()
	w, err := NewScheduledWorker(f.workflow(t, pool))
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func assertScheduledWorkerRuns(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_scheduled_runs WHERE owner_id=$1`, owner).Scan(&got); err != nil || got != want {
		t.Fatal("unexpected durable worker admissions", got, want, err)
	}
}

func TestPostgresScheduledWorkerRestartReconcilesAndSettlesWithoutResend(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "PROPOSE")
	var observations, settlements atomic.Int32
	observe, settle := f.deps.Observation, f.deps.Settlement
	f.deps.Observation = postgresObservationFunc(func(c context.Context, cr *financial.Credentials, s ConfirmedSubmission, a Attempt) (BrokerObservation, error) {
		observations.Add(1)
		return observe.CollectExecutionObservation(c, cr, s, a)
	})
	f.deps.Settlement = settlementProviderFunc(func(c context.Context, cr *financial.Credentials, s ConfirmedSubmission, a Attempt) (AccountSettlementEvidence, error) {
		settlements.Add(1)
		return settle.CollectAccountSettlement(c, cr, s, a)
	})
	first, err := scheduledWorkerForFixture(t, f, pool).RunOnce(ctx)
	if err != nil || first.Status != "RUN" || !first.ScheduledFor.Equal(f.slot.ScheduledFor) || first.Generation == nil || first.Generation.Result == nil || first.Order == nil || first.Order.State != "BROKER_ACKNOWLEDGED" || !first.Order.AccountHeld || !first.Order.CapitalHeld {
		t.Fatal("worker did not complete exactly the admitted slot", first, err)
	}
	if observations.Load() != 0 || settlements.Load() != 0 {
		t.Fatal("one tick advanced more than its generation and submission chain")
	}
	assertPilotAllocationBalance(t, ctx, pool, f.c.request, "100", "0", 0, true)

	// Reconstruct every worker: only durable PostgreSQL state guides recovery.
	filled, err := scheduledWorkerForFixture(t, f, pool).RunOnce(ctx)
	if err != nil || filled.Status != "RECONCILE" || filled.Order == nil || filled.Order.ID != first.Order.ID || filled.Order.State != "AWAITING_ACCOUNT_SETTLEMENT" || filled.Order.AccountHeld || !filled.Order.CapitalHeld || filled.Order.FillCount != 1 || observations.Load() != 1 || settlements.Load() != 0 {
		t.Fatal("restart did not take exactly one fill/status step", filled, err)
	}
	settled, err := scheduledWorkerForFixture(t, f, pool).RunOnce(ctx)
	if err != nil || settled.Status != "SETTLE" || settled.Order == nil || settled.Order.ID != first.Order.ID || settled.Order.State != "SETTLED" || settled.Order.AccountHeld || settled.Order.CapitalHeld || settlements.Load() != 1 {
		t.Fatal("restart did not settle the original order", settled, err)
	}
	assertPilotAllocationBalance(t, ctx, pool, f.c.request, "93.94", "0.0001", 1, false)
	markets := f.markets.Load()
	for range 2 {
		replayed, err := scheduledWorkerForFixture(t, f, pool).RunOnce(ctx)
		if err != nil || replayed.Status != "SLOT_RECORDED" {
			t.Fatal("settled slot was not recognized after restart", replayed, err)
		}
	}
	if f.models.Load() != 1 || f.markets.Load() != markets || f.refreshes.Load() != 1 || f.previews.Load() != 1 || f.sends.Load() != 1 || observations.Load() != 1 || settlements.Load() != 1 {
		t.Fatal("restart rerolled generation, send or completed lifecycle work")
	}
	assertScheduledWorkerRuns(t, ctx, pool, f.c.request.OwnerID, 1)
	assertScheduledWorkflowCounts(t, ctx, pool, f.c.request.OwnerID, 1, 1, 1, 1, 1)
}

func TestPostgresScheduledWorkerRecoversUnknownAfterConsentRevocation(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "PROPOSE")
	f.deps.Sender = sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		f.sends.Add(1)
		return SubmissionAcknowledgement{}, errors.New("synthetic lost response")
	})
	first, err := scheduledWorkerForFixture(t, f, pool).RunOnce(ctx)
	if !errors.Is(err, ErrSubmissionUnknown) || first.Order != nil {
		t.Fatal("unknown submission was presented as successful", first, err)
	}
	if err = NewPostgresStore(pool).RevokeMandateConsent(ctx, f.c.request.OwnerID, f.c.consent.ID); err != nil {
		t.Fatal(err)
	}
	recovered, err := scheduledWorkerForFixture(t, f, pool).RunOnce(ctx)
	if err != nil || recovered.Status != "RECOVERY" || recovered.Order == nil || recovered.Order.State != "BROKER_ACKNOWLEDGED" || !recovered.Order.AccountHeld || !recovered.Order.CapitalHeld || f.lookups.Load() != 1 {
		t.Fatal("revoked consent stranded an existing unknown attempt", recovered, err)
	}
	filled, err := scheduledWorkerForFixture(t, f, pool).RunOnce(ctx)
	if err != nil || filled.Status != "RECONCILE" || filled.Order == nil || filled.Order.ID != recovered.Order.ID || filled.Order.State != "AWAITING_ACCOUNT_SETTLEMENT" {
		t.Fatal("revoked-consent lifecycle did not preserve original order", filled, err)
	}
	settled, err := scheduledWorkerForFixture(t, f, pool).RunOnce(ctx)
	if err != nil || settled.Status != "SETTLE" || settled.Order == nil || settled.Order.ID != recovered.Order.ID || settled.Order.State != "SETTLED" {
		t.Fatal("revoked consent stranded capital settlement", settled, err)
	}
	if f.models.Load() != 1 || f.refreshes.Load() != 1 || f.previews.Load() != 1 || f.sends.Load() != 1 || f.lookups.Load() != 1 {
		t.Fatal("recovery renewed consent, regenerated a decision or resent")
	}
	assertScheduledWorkerRuns(t, ctx, pool, f.c.request.OwnerID, 1)
	assertScheduledWorkflowCounts(t, ctx, pool, f.c.request.OwnerID, 1, 1, 1, 1, 1)
	assertPilotAllocationBalance(t, ctx, pool, f.c.request, "93.94", "0.0001", 1, false)
}

func TestPostgresScheduledWorkerConcurrentTicksAdmitOneModelAndSend(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "PROPOSE")
	entered, release := make(chan struct{}), make(chan struct{})
	var enterOnce, releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	model := f.model
	f.model = func(c context.Context, p authorization.Principal, connection, modelID string, input neural.LivePilotDecisionRequest) (neural.LivePilotDecision, error) {
		enterOnce.Do(func() { close(entered) })
		select {
		case <-release:
			return model(c, p, connection, modelID, input)
		case <-c.Done():
			return neural.LivePilotDecision{}, c.Err()
		}
	}
	type result struct {
		receipt ScheduledWorkerReceipt
		err     error
	}
	start, results := make(chan struct{}), make(chan result, 4)
	for range cap(results) {
		w := scheduledWorkerForFixture(t, f, pool)
		go func() {
			<-start
			r, err := w.RunOnce(ctx)
			results <- result{r, err}
		}()
	}
	close(start)
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("admitted worker did not reach synthetic model", ctx.Err())
	}
	// Keep the winning model in flight until every other contender has observed
	// the durable admission. No contender can progress to concurrent lifecycle I/O.
	for range cap(results) - 1 {
		select {
		case r := <-results:
			if r.err != nil || r.receipt.Status != "SLOT_RECORDED" {
				t.Fatal("competing tick did not stop at durable admission", r)
			}
		case <-ctx.Done():
			t.Fatal("competing ticks did not finish independently of model", ctx.Err())
		}
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case r := <-results:
		if r.err != nil || r.receipt.Status != "RUN" || r.receipt.Order == nil || r.receipt.Order.State != "BROKER_ACKNOWLEDGED" {
			t.Fatal("admitted worker failed", r)
		}
	case <-ctx.Done():
		t.Fatal("admitted worker did not complete", ctx.Err())
	}
	if f.models.Load() != 1 || f.markets.Load() != 2 || f.refreshes.Load() != 1 || f.previews.Load() != 1 || f.sends.Load() != 1 || f.lookups.Load() != 0 {
		t.Fatal("competing ticks caused duplicate external work")
	}
	assertScheduledWorkerRuns(t, ctx, pool, f.c.request.OwnerID, 1)
	assertScheduledWorkflowCounts(t, ctx, pool, f.c.request.OwnerID, 1, 1, 1, 1, 0)
}

func TestPostgresScheduledWorkerFailedMarketIsNotRetriedOnRestart(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "PROPOSE")
	f.deps = ownerNoIODependencies()
	want := errors.New("synthetic public market unavailable")
	f.market = func(context.Context, string, string, string) (ScheduledGenerationMarket, error) {
		f.markets.Add(1)
		return ScheduledGenerationMarket{}, want
	}
	if r, err := scheduledWorkerForFixture(t, f, pool).RunOnce(ctx); !errors.Is(err, ErrNotAuthorized) || r.Order != nil {
		t.Fatal("market failure did not stop the admitted slot", r, err)
	}
	for range 2 {
		r, err := scheduledWorkerForFixture(t, f, pool).RunOnce(ctx)
		if err != nil || r.Status != "SLOT_RECORDED" || r.Order != nil {
			t.Fatal("restart retried an admitted failed market collection", r, err)
		}
	}
	if f.markets.Load() != 1 || f.models.Load() != 0 || f.refreshes.Load() != 0 {
		t.Fatal("failed market admission was replayed")
	}
	assertScheduledWorkerRuns(t, ctx, pool, f.c.request.OwnerID, 1)
	assertScheduledWorkflowCounts(t, ctx, pool, f.c.request.OwnerID, 0, 0, 0, 0, 0)
}

func TestPostgresScheduledWorkerFailedPreflightWaitsWithoutRetry(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "PROPOSE")
	f.deps.Preflight = captureProvider(func(context.Context, *financial.Credentials, Order, string) (ProviderPreflight, error) {
		f.previews.Add(1)
		return ProviderPreflight{}, ErrUnreconciled
	})
	if r, err := scheduledWorkerForFixture(t, f, pool).RunOnce(ctx); err == nil || r.Order != nil {
		t.Fatal("preflight failure was presented as an executed order", r, err)
	}
	markets := f.markets.Load()
	for range 2 {
		r, err := scheduledWorkerForFixture(t, f, pool).RunOnce(ctx)
		if err != nil || r.Status != "WAITING" || r.Order == nil || r.Order.State != "PREPARED" || r.Order.Attempted {
			t.Fatal("failed preflight was retried instead of holding preparation", r, err)
		}
	}
	if f.models.Load() != 1 || f.markets.Load() != markets || f.refreshes.Load() != 1 || f.previews.Load() != 1 || f.sends.Load() != 0 {
		t.Fatal("prepared order caused repeated private evidence or a send")
	}
	assertScheduledWorkerRuns(t, ctx, pool, f.c.request.OwnerID, 1)
	assertScheduledWorkflowCounts(t, ctx, pool, f.c.request.OwnerID, 1, 1, 1, 0, 0)
}

func TestPostgresScheduledWorkerLostAdmissionCommitNeverCallsExternalDependencies(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "PROPOSE")
	f.deps = ownerNoIODependencies()
	wf, err := NewScheduledWorkflow(NewPostgresStore(lostCommitDB{pool}), f.scope, f.deps, f.market, f.model, f.refresh)
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewScheduledWorker(wf)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := w.RunOnce(ctx); !errors.Is(err, ErrCommitUnknown) || r.Order != nil {
		t.Fatal("ambiguous admission commit was allowed to continue", r, err)
	}
	assertScheduledWorkerRuns(t, ctx, pool, f.c.request.OwnerID, 1)
	r, err := scheduledWorkerForFixture(t, f, pool).RunOnce(ctx)
	if err != nil || r.Status != "SLOT_RECORDED" || r.Order != nil {
		t.Fatal("restart retried an ambiguously acknowledged admission", r, err)
	}
	if f.markets.Load() != 0 || f.models.Load() != 0 || f.refreshes.Load() != 0 || f.sends.Load() != 0 {
		t.Fatal("unknown admission commit reached an external dependency")
	}
	assertScheduledWorkflowCounts(t, ctx, pool, f.c.request.OwnerID, 0, 0, 0, 0, 0)
}

func TestPostgresScheduledWorkerDoesNotBackfillMissedSlot(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "PROPOSE")
	var now time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	// Consent an immutable version whose current slot is beyond its two-minute
	// dispatch window. A timer tick must not backfill it or jump into the future.
	f.c = renewScheduledMandateFixture(t, ctx, pool, f.c, now.Add(-3*time.Minute))
	f.scope.MandateApprovalID, f.scope.MandateVersion = f.c.consent.ID, f.c.consent.MandateVersion
	f.deps = ownerNoIODependencies()
	for range 2 {
		r, err := scheduledWorkerForFixture(t, f, pool).RunOnce(ctx)
		if err != nil || r.Status != "IDLE" || r.Order != nil || r.Generation != nil {
			t.Fatal("worker backfilled a stale schedule slot", r, err)
		}
	}
	if f.markets.Load() != 0 || f.models.Load() != 0 || f.refreshes.Load() != 0 {
		t.Fatal("missed slot reached external dependencies")
	}
	assertScheduledWorkerRuns(t, ctx, pool, f.c.request.OwnerID, 0)
	assertScheduledWorkflowCounts(t, ctx, pool, f.c.request.OwnerID, 0, 0, 0, 0, 0)
}
