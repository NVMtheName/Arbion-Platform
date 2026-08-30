package strategy

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/aiconnection"
	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automationnotification"
	"github.com/arbion/platform/services/api/internal/neural"
)

type scheduleStoreFake struct {
	run                      *ScheduledRun
	completion               ScheduleCompletion
	claims                   int
	reconciliationMarker     string
	reconciliationMarkerTime time.Time
	markerErr                error
	scorecard                *ShadowScorecard
	scorecardErr             error
	scorecardCalls           int
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
func (f *scheduleStoreFake) RecordReconciliationNotification(_ context.Context, _ ScheduledRun, reconciliationID string, deliveredAt time.Time) error {
	if f.markerErr != nil {
		return f.markerErr
	}
	f.reconciliationMarker = reconciliationID
	f.reconciliationMarkerTime = deliveredAt
	return nil
}
func (f *scheduleStoreFake) ShadowScorecard(context.Context, string, string) (ShadowScorecard, error) {
	f.scorecardCalls++
	if f.scorecardErr != nil {
		return ShadowScorecard{}, f.scorecardErr
	}
	if f.scorecard == nil {
		return ShadowScorecard{}, ErrNotFound
	}
	return *f.scorecard, nil
}

type scheduledEvaluatorFake struct {
	calls     int
	eventID   string
	principal authorization.Principal
	outcome   EvaluationOutcome
	err       error
}

type scheduledReconcilerFake struct {
	calls            int
	principal        authorization.Principal
	accountID        string
	now              time.Time
	reconciliationID string
	reviewRequired   bool
	err              error
}

func (f *scheduledReconcilerFake) EnsureScheduledReconciliation(_ context.Context, principal authorization.Principal, accountID string, now time.Time) (string, bool, error) {
	f.calls++
	f.principal = principal
	f.accountID = accountID
	f.now = now
	return f.reconciliationID, f.reviewRequired, f.err
}

type scheduleNotifierFake struct {
	events []automationnotification.Event
	err    error
}

func (f *scheduleNotifierFake) Send(_ context.Context, event automationnotification.Event) error {
	f.events = append(f.events, event)
	return f.err
}

func (f *scheduledEvaluatorFake) Evaluate(_ context.Context, principal authorization.Principal, _ string, eventID string) (EvaluationOutcome, error) {
	f.calls++
	f.eventID = eventID
	f.principal = principal
	return f.outcome, f.err
}

func scheduledRun(state State, scheduledFor time.Time) *ScheduledRun {
	return &ScheduledRun{StrategyInstanceID: "instance", UserID: "owner", FinancialAccountID: "account", OwnerEmail: "owner@example.com", OwnerEmailVerified: true, MandateID: "mandate", ExecutionMode: Paper, CurrentState: state, IntervalMinutes: 60, Session: "US_EQUITIES_REGULAR", ScheduledFor: scheduledFor, LeaseToken: "lease"}
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

func TestSchedulerRecordsOnlyNonLiveEvaluationDisposition(t *testing.T) {
	now := time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC)
	run := scheduledRun(AIMonitoring, now.Add(-time.Minute))
	run.ExecutionMode = Shadow
	run.Session = "CONTINUOUS"
	store := &scheduleStoreFake{run: run}
	evaluator := &scheduledEvaluatorFake{outcome: EvaluationOutcome{
		AIDecision: "PROPOSE",
		Execution:  ExecutionResult{Status: WouldHaveSubmitted},
	}}
	scheduler := NewScheduler(store, evaluator)
	scheduler.now = func() time.Time { return now }

	claimed, err := scheduler.RunOnce(context.Background())
	if err != nil || !claimed {
		t.Fatalf("scheduled AI evaluation failed: claimed=%v err=%v", claimed, err)
	}
	if store.completion.Status != "SUCCEEDED" || store.completion.AIDecision != "PROPOSE" || store.completion.ExecutionStatus != WouldHaveSubmitted || store.completion.DuplicateRecovered {
		t.Fatalf("non-live disposition was not preserved: %#v", store.completion)
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

func TestSchedulerRunsContinuousAIShadowOutsideEquitiesSession(t *testing.T) {
	now := time.Date(2026, 8, 22, 15, 0, 0, 0, time.UTC) // Saturday.
	run := scheduledRun(AIMonitoring, now)
	run.ExecutionMode = Shadow
	run.Session = "CONTINUOUS"
	store := &scheduleStoreFake{run: run}
	evaluator := &scheduledEvaluatorFake{}
	scheduler := NewScheduler(store, evaluator)
	scheduler.now = func() time.Time { return now }
	if claimed, err := scheduler.RunOnce(context.Background()); err != nil || !claimed {
		t.Fatalf("continuous AI shadow run failed: %v", err)
	}
	if evaluator.calls != 1 || store.completion.Status != "SUCCEEDED" {
		t.Fatalf("continuous AI shadow was improperly session-gated: calls=%d completion=%#v", evaluator.calls, store.completion)
	}
}

func TestSchedulerRefreshesAIShadowReconciliationBeforeEvaluation(t *testing.T) {
	now := time.Date(2026, 8, 22, 15, 0, 0, 0, time.UTC)
	run := scheduledRun(AIMonitoring, now)
	run.ExecutionMode = Shadow
	run.Session = "CONTINUOUS"
	store := &scheduleStoreFake{run: run}
	evaluator := &scheduledEvaluatorFake{}
	reconciler := &scheduledReconcilerFake{reconciliationID: "reconciliation-1"}
	scheduler := NewScheduler(store, evaluator)
	scheduler.ConfigureReconciliation(reconciler)
	scheduler.now = func() time.Time { return now }

	if claimed, err := scheduler.RunOnce(context.Background()); err != nil || !claimed {
		t.Fatalf("AI Shadow schedule failed: claimed=%v err=%v", claimed, err)
	}
	if reconciler.calls != 1 || reconciler.accountID != "account" || !reconciler.now.Equal(now) || reconciler.principal.UserID != "owner" || evaluator.calls != 1 || store.completion.Status != "SUCCEEDED" || store.completion.ReconciliationID != "reconciliation-1" {
		t.Fatalf("reconciliation did not precede the evaluation boundary: reconciler=%#v evaluator_calls=%d completion=%#v", reconciler, evaluator.calls, store.completion)
	}
}

func TestSchedulerDoesNotReconcilePaperStrategy(t *testing.T) {
	now := time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC)
	store := &scheduleStoreFake{run: scheduledRun(ReadyForPut, now)}
	evaluator := &scheduledEvaluatorFake{}
	reconciler := &scheduledReconcilerFake{}
	scheduler := NewScheduler(store, evaluator)
	scheduler.ConfigureReconciliation(reconciler)
	scheduler.now = func() time.Time { return now }

	if claimed, err := scheduler.RunOnce(context.Background()); err != nil || !claimed {
		t.Fatalf("PAPER schedule failed: claimed=%v err=%v", claimed, err)
	}
	if reconciler.calls != 0 || evaluator.calls != 1 || store.completion.Status != "SUCCEEDED" {
		t.Fatalf("PAPER schedule crossed the AI reconciliation boundary: reconciler_calls=%d evaluator_calls=%d completion=%#v", reconciler.calls, evaluator.calls, store.completion)
	}
}

func TestSchedulerFailsClosedWhenAIShadowReconciliationCannotRun(t *testing.T) {
	now := time.Date(2026, 8, 22, 15, 0, 0, 0, time.UTC)
	run := scheduledRun(AIMonitoring, now)
	run.ExecutionMode = Shadow
	run.Session = "CONTINUOUS"
	store := &scheduleStoreFake{run: run}
	evaluator := &scheduledEvaluatorFake{}
	reconciler := &scheduledReconcilerFake{err: errors.New("sensitive provider detail")}
	scheduler := NewScheduler(store, evaluator)
	scheduler.ConfigureReconciliation(reconciler)
	scheduler.now = func() time.Time { return now }

	if claimed, err := scheduler.RunOnce(context.Background()); err != nil || !claimed {
		t.Fatalf("failed reconciliation was not durably completed: claimed=%v err=%v", claimed, err)
	}
	if reconciler.calls != 1 || evaluator.calls != 0 || store.completion.Status != "FAILED" || store.completion.ErrorCode != "RECONCILIATION_REFRESH_FAILED" {
		t.Fatalf("unsafe evaluation or raw error leakage: reconciler=%#v evaluator_calls=%d completion=%#v", reconciler, evaluator.calls, store.completion)
	}
}

func TestSchedulerFailsClosedWhenSessionCalendarIsUnavailable(t *testing.T) {
	now := time.Date(2029, 1, 2, 16, 0, 0, 0, time.UTC)
	store := &scheduleStoreFake{run: scheduledRun(ReadyForPut, now)}
	evaluator := &scheduledEvaluatorFake{}
	scheduler := NewScheduler(store, evaluator)
	scheduler.now = func() time.Time { return now }

	claimed, err := scheduler.RunOnce(context.Background())
	if err != nil || !claimed {
		t.Fatalf("run was not safely completed: %v", err)
	}
	if evaluator.calls != 0 || store.completion.Status != "FAILED" || store.completion.ErrorCode != "SESSION_CALENDAR_UNAVAILABLE" || !store.completion.NextRunAt.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("unsupported calendar did not fail closed: calls=%d completion=%#v", evaluator.calls, store.completion)
	}
}

func TestSchedulerTreatsCommittedDuplicateAsRecoveredSuccess(t *testing.T) {
	now := time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC)
	store := &scheduleStoreFake{run: scheduledRun(ReadyForCall, now)}
	evaluator := &scheduledEvaluatorFake{err: ErrDuplicate}
	scheduler := NewScheduler(store, evaluator)
	scheduler.now = func() time.Time { return now }
	_, err := scheduler.RunOnce(context.Background())
	if err != nil || store.completion.Status != "SUCCEEDED" || store.completion.ErrorCode != "" || !store.completion.DuplicateRecovered {
		t.Fatalf("committed retry was not recovered: %#v %v", store.completion, err)
	}
	evaluator.err = errors.New("sensitive provider detail")
	store.run = scheduledRun(ReadyForCall, now)
	_, _ = scheduler.RunOnce(context.Background())
	if store.completion.Status != "FAILED" || store.completion.ErrorCode != "INTERNAL" {
		t.Fatalf("raw failure leaked into schedule status: %#v", store.completion)
	}
}

