package strategy

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const paperEvidenceReviewColumns = `id::text,strategy_instance_id::text,financial_account_id::text,mandate_id::text,mandate_version,encode(evidence_hash,'hex'),gate_status,evidence_started_at,evidence_eligible_at,evidence_as_of,evidence_window_hours,decision_count,portfolio_version,portfolio_updated_at,latest_checkpoint_run_id::text,latest_checkpoint_as_of,scheduler_sample_count,scheduler_success_count,scheduler_failure_count,last_schedule_status,consecutive_schedule_failures,route_continuity_status,input_coverage_status,input_freshness_status,ledger_contract_status,no_live_safety_status,execution_boundary,review_scope,grants_authority,live_promotion_available,mfa_method,reviewed_at,created_at`

func scanPaperEvidenceReview(row pgx.Row) (review PaperEvidenceReview, err error) {
	err = row.Scan(
		&review.ID, &review.StrategyInstanceID, &review.FinancialAccountID, &review.MandateID, &review.MandateVersion,
		&review.EvidenceFingerprint, &review.GateStatus, &review.EvidenceStartedAt, &review.EvidenceEligibleAt, &review.EvidenceAsOf,
		&review.EvidenceWindowHours, &review.DecisionCount, &review.PortfolioVersion, &review.PortfolioUpdatedAt,
		&review.LatestCheckpointRunID, &review.LatestCheckpointAsOf,
		&review.SchedulerSampleCount, &review.SchedulerSuccessCount, &review.SchedulerFailureCount,
		&review.LastScheduleStatus, &review.ConsecutiveScheduleFailures, &review.RouteContinuityStatus,
		&review.InputCoverageStatus, &review.InputFreshnessStatus, &review.LedgerContractStatus, &review.NoLiveSafetyStatus,
		&review.ExecutionBoundary, &review.ReviewScope, &review.GrantsAuthority, &review.LivePromotionAvailable,
		&review.MFAMethod, &review.ReviewedAt, &review.CreatedAt,
	)
	return review, err
}

func (s *PostgresStore) CreatePaperEvidenceReview(ctx context.Context, userID string, review PaperEvidenceReview) (PaperEvidenceReview, error) {
	created, err := scanPaperEvidenceReview(s.db.QueryRow(ctx, `INSERT INTO paper_evidence_reviews(
		user_id,strategy_instance_id,financial_account_id,mandate_id,mandate_version,evidence_hash,gate_status,
		evidence_started_at,evidence_eligible_at,evidence_as_of,evidence_window_hours,decision_count,portfolio_version,portfolio_updated_at,
		latest_checkpoint_run_id,latest_checkpoint_as_of,scheduler_sample_count,scheduler_success_count,scheduler_failure_count,
		last_schedule_status,consecutive_schedule_failures,route_continuity_status,input_coverage_status,input_freshness_status,
		ledger_contract_status,no_live_safety_status,execution_boundary,review_scope,grants_authority,live_promotion_available,mfa_method,reviewed_at)
		VALUES($1,$2,$3,$4,$5,decode($6,'hex'),$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32)
		RETURNING `+paperEvidenceReviewColumns,
		userID, review.StrategyInstanceID, review.FinancialAccountID, review.MandateID, review.MandateVersion,
		review.EvidenceFingerprint, review.GateStatus, review.EvidenceStartedAt, review.EvidenceEligibleAt, review.EvidenceAsOf,
		review.EvidenceWindowHours, review.DecisionCount, review.PortfolioVersion, review.PortfolioUpdatedAt,
		review.LatestCheckpointRunID, review.LatestCheckpointAsOf,
		review.SchedulerSampleCount, review.SchedulerSuccessCount, review.SchedulerFailureCount,
		review.LastScheduleStatus, review.ConsecutiveScheduleFailures, review.RouteContinuityStatus,
		review.InputCoverageStatus, review.InputFreshnessStatus, review.LedgerContractStatus, review.NoLiveSafetyStatus,
		review.ExecutionBoundary, review.ReviewScope, review.GrantsAuthority, review.LivePromotionAvailable, review.MFAMethod, review.ReviewedAt,
	))
	if err == nil {
		return created, nil
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		latest, latestErr := s.LatestPaperEvidenceReview(ctx, userID, review.StrategyInstanceID)
		if latestErr == nil && latest != nil && latest.EvidenceFingerprint == review.EvidenceFingerprint {
			return *latest, nil
		}
		return PaperEvidenceReview{}, ErrConflict
	}
	if errors.As(err, &postgresError) && (postgresError.Code == "23503" || postgresError.Code == "23514" || postgresError.Code == "P0001") {
		return PaperEvidenceReview{}, ErrEvidenceSnapshotChanged
	}
	return PaperEvidenceReview{}, err
}

func (s *PostgresStore) LatestPaperEvidenceReview(ctx context.Context, userID, instanceID string) (*PaperEvidenceReview, error) {
	review, err := scanPaperEvidenceReview(s.db.QueryRow(ctx, `SELECT `+paperEvidenceReviewColumns+`
		FROM paper_evidence_reviews
		WHERE user_id=$1 AND strategy_instance_id=$2
		ORDER BY reviewed_at DESC,id DESC
		LIMIT 1`, userID, instanceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &review, nil
}

func (s *PostgresStore) PaperEvidenceReviews(ctx context.Context, userID, instanceID string, limit int, cursor *PaperEvidenceReviewCursor) ([]PaperEvidenceReview, error) {
	query := `SELECT ` + paperEvidenceReviewColumns + `
		FROM paper_evidence_reviews
		WHERE user_id=$1 AND strategy_instance_id=$2
		ORDER BY reviewed_at DESC,id DESC
		LIMIT $3`
	args := []any{userID, instanceID, limit}
	if cursor != nil {
		query = `SELECT ` + paperEvidenceReviewColumns + `
			FROM paper_evidence_reviews
			WHERE user_id=$1 AND strategy_instance_id=$2
			  AND (reviewed_at,id) < ($3,$4::uuid)
			ORDER BY reviewed_at DESC,id DESC
			LIMIT $5`
		args = []any{userID, instanceID, cursor.ReviewedAt, cursor.ID, limit}
	}
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reviews := []PaperEvidenceReview{}
	for rows.Next() {
		review, scanErr := scanPaperEvidenceReview(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		reviews = append(reviews, review)
	}
	return reviews, rows.Err()
}
