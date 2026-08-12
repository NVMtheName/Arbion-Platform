package authorization

import (
	"context"
	"errors"
)

type Role string

const (
	RoleUser       Role = "user"
	RoleAdmin      Role = "admin"
	RoleSuperadmin Role = "superadmin"
)

type Entitlement string

const (
	EntitlementFree           Entitlement = "free"
	EntitlementPro            Entitlement = "pro"
	EntitlementPremium        Entitlement = "premium"
	EntitlementFounder        Entitlement = "founder"
	EntitlementInternalComped Entitlement = "internal_comped"
)

var ErrForbidden = errors.New("forbidden")

type Principal struct {
	UserID          string
	Role            Role
	Entitlement     Entitlement
	BillingRequired bool
}

func RequireAuthenticated(p Principal) error {
	if p.UserID == "" {
		return ErrForbidden
	}
	return nil
}
func RequireAdmin(p Principal) error {
	if p.Role != RoleAdmin && p.Role != RoleSuperadmin {
		return ErrForbidden
	}
	return nil
}
func RequireSuperadmin(p Principal) error {
	if p.Role != RoleSuperadmin {
		return ErrForbidden
	}
	return nil
}
func HasEntitlement(p Principal, wanted Entitlement) bool {
	return p.Entitlement == EntitlementFounder || p.Entitlement == wanted
}
func RequireEntitlement(p Principal, wanted Entitlement) error {
	if !HasEntitlement(p, wanted) {
		return ErrForbidden
	}
	return nil
}

// Product capabilities are intentionally independent from administrative roles.
func CanUseNeuralEngine(p Principal) bool {
	return p.Entitlement == EntitlementFounder || p.Entitlement == EntitlementPremium
}
func CanConnectFinancialAccounts(p Principal) bool {
	return p.Entitlement == EntitlementFounder || p.Entitlement == EntitlementPro || p.Entitlement == EntitlementPremium
}
func CanUseAdvancedAnalytics(p Principal) bool { return CanUseNeuralEngine(p) }
func CanUseAutomation(p Principal) bool        { return p.Entitlement == EntitlementFounder }

type User struct {
	ID              string      `json:"id"`
	Email           string      `json:"email"`
	DisplayName     string      `json:"display_name"`
	Role            Role        `json:"role"`
	Entitlement     Entitlement `json:"entitlement"`
	BillingRequired bool        `json:"billing_required"`
}
type Store interface {
	List(context.Context) ([]User, error)
	Get(context.Context, string) (User, error)
	SetRole(context.Context, string, Role) (Role, error)
	SetEntitlement(context.Context, string, Entitlement, bool, string) (Entitlement, error)
	BootstrapFounder(context.Context, string) (User, bool, error)
}
type Auditor interface {
	Record(context.Context, *string, string, map[string]any) error
}
type Service struct {
	store Store
	audit Auditor
}

func NewService(s Store, a Auditor) *Service { return &Service{store: s, audit: a} }
func (s *Service) List(ctx context.Context, p Principal) ([]User, error) {
	if RequireAdmin(p) != nil {
		return nil, ErrForbidden
	}
	s.record(ctx, p, "admin.users_viewed", "", nil)
	return s.store.List(ctx)
}
func (s *Service) Get(ctx context.Context, p Principal, id string) (User, error) {
	if RequireAdmin(p) != nil {
		return User{}, ErrForbidden
	}
	s.record(ctx, p, "admin.user_viewed", id, nil)
	return s.store.Get(ctx, id)
}
func (s *Service) SetRole(ctx context.Context, p Principal, id string, r Role) (User, error) {
	if RequireSuperadmin(p) != nil {
		return User{}, ErrForbidden
	}
	if r != RoleUser && r != RoleAdmin && r != RoleSuperadmin {
		return User{}, ErrForbidden
	}
	target, e := s.store.Get(ctx, id)
	if e != nil {
		return User{}, e
	}
	if target.Entitlement == EntitlementFounder && r != RoleSuperadmin {
		return User{}, ErrForbidden
	}
	old, e := s.store.SetRole(ctx, id, r)
	if e != nil {
		return User{}, e
	}
	s.record(ctx, p, "authorization.role_changed", id, map[string]any{"previous": old, "new": r})
	return s.store.Get(ctx, id)
}
func (s *Service) SetEntitlement(ctx context.Context, p Principal, id string, e Entitlement, billing bool) (User, error) {
	if RequireSuperadmin(p) != nil {
		return User{}, ErrForbidden
	}
	target, err := s.store.Get(ctx, id)
	if err != nil {
		return User{}, err
	}
	if target.Entitlement == EntitlementFounder && (e != EntitlementFounder || billing) {
		return User{}, ErrForbidden
	}
	if e == EntitlementFounder && billing {
		return User{}, ErrForbidden
	}
	old, err := s.store.SetEntitlement(ctx, id, e, billing, "admin")
	if err != nil {
		return User{}, err
	}
	s.record(ctx, p, "entitlement.changed", id, map[string]any{"previous": old, "new": e, "billing_required": billing})
	return s.store.Get(ctx, id)
}
func (s *Service) record(ctx context.Context, p Principal, action, target string, m map[string]any) {
	if m == nil {
		m = map[string]any{}
	}
	m["target_user_id"] = target
	id := p.UserID
	_ = s.audit.Record(ctx, &id, action, m)
}