func TestScheduleErrorClassificationPreservesSafeEvaluationDiagnostics(t *testing.T) {
	tests := map[string]error{
		"STRATEGY_NOT_ACTIVE":            ErrEvaluationInactive,
		"STRATEGY_CONFIGURATION_CHANGED": ErrEvaluationConfigurationChanged,
		"STRATEGY_PARAMETERS_INVALID":    ErrEvaluationParametersInvalid,
		"PAPER_STATE_UNAVAILABLE":        ErrEvaluationPaperStateUnavailable,
		"MARKET_DATA_STALE":              ErrEvaluationMarketDataStale,
		"NO_ELIGIBLE_OPTION_CONTRACTS":   ErrEvaluationNoEligibleContracts,
		"AI_DECISION_BUDGET_EXHAUSTED":   aiconnection.ErrRateLimit,
		"AI_PROVIDER_RATE_LIMITED":       &neural.ProviderError{Code: neural.RateLimited},
		"AI_REQUEST_INVALID":             &neural.ProviderError{Code: neural.InvalidRequest},
	}
	for want, err := range tests {
		if got := classifyScheduleError(err); got != want {
			t.Fatalf("classification=%q want=%q", got, want)
		}
	}
}

func TestScheduleNotificationsAreOptInVerifiedAndDeduplicated(t *testing.T) {
	scheduledFor := time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC)
	waiting := "WAITING_FOR_LIFECYCLE"
	tests := []struct {
		name       string
		run        ScheduledRun
		completion ScheduleCompletion
		kind       automationnotification.Kind
	}{
		{
			name: "new blocking reconciliation drift",
			run: ScheduledRun{
				OwnerEmail: "owner@example.com", OwnerEmailVerified: true, MandateID: "mandate", FinancialAccountID: "account",
				ExecutionMode: Shadow, ScheduledFor: scheduledFor, NotifyReconciliationReview: true,
			},
			completion: ScheduleCompletion{Status: "SUCCEEDED", ReconciliationID: "reconciliation-1", ReconciliationReviewRequired: true},
			kind:       automationnotification.ReconciliationReviewRequired,
		},
		{
			name:       "evaluation",
			run:        ScheduledRun{OwnerEmail: "owner@example.com", OwnerEmailVerified: true, MandateID: "mandate", ExecutionMode: Paper, ScheduledFor: scheduledFor, NotifyEvaluation: true},
			completion: ScheduleCompletion{Status: "SUCCEEDED"},
			kind:       automationnotification.EvaluationCompleted,
		},
		{
			name:       "lifecycle first observation",
			run:        ScheduledRun{OwnerEmail: "owner@example.com", OwnerEmailVerified: true, MandateID: "mandate", ExecutionMode: Paper, ScheduledFor: scheduledFor, NotifyLifecycle: true},
			completion: ScheduleCompletion{Status: "SKIPPED", ErrorCode: "WAITING_FOR_LIFECYCLE"},
			kind:       automationnotification.LifecycleRequired,
		},
		{
			name:       "first failure",
			run:        ScheduledRun{OwnerEmail: "owner@example.com", OwnerEmailVerified: true, MandateID: "mandate", ExecutionMode: Shadow, ScheduledFor: scheduledFor, NotifyFirstFailure: true},
			completion: ScheduleCompletion{Status: "FAILED", ErrorCode: "PROVIDER"},
			kind:       automationnotification.FirstFailure,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			event := scheduleNotification(testCase.run, testCase.completion)
			if event == nil || event.Kind != testCase.kind || event.Recipient != "owner@example.com" || event.MandateID != "mandate" {
				t.Fatalf("unexpected notification: %#v", event)
			}
		})
	}

	suppressed := []ScheduledRun{
		{OwnerEmail: "owner@example.com", OwnerEmailVerified: false, NotifyEvaluation: true},
		{OwnerEmail: "owner@example.com", OwnerEmailVerified: true},
		{OwnerEmail: "owner@example.com", OwnerEmailVerified: true, NotifyLifecycle: true, PreviousErrorCode: &waiting},
		{OwnerEmail: "owner@example.com", OwnerEmailVerified: true, NotifyFirstFailure: true, ConsecutiveFailures: 1},
		{OwnerEmail: "owner@example.com", OwnerEmailVerified: true, NotifyReconciliationReview: true, LastReconciliationNotificationID: stringPointer("reconciliation-1")},
	}
	completions := []ScheduleCompletion{
		{Status: "SUCCEEDED"},
		{Status: "SUCCEEDED"},
		{Status: "SKIPPED", ErrorCode: "WAITING_FOR_LIFECYCLE"},
		{Status: "FAILED", ErrorCode: "PROVIDER"},
		{Status: "SUCCEEDED", ReconciliationID: "reconciliation-1", ReconciliationReviewRequired: true},
	}
	for index := range suppressed {
		if event := scheduleNotification(suppressed[index], completions[index]); event != nil {
			t.Fatalf("notification was not suppressed: %#v", event)
		}
	}
}

