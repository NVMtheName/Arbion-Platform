package strategy

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) DueShadowOutcomes(ctx context.Context, instance Instance, nowTime time.Time) ([]ShadowOutcomeCandidate, error) {
	rows, err := s.db.Query(ctx, `SELECT x.id::text,h.horizon,x.symbol,x.side,x.quantity::text,x.price::text,x.created_at
		FROM nonlive_execution_records x
		JOIN strategy_instances i ON i.id=x.strategy_instance_id AND i.user_id=x.user_id
		CROSS JOIN (VALUES ('ONE_HOUR'::text,interval '1 hour'),('TWENTY_FOUR_HOURS'::text,interval '24 hours')) h(horizon,minimum_age)
		LEFT JOIN shadow_execution_outcomes o ON o.execution_record_id=x.id AND o.horizon=h.horizon
		WHERE x.user_id=$1 AND x.strategy_instance_id=$2 AND x.mode='SHADOW' AND x.status='WOULD_HAVE_SUBMITTED'
		  AND i.strategy_identifier='ai_shadow' AND i.execution_mode='SHADOW'
		  AND x.price IS NOT NULL AND x.created_at <= $3::timestamptz-h.minimum_age AND o.id IS NULL
		ORDER BY x.created_at,h.minimum_age LIMIT 32`, instance.UserID, instance.ID, nowTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := []ShadowOutcomeCandidate{}
	for rows.Next() {
		var candidate ShadowOutcomeCandidate
		if err = rows.Scan(&candidate.ExecutionRecordID, &candidate.Horizon, &candidate.Symbol, &candidate.Side, &candidate.Quantity, &candidate.EntryPrice, &candidate.CreatedAt); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (s *PostgresStore) RecordShadowOutcome(ctx context.Context, instance Instance, outcome ShadowOutcome) error {
	var id string
	err := s.db.QueryRow(ctx, `INSERT INTO shadow_execution_outcomes(user_id,strategy_instance_id,execution_record_id,horizon,symbol,side,quantity,entry_price,observed_price,directional_change_usd,directional_change_percent,pricing_basis,market_feed,market_quality,market_observed_at,evaluated_at,elapsed_seconds)
		SELECT x.user_id,x.strategy_instance_id,x.id,$4,x.symbol,x.side,x.quantity,x.price,$5,$6,$7,$8,$9,$10,$11,$12,$13
		FROM nonlive_execution_records x
		JOIN strategy_instances i ON i.id=x.strategy_instance_id AND i.user_id=x.user_id
		WHERE x.id=$1 AND x.user_id=$2 AND x.strategy_instance_id=$3 AND x.mode='SHADOW' AND x.status='WOULD_HAVE_SUBMITTED'
		  AND i.strategy_identifier='ai_shadow' AND i.execution_mode='SHADOW'
		  AND x.symbol=$14 AND x.side=$15 AND x.quantity=$16 AND x.price=$17
		ON CONFLICT (execution_record_id,horizon) DO NOTHING
		RETURNING id::text`, outcome.ExecutionRecordID, instance.UserID, instance.ID, outcome.Horizon, outcome.ObservedPrice, outcome.DirectionalChangeUSD, outcome.DirectionalChangePercent, outcome.PricingBasis, outcome.MarketFeed, outcome.MarketQuality, outcome.MarketObservedAt, outcome.EvaluatedAt, outcome.ElapsedSeconds, outcome.Symbol, outcome.Side, outcome.Quantity, outcome.EntryPrice).Scan(&id)
	if err == pgx.ErrNoRows {
		var exists, exact bool
		checkErr := s.db.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM shadow_execution_outcomes
			WHERE execution_record_id=$1 AND horizon=$2 AND user_id=$3 AND strategy_instance_id=$4
		), EXISTS (
			SELECT 1 FROM shadow_execution_outcomes
			WHERE execution_record_id=$1 AND horizon=$2 AND user_id=$3 AND strategy_instance_id=$4
			  AND symbol=$5 AND side=$6 AND quantity=$7 AND entry_price=$8 AND observed_price=$9
			  AND directional_change_usd=$10 AND directional_change_percent=$11 AND pricing_basis=$12
			  AND market_feed=$13 AND market_quality=$14 AND market_observed_at=$15
			  AND evaluated_at=$16 AND elapsed_seconds=$17
		)`, outcome.ExecutionRecordID, outcome.Horizon, instance.UserID, instance.ID,
			outcome.Symbol, outcome.Side, outcome.Quantity, outcome.EntryPrice, outcome.ObservedPrice,
			outcome.DirectionalChangeUSD, outcome.DirectionalChangePercent, outcome.PricingBasis,
			outcome.MarketFeed, outcome.MarketQuality, outcome.MarketObservedAt, outcome.EvaluatedAt,
			outcome.ElapsedSeconds).Scan(&exists, &exact)
		if checkErr != nil {
			return checkErr
		}
		if exact {
			return nil
		}
		if exists {
			return ErrConflict
		}
		return ErrInvalid
	}
	return err
}

func (s *PostgresStore) ShadowOutcomes(ctx context.Context, userID, instanceID string) ([]ShadowOutcome, error) {
	rows, err := s.db.Query(ctx, `SELECT o.id::text,o.execution_record_id::text,o.horizon,o.symbol,o.side,o.quantity::text,o.entry_price::text,o.observed_price::text,o.directional_change_usd::text,o.directional_change_percent::text,o.pricing_basis,o.market_feed,o.market_quality,o.market_observed_at,o.evaluated_at,o.elapsed_seconds
		FROM shadow_execution_outcomes o
		JOIN strategy_instances i ON i.id=o.strategy_instance_id AND i.user_id=o.user_id
		WHERE o.strategy_instance_id=$1 AND o.user_id=$2 AND i.strategy_identifier='ai_shadow' AND i.execution_mode='SHADOW'
		ORDER BY o.evaluated_at DESC,o.id DESC
		LIMIT 200`, instanceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	outcomes := []ShadowOutcome{}
	for rows.Next() {
		var outcome ShadowOutcome
		if err = rows.Scan(&outcome.ID, &outcome.ExecutionRecordID, &outcome.Horizon, &outcome.Symbol, &outcome.Side, &outcome.Quantity, &outcome.EntryPrice, &outcome.ObservedPrice, &outcome.DirectionalChangeUSD, &outcome.DirectionalChangePercent, &outcome.PricingBasis, &outcome.MarketFeed, &outcome.MarketQuality, &outcome.MarketObservedAt, &outcome.EvaluatedAt, &outcome.ElapsedSeconds); err != nil {
			return nil, err
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, rows.Err()
}

func (s *PostgresStore) ShadowScorecard(ctx context.Context, userID, instanceID string) (ShadowScorecard, error) {
	rows, err := s.db.Query(ctx, `WITH horizons(horizon,ordinal) AS (
		VALUES ('ONE_HOUR'::text,1),('TWENTY_FOUR_HOURS'::text,2)
	)
	SELECT i.id::text,h.horizon,
		COUNT(o.id)::integer,
		(COUNT(*) FILTER (WHERE o.directional_change_percent>0))::integer,
		(COUNT(*) FILTER (WHERE o.directional_change_percent<0))::integer,
		(COUNT(*) FILTER (WHERE o.directional_change_percent=0))::integer,
		COALESCE(round(((COUNT(*) FILTER (WHERE o.directional_change_percent>0))::numeric / NULLIF(COUNT(o.id),0)::numeric)*100,10)::text,''),
		COALESCE(round(avg(o.directional_change_percent),10)::text,''),
		min(o.evaluated_at),max(o.evaluated_at)
	FROM strategy_instances i
	CROSS JOIN horizons h
	LEFT JOIN shadow_execution_outcomes o
	  ON o.strategy_instance_id=i.id AND o.user_id=i.user_id AND o.horizon=h.horizon
	WHERE i.id=$1 AND i.user_id=$2 AND i.strategy_identifier='ai_shadow' AND i.execution_mode='SHADOW'
	GROUP BY i.id,h.horizon,h.ordinal
	ORDER BY h.ordinal`, instanceID, userID)
	if err != nil {
		return ShadowScorecard{}, err
	}
	defer rows.Close()
	scorecard := ShadowScorecard{StrategyInstanceID: instanceID, Horizons: []ShadowHorizonScore{}}
	for rows.Next() {
		var score ShadowHorizonScore
		var favorableRate, averageChange string
		if err = rows.Scan(&scorecard.StrategyInstanceID, &score.Horizon, &score.SampleSize,
			&score.FavorableMarks, &score.UnfavorableMarks, &score.FlatMarks,
			&favorableRate, &averageChange, &score.FirstEvaluatedAt, &score.LastEvaluatedAt); err != nil {
			return ShadowScorecard{}, err
		}
		if favorableRate != "" {
			score.FavorableRatePercent = &favorableRate
		}
		if averageChange != "" {
			score.AverageDirectionalChangePercent = &averageChange
		}
		score.Interpretation = "INSUFFICIENT_SAMPLE"
		if score.SampleSize >= ShadowScorecardMinimumSample {
			score.Interpretation = "OBSERVATIONAL"
		}
		score.MinimumSampleForObservationalLabel = ShadowScorecardMinimumSample
		scorecard.TotalMarks += score.SampleSize
		scorecard.Horizons = append(scorecard.Horizons, score)
	}
	if err = rows.Err(); err != nil {
		return ShadowScorecard{}, err
	}
	if len(scorecard.Horizons) == 0 {
		return ShadowScorecard{}, ErrNotFound
	}
	return scorecard, nil
}
