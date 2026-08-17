package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/platform/config"
	"github.com/redis/go-redis/v9"
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

type authUsers struct{ user auth.User }

func (f *authUsers) Create(_ context.Context, email, n, hash, name string) (auth.User, error) {
	f.user = auth.User{ID: "user-1", Email: email, NormalizedEmail: n, PasswordHash: hash, DisplayName: name, Status: "active"}
	return f.user, nil
}
func (f *authUsers) ByNormalizedEmail(context.Context, string) (auth.User, error) { return f.user, nil }
func (f *authUsers) ByID(context.Context, string) (auth.User, error)              { return f.user, nil }
func (f *authUsers) RecordLogin(context.Context, string, time.Time) error         { return nil }

type auditSink struct{}

func (auditSink) Record(context.Context, *string, string, map[string]any) error { return nil }
func TestAuthenticationRoutesCSRFAndProtection(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	sessions := auth.NewRedisStore(redisClient)
	service := auth.NewService(&authUsers{}, sessions, sessions, auditSink{}, time.Hour)
	cfg := config.Config{Database: config.Database{ReadinessTimeout: time.Second}, Auth: config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"http://localhost:3000"}}}
	handler := NewApplicationHandler(checker{}, cfg, service)
	rejected := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"email":"person@example.com","password":"correct horse battery staple"}`))
	rejected.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, rejected)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected CSRF rejection, got %d", rr.Code)
	}
	register := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"email":"Person@Example.com","password":"correct horse battery staple","display_name":"Person"}`))
	register.Header.Set("Origin", "http://localhost:3000")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, register)
	if rr.Code != http.StatusCreated {
		t.Fatalf("registration failed: %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "password") || strings.Contains(rr.Body.String(), "normalized") || strings.Contains(rr.Body.String(), "token") {
		t.Fatal("unsafe user serialization")
	}
	cookie := rr.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.Path != "/" {
		t.Fatal("insecure cookie")
	}
	me := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	me.AddCookie(cookie)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, me)
	if rr.Code != http.StatusOK {
		t.Fatalf("protected access failed: %d", rr.Code)
	}
	anonymous := httptest.NewRequest(http.MethodGet, "/api/auth/protected-test", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, anonymous)
	if rr.Code != http.StatusUnauthorized {
		t.Fatal("unauthenticated request accepted")
	}
	logout := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logout.Header.Set("Origin", "http://localhost:3000")
	logout.AddCookie(cookie)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, logout)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("logout failed: %d", rr.Code)
	}
	if _, err := sessions.Get(context.Background(), cookie.Value); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("session retained after logout")
	}
}

func TestRegistrationAllowlistUsesGenericRejection(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	sessions := auth.NewRedisStore(redisClient)
	service := auth.NewService(&authUsers{}, sessions, sessions, auditSink{}, time.Hour, auth.RegistrationPolicy{Restricted: true, AllowedEmails: []string{"founder@example.com"}})
	cfg := config.Config{Database: config.Database{ReadinessTimeout: time.Second}, Auth: config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"http://localhost:3000"}}}
	handler := NewApplicationHandler(checker{}, cfg, service)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"email":"outsider@example.com","password":"correct horse battery staple"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:3000")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"registration_unavailable"`) || strings.Contains(recorder.Body.String(), "outsider@example.com") {
		t.Fatalf("unsafe allowlist rejection: %d %s", recorder.Code, recorder.Body.String())
	}
	if len(recorder.Result().Cookies()) != 0 {
		t.Fatal("rejected registration created a session cookie")
	}
}

func TestNeuralInsightRequiresAuthenticationAndTrustedOrigin(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	sessions := auth.NewRedisStore(redisClient)
	service := auth.NewService(&authUsers{}, sessions, sessions, auditSink{}, time.Hour)
	cfg := config.Config{Database: config.Database{ReadinessTimeout: time.Second}, Auth: config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"http://localhost:3000"}}}
	handler := NewFullApplicationHandler(checker{}, cfg, service, nil, nil)

	anonymous := httptest.NewRequest(http.MethodPost, "/api/neural/insight", strings.NewReader(`{"prompt":"question"}`))
	anonymous.Header.Set("Origin", "http://localhost:3000")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, anonymous)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous insight accepted: %d", recorder.Code)
	}

	register := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"email":"Person@Example.com","password":"correct horse battery staple","display_name":"Person"}`))
	register.Header.Set("Origin", "http://localhost:3000")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, register)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("registration failed: %d %s", recorder.Code, recorder.Body.String())
	}

	missingOrigin := httptest.NewRequest(http.MethodPost, "/api/neural/insight", strings.NewReader(`{"prompt":"question"}`))
	missingOrigin.AddCookie(recorder.Result().Cookies()[0])
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, missingOrigin)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("insight without trusted origin accepted: %d", recorder.Code)
	}
}
