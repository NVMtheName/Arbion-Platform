package strategy

import (
	"context"
	"errors"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5"
)

var ErrCommitReconciliationUnavailable = errors.New("current enforced reconciliation does not permit the prepared Shadow action")

// The access guard already holds the owner-bound account FOR SHARE. The
// reconciliation INSERT trigger takes NO KEY UPDATE on that same row, so no
// new comparison can become visible between this read and transaction commit.
// Read after the other locks and provisional writes; refusal rolls them back.
// A newer eligible comparison is allowed: identity or routine cash changes are
// not themselves drift. Paper uses its isolated ledger and never calls this.
func checkAICommitReconciliation(ctx context.Context, tx pgx.Tx, instance Instance, expectedProvider string) error {
	if instance.ExecutionMode != Shadow || instance.StrategyIdentifier != "ai_shadow" || expectedProvider == "" {
		return ErrInvalid
	}
	var snapshot risk.ReconciliationSnapshot
	var provider string
	err := tx.QueryRow(ctx, `SELECT financial_account_id::text,provider_name,
		comparison_status,balances_status,positions_status,autonomy_signal,
		autonomy_enforcement_active,blocks_new_actions,change_count,blocking_change_count,observed_at
		FROM portfolio_reconciliations WHERE user_id=$1 AND financial_account_id=$2
		ORDER BY observed_at DESC,id DESC LIMIT 1`, instance.UserID, instance.FinancialAccountID).Scan(
		&snapshot.AccountID, &provider, &snapshot.ComparisonStatus, &snapshot.BalancesStatus,
		&snapshot.PositionsStatus, &snapshot.AutonomySignal, &snapshot.AutonomyEnforcementActive,
		&snapshot.BlocksNewActions, &snapshot.ChangeCount, &snapshot.BlockingChangeCount, &snapshot.ObservedAt)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && provider != expectedProvider) {
		return ErrCommitReconciliationUnavailable
	}
	if err != nil {
		return err
	}
	// now() is frozen at transaction start and can incorrectly extend the
	// existing 24-hour policy while waiting on account/runtime/capital locks.
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if risk.CheckAutonomousReconciliation(&snapshot, instance.FinancialAccountID, now).Result != risk.Pass {
		return ErrCommitReconciliationUnavailable
	}
	return nil
}
