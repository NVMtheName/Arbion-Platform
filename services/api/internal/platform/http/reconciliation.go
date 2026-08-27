package http

import (
	"errors"
	stdhttp "net/http"

	"github.com/arbion/platform/services/api/internal/financialconnection"
)

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
