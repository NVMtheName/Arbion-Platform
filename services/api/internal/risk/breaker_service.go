package risk

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/arbion/platform/services/api/internal/authorization"
)

var (
	ErrBreakerForbidden = errors.New("automation circuit breaker entitlement required")
	ErrBreakerInvalid   = errors.New("invalid automation circuit breaker command")
	ErrBreakerNotFound  = errors.New("automation circuit breaker resource not found")
	ErrBreakerConflict  = errors.New("automation circuit breaker state conflict")
)

type BreakerStore interface {
	AutomationOwned(context.Context, string, string) (bool, error)
	OpenAutomationBreaker(context.Context, string, string) (*CircuitBreaker, error)
	EngageAutomationBreaker(context.Context, string, string, string, time.Time) (CircuitBreaker, error)
	ReleaseAutomationBreaker(context.Context, string, string, time.Time) (CircuitBreaker, error)
	AccountOwned(context.Context, string, string) (bool, error)
	OpenAccountBreaker(context.Context, string, string) (*CircuitBreaker, error)
	EngageAccountBreaker(context.Context, string, string, string, time.Time) (CircuitBreaker, error)
	ReleaseAccountBreaker(context.Context, string, string, time.Time) (CircuitBreaker, error)
}

type BreakerAuditor interface {
	Record(context.Context, *string, string, map[string]any) error
}

type BreakerService struct {
	store BreakerStore
	audit BreakerAuditor
	now   func() time.Time
}

type BreakerCommand struct {
	Reason  string `json:"reason"`
	Confirm bool   `json:"confirm"`
}

func NewBreakerService(store BreakerStore, audit BreakerAuditor) *BreakerService {
	return &BreakerService{store: store, audit: audit, now: func() time.Time { return time.Now().UTC() }}
}

func breakerAllowed(principal authorization.Principal) bool {
	return authorization.CanUseAutomation(principal) && authorization.CanConnectFinancialAccounts(principal)
}

func validateBreakerReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) < 8 || utf8.RuneCountInString(reason) > 280 {
		return "", ErrBreakerInvalid
	}
	for _, value := range reason {
		if unicode.IsControl(value) {
			return "", ErrBreakerInvalid
		}
	}
	return reason, nil
}

func (service *BreakerService) CurrentAutomation(ctx context.Context, principal authorization.Principal, automationID string) (*CircuitBreaker, error) {
	if !breakerAllowed(principal) {
		return nil, ErrBreakerForbidden
	}
	owned, err := service.store.AutomationOwned(ctx, principal.UserID, automationID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, ErrBreakerNotFound
	}
	return service.store.OpenAutomationBreaker(ctx, principal.UserID, automationID)
}

func (service *BreakerService) EngageAutomation(ctx context.Context, principal authorization.Principal, automationID string, command BreakerCommand) (CircuitBreaker, error) {
	if !breakerAllowed(principal) {
		return CircuitBreaker{}, ErrBreakerForbidden
	}
	if !command.Confirm {
		return CircuitBreaker{}, ErrBreakerInvalid
	}
	reason, err := validateBreakerReason(command.Reason)
	if err != nil {
		return CircuitBreaker{}, err
	}
	owned, err := service.store.AutomationOwned(ctx, principal.UserID, automationID)
	if err != nil {
		return CircuitBreaker{}, err
	}
	if !owned {
		return CircuitBreaker{}, ErrBreakerNotFound
	}
	breaker, err := service.store.EngageAutomationBreaker(ctx, principal.UserID, automationID, reason, service.now())
	if err != nil {
		return CircuitBreaker{}, err
	}
	service.record(ctx, principal.UserID, "automation_circuit_breaker.engaged", breaker, reason)
	return breaker, nil
}

