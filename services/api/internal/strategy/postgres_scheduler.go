package strategy

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type scheduleRunScanner interface {
	Scan(...any) error
}

func scanScheduleRun(row scheduleRunScanner) (ScheduleRun, error) {
	var run ScheduleRun
	err := row.Scan(
		&run.ID, &run.StrategyInstanceID, &run.MandateID, &run.MandateVersion,
		&run.ExecutionMode, &run.StrategyState, &run.ScheduledFor, &run.StartedAt,
		&run.CompletedAt, &run.NextRunAt, &run.Status, &run.ErrorCode,
		&run.AIDecision, &run.ExecutionStatus, &run.DuplicateRecovered,
		&run.ReconciliationID, &run.ReconciliationReviewRequired,
		&run.ConsecutiveFailures,
	)
	return run, err
}

func (s *PostgresStore) ScheduleRuns(ctx context.Context, userID, instanceID string, limit int, cursor *ScheduleRunCursor) ([]ScheduleRun, error) {
	const columns = `r.id::text,r.strategy_instance_id::text,r.mandate_id::text,r.mandate_version,
		r.execution_mode,r.strategy_state,r.scheduled_for,r.started_at,r.completed_at,r.next_run_at,
		r.status,r.error_code,r.ai_decision,r.execution_status,r.duplicate_recovered,
		r.reconciliation_id::text,r.reconciliation_review_required,r.consecutive_failures`
	query := `SELECT ` + columns + ` FROM nonlive_schedule_runs r
		JOIN strategy_instances i ON i.id=r.strategy_instance_id AND i.user_id=r.user_id
		WHERE r.user_id=$1 AND r.strategy_instance_id=$2
		ORDER BY r.scheduled_for DESC,r.id DESC LIMIT $3`
	args := []any{userID, instanceID, limit}
	if cursor != nil {
		query = `SELECT ` + columns + ` FROM nonlive_schedule_runs r
			JOIN strategy_instances i ON i.id=r.strategy_instance_id AND i.user_id=r.user_id
			WHERE r.user_id=$1 AND r.strategy_instance_id=$2
			  AND (r.scheduled_for,r.id) < ($3,$4::uuid)
			ORDER BY r.scheduled_for DESC,r.id DESC LIMIT $5`
		args = []any{userID, instanceID, cursor.ScheduledFor, cursor.ID, limit}
	}
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []ScheduleRun{}
	for rows.Next() {
		run, scanErr := scanScheduleRun(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// ClaimDueSchedule takes one durable lease. The query repeats every authority
// check so a revoked entitlement, explicit stop, corrupted immutable version,
// or live mode fails closed before the worker receives any work. A newer DRAFT
// does not mutate or interrupt the reviewed version pinned to the instance.
func (s *PostgresStore) ClaimDueSchedule(ctx context.Context, now time.Time, leaseFor time.Duration) (*ScheduledRun, error) {
	var run ScheduledRun
	err := s.db.QueryRow(ctx, `WITH candidate AS (
		SELECT s.strategy_instance_id
		FROM nonlive_strategy_schedules s
		JOIN strategy_instances i ON i.id=s.strategy_instance_id AND i.user_id=s.user_id
		JOIN automation_mandates m ON m.id=s.mandate_id AND m.user_id=s.user_id
		JOIN automation_mandate_versions v ON v.mandate_id=s.mandate_id AND v.version_number=s.mandate_version
		JOIN users u ON u.id=s.user_id
		WHERE s.next_run_at <= $1
		  AND (s.lease_token IS NULL OR s.lease_expires_at <= $1)
		  AND i.status='ACTIVE' AND i.execution_mode IN ('PAPER','SHADOW')
		  AND i.automation_mandate_id=s.mandate_id AND i.mandate_version=s.mandate_version
		  AND m.status IN ('READY','DRAFT') AND m.current_version >= s.mandate_version
		  AND v.snapshot->>'status'='READY'
		  AND ((v.snapshot->>'automation_type'='STRATEGY' AND v.snapshot->>'autonomy_level'='STRATEGY_AUTONOMOUS') OR
		       (v.snapshot->>'automation_type'='AI_AUTONOMOUS' AND v.snapshot->>'autonomy_level'='FULL_AUTONOMOUS' AND i.strategy_identifier='ai_shadow' AND i.execution_mode IN ('PAPER','SHADOW')))
		  AND v.snapshot->>'execution_mode'=i.execution_mode
		  AND (v.snapshot->>'effective_from')::timestamptz <= $1
		  AND ((v.snapshot->>'effective_until') IS NULL OR (v.snapshot->>'effective_until')::timestamptz > $1)
		  AND v.snapshot #>> '{schedule_conditions,enabled}'='true'
		  AND v.snapshot #>> '{schedule_conditions,interval_minutes}'=s.interval_minutes::text
		  AND v.snapshot #>> '{schedule_conditions,session}'=s.session
		  AND u.status='active'
		  AND EXISTS (SELECT 1 FROM user_entitlements e WHERE e.user_id=s.user_id AND e.entitlement_key='founder' AND e.status='active' AND e.starts_at <= $1 AND (e.expires_at IS NULL OR e.expires_at > $1))
		ORDER BY s.next_run_at,s.strategy_instance_id
		FOR UPDATE OF s SKIP LOCKED LIMIT 1
	), claimed AS (
		UPDATE nonlive_strategy_schedules s
		SET lease_token=gen_random_uuid(),lease_expires_at=$1+($2 * interval '1 second'),last_started_at=$1,updated_at=$1
		FROM candidate c WHERE s.strategy_instance_id=c.strategy_instance_id
		RETURNING s.*
	)
	SELECT c.strategy_instance_id::text,c.user_id::text,i.financial_account_id::text,COALESCE(u.normalized_email,''),u.email_verified_at IS NOT NULL,c.mandate_id::text,c.mandate_version,
		i.execution_mode,i.current_state,c.interval_minutes,c.session,c.next_run_at,c.last_started_at,c.lease_token::text,
		COALESCE((v.snapshot #>> '{schedule_conditions,notifications,evaluation_completed}')::boolean,false),
		COALESCE((v.snapshot #>> '{schedule_conditions,notifications,lifecycle_required}')::boolean,false),
		COALESCE((v.snapshot #>> '{schedule_conditions,notifications,first_failure}')::boolean,false),
		COALESCE((v.snapshot #>> '{schedule_conditions,notifications,reconciliation_review_required}')::boolean,false),
		c.last_reconciliation_notification_id::text,c.last_error_code,c.consecutive_failures
	FROM claimed c
	JOIN strategy_instances i ON i.id=c.strategy_instance_id
	JOIN automation_mandate_versions v ON v.mandate_id=c.mandate_id AND v.version_number=c.mandate_version
	JOIN users u ON u.id=c.user_id`, now, int(leaseFor/time.Second)).Scan(
		&run.StrategyInstanceID, &run.UserID, &run.FinancialAccountID, &run.OwnerEmail, &run.OwnerEmailVerified, &run.MandateID, &run.MandateVersion,
		&run.ExecutionMode, &run.CurrentState, &run.IntervalMinutes, &run.Session,
		&run.ScheduledFor, &run.StartedAt, &run.LeaseToken, &run.NotifyEvaluation, &run.NotifyLifecycle,
		&run.NotifyFirstFailure, &run.NotifyReconciliationReview, &run.LastReconciliationNotificationID,
		&run.PreviousErrorCode, &run.ConsecutiveFailures,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *PostgresStore) RecordReconciliationNotification(ctx context.Context, run ScheduledRun, reconciliationID string, deliveredAt time.Time) error {
	command, err := s.db.Exec(ctx, `UPDATE nonlive_strategy_schedules
		SET last_reconciliation_notification_id=$3,last_reconciliation_notification_at=$4,updated_at=$4
		WHERE strategy_instance_id=$1 AND user_id=$2
		  AND last_reconciliation_notification_id IS DISTINCT FROM $3::uuid`,
		run.StrategyInstanceID, run.UserID, reconciliationID, deliveredAt.UTC())
	if err != nil {
		return err
	}
	if command.RowsAffected() == 1 {
		return nil
	}
	var existing *string
	err = s.db.QueryRow(ctx, `SELECT last_reconciliation_notification_id::text
		FROM nonlive_strategy_schedules WHERE strategy_instance_id=$1 AND user_id=$2`, run.StrategyInstanceID, run.UserID).Scan(&existing)
	if err != nil {
		return err
	}
	if existing != nil && *existing == reconciliationID {
		return nil
	}
	return ErrConflict
}

func (s *PostgresStore) CompleteSchedule(ctx context.Context, run ScheduledRun, completion ScheduleCompletion) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var consecutiveFailures int
	err = tx.QueryRow(ctx, `UPDATE nonlive_strategy_schedules
		SET next_run_at=$5,lease_token=NULL,lease_expires_at=NULL,last_completed_at=$4,last_status=$6,
			last_error_code=NULLIF($7,''),consecutive_failures=CASE WHEN $6='FAILED' THEN consecutive_failures+1 ELSE 0 END,updated_at=$4
		WHERE strategy_instance_id=$1 AND user_id=$2 AND lease_token=$3 AND next_run_at=$8
		RETURNING consecutive_failures`,
		run.StrategyInstanceID, run.UserID, run.LeaseToken, completion.CompletedAt.UTC(), completion.NextRunAt.UTC(), completion.Status, completion.ErrorCode, run.ScheduledFor.UTC()).Scan(&consecutiveFailures)
	if err == pgx.ErrNoRows {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO nonlive_schedule_runs(
		user_id,strategy_instance_id,mandate_id,mandate_version,execution_mode,strategy_state,
		scheduled_for,started_at,completed_at,next_run_at,status,error_code,ai_decision,execution_status,
		duplicate_recovered,reconciliation_id,reconciliation_review_required,consecutive_failures
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),NULLIF($13,''),NULLIF($14,''),$15,NULLIF($16,'')::uuid,$17,$18)`,
		run.UserID, run.StrategyInstanceID, run.MandateID, run.MandateVersion, run.ExecutionMode, run.CurrentState,
		run.ScheduledFor.UTC(), run.StartedAt.UTC(), completion.CompletedAt.UTC(), completion.NextRunAt.UTC(), completion.Status,
		completion.ErrorCode, completion.AIDecision, completion.ExecutionStatus, completion.DuplicateRecovered,
		completion.ReconciliationID, completion.ReconciliationReviewRequired, consecutiveFailures)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
