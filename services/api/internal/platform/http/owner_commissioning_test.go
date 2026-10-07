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

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/platform/config"
)

type ownerCommissioningFake struct {
	review         execution.OwnerCommissioningReview
	err, reviewErr error
	action         string
	args           []string
	principal      authorization.Principal
	calls          int
}

func (f *ownerCommissioningFake) Review(context.Context, authorization.Principal) (execution.OwnerCommissioningReview, error) {
	return f.review, f.reviewErr
}
func (f *ownerCommissioningFake) record(p authorization.Principal, action string, args ...string) {
	f.principal, f.action, f.args = p, action, args
	f.calls++
}
func (f *ownerCommissioningFake) receipt() execution.OwnerCommissioningReceipt {
	return execution.OwnerCommissioningReceipt{OwnerCommissioningReview: f.review, SnapshotDigest: strings.Repeat("b", 64)}
}
func (f *ownerCommissioningFake) Read(_ context.Context, p authorization.Principal) (execution.OwnerCommissioningReceipt, error) {
	f.record(p, "read")
	return f.receipt(), f.err
}
func (f *ownerCommissioningFake) Prepare(_ context.Context, p authorization.Principal, digest string) (execution.OwnerCommissioningReceipt, error) {
	f.record(p, "prepare", digest)
	return f.receipt(), f.err
}
func (f *ownerCommissioningFake) Approve(_ context.Context, p authorization.Principal, terms, snapshot, code string) (execution.MandateConsent, error) {
	f.record(p, "approve", terms, snapshot, code)
	return execution.MandateConsent{}, f.err
}
func (f *ownerCommissioningFake) Consent(_ context.Context, p authorization.Principal) (execution.OwnerCommissioningConsent, error) {
	f.principal = p
	return execution.OwnerCommissioningConsent{ID: ownerExecutionTestOrder, SnapshotDigest: strings.Repeat("b", 64), ApprovedAt: time.Date(2026, 10, 7, 1, 0, 0, 0, time.UTC), ExpiresAt: time.Date(2026, 10, 7, 2, 0, 0, 0, time.UTC)}, f.err
}
func (f *ownerCommissioningFake) RevokeInitialConsent(ctx context.Context, p authorization.Principal) (execution.OwnerCommissioningConsent, error) {
	f.record(p, "revoke")
	return f.Consent(ctx, p)
}

func commissioningHTTPFixture(t *testing.T) (stdhttp.Handler, *ownerCommissioningFake, *stdhttp.Cookie) {
	_, _, cookie, service, cfg := ownerExecutionFixture(t)
	f := &ownerCommissioningFake{review: execution.OwnerCommissioningReview{TermsDigest: strings.Repeat("a", 64), MandateID: "private-mandate", MandateVersion: 1,
		Terms: execution.OwnerCommissioningTerms{Pilot: execution.PilotAllocation{OwnerID: "private-owner", AccountID: "private-account", ConnectionID: "private-connection", CapitalBucketID: "private-bucket", ProductID: "BTC-USD", InitialCashUSD: "100", Limits: execution.OwnerPilotLimits{MaximumOrderUSD: "25", ExpiresAt: time.Date(2026, 10, 7, 2, 0, 0, 0, time.UTC)}},
			AIConnectionID: "private-ai", AIModelID: "test-model", Objective: "Explicit fixed objective", IntervalMinutes: 30, MaxTradesPerDay: 2, MaxCapitalDeployedUSD: "100", MaxSinglePositionUSD: "25", MinimumCashReserveUSD: "0", EffectiveFrom: time.Date(2026, 10, 7, 1, 0, 0, 0, time.UTC)}}}
	return newOwnerCommissioningHandler(cfg, service, f), f, cookie
}

