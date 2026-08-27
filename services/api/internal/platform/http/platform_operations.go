package http

import (
	"context"
	"errors"
	stdhttp "net/http"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/platformops"
)

type platformOperationsController interface {
	Overview(context.Context, authorization.Principal) (platformops.Overview, error)
}

func registerPlatformOperationsRoutes(mux *stdhttp.ServeMux, handler *authHandler) {
	if handler.platformOperations == nil {
		return
	}
	mux.Handle("GET /api/admin/operations/readiness", handler.require(handler.requireSuperadmin(stdhttp.HandlerFunc(handler.platformOperationsReadiness))))
}

func (handler *authHandler) platformOperationsReadiness(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	overview, err := handler.platformOperations.Overview(request.Context(), principal(request))
	if err != nil {
		if errors.Is(err, platformops.ErrSuperadminRequired) {
			writeError(writer, stdhttp.StatusForbidden, "SUPERADMIN_REQUIRED", "Current active superadmin access is required for platform operations evidence.")
			return
		}
		writeError(writer, stdhttp.StatusInternalServerError, "INTERNAL_ERROR", "Platform operations evidence is temporarily unavailable.")
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"operations":               overview,
		"live_execution_available": false,
		"broker_action_requested":  false,
	})
}
