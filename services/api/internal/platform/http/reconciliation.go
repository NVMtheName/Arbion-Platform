package http

import (
	"errors"
	stdhttp "net/http"
	"strconv"
	"strings"

	"github.com/arbion/platform/services/api/internal/financialconnection"
)

func (handler *authHandler) portfolioReconciliationHistory(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if handler.financial == nil {
		writeError(writer, stdhttp.StatusServiceUnavailable, "RECONCILIATION_UNAVAILABLE", "Portfolio reconciliation is not available.")
		return
	}
	values := request.URL.Query()
	for key := range values {
		if key != "limit" && key != "cursor" {
			writeError(writer, stdhttp.StatusBadRequest, "INVALID_RECONCILIATION_HISTORY", "The reconciliation history request is invalid.")
			return
		}
	}
	if len(values["limit"]) > 1 || len(values["cursor"]) > 1 {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_RECONCILIATION_HISTORY", "The reconciliation history request is invalid.")
		return
	}
	query := financialconnection.ReconciliationHistoryQuery{Cursor: strings.TrimSpace(values.Get("cursor"))}
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 50 {
			writeError(writer, stdhttp.StatusBadRequest, "INVALID_RECONCILIATION_HISTORY", "The reconciliation history request is invalid.")
			return
		}
		query.Limit = limit
	}
	page, err := handler.financial.ReconciliationHistory(request.Context(), principal(request), request.PathValue("id"), query)
	if err != nil {
		handler.financialError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"history": page, "live_execution_available": false, "provider_read_performed": false,
	})
}

func (handler *authHandler) latestPortfolioReconciliation(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if handler.financial == nil {
		writeError(writer, stdhttp.StatusServiceUnavailable, "RECONCILIATION_UNAVAILABLE", "Portfolio reconciliation is not available.")
		return
	}
	report, err := handler.financial.LatestReconciliation(request.Context(), principal(request), request.PathValue("id"))
	if errors.Is(err, financialconnection.ErrReconciliationNotFound) {
		writeError(writer, stdhttp.StatusNotFound, "RECONCILIATION_NOT_FOUND", "No portfolio reconciliation has been recorded for this account.")
		return
	}
	if err != nil {
		handler.financialError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"reconciliation": report, "live_execution_available": false, "autonomy_enforcement_active": report.AutonomyEnforcementActive,
	})
}

func (handler *authHandler) runPortfolioReconciliation(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if !handler.csrf(request) {
		writeError(writer, stdhttp.StatusForbidden, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	if handler.financial == nil {
		writeError(writer, stdhttp.StatusServiceUnavailable, "RECONCILIATION_UNAVAILABLE", "Portfolio reconciliation is not available.")
		return
	}
	var command financialconnection.ReconciliationCommand
	if !decodeOptional(writer, request, &command) {
		return
	}
	report, err := handler.financial.RunReconciliationCommand(request.Context(), principal(request), request.PathValue("id"), command)
	if err != nil {
		handler.financialError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusCreated, map[string]any{
		"reconciliation": report, "live_execution_available": false, "autonomy_enforcement_active": report.AutonomyEnforcementActive,
	})
}
