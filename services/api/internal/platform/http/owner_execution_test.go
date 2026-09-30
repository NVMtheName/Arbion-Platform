package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/platform/config"
	"github.com/redis/go-redis/v9"
)

const (
	ownerExecutionTestOwner = "11111111-1111-4111-8111-111111111111"
	ownerExecutionTestOrder = "22222222-2222-4222-8222-222222222222"
	ownerExecutionTestProof = "33333333-3333-4333-8333-333333333333"
	ownerExecutionTestRoot  = "/api/personal-execution/orders"
	ownerExecutionTestURL   = ownerExecutionTestRoot + "/" + ownerExecutionTestOrder
	ownerExecutionPrepare   = `{"request_key":"44444444-4444-4444-8444-444444444444","side":"BUY","base_size":"0.001","limit_price":"30000","fee_allowance_usd":"0.10","maximum_debit_usd":"30.10"}`
)

type ownerExecutionFake struct {
	action    string
	principal authorization.Principal
	id        string
	command   any
	calls     int
	err       error
}

func (f *ownerExecutionFake) record(action string, p authorization.Principal, id string, command any) (execution.OwnerOrder, error) {
	f.action, f.principal, f.id, f.command = action, p, id, command
	f.calls++
	return execution.OwnerOrder{}, f.err
}

func (f *ownerExecutionFake) Prepare(_ context.Context, p authorization.Principal, c execution.OwnerPrepareCommand) (execution.OwnerOrder, error) {
	return f.record("prepare", p, "", c)
}
func (f *ownerExecutionFake) Get(_ context.Context, p authorization.Principal, id string) (execution.OwnerOrder, error) {
	return f.record("get", p, id, nil)
}
func (f *ownerExecutionFake) Approve(_ context.Context, p authorization.Principal, id string, c execution.OwnerApproveCommand) (execution.OwnerOrder, error) {
	return f.record("approve", p, id, c)
}
func (f *ownerExecutionFake) Send(_ context.Context, p authorization.Principal, id string, c execution.OwnerSendCommand) (execution.OwnerOrder, error) {
	return f.record("send", p, id, c)
}
func (f *ownerExecutionFake) Capture(_ context.Context, p authorization.Principal, id string) (execution.OwnerPreflight, error) {
	o, err := f.record("preflight", p, id, nil)
	return execution.OwnerPreflight{EvidenceID: ownerExecutionTestProof, Order: o}, err
}
func (f *ownerExecutionFake) Revoke(_ context.Context, p authorization.Principal, id string) (execution.OwnerOrder, error) {
	return f.record("revoke", p, id, nil)
}
func (f *ownerExecutionFake) Recover(_ context.Context, p authorization.Principal, id string) (execution.OwnerOrder, error) {
	return f.record("recover", p, id, nil)
}
func (f *ownerExecutionFake) Reconcile(_ context.Context, p authorization.Principal, id string) (execution.OwnerOrder, error) {
	return f.record("reconcile", p, id, nil)
}
func (f *ownerExecutionFake) Cancel(_ context.Context, p authorization.Principal, id string) (execution.OwnerOrder, error) {
	return f.record("cancel", p, id, nil)
}
func (f *ownerExecutionFake) Settle(_ context.Context, p authorization.Principal, id string) (execution.OwnerOrder, error) {
	return f.record("settle", p, id, nil)
}

