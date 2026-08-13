package financial

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type AuthType string

const (
	OAuth2AuthorizationCode AuthType = "oauth2_authorization_code"
	OAuth1                  AuthType = "oauth1"
	APIKey                  AuthType = "api_key"
	JWTKeyPair              AuthType = "jwt_key_pair"
	ManagedCredential       AuthType = "managed_credential"
)

type Availability string

const (
	Implemented Availability = "implemented"
	Planned     Availability = "planned"
)

type ProviderCapabilities struct {
	AccountDiscovery  bool `json:"account_discovery"`
	Balances          bool `json:"balances"`
	Positions         bool `json:"positions"`
	Orders            bool `json:"orders"`
	Options           bool `json:"options"`
	MarginInformation bool `json:"margin_information"`
	MarketData        bool `json:"market_data"`
	TokenRefresh      bool `json:"token_refresh"`
	Revocation        bool `json:"revocation"`
	Sandbox           bool `json:"sandbox_availability"`
}

type ProviderDefinition struct {
	ID           string               `json:"id"`
	Label        string               `json:"label"`
	AuthType     AuthType             `json:"auth_type"`
	Availability Availability         `json:"availability"`
	Capabilities ProviderCapabilities `json:"capabilities"`
}
type Registry map[string]ProviderDefinition

func DefaultRegistry() Registry {
	return Registry{
		"schwab":   {ID: "schwab", Label: "Charles Schwab", AuthType: OAuth2AuthorizationCode, Availability: Implemented, Capabilities: ProviderCapabilities{AccountDiscovery: true, Balances: true, Positions: true, TokenRefresh: true}},
		"etrade":   {ID: "etrade", Label: "E*TRADE", AuthType: OAuth1, Availability: Planned},
		"coinbase": {ID: "coinbase", Label: "Coinbase", AuthType: OAuth2AuthorizationCode, Availability: Planned},
	}
}
func (r Registry) List() []ProviderDefinition {
	return []ProviderDefinition{r["schwab"], r["etrade"], r["coinbase"]}
}

type CapabilityState string

const (
	Supported   CapabilityState = "SUPPORTED"
	Unsupported CapabilityState = "UNSUPPORTED"
	Unknown     CapabilityState = "UNKNOWN"
)

type Capabilities map[string]CapabilityState

// Decimal is a provider-precision-preserving transport value. It is never converted to float64.
type Decimal string
type Money struct {
	Amount   Decimal `json:"amount"`
	Currency string  `json:"currency"`
}
type FinancialAccount struct {
	ID                   string       `json:"id"`
	UserID               string       `json:"-"`
	ProviderConnectionID string       `json:"provider_connection_id"`
	Provider             string       `json:"provider"`
	ProviderAccountID    string       `json:"-"`
	DisplayName          string       `json:"display_name"`
	MaskedIdentifier     string       `json:"masked_identifier"`
	AccountType          string       `json:"account_type"`
	BaseCurrency         string       `json:"base_currency"`
	Status               string       `json:"status"`
	Capabilities         Capabilities `json:"capabilities"`
	DiscoveredAt         time.Time    `json:"discovered_at"`
	LastSyncedAt         time.Time    `json:"last_synced_at"`
}
type Balances struct {
	Cash               *Money `json:"cash,omitempty"`
	AvailableCash      *Money `json:"available_cash,omitempty"`
	BuyingPower        *Money `json:"buying_power,omitempty"`
	SettledCash        *Money `json:"settled_cash,omitempty"`
	AccountValue       *Money `json:"account_value,omitempty"`
	Equity             *Money `json:"equity,omitempty"`
	MarginBuyingPower  *Money `json:"margin_buying_power,omitempty"`
	OptionsBuyingPower *Money `json:"options_buying_power,omitempty"`
}
type Position struct {
	AccountID            string  `json:"account_id"`
	InstrumentType       string  `json:"instrument_type"`
	Symbol               string  `json:"symbol"`
	Quantity             Decimal `json:"quantity"`
	Direction            string  `json:"direction"`
	MarketValue          *Money  `json:"market_value,omitempty"`
	CostBasis            *Money  `json:"cost_basis,omitempty"`
	ProviderInstrumentID string  `json:"-"`
}
type Credentials struct {
	AccessToken      string     `json:"access_token"`
	RefreshToken     string     `json:"refresh_token"`
	TokenType        string     `json:"token_type"`
	AccessExpiresAt  time.Time  `json:"access_expires_at"`
	RefreshExpiresAt *time.Time `json:"refresh_expires_at,omitempty"`
}

func (c Credentials) Bytes() ([]byte, error) { return json.Marshal(c) }

type ProviderErrorCode string

const (
	AuthorizationFailed     ProviderErrorCode = "AUTHORIZATION_FAILED"
	AuthorizationExpired    ProviderErrorCode = "AUTHORIZATION_EXPIRED"
	ProviderUnavailable     ProviderErrorCode = "PROVIDER_UNAVAILABLE"
	RateLimited             ProviderErrorCode = "RATE_LIMITED"
	Timeout                 ProviderErrorCode = "TIMEOUT"
	AccountNotFound         ProviderErrorCode = "ACCOUNT_NOT_FOUND"
	PermissionDenied        ProviderErrorCode = "PERMISSION_DENIED"
	InvalidProviderResponse ProviderErrorCode = "INVALID_PROVIDER_RESPONSE"
	InternalError           ProviderErrorCode = "INTERNAL_ERROR"
)

type ProviderError struct {
	Code ProviderErrorCode
	Err  error
}

func (e *ProviderError) Error() string { return string(e.Code) }
func (e *ProviderError) Unwrap() error { return e.Err }

var ErrDisabled = errors.New("financial connection disabled")

type BrokerProvider interface {
	VerifyConnection(context.Context, *Credentials) error
	RefreshAuthorization(context.Context, *Credentials) error
	ListAccounts(context.Context, *Credentials) ([]FinancialAccount, error)
	GetAccount(context.Context, *Credentials, string) (FinancialAccount, error)
	GetBalances(context.Context, *Credentials, string) (Balances, error)
	GetPositions(context.Context, *Credentials, string) ([]Position, error)
	GetCapabilities(context.Context, *Credentials, string) (Capabilities, error)
	Disconnect(context.Context, *Credentials) error
}
