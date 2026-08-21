package marketintelligence

import (
	"context"
	"errors"
	"time"
)

const (
	healthStorageInterval = 5 * time.Minute
	healthViewInterval    = time.Hour
	healthHistoryWindow   = 24 * time.Hour
	healthRecordTimeout   = 250 * time.Millisecond
	healthReadTimeout     = 500 * time.Millisecond
)

var ErrHealthHistoryUnavailable = errors.New("market source health history unavailable")

// HealthOutcome is one completed, provider-level result. It deliberately has
// no user, account, instrument, request, provider-ID, or raw-error field.
type HealthOutcome struct {
	SourceID        string
	Capability      Capability
	State           VerificationState
	FailureCategory string
	ObservedAt      time.Time
}

// HealthBucket is a durable, hourly aggregate of completed provider outcomes.
// Missing buckets mean no outcome was recorded, not that a provider failed.
type HealthBucket struct {
	SourceID          string            `json:"source_id"`
	Capability        Capability        `json:"capability"`
	IntervalStarted   time.Time         `json:"interval_started_at"`
	LastObservedAt    time.Time         `json:"last_observed_at"`
	CompletedAttempts uint64            `json:"completed_attempts"`
	Successes         uint64            `json:"successes"`
	Failures          uint64            `json:"failures"`
	LastState         VerificationState `json:"last_state"`
	FailureCategory   string            `json:"failure_category,omitempty"`
}

type HealthHistory struct {
	Buckets         []HealthBucket `json:"buckets"`
	WindowStartedAt time.Time      `json:"window_started_at"`
	WindowEndedAt   time.Time      `json:"window_ended_at"`
	WindowHours     int            `json:"window_hours"`
	IntervalMinutes int            `json:"interval_minutes"`
}

type HealthHistoryStore interface {
	RecordOutcome(context.Context, HealthOutcome) error
	Hourly(context.Context, time.Time, time.Time) ([]HealthBucket, error)
}

func (service *Service) SourceHealthHistory(ctx context.Context) (HealthHistory, error) {
	if service.healthHistory == nil {
		return HealthHistory{}, ErrHealthHistoryUnavailable
	}
	now := service.now()
	windowEnd := now.Truncate(healthViewInterval).Add(healthViewInterval)
	windowStart := windowEnd.Add(-healthHistoryWindow)
	readContext, cancel := context.WithTimeout(ctx, healthReadTimeout)
	defer cancel()
	buckets, err := service.healthHistory.Hourly(readContext, windowStart, windowEnd)
	if err != nil {
		return HealthHistory{}, ErrHealthHistoryUnavailable
	}
	return HealthHistory{
		Buckets:         buckets,
		WindowStartedAt: windowStart,
		WindowEndedAt:   windowEnd,
		WindowHours:     int(healthHistoryWindow / time.Hour),
		IntervalMinutes: int(healthViewInterval / time.Minute),
	}, nil
}

func (service *Service) persistHealthOutcome(outcome HealthOutcome) {
	if service.healthHistory == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), healthRecordTimeout)
	defer cancel()
	// Source reads fail open when operational history is unavailable. Current
	// process status still records the outcome and the API never substitutes data.
	_ = service.healthHistory.RecordOutcome(ctx, outcome)
}

func validHealthOutcome(outcome HealthOutcome) bool {
	if outcome.ObservedAt.IsZero() || (outcome.State != Verified && outcome.State != Degraded) {
		return false
	}
	if outcome.State == Verified && outcome.FailureCategory != "" {
		return false
	}
	if outcome.State == Degraded && !validFailureCategory(outcome.FailureCategory) {
		return false
	}
	for _, source := range DefaultSources() {
		if source.ID != outcome.SourceID {
			continue
		}
		for _, capability := range source.Capabilities {
			if capability == outcome.Capability {
				return true
			}
		}
	}
	return false
}

func validFailureCategory(category string) bool {
	switch category {
	case "TIMEOUT", "STALE_DATA", "FUTURE_DATED_DATA", "MISSING_PROVENANCE", "INVALID_DATA", "UPSTREAM_FAILURE":
		return true
	default:
		return false
	}
}
