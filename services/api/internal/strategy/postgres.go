package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/connectionguard"
	"github.com/arbion/platform/services/api/internal/neural"
	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ db *pgxpool.Pool }

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore { return &PostgresStore{db} }

const instanceColumns = `id::text,user_id::text,automation_mandate_id::text,mandate_version,financial_account_id::text,capital_bucket_id::text,strategy_identifier,strategy_definition_version,execution_mode,current_state,state_version,status,started_at,updated_at,paused_at,completed_at,last_evaluated_at`

func scanInstance(r pgx.Row) (i Instance, e error) {
	e = r.Scan(&i.ID, &i.UserID, &i.AutomationMandateID, &i.MandateVersion, &i.FinancialAccountID, &i.CapitalBucketID, &i.StrategyIdentifier, &i.DefinitionVersion, &i.ExecutionMode, &i.CurrentState, &i.StateVersion, &i.Status, &i.StartedAt, &i.UpdatedAt, &i.PausedAt, &i.CompletedAt, &i.LastEvaluatedAt)
	return
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func initializeError(err error) error {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return err
	}
	if postgresError.ConstraintName == "strategy_one_active_account_idx" || postgresError.ConstraintName == "strategy_one_active_bucket_idx" || postgresError.ConstraintName == "strategy_one_active_reservation_bucket_idx" || postgresError.ConstraintName == "strategy_capital_reservation_account_guard" {
		return ErrAccountInUse
	}
	if postgresError.ConstraintName == "strategy_capital_reservation_resolved_guard" || postgresError.ConstraintName == "strategy_capital_reservation_instance_guard" || postgresError.ConstraintName == "strategy_capital_reservation_bucket_guard" || postgresError.ConstraintName == "strategy_capital_reservation_basis_guard" {
		return ErrCapitalReservation
	}
	if postgresError.Code != "23505" {
		return err
	}
	return ErrConflict
}

func (s *PostgresStore) Initialize(c context.Context, u string, m automation.Mandate, cash string, state State) (Instance, error) {
	schedule, e := automation.ParseScheduleConditions(m.ScheduleConditions)
	if e != nil {
		return Instance{}, e
	}
	tx, e := s.db.Begin(c)
	if e != nil {
		return Instance{}, e
	}
	defer tx.Rollback(c)
	if e = connectionguard.LockActive(c, tx, u, m.FinancialAccountID, m.AIProviderConnectionID); e != nil {
		if errors.Is(e, connectionguard.ErrUnavailable) {
			return Instance{}, ErrMandateStale
		}
		return Instance{}, e
	}
	var currentStatus, currentFinancialAccountID, currentCapitalBucketID, currentAutomationType, currentExecutionMode string
	var currentVersion int
	var currentAIConnectionID, currentStrategyIdentifier *string
	currentErr := tx.QueryRow(c, `SELECT status,current_version,financial_account_id::text,capital_bucket_id::text,automation_type,execution_mode,ai_provider_connection_id::text,strategy_identifier
		FROM automation_mandates WHERE id=$1 AND user_id=$2 FOR UPDATE`, m.ID, u).Scan(
		&currentStatus, &currentVersion, &currentFinancialAccountID, &currentCapitalBucketID,
		&currentAutomationType, &currentExecutionMode, &currentAIConnectionID, &currentStrategyIdentifier,
	)
	if currentErr != nil {
		if errors.Is(currentErr, pgx.ErrNoRows) {
			return Instance{}, ErrMandateStale
		}
		return Instance{}, currentErr
	}
	if currentStatus != "READY" || currentVersion != m.CurrentVersion || currentFinancialAccountID != m.FinancialAccountID || currentCapitalBucketID != m.CapitalBucketID || currentAutomationType != m.AutomationType || currentExecutionMode != m.ExecutionMode || !sameOptionalString(currentAIConnectionID, m.AIProviderConnectionID) || !sameOptionalString(currentStrategyIdentifier, m.StrategyIdentifier) {
		return Instance{}, ErrMandateStale
	}
	var bucket automation.CapitalBucket
	if e = tx.QueryRow(c, `SELECT id::text,user_id::text,financial_account_id::text,name,allocation_type,allocation_value::text,currency,is_reserve,protected_amount::text,allocation_limit::text,status,created_at,updated_at FROM capital_buckets WHERE id=$1 AND user_id=$2 AND financial_account_id=$3 FOR UPDATE`, m.CapitalBucketID, u, m.FinancialAccountID).Scan(
		&bucket.ID, &bucket.UserID, &bucket.FinancialAccountID, &bucket.Name, &bucket.AllocationType, &bucket.AllocationValue,
		&bucket.Currency, &bucket.IsReserve, &bucket.ProtectedAmount, &bucket.AllocationLimit, &bucket.Status, &bucket.CreatedAt, &bucket.UpdatedAt,
	); e != nil {
		return Instance{}, e
	}
	claim, e := reservationClaim(bucket, ExecutionMode(m.ExecutionMode), cash)
	if e != nil {
		return Instance{}, e
	}
	identifier := "ai_shadow"
	if m.StrategyIdentifier != nil {
		identifier = *m.StrategyIdentifier
	}
	i, e := scanInstance(tx.QueryRow(c, `INSERT INTO strategy_instances(user_id,automation_mandate_id,mandate_version,financial_account_id,capital_bucket_id,strategy_identifier,strategy_definition_version,execution_mode,current_state) VALUES($1,$2,$3,$4,$5,$6,1,$7,$8) RETURNING `+instanceColumns, u, m.ID, m.CurrentVersion, m.FinancialAccountID, m.CapitalBucketID, identifier, m.ExecutionMode, state))
	if e != nil {
		return i, initializeError(e)
	}
	if m.ExecutionMode == "PAPER" {
		if _, e = tx.Exec(c, `INSERT INTO paper_portfolios(user_id,strategy_instance_id,currency,starting_cash,cash) SELECT $1,$2,b.currency,$3,$3 FROM capital_buckets b WHERE b.id=$4`, u, i.ID, cash, m.CapitalBucketID); e != nil {
			return i, e
		}
	}
	var reservationID string
	if e = tx.QueryRow(c, `INSERT INTO strategy_capital_reservations(user_id,financial_account_id,capital_bucket_id,strategy_instance_id,execution_mode,reservation_amount,currency,reservation_basis,account_allocation_limit,reserved_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id::text`, u, m.FinancialAccountID, m.CapitalBucketID, i.ID, m.ExecutionMode, claim.Amount, claim.Currency, claim.Basis, claim.AccountAllocationLimit, i.StartedAt).Scan(&reservationID); e != nil {
		return i, initializeError(e)
	}
	meta, _ := json.Marshal(map[string]any{"mandate_version": m.CurrentVersion, "definition_version": 1, "capital_reservation_id": reservationID, "reserved_amount": claim.Amount, "reservation_currency": claim.Currency, "reservation_basis": claim.Basis})
	_, e = tx.Exec(c, `INSERT INTO strategy_state_transitions(strategy_instance_id,previous_state,new_state,state_version,trigger,metadata) VALUES($1,$2,$2,1,'INITIALIZED',$3)`, i.ID, state, meta)
	if e != nil {
		return i, e
	}
	if schedule.Enabled {
		_, e = tx.Exec(c, `INSERT INTO nonlive_strategy_schedules(strategy_instance_id,user_id,mandate_id,mandate_version,interval_minutes,session,next_run_at) VALUES($1,$2,$3,$4,$5::integer,$6,now()+($5::integer * interval '1 minute'))`, i.ID, u, m.ID, m.CurrentVersion, schedule.IntervalMinutes, schedule.Session)
		if e != nil {
			return i, e
		}
	}
	return i, tx.Commit(c)
}