func commissioningHTTPRequest(h stdhttp.Handler, cookie *stdhttp.Cookie, method, path, origin, body string, bindings ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, ownerCommissioningRoot+path, strings.NewReader(body))
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	for _, b := range bindings {
		r.Header.Add("X-Arbion-Execution-Session", b)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestOwnerCommissioningHTTPExactAuthenticatedCommandsAndSafeProjection(t *testing.T) {
	h, f, cookie := commissioningHTTPFixture(t)
	binding := ownerCommissioningBinding(cookie.Value, f.review.TermsDigest)
	contextResponse := commissioningHTTPRequest(h, cookie, "GET", "/context", "", "")
	assertOwnerExecutionResponse(t, contextResponse, 200)
	var contextView struct {
		Available bool                   `json:"available"`
		Binding   string                 `json:"session_binding"`
		Review    ownerCommissioningView `json:"review"`
	}
	if json.Unmarshal(contextResponse.Body.Bytes(), &contextView) != nil || !contextView.Available || contextView.Binding != binding || contextView.Review != commissioningView(f.review) {
		t.Fatal("context omitted exact terms")
	}
	for _, tc := range []struct {
		method, path, body, action string
		args                       []string
	}{
		{"GET", "/receipt", "", "read", nil},
		{"POST", "/prepare", `{"expected_terms_digest":"terms"}`, "prepare", []string{"terms"}},
		{"POST", "/approve", `{"expected_terms_digest":"terms","expected_snapshot_digest":"snapshot","mfa_code":"123456"}`, "approve", []string{"terms", "snapshot", "123456"}},
		{"POST", "/revoke", `{}`, "revoke", nil},
	} {
		t.Run(tc.action, func(t *testing.T) {
			before := f.calls
			w := commissioningHTTPRequest(h, cookie, tc.method, tc.path, "https://owner.example", tc.body, binding)
			assertOwnerExecutionResponse(t, w, 200)
			if f.calls != before+1 || f.action != tc.action || !reflect.DeepEqual(f.args, tc.args) || f.principal.UserID != ownerExecutionTestOwner || f.principal.Entitlement != authorization.EntitlementFounder {
				t.Fatal("command changed authenticated scope or arguments")
			}
			for _, private := range []string{"private-", "mfa_code", "123456", "credential_generation", "owner_id", "account_id", "connection_id", "capital_bucket_id", "mandate_id", "mfa_verified_at"} {
				if strings.Contains(w.Body.String(), private) || strings.Contains(contextResponse.Body.String(), private) {
					t.Fatalf("disclosed private field %s", private)
				}
			}
		})
	}
	w := commissioningHTTPRequest(h, cookie, "GET", "/consent", "", "", binding)
	assertOwnerExecutionResponse(t, w, 200)
	var body map[string]map[string]json.RawMessage
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body) != 1 || len(body["consent"]) != 5 || string(body["consent"]["revoked_at"]) != "null" {
		t.Fatal("consent projection expanded")
	}
}

func TestOwnerCommissioningHTTPRejectsStaleSessionTermsAndOrigins(t *testing.T) {
	h, f, cookie := commissioningHTTPFixture(t)
	binding := ownerCommissioningBinding(cookie.Value, f.review.TermsDigest)
	for _, bindings := range [][]string{nil, {""}, {binding, binding}, {ownerExecutionBinding(cookie.Value, f.review.TermsDigest)}, {ownerCommissioningBinding("oldsession", f.review.TermsDigest)}} {
		assertOwnerExecutionResponse(t, commissioningHTTPRequest(h, cookie, "POST", "/revoke", "https://owner.example", `{}`, bindings...), 401)
	}
	for _, origin := range []string{"", "null", "http://owner.example", "https://owner.example.evil", "https://owner.example/", "https://owner.example:443"} {
		assertOwnerExecutionResponse(t, commissioningHTTPRequest(h, cookie, "POST", "/revoke", origin, `{}`, binding), 403)
	}
	for _, c := range []*stdhttp.Cookie{nil, {Name: "session", Value: "expired"}} {
		assertOwnerExecutionResponse(t, commissioningHTTPRequest(h, c, "GET", "/context", "", ""), 401)
	}
	f.review.TermsDigest = strings.Repeat("c", 64)
	assertOwnerExecutionResponse(t, commissioningHTTPRequest(h, cookie, "POST", "/revoke", "https://owner.example", `{}`, binding), 401)
	f.reviewErr = execution.ErrNotAuthorized
	assertOwnerExecutionResponse(t, commissioningHTTPRequest(h, cookie, "GET", "/context", "", ""), 403)
	if f.calls != 0 {
		t.Fatal("invalid scope/origin reached command")
	}
}

