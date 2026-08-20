package http

import (
	stdhttp "net/http"
)

func (h *authHandler) listMarketSources(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"sources":                  h.marketSources,
		"live_execution_available": false,
	})
}