func (s *PostgresStore) Pause(c context.Context, userID, instanceID string, expectedStateVersion int, pausedAt time.Time) (Instance, error) {
	tx, err := s.db.Begin(c)
	if err != nil {
		return Instance{}, err
	}
	defer tx.Rollback(c)

	current, err := scanInstance(tx.QueryRow(c, `SELECT `+instanceColumns+` FROM strategy_instances WHERE id=$1 AND user_id=$2 FOR UPDATE`, instanceID, userID))
	if err == pgx.ErrNoRows {
		return Instance{}, ErrNotFound
	}
	if err != nil {
		return Instance{}, err
	}
	if current.StateVersion != expectedStateVersion || current.Status != "ACTIVE" {
		return Instance{}, ErrConflict
	}
	paused, err := scanInstance(tx.QueryRow(c, `UPDATE strategy_instances
		SET status='PAUSED',state_version=state_version+1,paused_at=$4,updated_at=$4
		WHERE id=$1 AND user_id=$2 AND state_version=$3 AND status='ACTIVE'
		RETURNING `+instanceColumns, instanceID, userID, expectedStateVersion, pausedAt))
	if err == pgx.ErrNoRows {
		return Instance{}, ErrConflict
	}
	if err != nil {
		return Instance{}, err
	}
	metadata, _ := json.Marshal(map[string]any{"previous_status": "ACTIVE", "new_status": "PAUSED"})
	if _, err = tx.Exec(c, `INSERT INTO strategy_state_transitions(strategy_instance_id,previous_state,new_state,state_version,trigger,metadata) VALUES($1,$2,$2,$3,'PAUSED',$4)`, paused.ID, current.CurrentState, paused.StateVersion, metadata); err != nil {
		return Instance{}, err
	}
	return paused, tx.Commit(c)
}

func (s *PostgresStore) Resume(c context.Context, userID, instanceID string, expectedStateVersion int, resumedAt time.Time) (Instance, error) {
	tx, err := s.db.Begin(c)
	if err != nil {
		return Instance{}, err
	}
	defer tx.Rollback(c)

	current, err := scanInstance(tx.QueryRow(c, `SELECT `+instanceColumns+` FROM strategy_instances WHERE id=$1 AND user_id=$2 FOR UPDATE`, instanceID, userID))
	if err == pgx.ErrNoRows {
		return Instance{}, ErrNotFound
	}
	if err != nil {
		return Instance{}, err
	}
	if current.StateVersion != expectedStateVersion || current.Status != "PAUSED" {
		return Instance{}, ErrConflict
	}
	var mandateReady bool
	if err = tx.QueryRow(c, `SELECT EXISTS(
		SELECT 1
		FROM automation_mandates m
		JOIN automation_mandate_versions v ON v.mandate_id=m.id AND v.version_number=$3
		WHERE m.id=$1 AND m.user_id=$2 AND m.status IN ('READY','DRAFT') AND m.current_version >= $3
		  AND v.snapshot->>'status'='READY'
		  AND v.snapshot->>'financial_account_id'=$4
		  AND v.snapshot->>'capital_bucket_id'=$5
		  AND v.snapshot->>'execution_mode'=$6
	)`, current.AutomationMandateID, userID, current.MandateVersion, current.FinancialAccountID, current.CapitalBucketID, current.ExecutionMode).Scan(&mandateReady); err != nil {
		return Instance{}, err
	}
	if !mandateReady {
		return Instance{}, ErrMandateStale
	}
	resumed, err := scanInstance(tx.QueryRow(c, `UPDATE strategy_instances
		SET status='ACTIVE',state_version=state_version+1,paused_at=NULL,updated_at=$4
		WHERE id=$1 AND user_id=$2 AND state_version=$3 AND status='PAUSED'
		RETURNING `+instanceColumns, instanceID, userID, expectedStateVersion, resumedAt))
	if err == pgx.ErrNoRows {
		return Instance{}, ErrConflict
	}
	if err != nil {
		return Instance{}, err
	}
	metadata, _ := json.Marshal(map[string]any{"previous_status": "PAUSED", "new_status": "ACTIVE"})
	if _, err = tx.Exec(c, `INSERT INTO strategy_state_transitions(strategy_instance_id,previous_state,new_state,state_version,trigger,metadata) VALUES($1,$2,$2,$3,'RESUMED',$4)`, resumed.ID, current.CurrentState, resumed.StateVersion, metadata); err != nil {
		return Instance{}, err
	}
	return resumed, tx.Commit(c)
}

