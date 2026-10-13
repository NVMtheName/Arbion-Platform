package coinbase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/arbion/platform/services/api/internal/financial"
)

func TestClientPermissionEvidenceFailsClosed(t *testing.T) {
	const valid = `{"can_view":true,"can_trade":true,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`
	cases := map[string]string{
		"null": "null", "array": "[]", "empty": "{}",
		"trailing JSON": valid + `{}`, "oversized whitespace": valid + strings.Repeat(" ", maxBodyBytes),
		"duplicate":           strings.Replace(valid, `"can_transfer":false`, `"can_transfer":true,"can_transfer":false`, 1),
		"case duplicate":      strings.Replace(valid, `"can_transfer":false`, `"CAN_TRANSFER":true,"can_transfer":false`, 1),
		"escaped duplicate":   strings.Replace(valid, `"can_transfer":false`, `"can_tran\u0073fer":true,"can_transfer":false`, 1),
		"unicode alias":       strings.Replace(valid, `"can_transfer"`, `"can_tran\u017ffer"`, 1),
		"duplicate portfolio": strings.Replace(valid, `"portfolio_uuid":`, `"portfolio_uuid":"other","portfolio_uuid":`, 1),
	}
	for field, value := range map[string]string{"can_view": "true", "can_trade": "true", "can_transfer": "false", "portfolio_uuid": `"portfolio-123"`} {
		entry := fmt.Sprintf(`"%s":%s`, field, value)
		missing := strings.Replace(valid, entry+",", "", 1)
		if missing == valid {
			missing = strings.Replace(valid, ","+entry, "", 1)
		}
		cases[field+" missing"] = missing
		for label, replacement := range map[string]string{"null": "null", "number": "0", "object": "{}"} {
			cases[field+" "+label] = strings.Replace(valid, entry, fmt.Sprintf(`"%s":%s`, field, replacement), 1)
		}
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) }))
			defer server.Close()
			client, err := New(Config{BaseURL: server.URL}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			credentials, _ := testCredentials(t)
			credentials.PortfolioID = "portfolio-123"
			credentials.ProviderCanTrade = true
			err = client.VerifyConnection(context.Background(), &credentials)
			var providerError *financial.ProviderError
			if !errors.As(err, &providerError) || providerError.Code != financial.InvalidProviderResponse {
				t.Fatalf("malformed permission evidence not denied: %v", err)
			}
			if credentials.PortfolioID != "portfolio-123" || !credentials.ProviderCanTrade {
				t.Fatal("rejected evidence mutated permission state")
			}
		})
	}
}

func TestClientPermissionEvidencePreservesValidAndDeniedScopes(t *testing.T) {
	for _, tc := range []struct {
		name, body       string
		denied, canTrade bool
	}{
		{"view only", `{"can_view":true,"can_trade":false,"can_transfer":false,"portfolio_uuid":"portfolio-123","portfolio_type":"CONSUMER","future_field":true}`, false, false},
		{"trade", `{"can_view":true,"can_trade":true,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`, false, true},
		{"transfer", `{"can_view":true,"can_trade":true,"can_transfer":true,"portfolio_uuid":"portfolio-123"}`, true, false},
		{"no view", `{"can_view":false,"can_trade":true,"can_transfer":false,"portfolio_uuid":"portfolio-123"}`, true, false},
		{"wrong portfolio", `{"can_view":true,"can_trade":true,"can_transfer":false,"portfolio_uuid":"other"}`, true, false},
		{"empty portfolio", `{"can_view":true,"can_trade":true,"can_transfer":false,"portfolio_uuid":" "}`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, tc.body) }))
			defer server.Close()
			client, err := New(Config{BaseURL: server.URL}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			credentials, _ := testCredentials(t)
			credentials.PortfolioID = "portfolio-123"
			err = client.RefreshAuthorization(context.Background(), &credentials)
			var providerError *financial.ProviderError
			if tc.denied {
				if !errors.As(err, &providerError) || providerError.Code != financial.PermissionDenied {
					t.Fatalf("expected permission denial: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if credentials.PortfolioID != "portfolio-123" || credentials.ProviderCanTrade != tc.canTrade {
				t.Fatal("unexpected permission state")
			}
		})
	}
}

func TestClientSignedRequestsNeverFollowRedirects(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			for _, crossOrigin := range []bool{false, true} {
				for _, injected := range []bool{false, true} {
					t.Run(fmt.Sprintf("%d/%s/cross=%t/injected=%t", status, method, crossOrigin, injected), func(t *testing.T) {
						var originalHits, targetHits, policyCalls atomic.Int32
						target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { targetHits.Add(1); _, _ = io.WriteString(w, `{}`) }))
						defer target.Close()
						source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							if r.URL.Path == "/redirect-target" {
								targetHits.Add(1)
								_, _ = io.WriteString(w, `{}`)
								return
							}
							originalHits.Add(1)
							location := "/redirect-target"
							if crossOrigin {
								location = target.URL + location
							}
							w.Header().Set("Location", location)
							w.WriteHeader(status)
						}))
						defer source.Close()
						var shared *http.Client
						if injected {
							shared = source.Client()
							shared.CheckRedirect = func(*http.Request, []*http.Request) error { policyCalls.Add(1); return nil }
						}
						client, err := New(Config{BaseURL: source.URL}, shared)
						if err != nil {
							t.Fatal(err)
						}
						if shared != nil {
							_ = shared.CheckRedirect(nil, nil)
							if policyCalls.Load() != 1 {
								t.Fatal("constructor changed caller redirect policy")
							}
							policyCalls.Store(0)
						}
						// A later mutation of the caller's shared client cannot undo the boundary.
						if shared != nil {
							shared.CheckRedirect = func(*http.Request, []*http.Request) error { policyCalls.Add(1); return nil }
						}
						credentials, _ := testCredentials(t)
						var output map[string]any
						var input any
						if method == http.MethodPost {
							input = map[string]string{"product_id": "BTC-USD"}
						}
						err = client.request(context.Background(), &credentials, method, "/api/v3/brokerage/orders/preview", input, &output)
						var providerError *financial.ProviderError
						if !errors.As(err, &providerError) || providerError.Code != financial.InvalidProviderResponse {
							t.Fatalf("redirect not rejected: %v", err)
						}
						if originalHits.Load() != 1 || targetHits.Load() != 0 || policyCalls.Load() != 0 {
							t.Fatal("signed request followed redirect or invoked caller policy")
						}
						if shared != nil {
							_ = shared.CheckRedirect(nil, nil)
							if policyCalls.Load() != 1 {
								t.Fatal("caller redirect policy was replaced")
							}
						}
					})
				}
			}
		}
	}
}

type securityInspectTransport func(*http.Request) (*http.Response, error)

func (f securityInspectTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestClientPreviewBodyIsNotReplayable(t *testing.T) {
	var calls int
	shared := &http.Client{Transport: securityInspectTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodPost || r.GetBody != nil {
			t.Error("preview body can be replayed")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != `{"product_id":"BTC-USD"}` {
			t.Error("preview body changed")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})}
	client, err := New(Config{}, shared)
	if err != nil {
		t.Fatal(err)
	}
	credentials, _ := testCredentials(t)
	var output map[string]any
	if err := client.post(context.Background(), &credentials, "/api/v3/brokerage/orders/preview", map[string]string{"product_id": "BTC-USD"}, &output); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected one transport call, got %d", calls)
	}
}
