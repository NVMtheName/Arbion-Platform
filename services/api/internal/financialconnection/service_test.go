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
	orders   int
	costs    int
	previews int
}

func (provider *coinbaseProviderFake) VerifyConnection(_ context.Context, credentials *financial.Credentials) error {
	provider.verified++
	if credentials.APIKeyName != "organizations/org/apiKeys/key" || credentials.APIPrivateKey != "private-key" {
		return &financial.ProviderError{Code: financial.AuthorizationFailed}
	}
	credentials.PortfolioID = "portfolio-1"
	credentials.ProviderCanTrade = true
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
func (provider *coinbaseProviderFake) GetOrderHistory(_ context.Context, _ *financial.Credentials, id string, limit int) (financial.OrderHistoryPage, error) {
	provider.orders++
	if id != "portfolio:portfolio-1" || limit != 50 {
		return financial.OrderHistoryPage{}, errors.New("unexpected order history boundary")
	}
	return financial.OrderHistoryPage{Provider: "coinbase", Feed: "advanced_trade_orders", Orders: []financial.OrderObservation{{ProductID: "BTC-USD", Status: "OPEN", Side: "BUY"}}}, nil
}
func (provider *coinbaseProviderFake) GetTradingCostSummary(_ context.Context, _ *financial.Credentials, id string) (financial.TradingCostSummary, error) {
	provider.costs++
	if id != "portfolio:portfolio-1" {
		return financial.TradingCostSummary{}, errors.New("unexpected trading-cost boundary")
	}
	return financial.TradingCostSummary{Provider: "coinbase", Feed: "advanced_trade_transaction_summary", ProductType: "SPOT", MakerFeeRate: "0.0020", TakerFeeRate: "0.0030"}, nil
}
func (provider *coinbaseProviderFake) PreviewSpotOrder(_ context.Context, credentials *financial.Credentials, id string, input financial.SpotOrderPreviewRequest) (financial.SpotOrderPreview, error) {
	provider.previews++
	if id != "portfolio:portfolio-1" || input.Symbol != "BTC" || input.Side != "BUY" || input.Size != "25.50" || !credentials.ProviderCanTrade {
		return financial.SpotOrderPreview{}, errors.New("unexpected order preview boundary")
	}
	return financial.SpotOrderPreview{Provider: "coinbase", Feed: "advanced_trade_order_preview", ProductID: "BTC-USD", Side: "BUY", PreviewState: "READY", ProviderTradingAuthorized: true}, nil
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
	if stored.APIPrivateKey != "private-key" || stored.PortfolioID != "portfolio-1" || !stored.ProviderCanTrade {
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
	orders, err := service.GetOrderHistory(context.Background(), founder(), "account-1")
	if err != nil || len(orders.Orders) != 1 || orders.Orders[0].Status != "OPEN" || provider.orders != 1 {
		t.Fatalf("Coinbase order history was not used for owner-scoped reads: %#v %v", orders, err)
	}
	costs, err := service.GetTradingCostSummary(context.Background(), founder(), "account-1")
	if err != nil || costs.MakerFeeRate != "0.0020" || costs.ProductType != "SPOT" || provider.costs != 1 {
		t.Fatalf("Coinbase trading costs were not used for owner-scoped reads: %#v %v", costs, err)
	}
	preview, err := service.PreviewSpotOrder(context.Background(), founder(), "account-1", financial.SpotOrderPreviewRequest{Symbol: "btc", Side: "buy", Size: "25.50"})
	if err != nil || preview.ProductID != "BTC-USD" || !preview.ProviderTradingAuthorized || provider.previews != 1 {
		t.Fatalf("Coinbase preview was not owner-scoped through the non-executing boundary: %#v %v", preview, err)
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

func TestPreviewSpotOrderRejectsInvalidOrUnentitledRequestsBeforeProviderAccess(t *testing.T) {
	store := &connectionStoreFake{connection: Connection{ID: "connection-1", Provider: "coinbase", Status: "active"}, account: financial.FinancialAccount{ID: "account-1", ProviderConnectionID: "connection-1", Provider: "coinbase", ProviderAccountID: "portfolio:portfolio-1", Status: "active"}}
	provider := &coinbaseProviderFake{}
	service := NewService(store, &vaultFake{values: map[string][]byte{"connection-1": []byte(`{"api_key_name":"organizations/org/apiKeys/key","api_private_key":"private-key","portfolio_id":"portfolio-1","provider_can_trade":true}`)}}, nil, nil, nil, NamedProvider{ID: "coinbase", Provider: provider})
	for _, input := range []financial.SpotOrderPreviewRequest{
		{Symbol: "BTC-USD", Side: "BUY", Size: "10"},
		{Symbol: "BTC", Side: "HOLD", Size: "10"},
		{Symbol: "BTC", Side: "BUY", Size: "0"},
		{Symbol: "BTC", Side: "BUY", Size: "1e3"},
	} {
		if _, err := service.PreviewSpotOrder(context.Background(), founder(), "account-1", input); !errors.Is(err, ErrInvalidOrderPreview) {
			t.Fatalf("invalid preview was accepted: %#v %v", input, err)
		}
	}
	free := authorization.Principal{UserID: "user-1", Entitlement: authorization.EntitlementFree}
	if _, err := service.PreviewSpotOrder(context.Background(), free, "account-1", financial.SpotOrderPreviewRequest{Symbol: "BTC", Side: "BUY", Size: "10"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unentitled preview was accepted: %v", err)
	}
	if provider.previews != 0 {
		t.Fatalf("provider received rejected previews: %d", provider.previews)
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
