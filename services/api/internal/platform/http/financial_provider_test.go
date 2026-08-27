package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/financialconnection"
	"github.com/arbion/platform/services/api/internal/orderintent"
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

func TestCoinbaseEnrollmentErrorsGiveSafeActionableGuidance(t *testing.T) {
	for _, test := range []struct {
		code        financial.ProviderErrorCode
		status      int
		messagePart string
	}{
		{code: financial.InvalidCredentialFormat, status: http.StatusBadRequest, messagePart: "ECDSA (ES256)"},
		{code: financial.AuthorizationFailed, status: http.StatusBadRequest, messagePart: "same active key"},
		{code: financial.PermissionDenied, status: http.StatusForbidden, messagePart: "Transfer disabled"},
	} {
		t.Run(string(test.code), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			(&authHandler{}).financialError(recorder, &financial.ProviderError{Code: test.code})
			var response struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != test.status || response.Error.Code != string(test.code) || !strings.Contains(response.Error.Message, test.messagePart) {
				t.Fatalf("unexpected safe error response: status=%d response=%+v", recorder.Code, response)
			}
			if strings.Contains(response.Error.Message, "organizations/test") || strings.Contains(response.Error.Message, "PRIVATE KEY") {
				t.Fatalf("credential material appeared in public error: %q", response.Error.Message)
			}
		})
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

func TestPortfolioReconciliationRoutesRequireAuthenticationAndApprovedOrigin(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	sessions := auth.NewRedisStore(redisClient)
	service := auth.NewService(&authUsers{}, sessions, sessions, auditSink{}, time.Hour)
	cfg := config.Config{Database: config.Database{ReadinessTimeout: time.Second}, Auth: config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"http://localhost:3000"}}}
	handler := NewFullApplicationHandler(checker{}, cfg, service, nil, nil, &financialconnection.Service{})
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/accounts/account-1/reconciliations/latest", nil),
		httptest.NewRequest(http.MethodGet, "/api/accounts/account-1/reconciliations", nil),
		httptest.NewRequest(http.MethodPost, "/api/accounts/account-1/reconciliations", nil),
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous reconciliation route returned %d: %s", recorder.Code, recorder.Body.String())
		}
	}

	direct := &authHandler{cfg: cfg.Auth, financial: &financialconnection.Service{}}
	request := httptest.NewRequest(http.MethodPost, "/api/accounts/account-1/reconciliations", nil)
	recorder := httptest.NewRecorder()
	direct.runPortfolioReconciliation(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("reconciliation without an approved origin returned %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestPortfolioReconciliationReviewErrorsAreSafeAndActionable(t *testing.T) {
	for _, testCase := range []struct {
		err         error
		status      int
		code        string
		messagePart string
	}{
		{financialconnection.ErrReconciliationReviewRequired, http.StatusConflict, "RECONCILIATION_REVIEW_REQUIRED", "explicitly confirm"},
		{financialconnection.ErrReconciliationChanged, http.StatusConflict, "RECONCILIATION_CHANGED", "Refresh the account"},
		{financialconnection.ErrInvalidReconciliationCommand, http.StatusBadRequest, "INVALID_RECONCILIATION_COMMAND", "request is invalid"},
		{financialconnection.ErrInvalidReconciliationHistory, http.StatusBadRequest, "INVALID_RECONCILIATION_HISTORY", "history request is invalid"},
	} {
		recorder := httptest.NewRecorder()
		(&authHandler{}).financialError(recorder, testCase.err)
		var response struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != testCase.status || response.Error.Code != testCase.code || !strings.Contains(response.Error.Message, testCase.messagePart) {
			t.Fatalf("unexpected review error: status=%d response=%+v", recorder.Code, response)
		}
	}
}

func TestReconciliationHistoryQueryRejectsAmbiguousOrInvalidLimits(t *testing.T) {
	handler := &authHandler{financial: &financialconnection.Service{}}
	for _, target := range []string{
		"/api/accounts/account-1/reconciliations?limit=one",
		"/api/accounts/account-1/reconciliations?limit=0",
		"/api/accounts/account-1/reconciliations?limit=51",
		"/api/accounts/account-1/reconciliations?limit=10&limit=20",
		"/api/accounts/account-1/reconciliations?cursor=one&cursor=two",
		"/api/accounts/account-1/reconciliations?include=positions",
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		recorder := httptest.NewRecorder()
		handler.portfolioReconciliationHistory(recorder, request)
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "INVALID_RECONCILIATION_HISTORY") {
			t.Fatalf("invalid history query %q returned %d: %s", target, recorder.Code, recorder.Body.String())
		}
	}
}

func TestOptionalReconciliationCommandBodyIsStrictAndBackwardCompatible(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		body   string
		ok     bool
		status int
	}{
		{name: "legacy empty body", body: "", ok: true, status: http.StatusOK},
		{name: "valid command", body: `{"expected_reconciliation_id":"snapshot-1","acknowledge_current_drift":true}`, ok: true, status: http.StatusOK},
		{name: "unknown field", body: `{"approve":true}`, status: http.StatusBadRequest},
		{name: "multiple objects", body: `{}` + "\n" + `{}`, status: http.StatusBadRequest},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/accounts/account-1/reconciliations", strings.NewReader(testCase.body))
			var command financialconnection.ReconciliationCommand
			ok := decodeOptional(recorder, request, &command)
			if ok != testCase.ok || recorder.Code != testCase.status {
				t.Fatalf("decode result=%v status=%d command=%#v", ok, recorder.Code, command)
			}
		})
	}
}

func TestOrderIntentRoutesRequireAuthenticationAndApprovedOrigin(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	sessions := auth.NewRedisStore(redisClient)
	authService := auth.NewService(&authUsers{}, sessions, sessions, auditSink{}, time.Hour)
	cfg := config.Config{Database: config.Database{ReadinessTimeout: time.Second}, Auth: config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"http://localhost:3000"}}}
	intents := orderintent.NewService(nil, nil, nil, nil)
	handler := NewFullApplicationHandlerWithEvaluationMarketsAndOrderIntents(checker{}, cfg, authService, nil, nil, nil, nil, nil, nil, nil, nil, intents)
	request := httptest.NewRequest(http.MethodPost, "/api/accounts/account-1/order-intents", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous intent route returned %d: %s", recorder.Code, recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/accounts/account-1/order-intents/ai-proposals", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous AI proposal route returned %d: %s", recorder.Code, recorder.Body.String())
	}

	direct := &authHandler{cfg: cfg.Auth, orderIntents: intents}
	request = httptest.NewRequest(http.MethodPost, "/api/order-intents/intent-1/review", nil)
	recorder = httptest.NewRecorder()
	direct.reviewOrderIntent(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("intent review without an approved origin returned %d: %s", recorder.Code, recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/accounts/account-1/order-intents/ai-proposals", nil)
	recorder = httptest.NewRecorder()
	direct.createAIOrderProposal(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("AI proposal without an approved origin returned %d: %s", recorder.Code, recorder.Body.String())
	}
}
