package financialconnection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/financial/oauthstate"
)

var (
	ErrForbidden           = errors.New("financial account entitlement required")
	ErrNotFound            = errors.New("financial resource not found")
	ErrDisabled            = errors.New("connection disabled")
	ErrInvalidInput        = errors.New("financial credential input is invalid")
	ErrInvalidOrderPreview = errors.New("order preview input is invalid")
	ErrConnectionInUse     = errors.New("financial connection is used by active automation")
)

const schwabAuthorizationLifetime = 7 * 24 * time.Hour

var (
	previewSymbolPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,15}$`)
	previewSizePattern   = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$`)
)

type Connection struct {
	ID                     string     `json:"id"`
	Provider               string     `json:"provider"`
	DisplayName            string     `json:"display_name"`
	Status                 string     `json:"status"`
	TokenExpiresAt         *time.Time `json:"token_expires_at,omitempty"`
	AuthorizationExpiresAt *time.Time `json:"authorization_expires_at,omitempty"`
	LastSyncedAt           *time.Time `json:"last_synced_at,omitempty"`
	CredentialStorage      string     `json:"credential_storage"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}
type Store interface {
	ListConnections(context.Context, string) ([]Connection, error)
	UpsertConnection(context.Context, string, string, string, *time.Time, *time.Time) (Connection, error)
	UpsertConnectionForAccount(context.Context, string, string, string, string, *time.Time, *time.Time) (Connection, error)
	GetConnection(context.Context, string, string) (Connection, error)
	SetStatus(context.Context, string, string, string, *time.Time) (Connection, error)
	ConnectionInUse(context.Context, string, string) (bool, error)
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
type NamedProvider struct {
	ID       string
	Provider financial.BrokerProvider
}
type Auditor interface {
	Record(context.Context, *string, string, map[string]any) error
}
type Service struct {
	store       Store
	vault       credential.Vault
	states      *oauthstate.Manager
	providers   map[string]financial.BrokerProvider
	authorizers map[string]Authorizer
	audit       Auditor
}

func NewService(s Store, v credential.Vault, states *oauthstate.Manager, schwab Authorizer, a Auditor, additional ...NamedProvider) *Service {
	service := &Service{store: s, vault: v, states: states, providers: map[string]financial.BrokerProvider{}, authorizers: map[string]Authorizer{}, audit: a}
	if schwab != nil {
		service.providers["schwab"] = schwab
		service.authorizers["schwab"] = schwab
	}
	for _, provider := range additional {
		id := strings.ToLower(strings.TrimSpace(provider.ID))
		if id != "" && provider.Provider != nil {
			service.providers[id] = provider.Provider
		}
	}
	return service
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
	provider, ok := s.authorizers["schwab"]
	if !ok {
		return "", &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	u, e := provider.AuthorizationURL(state)
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
	provider, ok := s.authorizers["schwab"]
	if !ok {
		return r.UserID, &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	cr, e := provider.Exchange(ctx, code)
	if e != nil {
		s.record(ctx, r.UserID, "financial.authorization_failed", map[string]any{"provider": "schwab"})
		return r.UserID, e
	}
	authorizationExpiresAt := time.Now().UTC().Add(schwabAuthorizationLifetime)
	c, e := s.store.UpsertConnection(ctx, r.UserID, "schwab", "Charles Schwab", credentialExpiry(cr), &authorizationExpiresAt)
	if e != nil {
		return r.UserID, e
	}
	raw, e := cr.Bytes()
	if e != nil {
		return r.UserID, e
	}
	defer clear(raw)
	loc := credential.Locator{ConnectionID: c.ID, UserID: r.UserID, Class: credential.Financial}
	if e = s.store.WithLock(ctx, c.ID, func() error {
		if storeErr := s.vault.Store(ctx, loc, raw); storeErr != nil {
			return s.vault.Replace(ctx, loc, raw)
		}
		return nil
	}); e != nil {
		return r.UserID, e
	}
	if e = s.sync(ctx, r.UserID, c.ID); e != nil {
		s.store.SetStatus(ctx, r.UserID, c.ID, "error", nil)
		return r.UserID, e
	}
	s.record(ctx, r.UserID, "financial.authorization_completed", map[string]any{"provider": "schwab", "connection_id": c.ID, "authorization_expires_at": authorizationExpiresAt})
	return r.UserID, nil
}
func (s *Service) ConnectAPIKey(ctx context.Context, p authorization.Principal, providerID, keyName, privateKey string) (Connection, error) {
	if !allowed(p) {
		return Connection{}, ErrForbidden
	}
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	keyName = strings.TrimSpace(keyName)
	privateKey = strings.TrimSpace(privateKey)
	if providerID != "coinbase" || keyName == "" || len(keyName) > 256 || privateKey == "" || len(privateKey) > 4096 || strings.ContainsRune(keyName, '\x00') || strings.ContainsRune(privateKey, '\x00') {
		return Connection{}, ErrInvalidInput
	}
	provider, ok := s.providers[providerID]
	if !ok {
		return Connection{}, &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	credentials := financial.Credentials{APIKeyName: keyName, APIPrivateKey: privateKey}
	if err := provider.VerifyConnection(ctx, &credentials); err != nil {
		metadata := map[string]any{"provider": providerID}
		var providerError *financial.ProviderError
		if errors.As(err, &providerError) {
			metadata["code"] = providerError.Code
		}
		s.record(ctx, p.UserID, "financial.authorization_failed", metadata)
		return Connection{}, err
	}
	if strings.TrimSpace(credentials.PortfolioID) == "" {
		return Connection{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	providerAccountID := "portfolio:" + credentials.PortfolioID
	connection, err := s.store.UpsertConnectionForAccount(ctx, p.UserID, providerID, apiKeyConnectionName(providerID, providerAccountID), providerAccountID, nil, nil)
	if err != nil {
		return Connection{}, err
	}
	raw, err := credentials.Bytes()
	if err != nil {
		return Connection{}, err
	}
	defer clear(raw)
	locator := credential.Locator{ConnectionID: connection.ID, UserID: p.UserID, Class: credential.Financial}
	if err = s.store.WithLock(ctx, connection.ID, func() error {
		if storeErr := s.vault.Store(ctx, locator, raw); storeErr != nil {
			return s.vault.Replace(ctx, locator, raw)
		}
		return nil
	}); err != nil {
		_, _ = s.store.SetStatus(ctx, p.UserID, connection.ID, "error", nil)
		return Connection{}, err
	}
	if err = s.sync(ctx, p.UserID, connection.ID); err != nil {
		_, _ = s.store.SetStatus(ctx, p.UserID, connection.ID, "error", nil)
		return Connection{}, err
	}
	connection, err = s.store.GetConnection(ctx, p.UserID, connection.ID)
	if err == nil {
		s.record(ctx, p.UserID, "financial.authorization_completed", map[string]any{"provider": providerID, "connection_id": connection.ID})
	}
	return connection, err
}

func apiKeyConnectionName(provider, providerAccountID string) string {
	digest := sha256.Sum256([]byte(provider + ":" + providerAccountID))
	return "Coinbase portfolio " + hex.EncodeToString(digest[:4])
}
func credentialExpiry(credentials financial.Credentials) *time.Time {
	if credentials.AccessExpiresAt.IsZero() {
		return nil
	}
	return &credentials.AccessExpiresAt
}
func (s *Service) provider(id string) (financial.BrokerProvider, error) {
	provider, ok := s.providers[strings.ToLower(strings.TrimSpace(id))]
	if !ok {
		return nil, &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	return provider, nil
}
func (s *Service) credentials(ctx context.Context, user, id string, allowDisabled ...bool) (Connection, financial.Credentials, error) {
	c, e := s.store.GetConnection(ctx, user, id)
	if e != nil {
		return c, financial.Credentials{}, e
	}
	if c.Status == "disabled" && (len(allowDisabled) == 0 || !allowDisabled[0]) {
		return c, financial.Credentials{}, ErrDisabled
	}
	provider, e := s.provider(c.Provider)
	if e != nil {
		return c, financial.Credentials{}, e
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
		if !cr.AccessExpiresAt.IsZero() && time.Until(cr.AccessExpiresAt) < 2*time.Minute {
			if e = provider.RefreshAuthorization(ctx, &cr); e != nil {
				s.store.SetStatus(ctx, user, id, "expired", nil)
				metadata := map[string]any{"connection_id": id}
				var providerError *financial.ProviderError
				if errors.As(e, &providerError) {
					metadata["code"] = providerError.Code
				}
				s.record(ctx, user, "financial.refresh_failed", metadata)
				return e
			}
			fresh, _ := cr.Bytes()
			if e = s.vault.Replace(ctx, credential.Locator{ConnectionID: id, UserID: user, Class: credential.Financial}, fresh); e != nil {
				return e
			}
			s.store.SetStatus(ctx, user, id, "active", credentialExpiry(cr))
			s.record(ctx, user, "financial.connection_refreshed", map[string]any{"connection_id": id})
		}
		return nil
	})
	return c, cr, e
}
func (s *Service) sync(ctx context.Context, user, id string) error {
	connection, cr, e := s.credentials(ctx, user, id)
	if e != nil {
		return e
	}
	provider, e := s.provider(connection.Provider)
	if e != nil {
		return e
	}
	accounts, e := provider.ListAccounts(ctx, &cr)
	if e != nil {
		s.observeProviderFailure(ctx, user, connection, e)
		return e
	}
	if len(accounts) == 0 {
		return &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	for i := range accounts {
		detail, de := provider.GetAccount(ctx, &cr, accounts[i].ProviderAccountID)
		if de == nil {
			detail.MaskedIdentifier = accounts[i].MaskedIdentifier
			if detail.MaskedIdentifier != "" {
				detail.DisplayName = detail.DisplayName + " " + detail.MaskedIdentifier
			}
			accounts[i] = detail
		}
	}
	if e = s.store.SyncAccounts(ctx, user, id, accounts); e == nil {
		for range accounts {
			s.record(ctx, user, "financial.account_discovered", map[string]any{"connection_id": id})
		}
		s.store.SetStatus(ctx, user, id, "active", credentialExpiry(cr))
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
	if !enabled {
		inUse, err := s.store.ConnectionInUse(ctx, p.UserID, id)
		if err != nil {
			return Connection{}, err
		}
		if inUse {
			return Connection{}, ErrConnectionInUse
		}
	}
	if enabled {
		status = "active"
		action = "financial.connection_enabled"
		connection, cr, e := s.credentials(ctx, p.UserID, id, true)
		if e != nil {
			return Connection{}, e
		}
		provider, providerErr := s.provider(connection.Provider)
		if providerErr != nil {
			return Connection{}, providerErr
		}
		if e = provider.VerifyConnection(ctx, &cr); e != nil {
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
	inUse, e := s.store.ConnectionInUse(ctx, p.UserID, id)
	if e != nil {
		return e
	}
	if inUse {
		return ErrConnectionInUse
	}
	connection, cr, e := s.credentials(ctx, p.UserID, id, true)
	if e != nil {
		return e
	}
	provider, providerErr := s.provider(connection.Provider)
	if providerErr != nil {
		return providerErr
	}
	_ = provider.Disconnect(ctx, &cr)
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
	connection, cr, e := s.credentials(ctx, p.UserID, a.ProviderConnectionID)
	if e != nil {
		return financial.Balances{}, e
	}
	provider, e := s.provider(connection.Provider)
	if e != nil {
		return financial.Balances{}, e
	}
	balances, err := provider.GetBalances(ctx, &cr, a.ProviderAccountID)
	s.observeProviderFailure(ctx, p.UserID, connection, err)
	return balances, err
}
func (s *Service) GetPositions(ctx context.Context, p authorization.Principal, id string) ([]financial.Position, error) {
	a, e := s.GetAccount(ctx, p, id)
	if e != nil {
		return nil, e
	}
	connection, cr, e := s.credentials(ctx, p.UserID, a.ProviderConnectionID)
	if e != nil {
		return nil, e
	}
	provider, e := s.provider(connection.Provider)
	if e != nil {
		return nil, e
	}
	items, e := provider.GetPositions(ctx, &cr, a.ProviderAccountID)
	s.observeProviderFailure(ctx, p.UserID, connection, e)
	for i := range items {
		items[i].AccountID = a.ID
	}
	return items, e
}

func (s *Service) observeProviderFailure(ctx context.Context, user string, connection Connection, err error) {
	if err == nil {
		return
	}
	var providerError *financial.ProviderError
	if !errors.As(err, &providerError) || (providerError.Code != financial.AuthorizationFailed && providerError.Code != financial.PermissionDenied) {
		return
	}
	_, _ = s.store.SetStatus(ctx, user, connection.ID, "error", nil)
	s.record(ctx, user, "financial.connection_requires_attention", map[string]any{"connection_id": connection.ID, "provider": connection.Provider, "code": providerError.Code})
}

func (s *Service) GetTradeFills(ctx context.Context, p authorization.Principal, id string) (financial.TradeFillPage, error) {
	a, e := s.GetAccount(ctx, p, id)
	if e != nil {
		return financial.TradeFillPage{}, e
	}
	connection, cr, e := s.credentials(ctx, p.UserID, a.ProviderConnectionID)
	if e != nil {
		return financial.TradeFillPage{}, e
	}
	broker, e := s.provider(connection.Provider)
	if e != nil {
		return financial.TradeFillPage{}, e
	}
	provider, ok := broker.(financial.TradeHistoryProvider)
	if !ok {
		return financial.TradeFillPage{}, &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	return provider.GetTradeFills(ctx, &cr, a.ProviderAccountID, 50)
}

func (s *Service) GetOrderHistory(ctx context.Context, p authorization.Principal, id string) (financial.OrderHistoryPage, error) {
	a, e := s.GetAccount(ctx, p, id)
	if e != nil {
		return financial.OrderHistoryPage{}, e
	}
	connection, cr, e := s.credentials(ctx, p.UserID, a.ProviderConnectionID)
	if e != nil {
		return financial.OrderHistoryPage{}, e
	}
	broker, e := s.provider(connection.Provider)
	if e != nil {
		return financial.OrderHistoryPage{}, e
	}
	provider, ok := broker.(financial.OrderHistoryProvider)
	if !ok {
		return financial.OrderHistoryPage{}, &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	return provider.GetOrderHistory(ctx, &cr, a.ProviderAccountID, 50)
}

func (s *Service) GetTradingCostSummary(ctx context.Context, p authorization.Principal, id string) (financial.TradingCostSummary, error) {
	a, e := s.GetAccount(ctx, p, id)
	if e != nil {
		return financial.TradingCostSummary{}, e
	}
	connection, cr, e := s.credentials(ctx, p.UserID, a.ProviderConnectionID)
	if e != nil {
		return financial.TradingCostSummary{}, e
	}
	broker, e := s.provider(connection.Provider)
	if e != nil {
		return financial.TradingCostSummary{}, e
	}
	provider, ok := broker.(financial.TradingCostProvider)
	if !ok {
		return financial.TradingCostSummary{}, &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	return provider.GetTradingCostSummary(ctx, &cr, a.ProviderAccountID)
}

func (s *Service) PreviewSpotOrder(ctx context.Context, p authorization.Principal, id string, input financial.SpotOrderPreviewRequest) (financial.SpotOrderPreview, error) {
	input.Symbol = strings.ToUpper(strings.TrimSpace(input.Symbol))
	input.Side = strings.ToUpper(strings.TrimSpace(input.Side))
	input.Size = financial.Decimal(strings.TrimSpace(string(input.Size)))
	size, sizeOK := new(big.Rat).SetString(string(input.Size))
	if !previewSymbolPattern.MatchString(input.Symbol) || input.Symbol == "USD" || (input.Side != "BUY" && input.Side != "SELL") || !previewSizePattern.MatchString(string(input.Size)) || !sizeOK || size.Sign() <= 0 {
		return financial.SpotOrderPreview{}, ErrInvalidOrderPreview
	}
	a, err := s.GetAccount(ctx, p, id)
	if err != nil {
		return financial.SpotOrderPreview{}, err
	}
	connection, credentials, err := s.credentials(ctx, p.UserID, a.ProviderConnectionID)
	if err != nil {
		return financial.SpotOrderPreview{}, err
	}
	broker, err := s.provider(connection.Provider)
	if err != nil {
		return financial.SpotOrderPreview{}, err
	}
	provider, ok := broker.(financial.OrderPreviewProvider)
	if !ok {
		return financial.SpotOrderPreview{}, &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	preview, err := provider.PreviewSpotOrder(ctx, &credentials, a.ProviderAccountID, input)
	metadata := map[string]any{"connection_id": connection.ID, "account_id": a.ID, "provider": connection.Provider, "symbol": input.Symbol, "side": input.Side}
	if err != nil {
		metadata["outcome"] = "failed"
		s.record(ctx, p.UserID, "financial.order_previewed", metadata)
		return financial.SpotOrderPreview{}, err
	}
	metadata["outcome"] = strings.ToLower(preview.PreviewState)
	s.record(ctx, p.UserID, "financial.order_previewed", metadata)
	return preview, nil
}

func (s *Service) GetQuote(ctx context.Context, p authorization.Principal, accountID, symbol string) (financial.Quote, error) {
	a, e := s.GetAccount(ctx, p, accountID)
	if e != nil {
		return financial.Quote{}, e
	}
	connection, cr, e := s.credentials(ctx, p.UserID, a.ProviderConnectionID)
	if e != nil {
		return financial.Quote{}, e
	}
	broker, e := s.provider(connection.Provider)
	if e != nil {
		return financial.Quote{}, e
	}
	provider, ok := broker.(financial.MarketDataProvider)
	if !ok {
		return financial.Quote{}, &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	return provider.GetQuote(ctx, &cr, symbol)
}

func (s *Service) GetOptionChain(ctx context.Context, p authorization.Principal, accountID string, request financial.OptionChainRequest) (financial.OptionChain, error) {
	a, e := s.GetAccount(ctx, p, accountID)
	if e != nil {
		return financial.OptionChain{}, e
	}
	connection, cr, e := s.credentials(ctx, p.UserID, a.ProviderConnectionID)
	if e != nil {
		return financial.OptionChain{}, e
	}
	broker, e := s.provider(connection.Provider)
	if e != nil {
		return financial.OptionChain{}, e
	}
	provider, ok := broker.(financial.MarketDataProvider)
	if !ok {
		return financial.OptionChain{}, &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	return provider.GetOptionChain(ctx, &cr, request)
}
