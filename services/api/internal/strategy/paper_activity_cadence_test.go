package strategy

import (
	"testing"
	"time"
)

func cadenceText(value string) *string { return &value }

func cadenceRun(id, instanceID string, scheduled time.Time, status, decision, execution string) ScheduleRun {
	run := ScheduleRun{
		ID: id, StrategyInstanceID: instanceID, MandateID: "mandate", MandateVersion: 1, ExecutionMode: Paper, StrategyState: AIMonitoring,
		ScheduledFor: scheduled, StartedAt: scheduled.Add(time.Second), CompletedAt: scheduled.Add(10 * time.Minute), NextRunAt: scheduled.Add(time.Hour), Status: status,
	}
	if decision != "" {
		run.AIDecision = cadenceText(decision)
	}
	if execution != "" {
		run.ExecutionStatus = cadenceText(execution)
	}
	if status != "SUCCEEDED" {
		run.ErrorCode = cadenceText("TEST_FAILURE")
	}
	return run
}

func TestProjectPaperActivityCadenceSeparatesScheduleAndFillChronology(t *testing.T) {
	instanceID := "instance"
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	runs := make([]ScheduleRun, 0, 25)
	for index := 0; index <= 24; index++ {
		runs = append(runs, cadenceRun(time.Duration(index).String(), instanceID, base.Add(time.Duration(index)*time.Hour), "SUCCEEDED", "ABSTAIN", "CANCELED"))
	}
	runs[8] = cadenceRun("risk-denied", instanceID, base.Add(8*time.Hour), "SUCCEEDED", "PROPOSE", "RISK_DENIED")
	runs[15] = cadenceRun("failed", instanceID, base.Add(15*time.Hour), "FAILED", "", "")
	runs[24] = cadenceRun("filled", instanceID, base.Add(24*time.Hour), "SUCCEEDED", "PROPOSE", "SIMULATED_FILLED")
	fills := []paperRealizedFill{
		exactExecutionFill("first", "BTC", "BUY", "1", "100", "100.25", "100.25", "0.50125", base.Add(2*time.Hour)),
		exactExecutionFill("second", "ETH", "BUY", "1", "100", "100.25", "100.25", "0.50125", base.Add(6*time.Hour)),
		exactExecutionFill("third", "BTC", "SELL", "0.5", "110", "109.725", "54.8625", "0.2743125", base.Add(24*time.Hour)),
	}
	result := projectPaperActivityCadence(instanceID, base.Add(-48*time.Hour), 60, runs, fills, true)
	if result.Status != PaperActivityCadenceAvailable || result.CalculationMethod != PaperActivityCadenceMethod || result.AsOf == nil || result.ScheduleIntervalMinutes != 60 {
		t.Fatalf("activity cadence unavailable: %#v", result)
	}
	window := result.TwentyFourHours
	if window.Status != PaperActivityCadenceAvailable || window.ScheduledCycleCount != 24 || window.SucceededCycleCount != 23 || window.FailedCycleCount != 1 ||
		window.AbstentionCount != 21 || window.DeterministicDenyCount != 1 || window.SimulatedFillCount != 1 || window.OtherSucceededCount != 0 {
		t.Fatalf("24-hour exact disposition changed: %#v", window)
	}
	if result.SevenDays.Status != PaperActivityCadenceUnavailable || result.SevenDays.ScheduledCycleCount != 25 {
		t.Fatalf("incomplete seven-day window was inferred: %#v", result.SevenDays)
	}
	if result.FillTiming.Status != PaperActivityCadenceAvailable || result.FillTiming.FillCount != 3 || result.FillTiming.MinimumInterFillSeconds != "14400.0000000000" ||
		result.FillTiming.MedianInterFillSeconds != "39600.0000000000" || result.FillTiming.MaximumInterFillSeconds != "64800.0000000000" || len(result.FillTiming.Symbols) != 2 ||
		result.FillTiming.Symbols[0].Symbol != "BTC" || result.FillTiming.Symbols[0].MaximumInterFillSeconds != "79200.0000000000" || result.FillTiming.Symbols[1].Status != PaperFillTimingInsufficient {
		t.Fatalf("exact fill timing changed: %#v", result.FillTiming)
	}
	if result.LongestNoFillInterval.Status != PaperActivityCadenceAvailable || result.LongestNoFillInterval.CycleCount < 1 || result.LongestNoFillInterval.IntervalSeconds == "" {
		t.Fatalf("no-fill interval missing: %#v", result.LongestNoFillInterval)
	}
}

func TestProjectPaperActivityCadenceFailsClosedOnBrokenScheduleChain(t *testing.T) {
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	runs := []ScheduleRun{
		cadenceRun("one", "instance", base, "SUCCEEDED", "ABSTAIN", "CANCELED"),
		cadenceRun("two", "instance", base.Add(2*time.Hour), "SUCCEEDED", "ABSTAIN", "CANCELED"),
	}
	result := projectPaperActivityCadence("instance", base.Add(-48*time.Hour), 60, runs, nil, true)
	if result.TwentyFourHours.Status != PaperActivityCadenceUnavailable || result.Status != PaperActivityCadenceUnavailable {
		t.Fatalf("broken schedule chain did not fail closed: %#v", result)
	}
}

func TestProjectPaperActivityCadenceDoesNotInferIncompleteFillEvidence(t *testing.T) {
	base := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	run := cadenceRun("one", "instance", base, "SUCCEEDED", "ABSTAIN", "CANCELED")
	result := projectPaperActivityCadence("instance", base.Add(-48*time.Hour), 60, []ScheduleRun{run}, []paperRealizedFill{exactExecutionFill("fill", "BTC", "BUY", "1", "100", "100.25", "100.25", "0.50125", base)}, false)
	if result.FillTiming.Status != PaperActivityCadenceUnavailable || result.Status != PaperActivityCadenceUnavailable {
		t.Fatalf("incomplete fill history was inferred: %#v", result)
	}
}
