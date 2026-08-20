package http

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/arbion/platform/services/api/internal/marketintelligence"
)

func TestMarketSourcesAreNoStoreReadOnlyMetadata(t *testing.T) {
	handler := &authHandler{marketSources: marketintelligence.DefaultSources()}
	recorder := httptest.NewRecorder()
	handler.listMarketSources(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/markets/sources", nil))

	var response struct {
		Sources                []marketintelligence.Source `json:"sources"`
		LiveExecutionAvailable bool                        `json:"live_execution_available"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected response metadata: status=%d cache=%q", recorder.Code, recorder.Header().Get("Cache-Control"))
	}
	if len(response.Sources) != 6 || response.LiveExecutionAvailable {
		t.Fatalf("unexpected market source response: %+v", response)
	}
	for _, source := range response.Sources {
		if source.Enabled || source.Healthy {
			t.Fatalf("unwired source exposed as available: %+v", source)
		}
	}
}
