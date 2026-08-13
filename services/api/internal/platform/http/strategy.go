package http

import (
	"errors"
	stdhttp "net/http"

	"github.com/arbion/platform/services/api/internal/strategy"
)

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
}
func (h *authHandler) strategyError(w stdhttp.ResponseWriter, e error) {
	switch {
	case errors.Is(e, strategy.ErrForbidden):
		writeError(w, 403, "PERMISSION_DENIED", "Automation entitlement is required.")
	case errors.Is(e, strategy.ErrInvalid):
		writeError(w, 422, "INVALID_STRATEGY", "The deterministic strategy request is invalid or unsupported.")
	case errors.Is(e, strategy.ErrConflict):
		writeError(w, 409, "STRATEGY_CONFLICT", "A strategy instance already exists for this mandate version.")
	default:
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