func ownerExecutionFixture(t *testing.T) (stdhttp.Handler, *ownerExecutionFake, *stdhttp.Cookie, *auth.Service, config.Auth) {
	t.Helper()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	sessions := auth.NewRedisStore(client)
	users := &authUsers{user: auth.User{ID: ownerExecutionTestOwner, Status: "active", Role: "user", Entitlement: "founder"}}
	service := auth.NewService(users, sessions, sessions, auditSink{}, time.Hour)
	token, _, err := sessions.Create(context.Background(), ownerExecutionTestOwner, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Auth{SessionCookie: "session", SessionTTL: time.Hour, AllowedOrigins: []string{"https://owner.example"}}
	fake := &ownerExecutionFake{}
	return newOwnerExecutionHandler(cfg, service, fake), fake, &stdhttp.Cookie{Name: cfg.SessionCookie, Value: token}, service, cfg
}

func ownerExecutionRequest(h stdhttp.Handler, cookie *stdhttp.Cookie, method, path, origin, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func assertOwnerExecutionResponse(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unsafe response: status=%d headers=%v body=%s", w.Code, w.Header(), w.Body.String())
	}
}

func TestOwnerExecutionAuthenticatedExactCommandsAndSafeDTO(t *testing.T) {
	h, fake, cookie, _, _ := ownerExecutionFixture(t)
	tests := []struct{ action, method, path, body string }{
		{"prepare", "POST", ownerExecutionTestRoot, ownerExecutionPrepare},
		{"get", "GET", ownerExecutionTestURL, ""},
		{"approve", "POST", ownerExecutionTestURL + "/approve", `{"expected_digest":"` + strings.Repeat("a", 64) + `","mfa_code":"123456"}`},
		{"send", "POST", ownerExecutionTestURL + "/send", `{"evidence_id":"` + ownerExecutionTestProof + `"}`},
	}
	for _, action := range []string{"revoke", "preflight", "recover", "reconcile", "cancel", "settle"} {
		tests = append(tests, struct{ action, method, path, body string }{action, "POST", ownerExecutionTestURL + "/" + action, `{}`})
	}
	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			before := fake.calls
			w := ownerExecutionRequest(h, cookie, tc.method, tc.path, "https://owner.example", tc.body)
			assertOwnerExecutionResponse(t, w, stdhttp.StatusOK)
			wantID := ownerExecutionTestOrder
			if tc.action == "prepare" {
				wantID = ""
			}
			if fake.calls != before+1 || fake.action != tc.action || fake.id != wantID || fake.principal != (authorization.Principal{UserID: ownerExecutionTestOwner, Role: authorization.RoleUser, Entitlement: authorization.EntitlementFounder}) {
				t.Fatalf("command or authenticated identity changed: %#v", fake)
			}
			if tc.action == "prepare" {
				var want execution.OwnerPrepareCommand
				_ = json.Unmarshal([]byte(tc.body), &want)
				if !reflect.DeepEqual(fake.command, want) {
					t.Fatal("prepare economics changed")
				}
			}
			if tc.action == "approve" && fake.command != (execution.OwnerApproveCommand{ExpectedDigest: strings.Repeat("a", 64), MFACode: "123456"}) {
				t.Fatal("approval changed")
			}
			if tc.action == "send" && fake.command != (execution.OwnerSendCommand{EvidenceID: ownerExecutionTestProof}) {
				t.Fatal("send evidence changed")
			}
			var response map[string]json.RawMessage
			if json.Unmarshal(w.Body.Bytes(), &response) != nil || response["order"] == nil || (tc.action != "preflight" && len(response) != 1) || (tc.action == "preflight" && (len(response) != 2 || string(response["evidence_id"]) != `"`+ownerExecutionTestProof+`"`)) {
				t.Fatal("response widened beyond the owner DTO", w.Body.String())
			}
			wantOrder, _ := json.Marshal(execution.OwnerOrder{})
			if !reflect.DeepEqual([]byte(response["order"]), wantOrder) {
				t.Fatal("response included a non-owner projection")
			}
			for _, private := range []string{"mfa_code", "123456", "APIPrivateKey", "api_private_key", "credential_generation", "authorization_id", "ProviderPreflight", "provider_payload", "client_order_id", "provider_order_id", "preview_id", "sender_available"} {
				if strings.Contains(w.Body.String(), private) {
					t.Fatalf("response disclosed private field %q: %s", private, w.Body.String())
				}
			}
		})
	}
}

func TestOwnerExecutionSessionAndExactOriginRequiredBeforeWorkflow(t *testing.T) {
	h, fake, cookie, _, _ := ownerExecutionFixture(t)
	for _, path := range []string{ownerExecutionTestRoot, ownerExecutionTestURL + "/send", ownerExecutionTestURL + "/cancel", ownerExecutionTestURL + "/preflight"} {
		for _, c := range []*stdhttp.Cookie{nil, {Name: "session", Value: "unknown"}} {
			w := ownerExecutionRequest(h, c, "POST", path, "https://owner.example", `{}`)
			assertOwnerExecutionResponse(t, w, stdhttp.StatusUnauthorized)
		}
		for _, origin := range []string{"", "null", "http://owner.example", "https://owner.example.evil", "https://owner.example/", " https://owner.example", "https://owner.example:443"} {
			w := ownerExecutionRequest(h, cookie, "POST", path, origin, `{}`)
			assertOwnerExecutionResponse(t, w, stdhttp.StatusForbidden)
		}
	}
	assertOwnerExecutionResponse(t, ownerExecutionRequest(h, nil, "GET", ownerExecutionTestURL, "", ""), stdhttp.StatusUnauthorized)
	if fake.calls != 0 {
		t.Fatal("unauthorized request entered workflow")
	}
}

