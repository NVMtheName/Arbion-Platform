package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/platformops"
)

type platformOperationsFake struct {
	overview  platformops.Overview
	err       error
	principal authorization.Principal
	calls     int
}

func (fake *platformOperationsFake) Overview(_ context.Context, principal authorization.Principal) (platformops.Overview, error) {
	fake.calls++
	fake.principal = principal
	return fake.overview, fake.err
}

func platformOperationsRequest(role string) *stdhttp.Request {
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/admin/operations/readiness", nil)
	user := auth.SafeUser{ID: "operator", Role: role, Entitlement: "founder"}
	return request.WithContext(context.WithValue(request.Context(), identityKey{}, user))
}

func TestPlatformOperationsTransportIsSuperadminOnlyNoStoreAndNonExecuting(t *testing.T) {
	now := time.Date(2026, 8, 27, 23, 30, 0, 0, time.UTC)
	fake := &platformOperationsFake{overview: platformops.Overview{
		GeneratedAt:             now,
		OperationalStatus:       platformops.StatusNominal,
		ActiveAIShadowInstances: 2,
		Signals: []platformops.Signal{{
			Code: "SHADOW_EXECUTION_BOUNDARY", State: platformops.SignalPass,
		}},
	}}
	handler := &authHandler{platformOperations: fake}
	recorder := httptest.NewRecorder()
	handler.platformOperationsReadiness(recorder, platformOperationsRequest("superadmin"))
	if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected operations response: %d %#v %s", recorder.Code, recorder.Header(), recorder.Body.String())
	}
	if fake.principal.UserID != "operator" || fake.principal.Role != authorization.RoleSuperadmin {
		t.Fatalf("operations principal changed: %#v", fake.principal)
	}
	var response struct {
		Operations             platformops.Overview `json:"operations"`
		LiveExecutionAvailable bool                 `json:"live_execution_available"`
		BrokerActionRequested  bool                 `json:"broker_action_requested"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Operations.ActiveAIShadowInstances != 2 || response.LiveExecutionAvailable || response.BrokerActionRequested {
		t.Fatalf("operations boundary changed: %#v err=%v", response, err)
	}
}

func TestPlatformOperationsMiddlewareRejectsOrdinaryAdminBeforeController(t *testing.T) {
	fake := &platformOperationsFake{}
	handler := &authHandler{platformOperations: fake}
	recorder := httptest.NewRecorder()
	handler.requireSuperadmin(stdhttp.HandlerFunc(handler.platformOperationsReadiness)).ServeHTTP(recorder, platformOperationsRequest("admin"))
	if recorder.Code != stdhttp.StatusForbidden || fake.calls != 0 {
		t.Fatalf("ordinary admin reached platform operations: status=%d calls=%d body=%s", recorder.Code, fake.calls, recorder.Body.String())
	}
}

func TestPlatformOperationsMapsFreshRoleFailure(t *testing.T) {
	fake := &platformOperationsFake{err: platformops.ErrSuperadminRequired}
	handler := &authHandler{platformOperations: fake}
	recorder := httptest.NewRecorder()
	handler.platformOperationsReadiness(recorder, platformOperationsRequest("superadmin"))
	if recorder.Code != stdhttp.StatusForbidden {
		t.Fatalf("stale superadmin failure changed: %d %s", recorder.Code, recorder.Body.String())
	}
}
