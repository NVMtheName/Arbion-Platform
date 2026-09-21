package risk

import "time"

// CheckAutonomousReconciliation applies the same immutable-evidence policy at
// evaluation and persistence boundaries. The caller decides applicability and
// supplies the latest owner-scoped snapshot and a trusted, nonzero clock. This
// pure check does not fetch, refresh, compare, or acknowledge account evidence.
// A passing check is not authorization to trade or an execution result.
func CheckAutonomousReconciliation(snapshot *ReconciliationSnapshot, accountID string, now time.Time) RiskCheck {
	if snapshot == nil || snapshot.AccountID != accountID || !snapshot.AutonomyEnforcementActive {
		return check(ReconciliationRequired, false, "A current enforced broker reconciliation is required for autonomous proposals.")
	}
	if now.IsZero() || snapshot.ObservedAt.IsZero() || snapshot.ObservedAt.After(now) || now.Sub(snapshot.ObservedAt) > AutonomousReconciliationMaxAge {
		return check(ReconciliationStale, false, "The latest enforced broker reconciliation is stale or invalid.")
	}
	if snapshot.ComparisonStatus == "INCOMPLETE" || snapshot.BalancesStatus != "READY" || snapshot.PositionsStatus != "READY" {
		return check(ReconciliationIncomplete, false, "The latest broker reconciliation has incomplete balance or position coverage.")
	}
	if snapshot.ComparisonStatus == "DRIFT_DETECTED" || snapshot.AutonomySignal == "REVIEW_RECOMMENDED" || snapshot.BlockingChangeCount > 0 {
		return check(ReconciliationDriftDetected, false, "Broker-reported position drift must be confirmed by a later matching snapshot.")
	}
	if snapshot.ComparisonStatus != "MATCHED" || snapshot.AutonomySignal != "CLEAR" || snapshot.BlocksNewActions {
		return check(ReconciliationRequired, false, "Two matching complete broker snapshots are required for autonomous proposals.")
	}
	return check(ReconciliationRequired, true, "The latest enforced broker reconciliation is complete, matched, and current.")
}
