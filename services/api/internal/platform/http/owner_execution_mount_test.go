package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/platform/config"
)

func TestOwnerExecutionDefaultMountAuthenticatesAndNeverRunsCommands(t *testing.T) {
	_, _, cookie, service, cfg := ownerExecutionFixture(t)
	var fallback int
	base := stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) { fallback++; w.WriteHeader(418) })
	h := WithOwnerExecution(base, config.Config{Auth: cfg}, service, nil)
	for _, path := range []string{"/api/personal-execution/context", ownerExecutionTestRoot, ownerExecutionTestURL, ownerExecutionTestURL + "/send", ownerExecutionTestURL + "/cancel"} {
		assertOwnerExecutionResponse(t, ownerExecutionRequest(h, nil, "GET", path, "", ""), 401)
		if path == "/api/personal-execution/context" {
			w := ownerExecutionRequest(h, cookie, "GET", path, "", "")
			assertOwnerExecutionResponse(t, w, 200)
			if strings.TrimSpace(w.Body.String()) != `{"available":false}` {
				t.Fatal("disabled context exposed data")
			}
			continue
		}
		assertOwnerExecutionResponse(t, ownerExecutionRequest(h, cookie, "POST", path, "https://owner.example", `{}`), 503)
	}
	assertOwnerExecutionResponse(t, ownerExecutionRequest(h, cookie, "HEAD", "/api/personal-execution/context", "", ""), 405)
	for _, path := range []string{"/healthz", "/api/personal-execution-other", "/api/accounts/test"} {
		if w := ownerExecutionRequest(h, cookie, "GET", path, "", ""); w.Code != 418 {
			t.Fatal("mount intercepted another namespace")
		}
	}
	if fallback != 3 {
		t.Fatal("disconnected execution fell through to another handler")
	}
}

func TestOwnerExecutionContextBindingRejectsReplacementSessionsAndScope(t *testing.T) {
	h, fake, cookie, service, _ := ownerExecutionFixture(t)
	w := ownerExecutionRequest(h, cookie, "GET", "/api/personal-execution/context", "", "")
	assertOwnerExecutionResponse(t, w, 200)
	var v struct {
		Available bool   `json:"available"`
		Product   string `json:"product_id"`
		Label     string `json:"account_label"`
		Binding   string `json:"session_binding"`
	}
	if json.Unmarshal(w.Body.Bytes(), &v) != nil || !v.Available || v.Product != "BTC-USD" || v.Label != "Dedicated Coinbase portfolio" || len(v.Binding) != 64 || strings.Contains(w.Body.String(), cookie.Value) || strings.Contains(w.Body.String(), "fixed-test-scope") || fake.calls != 0 {
		t.Fatal("unsafe owner context")
	}
	request := func(bindings []string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", ownerExecutionTestURL+"/send", strings.NewReader(`{"evidence_id":"`+ownerExecutionTestProof+`"}`))
		r.AddCookie(cookie)
		r.Header.Set("Origin", "https://owner.example")
		for _, binding := range bindings {
			r.Header.Add("X-Arbion-Execution-Session", binding)
		}
		out := httptest.NewRecorder()
		h.ServeHTTP(out, r)
		return out
	}
	for _, bindings := range [][]string{nil, {""}, {strings.Repeat("a", 64)}, {v.Binding, v.Binding}, {ownerExecutionBinding("another-session", "fixed-test-scope")}} {
		out := request(bindings)
		assertOwnerExecutionResponse(t, out, 401)
		if !strings.Contains(out.Body.String(), "execution_session_changed") {
			t.Fatal("missing invalidation response")
		}
	}
	fake.scopeID = "changed-fixed-scope"
	assertOwnerExecutionResponse(t, request([]string{v.Binding}), 401)
	if fake.calls != 0 {
		t.Fatal("stale session or scope reached sender")
	}
	fake.scopeID = ""
	assertOwnerExecutionResponse(t, request([]string{v.Binding}), 200)
	if fake.calls != 1 {
		t.Fatal("exact current session failed")
	}
	if err := service.Logout(context.Background(), cookie.Value, nil); err != nil {
		t.Fatal(err)
	}
	assertOwnerExecutionResponse(t, request([]string{v.Binding}), 401)
	assertOwnerExecutionResponse(t, ownerExecutionRequest(h, cookie, "GET", "/api/personal-execution/context", "", ""), 401)
	if fake.calls != 1 {
		t.Fatal("revoked session reached sender")
	}
}

func TestOwnerExecutionPresentationRequiresCurrentOwnerAccess(t *testing.T) {
	h, fake, cookie, _, _ := ownerExecutionFixture(t)
	fake.viewErr = execution.ErrNotAuthorized
	for _, path := range []string{"/api/personal-execution/context", ownerExecutionTestURL} {
		assertOwnerExecutionResponse(t, ownerExecutionRequest(h, cookie, "GET", path, "", ""), 403)
	}
	if fake.calls != 0 {
		t.Fatal("unavailable owner entered command")
	}
}
