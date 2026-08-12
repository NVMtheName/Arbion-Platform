package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type checker struct{ err error }

func (c checker) Ping(context.Context) error { return c.err }

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	NewHandler(checker{}, time.Second).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if got := strings.TrimSpace(recorder.Body.String()); got != `{"service":"api","status":"ok"}` {
		t.Fatalf("unexpected response body: %s", got)
	}
}

func TestReadiness(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
		body   string
	}{{"ready", nil, http.StatusOK, `{"service":"api","status":"ready"}`}, {"dependency unavailable", errors.New("unavailable"), http.StatusServiceUnavailable, `{"service":"api","status":"not_ready"}`}} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			recorder := httptest.NewRecorder()
			NewHandler(checker{test.err}, time.Second).ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("expected %d, got %d", test.status, recorder.Code)
			}
			if got := strings.TrimSpace(recorder.Body.String()); got != test.body {
				t.Fatalf("unexpected body: %s", got)
			}
		})
	}
}
