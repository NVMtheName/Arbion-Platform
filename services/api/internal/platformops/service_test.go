package platformops

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

type storeFake struct {
	active bool
	facts  Facts
	err    error
	calls  int
}

type auditorFake struct {
	userID   *string
	action   string
	metadata map[string]any
}

func (audit *auditorFake) Record(_ context.Context, userID *string, action string, metadata map[string]any) error {
	audit.userID, audit.action, audit.metadata = userID, action, metadata
	return nil
}

func (store *storeFake) ActiveSuperadmin(context.Context, string) (bool, error) {
	return store.active, store.err
}

func (store *storeFake) Snapshot(context.Context, time.Time) (Facts, error) {
	store.calls++
	return store.facts, store.err
}

func operationsPrincipal(role authorization.Role) authorization.Principal {
	return authorization.Principal{UserID: "operator", Role: role, Entitlement: authorization.EntitlementFounder}
}

func TestOverviewRequiresCurrentActiveSuperadmin(t *testing.T) {
	store := &storeFake{active: true}
	service := NewService(store)
	if _, err := service.Overview(context.Background(), operationsPrincipal(authorization.RoleAdmin)); !errors.Is(err, ErrSuperadminRequired) {
		t.Fatalf("admin received platform operations evidence: %v", err)
	}
	store.active = false
	if _, err := service.Overview(context.Background(), operationsPrincipal(authorization.RoleSuperadmin)); !errors.Is(err, ErrSuperadminRequired) {
		t.Fatalf("stale superadmin received platform operations evidence: %v", err)
	}
	if store.calls != 0 {
		t.Fatalf("unauthorized request reached aggregate snapshot: %d", store.calls)
	}
}

func TestOverviewReportsNominalCredentialFreeShadowEvidence(t *testing.T) {
	now := time.Date(2026, 8, 27, 23, 0, 0, 0, time.UTC)
	store := &storeFake{active: true, facts: Facts{
		ActiveAIShadowInstances: 2,
		ActiveAIPaperInstances:  1,
		ExecutionBoundary: ExecutionBoundary{
			NonShadowAIInstances:          1,
			NonShadowAIExecutions:         2,
			NonExecutingAIProposals:       3,
			ReviewedNonExecutingProposals: 1,
		},
	}}
	audit := &auditorFake{}
	service := NewService(store, audit)
	service.now = func() time.Time { return now }
	overview, err := service.Overview(context.Background(), operationsPrincipal(authorization.RoleSuperadmin))
	if err != nil {
		t.Fatal(err)
	}
	if overview.OperationalStatus != StatusNominal || overview.ActiveAIShadowInstances != 2 || overview.ActiveAIPaperInstances != 1 || overview.LiveExecutionAvailable || overview.BrokerActionRequested || !overview.GeneratedAt.Equal(now) {
		t.Fatalf("nominal overview changed: %#v", overview)
	}
	if len(overview.Signals) != 5 {
		t.Fatalf("unexpected signal count: %#v", overview.Signals)
	}
	for _, signal := range overview.Signals {
		if signal.State != SignalPass || signal.Count != 0 {
			t.Fatalf("nominal signal was not a zero-count pass: %#v", signal)
		}
	}
	if audit.userID == nil || *audit.userID != "operator" || audit.action != "platform.operations_viewed" || audit.metadata["operational_status"] != StatusNominal || audit.metadata["live_execution_available"] != false {
		t.Fatalf("operations read audit evidence is incomplete: %#v", audit)
	}
}

func TestOverviewPrioritizesGlobalStopAndPreservesBoundaryViolations(t *testing.T) {
	store := &storeFake{active: true, facts: Facts{
		ActiveAIShadowInstances:         2,
		UnhealthyAISchedules:            1,
		UnhealthyAIReconciliations:      2,
		UnavailableFinancialConnections: 1,
		OpenGlobalBreakers:              1,
		OpenScopedBreakers:              2,
		ExecutionBoundary: ExecutionBoundary{
			LiveMandates:              1,
			NonShadowAIInstances:      1,
			NonShadowAIExecutions:     1,
			UnsafeAIInstances:         1,
			UnsafeAIExecutions:        1,
			ExecutableRiskEvaluations: 1,
		},
	}}
	overview, err := NewService(store).Overview(context.Background(), operationsPrincipal(authorization.RoleSuperadmin))
	if err != nil {
		t.Fatal(err)
	}
	if overview.OperationalStatus != StatusStopped || overview.LiveExecutionAvailable || overview.BrokerActionRequested {
		t.Fatalf("global stop did not dominate safely: %#v", overview)
	}
	if overview.Signals[0].Code != "NONLIVE_EXECUTION_BOUNDARY" || overview.Signals[0].State != SignalAttention || overview.Signals[0].Count != 4 {
		t.Fatalf("execution violations were not exact: %#v", overview.Signals[0])
	}
	breaker := overview.Signals[len(overview.Signals)-1]
	if breaker.State != SignalStopped || breaker.Count != 3 {
		t.Fatalf("breaker evidence was not exact: %#v", breaker)
	}
}
