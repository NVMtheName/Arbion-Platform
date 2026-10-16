package execution

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These are joined PostgreSQL control-plane tests. Every market, AI, financial
// credential and broker dependency is synthetic; no external API is contacted.
func newOwnerRuntimeCommissioningFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) ownerCommissioningFixture {
	t.Helper()
	f := newOwnerCommissioningFixture(t, ctx, pool)
	// Commissioning's older fixture names a historical model. Select an existing
	// supported route BEFORE any immutable pilot or consent is prepared.
	f.terms.AIModelID = "gpt-5.6-sol"
	var err error
	f.workflow, err = NewOwnerCommissioning(NewPostgresStore(pool), f.terms, mandateConsentStepUp(pool))
	if err != nil {
		t.Fatal(err)
	}
	f.review, err = f.workflow.Review(ctx, f.principal)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func ownerRuntimeFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f ownerCommissioningFixture) *scheduledWorkflowFixture {
	t.Helper()
	p := f.terms.Pilot
	r := Request{OwnerID: p.OwnerID, AccountID: p.AccountID, ConnectionID: p.ConnectionID, CapitalBucketID: p.CapitalBucketID,
		ProductID: p.ProductID, Side: "BUY", BaseSize: "0.0001", LimitPrice: "60000", FeeAllowanceUSD: "0.06", MaximumDebitUSD: "6.06", PilotLimits: &p.Limits,
		MandateApprovalID: ownerCommissioningConsentID(p.OwnerID, p.AccountID)}
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&r.ClientOrderID); err != nil {
		t.Fatal(err)
	}
	// Expected identities for callback assertions only. Nothing is inserted or
	// approved here; separate commissioning and MFA must create actual authority.
	c := mandateConsentFixture{request: r, mandateID: f.review.MandateID, aiConnectionID: f.terms.AIConnectionID, principal: f.principal,
		consent: MandateConsent{ID: r.MandateApprovalID, MandateVersion: 1}}
	if err := pool.QueryRow(ctx, `SELECT credential_generation FROM provider_connections WHERE id=$1`, r.ConnectionID).Scan(&c.consent.CredentialGeneration); err != nil {
		t.Fatal(err)
	}
	slot := ScheduledGenerationSlot{MandateApprovalID: c.consent.ID, MandateID: c.mandateID, MandateVersion: 1, ScheduledFor: f.terms.EffectiveFrom}
	w := scheduledWorkflowFixtureFromConsent(t, pool, c, slot, "PROPOSE", f.terms.AIModelID)
	w.deps.StepUp = mandateConsentStepUp(pool)
	return w
}

func ownerRuntimeForFixture(t *testing.T, pool *pgxpool.Pool, terms OwnerCommissioningTerms, f *scheduledWorkflowFixture) *OwnerRuntime {
	t.Helper()
	r, err := NewOwnerRuntime(NewPostgresStore(pool), &terms, f.deps, f.market, f.model, f.refresh)
	if err != nil || r == nil || r.Commissioning() == nil || r.Owner() == nil || r.Worker() == nil {
		t.Fatal("complete fixed terms did not compose runtime", err)
	}
	return r
}

func assertOwnerRuntimeNoCallbacks(t *testing.T, f *scheduledWorkflowFixture) {
	t.Helper()
	if f.models.Load() != 0 || f.markets.Load() != 0 || f.refreshes.Load() != 0 || f.previews.Load() != 0 || f.sends.Load() != 0 || f.lookups.Load() != 0 {
		t.Fatal("uncommissioned or conflicting runtime reached an external callback")
	}
}

