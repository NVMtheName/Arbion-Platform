package strategy

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// completeEvaluationInstance binds every final save to the stored engine, not
// just the supplied ID and runtime version. Keep this at the existing final
// update point: an early instance lock would invert the mandate/bucket/instance
// order used by accepted AI commits. The update rechecks all predicates after
// any row-lock wait and holds the row lock until the surrounding transaction
// commits. A mismatch rolls back the event claim and all provisional evidence.
func completeEvaluationInstance(ctx context.Context, tx pgx.Tx, instance Instance, expectedVersion int, resultingState State, stateChanged bool, evaluatedAt time.Time) (int, error) {
	// Preserve the non-transition SET shape: state_version belongs to a unique
	// key, and concurrent event claims already hold foreign-key KEY SHARE locks.
	// Only real transitions should target state_version/current_state.
	set := `last_evaluated_at=$5,updated_at=$5`
	args := []any{instance.ID, instance.UserID, expectedVersion, instance.CurrentState,
		evaluatedAt, instance.StrategyIdentifier, instance.DefinitionVersion, instance.ExecutionMode,
		instance.AutomationMandateID, instance.MandateVersion, instance.FinancialAccountID, instance.CapitalBucketID}
	if stateChanged {
		set += `,current_state=$13,state_version=state_version+1`
		args = append(args, resultingState)
	}
	var version int
	err := tx.QueryRow(ctx, `UPDATE strategy_instances SET `+set+`
		WHERE id=$1 AND user_id=$2 AND state_version=$3 AND current_state=$4 AND status='ACTIVE'
		AND strategy_identifier=$6 AND strategy_definition_version=$7 AND execution_mode=$8
		AND automation_mandate_id=$9 AND mandate_version=$10 AND financial_account_id=$11
		AND capital_bucket_id=NULLIF($12,'')::uuid
		RETURNING state_version`, args...).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrConflict
	}
	return version, err
}