func TestSuccessfulAIShadowEvaluationAddsDurableGateStatusToExistingOptInEmail(t *testing.T) {
	now := time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC)
	run := scheduledRun(AIMonitoring, now.Add(-time.Minute))
	run.ExecutionMode = Shadow
	run.Session = "CONTINUOUS"
	run.NotifyEvaluation = true
	store := &scheduleStoreFake{
		run: run,
		scorecard: &ShadowScorecard{EvidenceGate: ShadowEvidenceGate{
			Status:                 ShadowEvidenceReviewable,
			ScheduleHealthy:        true,
			LiveExecutionAvailable: false,
		}},
	}
	notifier := &scheduleNotifierFake{}
	scheduler := NewScheduler(store, &scheduledEvaluatorFake{}, notifier)
	scheduler.now = func() time.Time { return now }

	claimed, err := scheduler.RunOnce(context.Background())
	if err != nil || !claimed || store.completion.Status != "SUCCEEDED" || store.scorecardCalls != 1 || len(notifier.events) != 1 {
		t.Fatalf("reviewable gate was not read after durable completion: claimed=%v completion=%#v scorecard_calls=%d events=%#v err=%v", claimed, store.completion, store.scorecardCalls, notifier.events, err)
	}
	event := notifier.events[0]
	if event.Kind != automationnotification.EvaluationCompleted || event.EvidenceGateStatus != ShadowEvidenceReviewable || event.ExecutionMode != string(Shadow) {
		t.Fatalf("existing opt-in email was not safely enriched: %#v", event)
	}
}

