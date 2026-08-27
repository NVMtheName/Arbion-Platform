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
	ErrBreakerForbidden     = errors.New("automation circuit breaker entitlement required")
	ErrBreakerAdminRequired = errors.New("superadmin circuit breaker authority required")
	ErrBreakerStepUp        = errors.New("fresh authenticator verification required")
	ErrBreakerInvalid       = errors.New("invalid automation circuit breaker command")
	ErrBreakerNotFound      = errors.New("automation circuit breaker resource not found")
	ErrBreakerConflict      = errors.New("automation circuit breaker state conflict")
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
	UserExists(context.Context, string) (bool, error)
	OpenUserBreaker(context.Context, string) (*CircuitBreaker, error)
	EngageUserBreaker(context.Context, string, string, time.Time) (CircuitBreaker, error)
	ReleaseUserBreaker(context.Context, string, time.Time) (CircuitBreaker, error)
	ActiveSuperadmin(context.Context, string) (bool, error)
	OpenGlobalBreaker(context.Context, string) (*CircuitBreaker, error)
	EngageGlobalBreaker(context.Context, string, string, time.Time) (CircuitBreaker, error)
	ReleaseGlobalBreaker(context.Context, string, time.Time) (CircuitBreaker, error)
}

type BreakerAuditor interface {
	Record(context.Context, *string, string, map[string]any) error
}

type BreakerService struct {
	store  BreakerStore
	audit  BreakerAuditor
	stepUp BreakerStepUpVerifier
	now    func() time.Time
}

type BreakerStepUpVerifier interface {
	VerifySafetyControlStepUp(context.Context, string, string) (string, time.Time, error)
}

type BreakerCommand struct {
	Reason  string `json:"reason"`
	Confirm bool   `json:"confirm"`
	MFACode string `json:"mfa_code,omitempty"`
}

func NewBreakerService(store BreakerStore, audit BreakerAuditor, stepUp ...BreakerStepUpVerifier) *BreakerService {
	service := &BreakerService{store: store, audit: audit, now: func() time.Time { return time.Now().UTC() }}
	if len(stepUp) > 0 {
		service.stepUp = stepUp[0]
	}
	return service
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

func (service *BreakerService) CurrentUser(ctx context.Context, principal authorization.Principal) (*CircuitBreaker, error) {
	if !breakerAllowed(principal) {
		return nil, ErrBreakerForbidden
	}
	exists, err := service.store.UserExists(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrBreakerNotFound
	}
	return service.store.OpenUserBreaker(ctx, principal.UserID)
}

func (service *BreakerService) EngageUser(ctx context.Context, principal authorization.Principal, command BreakerCommand) (CircuitBreaker, error) {
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
	exists, err := service.store.UserExists(ctx, principal.UserID)
	if err != nil {
		return CircuitBreaker{}, err
	}
	if !exists {
		return CircuitBreaker{}, ErrBreakerNotFound
	}
	breaker, err := service.store.EngageUserBreaker(ctx, principal.UserID, reason, service.now())
	if err != nil {
		return CircuitBreaker{}, err
	}
	service.record(ctx, principal.UserID, "user_circuit_breaker.engaged", breaker, reason)
	return breaker, nil
}

func (service *BreakerService) ReleaseUser(ctx context.Context, principal authorization.Principal, command BreakerCommand) (CircuitBreaker, error) {
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
	exists, err := service.store.UserExists(ctx, principal.UserID)
	if err != nil {
		return CircuitBreaker{}, err
	}
	if !exists {
		return CircuitBreaker{}, ErrBreakerNotFound
	}
	breaker, err := service.store.ReleaseUserBreaker(ctx, principal.UserID, service.now())
	if err != nil {
		return CircuitBreaker{}, err
	}
	service.record(ctx, principal.UserID, "user_circuit_breaker.released", breaker, reason)
	return breaker, nil
}

func (service *BreakerService) CurrentGlobal(ctx context.Context, principal authorization.Principal) (*CircuitBreaker, error) {
	if authorization.RequireSuperadmin(principal) != nil {
		return nil, ErrBreakerAdminRequired
	}
	active, err := service.store.ActiveSuperadmin(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, ErrBreakerAdminRequired
	}
	return service.store.OpenGlobalBreaker(ctx, principal.UserID)
}

func (service *BreakerService) EngageGlobal(ctx context.Context, principal authorization.Principal, command BreakerCommand) (CircuitBreaker, error) {
	if authorization.RequireSuperadmin(principal) != nil {
		return CircuitBreaker{}, ErrBreakerAdminRequired
	}
	if !command.Confirm {
		return CircuitBreaker{}, ErrBreakerInvalid
	}
	reason, err := validateBreakerReason(command.Reason)
	if err != nil {
		return CircuitBreaker{}, err
	}
	active, err := service.store.ActiveSuperadmin(ctx, principal.UserID)
	if err != nil {
		return CircuitBreaker{}, err
	}
	if !active {
		return CircuitBreaker{}, ErrBreakerAdminRequired
	}
	breaker, err := service.store.EngageGlobalBreaker(ctx, principal.UserID, reason, service.now())
	if err != nil {
		return CircuitBreaker{}, err
	}
	service.record(ctx, principal.UserID, "global_circuit_breaker.engaged", breaker, reason)
	return breaker, nil
}

func (service *BreakerService) ReleaseGlobal(ctx context.Context, principal authorization.Principal, command BreakerCommand) (CircuitBreaker, error) {
	if authorization.RequireSuperadmin(principal) != nil {
		return CircuitBreaker{}, ErrBreakerAdminRequired
	}
	if !command.Confirm {
		return CircuitBreaker{}, ErrBreakerInvalid
	}
	reason, err := validateBreakerReason(command.Reason)
	if err != nil {
		return CircuitBreaker{}, err
	}
	active, err := service.store.ActiveSuperadmin(ctx, principal.UserID)
	if err != nil {
		return CircuitBreaker{}, err
	}
	if !active {
		return CircuitBreaker{}, ErrBreakerAdminRequired
	}
	current, err := service.store.OpenGlobalBreaker(ctx, principal.UserID)
	if err != nil {
		return CircuitBreaker{}, err
	}
	if current == nil {
		return CircuitBreaker{}, ErrBreakerConflict
	}
	if service.stepUp == nil {
		return CircuitBreaker{}, ErrBreakerStepUp
	}
	if _, _, err = service.stepUp.VerifySafetyControlStepUp(ctx, principal.UserID, strings.TrimSpace(command.MFACode)); err != nil {
		return CircuitBreaker{}, ErrBreakerStepUp
	}
	breaker, err := service.store.ReleaseGlobalBreaker(ctx, principal.UserID, service.now())
	if err != nil {
		return CircuitBreaker{}, err
	}
	service.record(ctx, principal.UserID, "global_circuit_breaker.released", breaker, reason)
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
		"source":                     breaker.Source,
		"live_execution_available":   false,
		"broker_execution_requested": false,
	}
	if breaker.Scope == ScopeAccount {
		metadata["financial_account_id"] = scopeID
	} else if breaker.Scope == ScopeAutomation {
		metadata["automation_id"] = scopeID
	} else if breaker.Scope == ScopeUser {
		metadata["subject_user_id"] = scopeID
	}
	_ = service.audit.Record(ctx, &userID, action, metadata)
}
