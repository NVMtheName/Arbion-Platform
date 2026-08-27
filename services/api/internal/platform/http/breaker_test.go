package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/platform/config"
	"github.com/arbion/platform/services/api/internal/risk"
)

type breakerControllerFake struct {
	current         *risk.CircuitBreaker
	engaged         risk.CircuitBreaker
	released        risk.CircuitBreaker
	principal       authorization.Principal
	automationID    string
	engageCommand   risk.BreakerCommand
	releaseCommand  risk.BreakerCommand
	currentError    error
	engagementError error
	releaseError    error
	accountID       string
	accountCurrent  *risk.CircuitBreaker
	accountEngaged  risk.CircuitBreaker
	accountReleased risk.CircuitBreaker
}

func (fake *breakerControllerFake) CurrentAutomation(_ context.Context, principal authorization.Principal, automationID string) (*risk.CircuitBreaker, error) {
	fake.principal, fake.automationID = principal, automationID
	return fake.current, fake.currentError
}
func (fake *breakerControllerFake) EngageAutomation(_ context.Context, principal authorization.Principal, automationID string, command risk.BreakerCommand) (risk.CircuitBreaker, error) {
	fake.principal, fake.automationID, fake.engageCommand = principal, automationID, command
	return fake.engaged, fake.engagementError
}
func (fake *breakerControllerFake) ReleaseAutomation(_ context.Context, principal authorization.Principal, automationID string, command risk.BreakerCommand) (risk.CircuitBreaker, error) {
	fake.principal, fake.automationID, fake.releaseCommand = principal, automationID, command
	return fake.released, fake.releaseError
}
func (fake *breakerControllerFake) CurrentAccount(_ context.Context, principal authorization.Principal, accountID string) (*risk.CircuitBreaker, error) {
	fake.principal, fake.accountID = principal, accountID
	return fake.accountCurrent, fake.currentError
}
func (fake *breakerControllerFake) EngageAccount(_ context.Context, principal authorization.Principal, accountID string, command risk.BreakerCommand) (risk.CircuitBreaker, error) {
	fake.principal, fake.accountID, fake.engageCommand = principal, accountID, command
	return fake.accountEngaged, fake.engagementError
}
func (fake *breakerControllerFake) ReleaseAccount(_ context.Context, principal authorization.Principal, accountID string, command risk.BreakerCommand) (risk.CircuitBreaker, error) {
	fake.principal, fake.accountID, fake.releaseCommand = principal, accountID, command
	return fake.accountReleased, fake.releaseError
}

func breakerRequest(method, path, body string) *stdhttp.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Content-Type", "application/json")
	request.SetPathValue("id", "mandate-1")
	user := auth.SafeUser{ID: "owner", Role: "user", Entitlement: "founder"}
	return request.WithContext(context.WithValue(request.Context(), identityKey{}, user))
}

func TestAutomationBreakerTransportIsOwnerScopedAndNonLive(t *testing.T) {
	automationID := "mandate-1"
	now := time.Date(2026, 8, 19, 18, 0, 0, 0, time.UTC)
	breaker := risk.CircuitBreaker{ID: "breaker-1", Scope: risk.ScopeAutomation, ScopeID: &automationID, State: risk.BreakerOpen, Reason: "owner requested stop", Source: "UI", EngagedAt: now}
	fake := &breakerControllerFake{current: &breaker, engaged: breaker}
	handler := &authHandler{breakers: fake, cfg: config.Auth{AllowedOrigins: []string{"http://localhost:3000"}}}

	currentRecorder := httptest.NewRecorder()
	handler.currentAutomationBreaker(currentRecorder, breakerRequest(stdhttp.MethodGet, "/api/automations/mandate-1/circuit-breaker", ""))
	if currentRecorder.Code != stdhttp.StatusOK || currentRecorder.Header().Get("Cache-Control") != "no-store" || !strings.Contains(currentRecorder.Body.String(), `"live_execution_available":false`) {
		t.Fatalf("unexpected current response: %d %s", currentRecorder.Code, currentRecorder.Body.String())
	}

	engageRecorder := httptest.NewRecorder()
	handler.engageAutomationBreaker(engageRecorder, breakerRequest(stdhttp.MethodPost, "/api/automations/mandate-1/circuit-breaker/engage", `{"reason":"owner requested stop","confirm":true}`))
	if engageRecorder.Code != stdhttp.StatusOK {
		t.Fatalf("engage failed: %d %s", engageRecorder.Code, engageRecorder.Body.String())
	}
	if fake.principal.UserID != "owner" || fake.automationID != "mandate-1" || fake.engageCommand.Reason != "owner requested stop" || !fake.engageCommand.Confirm {
		t.Fatalf("engage command changed: %#v", fake)
	}
	var response map[string]any
	if err := json.Unmarshal(engageRecorder.Body.Bytes(), &response); err != nil || response["broker_action_requested"] != false {
		t.Fatalf("non-live boundary missing: %s err=%v", engageRecorder.Body.String(), err)
	}
}

