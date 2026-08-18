package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/strategy"
)

const defaultJournalPageSize = 25

var journalUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

type journalCursorPayload struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func registerStrategyRoutes(m *stdhttp.ServeMux, h *authHandler) {
	if h.strategies == nil {
		return
	}
	m.Handle("POST /api/automations/{id}/strategy/initialize", h.require(stdhttp.HandlerFunc(h.initializeStrategy)))
	m.Handle("GET /api/strategy-instances", h.require(stdhttp.HandlerFunc(h.listStrategyInstances)))
	m.Handle("GET /api/strategy-instances/{id}", h.require(stdhttp.HandlerFunc(h.getStrategyInstance)))
	m.Handle("GET /api/strategy-instances/{id}/history", h.require(stdhttp.HandlerFunc(h.strategyHistory)))
	m.Handle("GET /api/strategy-instances/{id}/decisions", h.require(stdhttp.HandlerFunc(h.strategyDecisions)))
	m.Handle("GET /api/strategy-instances/{id}/executions", h.require(stdhttp.HandlerFunc(h.strategyExecutions)))
	m.Handle("GET /api/strategy-instances/{id}/paper-portfolio", h.require(stdhttp.HandlerFunc(h.strategyPaperPortfolio)))
	m.Handle("GET /api/strategy-instances/{id}/schedule", h.require(stdhttp.HandlerFunc(h.strategySchedule)))
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
	case errors.Is(e, strategy.ErrInvalid):
		writeError(w, 422, "INVALID_STRATEGY", "The deterministic strategy request is invalid or unsupported.")
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
func (h *authHandler) strategyHistory(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.strategies.History(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.strategyError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"transitions": v})
}
func (h *authHandler) strategyDecisions(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.strategies.Decisions(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.strategyError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"decisions": v})
}
func (h *authHandler) strategyExecutions(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.strategies.Executions(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.strategyError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"executions": v})
}

func (h *authHandler) strategyPaperPortfolio(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.strategies.PaperPortfolio(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.strategyError(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"paper_portfolio": v, "live_execution_available": false})
}

func (h *authHandler) strategySchedule(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.strategies.Schedule(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.strategyError(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"schedule": v, "scheduler_enabled": h.schedulerEnabled, "live_execution_available": false})
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
