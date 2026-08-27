package platformops

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

var ErrSuperadminRequired = errors.New("current active superadmin required")

type Store interface {
	ActiveSuperadmin(context.Context, string) (bool, error)
	Snapshot(context.Context, time.Time) (Facts, error)
}

type Auditor interface {
	Record(context.Context, *string, string, map[string]any) error
}

type Service struct {
	store Store
	audit Auditor
	now   func() time.Time
}

func NewService(store Store, auditors ...Auditor) *Service {
	service := &Service{store: store, now: func() time.Time { return time.Now().UTC() }}
	if len(auditors) > 0 {
		service.audit = auditors[0]
	}
	return service
}

func (service *Service) Overview(ctx context.Context, principal authorization.Principal) (Overview, error) {
	if authorization.RequireSuperadmin(principal) != nil {
		return Overview{}, ErrSuperadminRequired
	}
	active, err := service.store.ActiveSuperadmin(ctx, principal.UserID)
	if err != nil {
		return Overview{}, err
	}
	if !active {
		return Overview{}, ErrSuperadminRequired
	}
	generatedAt := service.now().UTC()
	facts, err := service.store.Snapshot(ctx, generatedAt)
	if err != nil {
		return Overview{}, err
	}
	boundaryViolations := facts.ExecutionBoundary.LiveMandates +
		facts.ExecutionBoundary.NonShadowAIInstances +
		facts.ExecutionBoundary.NonShadowAIExecutions +
		facts.ExecutionBoundary.ExecutableRiskEvaluations
	status := StatusNominal
	if boundaryViolations > 0 || facts.UnhealthyAISchedules > 0 || facts.UnhealthyAIReconciliations > 0 || facts.UnavailableFinancialConnections > 0 || facts.OpenScopedBreakers > 0 {
		status = StatusAttention
	}
	if facts.OpenGlobalBreakers > 0 {
		status = StatusStopped
	}
	overview := Overview{
		GeneratedAt:                     generatedAt,
		OperationalStatus:               status,
		ActiveAIShadowInstances:         facts.ActiveAIShadowInstances,
		UnhealthyAISchedules:            facts.UnhealthyAISchedules,
		UnhealthyAIReconciliations:      facts.UnhealthyAIReconciliations,
		UnavailableFinancialConnections: facts.UnavailableFinancialConnections,
		OpenGlobalBreakers:              facts.OpenGlobalBreakers,
		OpenScopedBreakers:              facts.OpenScopedBreakers,
		ExecutionBoundary:               facts.ExecutionBoundary,
		Signals: []Signal{
			boundarySignal(boundaryViolations),
			countSignal("AI_SCHEDULER", facts.UnhealthyAISchedules, "All active AI Shadow schedules report zero failures.", "One or more active AI Shadow schedules require operator review."),
			countSignal("PORTFOLIO_RECONCILIATION", facts.UnhealthyAIReconciliations, "All active AI Shadow accounts have current, enforced reconciliation evidence.", "One or more active AI Shadow accounts have missing, stale, incomplete, or blocking reconciliation evidence."),
			countSignal("FINANCIAL_CONNECTIONS", facts.UnavailableFinancialConnections, "All financial connections used by active AI Shadow instances are active.", "One or more active AI Shadow instances use an unavailable account or connection."),
			breakerSignal(facts.OpenGlobalBreakers, facts.OpenScopedBreakers),
		},
		LiveExecutionAvailable: false,
		BrokerActionRequested:  false,
	}
	if service.audit != nil {
		userID := principal.UserID
		_ = service.audit.Record(ctx, &userID, "platform.operations_viewed", map[string]any{
			"operational_status":         overview.OperationalStatus,
			"active_ai_shadow_instances": overview.ActiveAIShadowInstances,
			"live_execution_available":   false,
			"broker_action_requested":    false,
		})
	}
	return overview, nil
}

func boundarySignal(count int64) Signal {
	if count == 0 {
		return Signal{Code: "SHADOW_EXECUTION_BOUNDARY", State: SignalPass, Summary: "No live mandate, non-Shadow AI runtime, non-Shadow AI execution, or executable risk record exists."}
	}
	return Signal{Code: "SHADOW_EXECUTION_BOUNDARY", State: SignalAttention, Count: count, Summary: "The Shadow-only execution invariant has incompatible durable records and requires immediate review."}
}

func countSignal(code string, count int64, healthy, attention string) Signal {
	if count == 0 {
		return Signal{Code: code, State: SignalPass, Summary: healthy}
	}
	return Signal{Code: code, State: SignalAttention, Count: count, Summary: attention}
}

func breakerSignal(global, scoped int64) Signal {
	total := global + scoped
	if global > 0 {
		return Signal{Code: "CIRCUIT_BREAKERS", State: SignalStopped, Count: total, Summary: "The platform-wide emergency stop is active; all new risk-gated actions are denied."}
	}
	if scoped > 0 {
		return Signal{Code: "CIRCUIT_BREAKERS", State: SignalAttention, Count: scoped, Summary: fmt.Sprintf("%d scoped emergency stop(s) are active and denying affected new actions.", scoped)}
	}
	return Signal{Code: "CIRCUIT_BREAKERS", State: SignalPass, Summary: "No platform, owner, account, or automation emergency stop is active."}
}
