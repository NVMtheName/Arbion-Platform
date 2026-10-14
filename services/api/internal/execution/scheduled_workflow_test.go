package execution

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/neural"
)

func workflowScope(c ScheduledGenerationClaim) ScheduledScope {
	p := c.Facts.Pilot
	return ScheduledScope{Owner: OwnerScope{OwnerID: p.OwnerID, AccountID: p.AccountID, ConnectionID: p.ConnectionID, CapitalBucketID: p.CapitalBucketID, ProductID: p.ProductID, PilotLimits: p.Limits},
		MandateApprovalID: c.Slot.MandateApprovalID, MandateID: c.Slot.MandateID, MandateVersion: c.Slot.MandateVersion}
}

type workflowRefreshFunc func(context.Context, authorization.Principal, string) error

func (f workflowRefreshFunc) RefreshScheduledExecutionReconciliation(ctx context.Context, p authorization.Principal, account string) error {
	return f(ctx, p, account)
}

func TestScheduledWorkflowScopeBindsEveryImmutableField(t *testing.T) {
	c := generationFixture(time.Now().UTC())
	s := scopedGenerationStore{scope: workflowScope(c)}
	if !s.matches(c.Facts, c.Slot) {
		t.Fatal("exact scope rejected")
	}
	for name, change := range map[string]func(*ScheduledScope){
		"owner":      func(s *ScheduledScope) { s.Owner.OwnerID = c.ID },
		"account":    func(s *ScheduledScope) { s.Owner.AccountID = c.ID },
		"connection": func(s *ScheduledScope) { s.Owner.ConnectionID = c.ID },
		"bucket":     func(s *ScheduledScope) { s.Owner.CapitalBucketID = c.ID },
		"product":    func(s *ScheduledScope) { s.Owner.ProductID = "ETH-USD" },
		"cap":        func(s *ScheduledScope) { s.Owner.PilotLimits.MaximumOrderUSD = "100" },
		"expiry": func(s *ScheduledScope) {
			s.Owner.PilotLimits.ExpiresAt = s.Owner.PilotLimits.ExpiresAt.Add(time.Second)
		},
		"consent": func(s *ScheduledScope) { s.MandateApprovalID = c.ID },
		"mandate": func(s *ScheduledScope) { s.MandateID = c.ID },
		"version": func(s *ScheduledScope) { s.MandateVersion++ },
	} {
		t.Run(name, func(t *testing.T) {
			x := s
			change(&x.scope)
			if x.matches(c.Facts, c.Slot) {
				t.Fatal("changed scope accepted")
			}
		})
	}
}

func TestScheduledWorkflowConstructorAndEntryAreInert(t *testing.T) {
	c := generationFixture(time.Now().UTC())
	scope := workflowScope(c)
	store := NewPostgresStore(ownerNoDatabase{})
	market := generationMarketFunc(func(context.Context, string, string, string) (ScheduledGenerationMarket, error) { panic("market I/O") })
	model := generationDecisionFunc(func(context.Context, authorization.Principal, string, string, neural.LivePilotDecisionRequest) (neural.LivePilotDecision, error) {
		panic("model I/O")
	})
	refresh := workflowRefreshFunc(func(context.Context, authorization.Principal, string) error { panic("reconciliation I/O") })
	w, err := NewScheduledWorkflow(store, scope, ownerNoIODependencies(), market, model, refresh)
	if err != nil || w == nil {
		t.Fatal(err)
	}
	scope.Owner.ProductID = "ETH-USD"
	if w.scope.Owner.ProductID != "BTC-USD" || w.owner.scope.ProductID != "BTC-USD" {
		t.Fatal("scope aliased caller")
	}
	for name, mutate := range map[string]func(*ScheduledScope){
		"empty":   func(s *ScheduledScope) { *s = ScheduledScope{} },
		"consent": func(s *ScheduledScope) { s.MandateApprovalID = "" },
		"mandate": func(s *ScheduledScope) { s.MandateID = "" },
		"version": func(s *ScheduledScope) { s.MandateVersion = 0 },
		"pilot":   func(s *ScheduledScope) { s.Owner.PilotLimits.MaximumOrderUSD = "" },
	} {
		t.Run(name, func(t *testing.T) {
			s := workflowScope(c)
			mutate(&s)
			if got, e := NewScheduledWorkflow(store, s, ownerNoIODependencies(), market, model, refresh); e == nil || got != nil {
				t.Fatal("invalid scope accepted")
			}
		})
	}
	for _, test := range []struct {
		store   *PostgresStore
		market  ScheduledGenerationMarketCollector
		model   ScheduledDecisionGenerator
		refresh ScheduledReconciliationRefresher
	}{
		{nil, market, model, refresh}, {NewPostgresStore(nil), market, model, refresh}, {store, nil, model, refresh}, {store, market, nil, refresh}, {store, market, model, nil},
	} {
		if got, e := NewScheduledWorkflow(test.store, workflowScope(c), ownerNoIODependencies(), test.market, test.model, test.refresh); e == nil || got != nil {
			t.Fatal("missing dependency accepted")
		}
	}
	for _, p := range []authorization.Principal{{}, {UserID: c.ID, Entitlement: authorization.EntitlementFounder}, {UserID: c.Facts.Pilot.OwnerID}} {
		if _, o, e := w.RunSlot(context.Background(), p, c.Slot.ScheduledFor); !errors.Is(e, ErrNotAuthorized) || o != nil {
			t.Fatal("untrusted entry admitted", e)
		}
	}
	p := authorization.Principal{UserID: c.Facts.Pilot.OwnerID, Entitlement: authorization.EntitlementFounder}
	if _, _, e := w.RunSlot(context.Background(), p, c.Slot.ScheduledFor.Add(time.Nanosecond)); !errors.Is(e, ErrNotAuthorized) {
		t.Fatal(e)
	}
	if _, _, e := (*ScheduledWorkflow)(nil).RunSlot(context.Background(), p, c.Slot.ScheduledFor); !errors.Is(e, ErrNotAuthorized) {
		t.Fatal(e)
	}
	for _, run := range []func(context.Context, authorization.Principal, string) (OwnerOrder, error){(*ScheduledWorkflow)(nil).Get, (*ScheduledWorkflow)(nil).Recover, (*ScheduledWorkflow)(nil).Reconcile, (*ScheduledWorkflow)(nil).Cancel, (*ScheduledWorkflow)(nil).Settle} {
		if _, e := run(context.Background(), p, c.ID); !errors.Is(e, ErrNotAuthorized) {
			t.Fatal("nil lifecycle admitted", e)
		}
	}
}
