package risk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

type breakerStoreFake struct {
	owned           bool
	current         *CircuitBreaker
	engaged         CircuitBreaker
	released        CircuitBreaker
	engageUser      string
	engageMandate   string
	engageReason    string
	engageTime      time.Time
	releaseUser     string
	releaseMandate  string
	releaseTime     time.Time
	accountCurrent  *CircuitBreaker
	accountEngaged  CircuitBreaker
	accountReleased CircuitBreaker
	engageAccount   string
	releaseAccount  string
	userCurrent     *CircuitBreaker
	userEngaged     CircuitBreaker
	userReleased    CircuitBreaker
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
func (store *breakerStoreFake) AccountOwned(context.Context, string, string) (bool, error) {
	return store.owned, nil
}
func (store *breakerStoreFake) OpenAccountBreaker(context.Context, string, string) (*CircuitBreaker, error) {
	return store.accountCurrent, nil
}
func (store *breakerStoreFake) EngageAccountBreaker(_ context.Context, userID, accountID, reason string, at time.Time) (CircuitBreaker, error) {
	store.engageUser, store.engageAccount, store.engageReason, store.engageTime = userID, accountID, reason, at
	return store.accountEngaged, nil
}
func (store *breakerStoreFake) ReleaseAccountBreaker(_ context.Context, userID, accountID string, at time.Time) (CircuitBreaker, error) {
	store.releaseUser, store.releaseAccount, store.releaseTime = userID, accountID, at
	return store.accountReleased, nil
}
func (store *breakerStoreFake) UserExists(context.Context, string) (bool, error) {
	return store.owned, nil
}
func (store *breakerStoreFake) OpenUserBreaker(context.Context, string) (*CircuitBreaker, error) {
	return store.userCurrent, nil
}
func (store *breakerStoreFake) EngageUserBreaker(_ context.Context, userID, reason string, at time.Time) (CircuitBreaker, error) {
	store.engageUser, store.engageReason, store.engageTime = userID, reason, at
	return store.userEngaged, nil
}
func (store *breakerStoreFake) ReleaseUserBreaker(_ context.Context, userID string, at time.Time) (CircuitBreaker, error) {
	store.releaseUser, store.releaseTime = userID, at
	return store.userReleased, nil
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

func TestAccountBreakerEngageAndReleaseAreOwnerScopedAndAudited(t *testing.T) {
	now := time.Date(2026, 8, 27, 14, 0, 0, 0, time.UTC)
	accountID := "account"
	engaged := CircuitBreaker{ID: "breaker", Scope: ScopeAccount, ScopeID: &accountID, State: BreakerOpen, Reason: "account connectivity requires review", Source: "UI", EngagedAt: now}
	released := engaged
	released.State = BreakerClosed
	released.ReleasedAt = &now
	store := &breakerStoreFake{owned: true, accountEngaged: engaged, accountReleased: released, accountCurrent: &engaged}
	audit := &breakerAuditFake{}
	service := NewBreakerService(store, audit)
	service.now = func() time.Time { return now }

	current, err := service.CurrentAccount(context.Background(), founderPrincipal(), accountID)
	if err != nil || current == nil || current.Scope != ScopeAccount {
		t.Fatalf("account breaker was not owner-scoped: current=%#v err=%v", current, err)
	}
	result, err := service.EngageAccount(context.Background(), founderPrincipal(), accountID, BreakerCommand{Reason: "  account connectivity requires review  ", Confirm: true})
	if err != nil || result.State != BreakerOpen || store.engageUser != "owner" || store.engageAccount != accountID || store.engageReason != "account connectivity requires review" || !store.engageTime.Equal(now) {
		t.Fatalf("account engage changed: result=%#v store=%#v err=%v", result, store, err)
	}
	if audit.action != "account_circuit_breaker.engaged" || audit.metadata["financial_account_id"] != accountID || audit.metadata["scope"] != ScopeAccount || audit.metadata["broker_execution_requested"] != false {
		t.Fatalf("account engage audit evidence is incomplete: %#v", audit)
	}

	result, err = service.ReleaseAccount(context.Background(), founderPrincipal(), accountID, BreakerCommand{Reason: "connection and holdings were reviewed", Confirm: true})
	if err != nil || result.State != BreakerClosed || store.releaseUser != "owner" || store.releaseAccount != accountID || !store.releaseTime.Equal(now) {
		t.Fatalf("account release changed: result=%#v store=%#v err=%v", result, store, err)
	}
	if audit.action != "account_circuit_breaker.released" || audit.metadata["reason"] != "connection and holdings were reviewed" || audit.metadata["live_execution_available"] != false {
		t.Fatalf("account release audit evidence is incomplete: %#v", audit)
	}
}

func TestAccountBreakerRejectsUnownedAndInvalidCommands(t *testing.T) {
	store := &breakerStoreFake{owned: true}
	service := NewBreakerService(store, nil)
	if _, err := service.EngageAccount(context.Background(), founderPrincipal(), "account", BreakerCommand{Reason: "planned maintenance", Confirm: false}); !errors.Is(err, ErrBreakerInvalid) {
		t.Fatalf("unconfirmed account stop was accepted: %v", err)
	}
	store.owned = false
	if _, err := service.CurrentAccount(context.Background(), founderPrincipal(), "other"); !errors.Is(err, ErrBreakerNotFound) {
		t.Fatalf("unowned account was not hidden: %v", err)
	}
	if _, err := service.ReleaseAccount(context.Background(), founderPrincipal(), "other", BreakerCommand{Reason: "connection was reviewed", Confirm: true}); !errors.Is(err, ErrBreakerNotFound) {
		t.Fatalf("unowned account release was accepted: %v", err)
	}
}

func TestUserBreakerEngageAndReleaseAreAuthenticatedAndAudited(t *testing.T) {
	now := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	userID := "owner"
	engaged := CircuitBreaker{ID: "breaker", Scope: ScopeUser, ScopeID: &userID, State: BreakerOpen, Reason: "owner is stopping all new actions", Source: "UI", EngagedAt: now}
	released := engaged
	released.State = BreakerClosed
	released.ReleasedAt = &now
	store := &breakerStoreFake{owned: true, userEngaged: engaged, userReleased: released, userCurrent: &engaged}
	audit := &breakerAuditFake{}
	service := NewBreakerService(store, audit)
	service.now = func() time.Time { return now }

	current, err := service.CurrentUser(context.Background(), founderPrincipal())
	if err != nil || current == nil || current.Scope != ScopeUser {
		t.Fatalf("user breaker was not owner-scoped: current=%#v err=%v", current, err)
	}
	result, err := service.EngageUser(context.Background(), founderPrincipal(), BreakerCommand{Reason: "  owner is stopping all new actions  ", Confirm: true})
	if err != nil || result.State != BreakerOpen || store.engageUser != userID || store.engageReason != "owner is stopping all new actions" || !store.engageTime.Equal(now) {
		t.Fatalf("user engage changed: result=%#v store=%#v err=%v", result, store, err)
	}
	if audit.action != "user_circuit_breaker.engaged" || audit.metadata["subject_user_id"] != userID || audit.metadata["scope"] != ScopeUser || audit.metadata["broker_execution_requested"] != false {
		t.Fatalf("user engage audit evidence is incomplete: %#v", audit)
	}

	result, err = service.ReleaseUser(context.Background(), founderPrincipal(), BreakerCommand{Reason: "all connected accounts were reviewed", Confirm: true})
	if err != nil || result.State != BreakerClosed || store.releaseUser != userID || !store.releaseTime.Equal(now) {
		t.Fatalf("user release changed: result=%#v store=%#v err=%v", result, store, err)
	}
	if audit.action != "user_circuit_breaker.released" || audit.metadata["reason"] != "all connected accounts were reviewed" || audit.metadata["live_execution_available"] != false {
		t.Fatalf("user release audit evidence is incomplete: %#v", audit)
	}
}

func TestUserBreakerRequiresEntitlementConfirmationAndExistingUser(t *testing.T) {
	store := &breakerStoreFake{owned: true}
	service := NewBreakerService(store, nil)
	if _, err := service.EngageUser(context.Background(), founderPrincipal(), BreakerCommand{Reason: "owner requested global stop", Confirm: false}); !errors.Is(err, ErrBreakerInvalid) {
		t.Fatalf("unconfirmed user stop was accepted: %v", err)
	}
	if _, err := service.CurrentUser(context.Background(), authorization.Principal{UserID: "basic", Entitlement: authorization.EntitlementFree}); !errors.Is(err, ErrBreakerForbidden) {
		t.Fatalf("unentitled user reached global stop state: %v", err)
	}
	store.owned = false
	if _, err := service.ReleaseUser(context.Background(), founderPrincipal(), BreakerCommand{Reason: "all accounts were reviewed", Confirm: true}); !errors.Is(err, ErrBreakerNotFound) {
		t.Fatalf("missing user release was accepted: %v", err)
	}
}
