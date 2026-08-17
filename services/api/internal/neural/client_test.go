package neural

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestAnalyzeSendsBoundedFieldsAndNormalizesInsight(t *testing.T) {
	const secret = "secret-value"
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/internal/neural/insight" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["credential"] != secret || body["profile"] != "fast" || body["model"] != "" || body["prompt"] != "Explain diversification" || body["safety_identifier"] != strings.Repeat("a", 64) {
			t.Fatalf("unexpected request fields: %#v", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"insight":{"summary":"Diversification reduces concentration risk.","key_points":["Spread exposure."],"risk_flags":["Loss remains possible."],"limitations":["No live data."],"requires_current_data":false,"metadata":{"provider":"openai","model":"gpt-5.6-luna","profile":"fast","input_usage":30,"output_usage":45,"request_id":"resp-safe","latency_ms":120}}}`)),
		}, nil
	})
	client := NewHTTPClient("http://ai.internal", "internal-token", &http.Client{Transport: transport})
	result, err := client.Analyze(context.Background(), "openai", "fast", []byte(secret), "Explain diversification", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary == "" || result.Metadata.Profile != "fast" || result.Metadata.RequestID != "resp-safe" || result.Metadata.InputUsage == nil || *result.Metadata.InputUsage != 30 {
		t.Fatalf("unexpected normalized insight: %#v", result)
	}
}

func TestVerifyKeepsRequestBodyUntilTransportReadsIt(t *testing.T) {
	const secret = "secret-value"
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), secret) {
			t.Fatalf("credential was cleared before the request body was sent: %q", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"valid":true,"provider":"openai"}`)),
		}, nil
	})
	client := NewHTTPClient("http://ai.internal", "internal-token", &http.Client{Transport: transport})
	if err := client.Verify(context.Background(), "openai", []byte(secret)); err != nil {
		t.Fatal(err)
	}
}
