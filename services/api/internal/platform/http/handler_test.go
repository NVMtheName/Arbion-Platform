package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/mailer"
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

func TestSecurityActivityCursorRoundTripsAndRejectsMalformedInput(t *testing.T) {
	want := &auth.SecurityActivityCursor{
		OccurredAt: time.Date(2026, 8, 28, 1, 30, 0, 123, time.UTC),
		ID:         "11111111-1111-4111-8111-111111111111",
	}
	encoded := encodeSecurityActivityCursor(want)
	got, err := decodeSecurityActivityCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || !got.OccurredAt.Equal(want.OccurredAt) {
		t.Fatalf("security activity cursor changed during round trip: %#v", got)
	}
	for _, input := range []string{"not-base64", "e30", encodeSecurityActivityCursor(&auth.SecurityActivityCursor{OccurredAt: time.Now(), ID: "not-a-uuid"})} {
		if _, err = decodeSecurityActivityCursor(input); err == nil {
			t.Fatalf("malformed security activity cursor was accepted: %q", input)
		}
	}
}

type authUsers struct{ user auth.User }

func (f *authUsers) Create(_ context.Context, email, n, hash, name, status string) (auth.User, error) {
	f.user = auth.User{ID: "user-1", Email: email, NormalizedEmail: n, PasswordHash: hash, DisplayName: name, Status: status}
	return f.user, nil
}
func (f *authUsers) ByNormalizedEmail(context.Context, string) (auth.User, error) { return f.user, nil }
func (f *authUsers) ByID(context.Context, string) (auth.User, error)              { return f.user, nil }
func (f *authUsers) RecordLogin(context.Context, string, time.Time) error         { return nil }
func (f *authUsers) UpdatePassword(_ context.Context, id, currentHash, nextHash string, _ time.Time) (bool, error) {
	if f.user.ID != id || f.user.PasswordHash != currentHash {
		return false, nil
	}
	f.user.PasswordHash = nextHash
	return true, nil
}

type auditSink struct{}

func (auditSink) Record(context.Context, *string, string, map[string]any) error { return nil }

type securityActivitySink struct {
	activities     []auth.SecurityActivity
	requestedUser  string
	requestedLimit int
}

func (*securityActivitySink) Record(context.Context, *string, string, map[string]any) error {
	return nil
}

func (sink *securityActivitySink) SecurityActivities(_ context.Context, userID string, limit int, _ *auth.SecurityActivityCursor) ([]auth.SecurityActivity, error) {
	sink.requestedUser = userID
	sink.requestedLimit = limit
	return sink.activities, nil
}

type authEmailTokens struct{}

func (authEmailTokens) ReplaceEmailToken(context.Context, string, auth.EmailTokenPurpose, []byte, time.Time, time.Time) error {
	return nil
}
func (authEmailTokens) ActiveEmailTokenUser(context.Context, auth.EmailTokenPurpose, []byte, time.Time) (string, error) {
	return "", auth.ErrInvalidEmailToken
}
func (authEmailTokens) ConsumeVerificationToken(context.Context, []byte, time.Time) (string, error) {
	return "", auth.ErrInvalidEmailToken
}
func (authEmailTokens) ConsumePasswordResetToken(context.Context, []byte, string, time.Time) (string, error) {
	return "", auth.ErrInvalidEmailToken
}

type emailSink struct{ count int }

func (e *emailSink) Send(context.Context, mailer.Message) error { e.count++; return nil }

type transportMFAStore struct {
	factor   auth.TOTPFactor
	consumed bool
}

func (s *transportMFAStore) MFAStatus(context.Context, string) (auth.MFAStatus, error) {
	return auth.MFAStatus{Enabled: s.factor.EnabledAt != nil, RecoveryCodesRemaining: 1}, nil
}
func (*transportMFAStore) SetPendingTOTP(context.Context, string, []byte, time.Time, time.Time) error {
	return nil
}
func (s *transportMFAStore) TOTPFactor(context.Context, string) (auth.TOTPFactor, error) {
	return s.factor, nil
}
func (*transportMFAStore) ActivateTOTP(context.Context, string, [][]byte, int64, time.Time) error {
	return nil
}
func (*transportMFAStore) AdvanceTOTPStep(context.Context, string, int64, time.Time) (bool, error) {
	return false, nil
}
func (s *transportMFAStore) ConsumeRecoveryCode(context.Context, string, []byte, time.Time) (bool, error) {
	if s.consumed {
		return false, nil
	}
	s.consumed = true
	return true, nil
}
func (*transportMFAStore) ReplaceRecoveryCodes(context.Context, string, [][]byte, time.Time) error {
	return nil
}
func (*transportMFAStore) DisableTOTP(context.Context, string) (bool, error) {
	return true, nil
}

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

