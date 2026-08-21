package marketintelligence

import (
	"context"
	"errors"
	"testing"
	"time"
)

type healthStoreFake struct {
	outcomes []HealthOutcome
	buckets  []HealthBucket
	err      error
}

func (store *healthStoreFake) RecordOutcome(_ context.Context, outcome HealthOutcome) error {
	store.outcomes = append(store.outcomes, outcome)
	return store.err
}

func (store *healthStoreFake) Hourly(_ context.Context, _, _ time.Time) ([]HealthBucket, error) {
	return append([]HealthBucket(nil), store.buckets...), store.err
}

type failingEquityHealthProvider struct{ err error }

func (provider failingEquityHealthProvider) LatestEquityQuote(context.Context, string) (QuoteObservation, error) {
	return QuoteObservation{}, provider.err
}

func TestServicePersistsOnlySafeCompletedProviderOutcomes(t *testing.T) {
	store := &healthStoreFake{}
	service, err := NewService(ServiceConfig{
		HealthHistory:  store,
		EquityProvider: &fakeEquityProvider{}, EquitySourceID: "alpaca_iex",
		EquityCacheTTL: time.Minute, EquityInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 21, 20, 15, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	if _, _, err = service.LatestEquityQuote(context.Background(), "SPY"); err != nil {
		t.Fatal(err)
	}
	if len(store.outcomes) != 1 {
		t.Fatalf("expected one durable provider outcome, got %d", len(store.outcomes))
	}
	outcome := store.outcomes[0]
	if outcome.SourceID != "alpaca_iex" || outcome.Capability != EquityQuote || outcome.State != Verified || outcome.FailureCategory != "" || !outcome.ObservedAt.Equal(now) {
		t.Fatalf("unexpected durable success outcome: %+v", outcome)
	}
}

func TestServicePersistsSafeFailureCategoryWithoutAffectingProviderError(t *testing.T) {
	providerErr := errors.New("sensitive upstream response")
	store := &healthStoreFake{err: errors.New("history database unavailable")}
	service, err := NewService(ServiceConfig{
		HealthHistory:  store,
		EquityProvider: failingEquityHealthProvider{err: providerErr}, EquitySourceID: "alpaca_iex",
		EquityCacheTTL: time.Minute, EquityInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.LatestEquityQuote(context.Background(), "SPY"); !errors.Is(err, providerErr) {
		t.Fatalf("history failure changed the provider result: %v", err)
	}
	if len(store.outcomes) != 1 || store.outcomes[0].State != Degraded || store.outcomes[0].FailureCategory != "UPSTREAM_FAILURE" {
		t.Fatalf("provider error was not reduced to a safe category: %+v", store.outcomes)
	}
}

func TestServiceReturnsFixedBoundedHealthHistoryWindow(t *testing.T) {
	now := time.Date(2026, time.August, 21, 20, 15, 0, 0, time.UTC)
	store := &healthStoreFake{buckets: []HealthBucket{{SourceID: "sec_edgar", Capability: InsiderFiling, LastState: Verified}}}
	service, err := NewService(ServiceConfig{HealthHistory: store})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	history, err := service.SourceHealthHistory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if history.WindowHours != 24 || history.IntervalMinutes != 60 || !history.WindowStartedAt.Equal(time.Date(2026, time.August, 20, 21, 0, 0, 0, time.UTC)) || !history.WindowEndedAt.Equal(time.Date(2026, time.August, 21, 21, 0, 0, 0, time.UTC)) || len(history.Buckets) != 1 {
		t.Fatalf("unexpected durable history window: %+v", history)
	}
}

func TestHealthOutcomeRejectsSubjectOrUnsafeStateExpansion(t *testing.T) {
	now := time.Now().UTC()
	for _, outcome := range []HealthOutcome{
		{SourceID: "unknown", Capability: EquityQuote, State: Verified, ObservedAt: now},
		{SourceID: "alpaca_iex", Capability: CryptoTrades, State: Verified, ObservedAt: now},
		{SourceID: "alpaca_iex", Capability: EquityQuote, State: VerificationExpired, ObservedAt: now},
		{SourceID: "alpaca_iex", Capability: EquityQuote, State: Degraded, FailureCategory: "RAW_PROVIDER_ERROR", ObservedAt: now},
	} {
		if validHealthOutcome(outcome) {
			t.Fatalf("unsafe health outcome was accepted: %+v", outcome)
		}
	}
}
