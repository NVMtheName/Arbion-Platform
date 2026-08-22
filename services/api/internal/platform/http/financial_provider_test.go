package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/financialconnection"
	"github.com/arbion/platform/services/api/internal/platform/config"
	"github.com/redis/go-redis/v9"
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
					AuthType   string `json:"auth_type"`
				} `json:"providers"`
			}
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusOK || len(response.Providers) == 0 || response.Providers[0].ID != "schwab" || response.Providers[0].Configured != test.configured {
				t.Fatalf("unexpected provider response: status=%d providers=%+v", recorder.Code, response.Providers)
			}
			if len(response.Providers) < 2 || response.Providers[1].ID != "coinbase" || !response.Providers[1].Configured || response.Providers[1].AuthType != "jwt_key_pair" {
				t.Fatalf("Coinbase connection was not advertised safely: %+v", response.Providers)
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

func TestCoinbaseOrderPreviewRouteRequiresAuthenticationAndApprovedOrigin(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	sessions := auth.NewRedisStore(redisClient)
	service := auth.NewService(&authUsers{}, sessions, sessions, auditSink{}, time.Hour)
	cfg := config.Config{Database: config.Database{ReadinessTimeout: time.Second}, Auth: config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"http://localhost:3000"}}}
	handler := NewFullApplicationHandler(checker{}, cfg, service, nil, nil, &financialconnection.Service{})
	request := httptest.NewRequest(http.MethodPost, "/api/accounts/account-1/orders/preview", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous preview route returned %d: %s", recorder.Code, recorder.Body.String())
	}

	direct := &authHandler{cfg: cfg.Auth}
	request = httptest.NewRequest(http.MethodPost, "/api/accounts/account-1/orders/preview", nil)
	request = request.WithContext(context.WithValue(request.Context(), identityKey{}, struct{}{}))
	recorder = httptest.NewRecorder()
	direct.previewSpotOrder(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("preview without an approved origin returned %d: %s", recorder.Code, recorder.Body.String())
	}
}