func TestSecurityActivityRouteIsAuthenticatedBoundedAndMetadataFree(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	sessions := auth.NewRedisStore(redisClient)
	sink := &securityActivitySink{activities: []auth.SecurityActivity{{
		ID:         "11111111-1111-4111-8111-111111111111",
		Action:     "auth.login",
		OccurredAt: time.Date(2026, 8, 28, 1, 30, 0, 0, time.UTC),
	}}}
	service := auth.NewService(&authUsers{}, sessions, sessions, sink, time.Hour)
	cfg := config.Config{Database: config.Database{ReadinessTimeout: time.Second}, Auth: config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"http://localhost:3000"}}}
	handler := NewApplicationHandler(checker{}, cfg, service)

	register := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"email":"person@example.com","password":"correct horse battery staple","display_name":"Person"}`))
	register.Header.Set("Origin", "http://localhost:3000")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, register)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("registration failed: %d %s", recorder.Code, recorder.Body.String())
	}
	cookie := recorder.Result().Cookies()[0]

	request := httptest.NewRequest(http.MethodGet, "/api/auth/security-activity?limit=1", nil)
	request.AddCookie(cookie)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("security activity route failed: %d %s", recorder.Code, recorder.Body.String())
	}
	if sink.requestedUser != "user-1" || sink.requestedLimit != 2 {
		t.Fatalf("security activity owner or lookahead changed: %#v", sink)
	}
	body := recorder.Body.String()
	for _, expected := range []string{`"action":"auth.login"`, `"metadata_exposed":false`, `"credentials_exposed":false`, `"broker_data_exposed":false`, `"live_execution_available":false`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("security activity response omitted %s: %s", expected, body)
		}
	}
	for _, prohibited := range []string{"metadata\":{", "actor_id", "target_id", "email"} {
		if strings.Contains(body, prohibited) {
			t.Fatalf("security activity exposed %q: %s", prohibited, body)
		}
	}

	anonymous := httptest.NewRequest(http.MethodGet, "/api/auth/security-activity", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, anonymous)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatal("anonymous security activity request was accepted")
	}

	malformed := httptest.NewRequest(http.MethodGet, "/api/auth/security-activity?cursor=not-base64", nil)
	malformed.AddCookie(cookie)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, malformed)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed security activity cursor was accepted: %d", recorder.Code)
	}
}

func TestSessionInventoryRoutesPreserveCurrentSessionAndExposeNoTrackingMetadata(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	sessions := auth.NewRedisStore(redisClient)
	users := &authUsers{}
	service := auth.NewService(users, sessions, sessions, auditSink{}, time.Hour)
	cfg := config.Config{Database: config.Database{ReadinessTimeout: time.Second}, Auth: config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"http://localhost:3000"}}}
	handler := NewApplicationHandler(checker{}, cfg, service)

	register := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"email":"person@example.com","password":"correct horse battery staple","display_name":"Person"}`))
	register.Header.Set("Origin", "http://localhost:3000")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, register)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("registration failed: %d %s", recorder.Code, recorder.Body.String())
	}
	currentCookie := recorder.Result().Cookies()[0]

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"person@example.com","password":"correct horse battery staple"}`))
	login.Header.Set("Origin", "http://localhost:3000")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, login)
	if recorder.Code != http.StatusOK {
		t.Fatalf("second login failed: %d %s", recorder.Code, recorder.Body.String())
	}
	otherCookie := recorder.Result().Cookies()[0]

	request := httptest.NewRequest(http.MethodGet, "/api/auth/sessions", nil)
	request.AddCookie(currentCookie)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("session inventory route failed: %d %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{`"active_count":2`, `"other_count":1`, `"network_metadata_exposed":false`, `"device_metadata_exposed":false`, `"credentials_exposed":false`, `"provider_data_exposed":false`, `"broker_action_requested":false`, `"live_execution_available":false`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("session inventory response omitted %s: %s", expected, body)
		}
	}
	for _, prohibited := range []string{currentCookie.Value, otherCookie.Value, "user_id", "last_activity_at", "user_agent", "ip_address", "device_id"} {
		if strings.Contains(body, prohibited) {
			t.Fatalf("session inventory exposed %q: %s", prohibited, body)
		}
	}

	missingOrigin := httptest.NewRequest(http.MethodPost, "/api/auth/logout-others", nil)
	missingOrigin.AddCookie(currentCookie)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, missingOrigin)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("other-session revocation without a trusted origin returned %d", recorder.Code)
	}
	if _, err := sessions.Get(context.Background(), otherCookie.Value); err != nil {
		t.Fatal("rejected other-session revocation removed a session")
	}

	revoke := httptest.NewRequest(http.MethodPost, "/api/auth/logout-others", nil)
	revoke.Header.Set("Origin", "http://localhost:3000")
	revoke.AddCookie(currentCookie)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, revoke)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" || !strings.Contains(recorder.Body.String(), `"revoked_session_count":1`) || !strings.Contains(recorder.Body.String(), `"current_session_preserved":true`) {
		t.Fatalf("other-session revocation failed: %d %s", recorder.Code, recorder.Body.String())
	}
	if _, err := sessions.Get(context.Background(), currentCookie.Value); err != nil {
		t.Fatal("other-session revocation removed the current session")
	}
	if _, err := sessions.Get(context.Background(), otherCookie.Value); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("other-session revocation retained another session")
	}

	anonymous := httptest.NewRequest(http.MethodGet, "/api/auth/sessions", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, anonymous)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatal("anonymous session inventory request was accepted")
	}
}

func TestMFALoginCreatesNoSessionUntilSecondFactorSucceeds(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	sessions := auth.NewRedisStore(redisClient)
	users := &authUsers{}
	service := auth.NewService(users, sessions, sessions, auditSink{}, time.Hour)
	_, _, err := service.Register(context.Background(), "person@example.com", "correct horse battery staple", "Person", "ip-register")
	if err != nil {
		t.Fatal(err)
	}
	protector, err := auth.NewMFASecretProtector(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := protector.Seal(users.user.ID, []byte("twenty-byte-secret!!"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	mfaStore := &transportMFAStore{factor: auth.TOTPFactor{SecretCiphertext: ciphertext, EnabledAt: &now}}
	service.ConfigureMFA(mfaStore, sessions, protector)
	cfg := config.Config{Database: config.Database{ReadinessTimeout: time.Second}, Auth: config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"http://localhost:3000"}}}
	handler := NewApplicationHandler(checker{}, cfg, service)

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"person@example.com","password":"correct horse battery staple"}`))
	login.Header.Set("Origin", "http://localhost:3000")
	loginRecorder := httptest.NewRecorder()
	handler.ServeHTTP(loginRecorder, login)
	if loginRecorder.Code != http.StatusAccepted || len(loginRecorder.Result().Cookies()) != 0 {
		t.Fatalf("password phase returned %d with %d cookies: %s", loginRecorder.Code, len(loginRecorder.Result().Cookies()), loginRecorder.Body.String())
	}
	if loginRecorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("MFA challenge response was cacheable")
	}
	var challenge struct {
		MFARequired    bool   `json:"mfa_required"`
		ChallengeToken string `json:"challenge_token"`
	}
	if err = json.Unmarshal(loginRecorder.Body.Bytes(), &challenge); err != nil || !challenge.MFARequired || challenge.ChallengeToken == "" {
		t.Fatalf("invalid MFA challenge response: %#v %v", challenge, err)
	}

	complete := httptest.NewRequest(http.MethodPost, "/api/auth/mfa/login", strings.NewReader(`{"challenge_token":"`+challenge.ChallengeToken+`","code":"ABCD-EFGH-JKLM-NPQR"}`))
	complete.Header.Set("Origin", "http://localhost:3000")
	completeRecorder := httptest.NewRecorder()
	handler.ServeHTTP(completeRecorder, complete)
	if completeRecorder.Code != http.StatusOK || len(completeRecorder.Result().Cookies()) != 1 || !completeRecorder.Result().Cookies()[0].HttpOnly {
		t.Fatalf("MFA completion returned %d with %d cookies: %s", completeRecorder.Code, len(completeRecorder.Result().Cookies()), completeRecorder.Body.String())
	}

	replay := httptest.NewRequest(http.MethodPost, "/api/auth/mfa/login", strings.NewReader(`{"challenge_token":"`+challenge.ChallengeToken+`","code":"ABCD-EFGH-JKLM-NPQR"}`))
	replay.Header.Set("Origin", "http://localhost:3000")
	replayRecorder := httptest.NewRecorder()
	handler.ServeHTTP(replayRecorder, replay)
	if replayRecorder.Code != http.StatusUnauthorized || !strings.Contains(replayRecorder.Body.String(), `"code":"invalid_mfa_challenge"`) {
		t.Fatalf("MFA challenge replay returned %d %s", replayRecorder.Code, replayRecorder.Body.String())
	}
}

