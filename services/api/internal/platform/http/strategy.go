package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/arbion/platform/services/api/internal/aiconnection"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/neural"
	"github.com/arbion/platform/services/api/internal/strategy"
)

const (
	defaultJournalPageSize              = 25
	defaultStrategyDecisionPageSize     = 24
	defaultStrategyRuntimePageSize      = 16
	defaultShadowEvidenceReviewPageSize = 8
)

var journalUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

type journalCursorPayload struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

type scheduleRunCursorPayload struct {
	ScheduledFor time.Time `json:"scheduled_for"`
	ID           string    `json:"id"`
}

type strategyDecisionCursorPayload struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

type strategyTransitionCursorPayload struct {
	StateVersion int    `json:"state_version"`
	ID           string `json:"id"`
}

type strategyExecutionCursorPayload struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

type shadowEvidenceReviewCursorPayload struct {
	ReviewedAt time.Time `json:"reviewed_at"`
	ID         string    `json:"id"`
}

type aiPaperSpotFillCursorPayload struct {
	SimulatedAt time.Time `json:"simulated_at"`
	ID          string    `json:"id"`
}

func registerStrategyRoutes(m *stdhttp.ServeMux, h *authHandler) {
	if h.strategies == nil {
		return
	}
	m.Handle("POST /api/automations/{id}/strategy/initialize", h.require(stdhttp.HandlerFunc(h.initializeStrategy)))
	m.Handle("GET /api/strategy-instances", h.require(stdhttp.HandlerFunc(h.listStrategyInstances)))
	m.Handle("GET /api/strategy-capital-reservations", h.require(stdhttp.HandlerFunc(h.listStrategyCapitalReservations)))
	m.Handle("GET /api/strategy-instances/{id}", h.require(stdhttp.HandlerFunc(h.getStrategyInstance)))
	m.Handle("GET /api/strategy-instances/{id}/capital-reservation", h.require(stdhttp.HandlerFunc(h.strategyCapitalReservation)))
	m.Handle("GET /api/strategy-instances/{id}/history", h.require(stdhttp.HandlerFunc(h.strategyHistory)))
	m.Handle("GET /api/strategy-instances/{id}/decisions", h.require(stdhttp.HandlerFunc(h.strategyDecisions)))
	m.Handle("GET /api/strategy-instances/{id}/executions", h.require(stdhttp.HandlerFunc(h.strategyExecutions)))
	m.Handle("GET /api/strategy-instances/{id}/shadow-outcomes", h.require(stdhttp.HandlerFunc(h.strategyShadowOutcomes)))
	m.Handle("GET /api/strategy-instances/{id}/shadow-scorecard", h.require(stdhttp.HandlerFunc(h.strategyShadowScorecard)))
	m.Handle("GET /api/strategy-instances/{id}/shadow-evidence-reviews", h.require(stdhttp.HandlerFunc(h.strategyShadowEvidenceReviews)))
	m.Handle("POST /api/strategy-instances/{id}/shadow-evidence-reviews", h.require(stdhttp.HandlerFunc(h.recordShadowEvidenceReview)))
	m.Handle("GET /api/strategy-instances/{id}/paper-portfolio", h.require(stdhttp.HandlerFunc(h.strategyPaperPortfolio)))
	m.Handle("GET /api/strategy-instances/{id}/ai-paper-fills", h.require(stdhttp.HandlerFunc(h.strategyAIPaperFills)))
	m.Handle("GET /api/strategy-instances/{id}/schedule", h.require(stdhttp.HandlerFunc(h.strategySchedule)))
	m.Handle("GET /api/strategy-instances/{id}/schedule-runs", h.require(stdhttp.HandlerFunc(h.strategyScheduleRuns)))
	m.Handle("POST /api/strategy-instances/{id}/pause", h.require(stdhttp.HandlerFunc(h.pauseStrategyInstance)))
	m.Handle("POST /api/strategy-instances/{id}/resume", h.require(stdhttp.HandlerFunc(h.resumeStrategyInstance)))
	m.Handle("POST /api/strategy-instances/{id}/finish", h.require(stdhttp.HandlerFunc(h.finishStrategyInstance)))
	m.Handle("POST /api/strategy-instances/{id}/lifecycle-events", h.require(stdhttp.HandlerFunc(h.recordStrategyLifecycle)))
	m.Handle("GET /api/decision-journal", h.require(stdhttp.HandlerFunc(h.decisionJournal)))
	if h.evaluations != nil {
		m.Handle("POST /api/strategy-instances/{id}/evaluate", h.require(stdhttp.HandlerFunc(h.evaluateStrategy)))
	}
}