func (service *BreakerService) ReleaseAutomation(ctx context.Context, principal authorization.Principal, automationID string, command BreakerCommand) (CircuitBreaker, error) {
	if !breakerAllowed(principal) {
		return CircuitBreaker{}, ErrBreakerForbidden
	}
	if !command.Confirm {
		return CircuitBreaker{}, ErrBreakerInvalid
	}
	reason, err := validateBreakerReason(command.Reason)
	if err != nil {
		return CircuitBreaker{}, err
	}
	owned, err := service.store.AutomationOwned(ctx, principal.UserID, automationID)
	if err != nil {
		return CircuitBreaker{}, err
	}
	if !owned {
		return CircuitBreaker{}, ErrBreakerNotFound
	}
	breaker, err := service.store.ReleaseAutomationBreaker(ctx, principal.UserID, automationID, service.now())
	if err != nil {
		return CircuitBreaker{}, err
	}
	service.record(ctx, principal.UserID, "automation_circuit_breaker.released", breaker, reason)
	return breaker, nil
}

func (service *BreakerService) CurrentAccount(ctx context.Context, principal authorization.Principal, accountID string) (*CircuitBreaker, error) {
	if !breakerAllowed(principal) {
		return nil, ErrBreakerForbidden
	}
	owned, err := service.store.AccountOwned(ctx, principal.UserID, accountID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, ErrBreakerNotFound
	}
	return service.store.OpenAccountBreaker(ctx, principal.UserID, accountID)
}

func (service *BreakerService) EngageAccount(ctx context.Context, principal authorization.Principal, accountID string, command BreakerCommand) (CircuitBreaker, error) {
	if !breakerAllowed(principal) {
		return CircuitBreaker{}, ErrBreakerForbidden
	}
	if !command.Confirm {
		return CircuitBreaker{}, ErrBreakerInvalid
	}
	reason, err := validateBreakerReason(command.Reason)
	if err != nil {
		return CircuitBreaker{}, err
	}
	owned, err := service.store.AccountOwned(ctx, principal.UserID, accountID)
	if err != nil {
		return CircuitBreaker{}, err
	}
	if !owned {
		return CircuitBreaker{}, ErrBreakerNotFound
	}
	breaker, err := service.store.EngageAccountBreaker(ctx, principal.UserID, accountID, reason, service.now())
	if err != nil {
		return CircuitBreaker{}, err
	}
	service.record(ctx, principal.UserID, "account_circuit_breaker.engaged", breaker, reason)
	return breaker, nil
}

func (service *BreakerService) ReleaseAccount(ctx context.Context, principal authorization.Principal, accountID string, command BreakerCommand) (CircuitBreaker, error) {
	if !breakerAllowed(principal) {
		return CircuitBreaker{}, ErrBreakerForbidden
	}
	if !command.Confirm {
		return CircuitBreaker{}, ErrBreakerInvalid
	}
	reason, err := validateBreakerReason(command.Reason)
	if err != nil {
		return CircuitBreaker{}, err
	}
	owned, err := service.store.AccountOwned(ctx, principal.UserID, accountID)
	if err != nil {
		return CircuitBreaker{}, err
	}
	if !owned {
		return CircuitBreaker{}, ErrBreakerNotFound
	}
	breaker, err := service.store.ReleaseAccountBreaker(ctx, principal.UserID, accountID, service.now())
	if err != nil {
		return CircuitBreaker{}, err
	}
	service.record(ctx, principal.UserID, "account_circuit_breaker.released", breaker, reason)
	return breaker, nil
}

func (service *BreakerService) record(ctx context.Context, userID, action string, breaker CircuitBreaker, reason string) {
	if service.audit == nil {
		return
	}
	scopeID := ""
	if breaker.ScopeID != nil {
		scopeID = *breaker.ScopeID
	}
	metadata := map[string]any{
		"circuit_breaker_id":         breaker.ID,
		"reason":                     reason,
		"scope":                      breaker.Scope,
		"source":                     "UI",
		"live_execution_available":   false,
		"broker_execution_requested": false,
	}
	if breaker.Scope == ScopeAccount {
		metadata["financial_account_id"] = scopeID
	} else if breaker.Scope == ScopeAutomation {
		metadata["automation_id"] = scopeID
	}
	_ = service.audit.Record(ctx, &userID, action, metadata)
}