func TestRequiredVerificationRegistrationReturnsPendingWithoutCookie(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	sessions := auth.NewRedisStore(redisClient)
	users := &authUsers{}
	sender := &emailSink{}
	service := auth.NewService(users, sessions, sessions, auditSink{}, time.Hour)
	service.ConfigureEmail(authEmailTokens{}, sender, auth.EmailPolicy{VerificationRequired: true, PublicBaseURL: "https://www.arbion.ai", VerificationTTL: 24 * time.Hour, PasswordResetTTL: 30 * time.Minute})
	cfg := config.Config{Database: config.Database{ReadinessTimeout: time.Second}, Auth: config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"http://localhost:3000"}}}
	handler := NewApplicationHandler(checker{}, cfg, service)

	request := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"email":"person@example.com","password":"correct horse battery staple"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted || !strings.Contains(recorder.Body.String(), `"verification_required":true`) || sender.count != 1 {
		t.Fatalf("pending registration returned %d %s with %d messages", recorder.Code, recorder.Body.String(), sender.count)
	}
	if len(recorder.Result().Cookies()) != 0 {
		t.Fatal("pending registration created an authenticated cookie")
	}

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"person@example.com","password":"correct horse battery staple"}`))
	login.Header.Set("Origin", "http://localhost:3000")
	loginRecorder := httptest.NewRecorder()
	handler.ServeHTTP(loginRecorder, login)
	if loginRecorder.Code != http.StatusForbidden || !strings.Contains(loginRecorder.Body.String(), `"code":"email_verification_required"`) {
		t.Fatalf("pending login returned %d %s", loginRecorder.Code, loginRecorder.Body.String())
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

func TestAccountSecurityRoutesRequireCurrentPasswordAndRevokeSessions(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	sessions := auth.NewRedisStore(redisClient)
	users := &authUsers{}
	service := auth.NewService(users, sessions, sessions, auditSink{}, time.Hour)
	cfg := config.Config{Database: config.Database{ReadinessTimeout: time.Second}, Auth: config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"http://localhost:3000"}}}
	handler := NewApplicationHandler(checker{}, cfg, service)

	register := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"email":"person@example.com","password":"correct horse battery staple"}`))
	register.Header.Set("Origin", "http://localhost:3000")
	registerRecorder := httptest.NewRecorder()
	handler.ServeHTTP(registerRecorder, register)
	if registerRecorder.Code != http.StatusCreated {
		t.Fatalf("registration failed: %d %s", registerRecorder.Code, registerRecorder.Body.String())
	}
	firstCookie := registerRecorder.Result().Cookies()[0]

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"person@example.com","password":"correct horse battery staple"}`))
	login.Header.Set("Origin", "http://localhost:3000")
	loginRecorder := httptest.NewRecorder()
	handler.ServeHTTP(loginRecorder, login)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("second login failed: %d %s", loginRecorder.Code, loginRecorder.Body.String())
	}
	secondCookie := loginRecorder.Result().Cookies()[0]

	missingOrigin := httptest.NewRequest(http.MethodPut, "/api/auth/password", strings.NewReader(`{"current_password":"correct horse battery staple","new_password":"a different secure passphrase"}`))
	missingOrigin.AddCookie(firstCookie)
	missingOriginRecorder := httptest.NewRecorder()
	handler.ServeHTTP(missingOriginRecorder, missingOrigin)
	if missingOriginRecorder.Code != http.StatusForbidden {
		t.Fatalf("password change without trusted origin returned %d", missingOriginRecorder.Code)
	}

	wrongCurrent := httptest.NewRequest(http.MethodPut, "/api/auth/password", strings.NewReader(`{"current_password":"wrong current password","new_password":"a different secure passphrase"}`))
	wrongCurrent.Header.Set("Origin", "http://localhost:3000")
	wrongCurrent.AddCookie(firstCookie)
	wrongRecorder := httptest.NewRecorder()
	handler.ServeHTTP(wrongRecorder, wrongCurrent)
	if wrongRecorder.Code != http.StatusUnauthorized || !strings.Contains(wrongRecorder.Body.String(), `"code":"invalid_current_password"`) {
		t.Fatalf("wrong current password returned %d %s", wrongRecorder.Code, wrongRecorder.Body.String())
	}
	if _, err := sessions.Get(context.Background(), secondCookie.Value); err != nil {
		t.Fatal("rejected password change revoked sessions")
	}

	change := httptest.NewRequest(http.MethodPut, "/api/auth/password", strings.NewReader(`{"current_password":"correct horse battery staple","new_password":"a different secure passphrase"}`))
	change.Header.Set("Origin", "http://localhost:3000")
	change.AddCookie(firstCookie)
	changeRecorder := httptest.NewRecorder()
	handler.ServeHTTP(changeRecorder, change)
	if changeRecorder.Code != http.StatusNoContent {
		t.Fatalf("password change failed: %d %s", changeRecorder.Code, changeRecorder.Body.String())
	}
	for _, cookie := range []*http.Cookie{firstCookie, secondCookie} {
		if _, err := sessions.Get(context.Background(), cookie.Value); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatal("password change retained a session")
		}
	}

	login = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"person@example.com","password":"a different secure passphrase"}`))
	login.Header.Set("Origin", "http://localhost:3000")
	loginRecorder = httptest.NewRecorder()
	handler.ServeHTTP(loginRecorder, login)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("new password login failed: %d %s", loginRecorder.Code, loginRecorder.Body.String())
	}
	newCookie := loginRecorder.Result().Cookies()[0]
	logoutAll := httptest.NewRequest(http.MethodPost, "/api/auth/logout-all", nil)
	logoutAll.Header.Set("Origin", "http://localhost:3000")
	logoutAll.AddCookie(newCookie)
	logoutRecorder := httptest.NewRecorder()
	handler.ServeHTTP(logoutRecorder, logoutAll)
	if logoutRecorder.Code != http.StatusNoContent {
		t.Fatalf("logout all failed: %d %s", logoutRecorder.Code, logoutRecorder.Body.String())
	}
	if _, err := sessions.Get(context.Background(), newCookie.Value); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("logout all retained the current session")
	}
}

