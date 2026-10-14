package financialconnection

import (
	"context"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/financial"
)

func executionRefreshFixture(t *testing.T) (*Service, *coinbaseProviderFake, *reconciliationStoreFake) {
	t.Helper()
	p, r := &coinbaseProviderFake{}, &reconciliationStoreFake{}
	s := reconciliationService(t, p, r)
	for i := 0; i < 2; i++ {
		if _, e := s.RunReconciliation(context.Background(), founder(), "account-1"); e != nil {
			t.Fatal(e)
		}
	}
	return s, p, r
}

func TestExecutionReconciliationAlwaysRefreshesMatchedEvidence(t *testing.T) {
	s, p, r := executionRefreshFixture(t)
	before := time.Now().UTC()
	locksBefore := s.store.(*connectionStoreFake).lockCalls
	if err := s.RefreshScheduledExecutionReconciliation(context.Background(), founder(), "account-1"); err != nil {
		t.Fatal(err)
	}
	if len(r.items) != 3 || r.items[2].ID == r.items[1].ID || r.items[2].ObservedAt.Before(before) || !executionReconciliationMatched(r.items[2], time.Now().UTC()) || p.balances != 3 || p.positions != 3 {
		t.Fatal("fresh comparison missing")
	}
	if p.orders != 0 || p.fills != 0 || p.previews != 0 || p.disconnected != 0 {
		t.Fatal("refresh crossed read-only boundary")
	}
	if store := s.store.(*connectionStoreFake); store.lockCalls <= locksBefore || store.lockDepth != 0 {
		t.Fatal("snapshot not serialized")
	}
}

func TestExecutionReconciliationNeverBootstrapsAcknowledgesOrUsesUnsafeHistory(t *testing.T) {
	for name, change := range map[string]func(*PortfolioReconciliation){
		"baseline":     func(r *PortfolioReconciliation) { r.ComparisonStatus = "BASELINE" },
		"drift":        func(r *PortfolioReconciliation) { r.ComparisonStatus = "DRIFT_DETECTED" },
		"incomplete":   func(r *PortfolioReconciliation) { r.ComparisonStatus = "INCOMPLETE" },
		"balances":     func(r *PortfolioReconciliation) { r.BalancesStatus = "UNAVAILABLE" },
		"positions":    func(r *PortfolioReconciliation) { r.PositionsStatus = "UNAVAILABLE" },
		"unenforced":   func(r *PortfolioReconciliation) { r.AutonomyEnforcementActive = false },
		"blocked":      func(r *PortfolioReconciliation) { r.BlocksNewActions = true },
		"changes":      func(r *PortfolioReconciliation) { r.BlockingChangeCount = 1 },
		"signal":       func(r *PortfolioReconciliation) { r.AutonomySignal = "REVIEW_REQUIRED" },
		"future":       func(r *PortfolioReconciliation) { r.ObservedAt = time.Now().Add(time.Hour) },
		"missing time": func(r *PortfolioReconciliation) { r.ObservedAt = time.Time{} },
		"provider":     func(r *PortfolioReconciliation) { r.Provider = "schwab" },
	} {
		t.Run(name, func(t *testing.T) {
			s, p, r := executionRefreshFixture(t)
			change(&r.items[1])
			if err := s.RefreshScheduledExecutionReconciliation(context.Background(), founder(), "account-1"); err == nil || len(r.items) != 2 || p.balances != 2 || p.positions != 2 {
				t.Fatal("unsafe evidence refreshed", err)
			}
		})
	}
	p, r := &coinbaseProviderFake{}, &reconciliationStoreFake{}
	s := reconciliationService(t, p, r)
	if err := s.RefreshScheduledExecutionReconciliation(context.Background(), founder(), "account-1"); err == nil || p.balances != 0 {
		t.Fatal("missing history bootstrapped")
	}
	if err := s.RefreshScheduledExecutionReconciliation(context.Background(), authorization.Principal{}, "account-1"); err == nil {
		t.Fatal("anonymous refresh allowed")
	}
	s.ConfigureReconciliation(nil)
	if err := s.RefreshScheduledExecutionReconciliation(context.Background(), founder(), "account-1"); err == nil {
		t.Fatal("missing storage allowed")
	}
}

func TestExecutionReconciliationPersistsButDeniesFreshDriftAndIncomplete(t *testing.T) {
	for _, mode := range []string{"drift", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			s, p, r := executionRefreshFixture(t)
			if mode == "drift" {
				p.positionData = []financial.Position{{Symbol: "BTC", Quantity: "0.01", Direction: "long", InstrumentType: "CRYPTO"}}
			} else {
				p.positionsErr = &financial.ProviderError{Code: financial.ProviderUnavailable}
			}
			if err := s.RefreshScheduledExecutionReconciliation(context.Background(), founder(), "account-1"); err == nil || len(r.items) != 3 || !r.items[2].BlocksNewActions {
				t.Fatal("unsafe fresh evidence accepted", err)
			}
			if err := s.RefreshScheduledExecutionReconciliation(context.Background(), founder(), "account-1"); err == nil || len(r.items) != 3 || p.balances != 3 || p.positions != 3 {
				t.Fatal("failure automatically retried or acknowledged", err)
			}
		})
	}
}