func encodeJournalCursor(cursor *strategy.JournalCursor) string {
	if cursor == nil {
		return ""
	}
	payload, _ := json.Marshal(journalCursorPayload{CreatedAt: cursor.CreatedAt, ID: cursor.ID})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeJournalCursor(encoded string) (*strategy.JournalCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	if len(encoded) > 512 {
		return nil, strategy.ErrInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, strategy.ErrInvalid
	}
	var decoded journalCursorPayload
	if err = json.Unmarshal(payload, &decoded); err != nil || decoded.CreatedAt.IsZero() || !journalUUID.MatchString(decoded.ID) {
		return nil, strategy.ErrInvalid
	}
	return &strategy.JournalCursor{CreatedAt: decoded.CreatedAt, ID: decoded.ID}, nil
}

func encodeAIPaperSpotFillCursor(cursor *strategy.AIPaperSpotFillCursor) string {
	if cursor == nil {
		return ""
	}
	payload, _ := json.Marshal(aiPaperSpotFillCursorPayload{SimulatedAt: cursor.SimulatedAt, ID: cursor.ID})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeAIPaperSpotFillCursor(encoded string) (*strategy.AIPaperSpotFillCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	if len(encoded) > 512 {
		return nil, strategy.ErrInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, strategy.ErrInvalid
	}
	var decoded aiPaperSpotFillCursorPayload
	if err = json.Unmarshal(payload, &decoded); err != nil || decoded.SimulatedAt.IsZero() || !journalUUID.MatchString(decoded.ID) {
		return nil, strategy.ErrInvalid
	}
	return &strategy.AIPaperSpotFillCursor{SimulatedAt: decoded.SimulatedAt, ID: decoded.ID}, nil
}

func encodeScheduleRunCursor(cursor *strategy.ScheduleRunCursor) string {
	if cursor == nil {
		return ""
	}
	payload, _ := json.Marshal(scheduleRunCursorPayload{ScheduledFor: cursor.ScheduledFor, ID: cursor.ID})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeScheduleRunCursor(encoded string) (*strategy.ScheduleRunCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	if len(encoded) > 512 {
		return nil, strategy.ErrInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, strategy.ErrInvalid
	}
	var decoded scheduleRunCursorPayload
	if err = json.Unmarshal(payload, &decoded); err != nil || decoded.ScheduledFor.IsZero() || !journalUUID.MatchString(decoded.ID) {
		return nil, strategy.ErrInvalid
	}
	return &strategy.ScheduleRunCursor{ScheduledFor: decoded.ScheduledFor, ID: decoded.ID}, nil
}

func encodeStrategyDecisionCursor(cursor *strategy.StrategyDecisionCursor) string {
	if cursor == nil {
		return ""
	}
	payload, _ := json.Marshal(strategyDecisionCursorPayload{CreatedAt: cursor.CreatedAt, ID: cursor.ID})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeStrategyDecisionCursor(encoded string) (*strategy.StrategyDecisionCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	if len(encoded) > 512 {
		return nil, strategy.ErrInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, strategy.ErrInvalid
	}
	var decoded strategyDecisionCursorPayload
	if err = json.Unmarshal(payload, &decoded); err != nil || decoded.CreatedAt.IsZero() || !journalUUID.MatchString(decoded.ID) {
		return nil, strategy.ErrInvalid
	}
	return &strategy.StrategyDecisionCursor{CreatedAt: decoded.CreatedAt, ID: decoded.ID}, nil
}

func encodeStrategyTransitionCursor(cursor *strategy.StrategyTransitionCursor) string {
	if cursor == nil {
		return ""
	}
	payload, _ := json.Marshal(strategyTransitionCursorPayload{StateVersion: cursor.StateVersion, ID: cursor.ID})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeStrategyTransitionCursor(encoded string) (*strategy.StrategyTransitionCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	if len(encoded) > 512 {
		return nil, strategy.ErrInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, strategy.ErrInvalid
	}
	var decoded strategyTransitionCursorPayload
	if err = json.Unmarshal(payload, &decoded); err != nil || decoded.StateVersion < 1 || !journalUUID.MatchString(decoded.ID) {
		return nil, strategy.ErrInvalid
	}
	return &strategy.StrategyTransitionCursor{StateVersion: decoded.StateVersion, ID: decoded.ID}, nil
}

func encodeStrategyExecutionCursor(cursor *strategy.StrategyExecutionCursor) string {
	if cursor == nil {
		return ""
	}
	payload, _ := json.Marshal(strategyExecutionCursorPayload{CreatedAt: cursor.CreatedAt, ID: cursor.ID})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeStrategyExecutionCursor(encoded string) (*strategy.StrategyExecutionCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	if len(encoded) > 512 {
		return nil, strategy.ErrInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, strategy.ErrInvalid
	}
	var decoded strategyExecutionCursorPayload
	if err = json.Unmarshal(payload, &decoded); err != nil || decoded.CreatedAt.IsZero() || !journalUUID.MatchString(decoded.ID) {
		return nil, strategy.ErrInvalid
	}
	return &strategy.StrategyExecutionCursor{CreatedAt: decoded.CreatedAt, ID: decoded.ID}, nil
}

func encodeShadowEvidenceReviewCursor(cursor *strategy.ShadowEvidenceReviewCursor) string {
	if cursor == nil {
		return ""
	}
	payload, _ := json.Marshal(shadowEvidenceReviewCursorPayload{ReviewedAt: cursor.ReviewedAt, ID: cursor.ID})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeShadowEvidenceReviewCursor(encoded string) (*strategy.ShadowEvidenceReviewCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	if len(encoded) > 512 {
		return nil, strategy.ErrInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, strategy.ErrInvalid
	}
	var decoded shadowEvidenceReviewCursorPayload
	if err = json.Unmarshal(payload, &decoded); err != nil || decoded.ReviewedAt.IsZero() || !journalUUID.MatchString(decoded.ID) {
		return nil, strategy.ErrInvalid
	}
	return &strategy.ShadowEvidenceReviewCursor{ReviewedAt: decoded.ReviewedAt, ID: decoded.ID}, nil
}

func (h *authHandler) decisionJournal(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	limit := defaultJournalPageSize
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, 400, "INVALID_PAGINATION", "Limit must be between 1 and 100.")
			return
		}
		limit = parsed
	}
	cursor, err := decodeJournalCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, 400, "INVALID_PAGINATION", "The journal cursor is invalid.")
		return
	}
	page, err := h.strategies.Journal(r.Context(), principal(r), limit, cursor)
	if err != nil {
		h.strategyError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"entries":                  page.Entries,
		"next_cursor":              encodeJournalCursor(page.NextCursor),
		"live_execution_available": false,
	})
}
func (h *authHandler) strategyError(w stdhttp.ResponseWriter, e error) {
	switch {
	case errors.Is(e, strategy.ErrForbidden):
		writeError(w, 403, "PERMISSION_DENIED", "Automation entitlement is required.")
	case errors.Is(e, strategy.ErrEvaluationInactive):
		writeError(w, 409, "STRATEGY_NOT_ACTIVE", "The non-live strategy must be active before it can be evaluated.")
	case errors.Is(e, strategy.ErrEvaluationConfigurationChanged):
		writeError(w, 409, "STRATEGY_CONFIGURATION_CHANGED", "The initialized strategy no longer matches its current mandate, capital bucket, or account.")
	case errors.Is(e, strategy.ErrEvaluationParametersInvalid):
		writeError(w, 422, "STRATEGY_PARAMETERS_INVALID", "The saved deterministic strategy parameters are invalid.")
	case errors.Is(e, strategy.ErrEvaluationPaperStateUnavailable):
		writeError(w, 409, "PAPER_STATE_UNAVAILABLE", "The PAPER portfolio state required for evaluation is unavailable.")
	case errors.Is(e, strategy.ErrEvaluationMarketDataStale):
		writeError(w, 422, "MARKET_DATA_STALE", "Schwab market data is missing a current provider timestamp.")
	case errors.Is(e, strategy.ErrEvaluationNoEligibleContracts):
		writeError(w, 422, "NO_ELIGIBLE_OPTION_CONTRACTS", "No option contract matched the saved symbol, expiration, delta, and premium filters.")
	case errors.Is(e, aiconnection.ErrRateLimit):
		writeError(w, 429, "AI_DECISION_BUDGET_EXHAUSTED", "Arbion's hourly AI decision budget is exhausted. Wait for the budget window to reset; no broker order was sent.")
	case neural.Code(e) == neural.RateLimited:
		writeError(w, 429, "AI_PROVIDER_RATE_LIMITED", "The AI provider is temporarily rate limited. Try again later; no broker order was sent.")
	case errors.Is(e, aiconnection.ErrInactive), errors.Is(e, aiconnection.ErrDisabled), errors.Is(e, aiconnection.ErrNotFound), neural.Code(e) == neural.AuthenticationFailed:
		writeError(w, 409, "AI_CONNECTION_UNAVAILABLE", "The mandate's AI connection is not currently usable. No broker order was sent.")
	case errors.Is(e, aiconnection.ErrProvider), neural.Code(e) == neural.ProviderUnavailable, neural.Code(e) == neural.Timeout:
		writeError(w, 503, "AI_PROVIDER_UNAVAILABLE", "The AI provider is temporarily unavailable. No broker order was sent.")
	case errors.Is(e, strategy.ErrInvalid):
		writeError(w, 422, "INVALID_STRATEGY", "The saved non-live automation request is invalid or unsupported.")
	case errors.Is(e, strategy.ErrCapitalLimit):
		writeError(w, 422, "PAPER_CAPITAL_LIMIT", "Starting simulated cash exceeds the selected capital bucket's protected capacity.")
	case errors.Is(e, strategy.ErrCapitalReservation):
		writeError(w, 422, "CAPITAL_RESERVATION_UNAVAILABLE", "The selected capital bucket cannot establish an exact non-live capital reservation. Percentage-based Shadow buckets require an absolute cap.")
	case errors.Is(e, strategy.ErrAccountInUse):
		writeError(w, 409, "ACCOUNT_CAPITAL_IN_USE", "The requested capital would overlap an active or paused non-live reservation. Use distinct fixed-amount buckets with the same explicit account ceiling, or finish the existing strategy.")
	case errors.Is(e, strategy.ErrOpenExposure):
		writeError(w, 409, "PAPER_POSITION_OPEN", "Resolve every open simulated position before finishing this PAPER strategy.")
	case errors.Is(e, strategy.ErrMandateStale):
		writeError(w, 409, "MANDATE_NOT_READY", "The strategy's pinned mandate version is not eligible for this non-live action.")
	case errors.Is(e, strategy.ErrEvidenceNotReviewable):
		writeError(w, 409, "EVIDENCE_NOT_REVIEWABLE", "This exact Shadow evidence snapshot has not reached the durable review gate.")
	case errors.Is(e, strategy.ErrEvidenceSnapshotChanged):
		writeError(w, 409, "EVIDENCE_SNAPSHOT_CHANGED", "The Shadow evidence changed. Refresh and review the current snapshot before recording your acknowledgment.")
	case errors.Is(e, strategy.ErrEvidenceReviewStepUp):
		writeError(w, 403, "EVIDENCE_REVIEW_MFA_REQUIRED", "Enter a fresh code from your authenticator app to record this non-live evidence review.")
	case errors.Is(e, strategy.ErrConflict):
		writeError(w, 409, "STRATEGY_CONFLICT", "The strategy state changed or this request conflicts with an existing record.")
	case errors.Is(e, strategy.ErrDuplicate):
		writeError(w, 409, "EVALUATION_DUPLICATE", "This manual evaluation was already recorded.")
	default:
		var providerError *financial.ProviderError
		if errors.As(e, &providerError) {
			h.financialError(w, e)
			return
		}
		writeError(w, 404, "NOT_FOUND", "The requested strategy resource was not found.")
	}
}
func (h *authHandler) initializeStrategy(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var x struct {
		StartingCash string `json:"starting_cash"`
	}
	if !decode(w, r, &x) {
		return
	}
	v, e := h.strategies.Initialize(r.Context(), principal(r), r.PathValue("id"), x.StartingCash)
	if e != nil {
		h.strategyError(w, e)
		return
	}
	writeJSON(w, 201, map[string]any{"strategy_instance": v, "live_execution_available": false})
}
func (h *authHandler) listStrategyInstances(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.strategies.List(r.Context(), principal(r))
	if e != nil {
		h.strategyError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"strategy_instances": v, "live_execution_available": false})
}
func (h *authHandler) getStrategyInstance(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.strategies.Get(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.strategyError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"strategy_instance": v, "live_execution_available": false})
}
func (h *authHandler) listStrategyCapitalReservations(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.strategies.CapitalReservations(r.Context(), principal(r))
	if e != nil {
		h.strategyError(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"capital_reservations":        nonNil(v),
		"reservation_scope":           "NONLIVE_STRATEGY_ONLY",
		"broker_funds_locked":         false,
		"broker_action_available":     false,
		"live_execution_available":    false,
		"execution_authority_granted": false,
	})
}
func (h *authHandler) strategyCapitalReservation(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.strategies.CapitalReservation(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.strategyError(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"capital_reservation":         v,
		"reservation_scope":           "NONLIVE_STRATEGY_ONLY",
		"broker_funds_locked":         false,
		"broker_action_available":     false,
		"live_execution_available":    false,
		"execution_authority_granted": false,
	})
}
func (h *authHandler) strategyHistory(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	limit := defaultStrategyRuntimePageSize
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 50 {
			writeError(w, 400, "INVALID_PAGINATION", "Limit must be between 1 and 50.")
			return
		}
		limit = parsed
	}
	cursor, err := decodeStrategyTransitionCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, 400, "INVALID_PAGINATION", "The strategy transition cursor is invalid.")
		return
	}
	page, err := h.strategies.TransitionPage(r.Context(), principal(r), r.PathValue("id"), limit, cursor)
	if err != nil {
		h.strategyError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"transitions":              page.Transitions,
		"next_cursor":              encodeStrategyTransitionCursor(page.NextCursor),
		"history_semantics":        "IMMUTABLE_OWNER_STRATEGY_STATE_HISTORY",
		"ordering":                 "NEWEST_FIRST",
		"state_mutation_available": false,
		"broker_action_available":  false,
		"live_execution_available": false,
	})
}
func (h *authHandler) strategyDecisions(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	limit := defaultStrategyDecisionPageSize
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 50 {
			writeError(w, 400, "INVALID_PAGINATION", "Limit must be between 1 and 50.")
			return
		}
		limit = parsed
	}
	cursor, err := decodeStrategyDecisionCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, 400, "INVALID_PAGINATION", "The strategy decision cursor is invalid.")
		return
	}
	page, err := h.strategies.DecisionPage(r.Context(), principal(r), r.PathValue("id"), limit, cursor)
	if err != nil {
		h.strategyError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"decisions":                   page.Decisions,
		"outcomes":                    page.Outcomes,
		"next_cursor":                 encodeStrategyDecisionCursor(page.NextCursor),
		"decision_history_semantics":  "IMMUTABLE_OWNER_STRATEGY_DECISION_HISTORY",
		"outcome_semantics":           "MATCHED_HYPOTHETICAL_DIRECTIONAL_MARKS",
		"fees_and_slippage_included":  false,
		"model_rerun":                 false,
		"financial_provider_called":   false,
		"broker_action_available":     false,
		"live_execution_available":    false,
		"execution_authority_granted": false,
	})
}
func (h *authHandler) strategyExecutions(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	limit := defaultStrategyRuntimePageSize
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 50 {
			writeError(w, 400, "INVALID_PAGINATION", "Limit must be between 1 and 50.")
			return
		}
		limit = parsed
	}
	cursor, err := decodeStrategyExecutionCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, 400, "INVALID_PAGINATION", "The strategy execution cursor is invalid.")
		return
	}
	page, err := h.strategies.ExecutionPage(r.Context(), principal(r), r.PathValue("id"), limit, cursor)
	if err != nil {
		h.strategyError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"executions":                  page.Executions,
		"next_cursor":                 encodeStrategyExecutionCursor(page.NextCursor),
		"history_semantics":           "IMMUTABLE_OWNER_NONLIVE_EXECUTION_HISTORY",
		"execution_boundary":          "PAPER_OR_SHADOW_ONLY",
		"broker_order_record":         false,
		"broker_action_available":     false,
		"live_execution_available":    false,
		"execution_authority_granted": false,
	})
}

