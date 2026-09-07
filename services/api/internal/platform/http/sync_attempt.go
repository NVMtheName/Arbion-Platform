package http

import (
	stdhttp "net/http"
	"strconv"
	"strings"

	"github.com/arbion/platform/services/api/internal/financialconnection"
)

func (handler *authHandler) accountSyncAttemptHistory(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if handler.financial == nil {
		writeError(writer, stdhttp.StatusServiceUnavailable, "SYNC_ATTEMPT_HISTORY_UNAVAILABLE", "Saved financial connection sync attempt history is not available.")
		return
	}
	values := request.URL.Query()
	for key := range values {
		if key != "limit" {
			writeError(writer, stdhttp.StatusBadRequest, "INVALID_SYNC_ATTEMPT_HISTORY", "The saved financial connection sync attempt history request is invalid.")
			return
		}
	}
	if len(values["limit"]) > 1 {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_SYNC_ATTEMPT_HISTORY", "The saved financial connection sync attempt history request is invalid.")
		return
	}
	query := financialconnection.SyncAttemptHistoryQuery{}
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 50 {
			writeError(writer, stdhttp.StatusBadRequest, "INVALID_SYNC_ATTEMPT_HISTORY", "The saved financial connection sync attempt history request is invalid.")
			return
		}
		query.Limit = limit
	}
	history, err := handler.financial.SyncAttemptHistory(request.Context(), principal(request), request.PathValue("id"), query)
	if err != nil {
		handler.financialError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"history":                  history,
		"history_semantics":        "IMMUTABLE_FINANCIAL_CONNECTION_SYNC_ATTEMPTS",
		"provider_read_performed":  false,
		"broker_action_available":  false,
		"live_execution_available": false,
	})
}
