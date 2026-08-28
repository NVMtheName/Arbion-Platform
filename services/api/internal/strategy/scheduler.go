package strategy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/arbion/platform/services/api/internal/aiconnection"
	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automationnotification"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/neural"
)

const (
	scheduleLeaseDuration = 2 * time.Minute
	schedulePollInterval  = 30 * time.Second
	maxClaimsPerPoll      = 10
)

type ScheduleStore interface {
	ClaimDueSchedule(context.Context, time.Time, time.Duration) (*ScheduledRun, error)
	CompleteSchedule(context.Context, ScheduledRun, ScheduleCompletion) error
	RecordReconciliationNotification(context.Context, ScheduledRun, string, time.Time) error
}

type ScheduledEvaluator interface {
	Evaluate(context.Context, authorization.Principal, string, string) (EvaluationOutcome, error)
}

type ScheduledReconciler interface {
	EnsureScheduledReconciliation(context.Context, authorization.Principal, string, time.Time) (string, bool, error)
}

type Scheduler struct {
	store      ScheduleStore
	evaluator  ScheduledEvaluator
	reconciler ScheduledReconciler
	notifier   automationnotification.Sender
	now        func() time.Time
	logger     *slog.Logger
}

func (s *Scheduler) ConfigureReconciliation(reconciler ScheduledReconciler) {
	s.reconciler = reconciler
}

func NewScheduler(store ScheduleStore, evaluator ScheduledEvaluator, notifier ...automationnotification.Sender) *Scheduler {
	scheduler := &Scheduler{store: store, evaluator: evaluator, now: func() time.Time { return time.Now().UTC() }, logger: slog.Default()}
	if len(notifier) > 0 {
		scheduler.notifier = notifier[0]
	}
	return scheduler
}

func (s *Scheduler) Run(ctx context.Context) {
	s.runBatch(ctx)
	ticker := time.NewTicker(schedulePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runBatch(ctx)
		}
	}
}

func (s *Scheduler) runBatch(ctx context.Context) {
	for count := 0; count < maxClaimsPerPoll; count++ {
		claimed, err := s.RunOnce(ctx)
		if err != nil {
			s.logger.Error("non-live schedule run failed", "error_code", classifyScheduleError(err))
			return
		}
		if !claimed {
			return
		}
	}
}

// RunOnce is deliberately small and deterministic so lease, session, and
// duplicate-recovery behavior can be tested without a ticker.
func (s *Scheduler) RunOnce(ctx context.Context) (bool, error) {
	now := s.now().UTC()
	run, err := s.store.ClaimDueSchedule(ctx, now, scheduleLeaseDuration)
	if err != nil || run == nil {
		return false, err
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = now
	}
	completion := ScheduleCompletion{NextRunAt: now.Add(time.Duration(run.IntervalMinutes) * time.Minute)}
	if run.ExecutionMode != Paper && run.ExecutionMode != Shadow {
		completion.Status, completion.ErrorCode = "FAILED", "UNSUPPORTED_MODE"
	} else if run.Session == "US_EQUITIES_REGULAR" && !inRegularSession(now) {
		if nextRunAt, ok := nextRegularSession(now); ok {
			completion.Status, completion.ErrorCode = "SKIPPED", "OUTSIDE_SESSION"
			completion.NextRunAt = nextRunAt
		} else {
			completion.Status, completion.ErrorCode = "FAILED", "SESSION_CALENDAR_UNAVAILABLE"
			completion.NextRunAt = now.Add(24 * time.Hour)
		}
	} else if run.Session != "US_EQUITIES_REGULAR" && run.Session != "CONTINUOUS" {
		completion.Status, completion.ErrorCode = "FAILED", "UNSUPPORTED_SESSION"
	} else if !actionableState(run.CurrentState) {
		completion.Status, completion.ErrorCode = "SKIPPED", "WAITING_FOR_LIFECYCLE"
	} else {
		principal := authorization.Principal{UserID: run.UserID, Entitlement: authorization.EntitlementFounder}
		eventID := fmt.Sprintf("scheduled:%d", run.ScheduledFor.UTC().Unix())
		if run.ExecutionMode == Shadow && run.CurrentState == AIMonitoring && s.reconciler != nil {
			completion.ReconciliationID, completion.ReconciliationReviewRequired, err = s.reconciler.EnsureScheduledReconciliation(ctx, principal, run.FinancialAccountID, now)
			if err != nil {
				completion.Status, completion.ErrorCode = "FAILED", "RECONCILIATION_REFRESH_FAILED"
			}
		}
		if completion.Status == "" {
			var outcome EvaluationOutcome
			outcome, err = s.evaluator.Evaluate(ctx, principal, run.StrategyInstanceID, eventID)
			if err == nil || errors.Is(err, ErrDuplicate) {
				completion.Status = "SUCCEEDED"
				completion.DuplicateRecovered = errors.Is(err, ErrDuplicate)
				if err == nil {
					completion.AIDecision = outcome.AIDecision
					completion.ExecutionStatus = outcome.Execution.Status
				}
			} else {
				completion.Status, completion.ErrorCode = "FAILED", classifyScheduleError(err)
			}
		}
	}
	completion.CompletedAt = s.now().UTC()
	if completion.CompletedAt.Before(run.StartedAt) {
		completion.CompletedAt = run.StartedAt
	}
	if err := s.store.CompleteSchedule(ctx, *run, completion); err != nil {
		return true, err
	}
	if event := scheduleNotification(*run, completion); event != nil && s.notifier != nil {
		if err := s.notifier.Send(ctx, *event); err != nil {
			s.logger.Error("non-live schedule notification delivery failed", "strategy_instance_id", run.StrategyInstanceID, "notification_kind", event.Kind)
		} else {
			if event.Kind == automationnotification.ReconciliationReviewRequired {
				if err := s.store.RecordReconciliationNotification(ctx, *run, completion.ReconciliationID, s.now().UTC()); err != nil {
					s.logger.Error("non-live reconciliation notification marker failed", "strategy_instance_id", run.StrategyInstanceID, "notification_kind", event.Kind)
				}
			}
			s.logger.Info("non-live schedule notification delivered", "strategy_instance_id", run.StrategyInstanceID, "notification_kind", event.Kind)
		}
	}
	s.logger.Info("non-live schedule completed", "strategy_instance_id", run.StrategyInstanceID, "status", completion.Status, "error_code", completion.ErrorCode)
	return true, nil
}

