package strategy

import (
	"reflect"
	"testing"
	"time"
)

func TestShadowEvidenceGateCollectsTransparentBlockers(t *testing.T) {
	now := time.Date(2026, 8, 26, 3, 0, 0, 0, time.UTC)
	scorecard := ShadowScorecard{Horizons: []ShadowHorizonScore{
		{Horizon: ShadowOutcomeOneHour, SampleSize: 3, FirstEvaluatedAt: &now, LastEvaluatedAt: &now},
		{Horizon: ShadowOutcomeTwentyFourHours, SampleSize: 0},
	}}
	gate := buildShadowEvidenceGate(scorecard, false, "", 0)
	want := []string{
		ShadowEvidenceOneHourIncomplete,
		ShadowEvidenceTwentyFourHourIncomplete,
		ShadowEvidenceWindowIncomplete,
		ShadowEvidenceScheduleNotVerified,
	}
	if gate.Status != ShadowEvidenceCollecting || !reflect.DeepEqual(gate.Blockers, want) || gate.ScheduleHealthy || gate.LiveExecutionAvailable || gate.ExecutionBoundary != "SHADOW_ONLY" {
		t.Fatalf("incomplete evidence gate was not conservative: %#v", gate)
	}
}

func TestShadowEvidenceGateMarksEvidenceReviewableWithoutGrantingExecution(t *testing.T) {
	last := time.Date(2026, 8, 26, 3, 0, 0, 0, time.UTC)
	first := last.Add(-8 * 24 * time.Hour)
	scorecard := ShadowScorecard{Horizons: []ShadowHorizonScore{
		{Horizon: ShadowOutcomeOneHour, SampleSize: 20, FirstEvaluatedAt: &first, LastEvaluatedAt: &last},
		{Horizon: ShadowOutcomeTwentyFourHours, SampleSize: 20, FirstEvaluatedAt: &first, LastEvaluatedAt: &last},
	}}
	gate := buildShadowEvidenceGate(scorecard, true, "SUCCEEDED", 0)
	if gate.Status != ShadowEvidenceReviewable || len(gate.Blockers) != 0 || !gate.ScheduleHealthy || gate.EvidenceWindowHours != 192 || gate.LiveExecutionAvailable || gate.ExecutionBoundary != "SHADOW_ONLY" {
		t.Fatalf("complete evidence gate was incorrect: %#v", gate)
	}
}