func TestOwnerExecutionRejectsAuthorityFieldsAndMalformedCommands(t *testing.T) {
	h, fake, cookie, _, _ := ownerExecutionFixture(t)
	for _, field := range []string{"owner_id", "account_id", "connection_id", "capital_bucket_id", "product_id", "client_order_id", "provider_order_id", "portfolio_id", "authorization", "credentials", "provider", "mode"} {
		body := strings.TrimSuffix(ownerExecutionPrepare, "}") + `,"` + field + `":"untrusted"}`
		assertOwnerExecutionResponse(t, ownerExecutionRequest(h, cookie, "POST", ownerExecutionTestRoot, "https://owner.example", body), stdhttp.StatusBadRequest)
	}
	for _, tc := range []struct{ path, body string }{
		{ownerExecutionTestRoot, `{}`},
		{ownerExecutionTestRoot, strings.Replace(ownerExecutionPrepare, `"side":"BUY"`, `"side":"BUY","side":"SELL"`, 1)},
		{ownerExecutionTestRoot, strings.Replace(ownerExecutionPrepare, `"side":"BUY"`, `"side":"BUY","\u0073ide":"SELL"`, 1)},
		{ownerExecutionTestRoot, strings.Replace(ownerExecutionPrepare, `"side":"BUY"`, `"Side":"BUY"`, 1)},
		{ownerExecutionTestRoot, strings.Replace(ownerExecutionPrepare, `"side":"BUY"`, `"side":null`, 1)},
		{ownerExecutionTestRoot, strings.Replace(ownerExecutionPrepare, `"base_size":"0.001"`, `"base_size":0.001`, 1)},
		{ownerExecutionTestURL + "/approve", `{"expected_digest":"digest","mfa_code":123456}`},
		{ownerExecutionTestURL + "/send", `{"evidence_id":"` + ownerExecutionTestProof + `","owner_id":"other"}`},
		{ownerExecutionTestURL + "/send", `{"evidence_id":{}}`},
	} {
		assertOwnerExecutionResponse(t, ownerExecutionRequest(h, cookie, "POST", tc.path, "https://owner.example", tc.body), stdhttp.StatusBadRequest)
	}
	for _, action := range []string{"revoke", "preflight", "recover", "reconcile", "cancel", "settle"} {
		for _, body := range []string{"", "null", "[]", `""`, `{"force":true}`, `{} {}`, "{" + strings.Repeat(" ", 4096) + "}", "{}" + strings.Repeat(" ", 4096)} {
			assertOwnerExecutionResponse(t, ownerExecutionRequest(h, cookie, "POST", ownerExecutionTestURL+"/"+action, "https://owner.example", body), stdhttp.StatusBadRequest)
		}
	}
	if fake.calls != 0 {
		t.Fatal("malformed or widened command reached workflow")
	}
}

func TestOwnerExecutionCrossOwnerIdentifierNeverChangesPrincipal(t *testing.T) {
	h, fake, cookie, _, _ := ownerExecutionFixture(t)
	fake.err = execution.ErrNotFound
	path := ownerExecutionTestRoot + "/55555555-5555-4555-8555-555555555555?owner_id=another-owner"
	w := ownerExecutionRequest(h, cookie, "GET", path, "", "")
	assertOwnerExecutionResponse(t, w, stdhttp.StatusNotFound)
	if fake.principal.UserID != ownerExecutionTestOwner || fake.id != "55555555-5555-4555-8555-555555555555" || strings.Contains(w.Body.String(), "another-owner") {
		t.Fatal("request controlled owner scope or exposed another owner")
	}
}

func TestOwnerExecutionUnsupportedMethodsNeverEnterWorkflow(t *testing.T) {
	h, fake, cookie, _, _ := ownerExecutionFixture(t)
	for _, tc := range []struct{ method, path string }{
		{"GET", ownerExecutionTestRoot}, {"DELETE", ownerExecutionTestURL}, {"HEAD", ownerExecutionTestURL},
		{"GET", ownerExecutionTestURL + "/send"}, {"PUT", ownerExecutionTestURL + "/approve"},
		{"GET", ownerExecutionTestURL + "/cancel"}, {"OPTIONS", ownerExecutionTestURL + "/settle"},
	} {
		assertOwnerExecutionResponse(t, ownerExecutionRequest(h, cookie, tc.method, tc.path, "https://owner.example", `{}`), stdhttp.StatusMethodNotAllowed)
	}
	assertOwnerExecutionResponse(t, ownerExecutionRequest(h, cookie, "POST", ownerExecutionTestURL+"/unlock", "https://owner.example", `{}`), stdhttp.StatusNotFound)
	if fake.calls != 0 {
		t.Fatal("unsupported route entered workflow")
	}
}

