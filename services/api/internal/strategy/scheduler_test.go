package strategy

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

type scheduleStoreFake struct {
	run        *ScheduledRun
	completion ScheduleCompletion
	claims     int
}

func (f *scheduleStoreFake) ClaimDueSchedule(context.Context, time.Time, time.Duration) (*ScheduledRun, error) {
	f.claims++
	run := f.run
	f.run = nil
	return run, nil
}
func (f *scheduleStoreFake) CompleteSchedule(_ context.Context, _ ScheduledRun, completion ScheduleCompletion) error {
	f.completion = completion
	return nil
}

type scheduledEvaluatorFake struct {
	calls     int
	eventID   string
	principal authorization.Principal
	err       error
}

func (f *scheduledEvaluatorFake) Evaluate(_ context.Context, principal authorization.Principal, _ string, eventID string) (EvaluationOutcome, error) {
	f.calls++
	f.eventID = eventID
	f.principal = principal
	return EvaluationOutcome{}, f.err
}

func scheduledRun(state State, scheduledFor time.Time) *ScheduledRun {
	return &ScheduledRun{StrategyInstanceID: "instance", UserID: "owner", ExecutionMode: Paper, CurrentState: state, IntervalMinutes: 60, Session: "US_EQUITIES_REGULAR", ScheduledFor: scheduledFor, LeaseToken: "lease"}
}

func TestSchedulerEvaluatesActionableStateWithStableEventID(t *testing.T) {
	now := time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC) // 11:00 a.m. EDT.
	scheduledFor := now.Add(-time.Minute)
	store := &scheduleStoreFake{run: scheduledRun(ReadyForPut, scheduledFor)}
	evaluator := &scheduledEvaluatorFake{}
	scheduler := NewScheduler(store, evaluator)
	scheduler.now = func() time.Time { return now }
	claimed, err := scheduler.RunOnce(context.Background())
	if err != nil || !claimed || evaluator.calls != 1 || evaluator.eventID != fmt.Sprintf("scheduled:%d", scheduledFor.Unix()) {
		t.Fatalf("unexpected scheduled evaluation: claimed=%v calls=%d event=%q err=%v", claimed, evaluator.calls, evaluator.eventID, err)
	}
	if evaluator.principal.UserID != "owner" || evaluator.principal.Entitlement != authorization.EntitlementFounder || store.completion.Status != "SUCCEEDED" || !store.completion.NextRunAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected authority or completion: %#v %#v", evaluator.principal, store.completion)
	}
}

func TestSchedulerFailsClosedOutsideSessionAndWhileWaitingForLifecycle(t *testing.T) {
	for name, testCase := range map[string]struct {
		now   time.Time
		state State
		code  string
	}{
		"weekend":  {time.Date(2026, 8, 22, 15, 0, 0, 0, time.UTC), ReadyForPut, "OUTSIDE_SESSION"},
		"open leg": {time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC), ShortPutOpen, "WAITING_FOR_LIFECYCLE"},
	} {
		t.Run(name, func(t *testing.T) {
			store := &scheduleStoreFake{run: scheduledRun(testCase.state, testCase.now)}
			evaluator := &scheduledEvaluatorFake{}
			scheduler := NewScheduler(store, evaluator)
			scheduler.now = func() time.Time { return testCase.now }
			if claimed, err := scheduler.RunOnce(context.Background()); err != nil || !claimed {
				t.Fatalf("run was not safely completed: %v", err)
			}
			if evaluator.calls != 0 || store.completion.Status != "SKIPPED" || store.completion.ErrorCode != testCase.code {
				t.Fatalf("unsafe provider call or status: calls=%d completion=%#v", evaluator.calls, store.completion)
			}
		})
	}
}

func TestSchedulerTreatsCommittedDuplicateAsRecoveredSuccess(t *testing.T) {
	now := time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC)
	store := &scheduleStoreFake{run: scheduledRun(ReadyForCall, now)}
	evaluator := &scheduledEvaluatorFake{err: ErrDuplicate}
	scheduler := NewScheduler(store, evaluator)
	scheduler.now = func() time.Time { return now }
	_, err := scheduler.RunOnce(context.Background())
	if err != nil || store.completion.Status != "SUCCEEDED" || store.completion.ErrorCode != "" {
		t.Fatalf("committed retry was not recovered: %#v %v", store.completion, err)
	}
	evaluator.err = errors.New("sensitive provider detail")
	store.run = scheduledRun(ReadyForCall, now)
	_, _ = scheduler.RunOnce(context.Background())
	if store.completion.Status != "FAILED" || store.completion.ErrorCode != "INTERNAL" {
		t.Fatalf("raw failure leaked into schedule status: %#v", store.completion)
	}
}
