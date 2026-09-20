package strategy

import (
	"context"
	"errors"
	"math/big"

	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/jackc/pgx/v5"
)

// lockAIPaperCommitBindings closes the gap between pre-model reads and the
// Paper ledger commit. These row locks remain held through the entire commit.
// Lock order is mandate, bucket, instance, reservation, then portfolio in the
// caller. Initialization takes mandate before bucket; pause/finish take instance
// before reservation. No provider call or AI request belongs in this transaction.
// This is binding/lifecycle validation, not a replacement for the risk engine or
// a new authorization boundary for live execution.
func lockAIPaperCommitBindings(ctx context.Context, tx pgx.Tx, instance Instance) (string, error) {
	var mandateValid bool
	err := tx.QueryRow(ctx, `SELECT COALESCE(
		m.status IN ('READY','DRAFT') AND m.current_version >= $3
		AND v.snapshot->>'status'='READY'
		AND v.snapshot->>'execution_mode'='PAPER'
		AND v.snapshot->>'automation_type'='AI_AUTONOMOUS'
		AND v.snapshot->>'autonomy_level'='FULL_AUTONOMOUS'
		AND v.snapshot->>'financial_account_id'=$4
		AND v.snapshot->>'capital_bucket_id'=$5, false)
		FROM automation_mandates m
		JOIN automation_mandate_versions v ON v.mandate_id=m.id AND v.version_number=$3
		WHERE m.id=$1 AND m.user_id=$2 FOR SHARE OF m`,
		instance.AutomationMandateID, instance.UserID, instance.MandateVersion,
		instance.FinancialAccountID, instance.CapitalBucketID).Scan(&mandateValid)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !mandateValid) {
		return "", ErrEvaluationConfigurationChanged
	}
	if err != nil {
		return "", err
	}
	// A newer draft does not replace the immutable version pinned to this
	// instance. Paused, disabled and archived current mandates always stop it.
	var bucket automation.CapitalBucket
	err = tx.QueryRow(ctx, `SELECT allocation_type,allocation_value::text,currency,
		is_reserve,protected_amount::text,allocation_limit::text,status
		FROM capital_buckets WHERE id=$1 AND user_id=$2 AND financial_account_id=$3 FOR SHARE`,
		instance.CapitalBucketID, instance.UserID, instance.FinancialAccountID).Scan(
		&bucket.AllocationType, &bucket.AllocationValue, &bucket.Currency, &bucket.IsReserve,
		&bucket.ProtectedAmount, &bucket.AllocationLimit, &bucket.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrCapitalReservation
	}
	if err != nil {
		return "", err
	}
	capacity, available := paperCashCapacity(bucket)
	if bucket.Status != "ACTIVE" || bucket.IsReserve || bucket.Currency != "USD" || !available {
		return "", ErrCapitalReservation
	}

	var instanceID string
	// Event claims already hold a foreign-key KEY SHARE lock on this instance.
	// NO KEY UPDATE serializes ledger writers without a mutual lock-upgrade
	// deadlock between two different event claims for the same instance.
	err = tx.QueryRow(ctx, `SELECT id::text FROM strategy_instances
		WHERE id=$1 AND user_id=$2 AND automation_mandate_id=$3 AND mandate_version=$4
		AND financial_account_id=$5 AND capital_bucket_id=$6 AND state_version=$7
		AND strategy_identifier='ai_shadow' AND execution_mode='PAPER'
		AND current_state='AI_MONITORING' AND status='ACTIVE' FOR NO KEY UPDATE`,
		instance.ID, instance.UserID, instance.AutomationMandateID, instance.MandateVersion,
		instance.FinancialAccountID, instance.CapitalBucketID, instance.StateVersion).Scan(&instanceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrConflict
	}
	if err != nil {
		return "", err
	}

	var amount string
	var accountLimit *string
	err = tx.QueryRow(ctx, `SELECT reservation_amount::text,account_allocation_limit::text
		FROM strategy_capital_reservations
		WHERE strategy_instance_id=$1 AND user_id=$2 AND financial_account_id=$3
		AND capital_bucket_id=$4 AND execution_mode='PAPER' AND currency='USD'
		AND reservation_basis='PAPER_STARTING_CASH' AND released_at IS NULL
		AND release_reason IS NULL FOR SHARE`, instance.ID, instance.UserID,
		instance.FinancialAccountID, instance.CapitalBucketID).Scan(&amount, &accountLimit)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrCapitalReservation
	}
	if err != nil {
		return "", err
	}
	reserved, valid := new(big.Rat).SetString(amount)
	if !valid || reserved.Sign() <= 0 || reserved.Cmp(capacity) > 0 {
		return "", ErrCapitalReservation
	}
	if bucket.AllocationType == "FIXED_AMOUNT" {
		if (accountLimit == nil) != (bucket.AllocationLimit == nil) ||
			(accountLimit != nil && !sameAIPaperDecimal(*accountLimit, *bucket.AllocationLimit)) {
			return "", ErrCapitalReservation
		}
	} else if accountLimit != nil {
		return "", ErrCapitalReservation
	}
	return amount, nil
}
