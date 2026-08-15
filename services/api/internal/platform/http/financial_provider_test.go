package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arbion/platform/services/api/internal/financial"
)

func TestFinancialProvidersExposeSchwabConfiguration(t *testing.T) {
	for _, test := range []struct {
		name       string
		configured bool
	}{
		{name: "not configured"},
		{name: "configured", configured: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := &authHandler{
				financialProviders: financial.DefaultRegistry(),
				schwabConfigured:   test.configured,
			}
			recorder := httptest.NewRecorder()
			h.listFinancialProviders(recorder, httptest.NewRequest(http.MethodGet, "/api/connections/financial/providers", nil))

			var response struct {
				Providers []struct {
					ID         string `json:"id"`
					Configured bool   `json:"configured"`
				} `json:"providers"`
			}
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusOK || len(response.Providers) == 0 || response.Providers[0].ID != "schwab" || response.Providers[0].Configured != test.configured {
				t.Fatalf("unexpected provider response: status=%d providers=%+v", recorder.Code, response.Providers)
			}
		})
	}
}

func TestStartSchwabRejectsUnconfiguredProvider(t *testing.T) {
	recorder := httptest.NewRecorder()
	(&authHandler{}).startSchwab(recorder, httptest.NewRequest(http.MethodPost, "/api/connections/financial/schwab/start", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
	if got := recorder.Body.String(); got == "" {
		t.Fatal("expected a safe provider-unavailable response")
	}
}
