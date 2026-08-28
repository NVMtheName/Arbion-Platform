package strategy

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const shadowEvidenceReviewColumns = `id::text,strategy_instance_id::text,mandate_id::text,mandate_version,encode(evidence_hash,'hex'),gate_status,one_hour_sample_size,twenty_four_hour_sample_size,evidence_window_hours,schedule_healthy,last_schedule_status,consecutive_schedule_failures,execution_boundary,live_execution_available,review_scope,mfa_method,reviewed_at,created_at`

func scanShadowEvidenceReview(row pgx.Row) (review ShadowEvidenceReview, err error) {
	err = row.Scan(
		&review.ID,
		&review.StrategyInstanceID,
		&review.MandateID,
		&review.MandateVersion,
		&review.EvidenceFingerprint,
		&review.GateStatus,
		&review.OneHourSampleSize,
		&review.TwentyFourHourSampleSize,
		&review.EvidenceWindowHours,
		&review.ScheduleHealthy,
		&review.LastScheduleStatus,
		&review.ConsecutiveScheduleFailures,
		&review.ExecutionBoundary,
		&review.LiveExecutionAvailable,
		&review.ReviewScope,
		&review.MFAMethod,
		&review.ReviewedAt,
		&review.CreatedAt,
	)
	return review, err
}

func (s *PostgresStore) CreateShadowEvidenceReview(ctx context.Context, userID string, review ShadowEvidenceReview) (ShadowEvidenceReview, error) {
	created, err := scanShadowEvidenceReview(s.db.QueryRow(ctx, `INSERT INTO shadow_evidence_reviews(
		user_id,strategy_instance_id,mandate_id,mandate_version,evidence_hash,gate_status,
		one_hour_sample_size,twenty_four_hour_sample_size,evidence_window_hours,
		schedule_healthy,last_schedule_status,consecutive_schedule_failures,
		execution_boundary,live_execution_available,review_scope,mfa_method,reviewed_at)
		VALUES($1,$2,$3,$4,decode($5,'hex'),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING `+shadowEvidenceReviewColumns,
		userID,
		review.StrategyInstanceID,
		review.MandateID,
		review.MandateVersion,
		review.EvidenceFingerprint,
		review.GateStatus,
		review.OneHourSampleSize,
		review.TwentyFourHourSampleSize,
		review.EvidenceWindowHours,
		review.ScheduleHealthy,
		review.LastScheduleStatus,
		review.ConsecutiveScheduleFailures,
		review.ExecutionBoundary,
		review.LiveExecutionAvailable,
		review.ReviewScope,
		review.MFAMethod,
		review.ReviewedAt,
	))
	if err == nil {
		return created, nil
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		latest, latestErr := s.LatestShadowEvidenceReview(ctx, userID, review.StrategyInstanceID)
		if latestErr == nil && latest != nil && latest.EvidenceFingerprint == review.EvidenceFingerprint {
			return *latest, nil
		}
		return ShadowEvidenceReview{}, ErrConflict
	}
	return ShadowEvidenceReview{}, err
}

func (s *PostgresStore) LatestShadowEvidenceReview(ctx context.Context, userID, instanceID string) (*ShadowEvidenceReview, error) {
	review, err := scanShadowEvidenceReview(s.db.QueryRow(ctx, `SELECT `+shadowEvidenceReviewColumns+`
		FROM shadow_evidence_reviews
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

func (s *PostgresStore) ShadowEvidenceReviews(ctx context.Context, userID, instanceID string, limit int, cursor *ShadowEvidenceReviewCursor) ([]ShadowEvidenceReview, error) {
	query := `SELECT ` + shadowEvidenceReviewColumns + `
		FROM shadow_evidence_reviews
		WHERE user_id=$1 AND strategy_instance_id=$2
		ORDER BY reviewed_at DESC,id DESC
		LIMIT $3`
	args := []any{userID, instanceID, limit}
	if cursor != nil {
		query = `SELECT ` + shadowEvidenceReviewColumns + `
			FROM shadow_evidence_reviews
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
	reviews := []ShadowEvidenceReview{}
	for rows.Next() {
		review, scanErr := scanShadowEvidenceReview(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		reviews = append(reviews, review)
	}
	return reviews, rows.Err()
}
