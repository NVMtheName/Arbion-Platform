package coinbase

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

func testCredentials(t *testing.T) (financial.Credentials, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return financial.Credentials{
		APIKeyName:    "organizations/test-org/apiKeys/test-key",
		APIPrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})),
	}, key
}

func verifyJWT(t *testing.T, request *http.Request, key *ecdsa.PrivateKey, expectedPath string) {
	t.Helper()
	token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("invalid JWT: %q", token)
	}
	decode := func(value string) []byte {
		decoded, err := base64.RawURLEncoding.DecodeString(value)
		if err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	var header map[string]any
	if err := json.Unmarshal(decode(parts[0]), &header); err != nil {
		t.Fatal(err)
	}
	if header["alg"] != "ES256" || header["kid"] != "organizations/test-org/apiKeys/test-key" || header["nonce"] == "" {
		t.Fatalf("unexpected JWT header: %#v", header)
	}
	var claims map[string]any
	if err := json.Unmarshal(decode(parts[1]), &claims); err != nil {
		t.Fatal(err)
	}
	expectedURI := request.Method + " " + request.Host + expectedPath
	if claims["iss"] != "cdp" || claims["sub"] != header["kid"] || claims["uri"] != expectedURI || int64(claims["exp"].(float64)-claims["nbf"].(float64)) != 120 {
		t.Fatalf("unexpected JWT claims: %#v", claims)
	}
	signature := decode(parts[2])
	if len(signature) != 64 {
		t.Fatalf("unexpected signature size: %d", len(signature))
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(&key.PublicKey, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
		t.Fatal("JWT signature did not verify")
	}
}

func TestClientConnectsViewOnlyPortfolioAndNormalizesHoldings(t *testing.T) {
	credentials, key := testCredentials(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		verifyJWT(t, request, key, request.URL.Path)
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v3/brokerage/key_permissions":
			_, _ = response.Write([]byte(`{"can_view":true,"can_trade":false,"can_transfer":false,"portfolio_uuid":"portfolio-123","portfolio_type":"CONSUMER"}`))
		case "/api/v3/brokerage/accounts":
			if request.URL.Query().Get("limit") != "250" {
				t.Fatalf("missing bounded page size: %s", request.URL.RawQuery)
			}
			if request.URL.Query().Get("cursor") == "" {
				_, _ = response.Write([]byte(`{"accounts":[{"uuid":"usd-wallet","currency":"USD","available_balance":{"value":"125.25","currency":"USD"},"hold":{"value":"4.75","currency":"USD"},"active":true,"ready":true,"type":"FIAT"},{"uuid":"btc-wallet","currency":"BTC","available_balance":{"value":"0.10000000","currency":"BTC"},"hold":{"value":"0.02000000","currency":"BTC"},"active":true,"ready":true,"type":"CRYPTO"}],"has_next":true,"cursor":"next-page"}`))
			} else {
				_, _ = response.Write([]byte(`{"accounts":[{"uuid":"eth-wallet","currency":"ETH","available_balance":{"value":"2.5","currency":"ETH"},"hold":{"value":"0","currency":"ETH"},"active":true,"ready":true,"type":"CRYPTO"}],"has_next":false,"cursor":""}`))
			}
		case "/api/v3/brokerage/portfolios/portfolio-123":
			if request.URL.Query().Get("currency") != "USD" {
				t.Fatalf("missing portfolio display currency: %s", request.URL.RawQuery)
			}
			_, _ = response.Write([]byte(`{"breakdown":{"portfolio":{"uuid":"portfolio-123"},"spot_positions":[{"asset":"BTC","account_uuid":"btc-wallet","total_balance_crypto":0.12000000,"available_to_trade_crypto":0.10000000,"account_type":"ACCOUNT_TYPE_CRYPTO"},{"asset":"ETH","account_uuid":"eth-staked-wallet","total_balance_crypto":2.5,"available_to_trade_crypto":0,"account_type":"ACCOUNT_TYPE_CRYPTO"},{"asset":"ETH","account_uuid":"eth-wallet","total_balance_crypto":0.5,"available_to_trade_crypto":0.25,"account_type":"ACCOUNT_TYPE_CRYPTO"}]}}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	if err = client.VerifyConnection(context.Background(), &credentials); err != nil {
		t.Fatal(err)
	}
	if credentials.PortfolioID != "portfolio-123" {
		t.Fatalf("portfolio ID was not captured: %#v", credentials)
	}
	accounts, err := client.ListAccounts(context.Background(), &credentials)
	if err != nil || len(accounts) != 1 || accounts[0].Provider != "coinbase" || accounts[0].ProviderAccountID != "portfolio:portfolio-123" {
		t.Fatalf("unexpected normalized accounts: %#v %v", accounts, err)
	}
	if accounts[0].Capabilities["trade_history"] != financial.Supported || accounts[0].Capabilities["order_history"] != financial.Supported || accounts[0].Capabilities["trading_costs"] != financial.Supported || accounts[0].Capabilities["order_preview"] != financial.Supported || accounts[0].Capabilities["provider_trade_authorization"] != financial.Unsupported || accounts[0].Capabilities["orders"] != financial.Unsupported {
		t.Fatalf("read history capability weakened the order boundary: %#v", accounts[0].Capabilities)
	}
	balances, err := client.GetBalances(context.Background(), &credentials, accounts[0].ProviderAccountID)
	if err != nil || balances.Cash == nil || balances.Cash.Amount != "130.00" || balances.AvailableCash.Amount != "125.25" {
		t.Fatalf("unexpected balances: %#v %v", balances, err)
	}
	positions, err := client.GetPositions(context.Background(), &credentials, accounts[0].ProviderAccountID)
	if err != nil || len(positions) != 2 || positions[0].Symbol != "BTC" || positions[0].Quantity != "0.12000000" || positions[0].AvailableQuantity == nil || *positions[0].AvailableQuantity != "0.10000000" || positions[0].UnavailableToTradeQuantity == nil || *positions[0].UnavailableToTradeQuantity != "0.02000000" || positions[1].Symbol != "ETH" || positions[1].Quantity != "3.0" || positions[1].AvailableQuantity == nil || *positions[1].AvailableQuantity != "0.25" || positions[1].UnavailableToTradeQuantity == nil || *positions[1].UnavailableToTradeQuantity != "2.75" {
		t.Fatalf("unexpected positions: %#v %v", positions, err)
	}
	if requests < 9 {
		t.Fatalf("expected permission and paginated account requests, got %d", requests)
	}
}

func TestClientRejectsPortfolioBreakdownWithImpossibleAvailableQuantity(t *testing.T) {
	credentials, key := testCredentials(t)
	credentials.PortfolioID = "portfolio-123"
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		verifyJWT(t, request, key, request.URL.Path)
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v3/brokerage/key_permissions":
			_, _ = response.Write([]byte(`{"can_view":true,"can_trade":false,"can_transfer":false,"portfolio_uuid":"portfolio-123","portfolio_type":"CONSUMER"}`))
		case "/api/v3/brokerage/portfolios/portfolio-123":
			_, _ = response.Write([]byte(`{"breakdown":{"portfolio":{"uuid":"portfolio-123"},"spot_positions":[{"asset":"ETH","account_uuid":"eth-wallet","total_balance_crypto":1,"available_to_trade_crypto":2,"account_type":"ACCOUNT_TYPE_CRYPTO"}]}}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetPositions(context.Background(), &credentials, "portfolio:portfolio-123")
	var providerError *financial.ProviderError
	if !errors.As(err, &providerError) || providerError.Code != financial.InvalidProviderResponse {
		t.Fatalf("expected impossible portfolio quantities to fail closed, got %v", err)
	}
}

func TestClientAcceptsTradePermissionAndRejectsTransferPermission(t *testing.T) {
	credentials, _ := testCredentials(t)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"can_view":true,"can_trade":true,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`))
	}))
	client, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err = client.VerifyConnection(context.Background(), &credentials); err != nil || !credentials.ProviderCanTrade {
		t.Fatalf("trade-authorized key was not captured safely: %#v %v", credentials, err)
	}
	server.Close()

	credentials, _ = testCredentials(t)
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"can_view":true,"can_trade":true,"can_transfer":true,"portfolio_uuid":"portfolio-123"}`))
	}))
	defer server.Close()
	client, err = New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	err = client.VerifyConnection(context.Background(), &credentials)
	var providerError *financial.ProviderError
	if !errors.As(err, &providerError) || providerError.Code != financial.PermissionDenied {
		t.Fatalf("expected transfer permission denial, got %v", err)
	}
}

