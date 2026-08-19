package risk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

type breakerStoreFake struct {
	owned          bool
	current        *CircuitBreaker
	engaged        CircuitBreaker
	released       CircuitBreaker
	engageUser     string
	engageMandate  string
	engageReason   string
	engageTime     time.Time
	releaseUser    string
	releaseMandate string
	releaseTime    time.Time
}

func (store *breakerStoreFake) AutomationOwned(context.Context, string, string) (bool, error) {
	return store.owned, nil
}
func (store *breakerStoreFake) OpenAutomationBreaker(context.Context, string, string) (*CircuitBreaker, error) {
	return store.current, nil
}
func (store *breakerStoreFake) EngageAutomationBreaker(_ context.Context, userID, automationID, reason string, at time.Time) (CircuitBreaker, error) {
	store.engageUser, store.engageMandate, store.engageReason, store.engageTime = userID, automationID, reason, at
	return store.engaged, nil
}
func (store *breakerStoreFake) ReleaseAutomationBreaker(_ context.Context, userID, automationID string, at time.Time) (CircuitBreaker, error) {
	store.releaseUser, store.releaseMandate, store.releaseTime = userID, automationID, at
	return store.released, nil
}

type breakerAuditFake struct {
	userID   *string
	action   string
	metadata map[string]any
}

func (audit *breakerAuditFake) Record(_ context.Context, userID *string, action string, metadata map[string]any) error {
	audit.userID, audit.action, audit.metadata = userID, action, metadata
	return nil
}

func founderPrincipal() authorization.Principal {
	return authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}
}

func TestAutomationBreakerRequiresOwnerConfirmationAndReason(t *testing.T) {
	store := &breakerStoreFake{owned: true}
	service := NewBreakerService(store, nil)

	for _, test := range []BreakerCommand{
		{Reason: "planned maintenance", Confirm: false},
		{Reason: "short", Confirm: true},
		{Reason: "invalid\nreason", Confirm: true},
	} {
		if _, err := service.EngageAutomation(context.Background(), founderPrincipal(), "mandate", test); !errors.Is(err, ErrBreakerInvalid) {
			t.Fatalf("unsafe command was accepted: %#v err=%v", test, err)
		}
	}
	if store.engageMandate != "" {
		t.Fatal("invalid request reached persistence")
	}

	store.owned = false
	if _, err := service.EngageAutomation(context.Background(), founderPrincipal(), "other", BreakerCommand{Reason: "owner requested stop", Confirm: true}); !errors.Is(err, ErrBreakerNotFound) {
		t.Fatalf("unowned mandate was not hidden: %v", err)
	}
	if _, err := service.CurrentAutomation(context.Background(), authorization.Principal{UserID: "basic", Entitlement: authorization.EntitlementFree}, "mandate"); !errors.Is(err, ErrBreakerForbidden) {
		t.Fatalf("unentitled principal reached breaker state: %v", err)
	}
}

func TestAutomationBreakerEngageAndReleaseAreOwnerScopedAndAudited(t *testing.T) {
	now := time.Date(2026, 8, 19, 17, 30, 0, 0, time.UTC)
	automationID := "mandate"
	engaged := CircuitBreaker{ID: "breaker", Scope: ScopeAutomation, ScopeID: &automationID, State: BreakerOpen, Reason: "owner requested stop", Source: "UI", EngagedAt: now}
	released := engaged
	released.State = BreakerClosed
	released.ReleasedAt = &now
	store := &breakerStoreFake{owned: true, engaged: engaged, released: released}
	audit := &breakerAuditFake{}
	service := NewBreakerService(store, audit)
	service.now = func() time.Time { return now }

	result, err := service.EngageAutomation(context.Background(), founderPrincipal(), automationID, BreakerCommand{Reason: "  owner requested stop  ", Confirm: true})
	if err != nil || result.State != BreakerOpen || store.engageUser != "owner" || store.engageMandate != automationID || store.engageReason != "owner requested stop" || !store.engageTime.Equal(now) {
		t.Fatalf("engage was not owner-scoped: result=%#v store=%#v err=%v", result, store, err)
	}
	if audit.userID == nil || *audit.userID != "owner" || audit.action != "automation_circuit_breaker.engaged" || audit.metadata["automation_id"] != automationID || audit.metadata["broker_execution_requested"] != false {
		t.Fatalf("engage audit evidence is incomplete: %#v", audit)
	}

	result, err = service.ReleaseAutomation(context.Background(), founderPrincipal(), automationID, BreakerCommand{Reason: "cause reviewed and cleared", Confirm: true})
	if err != nil || result.State != BreakerClosed || store.releaseUser != "owner" || store.releaseMandate != automationID || !store.releaseTime.Equal(now) {
		t.Fatalf("release was not owner-scoped: result=%#v store=%#v err=%v", result, store, err)
	}
	if audit.action != "automation_circuit_breaker.released" || audit.metadata["reason"] != "cause reviewed and cleared" || audit.metadata["live_execution_available"] != false {
		t.Fatalf("release audit evidence is incomplete: %#v", audit)
	}
}

func TestCurrentAutomationBreakerReturnsOpenStateOrNil(t *testing.T) {
	automationID := "mandate"
	breaker := &CircuitBreaker{ID: "breaker", Scope: ScopeAutomation, ScopeID: &automationID, State: BreakerOpen}
	store := &breakerStoreFake{owned: true, current: breaker}
	service := NewBreakerService(store, nil)

	current, err := service.CurrentAutomation(context.Background(), founderPrincipal(), automationID)
	if err != nil || current == nil || current.ID != "breaker" {
		t.Fatalf("open breaker not returned: %#v err=%v", current, err)
	}
	store.current = nil
	current, err = service.CurrentAutomation(context.Background(), founderPrincipal(), automationID)
	if err != nil || current != nil {
		t.Fatalf("cleared breaker did not return nil: %#v err=%v", current, err)
	}
}