func (h *authHandler) strategyShadowOutcomes(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.strategies.ShadowOutcomes(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.strategyError(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"outcomes":                   v,
		"performance_semantics":      "HYPOTHETICAL_DIRECTIONAL_MARK",
		"fees_and_slippage_included": false,
		"live_execution_available":   false,
	})
}

func (h *authHandler) strategyShadowScorecard(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.strategies.ShadowScorecard(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.strategyError(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"scorecard":                      v,
		"performance_semantics":          "HYPOTHETICAL_DIRECTIONAL_EVIDENCE",
		"prediction_accuracy_claimed":    false,
		"fees_and_slippage_included":     false,
		"realized_performance_available": false,
		"evidence_gate_grants_authority": false,
		"live_promotion_available":       false,
		"live_execution_available":       false,
	})
}

func (h *authHandler) recordShadowEvidenceReview(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var command strategy.ShadowEvidenceReviewCommand
	if !decode(w, r, &command) {
		return
	}
	review, err := h.strategies.RecordShadowEvidenceReview(r.Context(), principal(r), r.PathValue("id"), command)
	if err != nil {
		h.strategyError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 201, map[string]any{
		"evidence_review":                  review,
		"review_scope":                     strategy.ShadowEvidenceReviewScope,
		"evidence_review_grants_authority": false,
		"broker_action_available":          false,
		"live_promotion_available":         false,
		"live_execution_available":         false,
	})
}