func TestAutomationBreakerTransportRequiresCSRFAndMapsConflicts(t *testing.T) {
	fake := &breakerControllerFake{releaseError: risk.ErrBreakerConflict}
	handler := &authHandler{breakers: fake, cfg: config.Auth{AllowedOrigins: []string{"http://localhost:3000"}}}

	rejected := breakerRequest(stdhttp.MethodPost, "/api/automations/mandate-1/circuit-breaker/release", `{"reason":"cause reviewed and cleared","confirm":true}`)
	rejected.Header.Del("Origin")
	recorder := httptest.NewRecorder()
	handler.releaseAutomationBreaker(recorder, rejected)
	if recorder.Code != stdhttp.StatusForbidden || fake.releaseCommand.Reason != "" {
		t.Fatalf("CSRF request reached service: %d %#v", recorder.Code, fake.releaseCommand)
	}

	recorder = httptest.NewRecorder()
	handler.releaseAutomationBreaker(recorder, breakerRequest(stdhttp.MethodPost, "/api/automations/mandate-1/circuit-breaker/release", `{"reason":"cause reviewed and cleared","confirm":true}`))
	if recorder.Code != stdhttp.StatusConflict || !strings.Contains(recorder.Body.String(), "CIRCUIT_BREAKER_CONFLICT") {
		t.Fatalf("conflict was not stable: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestAccountBreakerTransportIsOwnerScopedAndNonLive(t *testing.T) {
	accountID := "account-1"
	now := time.Date(2026, 8, 27, 14, 0, 0, 0, time.UTC)
	breaker := risk.CircuitBreaker{ID: "breaker-1", Scope: risk.ScopeAccount, ScopeID: &accountID, State: risk.BreakerOpen, Reason: "account connectivity requires review", Source: "UI", EngagedAt: now}
	fake := &breakerControllerFake{accountCurrent: &breaker, accountEngaged: breaker}
	handler := &authHandler{breakers: fake, cfg: config.Auth{AllowedOrigins: []string{"http://localhost:3000"}}}

	currentRequest := breakerRequest(stdhttp.MethodGet, "/api/accounts/account-1/circuit-breaker", "")
	currentRequest.SetPathValue("id", accountID)
	currentRecorder := httptest.NewRecorder()
	handler.currentAccountBreaker(currentRecorder, currentRequest)
	if currentRecorder.Code != stdhttp.StatusOK || currentRecorder.Header().Get("Cache-Control") != "no-store" || !strings.Contains(currentRecorder.Body.String(), `"live_execution_available":false`) {
		t.Fatalf("unexpected account breaker response: %d %s", currentRecorder.Code, currentRecorder.Body.String())
	}

	engageRequest := breakerRequest(stdhttp.MethodPost, "/api/accounts/account-1/circuit-breaker/engage", `{"reason":"account connectivity requires review","confirm":true}`)
	engageRequest.SetPathValue("id", accountID)
	engageRecorder := httptest.NewRecorder()
	handler.engageAccountBreaker(engageRecorder, engageRequest)
	if engageRecorder.Code != stdhttp.StatusOK {
		t.Fatalf("account engage failed: %d %s", engageRecorder.Code, engageRecorder.Body.String())
	}
	if fake.principal.UserID != "owner" || fake.accountID != accountID || fake.engageCommand.Reason != "account connectivity requires review" || !fake.engageCommand.Confirm {
		t.Fatalf("account engage command changed: %#v", fake)
	}
	var response map[string]any
	if err := json.Unmarshal(engageRecorder.Body.Bytes(), &response); err != nil || response["broker_action_requested"] != false {
		t.Fatalf("account non-live boundary missing: %s err=%v", engageRecorder.Body.String(), err)
	}
}
