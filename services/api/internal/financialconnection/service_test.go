package financialconnection

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
)

type connectionStoreFake struct {
	connection Connection
	account    financial.FinancialAccount
}

func (store *connectionStoreFake) ListConnections(context.Context, string) ([]Connection, error) {
	return []Connection{store.connection}, nil
}
func (store *connectionStoreFake) UpsertConnection(_ context.Context, _ string, provider, displayName string, expires *time.Time) (Connection, error) {
	store.connection = Connection{ID: "connection-1", Provider: provider, DisplayName: displayName, Status: "pending", TokenExpiresAt: expires, CredentialStorage: "encrypted_database"}
	return store.connection, nil
}
func (store *connectionStoreFake) GetConnection(context.Context, string, string) (Connection, error) {
	if store.connection.ID == "" {
		return Connection{}, ErrNotFound
	}
	return store.connection, nil
}
func (store *connectionStoreFake) SetStatus(_ context.Context, _, _ string, status string, expires *time.Time) (Connection, error) {
	store.connection.Status = status
	if expires != nil {
		store.connection.TokenExpiresAt = expires
	}
	now := time.Now()
	store.connection.LastSyncedAt = &now
	return store.connection, nil
}
func (store *connectionStoreFake) SyncAccounts(_ context.Context, _ string, connectionID string, accounts []financial.FinancialAccount) error {
	if len(accounts) != 1 {
		return errors.New("expected one aggregate account")
	}
	store.account = accounts[0]
	store.account.ID = "account-1"
	store.account.ProviderConnectionID = connectionID
	return nil
}
func (store *connectionStoreFake) ListAccounts(context.Context, string) ([]financial.FinancialAccount, error) {
	return []financial.FinancialAccount{store.account}, nil
}
func (store *connectionStoreFake) GetAccount(context.Context, string, string) (financial.FinancialAccount, error) {
	if store.account.ID == "" {
		return financial.FinancialAccount{}, ErrNotFound
	}
	return store.account, nil
}
func (store *connectionStoreFake) Retire(context.Context, string, string) error {
	store.connection.Status = "revoked"
	return nil
}
func (*connectionStoreFake) WithLock(_ context.Context, _ string, fn func() error) error { return fn() }

type vaultFake struct{ values map[string][]byte }

func (vault *vaultFake) Store(_ context.Context, locator credential.Locator, value []byte) error {
	if vault.values == nil {
		vault.values = map[string][]byte{}
	}
	if _, exists := vault.values[locator.ConnectionID]; exists {
		return errors.New("already stored")
	}
	vault.values[locator.ConnectionID] = append([]byte(nil), value...)
	return nil
}
func (vault *vaultFake) Retrieve(_ context.Context, locator credential.Locator) ([]byte, error) {
	value, ok := vault.values[locator.ConnectionID]
	if !ok {
		return nil, credential.ErrNotFound
	}
	return append([]byte(nil), value...), nil
}
func (vault *vaultFake) Replace(_ context.Context, locator credential.Locator, value []byte) error {
	vault.values[locator.ConnectionID] = append([]byte(nil), value...)
	return nil
}
func (vault *vaultFake) Delete(_ context.Context, locator credential.Locator) error {
	delete(vault.values, locator.ConnectionID)
	return nil
}

type coinbaseProviderFake struct {
	verified int
	balances int
	fills    int
}

