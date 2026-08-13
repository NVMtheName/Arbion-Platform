package financialconnection

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/financial/oauthstate"
	"time"
)

var (
	ErrForbidden = errors.New("financial account entitlement required")
	ErrNotFound  = errors.New("financial resource not found")
	ErrDisabled  = errors.New("connection disabled")
)

type Connection struct {
	ID                string     `json:"id"`
	Provider          string     `json:"provider"`
	DisplayName       string     `json:"display_name"`
	Status            string     `json:"status"`
	TokenExpiresAt    *time.Time `json:"token_expires_at,omitempty"`
	LastSyncedAt      *time.Time `json:"last_synced_at,omitempty"`
	CredentialStorage string     `json:"credential_storage"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}
type Store interface {
	ListConnections(context.Context, string) ([]Connection, error)
	UpsertConnection(context.Context, string, string, *time.Time) (Connection, error)
	GetConnection(context.Context, string, string) (Connection, error)
	SetStatus(context.Context, string, string, string, *time.Time) (Connection, error)
	SyncAccounts(context.Context, string, string, []financial.FinancialAccount) error
	ListAccounts(context.Context, string) ([]financial.FinancialAccount, error)
	GetAccount(context.Context, string, string) (financial.FinancialAccount, error)
	Retire(context.Context, string, string) error
	WithLock(context.Context, string, func() error) error
}
type Authorizer interface {
	financial.BrokerProvider
	AuthorizationURL(string) (string, error)
	Exchange(context.Context, string) (financial.Credentials, error)
}
type Auditor interface {
	Record(context.Context, *string, string, map[string]any) error
}
type Service struct {
	store    Store
	vault    credential.Vault
	states   *oauthstate.Manager
	provider Authorizer
	audit    Auditor
}

func NewService(s Store, v credential.Vault, states *oauthstate.Manager, p Authorizer, a Auditor) *Service {
	return &Service{s, v, states, p, a}
}
func allowed(p authorization.Principal) bool { return authorization.CanConnectFinancialAccounts(p) }
func (s *Service) record(ctx context.Context, user, action string, meta map[string]any) {
	if s.audit != nil {
		s.audit.Record(ctx, &user, action, meta)
	}
}
func (s *Service) ListConnections(ctx context.Context, p authorization.Principal) ([]Connection, error) {
	if !allowed(p) {
		return nil, ErrForbidden
	}
	return s.store.ListConnections(ctx, p.UserID)
}
func (s *Service) StartAuthorization(ctx context.Context, p authorization.Principal) (string, error) {
	if !allowed(p) {
		return "", ErrForbidden
	}
	state, e := s.states.Start(ctx, p.UserID)
	if e != nil {
		return "", e
	}
	u, e := s.provider.AuthorizationURL(state)
	if e == nil {
		s.record(ctx, p.UserID, "financial.authorization_started", map[string]any{"provider": "schwab"})
	}
	return u, e
}
func (s *Service) CompleteAuthorization(ctx context.Context, state, code, providerError string) (string, error) {
	r, e := s.states.Take(ctx, state)
	if e != nil {
		return "", e
	}
	if providerError != "" || code == "" {
		s.record(ctx, r.UserID, "financial.authorization_failed", map[string]any{"provider": "schwab", "outcome": "cancelled"})
		return r.UserID, &financial.ProviderError{Code: financial.AuthorizationFailed}
	}
	cr, e := s.provider.Exchange(ctx, code)
	if e != nil {
		s.record(ctx, r.UserID, "financial.authorization_failed", map[string]any{"provider": "schwab"})
		return r.UserID, e
	}
	c, e := s.store.UpsertConnection(ctx, r.UserID, "schwab", &cr.AccessExpiresAt)
	if e != nil {
		return r.UserID, e
	}
	raw, e := cr.Bytes()
	if e != nil {
		return r.UserID, e
	}
	loc := credential.Locator{ConnectionID: c.ID, UserID: r.UserID, Class: credential.Financial}
	if e = s.vault.Store(ctx, loc, raw); e != nil {
		if e = s.vault.Replace(ctx, loc, raw); e != nil {
			return r.UserID, e
		}
	}
	if e = s.sync(ctx, r.UserID, c.ID); e != nil {
		s.store.SetStatus(ctx, r.UserID, c.ID, "error", nil)
		return r.UserID, e
	}
	s.record(ctx, r.UserID, "financial.authorization_completed", map[string]any{"provider": "schwab", "connection_id": c.ID})
	return r.UserID, nil
}
func (s *Service) credentials(ctx context.Context, user, id string, allowDisabled ...bool) (Connection, financial.Credentials, error) {
	c, e := s.store.GetConnection(ctx, user, id)
	if e != nil {
		return c, financial.Credentials{}, e
	}
	if c.Status == "disabled" && (len(allowDisabled) == 0 || !allowDisabled[0]) {
		return c, financial.Credentials{}, ErrDisabled
	}
	var cr financial.Credentials
	e = s.store.WithLock(ctx, id, func() error {
		raw, e := s.vault.Retrieve(ctx, credential.Locator{ConnectionID: id, UserID: user, Class: credential.Financial})
		if e != nil {
			return e
		}
		defer clear(raw)
		if e = json.Unmarshal(raw, &cr); e != nil {
			return e
		}
		if time.Until(cr.AccessExpiresAt) < 2*time.Minute {
			if e = s.provider.RefreshAuthorization(ctx, &cr); e != nil {
				s.store.SetStatus(ctx, user, id, "expired", nil)
				s.record(ctx, user, "financial.refresh_failed", map[string]any{"connection_id": id})
				return e
			}
			fresh, _ := cr.Bytes()
			if e = s.vault.Replace(ctx, credential.Locator{ConnectionID: id, UserID: user, Class: credential.Financial}, fresh); e != nil {
				return e
			}
			s.store.SetStatus(ctx, user, id, "active", &cr.AccessExpiresAt)
			s.record(ctx, user, "financial.connection_refreshed", map[string]any{"connection_id": id})
		}
		return nil
	})
	return c, cr, e
}
func (s *Service) sync(ctx context.Context, user, id string) error {
	_, cr, e := s.credentials(ctx, user, id)
	if e != nil {
		return e
	}
	accounts, e := s.provider.ListAccounts(ctx, &cr)
	if e != nil {
		return e
	}
	for i := range accounts {
		detail, de := s.provider.GetAccount(ctx, &cr, accounts[i].ProviderAccountID)
		if de == nil {
			detail.MaskedIdentifier = accounts[i].MaskedIdentifier
			detail.DisplayName = detail.DisplayName + " " + detail.MaskedIdentifier
			accounts[i] = detail
		}
	}
	if e = s.store.SyncAccounts(ctx, user, id, accounts); e == nil {
		for range accounts {
			s.record(ctx, user, "financial.account_discovered", map[string]any{"connection_id": id})
		}
		s.store.SetStatus(ctx, user, id, "active", &cr.AccessExpiresAt)
		s.record(ctx, user, "financial.connection_synced", map[string]any{"connection_id": id, "account_count": len(accounts)})
	}
	return e
}
func (s *Service) Sync(ctx context.Context, p authorization.Principal, id string) error {
	if !allowed(p) {
		return ErrForbidden
	}
	return s.sync(ctx, p.UserID, id)
}
func (s *Service) SetEnabled(ctx context.Context, p authorization.Principal, id string, enabled bool) (Connection, error) {
	if !allowed(p) {
		return Connection{}, ErrForbidden
	}
	status := "disabled"
	action := "financial.connection_disabled"
	if enabled {
		status = "active"
		action = "financial.connection_enabled"
		_, cr, e := s.credentials(ctx, p.UserID, id, true)
		if e != nil {
			return Connection{}, e
		}
		if e = s.provider.VerifyConnection(ctx, &cr); e != nil {
			return Connection{}, e
		}
	}
	c, e := s.store.SetStatus(ctx, p.UserID, id, status, nil)
	if e == nil {
		s.record(ctx, p.UserID, action, map[string]any{"connection_id": id})
	}
	return c, e
}
func (s *Service) Disconnect(ctx context.Context, p authorization.Principal, id string) error {
	if !allowed(p) {
		return ErrForbidden
	}
	_, cr, e := s.credentials(ctx, p.UserID, id, true)
	if e != nil {
		return e
	}
	_ = s.provider.Disconnect(ctx, &cr)
	if e = s.vault.Delete(ctx, credential.Locator{ConnectionID: id, UserID: p.UserID, Class: credential.Financial}); e != nil {
		return e
	}
	if e = s.store.Retire(ctx, p.UserID, id); e == nil {
		s.record(ctx, p.UserID, "financial.connection_disconnected", map[string]any{"connection_id": id})
	}
	return e
}
func (s *Service) ListAccounts(ctx context.Context, p authorization.Principal) ([]financial.FinancialAccount, error) {
	if !allowed(p) {
		return nil, ErrForbidden
	}
	return s.store.ListAccounts(ctx, p.UserID)
}
func (s *Service) GetAccount(ctx context.Context, p authorization.Principal, id string) (financial.FinancialAccount, error) {
	if !allowed(p) {
		return financial.FinancialAccount{}, ErrForbidden
	}
	return s.store.GetAccount(ctx, p.UserID, id)
}
func (s *Service) GetBalances(ctx context.Context, p authorization.Principal, id string) (financial.Balances, error) {
	a, e := s.GetAccount(ctx, p, id)
	if e != nil {
		return financial.Balances{}, e
	}
	_, cr, e := s.credentials(ctx, p.UserID, a.ProviderConnectionID)
	if e != nil {
		return financial.Balances{}, e
	}
	return s.provider.GetBalances(ctx, &cr, a.ProviderAccountID)
}
func (s *Service) GetPositions(ctx context.Context, p authorization.Principal, id string) ([]financial.Position, error) {
	a, e := s.GetAccount(ctx, p, id)
	if e != nil {
		return nil, e
	}
	_, cr, e := s.credentials(ctx, p.UserID, a.ProviderConnectionID)
	if e != nil {
		return nil, e
	}
	items, e := s.provider.GetPositions(ctx, &cr, a.ProviderAccountID)
	for i := range items {
		items[i].AccountID = a.ID
	}
	return items, e
}
