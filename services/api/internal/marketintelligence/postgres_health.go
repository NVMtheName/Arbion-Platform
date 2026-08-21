package marketintelligence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type healthDatabase interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type PostgresHealthStore struct{ db healthDatabase }

func NewPostgresHealthStore(db healthDatabase) *PostgresHealthStore {
	return &PostgresHealthStore{db: db}
}

func (store *PostgresHealthStore) RecordOutcome(ctx context.Context, outcome HealthOutcome) error {
	if store == nil || store.db == nil || !validHealthOutcome(outcome) {
		return ErrHealthHistoryUnavailable
	}
	bucketStart := outcome.ObservedAt.UTC().Truncate(healthStorageInterval)
	successes, failures := int64(0), int64(0)
	var failureCategory any
	if outcome.State == Verified {
		successes = 1
	} else {
		failures = 1
		failureCategory = outcome.FailureCategory
	}
	_, err := store.db.Exec(ctx, `WITH pruned AS (
		DELETE FROM market_source_health_buckets
		WHERE bucket_started_at < $4::timestamptz - interval '30 days'
	)
	INSERT INTO market_source_health_buckets(
		source_id,capability,bucket_started_at,completed_attempts,successes,failures,last_state,failure_category,last_observed_at
	) VALUES($1,$2,$3,1,$5,$6,$7,$8,$4)
	ON CONFLICT(source_id,capability,bucket_started_at) DO UPDATE SET
		completed_attempts=market_source_health_buckets.completed_attempts+1,
		successes=market_source_health_buckets.successes+EXCLUDED.successes,
		failures=market_source_health_buckets.failures+EXCLUDED.failures,
		last_state=CASE WHEN EXCLUDED.last_observed_at >= market_source_health_buckets.last_observed_at THEN EXCLUDED.last_state ELSE market_source_health_buckets.last_state END,
		failure_category=CASE WHEN EXCLUDED.last_observed_at >= market_source_health_buckets.last_observed_at THEN EXCLUDED.failure_category ELSE market_source_health_buckets.failure_category END,
		last_observed_at=GREATEST(market_source_health_buckets.last_observed_at,EXCLUDED.last_observed_at),
		updated_at=now()`, outcome.SourceID, outcome.Capability, bucketStart, outcome.ObservedAt.UTC(), successes, failures, outcome.State, failureCategory)
	return err
}

func (store *PostgresHealthStore) Hourly(ctx context.Context, start, end time.Time) ([]HealthBucket, error) {
	if store == nil || store.db == nil || start.IsZero() || !end.After(start) || end.Sub(start) > 31*24*time.Hour {
		return nil, ErrHealthHistoryUnavailable
	}
	rows, err := store.db.Query(ctx, `SELECT source_id,capability,date_trunc('hour',bucket_started_at) AS interval_started_at,
		max(last_observed_at),sum(completed_attempts)::bigint,sum(successes)::bigint,sum(failures)::bigint,
		(array_agg(last_state ORDER BY last_observed_at DESC,bucket_started_at DESC))[1],
		COALESCE((array_agg(failure_category ORDER BY last_observed_at DESC,bucket_started_at DESC))[1],'')
	FROM market_source_health_buckets
	WHERE bucket_started_at >= $1 AND bucket_started_at < $2
	GROUP BY source_id,capability,date_trunc('hour',bucket_started_at)
	ORDER BY interval_started_at,source_id,capability`, start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	buckets := make([]HealthBucket, 0)
	for rows.Next() {
		var bucket HealthBucket
		var attempts, successes, failures int64
		if err = rows.Scan(&bucket.SourceID, &bucket.Capability, &bucket.IntervalStarted, &bucket.LastObservedAt, &attempts, &successes, &failures, &bucket.LastState, &bucket.FailureCategory); err != nil {
			return nil, err
		}
		if attempts < 1 || successes < 0 || failures < 0 {
			return nil, errors.New("invalid market health history counts")
		}
		bucket.CompletedAttempts = uint64(attempts)
		bucket.Successes = uint64(successes)
		bucket.Failures = uint64(failures)
		if !validHealthOutcome(HealthOutcome{SourceID: bucket.SourceID, Capability: bucket.Capability, State: bucket.LastState, FailureCategory: bucket.FailureCategory, ObservedAt: bucket.LastObservedAt}) || bucket.CompletedAttempts != bucket.Successes+bucket.Failures {
			return nil, errors.New("invalid market health history row")
		}
		buckets = append(buckets, bucket)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return buckets, nil
}