func TestClientPreviewsTradeAuthorizedSpotBuyWithoutCreatingAnOrder(t *testing.T) {
	credentials, key := testCredentials(t)
	credentials.PortfolioID = "portfolio-123"
	now := time.Date(2026, 8, 21, 17, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		verifyJWT(t, request, key, request.URL.Path)
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v3/brokerage/key_permissions":
			if request.Method != http.MethodGet {
				t.Fatalf("permission check used %s", request.Method)
			}
			_, _ = response.Write([]byte(`{"can_view":true,"can_trade":true,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`))
		case "/api/v3/brokerage/products/BTC-USD":
			if request.Method != http.MethodGet || request.URL.RawQuery != "" {
				t.Fatalf("unsafe product-rules request: %s %s", request.Method, request.URL.RawQuery)
			}
			_, _ = response.Write([]byte(`{"product_id":"BTC-USD","product_type":"SPOT","base_currency_id":"BTC","quote_currency_id":"USD","base_increment":"0.00000001","quote_increment":"0.01","base_min_size":"0.00000001","base_max_size":"1000","quote_min_size":"1","quote_max_size":"1000000","status":"online","is_disabled":false,"trading_disabled":false,"cancel_only":false,"limit_only":false,"post_only":false,"auction_mode":false}`))
		case "/api/v3/brokerage/orders/preview":
			if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("unsafe preview request: %s %s", request.Method, request.Header.Get("Content-Type"))
			}
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(payload)
			body := string(encoded)
			for _, required := range []string{`"product_id":"BTC-USD"`, `"side":"BUY"`, `"quote_size":"25.50"`, `"rfq_disabled":true`} {
				if !strings.Contains(body, required) {
					t.Fatalf("preview payload missing %s: %s", required, body)
				}
			}
			for _, forbidden := range []string{"client_order_id", "retail_portfolio_id", "preview_id", "base_size"} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("preview payload contained submission field %q: %s", forbidden, body)
				}
			}
			_, _ = response.Write([]byte(`{"order_total":"25.50","commission_total":"0.15","errs":[],"warning":["SMALL_ORDER"],"quote_size":"25.50","base_size":"0.0004249","best_bid":"59990.12","best_ask":"60001.34","slippage":"0.0002","preview_id":"private-preview-id","est_average_filled_price":"60000.45"}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time { return now }
	preview, err := client.PreviewSpotOrder(context.Background(), &credentials, "portfolio:portfolio-123", financial.SpotOrderPreviewRequest{Symbol: "btc", Side: "buy", Size: "25.50"})
	if err != nil {
		t.Fatal(err)
	}
	if preview.PreviewState != "READY" || !preview.ProviderTradingAuthorized || preview.ProductID != "BTC-USD" || preview.RequestedSize.Amount != "25.50" || preview.RequestedSize.Currency != "USD" || preview.CommissionTotal.Amount != "0.15" || preview.EstimatedAverageFilledPrice == nil || preview.EstimatedAverageFilledPrice.Amount != "60000.45" || len(preview.Warnings) != 1 || preview.Warnings[0] != "SMALL_ORDER" || preview.PreviewedAt != now || preview.ProductRules == nil || !preview.ProductRules.MarketIOCEnabled || preview.ProductRules.Status != "ONLINE" || preview.ProductRules.QuoteIncrement != "0.01" || preview.ProductRules.ObservedAt != now {
		t.Fatalf("preview lost normalized provider evidence: %#v", preview)
	}
	encoded, err := json.Marshal(preview)
	if err != nil || strings.Contains(string(encoded), "private-preview-id") || strings.Contains(string(encoded), "preview_id") || strings.Contains(string(encoded), "client_order_id") {
		t.Fatalf("provider submission material escaped preview: %s %v", encoded, err)
	}
}

func TestClientNormalizesBlockedPreviewWithoutRawProviderReasons(t *testing.T) {
	credentials, _ := testCredentials(t)
	credentials.PortfolioID = "portfolio-123"
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v3/brokerage/key_permissions":
			_, _ = response.Write([]byte(`{"can_view":true,"can_trade":false,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`))
		case "/api/v3/brokerage/products/ETH-USD":
			_, _ = response.Write([]byte(`{"product_id":"ETH-USD","product_type":"SPOT","base_currency_id":"ETH","quote_currency_id":"USD","base_increment":"0.0001","quote_increment":"0.01","base_min_size":"0.001","base_max_size":"1000","quote_min_size":"1","quote_max_size":"1000000","status":"online","is_disabled":false,"trading_disabled":false,"cancel_only":false,"limit_only":false,"post_only":false,"auction_mode":false}`))
		case "/api/v3/brokerage/orders/preview":
			_, _ = response.Write([]byte(`{"order_total":"0","commission_total":"0","errs":["PREVIEW_INSUFFICIENT_LEDGER_BALANCE","PREVIEW_GEOFENCING_RESTRICTION"],"warning":[],"quote_size":"0","base_size":"0","best_bid":"60000","best_ask":"60001","preview_id":""}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	preview, err := client.PreviewSpotOrder(context.Background(), &credentials, "portfolio:portfolio-123", financial.SpotOrderPreviewRequest{Symbol: "ETH", Side: "SELL", Size: "1.25"})
	if err != nil {
		t.Fatal(err)
	}
	if preview.PreviewState != "BLOCKED" || len(preview.BlockReasons) != 2 || preview.BlockReasons[0] != "INSUFFICIENT_FUNDS" || preview.BlockReasons[1] != "ACCOUNT_RESTRICTED" || preview.ProviderTradingAuthorized {
		t.Fatalf("blocked preview was not normalized safely: %#v", preview)
	}
	encoded, _ := json.Marshal(preview)
	if strings.Contains(string(encoded), "LEDGER") || strings.Contains(string(encoded), "GEOFENCING") {
		t.Fatalf("raw provider reason escaped: %s", encoded)
	}
}

func TestClientProductRulesFailClosedForMarketIOC(t *testing.T) {
	credentials, _ := testCredentials(t)
	credentials.PortfolioID = "portfolio-123"
	now := time.Date(2026, 8, 21, 18, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v3/brokerage/key_permissions":
			_, _ = response.Write([]byte(`{"can_view":true,"can_trade":true,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`))
		case "/api/v3/brokerage/products/BTC-USD":
			_, _ = response.Write([]byte(`{"product_id":"BTC-USD","product_type":"SPOT","base_currency_id":"BTC","quote_currency_id":"USD","base_increment":"0.00000001","quote_increment":"0.10","base_min_size":"0.00000001","base_max_size":"1000","quote_min_size":"1","quote_max_size":"1000000","status":"offline","is_disabled":false,"trading_disabled":false,"cancel_only":false,"limit_only":false,"post_only":false,"auction_mode":false}`))
		case "/api/v3/brokerage/orders/preview":
			_, _ = response.Write([]byte(`{"order_total":"25.55","commission_total":"0.15","errs":[],"warning":[],"quote_size":"25.55","base_size":"0.0004249","best_bid":"59990.12","best_ask":"60001.34","preview_id":"private-preview-id","est_average_filled_price":"60000.45"}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time { return now }
	preview, err := client.PreviewSpotOrder(context.Background(), &credentials, "portfolio:portfolio-123", financial.SpotOrderPreviewRequest{Symbol: "BTC", Side: "BUY", Size: "25.55"})
	if err != nil {
		t.Fatal(err)
	}
	if preview.PreviewState != "BLOCKED" || preview.ProductRules == nil || preview.ProductRules.MarketIOCEnabled || len(preview.ProductRules.BlockReasons) != 2 || preview.ProductRules.BlockReasons[0] != "PRODUCT_DISABLED" || preview.ProductRules.BlockReasons[1] != "SIZE_INCREMENT_MISMATCH" || len(preview.BlockReasons) != 2 {
		t.Fatalf("product controls did not fail closed: %#v", preview)
	}
}

func TestClientRejectsMalformedCredentialsBeforeNetworkCall(t *testing.T) {
	client, err := New(Config{}, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	err = client.VerifyConnection(context.Background(), &financial.Credentials{APIKeyName: "wrong", APIPrivateKey: "not-pem"})
	var providerError *financial.ProviderError
	if !errors.As(err, &providerError) || providerError.Code != financial.InvalidCredentialFormat {
		t.Fatalf("expected credential-format failure, got %v", err)
	}
}

func TestClientAcceptsDocumentedQuotedAndEscapedCoinbaseCredentials(t *testing.T) {
	credentials, key := testCredentials(t)
	keyName := credentials.APIKeyName
	privateKey := strings.TrimSpace(credentials.APIPrivateKey) + "\n"
	encodedPrivateKey, err := json.Marshal(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	credentials.APIKeyName = `"` + keyName + `"`
	credentials.APIPrivateKey = string(encodedPrivateKey)

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		verifyJWT(t, request, key, request.URL.Path)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"can_view":true,"can_trade":false,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err = client.VerifyConnection(context.Background(), &credentials); err != nil {
		t.Fatal(err)
	}
	if credentials.APIKeyName != keyName || strings.Contains(credentials.APIPrivateKey, `\n`) || !strings.HasPrefix(credentials.APIPrivateKey, "-----BEGIN EC PRIVATE KEY-----") {
		t.Fatalf("credentials were not normalized into a stable storage form")
	}
}

func TestClientListsBoundedViewOnlyFillsWithoutProviderIdentifiers(t *testing.T) {
	credentials, key := testCredentials(t)
	credentials.PortfolioID = "portfolio-123"
	now := time.Date(2026, 8, 21, 15, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		verifyJWT(t, request, key, request.URL.Path)
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v3/brokerage/key_permissions":
			_, _ = response.Write([]byte(`{"can_view":true,"can_trade":false,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`))
		case "/api/v3/brokerage/orders/historical/fills":
			query := request.URL.Query()
			if query.Get("limit") != "50" || query.Get("product_types") != "SPOT" || query.Get("sort_by") != "TRADE_TIME" || len(query) != 3 {
				t.Fatalf("unexpected bounded fill query: %s", request.URL.RawQuery)
			}
			_, _ = response.Write([]byte(`{"fills":[{"entry_id":"private-entry","trade_id":"private-trade","order_id":"private-order","trade_time":"2026-08-21T14:59:00Z","trade_type":"FILL","price":"60123.123456789","size":"0.000000010000","commission":"0.00000001","product_id":"btc-usd","sequence_timestamp":"2026-08-21T14:59:01Z","liquidity_indicator":"MAKER","size_in_quote":false,"side":"BUY"},{"trade_time":"2026-08-21T14:58:00Z","trade_type":"FILL","price":"4200.25","size":"1.25","commission":"0","product_id":"ETH-USD","sequence_timestamp":"2026-08-21T14:58:01Z","liquidity_indicator":"TAKER","size_in_quote":true,"side":"SELL"}],"cursor":"next-private-page","proof_token_required":false}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time { return now }
	page, err := client.GetTradeFills(context.Background(), &credentials, "portfolio:portfolio-123", 50)
	if err != nil {
		t.Fatal(err)
	}
	if page.Provider != "coinbase" || page.Feed != "advanced_trade_fills" || !page.HasMore || len(page.Fills) != 2 {
		t.Fatalf("unexpected fill page: %#v", page)
	}
	first, second := page.Fills[0], page.Fills[1]
	if first.ProductID != "BTC-USD" || first.Price != "60123.123456789" || first.Size != "0.000000010000" || first.SizeUnit != "BTC" || first.Commission.Amount != "0.00000001" || first.Side != "BUY" || first.Liquidity != "MAKER" {
		t.Fatalf("first fill lost exact normalized evidence: %#v", first)
	}
	if second.SizeUnit != "USD" || second.Side != "SELL" || second.Liquidity != "TAKER" || second.Commission.Amount != "0" {
		t.Fatalf("quote-sized fill was not explicit: %#v", second)
	}
	encoded, err := json.Marshal(page)
	if err != nil || strings.Contains(string(encoded), "private-") || strings.Contains(string(encoded), "cursor") {
		t.Fatalf("provider identifiers escaped the normalized page: %s %v", encoded, err)
	}
}

func TestClientRejectsUnsafeFillResponses(t *testing.T) {
	for name, body := range map[string]string{
		"proof required":      `{"fills":[],"proof_token_required":true}`,
		"negative commission": `{"fills":[{"trade_time":"2026-08-21T14:59:00Z","trade_type":"FILL","price":"1","size":"1","commission":"-0.01","product_id":"BTC-USD","sequence_timestamp":"2026-08-21T14:59:01Z","side":"BUY"}]}`,
		"not a fill":          `{"fills":[{"trade_time":"2026-08-21T14:59:00Z","trade_type":"ORDER","price":"1","size":"1","commission":"0","product_id":"BTC-USD","sequence_timestamp":"2026-08-21T14:59:01Z","side":"BUY"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			credentials, _ := testCredentials(t)
			credentials.PortfolioID = "portfolio-123"
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/api/v3/brokerage/key_permissions" {
					_, _ = response.Write([]byte(`{"can_view":true,"can_trade":false,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`))
					return
				}
				_, _ = response.Write([]byte(body))
			}))
			defer server.Close()
			client, err := New(Config{BaseURL: server.URL}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			client.now = func() time.Time { return time.Date(2026, 8, 21, 15, 0, 0, 0, time.UTC) }
			_, err = client.GetTradeFills(context.Background(), &credentials, "portfolio:portfolio-123", 50)
			var providerError *financial.ProviderError
			if !errors.As(err, &providerError) || (providerError.Code != financial.InvalidProviderResponse && providerError.Code != financial.PermissionDenied) {
				t.Fatalf("expected safe rejection, got %v", err)
			}
		})
	}
}

func TestClientListsBoundedViewOnlyOrdersWithoutProviderIdentifiers(t *testing.T) {
	credentials, key := testCredentials(t)
	credentials.PortfolioID = "portfolio-123"
	now := time.Date(2026, 8, 21, 16, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		verifyJWT(t, request, key, request.URL.Path)
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v3/brokerage/key_permissions":
			_, _ = response.Write([]byte(`{"can_view":true,"can_trade":false,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`))
		case "/api/v3/brokerage/orders/historical/batch":
			query := request.URL.Query()
			if query.Get("limit") != "50" || query.Get("product_type") != "SPOT" || query.Get("order_placement_source") != "RETAIL_ADVANCED" || query.Get("use_simplified_total_value_calculation") != "true" || len(query) != 4 {
				t.Fatalf("unexpected bounded order query: %s", request.URL.RawQuery)
			}
			_, _ = response.Write([]byte(`{"orders":[{"order_id":"private-order","user_id":"private-user","retail_portfolio_id":"private-portfolio","product_id":"btc-usd","side":"BUY","status":"OPEN","created_time":"2026-08-21T15:50:00Z","completion_percentage":"25.000","average_filled_price":"60123.123456789","number_of_fills":"1","pending_cancel":false,"total_fees":"0.00000001","time_in_force":"GOOD_UNTIL_CANCELLED","filled_size":"0.000000010000","filled_value":"0.00060123123456789","order_type":"LIMIT","reject_reason":"REJECT_REASON_UNSPECIFIED","settled":false,"product_type":"SPOT","is_liquidation":false,"last_fill_time":"2026-08-21T15:55:00Z"},{"order_id":"another-private-order","product_id":"ETH-USD","side":"SELL","status":"CANCELLED","created_time":"2026-08-21T15:30:00Z","completion_percentage":"0","average_filled_price":"0","number_of_fills":"0","pending_cancel":false,"total_fees":"0","time_in_force":"IMMEDIATE_OR_CANCEL","filled_size":"0","filled_value":"0","order_type":"MARKET","reject_reason":"REJECT_REASON_UNSPECIFIED","settled":false,"product_type":"SPOT","is_liquidation":false,"last_fill_time":""}],"has_next":true,"cursor":"private-next-page","proof_token_required":false}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time { return now }
	page, err := client.GetOrderHistory(context.Background(), &credentials, "portfolio:portfolio-123", 50)
	if err != nil {
		t.Fatal(err)
	}
	if page.Provider != "coinbase" || page.Feed != "advanced_trade_orders" || !page.HasMore || len(page.Orders) != 2 {
		t.Fatalf("unexpected order page: %#v", page)
	}
	first, second := page.Orders[0], page.Orders[1]
	if first.ProductID != "BTC-USD" || first.Status != "OPEN" || first.CompletionPercentage != "25.000" || first.FilledSize != "0.000000010000" || first.FilledValue.Amount != "0.00060123123456789" || first.AverageFilledPrice == nil || first.AverageFilledPrice.Amount != "60123.123456789" || first.TotalFees.Amount != "0.00000001" || first.NumberOfFills != 1 || first.TimeInForce != "GOOD_UNTIL_CANCELLED" || first.OutcomeReason != "NONE" {
		t.Fatalf("open order lost exact normalized state: %#v", first)
	}
	if second.Status != "CANCELLED" || second.AverageFilledPrice != nil || second.LastFillAt != nil || second.OrderType != "MARKET" {
		t.Fatalf("unfilled order was not normalized safely: %#v", second)
	}
	encoded, err := json.Marshal(page)
	if err != nil || strings.Contains(string(encoded), "private-") || strings.Contains(string(encoded), "cursor") || strings.Contains(string(encoded), "order_id") || strings.Contains(string(encoded), "user_id") {
		t.Fatalf("provider identifiers escaped the normalized order page: %s %v", encoded, err)
	}
}

func TestClientRejectsUnsafeOrderResponses(t *testing.T) {
	for name, body := range map[string]string{
		"proof required":      `{"orders":[],"proof_token_required":true}`,
		"missing next cursor": `{"orders":[],"has_next":true,"cursor":""}`,
		"invalid percentage":  `{"orders":[{"product_id":"BTC-USD","side":"BUY","status":"OPEN","created_time":"2026-08-21T15:50:00Z","completion_percentage":"100.01","number_of_fills":"0","total_fees":"0","filled_size":"0","filled_value":"0","order_type":"LIMIT","time_in_force":"GOOD_UNTIL_CANCELLED","product_type":"SPOT"}]}`,
		"unknown status":      `{"orders":[{"product_id":"BTC-USD","side":"BUY","status":"MYSTERY","created_time":"2026-08-21T15:50:00Z","completion_percentage":"0","number_of_fills":"0","total_fees":"0","filled_size":"0","filled_value":"0","order_type":"LIMIT","time_in_force":"GOOD_UNTIL_CANCELLED","product_type":"SPOT"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			credentials, _ := testCredentials(t)
			credentials.PortfolioID = "portfolio-123"
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/api/v3/brokerage/key_permissions" {
					_, _ = response.Write([]byte(`{"can_view":true,"can_trade":false,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`))
					return
				}
				_, _ = response.Write([]byte(body))
			}))
			defer server.Close()
			client, err := New(Config{BaseURL: server.URL}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			client.now = func() time.Time { return time.Date(2026, 8, 21, 16, 0, 0, 0, time.UTC) }
			_, err = client.GetOrderHistory(context.Background(), &credentials, "portfolio:portfolio-123", 50)
			var providerError *financial.ProviderError
			if !errors.As(err, &providerError) || (providerError.Code != financial.InvalidProviderResponse && providerError.Code != financial.PermissionDenied) {
				t.Fatalf("expected safe rejection, got %v", err)
			}
		})
	}
}

func TestClientGetsViewOnlyTradingCostSummaryWithoutPrivateProviderFields(t *testing.T) {
	credentials, key := testCredentials(t)
	credentials.PortfolioID = "portfolio-123"
	now := time.Date(2026, 8, 21, 16, 30, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		verifyJWT(t, request, key, request.URL.Path)
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v3/brokerage/key_permissions":
			_, _ = response.Write([]byte(`{"can_view":true,"can_trade":false,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`))
		case "/api/v3/brokerage/transaction_summary":
			query := request.URL.Query()
			if query.Get("product_type") != "SPOT" || len(query) != 1 {
				t.Fatalf("unexpected trading-cost query: %s", request.URL.RawQuery)
			}
			_, _ = response.Write([]byte(`{"total_fees":25.00000001,"fee_tier":{"pricing_tier":"<$10k","taker_fee_rate":"0.0010","maker_fee_rate":"0.0020","aop_from":"private-lower","aop_to":"private-upper"},"margin_rate":0.5,"advanced_trade_only_volume":1000.123456789,"advanced_trade_only_fees":20.00000001,"coinbase_pro_volume":999,"coinbase_pro_fees":9,"total_balance":"private-balance","volume_breakdown":[{"volume_type":"VOLUME_TYPE_SPOT","volume":1000}],"has_cost_plus_commission":false}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time { return now }
	summary, err := client.GetTradingCostSummary(context.Background(), &credentials, "portfolio:portfolio-123")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Provider != "coinbase" || summary.Feed != "advanced_trade_transaction_summary" || summary.ProductType != "SPOT" || summary.PricingTier != "<$10k" || summary.MakerFeeRate != "0.0020" || summary.TakerFeeRate != "0.0010" || summary.AdvancedTradeVolume.Amount != "1000.123456789" || summary.AdvancedTradeFees.Amount != "20.00000001" || summary.TotalFees.Amount != "25.00000001" || summary.RetrievedAt != now {
		t.Fatalf("trading-cost evidence lost exact provider values: %#v", summary)
	}
	encoded, err := json.Marshal(summary)
	if err != nil || strings.Contains(string(encoded), "private-") || strings.Contains(string(encoded), "margin_rate") || strings.Contains(string(encoded), "total_balance") || strings.Contains(string(encoded), "volume_breakdown") || strings.Contains(string(encoded), "coinbase_pro") {
		t.Fatalf("excluded transaction-summary fields escaped the projection: %s %v", encoded, err)
	}
}

func TestClientRejectsUnsafeTradingCostSummary(t *testing.T) {
	for name, body := range map[string]string{
		"missing total fees": `{"fee_tier":{"pricing_tier":"<$10k","taker_fee_rate":"0.0010","maker_fee_rate":"0.0020"},"advanced_trade_only_volume":1000,"advanced_trade_only_fees":20}`,
		"negative fees":      `{"total_fees":-1,"fee_tier":{"pricing_tier":"<$10k","taker_fee_rate":"0.0010","maker_fee_rate":"0.0020"},"advanced_trade_only_volume":1000,"advanced_trade_only_fees":20}`,
		"rate over one":      `{"total_fees":1,"fee_tier":{"pricing_tier":"<$10k","taker_fee_rate":"1.0001","maker_fee_rate":"0.0020"},"advanced_trade_only_volume":1000,"advanced_trade_only_fees":20}`,
		"unsafe tier label":  `{"total_fees":1,"fee_tier":{"pricing_tier":"\u2603","taker_fee_rate":"0.0010","maker_fee_rate":"0.0020"},"advanced_trade_only_volume":1000,"advanced_trade_only_fees":20}`,
	} {
		t.Run(name, func(t *testing.T) {
			credentials, _ := testCredentials(t)
			credentials.PortfolioID = "portfolio-123"
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/api/v3/brokerage/key_permissions" {
					_, _ = response.Write([]byte(`{"can_view":true,"can_trade":false,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`))
					return
				}
				_, _ = response.Write([]byte(body))
			}))
			defer server.Close()
			client, err := New(Config{BaseURL: server.URL}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.GetTradingCostSummary(context.Background(), &credentials, "portfolio:portfolio-123")
			var providerError *financial.ProviderError
			if !errors.As(err, &providerError) || providerError.Code != financial.InvalidProviderResponse {
				t.Fatalf("expected safe rejection, got %v", err)
			}
		})
	}
}