func TestRecoveryRequestIsGenericAndConfirmationRejectsMalformedLinks(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	sessions := auth.NewRedisStore(redisClient)
	service := auth.NewService(&authUsers{}, sessions, sessions, auditSink{}, time.Hour)
	cfg := config.Config{Database: config.Database{ReadinessTimeout: time.Second}, Auth: config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"http://localhost:3000"}}}
	handler := NewApplicationHandler(checker{}, cfg, service)

	var bodies []string
	for _, email := range []string{"person@example.com", "missing@example.com"} {
		request := httptest.NewRequest(http.MethodPost, "/api/auth/password-reset/request", strings.NewReader(`{"email":"`+email+`"}`))
		request.Header.Set("Origin", "http://localhost:3000")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("generic reset request returned %d: %s", recorder.Code, recorder.Body.String())
		}
		bodies = append(bodies, recorder.Body.String())
	}
	if bodies[0] != bodies[1] || strings.Contains(bodies[0], "person@example.com") {
		t.Fatalf("recovery response exposed account state: %#v", bodies)
	}

	confirm := httptest.NewRequest(http.MethodPost, "/api/auth/password-reset/confirm", strings.NewReader(`{"token":"invalid","new_password":"a sufficiently long password"}`))
	confirm.Header.Set("Origin", "http://localhost:3000")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, confirm)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"invalid_email_link"`) {
		t.Fatalf("malformed reset link returned %d %s", recorder.Code, recorder.Body.String())
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