func TestOwnerExecutionFailuresAreSanitizedAndAmbiguityNeverInvitesRetry(t *testing.T) {
	h, fake, cookie, _, _ := ownerExecutionFixture(t)
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{errors.Join(execution.ErrNotAuthorized, execution.ErrSubmissionNotSent), 409, "EXECUTION_NOT_SENT"},
		{errors.Join(execution.ErrNotAuthorized, execution.ErrNoSendResolutionUnknown), 409, "EXECUTION_OUTCOME_UNKNOWN"},
		{execution.ErrCommitUnknown, 409, "EXECUTION_OUTCOME_UNKNOWN"},
		{execution.ErrSubmissionUnknown, 409, "EXECUTION_OUTCOME_UNKNOWN"},
		{execution.ErrCancellationUnknown, 409, "EXECUTION_OUTCOME_UNKNOWN"},
		{execution.ErrAlreadyAttempted, 409, "EXECUTION_ALREADY_ATTEMPTED"},
		{execution.ErrSubmissionRejected, 409, "EXECUTION_REJECTED"},
		{execution.ErrCancellationNotAccepted, 409, "EXECUTION_CANCELLATION_NOT_ACCEPTED"},
		{execution.ErrNotAuthorized, 403, "EXECUTION_NOT_AUTHORIZED"},
		{auth.ErrInvalidMFACode, 403, "EXECUTION_NOT_AUTHORIZED"},
		{auth.ErrRateLimited, 429, "EXECUTION_RATE_LIMITED"},
		{execution.ErrNotFound, 404, "EXECUTION_NOT_FOUND"},
		{execution.ErrInvalid, 400, "invalid_request"},
		{execution.ErrCapitalHeld, 409, "EXECUTION_REVIEW_REQUIRED"},
		{execution.ErrReconciliationBlocked, 409, "EXECUTION_REVIEW_REQUIRED"},
		{errors.New("provider body: APIPrivateKey=secret"), 503, "OWNER_EXECUTION_UNAVAILABLE"},
	} {
		fake.err = errors.Join(tc.err, errors.New("private provider payload and secret"))
		w := ownerExecutionRequest(h, cookie, "POST", ownerExecutionTestURL+"/send", "https://owner.example", `{"evidence_id":"`+ownerExecutionTestProof+`"}`)
		assertOwnerExecutionResponse(t, w, tc.status)
		var response apiError
		if json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Error.Code != tc.code || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), `"order"`) {
			t.Fatalf("failure was unsanitized or misclassified: %s", w.Body.String())
		}
		if tc.code == "EXECUTION_OUTCOME_UNKNOWN" && !strings.Contains(response.Error.Message, "do not retry") {
			t.Fatal("ambiguous outcome omitted no-retry instruction")
		}
	}
}

func TestOwnerExecutionConstructorRemainsUnmountedAndMissingDependenciesFailClosed(t *testing.T) {
	_, _, cookie, service, cfg := ownerExecutionFixture(t)
	app := NewApplicationHandler(checker{}, config.Config{Auth: cfg}, service)
	for _, tc := range []struct{ method, path string }{{"POST", ownerExecutionTestRoot}, {"GET", ownerExecutionTestURL}, {"POST", ownerExecutionTestURL + "/send"}} {
		w := ownerExecutionRequest(app, cookie, tc.method, tc.path, "https://owner.example", `{}`)
		if w.Code != stdhttp.StatusNotFound {
			t.Fatalf("default application activated owner execution: %d", w.Code)
		}
	}
	for _, h := range []stdhttp.Handler{NewOwnerExecutionHandler(cfg, service, nil), newOwnerExecutionHandler(cfg, nil, &ownerExecutionFake{})} {
		w := ownerExecutionRequest(h, cookie, "POST", ownerExecutionTestURL+"/send", "https://owner.example", `{}`)
		assertOwnerExecutionResponse(t, w, stdhttp.StatusServiceUnavailable)
	}
}
