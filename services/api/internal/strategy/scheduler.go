package strategy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automationnotification"
	"github.com/arbion/platform/services/api/internal/financial"
)

const (
	scheduleLeaseDuration = 2 * time.Minute
	schedulePollInterval  = 30 * time.Second
	maxClaimsPerPoll      = 10
)

type ScheduleStore interface {
	ClaimDueSchedule(context.Context, time.Time, time.Duration) (*ScheduledRun, error)
	CompleteSchedule(context.Context, ScheduledRun, ScheduleCompletion) error
}

type ScheduledEvaluator interface {
	Evaluate(context.Context, authorization.Principal, string, string) (EvaluationOutcome, error)
}

type Scheduler struct {
	store     ScheduleStore
	evaluator ScheduledEvaluator
	notifier  automationnotification.Sender
	now       func() time.Time
	logger    *slog.Logger
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
	completion := ScheduleCompletion{CompletedAt: now, NextRunAt: now.Add(time.Duration(run.IntervalMinutes) * time.Minute)}
	if run.ExecutionMode != Paper && run.ExecutionMode != Shadow {
		completion.Status, completion.ErrorCode = "FAILED", "UNSUPPORTED_MODE"
	} else if !inRegularSession(now) {
		if nextRunAt, ok := nextRegularSession(now); ok {
			completion.Status, completion.ErrorCode = "SKIPPED", "OUTSIDE_SESSION"
			completion.NextRunAt = nextRunAt
		} else {
			completion.Status, completion.ErrorCode = "FAILED", "SESSION_CALENDAR_UNAVAILABLE"
			completion.NextRunAt = now.Add(24 * time.Hour)
		}
	} else if !actionableState(run.CurrentState) {
		completion.Status, completion.ErrorCode = "SKIPPED", "WAITING_FOR_LIFECYCLE"
	} else {
		principal := authorization.Principal{UserID: run.UserID, Entitlement: authorization.EntitlementFounder}
		eventID := fmt.Sprintf("scheduled:%d", run.ScheduledFor.UTC().Unix())
		_, err = s.evaluator.Evaluate(ctx, principal, run.StrategyInstanceID, eventID)
		if err == nil || errors.Is(err, ErrDuplicate) {
			completion.Status = "SUCCEEDED"
		} else {
			completion.Status, completion.ErrorCode = "FAILED", classifyScheduleError(err)
		}
	}
	if err := s.store.CompleteSchedule(ctx, *run, completion); err != nil {
		return true, err
	}
	if event := scheduleNotification(*run, completion); event != nil && s.notifier != nil {
		if err := s.notifier.Send(ctx, *event); err != nil {
			s.logger.Error("non-live schedule notification delivery failed", "strategy_instance_id", run.StrategyInstanceID, "notification_kind", event.Kind)
		} else {
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
		Recipient:     run.OwnerEmail,
		MandateID:     run.MandateID,
		ExecutionMode: string(run.ExecutionMode),
		ScheduledFor:  run.ScheduledFor,
		SafeErrorCode: completion.ErrorCode,
	}
	switch {
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
	return state == ReadyForPut || state == Cash || state == ReadyForCall || state == LongShares
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
