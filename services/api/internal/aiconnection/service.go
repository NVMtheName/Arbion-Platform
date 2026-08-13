package aiconnection

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/credential"
)

var (
	ErrForbidden = errors.New("neural engine entitlement required")
	ErrNotFound  = errors.New("connection not found")
	ErrInvalid   = errors.New("invalid connection input")
	ErrConflict  = errors.New("connection has durable dependencies")
)

const MaxCredentialBytes = 4096

type Connection struct {
	ID             string     `json:"id"`
	Provider       string     `json:"provider"`
	ProviderLabel  string     `json:"provider_label"`
	DisplayName    string     `json:"display_name"`
	Status         string     `json:"status"`
	Enabled        bool       `json:"enabled"`
	CredentialHint string     `json:"credential_hint"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastVerifiedAt *time.Time `json:"last_verified_at,omitempty"`
}

type Store interface {
	List(context.Context, string) ([]Connection, error)
	Create(context.Context, string, string, string, string) (Connection, error)
	Get(context.Context, string, string) (Connection, error)
	Rename(context.Context, string, string, string) (Connection, error)
	SetStatus(context.Context, string, string, string) (Connection, error)
	SetCredentialPending(context.Context, string, string, string) (Connection, error)
	Delete(context.Context, string, string) error
	HasDependencies(context.Context, string, string) (bool, error)
}
type Auditor interface {
	Record(context.Context, *string, string, map[string]any) error
}
type Service struct {
	store    Store
	vault    credential.Vault
	audit    Auditor
	registry Registry
}

func NewService(s Store, v credential.Vault, a Auditor, r Registry) *Service {
	return &Service{s, v, a, r}
}
func (s *Service) Providers() []Provider { return s.registry.List() }
func (s *Service) List(ctx context.Context, p authorization.Principal) ([]Connection, error) {
	if authorization.RequireAuthenticated(p) != nil {
		return nil, ErrForbidden
	}
	return s.store.List(ctx, p.UserID)
}
func (s *Service) Create(ctx context.Context, p authorization.Principal, provider, name string, secret []byte) (Connection, error) {
	if !s.allowed(ctx, p, "ai_connection.create_rejected", "", provider) {
		return Connection{}, ErrForbidden
	}
	provider = strings.TrimSpace(provider)
	name = strings.TrimSpace(name)
	if _, ok := s.registry.Get(provider); !ok || name == "" || len(name) > 100 || !validSecret(secret) {
		return Connection{}, ErrInvalid
	}
	hint := maskedHint(secret)
	c, err := s.store.Create(ctx, p.UserID, provider, name, hint)
	if err != nil {
		return Connection{}, err
	}
	loc := credential.Locator{ConnectionID: c.ID, UserID: p.UserID, Class: credential.AI}
	if err = s.vault.Store(ctx, loc, secret); err != nil {
		_ = s.store.Delete(ctx, p.UserID, c.ID)
		return Connection{}, err
	}
	s.record(ctx, p.UserID, "ai_connection.created", c, nil)
	return c, nil
}
func (s *Service) Replace(ctx context.Context, p authorization.Principal, id string, secret []byte) (Connection, error) {
	c, err := s.ownedMutation(ctx, p, id, "ai_connection.credential_replace_rejected")
	if err != nil {
		return Connection{}, err
	}
	if !validSecret(secret) {
		return Connection{}, ErrInvalid
	}
	if err = s.vault.Replace(ctx, credential.Locator{ConnectionID: id, UserID: p.UserID, Class: credential.AI}, secret); err != nil {
		return Connection{}, err
	}
	previous := c.Status
	c, err = s.store.SetCredentialPending(ctx, p.UserID, id, maskedHint(secret))
	if err != nil {
		return Connection{}, err
	}
	s.record(ctx, p.UserID, "ai_connection.credential_replaced", c, map[string]any{"previous_status": previous, "new_status": "pending"})
	return c, nil
}
func (s *Service) Rename(ctx context.Context, p authorization.Principal, id, name string) (Connection, error) {
	c, err := s.ownedMutation(ctx, p, id, "ai_connection.rename_rejected")
	if err != nil {
		return Connection{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return Connection{}, ErrInvalid
	}
	c, err = s.store.Rename(ctx, p.UserID, id, name)
	if err == nil {
		s.record(ctx, p.UserID, "ai_connection.display_name_changed", c, nil)
	}
	return c, err
}
func (s *Service) SetEnabled(ctx context.Context, p authorization.Principal, id string, enabled bool) (Connection, error) {
	c, err := s.ownedMutation(ctx, p, id, "ai_connection.state_change_rejected")
	if err != nil {
		return Connection{}, err
	}
	previous := c.Status
	next := "disabled"
	action := "ai_connection.disabled"
	if enabled {
		next = "pending"
		action = "ai_connection.enabled"
	}
	c, err = s.store.SetStatus(ctx, p.UserID, id, next)
	if err == nil {
		s.record(ctx, p.UserID, action, c, map[string]any{"previous_status": previous, "new_status": next})
	}
	return c, err
}
func (s *Service) Delete(ctx context.Context, p authorization.Principal, id string) error {
	c, err := s.ownedMutation(ctx, p, id, "ai_connection.delete_rejected")
	if err != nil {
		return err
	}
	dependent, err := s.store.HasDependencies(ctx, p.UserID, id)
	if err != nil {
		return err
	}
	if dependent {
		return ErrConflict
	}
	loc := credential.Locator{ConnectionID: id, UserID: p.UserID, Class: credential.AI}
	if err = s.vault.Delete(ctx, loc); err != nil {
		return err
	}
	if err = s.store.Delete(ctx, p.UserID, id); err != nil {
		return err
	}
	s.record(ctx, p.UserID, "ai_connection.deleted", c, nil)
	return nil
}
func (s *Service) ownedMutation(ctx context.Context, p authorization.Principal, id, action string) (Connection, error) {
	if !s.allowed(ctx, p, action, id, "") {
		return Connection{}, ErrForbidden
	}
	return s.store.Get(ctx, p.UserID, id)
}
func (s *Service) allowed(ctx context.Context, p authorization.Principal, action, id, provider string) bool {
	if authorization.CanUseNeuralEngine(p) {
		return true
	}
	s.record(ctx, p.UserID, action, Connection{ID: id, Provider: provider}, map[string]any{"reason": "entitlement_required"})
	return false
}
func (s *Service) record(ctx context.Context, user, action string, c Connection, extra map[string]any) {
	if s.audit == nil {
		return
	}
	m := map[string]any{"connection_id": c.ID, "provider": c.Provider}
	for k, v := range extra {
		m[k] = v
	}
	_ = s.audit.Record(ctx, &user, action, m)
}
func validSecret(v []byte) bool {
	return len(v) >= 8 && len(v) <= MaxCredentialBytes && strings.TrimSpace(string(v)) != ""
}
func maskedHint(v []byte) string {
	r := []rune(string(v))
	n := 4
	if len(r) < n {
		n = len(r)
	}
	return "••••••••" + string(r[len(r)-n:])
}