func TestOwnerCommissioningHTTPStrictPayloadsAndMethods(t *testing.T) {
	h, f, cookie := commissioningHTTPFixture(t)
	binding := ownerCommissioningBinding(cookie.Value, f.review.TermsDigest)
	for _, body := range []string{`{}`, `null`, `[]`, `{"expected_terms_digest":null}`, `{"expected_terms_digest":2}`, `{"expected_terms_digest":"a","expected_terms_digest":"b"}`, `{"Expected_Terms_Digest":"a"}`, `{"expected_terms_digest":"a","owner_id":"untrusted"}`, `{"expected_terms_digest":"a"}{}`, `{"expected_terms_digest":"` + strings.Repeat("x", 5000) + `"}`} {
		assertOwnerExecutionResponse(t, commissioningHTTPRequest(h, cookie, "POST", "/prepare", "https://owner.example", body, binding), 400)
	}
	assertOwnerExecutionResponse(t, commissioningHTTPRequest(h, cookie, "POST", "/revoke", "https://owner.example", `{"consent_id":"untrusted"}`, binding), 400)
	for _, path := range []string{"/context", "/receipt", "/consent"} {
		assertOwnerExecutionResponse(t, commissioningHTTPRequest(h, cookie, "HEAD", path, "", "", binding), 405)
	}
	for _, path := range []string{"/prepare", "/approve", "/revoke"} {
		assertOwnerExecutionResponse(t, commissioningHTTPRequest(h, cookie, "GET", path, "", "", binding), 405)
	}
	for _, path := range []string{"/send", "/orders", "/context/", "/receipt/", "/activate"} {
		assertOwnerExecutionResponse(t, commissioningHTTPRequest(h, cookie, "GET", path, "", "", binding), 404)
	}
	if f.calls != 0 {
		t.Fatal("malformed input reached workflow")
	}
}

func TestOwnerCommissioningHTTPUnknownOutcomesStayErrorsAndPrivate(t *testing.T) {
	h, f, cookie := commissioningHTTPFixture(t)
	binding := ownerCommissioningBinding(cookie.Value, f.review.TermsDigest)
	for _, tc := range []struct {
		err    error
		status int
	}{{execution.ErrCommitUnknown, 409}, {execution.ErrNotFound, 404}, {execution.ErrNotAuthorized, 403}, {errors.New("private database secret"), 503}} {
		f.err = tc.err
		w := commissioningHTTPRequest(h, cookie, "POST", "/prepare", "https://owner.example", `{"expected_terms_digest":"a"}`, binding)
		assertOwnerExecutionResponse(t, w, tc.status)
		if strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "receipt") {
			t.Fatal("error leaked internals or success")
		}
	}
}

func TestOwnerCommissioningNilMountPrecedesOwnerExecutionWithoutActivation(t *testing.T) {
	_, _, cookie, service, cfg := ownerExecutionFixture(t)
	fallback := 0
	base := stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) { fallback++; w.WriteHeader(418) })
	h := WithOwnerCommissioning(WithOwnerExecution(base, config.Config{Auth: cfg}, service, nil), config.Config{Auth: cfg}, service, nil)
	for _, path := range []string{"/context", "/receipt", "/consent", "/prepare", "/approve", "/revoke", "/activate"} {
		assertOwnerExecutionResponse(t, commissioningHTTPRequest(h, nil, "GET", path, "", ""), 401)
		status := 503
		if path == "/context" {
			status = 200
		}
		w := commissioningHTTPRequest(h, cookie, "GET", path, "", "")
		assertOwnerExecutionResponse(t, w, status)
		if path == "/context" && strings.TrimSpace(w.Body.String()) != `{"available":false}` {
			t.Fatal("nil mount exposed terms")
		}
	}
	w := ownerExecutionRequest(h, cookie, "GET", "/api/personal-execution/context", "", "")
	assertOwnerExecutionResponse(t, w, 200)
	w = ownerExecutionRequest(h, cookie, "GET", "/unrelated", "", "")
	if w.Code != 418 || fallback != 1 {
		t.Fatal("mount escaped namespace")
	}
}