func scheduleNotification(run ScheduledRun, completion ScheduleCompletion) *automationnotification.Event {
	if run.OwnerEmail == "" || !run.OwnerEmailVerified {
		return nil
	}
	event := automationnotification.Event{
		Recipient:          run.OwnerEmail,
		MandateID:          run.MandateID,
		FinancialAccountID: run.FinancialAccountID,
		ReconciliationID:   completion.ReconciliationID,
		ExecutionMode:      string(run.ExecutionMode),
		ScheduledFor:       run.ScheduledFor,
		SafeErrorCode:      completion.ErrorCode,
	}
	switch {
	case completion.Status == "SUCCEEDED" && completion.ReconciliationReviewRequired && run.NotifyReconciliationReview && completion.ReconciliationID != "" && (run.LastReconciliationNotificationID == nil || *run.LastReconciliationNotificationID != completion.ReconciliationID):
		event.Kind = automationnotification.ReconciliationReviewRequired
	case completion.Status == "SUCCEEDED" && run.NotifyEvaluation:
		event.Kind = automationnotification.EvaluationCompleted
	case completion.ErrorCode == "WAITING_FOR_LIFECYCLE" && run.NotifyLifecycle && (run.PreviousErrorCode == nil || *run.PreviousErrorCode != "WAITING_FOR_LIFECYCLE"):
		event.Kind = automationnotification.LifecycleRequired
	case completion.Status == "FAILED" && run.NotifyFirstFailure && run.ConsecutiveFailures == 0:
		event.Kind = automationnotification.FirstFailure
	default:
		return nil
	}
	return &event
}

func actionableState(state State) bool {
	return state == ReadyForPut || state == Cash || state == ReadyForCall || state == LongShares || state == AIMonitoring
}

func classifyScheduleError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrForbidden):
		return "FORBIDDEN"
	case errors.Is(err, ErrNotFound):
		return "NOT_FOUND"
	case errors.Is(err, ErrEvaluationInactive):
		return "STRATEGY_NOT_ACTIVE"
	case errors.Is(err, ErrEvaluationConfigurationChanged):
		return "STRATEGY_CONFIGURATION_CHANGED"
	case errors.Is(err, ErrEvaluationParametersInvalid):
		return "STRATEGY_PARAMETERS_INVALID"
	case errors.Is(err, ErrEvaluationPaperStateUnavailable):
		return "PAPER_STATE_UNAVAILABLE"
	case errors.Is(err, ErrEvaluationMarketDataStale):
		return "MARKET_DATA_STALE"
	case errors.Is(err, ErrEvaluationNoEligibleContracts):
		return "NO_ELIGIBLE_OPTION_CONTRACTS"
	case errors.Is(err, ErrInvalid):
		return "INVALID"
	case errors.Is(err, aiconnection.ErrRateLimit):
		return "AI_DECISION_BUDGET_EXHAUSTED"
	case neural.Code(err) == neural.RateLimited:
		return "AI_PROVIDER_RATE_LIMITED"
	case errors.Is(err, aiconnection.ErrInactive), errors.Is(err, aiconnection.ErrDisabled), errors.Is(err, aiconnection.ErrNotFound), neural.Code(err) == neural.AuthenticationFailed:
		return "AI_CONNECTION_UNAVAILABLE"
	case errors.Is(err, aiconnection.ErrProvider), neural.Code(err) == neural.ProviderUnavailable, neural.Code(err) == neural.Timeout:
		return "AI_PROVIDER_UNAVAILABLE"
	case errors.Is(err, ErrConflict):
		return "CONFLICT"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "CANCELED"
	}
	var providerError *financial.ProviderError
	if errors.As(err, &providerError) {
		return "PROVIDER"
	}
	return "INTERNAL"
}
