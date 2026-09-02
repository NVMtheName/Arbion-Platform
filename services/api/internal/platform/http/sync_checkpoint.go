package http

import (
	stdhttp "net/http"
	"strconv"
	"strings"

	"github.com/arbion/platform/services/api/internal/financialconnection"
)

func (handler *authHandler) accountSyncCheckpointHistory(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if handler.financial == nil {
		writeError(writer, stdhttp.StatusServiceUnavailable, "SYNC_CHECKPOINT_HISTORY_UNAVAILABLE", "Saved financial account sync history is not available.")
		return
	}
	values := request.URL.Query()
	for key := range values {
		if key != "limit" && key != "cursor" {
			writeError(writer, stdhttp.StatusBadRequest, "INVALID_SYNC_CHECKPOINT_HISTORY", "The saved financial account sync history request is invalid.")
			return
		}
	}
	if len(values["limit"]) > 1 || len(values["cursor"]) > 1 {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_SYNC_CHECKPOINT_HISTORY", "The saved financial account sync history request is invalid.")
		return
	}
	query := financialconnection.SyncCheckpointHistoryQuery{Cursor: strings.TrimSpace(values.Get("cursor"))}
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 50 {
			writeError(writer, stdhttp.StatusBadRequest, "INVALID_SYNC_CHECKPOINT_HISTORY", "The saved financial account sync history request is invalid.")
			return
		}
		query.Limit = limit
	}
	page, err := handler.financial.SyncCheckpointHistory(request.Context(), principal(request), request.PathValue("id"), query)
	if err != nil {
		handler.financialError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"history":                  page,
		"history_semantics":        "IMMUTABLE_FINANCIAL_ACCOUNT_SYNC_CHECKPOINTS",
		"provider_read_performed":  false,
		"broker_action_available":  false,
		"live_execution_available": false,
	})
}
