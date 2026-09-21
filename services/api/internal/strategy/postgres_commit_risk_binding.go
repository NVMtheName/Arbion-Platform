package strategy

import (
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

// The Shadow adapter normally derives a would-have result only from ALLOW.
// Persistence independently binds that verdict to this exact prepared cycle;
// it must not repair contradictory mode, authority, identity or time evidence
// by substituting values in the immutable INSERT. This is not a new risk
// evaluation or live authorization. Denied/abstained records are separate paths.
func validAIShadowCommitRisk(instance Instance, evaluation risk.RiskEvaluation, result ExecutionResult, evaluatedAt time.Time) bool {
	return instance.StrategyIdentifier == "ai_shadow" && instance.ExecutionMode == Shadow &&
		result.Status == WouldHaveSubmitted && evaluation.Decision == risk.Allow &&
		!evaluation.ApprovalRequired && !evaluation.PlatformExecutionAvailable &&
		evaluation.Mode == string(Shadow) && evaluation.UserID == instance.UserID &&
		evaluation.AccountID == instance.FinancialAccountID && evaluation.MandateID != nil &&
		*evaluation.MandateID == instance.AutomationMandateID && evaluation.MandateVersion != nil &&
		*evaluation.MandateVersion == instance.MandateVersion && !evaluatedAt.IsZero() &&
		evaluation.Timestamp.Equal(evaluatedAt)
}