func TestAIShadowEvaluationFallsBackToGenericEmailWhenGateCannotBeVerified(t *testing.T) {
	now := time.Date(2026, 9, 1, 2, 0, 0, 0, time.UTC)
	run := scheduledRun(AIMonitoring, now.Add(-time.Minute))
	run.ExecutionMode = Shadow
	run.Session = "CONTINUOUS"
	run.NotifyEvaluation = true
	store := &scheduleStoreFake{run: run, scorecardErr: errors.New("database unavailable")}
	notifier := &scheduleNotifierFake{}
	scheduler := NewScheduler(store, &scheduledEvaluatorFake{}, notifier)
	scheduler.now = func() time.Time { return now }

	claimed, err := scheduler.RunOnce(context.Background())
	if err != nil || !claimed || store.completion.Status != "SUCCEEDED" || len(notifier.events) != 1 || notifier.events[0].EvidenceGateStatus != "" {
		t.Fatalf("unverified gate changed the completed schedule or invented readiness: claimed=%v completion=%#v events=%#v err=%v", claimed, store.completion, notifier.events, err)
	}
}

func TestShadowGateIsNotReadWithoutDeliverableOptInEmail(t *testing.T) {
	now := time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC)
	run := scheduledRun(AIMonitoring, now.Add(-time.Minute))
	run.ExecutionMode = Shadow
	run.Session = "CONTINUOUS"
	run.NotifyEvaluation = true
	store := &scheduleStoreFake{
		run:       run,
		scorecard: &ShadowScorecard{EvidenceGate: ShadowEvidenceGate{Status: ShadowEvidenceReviewable}},
	}
	scheduler := NewScheduler(store, &scheduledEvaluatorFake{})
	scheduler.now = func() time.Time { return now }

	claimed, err := scheduler.RunOnce(context.Background())
	if err != nil || !claimed || store.completion.Status != "SUCCEEDED" || store.scorecardCalls != 0 {
		t.Fatalf("gate was read without an available notification path: claimed=%v completion=%#v scorecard_calls=%d err=%v", claimed, store.completion, store.scorecardCalls, err)
	}

	run.OwnerEmailVerified = false
	store.run = run
	notifier := &scheduleNotifierFake{}
	scheduler = NewScheduler(store, &scheduledEvaluatorFake{}, notifier)
	scheduler.now = func() time.Time { return now }
	claimed, err = scheduler.RunOnce(context.Background())
	if err != nil || !claimed || store.scorecardCalls != 0 || len(notifier.events) != 0 {
		t.Fatalf("gate was read or email sent without a verified owner: claimed=%v scorecard_calls=%d events=%#v err=%v", claimed, store.scorecardCalls, notifier.events, err)
	}
}

