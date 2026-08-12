package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type healthResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

// NewHandler builds the API's root HTTP handler.
type ReadinessChecker interface{ Ping(context.Context) error }

func NewHandler(database ReadinessChecker, timeout time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /readyz", readiness(database, timeout))
	return mux
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(healthResponse{Service: "api", Status: "ok"}); err != nil {
		http.Error(w, "unable to encode response", http.StatusInternalServerError)
	}
}

func readiness(database ReadinessChecker, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), timeout)
		defer cancel()
		w.Header().Set("Content-Type", "application/json")
		if database == nil || database.Ping(ctx) != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(healthResponse{Service: "api", Status: "not_ready"})
			return
		}
		_ = json.NewEncoder(w).Encode(healthResponse{Service: "api", Status: "ready"})
	}
}
