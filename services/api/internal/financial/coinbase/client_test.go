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
	balances, err := client.GetBalances(context.Background(), &credentials, accounts[0].ProviderAccountID)
	if err != nil || balances.Cash == nil || balances.Cash.Amount != "130.00" || balances.AvailableCash.Amount != "125.25" {
		t.Fatalf("unexpected balances: %#v %v", balances, err)
	}
	positions, err := client.GetPositions(context.Background(), &credentials, accounts[0].ProviderAccountID)
	if err != nil || len(positions) != 2 || positions[0].Symbol != "BTC" || positions[0].Quantity != "0.12000000" || positions[1].Symbol != "ETH" {
		t.Fatalf("unexpected positions: %#v %v", positions, err)
	}
	if requests < 7 {
		t.Fatalf("expected permission and paginated account requests, got %d", requests)
	}
}

func TestClientRejectsKeysWithTradeOrTransferPermission(t *testing.T) {
	for _, field := range []string{"can_trade", "can_transfer"} {
		t.Run(field, func(t *testing.T) {
			credentials, _ := testCredentials(t)
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				_, _ = response.Write([]byte(`{"can_view":true,"` + field + `":true,"portfolio_uuid":"portfolio-123"}`))
			}))
			defer server.Close()
			client, err := New(Config{BaseURL: server.URL}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			err = client.VerifyConnection(context.Background(), &credentials)
			var providerError *financial.ProviderError
			if !errors.As(err, &providerError) || providerError.Code != financial.PermissionDenied {
				t.Fatalf("expected permission denial, got %v", err)
			}
		})
	}
}

func TestClientRejectsMalformedCredentialsBeforeNetworkCall(t *testing.T) {
	client, err := New(Config{}, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	err = client.VerifyConnection(context.Background(), &financial.Credentials{APIKeyName: "wrong", APIPrivateKey: "not-pem"})
	var providerError *financial.ProviderError
	if !errors.As(err, &providerError) || providerError.Code != financial.AuthorizationFailed {
		t.Fatalf("expected authorization failure, got %v", err)
	}
}
