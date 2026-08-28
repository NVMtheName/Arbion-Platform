package http

import (
	stdhttp "net/http"

	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/ownerattention"
	"github.com/arbion/platform/services/api/internal/platform/config"
)

// WithOwnerAttention adds the read-only owner Attention Center without
// widening the established application-handler constructor. The route reuses
// the same authenticated session boundary as the rest of the owner surface.
func WithOwnerAttention(base stdhttp.Handler, cfg config.Config, authService *auth.Service, attention *ownerattention.Service) stdhttp.Handler {
	h := &authHandler{service: authService, cfg: cfg.Auth}
	mux := stdhttp.NewServeMux()
	mux.Handle("GET /api/owner/attention", h.require(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		user, _ := request.Context().Value(identityKey{}).(auth.SafeUser)
		if attention == nil {
			writer.Header().Set("Cache-Control", "no-store")
			writeError(writer, stdhttp.StatusServiceUnavailable, "OWNER_ATTENTION_UNAVAILABLE", "Attention status is temporarily unavailable.")
			return
		}
		overview, err := attention.Overview(request.Context(), user.ID)
		if err != nil {
			writer.Header().Set("Cache-Control", "no-store")
			writeError(writer, stdhttp.StatusServiceUnavailable, "OWNER_ATTENTION_UNAVAILABLE", "Attention status is temporarily unavailable.")
			return
		}
		writer.Header().Set("Cache-Control", "no-store")
		writeJSON(writer, stdhttp.StatusOK, map[string]any{
			"attention":                overview,
			"credential_data_exposed":  false,
			"provider_payload_exposed": false,
			"portfolio_data_exposed":   false,
		})
	})))
	mux.Handle("/", base)
	return securityHeaders(mux)
}