func TestPostgresOwnerRuntimeRequiresSeparateConsentAndRecoversAcrossRestart(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newOwnerRuntimeCommissioningFixture(t, ctx, pool)
	w := ownerRuntimeFixture(t, ctx, pool, f)
	var stepUps atomic.Int32
	w.deps.StepUp = stepUpFunc(func(c context.Context, owner, code string) (string, time.Time, error) {
		stepUps.Add(1)
		return mandateConsentStepUp(pool)(c, owner, code)
	})
	runtime := func() *OwnerRuntime { return ownerRuntimeForFixture(t, pool, f.terms, w) }
	if _, err := runtime().Worker().RunOnce(ctx); err == nil {
		t.Fatal("configuration inferred commissioning or consent")
	}
	assertOwnerRuntimeNoCallbacks(t, w)
	assertCommissioningCounts(t, ctx, pool, f, 0, 0, 0, 0, 0, 0)
	assertScheduledWorkerRuns(t, ctx, pool, f.principal.UserID, 0)

	prepared, err := runtime().Commissioning().Prepare(ctx, f.principal, f.review.TermsDigest)
	if err != nil || prepared.OwnerCommissioningReview != f.review {
		t.Fatal("runtime did not prepare exact reviewed terms", prepared, err)
	}
	if _, err = runtime().Worker().RunOnce(ctx); err == nil {
		t.Fatal("prepared pilot inferred separate owner consent")
	}
	assertOwnerRuntimeNoCallbacks(t, w)
	assertCommissioningCounts(t, ctx, pool, f, 1, 1, 1, 0, 0, 0)
	assertScheduledWorkerRuns(t, ctx, pool, f.principal.UserID, 0)
	if stepUps.Load() != 0 {
		t.Fatal("construction or preparation consumed MFA")
	}
	consent, err := runtime().Commissioning().Approve(ctx, f.principal, prepared.TermsDigest, prepared.SnapshotDigest, "synthetic")
	if err != nil || consent.ID != w.c.consent.ID || consent.MandateID != w.c.mandateID || consent.MandateVersion != 1 || consent.CredentialGeneration != w.c.consent.CredentialGeneration || stepUps.Load() != 1 {
		t.Fatal("runtime did not bind separate MFA to exact initial consent", consent, err)
	}
	newSavedPreflightForAuthority(t, ctx, pool, authorityFixture{order: Order{Request: w.c.request}})
	w.deps.Sender = sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		w.sends.Add(1)
		return SubmissionAcknowledgement{}, errors.New("synthetic lost broker response")
	})
	first, err := runtime().Worker().RunOnce(ctx)
	if !errors.Is(err, ErrSubmissionUnknown) || first.Generation == nil || first.Generation.Result == nil || first.Order != nil || !first.ScheduledFor.Equal(f.terms.EffectiveFrom) {
		t.Fatal("derived runtime failed to admit exact slot or hid uncertain submission", first, err)
	}
	// Reconstruct after revocation: receipt equality must remain historical and
	// must not strand already-attempted orders behind a current-consent gate.
	if err = runtime().Commissioning().Revoke(ctx, f.principal, consent.ID); err != nil {
		t.Fatal(err)
	}
	var orderID string
	for _, step := range []struct{ action, state string }{{"RECOVERY", "BROKER_ACKNOWLEDGED"}, {"RECONCILE", "AWAITING_ACCOUNT_SETTLEMENT"}, {"SETTLE", "SETTLED"}} {
		got, err := runtime().Worker().RunOnce(ctx)
		if err != nil || got.Status != step.action || got.Order == nil || got.Order.State != step.state {
			t.Fatal("reconstructed runtime lost exact lifecycle dependency", step, got, err)
		}
		if orderID == "" {
			orderID = got.Order.ID
		} else if got.Order.ID != orderID {
			t.Fatal("runtime restart changed order identity")
		}
	}
	if _, err := runtime().Worker().RunOnce(ctx); err == nil {
		t.Fatal("revoked runtime resumed new work after settlement")
	}
	if stepUps.Load() != 1 || w.models.Load() != 1 || w.refreshes.Load() != 1 || w.previews.Load() != 1 || w.sends.Load() != 1 || w.lookups.Load() != 1 {
		t.Fatal("reconstruction renewed consent, regenerated or resent")
	}
	assertScheduledWorkerRuns(t, ctx, pool, f.principal.UserID, 1)
	assertScheduledWorkflowCounts(t, ctx, pool, f.principal.UserID, 1, 1, 1, 1, 1)
	assertPilotAllocationBalance(t, ctx, pool, w.c.request, "93.94", "0.0001", 1, false)
}

func TestPostgresOwnerRuntimeConflictingTermsCannotBorrowSavedAuthority(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newOwnerRuntimeCommissioningFixture(t, ctx, pool)
	w := ownerRuntimeFixture(t, ctx, pool, f)
	r := ownerRuntimeForFixture(t, pool, f.terms, w)
	prepared, err := r.Commissioning().Prepare(ctx, f.principal, f.review.TermsDigest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Commissioning().Approve(ctx, f.principal, prepared.TermsDigest, prepared.SnapshotDigest, "synthetic"); err != nil {
		t.Fatal(err)
	}
	newSavedPreflightForAuthority(t, ctx, pool, authorityFixture{order: Order{Request: w.c.request}})
	for _, name := range []string{"owner", "product", "AI model", "objective"} {
		t.Run(name, func(t *testing.T) {
			changed := f.terms
			changedCommissioningTerms()[name](&changed)
			other := ownerRuntimeForFixture(t, pool, changed, w)
			if _, err := other.Worker().RunOnce(ctx); err == nil {
				t.Fatal("changed configuration borrowed saved commissioning authority")
			}
			assertOwnerRuntimeNoCallbacks(t, w)
			assertScheduledWorkerRuns(t, ctx, pool, f.principal.UserID, 0)
		})
	}
	assertCommissioningCounts(t, ctx, pool, f, 1, 1, 1, 1, 0, 0)
	assertScheduledWorkflowCounts(t, ctx, pool, f.principal.UserID, 0, 0, 0, 0, 0)
}
