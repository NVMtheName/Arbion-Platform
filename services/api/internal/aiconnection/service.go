package aiconnection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/neural"
)

var (
	ErrForbidden = errors.New("neural engine entitlement required")
	ErrNotFound  = errors.New("connection not found")
	ErrInvalid   = errors.New("invalid connection input")
	ErrConflict  = errors.New("connection has durable dependencies")
	ErrDisabled  = errors.New("connection is disabled")
	ErrInactive  = errors.New("connection is not active")
	ErrProvider  = errors.New("neural provider failure")
	ErrRateLimit = errors.New("neural insight rate limit reached")
)

const (
	MaxCredentialBytes = 4096
	MaxPromptBytes     = 2000
	InsightCreditLimit = 12
	InsightWindow      = time.Hour
)

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
	SetVerification(context.Context, string, string, string, bool) (Connection, error)
	GetPreference(context.Context, string) (*Preference, error)
	SetPreference(context.Context, string, string, string) (Preference, error)
	Delete(context.Context, string, string) error
	HasDependencies(context.Context, string, string) (bool, error)
}
type Auditor interface {
	Record(context.Context, *string, string, map[string]any) error
}
type Limiter interface {
	AllowWeighted(context.Context, string, int, int, time.Duration) (bool, error)
}
type Service struct {
	store    Store
	vault    credential.Vault
	audit    Auditor
	registry Registry
	neural   neural.Client
	limiter  Limiter
}

func NewService(s Store, v credential.Vault, a Auditor, r Registry, client neural.Client, limiter Limiter) *Service {
	return &Service{store: s, vault: v, audit: a, registry: r, neural: client, limiter: limiter}
}

