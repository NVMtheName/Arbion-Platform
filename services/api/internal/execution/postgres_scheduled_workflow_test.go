package execution

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/neural"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This is a joined production-control-plane test, not a provider integration or
// trading-performance simulation. Every external dependency is synthetic.
type scheduledWorkflowFixture struct {
	c                                                    mandateConsentFixture
	slot                                                 ScheduledGenerationSlot
	scope                                                ScheduledScope
	deps                                                 OwnerWorkflowDependencies
	market                                               generationMarketFunc
	model                                                generationDecisionFunc
	refresh                                              scheduledWorkflowRefreshFunc
	models, markets, refreshes, previews, sends, lookups atomic.Int32
}

type scheduledWorkflowRefreshFunc func(context.Context, authorization.Principal, string) error

func (f scheduledWorkflowRefreshFunc) RefreshScheduledExecutionReconciliation(ctx context.Context, p authorization.Principal, account string) error {
	return f(ctx, p, account)
}

func newScheduledWorkflowFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, outcome string) *scheduledWorkflowFixture {
	t.Helper()
	c, slot, _ := newScheduledGenerationFixture(t, ctx, pool)
	return scheduledWorkflowFixtureFromConsent(t, pool, c, slot, outcome, "gpt-5.6-sol")
}

// Reuse only synthetic external callbacks when joining a different existing
// control-plane entry, without pre-creating its consent, allocation, or orders.
func scheduledWorkflowFixtureFromConsent(t *testing.T, pool *pgxpool.Pool, c mandateConsentFixture, slot ScheduledGenerationSlot, outcome, modelID string) *scheduledWorkflowFixture {
	t.Helper()
	f := &scheduledWorkflowFixture{c: c, slot: slot, deps: ownerNoIODependencies(),
		scope: ScheduledScope{Owner: ownerScopeFor(c.request), MandateApprovalID: c.consent.ID, MandateID: c.mandateID, MandateVersion: c.consent.MandateVersion}}
	// The explicit refresh stage must run AFTER intake and before preview. Reuse
	// the existing fixture shape without fabricating an order or owner approval.
	f.refresh = func(callCtx context.Context, p authorization.Principal, account string) error {
		f.refreshes.Add(1)
		if p.UserID != c.request.OwnerID || account != c.request.AccountID {
			return ErrNotAuthorized
		}
		newSavedPreflightForAuthority(t, callCtx, pool, authorityFixture{order: Order{Request: c.request}})
		return nil
	}
	f.deps.Vault = fixturePreflightVault{generation: c.consent.CredentialGeneration, retrieve: func(_ context.Context, l credential.Locator) ([]byte, error) {
		if l.UserID != c.request.OwnerID || l.ConnectionID != c.request.ConnectionID || l.Class != credential.Financial {
			return nil, ErrNotAuthorized
		}
		return json.Marshal(financial.Credentials{APIKeyName: "synthetic-workflow-key", APIPrivateKey: "synthetic-not-a-real-key", PortfolioID: c.request.AccountID})
	}}
	f.market = func(_ context.Context, owner, account, product string) (ScheduledGenerationMarket, error) {
		f.markets.Add(1)
		if owner != c.request.OwnerID || account != c.request.AccountID || product != c.request.ProductID {
			return ScheduledGenerationMarket{}, ErrNotAuthorized
		}
		m := generationPostgresMarket(time.Now().UTC())
		m.StartedAt = m.CompletedAt // fresh second collection follows model completion
		return m, nil
	}
	f.model = func(callCtx context.Context, p authorization.Principal, connection, model string, in neural.LivePilotDecisionRequest) (neural.LivePilotDecision, error) {
		f.models.Add(1)
		if p.UserID != c.request.OwnerID || connection != c.aiConnectionID || model != modelID || in.AvailableCashUSD != "100" || in.BudgetScope != "live-pilot:"+c.mandateID {
			return neural.LivePilotDecision{}, ErrNotAuthorized
		}
		r, err := NewPostgresStore(pool).ReadScheduledGeneration(callCtx, p.UserID, slot)
		if err != nil {
			return neural.LivePilotDecision{}, err
		}
		return scheduledGenerationTestDecision(r.Claim, outcome), nil
	}
	f.deps.Preflight = captureProvider(func(_ context.Context, cr *financial.Credentials, o Order, portfolio string) (ProviderPreflight, error) {
		f.previews.Add(1)
		if cr.PortfolioID != portfolio || portfolio != c.request.AccountID || o.Request.MandateApprovalID != c.consent.ID || o.Request.BaseSize != "0.0001" || o.Request.MaximumDebitUSD != "6.06" {
			return ProviderPreflight{}, ErrConflict
		}
		p := providerPreflightFixture(o, portfolio, time.Now().UTC())
		p.PreviewBaseSize, p.PreviewQuoteSize, p.PreviewFeeUSD, p.PreviewTotalUSD = "0.0001", "6", "0.06", "6.06"
		return p, nil
	})
	ack := func(s ConfirmedSubmission) SubmissionAcknowledgement {
		return SubmissionAcknowledgement{ProviderOrderID: c.request.ClientOrderID, ClientOrderID: s.Order.Request.ClientOrderID, ProductID: s.Order.Request.ProductID, Side: s.Order.Request.Side}
	}
	f.deps.Sender = sendFunc(func(_ context.Context, _ *financial.Credentials, s ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		f.sends.Add(1)
		if s.Order.Request.MandateApprovalID != c.consent.ID {
			return SubmissionAcknowledgement{}, ErrConflict
		}
		return ack(s), nil
	})
	f.deps.Lookup = lookupFunc(func(_ context.Context, _ *financial.Credentials, s ConfirmedSubmission, _ Attempt) (SubmissionAcknowledgement, error) {
		f.lookups.Add(1)
		return ack(s), nil
	})
	f.deps.Observation = postgresObservationFunc(func(_ context.Context, _ *financial.Credentials, s ConfirmedSubmission, a Attempt) (BrokerObservation, error) {
		return scheduledWorkflowObservation(s, a), nil
	})
	f.deps.Settlement = settlementProviderFunc(func(_ context.Context, _ *financial.Credentials, s ConfirmedSubmission, a Attempt) (AccountSettlementEvidence, error) {
		b := scheduledWorkflowObservation(s, a)
		return AccountSettlementEvidence{PortfolioID: s.PortfolioID, Observation: b, CashUSD: "993.94", AvailableCashUSD: "993.94", TotalBase: "1.0001", AvailableBase: "1.0001", Complete: true, NoOpenOrders: true, StartedAt: b.StartedAt, CompletedAt: b.ObservedAt}, nil
	})
	return f
}