func (provider *coinbaseProviderFake) VerifyConnection(_ context.Context, credentials *financial.Credentials) error {
	provider.verified++
	if credentials.APIKeyName != "organizations/org/apiKeys/key" || credentials.APIPrivateKey != "private-key" {
		return &financial.ProviderError{Code: financial.AuthorizationFailed}
	}
	credentials.PortfolioID = "portfolio-1"
	return nil
}
func (provider *coinbaseProviderFake) RefreshAuthorization(ctx context.Context, credentials *financial.Credentials) error {
	return provider.VerifyConnection(ctx, credentials)
}
func (*coinbaseProviderFake) ListAccounts(_ context.Context, credentials *financial.Credentials) ([]financial.FinancialAccount, error) {
	return []financial.FinancialAccount{{Provider: "coinbase", ProviderAccountID: "portfolio:" + credentials.PortfolioID, DisplayName: "Coinbase Portfolio", AccountType: "digital_asset_portfolio", BaseCurrency: "USD", Status: "active", Capabilities: financial.Capabilities{"crypto_assets": financial.Supported}}}, nil
}
func (*coinbaseProviderFake) GetAccount(_ context.Context, credentials *financial.Credentials, id string) (financial.FinancialAccount, error) {
	return financial.FinancialAccount{Provider: "coinbase", ProviderAccountID: id, DisplayName: "Coinbase Portfolio", AccountType: "digital_asset_portfolio", BaseCurrency: "USD", Status: "active", Capabilities: financial.Capabilities{"crypto_assets": financial.Supported}}, nil
}
func (provider *coinbaseProviderFake) GetBalances(context.Context, *financial.Credentials, string) (financial.Balances, error) {
	provider.balances++
	return financial.Balances{Cash: &financial.Money{Amount: "25", Currency: "USD"}}, nil
}
func (*coinbaseProviderFake) GetPositions(context.Context, *financial.Credentials, string) ([]financial.Position, error) {
	return []financial.Position{{Symbol: "BTC", Quantity: "0.1", Direction: "long", InstrumentType: "CRYPTO"}}, nil
}
func (provider *coinbaseProviderFake) GetTradeFills(_ context.Context, _ *financial.Credentials, id string, limit int) (financial.TradeFillPage, error) {
	provider.fills++
	if id != "portfolio:portfolio-1" || limit != 50 {
		return financial.TradeFillPage{}, errors.New("unexpected trade history boundary")
	}
	return financial.TradeFillPage{Provider: "coinbase", Feed: "advanced_trade_fills", Fills: []financial.TradeFill{{ProductID: "BTC-USD", Side: "BUY", Price: "1", Size: "1", SizeUnit: "BTC"}}}, nil
}
func (*coinbaseProviderFake) GetCapabilities(context.Context, *financial.Credentials, string) (financial.Capabilities, error) {
	return financial.Capabilities{"crypto_assets": financial.Supported}, nil
}
func (*coinbaseProviderFake) Disconnect(context.Context, *financial.Credentials) error { return nil }

func founder() authorization.Principal {
	return authorization.Principal{UserID: "user-1", Entitlement: authorization.EntitlementFounder}
}

func TestConnectAPIKeyStoresServerOnlyCredentialsAndRoutesAccountReads(t *testing.T) {
	store := &connectionStoreFake{}
	vault := &vaultFake{}
	provider := &coinbaseProviderFake{}
	service := NewService(store, vault, nil, nil, nil, NamedProvider{ID: "coinbase", Provider: provider})

	connection, err := service.ConnectAPIKey(context.Background(), founder(), "coinbase", "organizations/org/apiKeys/key", "private-key")
	if err != nil {
		t.Fatal(err)
	}
	if connection.Provider != "coinbase" || connection.Status != "active" || provider.verified != 1 {
		t.Fatalf("unexpected connection result: %#v verified=%d", connection, provider.verified)
	}
	var stored financial.Credentials
	if err := json.Unmarshal(vault.values[connection.ID], &stored); err != nil {
		t.Fatal(err)
	}
	if stored.APIPrivateKey != "private-key" || stored.PortfolioID != "portfolio-1" {
		t.Fatalf("credential payload was not stored through the vault: %#v", stored)
	}
	public, err := json.Marshal(connection)
	if err != nil || string(public) == "" || containsAny(string(public), "private-key", "organizations/org/apiKeys/key") {
		t.Fatalf("connection response leaked credential material: %s %v", public, err)
	}
	balances, err := service.GetBalances(context.Background(), founder(), "account-1")
	if err != nil || balances.Cash == nil || balances.Cash.Amount != "25" || provider.balances != 1 {
		t.Fatalf("Coinbase provider was not used for account reads: %#v %v", balances, err)
	}
	fills, err := service.GetTradeFills(context.Background(), founder(), "account-1")
	if err != nil || len(fills.Fills) != 1 || fills.Fills[0].ProductID != "BTC-USD" || provider.fills != 1 {
		t.Fatalf("Coinbase history provider was not used for owner-scoped reads: %#v %v", fills, err)
	}
}

func TestConnectAPIKeyRequiresFinancialEntitlementAndValidInput(t *testing.T) {
	service := NewService(&connectionStoreFake{}, &vaultFake{}, nil, nil, nil, NamedProvider{ID: "coinbase", Provider: &coinbaseProviderFake{}})
	free := authorization.Principal{UserID: "user-1", Entitlement: authorization.EntitlementFree}
	if _, err := service.ConnectAPIKey(context.Background(), free, "coinbase", "name", "key"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected entitlement rejection, got %v", err)
	}
	if _, err := service.ConnectAPIKey(context.Background(), founder(), "coinbase", "", "key"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input rejection, got %v", err)
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