func (h *authHandler) strategyShadowEvidenceReviews(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	limit := defaultShadowEvidenceReviewPageSize
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 50 {
			writeError(w, 400, "INVALID_PAGINATION", "Limit must be between 1 and 50.")
			return
		}
		limit = parsed
	}
	cursor, err := decodeShadowEvidenceReviewCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, 400, "INVALID_PAGINATION", "The Shadow evidence review cursor is invalid.")
		return
	}
	page, err := h.strategies.ShadowEvidenceReviews(r.Context(), principal(r), r.PathValue("id"), limit, cursor)
	if err != nil {
		h.strategyError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"evidence_reviews":                 page.Reviews,
		"next_cursor":                      encodeShadowEvidenceReviewCursor(page.NextCursor),
		"history_semantics":                "IMMUTABLE_NONLIVE_OWNER_REVIEW_EVIDENCE",
		"review_scope":                     strategy.ShadowEvidenceReviewScope,
		"evidence_review_grants_authority": false,
		"broker_action_available":          false,
		"live_promotion_available":         false,
		"live_execution_available":         false,
	})
}

func (h *authHandler) strategyPaperPortfolio(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.strategies.PaperPortfolio(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.strategyError(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"paper_portfolio":                 v,
		"realized_outcome_semantics":      "EXACT_IMMUTABLE_AVERAGE_COST_SIMULATION",
		"realized_outcome_includes_fees":  true,
		"execution_cost_semantics":        "EXACT_IMMUTABLE_SIMULATION_FEES_AND_ADVERSE_SLIPPAGE",
		"execution_costs_broker_reported": false,
		"activity_cadence_semantics":      "EXACT_IMMUTABLE_SCHEDULE_AND_SIMULATION_CHRONOLOGY",
		"activity_cadence_read_only":      true,
		"disposition_funnel_semantics":    "EXACT_IMMUTABLE_PAPER_EVALUATION_DISPOSITION_FUNNEL",
		"disposition_funnel_read_only":    true,
		"guardrail_evidence_semantics":    "EXACT_IMMUTABLE_PAPER_PROPOSAL_RISK_AND_SIMULATION_ATTRIBUTION",
		"guardrail_evidence_read_only":    true,
		"guardrail_coverage_semantics":    "EXACT_ORDERED_PAPER_CHECK_PLAN_AND_FAIL_CLOSED_PREFIX_ATTESTATION",
		"guardrail_coverage_read_only":    true,
		"broker_action_available":         false,
		"live_execution_available":        false,
	})
}