func scheduledWorkflowObservation(s ConfirmedSubmission, a Attempt) BrokerObservation {
	r, now := s.Order.Request, time.Now().UTC()
	id := BrokerIdentity{r.OwnerID, s.Order.ID, r.AccountID, r.ConnectionID, r.ClientOrderID, a.ProviderOrderID, r.ProductID, r.Side}
	return BrokerObservation{BrokerIdentity: id, Status: "FILLED", CompleteFills: true, Totals: Totals{1, "0.0001", "6", "0.06"}, StartedAt: now, ObservedAt: now,
		Fills: []Fill{{BrokerIdentity: id, TradeID: "workflow-" + r.ClientOrderID, BaseQuantity: "0.0001", PriceUSD: "60000", GrossUSD: "6", FeeUSD: "0.06", TradedAt: a.ClaimedAt, ObservedAt: now,
			ProviderEvidence: &FillProviderEvidence{EntryID: "workflow-" + r.ClientOrderID, SequenceAt: a.ClaimedAt, Size: "0.0001", FeeCurrency: "USD", FeeCurrencyBasis: "COINBASE_ADVANCED_QUOTE_ASSET_1_91"}}}}
}

func (f *scheduledWorkflowFixture) workflow(t *testing.T, pool *pgxpool.Pool) *ScheduledWorkflow {
	t.Helper()
	w, err := NewScheduledWorkflow(NewPostgresStore(pool), f.scope, f.deps, f.market, f.model, f.refresh)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func assertScheduledWorkflowCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner string, claims, results, orders, attempts, settlements int) {
	t.Helper()
	var got [6]int
	err := pool.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM execution_generation_claims WHERE owner_id=$1),
	 (SELECT count(*) FROM execution_generation_results WHERE owner_id=$1),
	 (SELECT count(*) FROM execution_orders WHERE owner_id=$1),
	 (SELECT count(*) FROM execution_dispatch_attempts WHERE owner_id=$1),
	 (SELECT count(*) FROM execution_account_settlements WHERE owner_id=$1),
	 (SELECT count(*) FROM execution_owner_approvals WHERE owner_id=$1)`, owner).Scan(&got[0], &got[1], &got[2], &got[3], &got[4], &got[5])
	if err != nil || got != [6]int{claims, results, orders, attempts, settlements, 0} {
		t.Fatal("joined workflow changed durable counts or manufactured owner approval", got, err)
	}
}

func TestPostgresScheduledWorkflowSettlementAndRestart(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "PROPOSE")
	w := f.workflow(t, pool)
	r, o, err := w.RunSlot(ctx, f.c.principal, f.slot.ScheduledFor)
	if err != nil || r.Result == nil || o == nil || o.State != "BROKER_ACKNOWLEDGED" || !o.AccountHeld || !o.CapitalHeld || f.models.Load() != 1 || f.refreshes.Load() != 1 || f.previews.Load() != 1 || f.sends.Load() != 1 {
		t.Fatal("joined generation and autonomous send failed", err, o)
	}
	assertPilotAllocationBalance(t, ctx, pool, f.c.request, "100", "0", 0, true)
	wrongScope := f.scope
	wrongScope.MandateID = f.c.request.ClientOrderID
	denied, err := NewScheduledWorkflow(NewPostgresStore(pool), wrongScope, ownerNoIODependencies(), f.market, f.model, f.refresh)
	if err != nil {
		t.Fatal(err)
	}
	for name, command := range map[string]func(context.Context, authorization.Principal, string) (OwnerOrder, error){
		"get": denied.Get, "recover": denied.Recover, "reconcile": denied.Reconcile, "cancel": denied.Cancel, "settle": denied.Settle,
	} {
		if _, err := command(ctx, f.c.principal, o.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal("same-owner other-mandate lifecycle access admitted", name, err)
		}
	}
	// Revocation prevents new dispatch, not recovery of an already attempted order.
	if err = NewPostgresStore(pool).RevokeMandateConsent(ctx, f.c.request.OwnerID, f.c.consent.ID); err != nil {
		t.Fatal(err)
	}
	w = f.workflow(t, pool)
	terminal, err := w.Reconcile(ctx, f.c.principal, o.ID)
	if err != nil || terminal.State != "AWAITING_ACCOUNT_SETTLEMENT" || terminal.AccountHeld || !terminal.CapitalHeld || terminal.FillCount != 1 {
		t.Fatal("fill reconciliation lost holds or totals", terminal, err)
	}
	settled, err := w.Settle(ctx, f.c.principal, o.ID)
	if err != nil || settled.State != "SETTLED" || settled.AccountHeld || settled.CapitalHeld || settled.Accounting == nil || settled.Accounting.ClosingCashUSD != "993.94" || settled.Accounting.ClosingBase != "1.0001" {
		t.Fatal("exact settlement failed", settled, err)
	}
	assertPilotAllocationBalance(t, ctx, pool, f.c.request, "93.94", "0.0001", 1, false)
	beforeMarkets := f.markets.Load()
	restarted := f.workflow(t, pool)
	replay, saved, err := restarted.RunSlot(ctx, f.c.principal, f.slot.ScheduledFor)
	if err != nil || saved == nil || saved.ID != o.ID || saved.State != "SETTLED" || replay.Claim.ID != r.Claim.ID || f.models.Load() != 1 || f.markets.Load() != beforeMarkets || f.refreshes.Load() != 1 || f.previews.Load() != 1 || f.sends.Load() != 1 {
		t.Fatal("restart reran a completed slot", saved, err)
	}
	if _, err = restarted.Settle(ctx, f.c.principal, o.ID); err != nil {
		t.Fatal("duplicate settlement failed", err)
	}
	assertScheduledWorkflowCounts(t, ctx, pool, f.c.request.OwnerID, 1, 1, 1, 1, 1)
	assertPilotAllocationBalance(t, ctx, pool, f.c.request, "93.94", "0.0001", 1, false)
}

func TestPostgresScheduledWorkflowAbstainNeverTouchesFinancialDependencies(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "ABSTAIN")
	f.deps = ownerNoIODependencies()
	for range 2 {
		r, o, err := f.workflow(t, pool).RunSlot(ctx, f.c.principal, f.slot.ScheduledFor)
		if err != nil || r.Result == nil || r.Result.Outcome != "ABSTAIN" || o != nil {
			t.Fatal("abstention reached order intake", err, o)
		}
	}
	if f.models.Load() != 1 || f.markets.Load() != 1 || f.refreshes.Load() != 0 {
		t.Fatal("abstention was regenerated")
	}
	assertScheduledWorkflowCounts(t, ctx, pool, f.c.request.OwnerID, 1, 1, 0, 0, 0)
}

func TestPostgresScheduledWorkflowScopeAndRevocationDenyBeforeGeneration(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "PROPOSE")
	f.deps = ownerNoIODependencies()
	original := f.scope
	for name, change := range map[string]func(*ScheduledScope){
		"account":    func(s *ScheduledScope) { s.Owner.AccountID = f.c.request.ClientOrderID },
		"connection": func(s *ScheduledScope) { s.Owner.ConnectionID = f.c.request.ClientOrderID },
		"capital":    func(s *ScheduledScope) { s.Owner.CapitalBucketID = f.c.request.ClientOrderID },
		"product":    func(s *ScheduledScope) { s.Owner.ProductID = "ETH-USD" },
		"limits":     func(s *ScheduledScope) { s.Owner.PilotLimits.MaximumOrderUSD = "10" },
		"mandate":    func(s *ScheduledScope) { s.MandateID = f.c.request.ClientOrderID },
	} {
		t.Run(name, func(t *testing.T) {
			f.scope = original
			change(&f.scope)
			_, o, err := f.workflow(t, pool).RunSlot(ctx, f.c.principal, f.slot.ScheduledFor)
			if err == nil || o != nil {
				t.Fatal("changed selection admitted work", err)
			}
		})
	}
	f.scope = original
	if err := NewPostgresStore(pool).RevokeMandateConsent(ctx, f.c.request.OwnerID, f.c.consent.ID); err != nil {
		t.Fatal(err)
	}
	if _, o, err := f.workflow(t, pool).RunSlot(ctx, f.c.principal, f.slot.ScheduledFor); !errors.Is(err, ErrNotAuthorized) || o != nil {
		t.Fatal("revoked consent admitted generation", err)
	}
	if f.models.Load() != 0 || f.markets.Load() != 0 || f.refreshes.Load() != 0 {
		t.Fatal("denied selection reached external dependencies")
	}
	assertScheduledWorkflowCounts(t, ctx, pool, f.c.request.OwnerID, 0, 0, 0, 0, 0)
}

func TestPostgresScheduledWorkflowUnknownSendRecoversWithoutResend(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "PROPOSE")
	f.deps.Sender = sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		f.sends.Add(1)
		return SubmissionAcknowledgement{}, errors.New("synthetic lost broker response")
	})
	r, o, err := f.workflow(t, pool).RunSlot(ctx, f.c.principal, f.slot.ScheduledFor)
	if !errors.Is(err, ErrSubmissionUnknown) || o != nil || r.Result == nil {
		t.Fatal("unknown send was presented as successful", err)
	}
	_, saved, err := f.workflow(t, pool).RunSlot(ctx, f.c.principal, f.slot.ScheduledFor)
	if err != nil || saved == nil || saved.State != "SUBMISSION_UNKNOWN" || !saved.AccountHeld || !saved.CapitalHeld {
		t.Fatal("restart lost held unknown attempt", err, saved)
	}
	if err = NewPostgresStore(pool).RevokeMandateConsent(ctx, f.c.request.OwnerID, f.c.consent.ID); err != nil {
		t.Fatal(err)
	}
	recovered, err := f.workflow(t, pool).Recover(ctx, f.c.principal, saved.ID)
	if err != nil || recovered.State != "BROKER_ACKNOWLEDGED" || recovered.ID != saved.ID || f.lookups.Load() != 1 {
		t.Fatal("lookup recovery lost identity", recovered, err)
	}
	if f.models.Load() != 1 || f.refreshes.Load() != 1 || f.previews.Load() != 1 || f.sends.Load() != 1 {
		t.Fatal("unknown outcome caused a second model, preview, or send")
	}
	assertScheduledWorkflowCounts(t, ctx, pool, f.c.request.OwnerID, 1, 1, 1, 1, 0)
}

func TestPostgresScheduledWorkflowRefreshFailureStopsBeforePreview(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "PROPOSE")
	f.deps = ownerNoIODependencies()
	f.refresh = func(context.Context, authorization.Principal, string) error {
		f.refreshes.Add(1)
		return ErrUnreconciled
	}
	r, o, err := f.workflow(t, pool).RunSlot(ctx, f.c.principal, f.slot.ScheduledFor)
	if !errors.Is(err, ErrUnreconciled) || o != nil || r.Result == nil || r.Result.Outcome != "PROPOSE" || f.models.Load() != 1 || f.refreshes.Load() != 1 {
		t.Fatal("failed refresh passed to preview or lost saved proposal", err, o)
	}
	assertScheduledWorkflowCounts(t, ctx, pool, f.c.request.OwnerID, 1, 1, 1, 0, 0)
}

func TestPostgresScheduledWorkflowCompetingWorkersSendOnlyOnce(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newScheduledWorkflowFixture(t, ctx, pool, "PROPOSE")
	start, results := make(chan struct{}), make(chan error, 4)
	var wg sync.WaitGroup
	for range cap(results) {
		w := f.workflow(t, pool)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, err := w.RunSlot(ctx, f.c.principal, f.slot.ScheduledFor)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		// A committed competing attempt denies send; a newer competing account
		// snapshot may instead invalidate old preflight evidence. Both must fail
		// closed, while the assertions below still require exactly one callback.
		if err != nil && !errors.Is(err, ErrAlreadyAttempted) && !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("unexpected competing-worker failure", err)
		}
	}
	_, o, err := f.workflow(t, pool).RunSlot(ctx, f.c.principal, f.slot.ScheduledFor)
	if err != nil || o == nil || o.State != "BROKER_ACKNOWLEDGED" || f.models.Load() != 1 || f.sends.Load() != 1 {
		t.Fatal("workers rerolled model or send", o, err, f.models.Load(), f.sends.Load())
	}
	assertScheduledWorkflowCounts(t, ctx, pool, f.c.request.OwnerID, 1, 1, 1, 1, 0)
}
