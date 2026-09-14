package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

func TestScheduledQuoteRejectionPreservesOnlyBoundedPreModelEvidence(t *testing.T) {
	now := time.Date(2026, 9, 14, 15, 0, 0, 0, time.UTC)
	notRealtime := false
	for _, test := range []struct {
		name      string
		cause     error
		quoteType string
		realtime  *bool
		observed  time.Time
		wantType  string
	}{
		{"fresh false NFL", ErrEvaluationMarketDataDelayed, "NFL", &notRealtime, now.Add(-time.Second), "NFL"},
		{"missing flag", ErrEvaluationMarketDataUnconfirmed, "NBBO", nil, now, "NBBO"},
		{"future time", ErrEvaluationMarketDataStale, "NBBO", nil, now.Add(time.Hour), "NBBO"},
		{"missing time and type", ErrEvaluationMarketDataStale, "", nil, time.Time{}, "UNAVAILABLE"},
		{"untrusted type", ErrEvaluationMarketDataInvalid, "secret-provider-payload", nil, now, "UNRECOGNIZED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := scheduledRun(AIMonitoring, now.Add(-time.Minute))
			run.Session = "CONTINUOUS"
			original := rejectSchwabQuote(test.cause, run.FinancialAccountID, "SPY",
				financial.Quote{QuoteType: test.quoteType, Realtime: test.realtime, ProviderTimestamp: test.observed}, now)
			if !errors.Is(original, test.cause) {
				t.Fatal("original rejection classification lost")
			}
			store := &scheduleStoreFake{run: run}
			evaluator := &scheduledEvaluatorFake{err: fmt.Errorf("evaluation: %w", original)}
			scheduler := NewScheduler(store, evaluator)
			scheduler.now = func() time.Time { return now }
			if claimed, err := scheduler.RunOnce(context.Background()); !claimed || err != nil {
				t.Fatal(err)
			}
			e := store.completion.QuoteRejection
			if e == nil || e.QuoteType != test.wantType || e.RejectionCode != classifyScheduleError(test.cause) ||
				e.FinancialAccountID != run.FinancialAccountID || e.RequestedSymbol != "SPY" ||
				!e.BeforeModel || store.completion.Status != "FAILED" || store.completion.AIDecision != "" ||
				store.completion.ExecutionStatus != "" {
				t.Fatalf("unsafe failure evidence: %#v", store.completion)
			}
			if test.observed.IsZero() != (e.ProviderObservedAt == nil) {
				t.Fatal("missing time inferred")
			}
			if e.ProviderObservedAt != nil && !e.ProviderObservedAt.Equal(test.observed) {
				t.Fatal("provider time changed")
			}
			body, err := json.Marshal(e)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "secret-provider-payload") || strings.Contains(original.Error(), "NFL") {
				t.Fatal("raw metadata leaked into errors or safe evidence")
			}
			if test.realtime != nil && (e.Realtime == nil || *e.Realtime != *test.realtime) {
				t.Fatal("false flag dropped")
			}
		})
	}
}

func TestQuoteDiagnosticFailureNeverReplacesTheOriginalSchedulerFailure(t *testing.T) {
	now := time.Date(2026, 9, 14, 15, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name, account, symbol string
		evaluated             time.Time
	}{
		{"foreign account", "other-account", "SPY", now},
		{"untrusted symbol", "account", "private payload", now},
		{"future evaluation", "account", "SPY", now.Add(time.Second)},
		{"past evaluation", "account", "SPY", now.Add(-time.Hour)},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := scheduledRun(AIMonitoring, now)
			run.Session = "CONTINUOUS"
			store := &scheduleStoreFake{run: run}
			evaluator := &scheduledEvaluatorFake{err: rejectSchwabQuote(ErrEvaluationMarketDataUnconfirmed, test.account, test.symbol, financial.Quote{}, test.evaluated)}
			scheduler := NewScheduler(store, evaluator)
			scheduler.now = func() time.Time { return now }
			if _, err := scheduler.RunOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			if store.completion.Status != "FAILED" || store.completion.ErrorCode != "MARKET_DATA_REALTIME_UNCONFIRMED" ||
				store.completion.QuoteRejection != nil {
				t.Fatalf("diagnostic changed primary result: %#v", store.completion)
			}
		})
	}
}
