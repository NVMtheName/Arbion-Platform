package http

import (
	"context"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/ownerattention"
	"github.com/arbion/platform/services/api/internal/platform/config"
	"github.com/redis/go-redis/v9"
)

type ownerAttentionStoreFake struct {
	items       []ownerattention.Item
	requestedID string
	err         error
}

func (store *ownerAttentionStoreFake) Items(_ context.Context, userID string, _ int) ([]ownerattention.Item, error) {
	store.requestedID = userID
	return store.items, store.err
}

func TestOwnerAttentionRouteIsAuthenticatedOwnerScopedAndCredentialFree(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	sessions := auth.NewRedisStore(redisClient)
	authService := auth.NewService(&authUsers{}, sessions, sessions, auditSink{}, time.Hour)
	cfg := config.Config{Database: config.Database{ReadinessTimeout: time.Second}, Auth: config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"http://localhost:3000"}}}
	resourceID := "11111111-1111-4111-8111-111111111111"
	store := &ownerAttentionStoreFake{items: []ownerattention.Item{{
		ID:           "22222222-2222-4222-8222-222222222222",
		Code:         "PORTFOLIO_DRIFT_REVIEW_REQUIRED",
		Severity:     ownerattention.SeverityAttention,
		ResourceType: "ACCOUNT",
		ResourceID:   &resourceID,
		OccurredAt:   time.Date(2026, 8, 28, 2, 30, 0, 0, time.UTC),
		Count:        1,
	}}}
	handler := WithOwnerAttention(NewApplicationHandler(checker{}, cfg, authService), cfg, authService, ownerattention.NewService(store))

	register := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/register", strings.NewReader(`{"email":"person@example.com","password":"correct horse battery staple","display_name":"Person"}`))
	register.Header.Set("Origin", "http://localhost:3000")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, register)
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("registration failed: %d %s", recorder.Code, recorder.Body.String())
	}
	cookie := recorder.Result().Cookies()[0]

	request := httptest.NewRequest(stdhttp.MethodGet, "/api/owner/attention", nil)
	request.AddCookie(cookie)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("owner attention route failed: %d %s", recorder.Code, recorder.Body.String())
	}
	if store.requestedID != "user-1" {
		t.Fatalf("attention request used the wrong owner: %q", store.requestedID)
	}
	body := recorder.Body.String()
	for _, expected := range []string{
		`"code":"PORTFOLIO_DRIFT_REVIEW_REQUIRED"`,
		`"status":"ATTENTION"`,
		`"live_execution_available":false`,
		`"broker_action_requested":false`,
		`"credential_data_exposed":false`,
		`"provider_payload_exposed":false`,
		`"portfolio_data_exposed":false`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("attention response omitted %s: %s", expected, body)
		}
	}
	for _, prohibited := range []string{"api_key", "client_secret", "encrypted_credential_payload", "provider_name", "reason", "symbol", "quantity", "\"order\""} {
		if strings.Contains(strings.ToLower(body), prohibited) {
			t.Fatalf("attention response exposed %q: %s", prohibited, body)
		}
	}

	anonymous := httptest.NewRequest(stdhttp.MethodGet, "/api/owner/attention", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, anonymous)
	if recorder.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("anonymous attention request was accepted: %d", recorder.Code)
	}

	store.err = errors.New("sensitive database failure")
	unavailable := httptest.NewRequest(stdhttp.MethodGet, "/api/owner/attention", nil)
	unavailable.AddCookie(cookie)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, unavailable)
	if recorder.Code != stdhttp.StatusServiceUnavailable || recorder.Header().Get("Cache-Control") != "no-store" || strings.Contains(recorder.Body.String(), "sensitive") {
		t.Fatalf("attention failure was not safe: %d %s", recorder.Code, recorder.Body.String())
	}
}