func (h *authHandler) strategyAIPaperFills(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	limit := 25
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, 400, "INVALID_PAGINATION", "Limit must be between 1 and 100.")
			return
		}
		limit = parsed
	}
	cursor, err := decodeAIPaperSpotFillCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, 400, "INVALID_PAGINATION", "The AI Paper fill cursor is invalid.")
		return
	}
	page, err := h.strategies.AIPaperSpotFills(r.Context(), principal(r), r.PathValue("id"), limit, cursor)
	if err != nil {
		h.strategyError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"fills":                       page.Fills,
		"next_cursor":                 encodeAIPaperSpotFillCursor(page.NextCursor),
		"history_semantics":           "IMMUTABLE_OWNER_AI_PAPER_SIMULATED_FILL_HISTORY",
		"pricing_includes_slippage":   true,
		"fees_included":               true,
		"provider_market_provenance":  true,
		"simulation_only":             true,
		"broker_order_record":         false,
		"broker_action_available":     false,
		"live_execution_available":    false,
		"execution_authority_granted": false,
	})
}

func (h *authHandler) strategySchedule(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.strategies.Schedule(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.strategyError(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"schedule": v, "scheduler_enabled": h.schedulerEnabled, "email_delivery_available": h.emailDeliveryAvailable, "live_execution_available": false})
}

