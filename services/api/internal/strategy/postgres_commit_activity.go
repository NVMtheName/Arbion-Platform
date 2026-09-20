package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5"
)

var (
	ErrCommitActionLimit         = errors.New("daily AI action limit reached at non-live commit")
	ErrCommitActionCooldown      = errors.New("AI repeat-action cooldown active at non-live commit")
	ErrCommitActivityUnavailable = errors.New("current AI action activity unavailable at non-live commit")
)

// Called only after the shared binding guard has locked this AI instance, and
// after all writes/lock waits, immediately before commit. Other accepted writers
// cannot pass that instance lock until this transaction finishes. Denied writers
// also require the same instance row before they can commit. Exclude this
// transaction's own provisional result, which rolls back if the guard refuses it.
// This repeats the existing instance-scoped action-count and one-hour cooldown
// rules; it is not a new risk evaluation or permission for live execution.
func checkAICommitActivity(ctx context.Context, tx pgx.Tx, instance Instance, action risk.ProposedAction, evaluatedAt time.Time) error {
	var now time.Time
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT clock_timestamp(),v.snapshot->'risk_parameters'
		FROM automation_mandate_versions v JOIN automation_mandates m ON m.id=v.mandate_id
		WHERE m.id=$1 AND m.user_id=$2 AND v.version_number=$3`,
		instance.AutomationMandateID, instance.UserID, instance.MandateVersion).Scan(&now, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEvaluationConfigurationChanged
	}
	if err != nil {
		return err
	}
	limit, err := parseAICommitActionLimit(raw)
	if err != nil {
		return err
	}
	if !sameAICommitActivityDay(evaluatedAt, now) {
		// Never charge an old prepared decision to a new UTC daily allowance.
		return ErrCommitActivityUnavailable
	}
	status := "WOULD_HAVE_SUBMITTED"
	if instance.ExecutionMode == Paper {
		status = "SIMULATED_FILLED"
	} else if instance.ExecutionMode != Shadow {
		return ErrInvalid
	}
	dayStart := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	var count int
	var cooldown, invalid bool
	err = tx.QueryRow(ctx, `SELECT
		COUNT(*) FILTER (WHERE created_at >= $4 AND created_at < $5),
		COALESCE(BOOL_OR(mode=$6 AND status=$7 AND upper(symbol)=upper($8) AND side=$9 AND created_at > $10),false),
		COALESCE(BOOL_OR(created_at > $11 OR mode<>$6),false)
		FROM nonlive_execution_records
		WHERE user_id=$1 AND strategy_instance_id=$2 AND proposed_action_id<>$3
		AND created_at >= LEAST($4::timestamptz,$10::timestamptz)`,
		instance.UserID, instance.ID, action.ID, dayStart, dayStart.Add(24*time.Hour),
		instance.ExecutionMode, status, action.Instrument, action.Side, now.Add(-risk.AIRepeatActionCooldown), now).Scan(&count, &cooldown, &invalid)
	if err != nil {
		return err
	}
	if invalid {
		return ErrCommitActivityUnavailable
	}
	// Match EvaluationFacts: all saved dispositions count toward the day's
	// action quota; only accepted results with the same symbol/side cool down.
	if limit != nil && count >= *limit {
		return ErrCommitActionLimit
	}
	if cooldown {
		return ErrCommitActionCooldown
	}
	return nil
}

func parseAICommitActionLimit(raw []byte) (*int, error) {
	var policy automation.RiskPolicy
	if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &policy) != nil ||
		(policy.MaxTradesPerDay != nil && *policy.MaxTradesPerDay < 0) {
		return nil, ErrEvaluationConfigurationChanged
	}
	// Preserve the risk engine's optional-limit semantics for legacy versions;
	// never infer a limit from the mutable current draft or from another engine.
	return policy.MaxTradesPerDay, nil
}

func sameAICommitActivityDay(evaluatedAt, now time.Time) bool {
	return !evaluatedAt.IsZero() && !now.IsZero() && !evaluatedAt.After(now) &&
		evaluatedAt.UTC().Year() == now.UTC().Year() && evaluatedAt.UTC().YearDay() == now.UTC().YearDay()
}
