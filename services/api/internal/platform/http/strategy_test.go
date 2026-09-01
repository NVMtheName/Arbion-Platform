package http

import (
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/aiconnection"
	"github.com/arbion/platform/services/api/internal/neural"
	"github.com/arbion/platform/services/api/internal/platform/config"
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

func TestAIPaperSpotFillCursorRoundTripsAndRejectsMalformedInput(t *testing.T) {
	want := &strategy.AIPaperSpotFillCursor{
		SimulatedAt: time.Date(2026, 8, 28, 19, 12, 30, 123, time.UTC),
		ID:          "33333333-3333-4333-8333-333333333333",
	}
	encoded := encodeAIPaperSpotFillCursor(want)
	got, err := decodeAIPaperSpotFillCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || !got.SimulatedAt.Equal(want.SimulatedAt) {
		t.Fatalf("AI Paper fill cursor changed during round trip: %#v", got)
	}
	if _, err = decodeAIPaperSpotFillCursor("not-base64"); !errors.Is(err, strategy.ErrInvalid) {
		t.Fatalf("malformed AI Paper fill cursor was accepted: %v", err)
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
		{strategy.ErrMandateStale, stdhttp.StatusConflict, "MANDATE_NOT_READY"},
		{strategy.ErrEvidenceNotReviewable, stdhttp.StatusConflict, "EVIDENCE_NOT_REVIEWABLE"},
		{strategy.ErrEvidenceSnapshotChanged, stdhttp.StatusConflict, "EVIDENCE_SNAPSHOT_CHANGED"},
		{strategy.ErrEvidenceReviewStepUp, stdhttp.StatusForbidden, "EVIDENCE_REVIEW_MFA_REQUIRED"},
		{strategy.ErrCapitalReservation, stdhttp.StatusUnprocessableEntity, "CAPITAL_RESERVATION_UNAVAILABLE"},
		{strategy.ErrAccountInUse, stdhttp.StatusConflict, "ACCOUNT_CAPITAL_IN_USE"},
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

func TestScheduleRunCursorRoundTripsAndRejectsMalformedInput(t *testing.T) {
	want := &strategy.ScheduleRunCursor{
		ScheduledFor: time.Date(2026, 8, 28, 1, 12, 30, 123, time.UTC),
		ID:           "11111111-1111-4111-8111-111111111111",
	}
	encoded := encodeScheduleRunCursor(want)
	got, err := decodeScheduleRunCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || !got.ScheduledFor.Equal(want.ScheduledFor) {
		t.Fatalf("schedule-run cursor changed during round trip: %#v", got)
	}
	for _, input := range []string{"not-base64", "e30", encodeScheduleRunCursor(&strategy.ScheduleRunCursor{ScheduledFor: time.Now(), ID: "not-a-uuid"})} {
		if _, err = decodeScheduleRunCursor(input); err == nil {
			t.Fatalf("malformed schedule-run cursor was accepted: %q", input)
		}
	}
}

func TestStrategyDecisionCursorRoundTripsAndRejectsMalformedInput(t *testing.T) {
	want := &strategy.StrategyDecisionCursor{
		CreatedAt: time.Date(2026, 8, 28, 6, 12, 30, 123, time.UTC),
		ID:        "11111111-1111-4111-8111-111111111111",
	}
	encoded := encodeStrategyDecisionCursor(want)
	got, err := decodeStrategyDecisionCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("strategy-decision cursor changed during round trip: %#v", got)
	}
	for _, input := range []string{
		"not-base64",
		"e30",
		strings.Repeat("a", 513),
		encodeStrategyDecisionCursor(&strategy.StrategyDecisionCursor{CreatedAt: time.Now(), ID: "not-a-uuid"}),
	} {
		if _, err = decodeStrategyDecisionCursor(input); err == nil {
			t.Fatalf("malformed strategy-decision cursor was accepted: %q", input)
		}
	}
}

func TestStrategyRuntimeCursorsRoundTripAndRejectMalformedInput(t *testing.T) {
	transition := &strategy.StrategyTransitionCursor{
		StateVersion: 7,
		ID:           "11111111-1111-4111-8111-111111111111",
	}
	encodedTransition := encodeStrategyTransitionCursor(transition)
	decodedTransition, err := decodeStrategyTransitionCursor(encodedTransition)
	if err != nil || decodedTransition.StateVersion != transition.StateVersion || decodedTransition.ID != transition.ID {
		t.Fatalf("strategy-transition cursor changed during round trip: %#v %v", decodedTransition, err)
	}
	execution := &strategy.StrategyExecutionCursor{
		CreatedAt: time.Date(2026, 8, 28, 7, 12, 30, 123, time.UTC),
		ID:        "22222222-2222-4222-8222-222222222222",
	}
	encodedExecution := encodeStrategyExecutionCursor(execution)
	decodedExecution, err := decodeStrategyExecutionCursor(encodedExecution)
	if err != nil || decodedExecution.ID != execution.ID || !decodedExecution.CreatedAt.Equal(execution.CreatedAt) {
		t.Fatalf("strategy-execution cursor changed during round trip: %#v %v", decodedExecution, err)
	}
	for _, input := range []string{
		"not-base64",
		"e30",
		strings.Repeat("a", 513),
		encodeStrategyTransitionCursor(&strategy.StrategyTransitionCursor{StateVersion: 0, ID: transition.ID}),
		encodeStrategyTransitionCursor(&strategy.StrategyTransitionCursor{StateVersion: 1, ID: "not-a-uuid"}),
	} {
		if _, err = decodeStrategyTransitionCursor(input); err == nil {
			t.Fatalf("malformed strategy-transition cursor was accepted: %q", input)
		}
	}
	for _, input := range []string{
		"not-base64",
		"e30",
		strings.Repeat("a", 513),
		encodeStrategyExecutionCursor(&strategy.StrategyExecutionCursor{CreatedAt: time.Now(), ID: "not-a-uuid"}),
	} {
		if _, err = decodeStrategyExecutionCursor(input); err == nil {
			t.Fatalf("malformed strategy-execution cursor was accepted: %q", input)
		}
	}
}

func TestShadowEvidenceReviewCursorRoundTripsAndRejectsMalformedInput(t *testing.T) {
	want := &strategy.ShadowEvidenceReviewCursor{
		ReviewedAt: time.Date(2026, 8, 28, 5, 12, 30, 123, time.UTC),
		ID:         "11111111-1111-4111-8111-111111111111",
	}
	encoded := encodeShadowEvidenceReviewCursor(want)
	got, err := decodeShadowEvidenceReviewCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || !got.ReviewedAt.Equal(want.ReviewedAt) {
		t.Fatalf("Shadow review cursor changed during round trip: %#v", got)
	}
	for _, input := range []string{"not-base64", "e30", encodeShadowEvidenceReviewCursor(&strategy.ShadowEvidenceReviewCursor{ReviewedAt: time.Now(), ID: "not-a-uuid"})} {
		if _, err = decodeShadowEvidenceReviewCursor(input); err == nil {
			t.Fatalf("malformed Shadow review cursor was accepted: %q", input)
		}
	}
}

func TestPaperEvidenceReviewCursorRoundTripsAndRejectsMalformedInput(t *testing.T) {
	want := &strategy.PaperEvidenceReviewCursor{
		ReviewedAt: time.Date(2026, 9, 5, 12, 30, 0, 0, time.UTC),
		ID:         "22222222-2222-4222-8222-222222222222",
	}
	encoded := encodePaperEvidenceReviewCursor(want)
	got, err := decodePaperEvidenceReviewCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || !got.ReviewedAt.Equal(want.ReviewedAt) {
		t.Fatalf("Paper review cursor changed during round trip: %#v", got)
	}
	for _, input := range []string{"not-base64", "e30", encodePaperEvidenceReviewCursor(&strategy.PaperEvidenceReviewCursor{ReviewedAt: time.Now(), ID: "not-a-uuid"})} {
		if _, err = decodePaperEvidenceReviewCursor(input); err == nil {
			t.Fatalf("malformed Paper review cursor was accepted: %q", input)
		}
	}
}

func TestShadowEvidenceReviewCommandRequiresTrustedOriginAndStrictJSON(t *testing.T) {
	handler := &authHandler{cfg: config.Auth{AllowedOrigins: []string{"https://www.arbion.ai"}}}
	withoutOrigin := httptest.NewRequest(stdhttp.MethodPost, "/api/strategy-instances/instance-1/shadow-evidence-reviews", strings.NewReader(`{"evidence_fingerprint":"`+strings.Repeat("a", 64)+`","confirm_non_live_review":true,"mfa_code":"123456"}`))
	withoutOrigin.SetPathValue("id", "instance-1")
	recorder := httptest.NewRecorder()
	handler.recordShadowEvidenceReview(recorder, withoutOrigin)
	if recorder.Code != stdhttp.StatusForbidden || !strings.Contains(recorder.Body.String(), "csrf_rejected") {
		t.Fatalf("untrusted review request returned %d: %s", recorder.Code, recorder.Body.String())
	}

	unknownField := httptest.NewRequest(stdhttp.MethodPost, "/api/strategy-instances/instance-1/shadow-evidence-reviews", strings.NewReader(`{"evidence_fingerprint":"`+strings.Repeat("a", 64)+`","confirm_non_live_review":true,"mfa_code":"123456","live_execution":true}`))
	unknownField.Header.Set("Origin", "https://www.arbion.ai")
	unknownField.SetPathValue("id", "instance-1")
	recorder = httptest.NewRecorder()
	handler.recordShadowEvidenceReview(recorder, unknownField)
	if recorder.Code != stdhttp.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid_request") {
		t.Fatalf("unrecognized review authority returned %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestPaperEvidenceReviewCommandRequiresTrustedOriginAndStrictJSON(t *testing.T) {
	handler := &authHandler{cfg: config.Auth{AllowedOrigins: []string{"https://www.arbion.ai"}}}
	withoutOrigin := httptest.NewRequest(stdhttp.MethodPost, "/api/strategy-instances/instance-1/paper-evidence-reviews", strings.NewReader(`{"evidence_fingerprint":"`+strings.Repeat("a", 64)+`","confirm_paper_review":true,"mfa_code":"123456"}`))
	withoutOrigin.SetPathValue("id", "instance-1")
	recorder := httptest.NewRecorder()
	handler.recordPaperEvidenceReview(recorder, withoutOrigin)
	if recorder.Code != stdhttp.StatusForbidden || !strings.Contains(recorder.Body.String(), "csrf_rejected") {
		t.Fatalf("untrusted Paper review request returned %d: %s", recorder.Code, recorder.Body.String())
	}

	unknownField := httptest.NewRequest(stdhttp.MethodPost, "/api/strategy-instances/instance-1/paper-evidence-reviews", strings.NewReader(`{"evidence_fingerprint":"`+strings.Repeat("a", 64)+`","confirm_paper_review":true,"mfa_code":"123456","live_promotion":true}`))
	unknownField.Header.Set("Origin", "https://www.arbion.ai")
	unknownField.SetPathValue("id", "instance-1")
	recorder = httptest.NewRecorder()
	handler.recordPaperEvidenceReview(recorder, unknownField)
	if recorder.Code != stdhttp.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid_request") {
		t.Fatalf("unrecognized Paper review authority returned %d: %s", recorder.Code, recorder.Body.String())
	}
}
