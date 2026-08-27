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
	"github.com/arbion/platform/services/api/internal/financial/oauthstate"
)

type connectionStoreFake struct {
	connection      Connection
	account         financial.FinancialAccount
	providerAccount string
	connectionInUse bool
}

func (store *connectionStoreFake) ListConnections(context.Context, string) ([]Connection, error) {
	return []Connection{store.connection}, nil
}
func (store *connectionStoreFake) UpsertConnection(_ context.Context, _ string, provider, displayName string, expires, authorizationExpires *time.Time) (Connection, error) {
	store.connection = Connection{ID: "connection-1", Provider: provider, DisplayName: displayName, Status: "pending", TokenExpiresAt: expires, AuthorizationExpiresAt: authorizationExpires, CredentialStorage: "encrypted_database"}
	return store.connection, nil
}
func (store *connectionStoreFake) UpsertConnectionForAccount(_ context.Context, _ string, provider, displayName, providerAccountID string, expires, authorizationExpires *time.Time) (Connection, error) {
	store.providerAccount = providerAccountID
	return store.UpsertConnection(context.Background(), "", provider, displayName, expires, authorizationExpires)
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
func (store *connectionStoreFake) ConnectionInUse(context.Context, string, string) (bool, error) {
	return store.connectionInUse, nil
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
	verified     int
	balances     int
	positions    int
	fills        int
	orders       int
	costs        int
	previews     int
	disconnected int
	positionData []financial.Position
	balancesErr  error
	positionsErr error
}

type schwabAuthorizerFake struct{ coinbaseProviderFake }

func (*schwabAuthorizerFake) AuthorizationURL(state string) (string, error) {
	return "https://schwab.example/authorize?state=" + state, nil
}

func (*schwabAuthorizerFake) Exchange(_ context.Context, code string) (financial.Credentials, error) {
	if code != "authorization-code" {
		return financial.Credentials{}, &financial.ProviderError{Code: financial.AuthorizationFailed}
	}
	return financial.Credentials{
		AccessToken:     "access-token",
		RefreshToken:    "refresh-token",
		TokenType:       "Bearer",
		AccessExpiresAt: time.Now().Add(30 * time.Minute),
	}, nil
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
	if provider.balancesErr != nil {
		return financial.Balances{}, provider.balancesErr
	}
	return financial.Balances{Cash: &financial.Money{Amount: "25", Currency: "USD"}}, nil
}

func (provider *coinbaseProviderFake) GetPositions(context.Context, *financial.Credentials, string) ([]financial.Position, error) {
	provider.positions++
	if provider.positionsErr != nil {
		return nil, provider.positionsErr
	}
	if provider.positionData != nil {
		return append([]financial.Position(nil), provider.positionData...), nil
	}
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
func (provider *coinbaseProviderFake) Disconnect(context.Context, *financial.Credentials) error {
	provider.disconnected++
	return nil
}

func founder() authorization.Principal {
	return authorization.Principal{UserID: "user-1", Entitlement: authorization.EntitlementFounder}
}

type reconciliationStoreFake struct {
	items []PortfolioReconciliation
}

func (store *reconciliationStoreFake) LatestReconciliation(_ context.Context, _, accountID string) (PortfolioReconciliation, error) {
	for index := len(store.items) - 1; index >= 0; index-- {
		if store.items[index].FinancialAccountID == accountID {
			return store.items[index], nil
		}
	}
	return PortfolioReconciliation{}, ErrReconciliationNotFound
}

func (store *reconciliationStoreFake) LatestReliableReconciliation(_ context.Context, _, accountID string) (PortfolioReconciliation, error) {
	for index := len(store.items) - 1; index >= 0; index-- {
		if store.items[index].FinancialAccountID == accountID && store.items[index].BalancesStatus == "READY" && store.items[index].PositionsStatus == "READY" {
			return store.items[index], nil
		}
	}
	return PortfolioReconciliation{}, ErrReconciliationNotFound
}

func (store *reconciliationStoreFake) CreateReconciliation(_ context.Context, _ string, report PortfolioReconciliation, _ []byte) (PortfolioReconciliation, error) {
	report.ID = "reconciliation-" + string(rune('1'+len(store.items)))
	report.CreatedAt = report.ObservedAt
	report.Positions = append([]ReconciliationPosition(nil), report.Positions...)
	report.Changes = append([]ReconciliationChange(nil), report.Changes...)
	store.items = append(store.items, report)
	return report, nil
}

func reconciliationService(t *testing.T, provider *coinbaseProviderFake, reports ReconciliationStore) *Service {
	t.Helper()
	store := &connectionStoreFake{
		connection: Connection{ID: "connection-1", Provider: "coinbase", Status: "active"},
		account: financial.FinancialAccount{
			ID: "account-1", UserID: founder().UserID, ProviderConnectionID: "connection-1", Provider: "coinbase",
			ProviderAccountID: "portfolio:portfolio-1", DisplayName: "Coinbase Portfolio", BaseCurrency: "USD", Status: "active",
		},
	}
	raw, err := json.Marshal(financial.Credentials{APIKeyName: "organizations/org/apiKeys/key", APIPrivateKey: "private-key", PortfolioID: "portfolio-1"})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, &vaultFake{values: map[string][]byte{"connection-1": raw}}, nil, nil, nil, NamedProvider{ID: "coinbase", Provider: provider})
	service.ConfigureReconciliation(reports)
	return service
}

func TestPortfolioReconciliationBuildsImmutableBaselineMatchAndDriftEvidence(t *testing.T) {
	average := financial.Money{Amount: "50000", Currency: "USD"}
	current := financial.Money{Amount: "60000", Currency: "USD"}
	openProfit := financial.Money{Amount: "1000", Currency: "USD"}
	provider := &coinbaseProviderFake{positionData: []financial.Position{{
		Symbol: "BTC", Quantity: "0.100000000000000000", Direction: "long", InstrumentType: "crypto",
		CostBasis: &average, CurrentPrice: &current, OpenProfitLoss: &openProfit, PriceBasis: "provider_position",
	}}}
	reports := &reconciliationStoreFake{}
	service := reconciliationService(t, provider, reports)

	baseline, err := service.RunReconciliation(context.Background(), founder(), "account-1")
	if err != nil {
		t.Fatal(err)
	}
	if baseline.ComparisonStatus != "BASELINE" || baseline.AutonomySignal != "INSUFFICIENT_EVIDENCE" || baseline.PerformanceStatus != "AVAILABLE" || baseline.RealizedPerformanceStatus != "UNAVAILABLE" || !baseline.AutonomyEnforcementActive || !baseline.BlocksNewActions || baseline.ChangeCount != 0 || len(baseline.EvidenceHash) != 64 {
		t.Fatalf("unexpected baseline: %#v", baseline)
	}
	matched, err := service.RunReconciliation(context.Background(), founder(), "account-1")
	if err != nil {
		t.Fatal(err)
	}
	if matched.ComparisonStatus != "MATCHED" || matched.AutonomySignal != "CLEAR" || !matched.AutonomyEnforcementActive || matched.BlocksNewActions || matched.PreviousReconciliationID == nil || *matched.PreviousReconciliationID != baseline.ID {
		t.Fatalf("unexpected matched report: %#v", matched)
	}
	provider.positionsErr = &financial.ProviderError{Code: financial.ProviderUnavailable}
	incomplete, err := service.RunReconciliation(context.Background(), founder(), "account-1")
	if err != nil || incomplete.ComparisonStatus != "INCOMPLETE" || !incomplete.BlocksNewActions {
		t.Fatalf("provider gap was not captured safely: %#v err=%v", incomplete, err)
	}
	provider.positionsErr = nil
	recovered, err := service.RunReconciliation(context.Background(), founder(), "account-1")
	if err != nil || recovered.ComparisonStatus != "MATCHED" || recovered.PreviousReconciliationID == nil || *recovered.PreviousReconciliationID != matched.ID {
		t.Fatalf("recovery did not compare with the last reliable snapshot: %#v err=%v", recovered, err)
	}
	provider.positionData[0].Quantity = "0.2"
	drifted, err := service.RunReconciliation(context.Background(), founder(), "account-1")
	if err != nil {
		t.Fatal(err)
	}
	if drifted.ComparisonStatus != "DRIFT_DETECTED" || drifted.AutonomySignal != "REVIEW_RECOMMENDED" || drifted.ChangeCount != 1 || drifted.BlockingChangeCount != 1 || len(drifted.Changes) != 1 || drifted.Changes[0].ChangeType != "QUANTITY_CHANGED" || drifted.Changes[0].ControlImpact != "TRADABLE_INVENTORY" || !drifted.BlocksNewActions {
		t.Fatalf("unexpected drift report: %#v", drifted)
	}
	if provider.orders != 0 || provider.fills != 0 || provider.previews != 0 || provider.disconnected != 0 {
		t.Fatalf("reconciliation crossed a mutation or unrelated provider boundary: %#v", provider)
	}
}

func TestCoinbaseReconciliationClassifiesExactUnavailableOnlyMovementWithoutGuessingItsCause(t *testing.T) {
	decimal := func(value string) *financial.Decimal {
		result := financial.Decimal(value)
		return &result
	}
	previous := []ReconciliationPosition{{
		Symbol: "USDC", InstrumentType: "CRYPTO", Direction: "long", Quantity: "17548.529979",
		AvailableQuantity: decimal("10.705979"), UnavailableQuantity: decimal("17537.824"),
	}}
	current := []ReconciliationPosition{{
		Symbol: "USDC", InstrumentType: "CRYPTO", Direction: "long", Quantity: "17548.557979",
		AvailableQuantity: decimal("10.705979"), UnavailableQuantity: decimal("17537.852"),
	}}

	changes := compareReconciliationPositions("coinbase", previous, current)
	if len(changes) != 1 || changes[0].ControlImpact != "NON_TRADABLE_QUANTITY_ONLY" || changes[0].PreviousAvailableQuantity == nil || *changes[0].PreviousAvailableQuantity != "10.705979" || changes[0].CurrentUnavailableQuantity == nil || *changes[0].CurrentUnavailableQuantity != "17537.852" {
		t.Fatalf("exact unavailable-only movement was not classified safely: %#v", changes)
	}

	current[0].AvailableQuantity = decimal("10.733979")
	current[0].UnavailableQuantity = decimal("17537.824")
	changes = compareReconciliationPositions("coinbase", previous, current)
	if len(changes) != 1 || changes[0].ControlImpact != "TRADABLE_INVENTORY" {
		t.Fatalf("available inventory movement did not remain blocking: %#v", changes)
	}

	current[0].Quantity = previous[0].Quantity
	current[0].AvailableQuantity = decimal("10.733979")
	current[0].UnavailableQuantity = decimal("17537.796")
	changes = compareReconciliationPositions("coinbase", previous, current)
	if len(changes) != 1 || changes[0].ControlImpact != "TRADABLE_INVENTORY" {
		t.Fatalf("offsetting available inventory movement was missed: %#v", changes)
	}

	current[0].Quantity = "17548.557979"
	current[0].AvailableQuantity = decimal("10.705979")
	current[0].UnavailableQuantity = decimal("17537.851")
	changes = compareReconciliationPositions("coinbase", previous, current)
	if len(changes) != 1 || changes[0].ControlImpact != "TRADABLE_INVENTORY" {
		t.Fatalf("non-reconciling provider quantities did not fail closed: %#v", changes)
	}

	changes = compareReconciliationPositions("schwab", previous, []ReconciliationPosition{{
		Symbol: "USDC", InstrumentType: "CRYPTO", Direction: "long", Quantity: "17548.557979",
		AvailableQuantity: decimal("10.705979"), UnavailableQuantity: decimal("17537.852"),
	}})
	if len(changes) != 1 || changes[0].ControlImpact != "TRADABLE_INVENTORY" {
		t.Fatalf("Coinbase-only classification escaped the provider boundary: %#v", changes)
	}
}

func TestCoinbaseUnavailableOnlyMovementKeepsAutonomousProposalGateClear(t *testing.T) {
	decimal := func(value string) *financial.Decimal {
		result := financial.Decimal(value)
		return &result
	}
	provider := &coinbaseProviderFake{positionData: []financial.Position{{
		Symbol: "USDC", Quantity: "17548.529979", Direction: "long", InstrumentType: "crypto",
		AvailableQuantity: decimal("10.705979"), UnavailableToTradeQuantity: decimal("17537.824"),
	}}}
	reports := &reconciliationStoreFake{}
	service := reconciliationService(t, provider, reports)
	if _, err := service.RunReconciliation(context.Background(), founder(), "account-1"); err != nil {
		t.Fatal(err)
	}
	provider.positionData[0].Quantity = "17548.557979"
	provider.positionData[0].UnavailableToTradeQuantity = decimal("17537.852")

	report, err := service.RunReconciliation(context.Background(), founder(), "account-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.ComparisonStatus != "MATCHED" || report.AutonomySignal != "CLEAR" || report.BlocksNewActions || report.ChangeCount != 1 || report.BlockingChangeCount != 0 || len(report.Changes) != 1 || report.Changes[0].ControlImpact != "NON_TRADABLE_QUANTITY_ONLY" {
		t.Fatalf("unavailable-only movement did not preserve the clear gate: %#v", report)
	}

	provider.positionData[0].Quantity = "17548.585979"
	provider.positionData[0].AvailableQuantity = decimal("10.733979")
	report, err = service.RunReconciliation(context.Background(), founder(), "account-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.ComparisonStatus != "DRIFT_DETECTED" || !report.BlocksNewActions || report.BlockingChangeCount != 1 || report.Changes[0].ControlImpact != "TRADABLE_INVENTORY" {
		t.Fatalf("tradable inventory movement did not fail closed: %#v", report)
	}
}

func TestPortfolioReconciliationPersistsIncompleteCoverageWithoutInventingPositions(t *testing.T) {
	provider := &coinbaseProviderFake{positionsErr: &financial.ProviderError{Code: financial.ProviderUnavailable}}
	reports := &reconciliationStoreFake{}
	service := reconciliationService(t, provider, reports)
	report, err := service.RunReconciliation(context.Background(), founder(), "account-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.ComparisonStatus != "INCOMPLETE" || report.PositionsStatus != "UNAVAILABLE" || report.PerformanceStatus != "UNAVAILABLE" || report.ObservedPositionCount != 0 || len(report.Positions) != 0 || report.AutonomySignal != "INSUFFICIENT_EVIDENCE" || !report.BlocksNewActions {
		t.Fatalf("incomplete provider coverage was not preserved explicitly: %#v", report)
	}
}

func TestPortfolioReconciliationDoesNotContinueProviderReadsAfterTerminalAuthorizationFailure(t *testing.T) {
	provider := &coinbaseProviderFake{balancesErr: &financial.ProviderError{Code: financial.PermissionDenied}}
	reports := &reconciliationStoreFake{}
	service := reconciliationService(t, provider, reports)
	report, err := service.RunReconciliation(context.Background(), founder(), "account-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.ComparisonStatus != "INCOMPLETE" || report.BalancesStatus != "UNAVAILABLE" || report.PositionsStatus != "UNAVAILABLE" || provider.positions != 0 {
		t.Fatalf("terminal authorization failure crossed another provider read boundary: report=%#v positions=%d", report, provider.positions)
	}
}

func TestScheduledReconciliationBootstrapsConfirmsAndRefreshesWithoutBrokerActions(t *testing.T) {
	now := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	provider := &coinbaseProviderFake{}
	reports := &reconciliationStoreFake{}
	service := reconciliationService(t, provider, reports)

	if err := service.EnsureScheduledReconciliation(context.Background(), founder(), "account-1", now); err != nil {
		t.Fatal(err)
	}
	if len(reports.items) != 1 || reports.items[0].ComparisonStatus != "BASELINE" || provider.balances != 1 || provider.positions != 1 {
		t.Fatalf("missing evidence did not create one read-only baseline: reports=%#v provider=%#v", reports.items, provider)
	}
	if err := service.EnsureScheduledReconciliation(context.Background(), founder(), "account-1", now.Add(29*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(reports.items) != 1 || provider.balances != 1 || provider.positions != 1 {
		t.Fatalf("baseline confirmation delay was bypassed: reports=%d provider=%#v", len(reports.items), provider)
	}
	confirmedAt := now.Add(31 * time.Minute)
	if err := service.EnsureScheduledReconciliation(context.Background(), founder(), "account-1", confirmedAt); err != nil {
		t.Fatal(err)
	}
	if len(reports.items) != 2 || reports.items[1].ComparisonStatus != "MATCHED" || reports.items[1].BlocksNewActions || provider.balances != 2 || provider.positions != 2 {
		t.Fatalf("stable baseline was not confirmed safely: reports=%#v provider=%#v", reports.items, provider)
	}
	if err := service.EnsureScheduledReconciliation(context.Background(), founder(), "account-1", confirmedAt.Add(12*time.Hour-time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(reports.items) != 2 {
		t.Fatalf("healthy evidence refreshed too early: %d", len(reports.items))
	}
	if err := service.EnsureScheduledReconciliation(context.Background(), founder(), "account-1", confirmedAt.Add(12*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(reports.items) != 3 || reports.items[2].ComparisonStatus != "MATCHED" || provider.balances != 3 || provider.positions != 3 {
		t.Fatalf("healthy evidence did not refresh before expiry: reports=%#v provider=%#v", reports.items, provider)
	}
	if provider.orders != 0 || provider.fills != 0 || provider.previews != 0 || provider.disconnected != 0 {
		t.Fatalf("scheduled reconciliation crossed a broker mutation boundary: %#v", provider)
	}
}

func TestScheduledReconciliationRetriesIncompleteButNeverClearsDrift(t *testing.T) {
	now := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	provider := &coinbaseProviderFake{}
	reports := &reconciliationStoreFake{items: []PortfolioReconciliation{{
		ID: "incomplete", FinancialAccountID: "account-1", Provider: "coinbase",
		ComparisonStatus: "INCOMPLETE", BalancesStatus: "UNAVAILABLE", PositionsStatus: "READY",
		AutonomyEnforcementActive: true, BlocksNewActions: true, ObservedAt: now.Add(-59 * time.Minute),
	}}}
	service := reconciliationService(t, provider, reports)

	if err := service.EnsureScheduledReconciliation(context.Background(), founder(), "account-1", now); err != nil {
		t.Fatal(err)
	}
	if len(reports.items) != 1 || provider.balances != 0 || provider.positions != 0 {
		t.Fatalf("incomplete evidence retried before the bounded delay: reports=%d provider=%#v", len(reports.items), provider)
	}
	if err := service.EnsureScheduledReconciliation(context.Background(), founder(), "account-1", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(reports.items) != 2 || reports.items[1].ComparisonStatus != "BASELINE" || provider.balances != 1 || provider.positions != 1 {
		t.Fatalf("incomplete evidence was not retried with provider reads only: reports=%#v provider=%#v", reports.items, provider)
	}

	reports.items = append(reports.items, PortfolioReconciliation{
		ID: "drift", FinancialAccountID: "account-1", Provider: "coinbase",
		ComparisonStatus: "DRIFT_DETECTED", BalancesStatus: "READY", PositionsStatus: "READY",
		AutonomyEnforcementActive: true, BlocksNewActions: true, ObservedAt: now.Add(2 * time.Minute),
	})
	if err := service.EnsureScheduledReconciliation(context.Background(), founder(), "account-1", now.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(reports.items) != 3 || provider.balances != 1 || provider.positions != 1 {
		t.Fatalf("confirmed drift was automatically cleared: reports=%#v provider=%#v", reports.items, provider)
	}
}

func TestScheduledReconciliationRejectsFutureEvidenceAndRefreshesLegacyMatch(t *testing.T) {
	now := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	complete := PortfolioReconciliation{ComparisonStatus: "MATCHED", BalancesStatus: "READY", PositionsStatus: "READY"}
	complete.ObservedAt = now.Add(time.Minute)
	if scheduledReconciliationDue(complete, now) {
		t.Fatal("future-dated evidence was automatically superseded")
	}
	complete.ObservedAt = now.Add(-time.Minute)
	if !scheduledReconciliationDue(complete, now) {
		t.Fatal("legacy advisory match was not selected for an enforced refresh")
	}
}

func TestSchwabAuthorizationStoresTheWeeklyReauthorizationDeadline(t *testing.T) {
	store := &connectionStoreFake{}
	states := oauthstate.New(oauthstate.NewMemoryStore(), time.Minute)
	provider := &schwabAuthorizerFake{}
	service := NewService(store, &vaultFake{}, states, provider, nil)
	state, err := states.Start(context.Background(), founder().UserID)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC()
	userID, err := service.CompleteAuthorization(context.Background(), state, "authorization-code", "")
	if err != nil {
		t.Fatal(err)
	}
	if userID != founder().UserID || store.connection.AuthorizationExpiresAt == nil {
		t.Fatalf("weekly authorization deadline missing: %#v", store.connection)
	}
	minimum := started.Add(schwabAuthorizationLifetime - time.Second)
	maximum := time.Now().UTC().Add(schwabAuthorizationLifetime + time.Second)
	if store.connection.AuthorizationExpiresAt.Before(minimum) || store.connection.AuthorizationExpiresAt.After(maximum) {
		t.Fatalf("unexpected weekly authorization deadline: %s", store.connection.AuthorizationExpiresAt)
	}
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
	if store.providerAccount != "portfolio:portfolio-1" || !strings.HasPrefix(connection.DisplayName, "Coinbase portfolio ") {
		t.Fatalf("connection identity was not bound to the verified portfolio: account=%q connection=%#v", store.providerAccount, connection)
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

func TestDisconnectAndDisableFailClosedWhileAutomationUsesConnection(t *testing.T) {
	store := &connectionStoreFake{
		connection:      Connection{ID: "connection-1", Provider: "coinbase", Status: "active"},
		account:         financial.FinancialAccount{ID: "account-1", ProviderConnectionID: "connection-1", Provider: "coinbase", ProviderAccountID: "portfolio:portfolio-1", Status: "active"},
		connectionInUse: true,
	}
	provider := &coinbaseProviderFake{}
	vault := &vaultFake{values: map[string][]byte{"connection-1": []byte(`{"api_key_name":"organizations/org/apiKeys/key","api_private_key":"private-key","portfolio_id":"portfolio-1"}`)}}
	service := NewService(store, vault, nil, nil, nil, NamedProvider{ID: "coinbase", Provider: provider})

	if err := service.Disconnect(context.Background(), founder(), "connection-1"); !errors.Is(err, ErrConnectionInUse) {
		t.Fatalf("in-use connection was not protected from disconnect: %v", err)
	}
	if _, err := service.SetEnabled(context.Background(), founder(), "connection-1", false); !errors.Is(err, ErrConnectionInUse) {
		t.Fatalf("in-use connection was not protected from disable: %v", err)
	}
	if provider.disconnected != 0 || vault.values["connection-1"] == nil || store.connection.Status != "active" {
		t.Fatalf("blocked mutation changed provider, credential, or status: disconnected=%d credential=%v status=%q", provider.disconnected, vault.values["connection-1"] != nil, store.connection.Status)
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
