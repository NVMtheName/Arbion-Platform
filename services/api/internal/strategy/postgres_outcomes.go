package strategy

import (
	"context"
	"fmt"
	"strings"
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

const shadowOutcomeColumns = `o.id::text,o.execution_record_id::text,o.horizon,o.symbol,o.side,o.quantity::text,o.entry_price::text,o.observed_price::text,o.directional_change_usd::text,o.directional_change_percent::text,o.pricing_basis,o.market_feed,o.market_quality,o.market_observed_at,o.evaluated_at,o.elapsed_seconds`

func scanShadowOutcome(row pgx.Row) (outcome ShadowOutcome, err error) {
	err = row.Scan(&outcome.ID, &outcome.ExecutionRecordID, &outcome.Horizon, &outcome.Symbol, &outcome.Side, &outcome.Quantity, &outcome.EntryPrice, &outcome.ObservedPrice, &outcome.DirectionalChangeUSD, &outcome.DirectionalChangePercent, &outcome.PricingBasis, &outcome.MarketFeed, &outcome.MarketQuality, &outcome.MarketObservedAt, &outcome.EvaluatedAt, &outcome.ElapsedSeconds)
	return outcome, err
}

func collectShadowOutcomes(rows pgx.Rows) ([]ShadowOutcome, error) {
	defer rows.Close()
	outcomes := []ShadowOutcome{}
	for rows.Next() {
		outcome, err := scanShadowOutcome(rows)
		if err != nil {
			return nil, err
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, rows.Err()
}

func (s *PostgresStore) ShadowOutcomes(ctx context.Context, userID, instanceID string) ([]ShadowOutcome, error) {
	rows, err := s.db.Query(ctx, `SELECT `+shadowOutcomeColumns+`
		FROM shadow_execution_outcomes o
		JOIN strategy_instances i ON i.id=o.strategy_instance_id AND i.user_id=o.user_id
		WHERE o.strategy_instance_id=$1 AND o.user_id=$2 AND i.strategy_identifier='ai_shadow' AND i.execution_mode='SHADOW'
		ORDER BY o.evaluated_at DESC,o.id DESC
		LIMIT 200`, instanceID, userID)
	if err != nil {
		return nil, err
	}
	return collectShadowOutcomes(rows)
}

func (s *PostgresStore) ShadowOutcomesForExecutions(ctx context.Context, userID, instanceID string, executionIDs []string) ([]ShadowOutcome, error) {
	if len(executionIDs) == 0 {
		return []ShadowOutcome{}, nil
	}
	rows, err := s.db.Query(ctx, `SELECT `+shadowOutcomeColumns+`
		FROM shadow_execution_outcomes o
		JOIN strategy_instances i ON i.id=o.strategy_instance_id AND i.user_id=o.user_id
		WHERE o.strategy_instance_id=$1 AND o.user_id=$2
		  AND i.strategy_identifier='ai_shadow' AND i.execution_mode='SHADOW'
		  AND o.execution_record_id=ANY(string_to_array($3, ',')::uuid[])
		ORDER BY o.evaluated_at DESC,o.id DESC`, instanceID, userID, strings.Join(executionIDs, ","))
	if err != nil {
		return nil, err
	}
	return collectShadowOutcomes(rows)
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
	scorecard.Behavior, err = s.shadowBehaviorScore(ctx, userID, instanceID)
	if err != nil {
		return ShadowScorecard{}, err
	}
	var lastScheduleStatus string
	var consecutiveFailures int
	scheduleObserved := true
	err = s.db.QueryRow(ctx, `SELECT COALESCE(last_status,''),consecutive_failures
		FROM nonlive_strategy_schedules
		WHERE strategy_instance_id=$1 AND user_id=$2`, instanceID, userID).Scan(&lastScheduleStatus, &consecutiveFailures)
	if err == pgx.ErrNoRows {
		scheduleObserved = false
	} else if err != nil {
		return ShadowScorecard{}, err
	} else if lastScheduleStatus == "" {
		scheduleObserved = false
	}
	scorecard.EvidenceGate = buildShadowEvidenceGate(scorecard, scheduleObserved, lastScheduleStatus, consecutiveFailures)
	return scorecard, nil
}

func (s *PostgresStore) shadowBehaviorScore(ctx context.Context, userID, instanceID string) (ShadowBehaviorScore, error) {
	rows, err := s.db.Query(ctx, `WITH base AS (
		SELECT d.id,d.execution_record_id,d.decision_type,d.created_at,
			(d.decision_type='DENY_RISK_DENIED' AND COALESCE(r.reason_codes,'[]'::jsonb) ? 'REPEAT_ACTION_COOLDOWN_ACTIVE') AS repeat_action_cooldown_hold,
			CASE WHEN COALESCE(d.structured_rationale->>'ai_provider','')<>''
			       AND COALESCE(d.structured_rationale->>'model_id','')<>''
			       AND COALESCE(d.structured_rationale->>'profile','')<>''
				THEN d.structured_rationale->>'ai_provider' ELSE '' END AS ai_provider,
			CASE WHEN COALESCE(d.structured_rationale->>'ai_provider','')<>''
			       AND COALESCE(d.structured_rationale->>'model_id','')<>''
			       AND COALESCE(d.structured_rationale->>'profile','')<>''
				THEN d.structured_rationale->>'model_id' ELSE '' END AS model_id,
			CASE WHEN COALESCE(d.structured_rationale->>'ai_provider','')<>''
			       AND COALESCE(d.structured_rationale->>'model_id','')<>''
			       AND COALESCE(d.structured_rationale->>'profile','')<>''
				THEN d.structured_rationale->>'profile' ELSE '' END AS profile,
			CASE WHEN COALESCE(d.structured_rationale->>'ai_provider','')<>''
			       AND COALESCE(d.structured_rationale->>'model_id','')<>''
			       AND COALESCE(d.structured_rationale->>'profile','')<>''
				THEN 'EXPLICIT' ELSE 'UNATTRIBUTED_LEGACY' END AS provenance_status,
			CASE WHEN COALESCE(d.structured_rationale->>'latency_ms','') ~ '^[0-9]{1,12}$'
				THEN (d.structured_rationale->>'latency_ms')::numeric END AS latency_ms,
			CASE WHEN COALESCE(d.structured_rationale->>'input_usage','') ~ '^[0-9]{1,12}$'
				THEN (d.structured_rationale->>'input_usage')::numeric END AS input_usage,
			CASE WHEN COALESCE(d.structured_rationale->>'output_usage','') ~ '^[0-9]{1,12}$'
				THEN (d.structured_rationale->>'output_usage')::numeric END AS output_usage
		FROM decision_journal_entries d
		JOIN strategy_instances i ON i.id=d.strategy_instance_id AND i.user_id=d.user_id
		LEFT JOIN risk_evaluations r ON r.id=d.risk_evaluation_id AND r.user_id=d.user_id
		WHERE d.strategy_instance_id=$1 AND d.user_id=$2 AND d.source='AI'
		  AND d.decision_type IN ('ABSTAIN','ALLOW_WOULD_HAVE_SUBMITTED','DENY_RISK_DENIED')
		  AND i.strategy_identifier='ai_shadow' AND i.execution_mode='SHADOW'
	), route_decisions AS (
		SELECT ai_provider,model_id,profile,provenance_status,
			COUNT(*)::integer AS total_decisions,
			(COUNT(*) FILTER (WHERE decision_type='ABSTAIN'))::integer AS abstentions,
			(COUNT(*) FILTER (WHERE decision_type IN ('ALLOW_WOULD_HAVE_SUBMITTED','DENY_RISK_DENIED')))::integer AS proposed_decisions,
			(COUNT(*) FILTER (WHERE decision_type='DENY_RISK_DENIED'))::integer AS risk_held_decisions,
			(COUNT(*) FILTER (WHERE repeat_action_cooldown_hold))::integer AS repeat_action_cooldown_holds,
			(COUNT(*) FILTER (WHERE decision_type='ALLOW_WOULD_HAVE_SUBMITTED'))::integer AS would_have_submitted_decisions,
			COUNT(latency_ms)::integer AS measured_latency_decisions,
			COALESCE(round(avg(latency_ms),2)::text,'') AS average_latency_milliseconds,
			(COUNT(*) FILTER (WHERE input_usage IS NOT NULL OR output_usage IS NOT NULL))::integer AS metered_usage_decisions,
			COALESCE(sum(input_usage),0)::bigint AS recorded_input_tokens,
			COALESCE(sum(output_usage),0)::bigint AS recorded_output_tokens,
			min(created_at) AS first_decision_at,max(created_at) AS last_decision_at
		FROM base GROUP BY ai_provider,model_id,profile,provenance_status
	), route_marks AS (
		SELECT b.ai_provider,b.model_id,b.profile,b.provenance_status,
			(COUNT(o.id) FILTER (WHERE o.horizon='ONE_HOUR'))::integer AS one_hour_outcome_marks,
			(COUNT(o.id) FILTER (WHERE o.horizon='TWENTY_FOUR_HOURS'))::integer AS twenty_four_hour_outcome_marks
		FROM base b
		JOIN shadow_execution_outcomes o
		  ON o.execution_record_id=b.execution_record_id AND o.strategy_instance_id=$1 AND o.user_id=$2
		GROUP BY b.ai_provider,b.model_id,b.profile,b.provenance_status
	)
	SELECT d.ai_provider,d.model_id,d.profile,d.provenance_status,
		d.total_decisions,d.abstentions,d.proposed_decisions,d.risk_held_decisions,d.repeat_action_cooldown_holds,d.would_have_submitted_decisions,
		COALESCE(m.one_hour_outcome_marks,0),COALESCE(m.twenty_four_hour_outcome_marks,0),
		d.measured_latency_decisions,d.average_latency_milliseconds,d.metered_usage_decisions,
		d.recorded_input_tokens,d.recorded_output_tokens,d.first_decision_at,d.last_decision_at
	FROM route_decisions d
	LEFT JOIN route_marks m USING(ai_provider,model_id,profile,provenance_status)
	ORDER BY d.last_decision_at DESC,d.ai_provider,d.model_id,d.profile`, instanceID, userID)
	if err != nil {
		return ShadowBehaviorScore{}, err
	}
	defer rows.Close()
	behavior := ShadowBehaviorScore{Routes: []ShadowRouteBehavior{}, Symbols: []ShadowSymbolBehavior{}}
	for rows.Next() {
		var route ShadowRouteBehavior
		var averageLatency string
		var firstDecisionAt, lastDecisionAt time.Time
		if err = rows.Scan(&route.AIProvider, &route.ModelID, &route.Profile, &route.ProvenanceStatus,
			&route.TotalDecisions, &route.Abstentions, &route.ProposedDecisions, &route.RiskHeldDecisions,
			&route.RepeatActionCooldownHolds, &route.WouldHaveSubmittedDecisions, &route.OneHourOutcomeMarks, &route.TwentyFourHourOutcomeMarks,
			&route.MeasuredLatencyDecisions, &averageLatency, &route.MeteredUsageDecisions,
			&route.RecordedInputTokens, &route.RecordedOutputTokens, &firstDecisionAt, &lastDecisionAt); err != nil {
			return ShadowBehaviorScore{}, err
		}
		if averageLatency != "" {
			route.AverageLatencyMilliseconds = &averageLatency
		}
		behavior.TotalAIDecisions += route.TotalDecisions
		behavior.Abstentions += route.Abstentions
		behavior.ProposedDecisions += route.ProposedDecisions
		behavior.RiskHeldDecisions += route.RiskHeldDecisions
		behavior.RepeatActionCooldownHolds += route.RepeatActionCooldownHolds
		behavior.WouldHaveSubmittedDecisions += route.WouldHaveSubmittedDecisions
		if route.ProvenanceStatus == ShadowRouteProvenanceExplicit {
			behavior.AttributedDecisions += route.TotalDecisions
		} else {
			behavior.UnattributedLegacyDecisions += route.TotalDecisions
		}
		if behavior.FirstDecisionAt == nil || firstDecisionAt.Before(*behavior.FirstDecisionAt) {
			value := firstDecisionAt
			behavior.FirstDecisionAt = &value
		}
		if behavior.LastDecisionAt == nil || lastDecisionAt.After(*behavior.LastDecisionAt) {
			value := lastDecisionAt
			behavior.LastDecisionAt = &value
		}
		behavior.Routes = append(behavior.Routes, route)
	}
	if err = rows.Err(); err != nil {
		return ShadowBehaviorScore{}, err
	}
	behavior.AbstentionRatePercent = shadowBehaviorRate(behavior.Abstentions, behavior.TotalAIDecisions)
	behavior.ProposalRatePercent = shadowBehaviorRate(behavior.ProposedDecisions, behavior.TotalAIDecisions)
	if behavior.TotalAIDecisions > 1 && behavior.FirstDecisionAt != nil && behavior.LastDecisionAt != nil {
		value := fmt.Sprintf("%.2f", behavior.LastDecisionAt.Sub(*behavior.FirstDecisionAt).Minutes()/float64(behavior.TotalAIDecisions-1))
		behavior.AverageDecisionIntervalMins = &value
	}

	symbolRows, err := s.db.Query(ctx, `WITH executions AS (
		SELECT x.id,x.symbol,x.status
		FROM decision_journal_entries d
		JOIN strategy_instances i ON i.id=d.strategy_instance_id AND i.user_id=d.user_id
		JOIN nonlive_execution_records x
		  ON x.id=d.execution_record_id AND x.user_id=d.user_id AND x.strategy_instance_id=d.strategy_instance_id
		WHERE d.strategy_instance_id=$1 AND d.user_id=$2 AND d.source='AI'
		  AND d.decision_type IN ('ALLOW_WOULD_HAVE_SUBMITTED','DENY_RISK_DENIED')
		  AND x.mode='SHADOW' AND x.status IN ('WOULD_HAVE_SUBMITTED','RISK_DENIED')
		  AND i.strategy_identifier='ai_shadow' AND i.execution_mode='SHADOW'
	), symbol_decisions AS (
		SELECT symbol,COUNT(*)::integer AS proposed_decisions,
			(COUNT(*) FILTER (WHERE status='RISK_DENIED'))::integer AS risk_held_decisions,
			(COUNT(*) FILTER (WHERE status='WOULD_HAVE_SUBMITTED'))::integer AS would_have_submitted_decisions
		FROM executions GROUP BY symbol
	), symbol_marks AS (
		SELECT e.symbol,
			(COUNT(o.id) FILTER (WHERE o.horizon='ONE_HOUR'))::integer AS one_hour_outcome_marks,
			(COUNT(o.id) FILTER (WHERE o.horizon='TWENTY_FOUR_HOURS'))::integer AS twenty_four_hour_outcome_marks
		FROM executions e
		JOIN shadow_execution_outcomes o
		  ON o.execution_record_id=e.id AND o.strategy_instance_id=$1 AND o.user_id=$2
		GROUP BY e.symbol
	)
	SELECT d.symbol,d.proposed_decisions,d.risk_held_decisions,d.would_have_submitted_decisions,
		COALESCE(m.one_hour_outcome_marks,0),COALESCE(m.twenty_four_hour_outcome_marks,0)
	FROM symbol_decisions d LEFT JOIN symbol_marks m USING(symbol)
	ORDER BY d.proposed_decisions DESC,d.symbol`, instanceID, userID)
	if err != nil {
		return ShadowBehaviorScore{}, err
	}
	defer symbolRows.Close()
	for symbolRows.Next() {
		var symbol ShadowSymbolBehavior
		if err = symbolRows.Scan(&symbol.Symbol, &symbol.ProposedDecisions, &symbol.RiskHeldDecisions,
			&symbol.WouldHaveSubmittedDecisions, &symbol.OneHourOutcomeMarks, &symbol.TwentyFourHourOutcomeMarks); err != nil {
			return ShadowBehaviorScore{}, err
		}
		behavior.Symbols = append(behavior.Symbols, symbol)
	}
	if err = symbolRows.Err(); err != nil {
		return ShadowBehaviorScore{}, err
	}
	return behavior, nil
}

func shadowBehaviorRate(count, total int) *string {
	if total == 0 {
		return nil
	}
	value := fmt.Sprintf("%.10f", (float64(count)/float64(total))*100)
	return &value
}