type Preference struct {
	ConnectionID string    `json:"connection_id"`
	ModelID      string    `json:"model_id"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (s *Service) Providers() []Provider { return s.registry.List() }
func (s *Service) List(ctx context.Context, p authorization.Principal) ([]Connection, error) {
	if authorization.RequireAuthenticated(p) != nil {
		return nil, ErrForbidden
	}
	return s.store.List(ctx, p.UserID)
}
func (s *Service) Verify(ctx context.Context, p authorization.Principal, id string) (Connection, error) {
	c, err := s.ownedMutation(ctx, p, id, "ai_connection.verification_rejected")
	if err != nil {
		return Connection{}, err
	}
	if c.Status == "disabled" {
		return Connection{}, ErrDisabled
	}
	secret, err := s.vault.Retrieve(ctx, credential.Locator{ConnectionID: id, UserID: p.UserID, Class: credential.AI})
	if err != nil {
		return Connection{}, err
	}
	defer clear(secret)
	if s.neural == nil {
		return Connection{}, ErrProvider
	}
	err = s.neural.Verify(ctx, c.Provider, secret)
	if err != nil {
		c, _ = s.store.SetVerification(ctx, p.UserID, id, "error", false)
		code := neural.Code(err)
		action := "ai_connection.verification_failed"
		if code == neural.ProviderUnavailable || code == neural.Timeout {
			action = "ai_connection.provider_unavailable"
		}
		s.record(ctx, p.UserID, action, c, map[string]any{"outcome": code})
		return c, &neural.ProviderError{Code: code}
	}
	c, err = s.store.SetVerification(ctx, p.UserID, id, "active", true)
	if err == nil {
		s.record(ctx, p.UserID, "ai_connection.verification_succeeded", c, map[string]any{"outcome": "verified"})
	}
	return c, err
}
func (s *Service) Models(ctx context.Context, p authorization.Principal, id string) ([]neural.Model, error) {
	c, err := s.ownedMutation(ctx, p, id, "ai_connection.models_rejected")
	if err != nil {
		return nil, err
	}
	if c.Status == "disabled" {
		return nil, ErrDisabled
	}
	if c.Status != "active" {
		return nil, ErrInactive
	}
	secret, err := s.vault.Retrieve(ctx, credential.Locator{ConnectionID: id, UserID: p.UserID, Class: credential.AI})
	if err != nil {
		return nil, err
	}
	defer clear(secret)
	return s.neural.Models(ctx, c.Provider, secret)
}
func (s *Service) Analyze(ctx context.Context, p authorization.Principal, prompt, profile string) (neural.Insight, error) {
	prompt = strings.TrimSpace(prompt)
	if !s.allowed(ctx, p, "neural_insight.rejected", "", "") {
		return neural.Insight{}, ErrForbidden
	}
	if prompt == "" || len([]byte(prompt)) > MaxPromptBytes {
		return neural.Insight{}, ErrInvalid
	}
	route, err := resolveInsightRoute(profile)
	if err != nil {
		return neural.Insight{}, err
	}
	pref, err := s.store.GetPreference(ctx, p.UserID)
	if err != nil {
		return neural.Insight{}, err
	}
	if pref == nil {
		return neural.Insight{}, ErrInactive
	}
	c, err := s.store.Get(ctx, p.UserID, pref.ConnectionID)
	if err != nil {
		return neural.Insight{}, err
	}
	if c.Status != "active" {
		return neural.Insight{}, ErrInactive
	}
	if c.Provider != route.Provider {
		s.record(ctx, p.UserID, "neural_insight.failed", c, routeAudit(route, "UNSUPPORTED"))
		return neural.Insight{}, &neural.ProviderError{Code: neural.Unsupported}
	}
	if s.neural == nil || s.limiter == nil {
		return neural.Insight{}, ErrProvider
	}
	allowed, err := s.limiter.AllowWeighted(ctx, "neural-insight:"+p.UserID, route.CreditUnits, InsightCreditLimit, InsightWindow)
	if err != nil {
		s.record(ctx, p.UserID, "neural_insight.failed", c, routeAudit(route, "RATE_LIMITER_UNAVAILABLE"))
		return neural.Insight{}, ErrProvider
	}
	if !allowed {
		s.record(ctx, p.UserID, "neural_insight.rate_limited", c, routeAudit(route, "RATE_LIMITED"))
		return neural.Insight{}, ErrRateLimit
	}
	secret, err := s.vault.Retrieve(ctx, credential.Locator{ConnectionID: c.ID, UserID: p.UserID, Class: credential.AI})
	if err != nil {
		return neural.Insight{}, err
	}
	defer clear(secret)
	digest := sha256.Sum256([]byte("arbion-neural:" + p.UserID))
	started := time.Now()
	result, err := s.neural.Analyze(ctx, c.Provider, string(route.Profile), secret, prompt, hex.EncodeToString(digest[:]))
	if err != nil {
		code := neural.Code(err)
		failed := routeAudit(route, code)
		failed["input_bytes"] = len([]byte(prompt))
		failed["latency_ms"] = time.Since(started).Milliseconds()
		s.record(ctx, p.UserID, "neural_insight.failed", c, failed)
		return neural.Insight{}, &neural.ProviderError{Code: code}
	}
	if result.Metadata.Provider != route.Provider || result.Metadata.Model != route.ModelID || result.Metadata.Profile != string(route.Profile) {
		failed := routeAudit(route, neural.InternalError)
		failed["input_bytes"] = len([]byte(prompt))
		failed["latency_ms"] = time.Since(started).Milliseconds()
		s.record(ctx, p.UserID, "neural_insight.failed", c, failed)
		return neural.Insight{}, &neural.ProviderError{Code: neural.InternalError}
	}
	extra := routeAudit(route, "COMPLETED")
	extra["input_bytes"] = len([]byte(prompt))
	extra["latency_ms"] = time.Since(started).Milliseconds()
	if result.Metadata.InputUsage != nil {
		extra["input_usage"] = *result.Metadata.InputUsage
	}
	if result.Metadata.OutputUsage != nil {
		extra["output_usage"] = *result.Metadata.OutputUsage
	}
	if result.Metadata.RequestID != "" {
		extra["provider_request_id"] = result.Metadata.RequestID
	}
	s.record(ctx, p.UserID, "neural_insight.completed", c, extra)
	return result, nil
}

func routeAudit(route InsightRoute, outcome any) map[string]any {
	return map[string]any{
		"profile": route.Profile, "model_id": route.ModelID, "credit_units": route.CreditUnits, "outcome": outcome,
	}
}
func (s *Service) Preference(ctx context.Context, p authorization.Principal) (*Preference, error) {
	if !s.allowed(ctx, p, "neural_preference.read_rejected", "", "") {
		return nil, ErrForbidden
	}
	return s.store.GetPreference(ctx, p.UserID)
}
func (s *Service) SetPreference(ctx context.Context, p authorization.Principal, connectionID, modelID string) (Preference, error) {
	if !s.allowed(ctx, p, "neural_preference.change_rejected", connectionID, "") {
		return Preference{}, ErrForbidden
	}
	c, err := s.store.Get(ctx, p.UserID, connectionID)
	if err != nil {
		return Preference{}, err
	}
	if c.Status != "active" {
		return Preference{}, ErrInactive
	}
	models, err := s.Models(ctx, p, connectionID)
	if err != nil {
		return Preference{}, err
	}
	found := false
	for _, m := range models {
		if m.ID == modelID {
			found = true
			break
		}
	}
	if !found {
		return Preference{}, ErrInvalid
	}
	previous, _ := s.store.GetPreference(ctx, p.UserID)
	pref, err := s.store.SetPreference(ctx, p.UserID, connectionID, modelID)
	if err != nil {
		return Preference{}, err
	}
	if previous == nil || previous.ConnectionID != connectionID {
		s.record(ctx, p.UserID, "neural_preference.provider_changed", c, map[string]any{"model_id": modelID})
	}
	if previous == nil || previous.ModelID != modelID {
		s.record(ctx, p.UserID, "neural_preference.model_changed", c, map[string]any{"model_id": modelID})
	}
	return pref, nil
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