func (s *PostgresStore) Finish(c context.Context, userID, instanceID string, expectedStateVersion int, finishedAt time.Time) (Instance, error) {
	tx, err := s.db.Begin(c)
	if err != nil {
		return Instance{}, err
	}
	defer tx.Rollback(c)

	current, err := scanInstance(tx.QueryRow(c, `SELECT `+instanceColumns+` FROM strategy_instances WHERE id=$1 AND user_id=$2 FOR UPDATE`, instanceID, userID))
	if err == pgx.ErrNoRows {
		return Instance{}, ErrNotFound
	}
	if err != nil {
		return Instance{}, err
	}
	if current.StateVersion != expectedStateVersion || (current.Status != "ACTIVE" && current.Status != "PAUSED") {
		return Instance{}, ErrConflict
	}
	if current.ExecutionMode == Paper {
		var openExposure bool
		err = tx.QueryRow(c, `SELECT EXISTS(
			SELECT 1 FROM paper_portfolios p
			JOIN paper_positions x ON x.paper_portfolio_id=p.id
			WHERE p.strategy_instance_id=$1 AND p.user_id=$2 AND x.quantity<>0
		)`, instanceID, userID).Scan(&openExposure)
		if err != nil {
			return Instance{}, err
		}
		if openExposure {
			return Instance{}, ErrOpenExposure
		}
	}

	finished, err := scanInstance(tx.QueryRow(c, `UPDATE strategy_instances
		SET status='COMPLETED',state_version=state_version+1,completed_at=$4,paused_at=NULL,updated_at=$4
		WHERE id=$1 AND user_id=$2 AND state_version=$3 AND status IN ('ACTIVE','PAUSED')
		RETURNING `+instanceColumns, instanceID, userID, expectedStateVersion, finishedAt))
	if err == pgx.ErrNoRows {
		return Instance{}, ErrConflict
	}
	if err != nil {
		return Instance{}, err
	}
	result, err := tx.Exec(c, `UPDATE strategy_capital_reservations
		SET released_at=$3,release_reason='COMPLETED'
		WHERE strategy_instance_id=$1 AND user_id=$2 AND released_at IS NULL`, instanceID, userID, finishedAt)
	if err != nil {
		return Instance{}, err
	}
	if result.RowsAffected() != 1 {
		return Instance{}, ErrConflict
	}
	metadata, _ := json.Marshal(map[string]any{"previous_status": current.Status, "new_status": "COMPLETED"})
	if _, err = tx.Exec(c, `INSERT INTO strategy_state_transitions(strategy_instance_id,previous_state,new_state,state_version,trigger,metadata) VALUES($1,$2,$2,$3,'FINISHED',$4)`, finished.ID, current.CurrentState, finished.StateVersion, metadata); err != nil {
		return Instance{}, err
	}
	return finished, tx.Commit(c)
}
func (s *PostgresStore) List(c context.Context, u string) ([]Instance, error) {
	rows, e := s.db.Query(c, `SELECT `+instanceColumns+` FROM strategy_instances WHERE user_id=$1 ORDER BY updated_at DESC`, u)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Instance{}
	for rows.Next() {
		i, e := scanInstance(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (s *PostgresStore) Get(c context.Context, u, id string) (Instance, error) {
	return scanInstance(s.db.QueryRow(c, `SELECT `+instanceColumns+` FROM strategy_instances WHERE id=$1 AND user_id=$2`, id, u))
}

func (s *PostgresStore) CapitalReservation(c context.Context, userID, instanceID string) (CapitalReservation, error) {
	var reservation CapitalReservation
	err := s.db.QueryRow(c, `SELECT id::text,strategy_instance_id::text,financial_account_id::text,capital_bucket_id::text,execution_mode,reservation_amount::text,currency,reservation_basis,account_allocation_limit::text,CASE WHEN released_at IS NULL THEN 'ACTIVE' ELSE 'RELEASED' END,reserved_at,released_at,release_reason
		FROM strategy_capital_reservations WHERE strategy_instance_id=$1 AND user_id=$2`, instanceID, userID).Scan(
		&reservation.ID, &reservation.StrategyInstanceID, &reservation.FinancialAccountID, &reservation.CapitalBucketID,
		&reservation.ExecutionMode, &reservation.ReservationAmount, &reservation.Currency, &reservation.ReservationBasis,
		&reservation.AccountAllocationLimit, &reservation.Status, &reservation.ReservedAt, &reservation.ReleasedAt, &reservation.ReleaseReason,
	)
	return reservation, err
}

func (s *PostgresStore) CapitalReservations(c context.Context, userID string) ([]CapitalReservation, error) {
	rows, err := s.db.Query(c, `SELECT id::text,strategy_instance_id::text,financial_account_id::text,capital_bucket_id::text,execution_mode,reservation_amount::text,currency,reservation_basis,account_allocation_limit::text,CASE WHEN released_at IS NULL THEN 'ACTIVE' ELSE 'RELEASED' END,reserved_at,released_at,release_reason
		FROM strategy_capital_reservations WHERE user_id=$1 AND released_at IS NULL ORDER BY reserved_at DESC,id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reservations := []CapitalReservation{}
	for rows.Next() {
		var reservation CapitalReservation
		if err = rows.Scan(
			&reservation.ID, &reservation.StrategyInstanceID, &reservation.FinancialAccountID, &reservation.CapitalBucketID,
			&reservation.ExecutionMode, &reservation.ReservationAmount, &reservation.Currency, &reservation.ReservationBasis,
			&reservation.AccountAllocationLimit, &reservation.Status, &reservation.ReservedAt, &reservation.ReleasedAt, &reservation.ReleaseReason,
		); err != nil {
			return nil, err
		}
		reservations = append(reservations, reservation)
	}
	return reservations, rows.Err()
}

func (s *PostgresStore) Schedule(c context.Context, u, id string) (ScheduleStatus, error) {
	var status ScheduleStatus
	status.StrategyInstanceID = id
	err := s.db.QueryRow(c, `SELECT s.strategy_instance_id IS NOT NULL,
		COALESCE(s.mandate_id::text,''),COALESCE(s.mandate_version,0),COALESCE(s.interval_minutes,0),COALESCE(s.session,''),
		s.next_run_at,s.last_started_at,s.last_completed_at,s.last_status,s.last_error_code,COALESCE(s.consecutive_failures,0)
		FROM strategy_instances i LEFT JOIN nonlive_strategy_schedules s ON s.strategy_instance_id=i.id
		WHERE i.id=$1 AND i.user_id=$2`, id, u).Scan(
		&status.Enabled, &status.MandateID, &status.MandateVersion, &status.IntervalMinutes, &status.Session,
		&status.NextRunAt, &status.LastStartedAt, &status.LastCompletedAt, &status.LastStatus, &status.LastErrorCode, &status.ConsecutiveFailures,
	)
	return status, err
}
func (s *PostgresStore) StrategyTransitionEntries(c context.Context, userID, instanceID string, limit int, after *StrategyTransitionCursor) ([]StrategyTransitionEvidence, error) {
	query := `SELECT t.id::text,t.strategy_instance_id::text,t.previous_state,t.new_state,t.state_version,t.trigger,t.occurred_at
		FROM strategy_state_transitions t
		JOIN strategy_instances i ON i.id=t.strategy_instance_id
		WHERE i.id=$1 AND i.user_id=$2
		ORDER BY t.state_version DESC,t.id DESC
		LIMIT $3`
	args := []any{instanceID, userID, limit}
	if after != nil {
		query = `SELECT t.id::text,t.strategy_instance_id::text,t.previous_state,t.new_state,t.state_version,t.trigger,t.occurred_at
			FROM strategy_state_transitions t
			JOIN strategy_instances i ON i.id=t.strategy_instance_id
			WHERE i.id=$1 AND i.user_id=$2 AND (t.state_version,t.id) < ($3,$4::uuid)
			ORDER BY t.state_version DESC,t.id DESC
			LIMIT $5`
		args = []any{instanceID, userID, after.StateVersion, after.ID, limit}
	}
	rows, err := s.db.Query(c, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	transitions := []StrategyTransitionEvidence{}
	for rows.Next() {
		var transition StrategyTransitionEvidence
		if err = rows.Scan(&transition.ID, &transition.StrategyInstanceID, &transition.PreviousState, &transition.NewState, &transition.StateVersion, &transition.Trigger, &transition.OccurredAt); err != nil {
			return nil, err
		}
		transitions = append(transitions, transition)
	}
	return transitions, rows.Err()
}

const strategyDecisionColumns = `d.id::text,d.strategy_instance_id::text,d.strategy_state,d.source,d.decision_type,d.structured_rationale,d.proposed_action_id,d.risk_evaluation_id::text,d.execution_record_id::text,d.resulting_state,d.created_at,r.decision,r.approval_required,r.reason_codes,r.checks,x.status,x.symbol,x.instrument,x.side,x.quantity::text,x.price::text,x.notional::text`

func scanStrategyDecision(row pgx.Row) (entry DecisionJournalEntry, err error) {
	err = row.Scan(
		&entry.ID, &entry.StrategyInstanceID, &entry.StrategyState, &entry.Source, &entry.DecisionType,
		&entry.StructuredRationale, &entry.ProposedActionID, &entry.RiskEvaluationID,
		&entry.ExecutionRecordID, &entry.ResultingState, &entry.CreatedAt, &entry.RiskDecision,
		&entry.ApprovalRequired, &entry.RiskReasonCodes, &entry.RiskChecks,
		&entry.ExecutionStatus, &entry.Symbol, &entry.Instrument, &entry.Side, &entry.Quantity,
		&entry.Price, &entry.Notional,
	)
	return entry, err
}

func collectStrategyDecisions(rows pgx.Rows) ([]DecisionJournalEntry, error) {
	defer rows.Close()
	entries := []DecisionJournalEntry{}
	for rows.Next() {
		entry, err := scanStrategyDecision(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *PostgresStore) StrategyDecisionEntries(c context.Context, userID, instanceID string, limit int, after *StrategyDecisionCursor) ([]DecisionJournalEntry, error) {
	query := `SELECT ` + strategyDecisionColumns + `
		FROM decision_journal_entries d
		JOIN strategy_instances i ON i.id=d.strategy_instance_id AND i.user_id=d.user_id
		LEFT JOIN risk_evaluations r ON r.id=d.risk_evaluation_id AND r.user_id=d.user_id
		LEFT JOIN nonlive_execution_records x ON x.id=d.execution_record_id AND x.user_id=d.user_id
		WHERE i.id=$1 AND i.user_id=$2
		ORDER BY d.created_at DESC,d.id DESC
		LIMIT $3`
	args := []any{instanceID, userID, limit}
	if after != nil {
		query = `SELECT ` + strategyDecisionColumns + `
			FROM decision_journal_entries d
			JOIN strategy_instances i ON i.id=d.strategy_instance_id AND i.user_id=d.user_id
			LEFT JOIN risk_evaluations r ON r.id=d.risk_evaluation_id AND r.user_id=d.user_id
			LEFT JOIN nonlive_execution_records x ON x.id=d.execution_record_id AND x.user_id=d.user_id
			WHERE i.id=$1 AND i.user_id=$2 AND (d.created_at,d.id) < ($3,$4::uuid)
			ORDER BY d.created_at DESC,d.id DESC
			LIMIT $5`
		args = []any{instanceID, userID, after.CreatedAt, after.ID, limit}
	}
	rows, err := s.db.Query(c, query, args...)
	if err != nil {
		return nil, err
	}
	return collectStrategyDecisions(rows)
}

const journalColumns = `d.id::text,d.created_at,d.strategy_instance_id::text,d.financial_account_id::text,a.display_name,d.mandate_id::text,d.mandate_version,i.strategy_identifier,i.execution_mode,d.strategy_state,d.resulting_state,d.source,d.decision_type,d.structured_rationale,r.decision,r.approval_required,r.reason_codes,r.checks,x.status,x.symbol,x.instrument,x.side,x.quantity::text,x.price::text,x.notional::text`

func scanJournalActivity(rows pgx.Rows) (JournalActivity, error) {
	var activity JournalActivity
	err := rows.Scan(
		&activity.ID, &activity.CreatedAt, &activity.StrategyInstanceID,
		&activity.FinancialAccountID, &activity.AccountDisplayName,
		&activity.MandateID, &activity.MandateVersion, &activity.StrategyIdentifier,
		&activity.ExecutionMode, &activity.StrategyState, &activity.ResultingState,
		&activity.Source, &activity.DecisionType, &activity.StructuredRationale,
		&activity.RiskDecision, &activity.ApprovalRequired, &activity.RiskReasonCodes,
		&activity.RiskChecks, &activity.ExecutionStatus, &activity.Symbol,
		&activity.Instrument, &activity.Side, &activity.Quantity, &activity.Price,
		&activity.Notional,
	)
	return activity, err
}

func (s *PostgresStore) Journal(c context.Context, userID string, limit int, cursor *JournalCursor) ([]JournalActivity, error) {
	const joins = ` FROM decision_journal_entries d JOIN strategy_instances i ON i.id=d.strategy_instance_id AND i.user_id=d.user_id JOIN financial_accounts a ON a.id=d.financial_account_id AND a.user_id=d.user_id LEFT JOIN risk_evaluations r ON r.id=d.risk_evaluation_id AND r.user_id=d.user_id LEFT JOIN nonlive_execution_records x ON x.id=d.execution_record_id AND x.user_id=d.user_id`
	query := `SELECT ` + journalColumns + joins + ` WHERE d.user_id=$1 ORDER BY d.created_at DESC,d.id DESC LIMIT $2`
	args := []any{userID, limit}
	if cursor != nil {
		query = `SELECT ` + journalColumns + joins + ` WHERE d.user_id=$1 AND (d.created_at,d.id) < ($2,$3::uuid) ORDER BY d.created_at DESC,d.id DESC LIMIT $4`
		args = []any{userID, cursor.CreatedAt, cursor.ID, limit}
	}
	rows, err := s.db.Query(c, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	activities := []JournalActivity{}
	for rows.Next() {
		activity, scanErr := scanJournalActivity(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		activities = append(activities, activity)
	}
	return activities, rows.Err()
}
func (s *PostgresStore) StrategyExecutionEntries(c context.Context, userID, instanceID string, limit int, after *StrategyExecutionCursor) ([]StrategyExecutionEvidence, error) {
	query := `SELECT x.id::text,x.strategy_instance_id::text,x.mandate_version,x.mode,x.status,x.symbol,x.instrument,x.side,x.quantity::text,x.price::text,x.notional::text,x.created_at
		FROM nonlive_execution_records x
		WHERE x.strategy_instance_id=$1 AND x.user_id=$2
		ORDER BY x.created_at DESC,x.id DESC
		LIMIT $3`
	args := []any{instanceID, userID, limit}
	if after != nil {
		query = `SELECT x.id::text,x.strategy_instance_id::text,x.mandate_version,x.mode,x.status,x.symbol,x.instrument,x.side,x.quantity::text,x.price::text,x.notional::text,x.created_at
			FROM nonlive_execution_records x
			WHERE x.strategy_instance_id=$1 AND x.user_id=$2 AND (x.created_at,x.id) < ($3,$4::uuid)
			ORDER BY x.created_at DESC,x.id DESC
			LIMIT $5`
		args = []any{instanceID, userID, after.CreatedAt, after.ID, limit}
	}
	rows, err := s.db.Query(c, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	executions := []StrategyExecutionEvidence{}
	for rows.Next() {
		var execution StrategyExecutionEvidence
		if err = rows.Scan(&execution.ID, &execution.StrategyInstanceID, &execution.MandateVersion, &execution.Mode, &execution.Status, &execution.Symbol, &execution.Instrument, &execution.Side, &execution.Quantity, &execution.Price, &execution.Notional, &execution.CreatedAt); err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	return executions, rows.Err()
}

func (s *PostgresStore) PaperPortfolio(c context.Context, userID, instanceID string) (PaperPortfolio, error) {
	tx, err := s.db.BeginTx(c, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return PaperPortfolio{}, err
	}
	defer tx.Rollback(c)

	var portfolioID string
	var instanceStartedAt time.Time
	var intervalMinutes int
	var latestScheduleCompletedAt *time.Time
	portfolio := PaperPortfolio{StrategyInstanceID: instanceID, Positions: []PaperPosition{}}
	err = tx.QueryRow(c, `SELECT f.id::text,f.currency,f.starting_cash::text,f.cash::text,f.version,f.updated_at,i.started_at,COALESCE(s.interval_minutes,0),s.last_completed_at
		FROM paper_portfolios f JOIN strategy_instances i ON i.id=f.strategy_instance_id AND i.user_id=f.user_id
		LEFT JOIN nonlive_strategy_schedules s ON s.strategy_instance_id=i.id AND s.user_id=i.user_id
		WHERE f.strategy_instance_id=$1 AND f.user_id=$2 AND i.execution_mode='PAPER'`, instanceID, userID).Scan(
		&portfolioID, &portfolio.Currency, &portfolio.StartingCash, &portfolio.Cash, &portfolio.Version, &portfolio.UpdatedAt, &instanceStartedAt, &intervalMinutes, &latestScheduleCompletedAt,
	)
	if err != nil {
		return PaperPortfolio{}, err
	}

	rows, err := tx.Query(c, `SELECT p.symbol,p.instrument,COALESCE(p.option_type,''),COALESCE(p.strike::text,''),COALESCE(p.expiration::text,''),p.quantity::text,p.average_price::text,p.quantity<>0,p.updated_at
		FROM paper_positions p
		JOIN paper_portfolios f ON f.id=p.paper_portfolio_id
		JOIN strategy_instances i ON i.id=f.strategy_instance_id AND i.user_id=f.user_id
		WHERE p.paper_portfolio_id=$1 AND f.strategy_instance_id=$2 AND f.user_id=$3 AND i.user_id=$3 AND i.execution_mode='PAPER'
		ORDER BY CASE WHEN p.quantity<>0 THEN 0 ELSE 1 END,p.instrument,p.symbol,p.expiration NULLS LAST,p.strike NULLS LAST,p.id`, portfolioID, instanceID, userID)
	if err != nil {
		return PaperPortfolio{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var position PaperPosition
		if err = rows.Scan(&position.Symbol, &position.Instrument, &position.OptionType, &position.Strike, &position.Expiration, &position.Quantity, &position.AveragePrice, &position.IsOpen, &position.UpdatedAt); err != nil {
			return PaperPortfolio{}, err
		}
		portfolio.Positions = append(portfolio.Positions, position)
	}
	if err = rows.Err(); err != nil {
		return PaperPortfolio{}, err
	}
	rows.Close()
	fillRows, err := tx.Query(c, `SELECT f.id::text,f.execution_record_id::text,f.proposed_action_id,f.risk_evaluation_id::text,
		f.symbol,f.instrument,f.side,f.quantity::text,f.requested_notional::text,f.reference_price::text,f.fill_price::text,f.gross_notional::text,f.fee::text,
		f.previous_cash::text,f.previous_position_quantity::text,f.resulting_cash::text,f.resulting_position_quantity::text,
		f.pricing_basis,f.market_provider,f.market_feed,f.market_quality,f.market_observed_at,f.simulated_at,f.simulation_only
		FROM ai_paper_spot_fills f
		WHERE f.paper_portfolio_id=$1 AND f.strategy_instance_id=$2 AND f.user_id=$3
		ORDER BY f.simulated_at,f.id`, portfolioID, instanceID, userID)
	if err != nil {
		return PaperPortfolio{}, err
	}
	defer fillRows.Close()
	fills := []paperRealizedFill{}
	for fillRows.Next() {
		var fill paperRealizedFill
		if err = fillRows.Scan(&fill.ID, &fill.ExecutionRecordID, &fill.ProposedActionID, &fill.RiskEvaluationID,
			&fill.Symbol, &fill.Instrument, &fill.Side, &fill.Quantity, &fill.RequestedNotional, &fill.ReferencePrice, &fill.FillPrice, &fill.GrossNotional, &fill.Fee,
			&fill.PreviousCash, &fill.PreviousPositionQuantity, &fill.ResultingCash, &fill.ResultingPositionQuantity,
			&fill.PricingBasis, &fill.MarketProvider, &fill.MarketFeed, &fill.MarketQuality, &fill.MarketObservedAt, &fill.SimulatedAt, &fill.SimulationOnly); err != nil {
			return PaperPortfolio{}, err
		}
		fills = append(fills, fill)
	}
	if err = fillRows.Err(); err != nil {
		return PaperPortfolio{}, err
	}
	fillRows.Close()
	portfolio.RealizedOutcome = projectPaperRealizedOutcome(portfolio.StartingCash, portfolio, fills)
	portfolio.ExecutionCosts = projectPaperExecutionCosts(portfolio.RealizedOutcome, portfolio.StartingCash, fills)
	runs := []ScheduleRun{}
	guardrailRows := []paperGuardrailProposalRow{}
	if latestScheduleCompletedAt != nil && intervalMinutes >= 30 {
		cadenceStart := latestScheduleCompletedAt.Add(-7*24*time.Hour - time.Duration(intervalMinutes)*time.Minute)
		const runColumns = `r.id::text,r.strategy_instance_id::text,r.mandate_id::text,r.mandate_version,
			r.execution_mode,r.strategy_state,r.scheduled_for,r.started_at,r.completed_at,r.next_run_at,
			r.status,r.error_code,r.ai_decision,r.execution_status,r.duplicate_recovered,
			r.reconciliation_id::text,r.reconciliation_review_required,r.consecutive_failures`
		runRows, runErr := tx.Query(c, `SELECT `+runColumns+` FROM nonlive_schedule_runs r
			WHERE r.strategy_instance_id=$1 AND r.user_id=$2 AND r.completed_at >= $3 AND r.completed_at <= $4
			ORDER BY r.scheduled_for,r.id`, instanceID, userID, cadenceStart, *latestScheduleCompletedAt)
		if runErr != nil {
			return PaperPortfolio{}, runErr
		}
		for runRows.Next() {
			run, scanErr := scanScheduleRun(runRows)
			if scanErr != nil {
				runRows.Close()
				return PaperPortfolio{}, scanErr
			}
			runs = append(runs, run)
		}
		if runErr = runRows.Err(); runErr != nil {
			runRows.Close()
			return PaperPortfolio{}, runErr
		}
		runRows.Close()
		guardrailQueryRows, guardrailErr := tx.Query(c, `SELECT d.id::text,d.created_at,d.decision_type,d.proposed_action_id,
			r.id::text,x.id::text,d.structured_rationale,r.decision,r.approval_required,r.execution_mode,r.platform_execution_available,r.reason_codes,r.checks,
			x.mode,x.status,x.symbol,x.instrument,x.side,x.quantity::text,x.notional::text
			FROM decision_journal_entries d
			JOIN risk_evaluations r ON r.id=d.risk_evaluation_id AND r.user_id=d.user_id
			  AND r.proposed_action_id=d.proposed_action_id AND r.financial_account_id=d.financial_account_id
			  AND r.mandate_id=d.mandate_id AND r.mandate_version=d.mandate_version
			JOIN nonlive_execution_records x ON x.id=d.execution_record_id AND x.user_id=d.user_id AND x.risk_evaluation_id=r.id
			  AND x.proposed_action_id=d.proposed_action_id AND x.strategy_instance_id=d.strategy_instance_id
			  AND x.mandate_id=d.mandate_id AND x.mandate_version=d.mandate_version
			WHERE d.strategy_instance_id=$1 AND d.user_id=$2 AND d.source='AI'
			  AND d.decision_type IN ('ALLOW_SIMULATED_FILLED','DENY_RISK_DENIED')
			  AND d.created_at >= $3 AND d.created_at <= $4
			ORDER BY d.created_at,d.id`, instanceID, userID, cadenceStart, *latestScheduleCompletedAt)
		if guardrailErr != nil {
			return PaperPortfolio{}, guardrailErr
		}
		for guardrailQueryRows.Next() {
			var row paperGuardrailProposalRow
			if guardrailErr = guardrailQueryRows.Scan(&row.DecisionJournalEntryID, &row.CreatedAt, &row.DecisionType, &row.ProposedActionID,
				&row.RiskEvaluationID, &row.ExecutionRecordID, &row.Rationale, &row.RiskDecision, &row.ApprovalRequired,
				&row.RiskExecutionMode, &row.PlatformExecutionAvailable, &row.ReasonCodes, &row.Checks,
				&row.ExecutionMode, &row.ExecutionStatus, &row.Symbol, &row.Instrument, &row.Side, &row.ExecutionQuantity, &row.ExecutionNotional); guardrailErr != nil {
				guardrailQueryRows.Close()
				return PaperPortfolio{}, guardrailErr
			}
			guardrailRows = append(guardrailRows, row)
		}
		if guardrailErr = guardrailQueryRows.Err(); guardrailErr != nil {
			guardrailQueryRows.Close()
			return PaperPortfolio{}, guardrailErr
		}
		guardrailQueryRows.Close()
	}
	portfolio.ActivityCadence = projectPaperActivityCadence(instanceID, instanceStartedAt, intervalMinutes, runs, fills, portfolio.ExecutionCosts.Status != PaperExecutionCostsUnavailable)
	portfolio.GuardrailEvidence = projectPaperGuardrailEvidence(portfolio.ActivityCadence, guardrailRows, fills)
	if err = tx.Commit(c); err != nil {
		return PaperPortfolio{}, err
	}
	return portfolio, nil
}

func (s *PostgresStore) AIPaperSpotFills(c context.Context, userID, instanceID string, limit int, after *AIPaperSpotFillCursor) ([]AIPaperSpotFill, error) {
	query := `SELECT f.id::text,f.strategy_instance_id::text,f.execution_record_id::text,f.proposed_action_id,f.risk_evaluation_id::text,
		f.symbol,f.instrument,f.side,f.quantity::text,f.requested_notional::text,f.reference_price::text,f.fill_price::text,
		f.gross_notional::text,f.fee::text,f.previous_cash::text,f.previous_position_quantity::text,f.resulting_cash::text,
		f.resulting_position_quantity::text,f.pricing_basis,f.market_provider,f.market_feed,f.market_quality,
		f.market_observed_at,f.simulated_at,f.simulation_only
		FROM ai_paper_spot_fills f
		JOIN strategy_instances i ON i.id=f.strategy_instance_id AND i.user_id=f.user_id
		WHERE f.strategy_instance_id=$1 AND f.user_id=$2
		  AND i.strategy_identifier='ai_shadow' AND i.execution_mode='PAPER'
		ORDER BY f.simulated_at DESC,f.id DESC LIMIT $3`
	args := []any{instanceID, userID, limit}
	if after != nil {
		query = `SELECT f.id::text,f.strategy_instance_id::text,f.execution_record_id::text,f.proposed_action_id,f.risk_evaluation_id::text,
			f.symbol,f.instrument,f.side,f.quantity::text,f.requested_notional::text,f.reference_price::text,f.fill_price::text,
			f.gross_notional::text,f.fee::text,f.previous_cash::text,f.previous_position_quantity::text,f.resulting_cash::text,
			f.resulting_position_quantity::text,f.pricing_basis,f.market_provider,f.market_feed,f.market_quality,
			f.market_observed_at,f.simulated_at,f.simulation_only
			FROM ai_paper_spot_fills f
			JOIN strategy_instances i ON i.id=f.strategy_instance_id AND i.user_id=f.user_id
			WHERE f.strategy_instance_id=$1 AND f.user_id=$2
			  AND i.strategy_identifier='ai_shadow' AND i.execution_mode='PAPER'
			  AND (f.simulated_at,f.id)<($3,$4)
			ORDER BY f.simulated_at DESC,f.id DESC LIMIT $5`
		args = []any{instanceID, userID, after.SimulatedAt, after.ID, limit}
	}
	rows, err := s.db.Query(c, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fills := []AIPaperSpotFill{}
	for rows.Next() {
		var fill AIPaperSpotFill
		if err = rows.Scan(&fill.ID, &fill.StrategyInstanceID, &fill.ExecutionRecordID, &fill.ProposedActionID, &fill.RiskEvaluationID,
			&fill.Symbol, &fill.Instrument, &fill.Side, &fill.Quantity, &fill.RequestedNotional, &fill.ReferencePrice, &fill.FillPrice,
			&fill.GrossNotional, &fill.Fee, &fill.PreviousCash, &fill.PreviousPositionQuantity, &fill.ResultingCash,
			&fill.ResultingPositionQuantity, &fill.PricingBasis, &fill.MarketProvider, &fill.MarketFeed, &fill.MarketQuality,
			&fill.MarketObservedAt, &fill.SimulatedAt, &fill.SimulationOnly); err != nil {
			return nil, err
		}
		fills = append(fills, fill)
	}
	return fills, rows.Err()
}

var _ Persistence = (*PostgresStore)(nil)
var _ Repository = (*PostgresStore)(nil)

func (s *PostgresStore) CommitEvaluation(c context.Context, instance Instance, expectedVersion int, decision Decision, evaluation risk.RiskEvaluation, result ExecutionResult, evaluatedAt time.Time) error {
	if decision.ProposedAction == nil || decision.ProposedAction.ID == "" || decision.ProposedAction.CorrelationID == "" || evaluation.ID == "" || evaluatedAt.IsZero() || expectedVersion < 1 {
		return ErrInvalid
	}
	action := *decision.ProposedAction
	source := decision.Source
	if source == "" {
		source = "STRATEGY"
	}
	instrumentType := decision.InstrumentType
	if instrumentType == "" {
		instrumentType = "OPTION"
	}
	if (source != "STRATEGY" && source != "AI") || (instrumentType != "OPTION" && instrumentType != "EQUITY" && instrumentType != "CRYPTO") || (source == "AI" && instance.StrategyIdentifier != "ai_shadow") {
		return ErrInvalid
	}
	if action.FinancialAccountID != instance.FinancialAccountID || evaluation.UserID != instance.UserID || evaluation.AccountID != instance.FinancialAccountID || action.MandateID == nil || *action.MandateID != instance.AutomationMandateID || action.MandateVersion == nil || *action.MandateVersion != instance.MandateVersion {
		return ErrInvalid
	}
	if !json.Valid(decision.Rationale) || len(decision.Rationale) == 0 || decision.Rationale[0] != '{' {
		return ErrInvalid
	}
	reasonCodes, err := json.Marshal(evaluation.ReasonCodes)
	if err != nil {
		return err
	}
	checks, err := json.Marshal(evaluation.Checks)
	if err != nil {
		return err
	}
	executionMetadata, err := json.Marshal(map[string]any{
		"candidate_count":          decision.CandidateCount,
		"expected_state":           result.ExpectedState,
		"live_execution_available": false,
		"proposed_notional":        action.Notional,
		"reason":                   result.Reason,
		"simulation_only":          instance.ExecutionMode == Paper,
	})
	if err != nil {
		return err
	}

	tx, err := s.db.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)

	claimed, err := tx.Exec(c, `INSERT INTO strategy_evaluation_events(strategy_instance_id,event_id,status,created_at,completed_at) VALUES($1,$2,'COMMITTED',$3,$3) ON CONFLICT DO NOTHING`, instance.ID, action.CorrelationID, evaluatedAt)
	if err != nil {
		return err
	}
	if claimed.RowsAffected() != 1 {
		return ErrDuplicate
	}
	_, err = tx.Exec(c, `INSERT INTO risk_evaluations(id,user_id,financial_account_id,proposed_action_id,correlation_id,mandate_id,mandate_version,decision,approval_required,execution_mode,platform_execution_available,reason_codes,checks,evaluated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,false,$11,$12,$13)`, evaluation.ID, evaluation.UserID, evaluation.AccountID, action.ID, action.CorrelationID, action.MandateID, action.MandateVersion, evaluation.Decision, evaluation.ApprovalRequired, instance.ExecutionMode, reasonCodes, checks, evaluatedAt)
	if err != nil {
		return err
	}

	price := result.Price
	if price == nil {
		price = action.EstimatedPrice
	}
	notional := result.Notional
	if notional == nil {
		notional = &action.Notional
	}
	var executionID string
	err = tx.QueryRow(c, `INSERT INTO nonlive_execution_records(idempotency_key,user_id,strategy_instance_id,mandate_id,mandate_version,proposed_action_id,risk_evaluation_id,mode,status,symbol,instrument,side,quantity,price,notional,metadata,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$17) RETURNING id::text`, action.ID, instance.UserID, instance.ID, instance.AutomationMandateID, instance.MandateVersion, action.ID, evaluation.ID, instance.ExecutionMode, result.Status, action.Instrument, instrumentType, action.Side, action.Quantity, price, notional, executionMetadata, evaluatedAt).Scan(&executionID)
	if err != nil {
		return err
	}

	resultingState := instance.CurrentState
	stateChanged := instance.ExecutionMode == Paper && result.Status == SimulatedFilled
	if stateChanged {
		if evaluation.Decision != risk.Allow || (result.ExpectedState != ShortPutOpen && result.ExpectedState != ShortCallOpen) || action.Option == nil || result.Price == nil || result.Notional == nil {
			return ErrInvalid
		}
		premium, ok := new(big.Rat).SetString(*result.Notional)
		quantity, quantityOK := new(big.Rat).SetString(action.Quantity)
		if !ok || premium.Sign() < 0 || !quantityOK || quantity.Sign() <= 0 {
			return ErrInvalid
		}
		var portfolioID string
		err = tx.QueryRow(c, `UPDATE paper_portfolios SET cash=cash+$3,version=version+1,updated_at=$4 WHERE strategy_instance_id=$1 AND user_id=$2 RETURNING id::text`, instance.ID, instance.UserID, *result.Notional, evaluatedAt).Scan(&portfolioID)
		if err != nil {
			return err
		}
		positionMetadata, marshalErr := json.Marshal(map[string]any{"proposed_action_id": action.ID, "risk_evaluation_id": evaluation.ID, "simulation": true})
		if marshalErr != nil {
			return marshalErr
		}
		negativeQuantity := new(big.Rat).Neg(quantity).FloatString(10)
		_, err = tx.Exec(c, `INSERT INTO paper_positions(paper_portfolio_id,symbol,instrument,option_type,strike,expiration,quantity,average_price,metadata,updated_at) VALUES($1,$2,'OPTION',$3,$4,$5,$6,$7,$8,$9)`, portfolioID, action.Option.Underlying, action.Option.PutCall, action.Option.Strike, action.Option.Expiration, negativeQuantity, *result.Price, positionMetadata, evaluatedAt)
		if err != nil {
			return err
		}
		resultingState = result.ExpectedState
	}

	decisionType := fmt.Sprintf("%s_%s", evaluation.Decision, result.Status)
	_, err = tx.Exec(c, `INSERT INTO decision_journal_entries(user_id,financial_account_id,mandate_id,mandate_version,strategy_instance_id,strategy_state,source,decision_type,structured_rationale,proposed_action_id,risk_evaluation_id,execution_record_id,resulting_state,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, instance.UserID, instance.FinancialAccountID, instance.AutomationMandateID, instance.MandateVersion, instance.ID, instance.CurrentState, source, decisionType, decision.Rationale, action.ID, evaluation.ID, executionID, resultingState, evaluatedAt)
	if err != nil {
		return err
	}

	if stateChanged {
		var nextVersion int
		err = tx.QueryRow(c, `UPDATE strategy_instances SET current_state=$5,state_version=state_version+1,last_evaluated_at=$6,updated_at=$6 WHERE id=$1 AND user_id=$2 AND state_version=$3 AND current_state=$4 AND status='ACTIVE' RETURNING state_version`, instance.ID, instance.UserID, expectedVersion, instance.CurrentState, resultingState, evaluatedAt).Scan(&nextVersion)
		if err == pgx.ErrNoRows {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		transitionMetadata, marshalErr := json.Marshal(map[string]any{"event_id": action.CorrelationID, "mode": instance.ExecutionMode, "risk_decision": evaluation.Decision, "simulation": true})
		if marshalErr != nil {
			return marshalErr
		}
		_, err = tx.Exec(c, `INSERT INTO strategy_state_transitions(strategy_instance_id,previous_state,new_state,state_version,trigger,proposed_action_id,risk_evaluation_id,execution_record_id,metadata,occurred_at) VALUES($1,$2,$3,$4,'PAPER_SIMULATED_FILL',$5,$6,$7,$8,$9)`, instance.ID, instance.CurrentState, resultingState, nextVersion, action.ID, evaluation.ID, executionID, transitionMetadata, evaluatedAt)
	} else {
		var id string
		err = tx.QueryRow(c, `UPDATE strategy_instances SET last_evaluated_at=$5,updated_at=$5 WHERE id=$1 AND user_id=$2 AND state_version=$3 AND current_state=$4 AND status='ACTIVE' RETURNING id::text`, instance.ID, instance.UserID, expectedVersion, instance.CurrentState, evaluatedAt).Scan(&id)
		if err == pgx.ErrNoRows {
			return ErrConflict
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit(c)
}

func (s *PostgresStore) CommitAIAbstention(c context.Context, instance Instance, eventID string, rationale json.RawMessage, evaluatedAt time.Time) error {
	if instance.StrategyIdentifier != "ai_shadow" || (instance.ExecutionMode != Paper && instance.ExecutionMode != Shadow) || instance.CurrentState != AIMonitoring || !evaluationEventID.MatchString(eventID) || !json.Valid(rationale) || len(rationale) == 0 || rationale[0] != '{' || evaluatedAt.IsZero() {
		return ErrInvalid
	}
	tx, err := s.db.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	claimed, err := tx.Exec(c, `INSERT INTO strategy_evaluation_events(strategy_instance_id,event_id,status,created_at,completed_at) VALUES($1,$2,'COMMITTED',$3,$3) ON CONFLICT DO NOTHING`, instance.ID, eventID, evaluatedAt)
	if err != nil {
		return err
	}
	if claimed.RowsAffected() != 1 {
		return ErrDuplicate
	}
	_, err = tx.Exec(c, `INSERT INTO decision_journal_entries(user_id,financial_account_id,mandate_id,mandate_version,strategy_instance_id,strategy_state,source,decision_type,structured_rationale,resulting_state,created_at) VALUES($1,$2,$3,$4,$5,$6,'AI','ABSTAIN',$7,$6,$8)`, instance.UserID, instance.FinancialAccountID, instance.AutomationMandateID, instance.MandateVersion, instance.ID, instance.CurrentState, rationale, evaluatedAt)
	if err != nil {
		return err
	}
	var id string
	err = tx.QueryRow(c, `UPDATE strategy_instances SET last_evaluated_at=$4,updated_at=$4 WHERE id=$1 AND user_id=$2 AND state_version=$3 AND current_state='AI_MONITORING' AND status='ACTIVE' AND execution_mode=$5 RETURNING id::text`, instance.ID, instance.UserID, instance.StateVersion, evaluatedAt, instance.ExecutionMode).Scan(&id)
	if err == pgx.ErrNoRows {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	return tx.Commit(c)
}

func (s *PostgresStore) EvaluationFacts(c context.Context, instance Instance, evaluatedAt time.Time) (EvaluationFacts, error) {
	facts := EvaluationFacts{Breakers: []risk.CircuitBreaker{}, RecentActions: []risk.RecentAction{}}
	rows, err := s.db.Query(c, `SELECT id::text,scope,scope_id::text,state,reason,source,engaged_at FROM risk_circuit_breakers WHERE state='OPEN' AND (scope='GLOBAL' OR (scope='USER' AND scope_id=$1) OR (scope='ACCOUNT' AND scope_id=$2) OR (scope='AUTOMATION' AND scope_id=$3)) ORDER BY engaged_at`, instance.UserID, instance.FinancialAccountID, instance.AutomationMandateID)
	if err != nil {
		return EvaluationFacts{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var breaker risk.CircuitBreaker
		if err = rows.Scan(&breaker.ID, &breaker.Scope, &breaker.ScopeID, &breaker.State, &breaker.Reason, &breaker.Source, &breaker.EngagedAt); err != nil {
			return EvaluationFacts{}, err
		}
		facts.Breakers = append(facts.Breakers, breaker)
	}
	if err = rows.Err(); err != nil {
		return EvaluationFacts{}, err
	}
	dayStart := time.Date(evaluatedAt.UTC().Year(), evaluatedAt.UTC().Month(), evaluatedAt.UTC().Day(), 0, 0, 0, 0, time.UTC)
	if err = s.db.QueryRow(c, `SELECT count(*) FROM nonlive_execution_records WHERE user_id=$1 AND strategy_instance_id=$2 AND created_at >= $3 AND created_at < $4`, instance.UserID, instance.ID, dayStart, dayStart.Add(24*time.Hour)).Scan(&facts.ActionsToday); err != nil {
		return EvaluationFacts{}, err
	}
	if instance.StrategyIdentifier == "ai_shadow" && (instance.ExecutionMode == Paper || instance.ExecutionMode == Shadow) {
		if instance.ExecutionMode == Shadow {
			var reconciliation risk.ReconciliationSnapshot
			reconciliationErr := s.db.QueryRow(c, `SELECT financial_account_id::text,comparison_status,balances_status,positions_status,autonomy_signal,autonomy_enforcement_active,blocks_new_actions,change_count,blocking_change_count,observed_at FROM portfolio_reconciliations WHERE user_id=$1 AND financial_account_id=$2 ORDER BY observed_at DESC,id DESC LIMIT 1`, instance.UserID, instance.FinancialAccountID).Scan(
				&reconciliation.AccountID, &reconciliation.ComparisonStatus, &reconciliation.BalancesStatus,
				&reconciliation.PositionsStatus, &reconciliation.AutonomySignal,
				&reconciliation.AutonomyEnforcementActive, &reconciliation.BlocksNewActions,
				&reconciliation.ChangeCount, &reconciliation.BlockingChangeCount, &reconciliation.ObservedAt,
			)
			if reconciliationErr != nil && !errors.Is(reconciliationErr, pgx.ErrNoRows) {
				return EvaluationFacts{}, reconciliationErr
			}
			if reconciliationErr == nil {
				facts.Reconciliation = &reconciliation
			}
		}

		recentStatus := "SIMULATED_FILLED"
		if instance.ExecutionMode == Shadow {
			recentStatus = "WOULD_HAVE_SUBMITTED"
		}
		recentRows, recentErr := s.db.Query(c, `SELECT symbol,side,created_at FROM nonlive_execution_records WHERE strategy_instance_id=$1 AND user_id=$2 AND mode=$3 AND status=$4 AND created_at >= $5 AND created_at < $6 ORDER BY created_at DESC LIMIT 101`, instance.ID, instance.UserID, instance.ExecutionMode, recentStatus, evaluatedAt.Add(-risk.AIRepeatActionCooldown), evaluatedAt)
		if recentErr != nil {
			return EvaluationFacts{}, recentErr
		}
		defer recentRows.Close()
		for recentRows.Next() {
			var action risk.RecentAction
			if err = recentRows.Scan(&action.Instrument, &action.Side, &action.OccurredAt); err != nil {
				return EvaluationFacts{}, err
			}
			facts.RecentActions = append(facts.RecentActions, action)
			if len(facts.RecentActions) > 100 {
				return EvaluationFacts{}, ErrInvalid
			}
		}
		if err = recentRows.Err(); err != nil {
			return EvaluationFacts{}, err
		}

		decisionRows, decisionErr := s.db.Query(c, `SELECT decision_type,structured_rationale,created_at FROM decision_journal_entries WHERE strategy_instance_id=$1 AND user_id=$2 AND source='AI' AND decision_type IN ('ABSTAIN','ALLOW_WOULD_HAVE_SUBMITTED','ALLOW_SIMULATED_FILLED','ALLOW_SIMULATED_REJECTED','DENY_RISK_DENIED') AND created_at >= $3 AND created_at < $4 ORDER BY created_at DESC,id DESC LIMIT $5`, instance.ID, instance.UserID, evaluatedAt.Add(-aiDecisionMemoryWindow), evaluatedAt, aiDecisionMemoryLimit)
		if decisionErr != nil {
			return EvaluationFacts{}, decisionErr
		}
		defer decisionRows.Close()
		for decisionRows.Next() {
			var decisionType string
			var rationale json.RawMessage
			var occurredAt time.Time
			if err = decisionRows.Scan(&decisionType, &rationale, &occurredAt); err != nil {
				return EvaluationFacts{}, err
			}
			var summary struct {
				Decision string `json:"decision"`
				Symbol   string `json:"symbol"`
				Side     string `json:"side"`
			}
			if err = json.Unmarshal(rationale, &summary); err != nil {
				return EvaluationFacts{}, ErrInvalid
			}
			disposition := ""
			switch decisionType {
			case "ABSTAIN":
				disposition = "ABSTAINED"
			case "ALLOW_WOULD_HAVE_SUBMITTED":
				disposition = "WOULD_HAVE_SUBMITTED"
			case "ALLOW_SIMULATED_FILLED":
				disposition = "SIMULATED_FILLED"
			case "ALLOW_SIMULATED_REJECTED":
				disposition = "SIMULATED_REJECTED"
			case "DENY_RISK_DENIED":
				disposition = "HELD_BY_CONTROLS"
			}
			memory := neural.ShadowRecentDecision{Decision: summary.Decision, Symbol: summary.Symbol, Side: summary.Side, Disposition: disposition, OccurredAt: occurredAt.UTC()}
			if !validAIRecentDecision(memory) || occurredAt.Before(evaluatedAt.Add(-aiDecisionMemoryWindow)) || !occurredAt.Before(evaluatedAt) {
				return EvaluationFacts{}, ErrInvalid
			}
			facts.RecentDecisions = append(facts.RecentDecisions, memory)
		}
		if err = decisionRows.Err(); err != nil {
			return EvaluationFacts{}, err
		}
	}
	if instance.ExecutionMode != Paper {
		return facts, nil
	}

	var portfolioID, cash string
	if err = s.db.QueryRow(c, `SELECT id::text,cash::text FROM paper_portfolios WHERE strategy_instance_id=$1 AND user_id=$2`, instance.ID, instance.UserID).Scan(&portfolioID, &cash); err != nil {
		return EvaluationFacts{}, err
	}
	positionRows, err := s.db.Query(c, `SELECT symbol,instrument,COALESCE(option_type,''),COALESCE(strike::text,''),quantity::text,average_price::text FROM paper_positions WHERE paper_portfolio_id=$1 ORDER BY symbol,instrument,expiration,strike`, portfolioID)
	if err != nil {
		return EvaluationFacts{}, err
	}
	defer positionRows.Close()
	positions := []Position{}
	exposureBySymbol := map[string]*big.Rat{}
	availableBySymbol := map[string]*big.Rat{}
	totalExposure := new(big.Rat)
	for positionRows.Next() {
		var symbol, instrument, optionType, strikeText, quantityText, averagePriceText string
		if err = positionRows.Scan(&symbol, &instrument, &optionType, &strikeText, &quantityText, &averagePriceText); err != nil {
			return EvaluationFacts{}, err
		}
		quantity, quantityOK := new(big.Rat).SetString(quantityText)
		if !quantityOK {
			return EvaluationFacts{}, ErrInvalid
		}
		quantity.Abs(quantity)
		priceText := averagePriceText
		multiplier := big.NewRat(1, 1)
		if instrument == "OPTION" {
			priceText = strikeText
			multiplier = big.NewRat(100, 1)
		}
		price, priceOK := new(big.Rat).SetString(priceText)
		if !priceOK || price.Sign() < 0 {
			return EvaluationFacts{}, ErrInvalid
		}
		exposure := new(big.Rat).Mul(quantity, new(big.Rat).Mul(price, multiplier))
		symbol = strings.ToUpper(symbol)
		if exposureBySymbol[symbol] == nil {
			exposureBySymbol[symbol] = new(big.Rat)
		}
		exposureBySymbol[symbol].Add(exposureBySymbol[symbol], exposure)
		totalExposure.Add(totalExposure, exposure)
		if availableBySymbol[symbol] == nil {
			availableBySymbol[symbol] = new(big.Rat)
		}
		if (instrument == "EQUITY" || instrument == "CRYPTO") && !strings.HasPrefix(quantityText, "-") {
			availableBySymbol[symbol].Add(availableBySymbol[symbol], quantity)
		}
		positions = append(positions, Position{Symbol: symbol, Instrument: instrument, Quantity: quantityText, AveragePrice: averagePriceText})
	}
	if err = positionRows.Err(); err != nil {
		return EvaluationFacts{}, err
	}
	symbols := make([]string, 0, len(exposureBySymbol))
	for symbol := range exposureBySymbol {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	riskPositions := make([]risk.Position, 0, len(symbols))
	for _, symbol := range symbols {
		riskPositions = append(riskPositions, risk.Position{Instrument: symbol, Exposure: exposureBySymbol[symbol].FloatString(10), AvailableQuantity: availableBySymbol[symbol].FloatString(10)})
	}
	facts.Paper = &PaperEvaluationFacts{Cash: cash, CurrentExposure: totalExposure.FloatString(10), Positions: positions, RiskPositions: riskPositions}
	return facts, nil
}
