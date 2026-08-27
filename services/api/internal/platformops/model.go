// Package platformops owns credential-free, aggregate production operations
// evidence for current superadmins. It has no provider, order, or mutation
// dependency.
package platformops

import "time"

type OperationalStatus string

const (
	StatusNominal   OperationalStatus = "NOMINAL"
	StatusAttention OperationalStatus = "ATTENTION"
	StatusStopped   OperationalStatus = "STOPPED"
)

type SignalState string

const (
	SignalPass      SignalState = "PASS"
	SignalAttention SignalState = "ATTENTION"
	SignalStopped   SignalState = "STOPPED"
)

type Signal struct {
	Code    string      `json:"code"`
	State   SignalState `json:"state"`
	Count   int64       `json:"count"`
	Summary string      `json:"summary"`
}

type ExecutionBoundary struct {
	LiveMandates                  int64 `json:"live_mandates"`
	NonShadowAIInstances          int64 `json:"non_shadow_ai_instances"`
	NonShadowAIExecutions         int64 `json:"non_shadow_ai_executions"`
	ExecutableRiskEvaluations     int64 `json:"executable_risk_evaluations"`
	NonExecutingAIProposals       int64 `json:"non_executing_ai_proposals"`
	ReviewedNonExecutingProposals int64 `json:"reviewed_non_executing_proposals"`
}

type Overview struct {
	GeneratedAt                     time.Time         `json:"generated_at"`
	OperationalStatus               OperationalStatus `json:"operational_status"`
	ActiveAIShadowInstances         int64             `json:"active_ai_shadow_instances"`
	UnhealthyAISchedules            int64             `json:"unhealthy_ai_schedules"`
	UnhealthyAIReconciliations      int64             `json:"unhealthy_ai_reconciliations"`
	UnavailableFinancialConnections int64             `json:"unavailable_financial_connections"`
	OpenGlobalBreakers              int64             `json:"open_global_breakers"`
	OpenScopedBreakers              int64             `json:"open_scoped_breakers"`
	ExecutionBoundary               ExecutionBoundary `json:"execution_boundary"`
	Signals                         []Signal          `json:"signals"`
	LiveExecutionAvailable          bool              `json:"live_execution_available"`
	BrokerActionRequested           bool              `json:"broker_action_requested"`
}

type Facts struct {
	ActiveAIShadowInstances         int64
	UnhealthyAISchedules            int64
	UnhealthyAIReconciliations      int64
	UnavailableFinancialConnections int64
	OpenGlobalBreakers              int64
	OpenScopedBreakers              int64
	ExecutionBoundary               ExecutionBoundary
}
