package execution

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/neural"
)

func TestOwnerRuntimeAbsentAndZeroAreDisconnected(t *testing.T) {
	r, err := NewOwnerRuntime(nil, nil, OwnerWorkflowDependencies{}, nil, nil, nil)
	if err != nil || r != nil {
		t.Fatal("absent selection constructed a runtime", err)
	}
	for _, r := range []*OwnerRuntime{r, {}} {
		if r.Commissioning() != nil || r.Owner() != nil || r.Worker() != nil {
			t.Fatal("disabled runtime exposed a capability")
		}
		if receipt, err := r.Worker().RunOnce(context.Background()); err != nil || receipt.Status != "DISABLED" {
			t.Fatal("disabled runtime performed work", err)
		}
	}
}

func TestOwnerRuntimeCopiesExactTermsAndHasNoConstructionIO(t *testing.T) {
	store := NewPostgresStore(ownerNoDatabase{})
	market := generationMarketFunc(func(context.Context, string, string, string) (ScheduledGenerationMarket, error) { panic("market I/O") })
	model := generationDecisionFunc(func(context.Context, authorization.Principal, string, string, neural.LivePilotDecisionRequest) (neural.LivePilotDecision, error) {
		panic("model I/O")
	})
	refresh := workflowRefreshFunc(func(context.Context, authorization.Principal, string) error { panic("refresh I/O") })
	terms := ownerCommissioningTermsFixture() // historical terms still permit construction/recovery
	zone := time.FixedZone("fixture", -4*60*60)
	terms.EffectiveFrom = terms.EffectiveFrom.In(zone)
	terms.Pilot.Limits.ExpiresAt = terms.Pilot.Limits.ExpiresAt.In(zone)
	r, err := NewOwnerRuntime(store, &terms, ownerNoIODependencies(), market, model, refresh)
	if err != nil || r == nil {
		t.Fatal("inert runtime construction failed", err)
	}
	c, o, w := r.Commissioning(), r.Owner(), r.Worker()
	normalized := ownerCommissioningTermsFixture()
	expected := ScheduledScope{Owner: OwnerScope{OwnerID: normalized.Pilot.OwnerID, AccountID: normalized.Pilot.AccountID,
		ConnectionID: normalized.Pilot.ConnectionID, CapitalBucketID: normalized.Pilot.CapitalBucketID,
		ProductID: normalized.Pilot.ProductID, PilotLimits: normalized.Pilot.Limits},
		MandateID: ownerCommissioningMandateID(terms.Pilot.OwnerID, terms.Pilot.AccountID), MandateVersion: 1,
		MandateApprovalID: ownerCommissioningConsentID(terms.Pilot.OwnerID, terms.Pilot.AccountID)}
	if c.terms != normalized || w.commissioning != c || w.workflow.owner != o || w.workflow.scope != expected || o.scope != expected.Owner {
		t.Fatal("runtime did not share the normalized immutable scope and receipt")
	}
	terms = OwnerCommissioningTerms{}
	if c.terms != normalized || w.workflow.scope != expected {
		t.Fatal("caller mutation changed registered authority")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := w.RunOnce(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled worker performed I/O", err)
	}
	for _, test := range []struct {
		store   *PostgresStore
		terms   OwnerCommissioningTerms
		deps    OwnerWorkflowDependencies
		market  ScheduledGenerationMarketCollector
		model   ScheduledDecisionGenerator
		refresh ScheduledReconciliationRefresher
	}{
		{nil, normalized, ownerNoIODependencies(), market, model, refresh},
		{NewPostgresStore(nil), normalized, ownerNoIODependencies(), market, model, refresh},
		{store, OwnerCommissioningTerms{}, ownerNoIODependencies(), market, model, refresh},
		{store, normalized, OwnerWorkflowDependencies{}, market, model, refresh},
		{store, normalized, ownerNoIODependencies(), nil, model, refresh},
		{store, normalized, ownerNoIODependencies(), market, nil, refresh},
		{store, normalized, ownerNoIODependencies(), market, model, nil},
	} {
		if got, err := NewOwnerRuntime(test.store, &test.terms, test.deps, test.market, test.model, test.refresh); err == nil || got != nil {
			t.Fatal("incomplete runtime accepted")
		}
	}
}