func (h *authHandler) strategyScheduleRuns(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	limit := 20
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, 400, "INVALID_PAGINATION", "Limit must be between 1 and 100.")
			return
		}
		limit = parsed
	}
	cursor, err := decodeScheduleRunCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, 400, "INVALID_PAGINATION", "The schedule-run cursor is invalid.")
		return
	}
	page, err := h.strategies.ScheduleRuns(r.Context(), principal(r), r.PathValue("id"), limit, cursor)
	if err != nil {
		h.strategyError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"runs":                     page.Runs,
		"next_cursor":              encodeScheduleRunCursor(page.NextCursor),
		"history_semantics":        "IMMUTABLE_NONLIVE_SCHEDULER_EVIDENCE",
		"broker_action_available":  false,
		"live_execution_available": false,
	})
}

func (h *authHandler) pauseStrategyInstance(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var input struct {
		ExpectedStateVersion int `json:"expected_state_version"`
	}
	if !decode(w, r, &input) {
		return
	}
	instance, err := h.strategies.Pause(r.Context(), principal(r), r.PathValue("id"), input.ExpectedStateVersion)
	if err != nil {
		h.strategyError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"strategy_instance": instance, "live_execution_available": false})
}

func (h *authHandler) resumeStrategyInstance(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var input struct {
		ExpectedStateVersion int  `json:"expected_state_version"`
		ConfirmNonLiveResume bool `json:"confirm_non_live_resume"`
	}
	if !decode(w, r, &input) {
		return
	}
	instance, err := h.strategies.Resume(r.Context(), principal(r), r.PathValue("id"), input.ExpectedStateVersion, input.ConfirmNonLiveResume)
	if err != nil {
		h.strategyError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"strategy_instance": instance, "live_execution_available": false})
}

