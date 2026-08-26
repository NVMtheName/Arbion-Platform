package http

import (
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/aiconnection"
	"github.com/arbion/platform/services/api/internal/neural"
	"github.com/arbion/platform/services/api/internal/strategy"
)

func TestDecisionJournalCursorRoundTrips(t *testing.T) {
	want := &strategy.JournalCursor{
		CreatedAt: time.Date(2026, 8, 18, 18, 12, 30, 123, time.UTC),
		ID:        "11111111-1111-4111-8111-111111111111",
	}
	encoded := encodeJournalCursor(want)
	got, err := decodeJournalCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("cursor changed during round trip: %#v", got)
	}
}

func TestStrategyErrorReturnsSafeEvaluationDiagnostics(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{strategy.ErrEvaluationInactive, stdhttp.StatusConflict, "STRATEGY_NOT_ACTIVE"},
		{strategy.ErrEvaluationConfigurationChanged, stdhttp.StatusConflict, "STRATEGY_CONFIGURATION_CHANGED"},
		{strategy.ErrEvaluationParametersInvalid, stdhttp.StatusUnprocessableEntity, "STRATEGY_PARAMETERS_INVALID"},
		{strategy.ErrEvaluationPaperStateUnavailable, stdhttp.StatusConflict, "PAPER_STATE_UNAVAILABLE"},
		{strategy.ErrEvaluationMarketDataStale, stdhttp.StatusUnprocessableEntity, "MARKET_DATA_STALE"},
		{strategy.ErrEvaluationNoEligibleContracts, stdhttp.StatusUnprocessableEntity, "NO_ELIGIBLE_OPTION_CONTRACTS"},
		{aiconnection.ErrRateLimit, stdhttp.StatusTooManyRequests, "AI_DECISION_BUDGET_EXHAUSTED"},
		{&neural.ProviderError{Code: neural.RateLimited}, stdhttp.StatusTooManyRequests, "AI_PROVIDER_RATE_LIMITED"},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			(&authHandler{}).strategyError(recorder, test.err)
			if recorder.Code != test.status {
				t.Fatalf("status=%d want=%d", recorder.Code, test.status)
			}
			var body apiError
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != test.code || body.Error.Message == "" {
				t.Fatalf("unexpected response: %#v", body)
			}
		})
	}
	if !errors.Is(strategy.ErrEvaluationMarketDataStale, strategy.ErrInvalid) {
		t.Fatal("diagnostic errors must preserve generic fail-closed classification")
	}
}

func TestDecisionJournalCursorRejectsMalformedInput(t *testing.T) {
	for _, input := range []string{"not-base64", "e30", encodeJournalCursor(&strategy.JournalCursor{CreatedAt: time.Now(), ID: "not-a-uuid"})} {
		if _, err := decodeJournalCursor(input); err == nil {
			t.Fatalf("malformed cursor was accepted: %q", input)
		}
	}
}