func stringPointer(value string) *string { return &value }

func TestSuccessfulDriftReviewDeliveryRecordsTheImmutableEvidenceMarker(t *testing.T) {
	now := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	run := scheduledRun(AIMonitoring, now)
	run.ExecutionMode = Shadow
	run.Session = "CONTINUOUS"
	run.NotifyReconciliationReview = true
	store := &scheduleStoreFake{run: run}
	reconciler := &scheduledReconcilerFake{reconciliationID: "reconciliation-1", reviewRequired: true}
	notifier := &scheduleNotifierFake{}
	scheduler := NewScheduler(store, &scheduledEvaluatorFake{}, notifier)
	scheduler.ConfigureReconciliation(reconciler)
	scheduler.now = func() time.Time { return now }

	claimed, err := scheduler.RunOnce(context.Background())
	if err != nil || !claimed || len(notifier.events) != 1 || notifier.events[0].Kind != automationnotification.ReconciliationReviewRequired || store.reconciliationMarker != "reconciliation-1" || !store.reconciliationMarkerTime.Equal(now) {
		t.Fatalf("drift delivery was not durably marked: claimed=%v events=%#v marker=%q marker_time=%s err=%v", claimed, notifier.events, store.reconciliationMarker, store.reconciliationMarkerTime, err)
	}
}

func TestFailedDriftReviewDeliveryLeavesTheEvidenceRetryable(t *testing.T) {
	now := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	run := scheduledRun(AIMonitoring, now)
	run.ExecutionMode = Shadow
	run.Session = "CONTINUOUS"
	run.NotifyReconciliationReview = true
	store := &scheduleStoreFake{run: run}
	reconciler := &scheduledReconcilerFake{reconciliationID: "reconciliation-1", reviewRequired: true}
	notifier := &scheduleNotifierFake{err: errors.New("delivery unavailable")}
	scheduler := NewScheduler(store, &scheduledEvaluatorFake{}, notifier)
	scheduler.ConfigureReconciliation(reconciler)
	scheduler.now = func() time.Time { return now }

	claimed, err := scheduler.RunOnce(context.Background())
	if err != nil || !claimed || store.completion.Status != "SUCCEEDED" || store.reconciliationMarker != "" {
		t.Fatalf("delivery failure consumed drift evidence or failed the schedule: claimed=%v completion=%#v marker=%q err=%v", claimed, store.completion, store.reconciliationMarker, err)
	}
}

func TestNotificationDeliveryFailureDoesNotFailCompletedSchedule(t *testing.T) {
	now := time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC)
	run := scheduledRun(ReadyForPut, now)
	run.NotifyEvaluation = true
	store := &scheduleStoreFake{run: run}
	notifier := &scheduleNotifierFake{err: errors.New("delivery unavailable")}
	scheduler := NewScheduler(store, &scheduledEvaluatorFake{}, notifier)
	scheduler.now = func() time.Time { return now }
	claimed, err := scheduler.RunOnce(context.Background())
	if err != nil || !claimed || len(notifier.events) != 1 || store.completion.Status != "SUCCEEDED" {
		t.Fatalf("delivery failure changed schedule result: claimed=%v completion=%#v events=%#v err=%v", claimed, store.completion, notifier.events, err)
	}
}