func (h *authHandler) finishStrategyInstance(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var input struct {
		ExpectedStateVersion int  `json:"expected_state_version"`
		ConfirmNonLiveFinish bool `json:"confirm_non_live_finish"`
	}
	if !decode(w, r, &input) {
		return
	}
	instance, err := h.strategies.Finish(r.Context(), principal(r), r.PathValue("id"), input.ExpectedStateVersion, input.ConfirmNonLiveFinish)
	if err != nil {
		h.strategyError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"strategy_instance": instance, "live_execution_available": false})
}

func (h *authHandler) recordStrategyLifecycle(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var command strategy.LifecycleCommand
	if !decode(w, r, &command) {
		return
	}
	result, err := h.strategies.RecordLifecycle(r.Context(), principal(r), r.PathValue("id"), command)
	if err != nil {
		h.strategyError(w, err)
		return
	}
	status := 201
	if result.Duplicate {
		status = 200
	}
	writeJSON(w, status, map[string]any{"lifecycle_event": result, "live_execution_available": false})
}

func (h *authHandler) evaluateStrategy(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var input struct {
		EventID string `json:"event_id"`
	}
	if !decode(w, r, &input) {
		return
	}
	outcome, err := h.evaluations.Evaluate(r.Context(), principal(r), r.PathValue("id"), input.EventID)
	if err != nil {
		h.strategyError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"evaluation": outcome})
}
