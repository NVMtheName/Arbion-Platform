package strategy

import "time"

const (
	ShadowEvidenceCollecting = "COLLECTING_EVIDENCE"
	ShadowEvidenceReviewable = "EVIDENCE_REVIEWABLE"

	ShadowEvidenceOneHourIncomplete        = "ONE_HOUR_SAMPLE_INCOMPLETE"
	ShadowEvidenceTwentyFourHourIncomplete = "TWENTY_FOUR_HOUR_SAMPLE_INCOMPLETE"
	ShadowEvidenceWindowIncomplete         = "EVIDENCE_WINDOW_INCOMPLETE"
	ShadowEvidenceScheduleNotVerified      = "SCHEDULE_NOT_VERIFIED"
	ShadowEvidenceScheduleUnhealthy        = "SCHEDULE_UNHEALTHY"
)

func buildShadowEvidenceGate(scorecard ShadowScorecard, scheduleObserved bool, lastScheduleStatus string, consecutiveFailures int) ShadowEvidenceGate {
	gate := ShadowEvidenceGate{
		Status:                      ShadowEvidenceCollecting,
		Blockers:                    []string{},
		MinimumSamplePerHorizon:     ShadowScorecardMinimumSample,
		MinimumEvidenceWindowHours:  ShadowEvidenceMinimumWindowHours,
		LastScheduleStatus:          lastScheduleStatus,
		ConsecutiveScheduleFailures: consecutiveFailures,
		ExecutionBoundary:           "SHADOW_ONLY",
		LiveExecutionAvailable:      false,
	}
	var earliest, latest *time.Time
	for _, horizon := range scorecard.Horizons {
		switch horizon.Horizon {
		case ShadowOutcomeOneHour:
			gate.OneHourSampleSize = horizon.SampleSize
		case ShadowOutcomeTwentyFourHours:
			gate.TwentyFourHourSampleSize = horizon.SampleSize
		}
		if horizon.FirstEvaluatedAt != nil && (earliest == nil || horizon.FirstEvaluatedAt.Before(*earliest)) {
			value := *horizon.FirstEvaluatedAt
			earliest = &value
		}
		if horizon.LastEvaluatedAt != nil && (latest == nil || horizon.LastEvaluatedAt.After(*latest)) {
			value := *horizon.LastEvaluatedAt
			latest = &value
		}
	}
	if earliest != nil && latest != nil && !latest.Before(*earliest) {
		gate.EvidenceWindowHours = int64(latest.Sub(*earliest) / time.Hour)
	}
	if gate.OneHourSampleSize < ShadowScorecardMinimumSample {
		gate.Blockers = append(gate.Blockers, ShadowEvidenceOneHourIncomplete)
	}
	if gate.TwentyFourHourSampleSize < ShadowScorecardMinimumSample {
		gate.Blockers = append(gate.Blockers, ShadowEvidenceTwentyFourHourIncomplete)
	}
	if gate.EvidenceWindowHours < ShadowEvidenceMinimumWindowHours {
		gate.Blockers = append(gate.Blockers, ShadowEvidenceWindowIncomplete)
	}
	gate.ScheduleHealthy = scheduleObserved && lastScheduleStatus == "SUCCEEDED" && consecutiveFailures == 0
	if !scheduleObserved {
		gate.Blockers = append(gate.Blockers, ShadowEvidenceScheduleNotVerified)
	} else if !gate.ScheduleHealthy {
		gate.Blockers = append(gate.Blockers, ShadowEvidenceScheduleUnhealthy)
	}
	if len(gate.Blockers) == 0 {
		gate.Status = ShadowEvidenceReviewable
	}
	return gate
}
