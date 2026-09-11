package etrade

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

// All credentials below are invented offline fixtures unless explicitly marked
// as E*TRADE's published signature example. No test can reach a financial API.
type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

func fixture(t *testing.T, env Environment, transport transportFunc) *Client {
	t.Helper()
	c, err := New(Config{Environment: env, ConsumerKey: "test-consumer", ConsumerSecret: "test-consumer-secret"}, &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return time.Date(2026, 9, 11, 15, 0, 0, 0, time.UTC) }
	return c
}

func access(c *Client) *Authorization {
	return &Authorization{token: "test-access", secret: "test-access-secret", binding: c.binding, issuedAt: c.now(), expiresAt: c.now().Add(time.Hour)}
}

func pending(c *Client) *PendingAuthorization {
	return &PendingAuthorization{token: "test-request", secret: "test-request-secret", binding: c.binding, issuedAt: c.now(), expiresAt: c.now().Add(requestTTL)}
}

func requireCode(t *testing.T, err error, code financial.ProviderErrorCode) {
	t.Helper()
	var providerErr *financial.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != code || providerErr.Err != nil {
		t.Fatalf("wanted redacted %s; got %v", code, err)
	}
}

func headerValues(t *testing.T, r *http.Request) url.Values {
	t.Helper()
	if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.Body != nil || r.URL.Scheme != "https" {
		t.Fatal("unexpected method, query, body, or scheme")
	}
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "OAuth ") {
		t.Fatal("OAuth authorization header missing")
	}
	values := url.Values{}
	for _, field := range strings.Split(strings.TrimPrefix(header, "OAuth "), ", ") {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			t.Fatal("invalid header field")
		}
		value, err := url.QueryUnescape(strings.Trim(parts[1], "\""))
		if err != nil {
			t.Fatal(err)
		}
		values.Set(parts[0], value)
	}
	if values.Get("oauth_nonce") == "" || values.Get("oauth_signature") == "" || values.Get("oauth_signature_method") != "HMAC-SHA1" {
		t.Fatal("signature metadata incomplete")
	}
	return values
}

func TestOfficialEtradeSignatureVector(t *testing.T) {
	// Public, mathematically valid fixture from E*TRADE Developer Guides, not
	// application credentials: https://developer.etrade.com/getting-started/developer-guides
	u, _ := url.Parse("https://api.etrade.com/v1/accounts/list")
	header, err := authorizationHeader(u, "c5bb4dcb7bd6826c7c4340df3f791188", "7d30246211192cda43ede3abd9b393b9",
		"VbiNYl63EejjlKdQM6FeENzcnrLACrZ2JYD6NQROfVI=", "XCF9RzyQr4UEPloA+WlC06BnTfYC1P0Fwr3GUw/B0Es=",
		"0bba225a40d1bbac2430aa0c6163ce44", 1344885636, nil)
	if err != nil || !strings.Contains(header, `oauth_signature="UOnPVdzExTAgHkcGWLLfeTaaMSM%3D"`) {
		t.Fatal("signature differs from official known-answer fixture")
	}
}

func TestOAuthEncodingAndParameterOrdering(t *testing.T) {
	if got := oauthEscape("a +/~!é"); got != "a%20%2B%2F~%21%C3%A9" {
		t.Fatalf("bad RFC3986 escaping: %s", got)
	}
	u, _ := url.Parse("https://API.ETRADE.COM:443/v1/accounts/list?a-=last&a=z&a=+&b=%2B")
	header, err := authorizationHeader(u, "consumer", "secret", "token", "token-secret", "nonce", 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Independently specified normalization catches key-prefix ordering and
	// duplicate-value sorting; sorting concatenated key=value pairs is wrong.
	normalized := "a=%20&a=z&a-=last&b=%2B&oauth_consumer_key=consumer&oauth_nonce=nonce&oauth_signature_method=HMAC-SHA1&oauth_timestamp=100&oauth_token=token"
	mac := hmac.New(sha1.New, []byte("secret&token-secret"))
	_, _ = mac.Write([]byte("GET&" + oauthEscape("https://api.etrade.com/v1/accounts/list") + "&" + oauthEscape(normalized)))
	expected := oauthEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	if !strings.Contains(header, `oauth_signature="`+expected+`"`) {
		t.Fatal("query parameters were not normalized exactly")
	}
	for _, raw := range []string{"http://api.etrade.com/x", "https://user:secret@api.etrade.com/x", "https://api.etrade.com/x#fragment", "https://api.etrade.com/x?oauth_token=other", "https://api.etrade.com/x?q=%ZZ"} {
		u, _ := url.Parse(raw)
		_, err := authorizationHeader(u, "k", "s", "t", "s", "n", 100, nil)
		requireCode(t, err, financial.InternalError)
	}
}

func TestOAuthLifecycleAndIsolatedEndpoints(t *testing.T) {
	for _, env := range []Environment{Sandbox, Production} {
		t.Run(string(env), func(t *testing.T) {
			nonces := map[string]bool{}
			calls := 0
			c := fixture(t, env, func(r *http.Request) (*http.Response, error) {
				calls++
				h := headerValues(t, r)
				if nonces[h.Get("oauth_nonce")] || len(h.Get("oauth_nonce")) != 64 {
					t.Fatal("nonce reused or insufficient entropy")
				}
				nonces[h.Get("oauth_nonce")] = true
				switch r.URL.Path {
				case "/oauth/request_token":
					if r.URL.Host != "api.etrade.com" || h.Get("oauth_callback") != "oob" || h.Get("oauth_token") != "" {
						t.Fatal("bad request-token request")
					}
					return response(200, "oauth_token=test-request&oauth_token_secret=test-request-secret&oauth_callback_confirmed=false"), nil
				case "/oauth/access_token":
					if r.URL.Host != "api.etrade.com" || h.Get("oauth_token") != "test-request" || h.Get("oauth_verifier") != "test-verifier" {
						t.Fatal("bad access-token exchange")
					}
					return response(200, "oauth_token=test-access&oauth_token_secret=test-access-secret"), nil
				case "/oauth/renew_access_token":
					if r.URL.Host != "api.etrade.com" || h.Get("oauth_token") != "test-access" || h.Get("oauth_verifier") != "" {
						t.Fatal("bad access-token renewal")
					}
					return response(200, "Access Token has been renewed"), nil
				case "/v1/accounts/list":
					wantHost := "api.etrade.com"
					if env == Sandbox {
						wantHost = "apisb.etrade.com"
					}
					if r.URL.Host != wantHost || h.Get("oauth_token") != "test-access" || r.Header.Get("Accept") != "application/json" {
						t.Fatal("wrong data environment or token kind")
					}
					return response(200, inventoryJSON), nil
				default:
					t.Fatal("unexpected endpoint; no order/transfer surface is permitted")
					return nil, errors.New("unexpected request")
				}
			})
			p, err := c.BeginAuthorization(context.Background())
			if err != nil || p.Status().CallbackConfirmed || !p.Status().ExpiresAt.Equal(c.now().Add(5*time.Minute)) {
				t.Fatal("request-token metadata invalid", err)
			}
			consent, err := c.AuthorizationURL(p)
			if err != nil || consent != "https://us.etrade.com/e/t/etws/authorize?key=test-consumer&token=test-request" {
				t.Fatal("bad consent URL")
			}
			a, err := c.Exchange(context.Background(), p, "test-verifier")
			if err != nil || a.ExpiresAt().Format(time.RFC3339) != "2026-09-12T04:00:00Z" {
				t.Fatal("access authorization/expiry incorrect", err)
			}
			before := a.ExpiresAt()
			if err := c.Renew(context.Background(), a); err != nil || !a.ExpiresAt().Equal(before) {
				t.Fatal("renewal changed expiry", err)
			}
			inventory, err := c.ListAccounts(context.Background(), a)
			if err != nil || len(inventory.Accounts) != 2 || inventory.Synthetic != (env == Sandbox) || inventory.Environment != env || inventory.Provider != "etrade" {
				t.Fatal("inventory provenance invalid", err)
			}
			_, err = c.Exchange(context.Background(), p, "test-verifier")
			requireCode(t, err, financial.AuthorizationFailed)
			if calls != 4 {
				t.Fatal("reused approval performed a network request")
			}
		})
	}
}

func TestRequestAuthorizationIsConsumedExactlyOnceUnderConcurrency(t *testing.T) {
	var calls atomic.Int32
	c := fixture(t, Production, func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return response(503, "secret-provider-body"), nil
	})
	p := pending(c)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Exchange(context.Background(), p, "test-verifier"); err == nil {
				t.Error("failed provider exchange succeeded")
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 || p.token != "" || p.secret != "" {
		t.Fatal("request grant was retried or retained after consumption")
	}
}

func TestDailyExpiryAcrossDSTAndClockBoundaries(t *testing.T) {
	for _, tc := range []struct{ now, expiry string }{
		{"2026-03-08T05:30:00Z", "2026-03-09T04:00:00Z"},
		{"2026-11-01T04:30:00Z", "2026-11-02T05:00:00Z"},
		{"2026-09-12T03:59:59Z", "2026-09-12T04:00:00Z"},
	} {
		t.Run(tc.now, func(t *testing.T) {
			calls := 0
			c := fixture(t, Production, func(*http.Request) (*http.Response, error) {
				calls++
				return response(200, "oauth_token=test-access&oauth_token_secret=test-access-secret"), nil
			})
			now, _ := time.Parse(time.RFC3339, tc.now)
			c.now = func() time.Time { return now }
			a, err := c.Exchange(context.Background(), pending(c), "test-verifier")
			if err != nil || a.ExpiresAt().Format(time.RFC3339) != tc.expiry {
				t.Fatal("DST calendar expiry mismatch", err)
			}
			now = a.ExpiresAt()
			requireCode(t, c.Renew(context.Background(), a), financial.AuthorizationExpired)
			_, err = c.ListAccounts(context.Background(), a)
			requireCode(t, err, financial.AuthorizationExpired)
			now = a.issuedAt.Add(-time.Second)
			requireCode(t, c.Renew(context.Background(), a), financial.AuthorizationExpired)
			if calls != 1 {
				t.Fatal("expired/future authorization contacted provider")
			}
		})
	}
}

func TestEnvironmentKeyAndPendingExpiryBindings(t *testing.T) {
	c := fixture(t, Production, func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid authorization must not contact provider")
		return nil, errors.New("unexpected request")
	})
	other := fixture(t, Sandbox, nil)
	_, err := c.ListAccounts(context.Background(), access(other))
	requireCode(t, err, financial.AuthorizationFailed)
	_, err = c.Exchange(context.Background(), pending(other), "test-verifier")
	requireCode(t, err, financial.AuthorizationFailed)
	changed, _ := New(Config{Environment: Production, ConsumerKey: "other-consumer", ConsumerSecret: "other-secret"}, nil)
	changed.now = c.now
	_, err = c.ListAccounts(context.Background(), access(changed))
	requireCode(t, err, financial.AuthorizationFailed)
	p := pending(c)
	p.expiresAt = c.now()
	_, err = c.AuthorizationURL(p)
	requireCode(t, err, financial.AuthorizationExpired)
	_, err = c.Exchange(context.Background(), p, "test-verifier")
	requireCode(t, err, financial.AuthorizationExpired)
	_, err = c.ListAccounts(context.Background(), nil)
	requireCode(t, err, financial.AuthorizationFailed)
}

func TestMalformedTokensAndRenewalsFailClosed(t *testing.T) {
	for _, body := range []string{"", "oauth_token=a", "oauth_token=a&oauth_token_secret=b&oauth_token=c", "oauth_token=%XX&oauth_token_secret=b", "oauth_token=a&oauth_token_secret=%0A", "oauth_token=a&oauth_token_secret=b&oauth_callback_confirmed=maybe", "oauth_token=a&oauth_token_secret=b&oauth_callback_confirmed=true&oauth_callback_confirmed=false"} {
		c := fixture(t, Production, func(*http.Request) (*http.Response, error) { return response(200, body), nil })
		_, err := c.BeginAuthorization(context.Background())
		requireCode(t, err, financial.InvalidProviderResponse)
	}
	c := fixture(t, Production, func(*http.Request) (*http.Response, error) { return response(200, "not a renewal"), nil })
	requireCode(t, c.Renew(context.Background(), access(c)), financial.InvalidProviderResponse)
}

const inventoryJSON = `{"AccountListResponse":{"Accounts":{"Account":[
	{"accountId":"123456789","accountIdKey":"test-opaque-one","accountMode":"MARGIN","accountName":"private full number 123456789","accountType":"INDIVIDUAL","accountStatus":"ACTIVE"},
	{"accountId":"987654321","accountIdKey":"test-opaque-two","accountMode":"IRA","accountType":"ROTHIRA","accountStatus":"CLOSED"}
]}}}`

func TestAccountsPreserveIdentityWithoutFabricatingBalancesOrPermissions(t *testing.T) {
	c := fixture(t, Production, func(*http.Request) (*http.Response, error) { return response(200, inventoryJSON), nil })
	out, err := c.ListAccounts(context.Background(), access(c))
	if err != nil {
		t.Fatal(err)
	}
	first := out.Accounts[0]
	if first.ProviderAccountID != "test-opaque-one" || first.MaskedIdentifier != "••••6789" || first.Status != "active" || out.Accounts[1].Status != "closed" || first.AccountType != "INDIVIDUAL" {
		t.Fatal("account identity was not normalized correctly")
	}
	if first.BaseCurrency != "" || !first.LastSyncedAt.IsZero() || first.Capabilities["orders"] != financial.Unsupported || first.Capabilities["margin"] != financial.Unknown {
		t.Fatal("discovery inferred balances/freshness/permissions")
	}
	encoded, _ := json.Marshal(out)
	for _, private := range []string{"123456789", "987654321", "test-opaque", "private full number"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("private account data escaped JSON boundary")
		}
	}
}

func TestMalformedInventoriesNeverBecomeEmptyOrPartialSuccess(t *testing.T) {
	for _, body := range []string{"", "null", "{}", `{"AccountListResponse":{}}`, `{"AccountListResponse":{"Accounts":{"Account":null}}}`,
		inventoryJSON + `{}`, strings.Replace(inventoryJSON, "test-opaque-two", "test-opaque-one", 1),
		strings.Replace(inventoryJSON, "987654321", "123456789", 1), strings.Replace(inventoryJSON, "CLOSED", "UNKNOWN", 1),
		strings.Replace(inventoryJSON, `"accountId":"123456789"`, `"accountId":"123456789","AccountID":"different"`, 1),
		strings.Replace(inventoryJSON, "INDIVIDUAL", "123456789", 1), strings.Repeat(" ", maxBody+1),
	} {
		c := fixture(t, Production, func(*http.Request) (*http.Response, error) { return response(200, body), nil })
		out, err := c.ListAccounts(context.Background(), access(c))
		requireCode(t, err, financial.InvalidProviderResponse)
		if out.Accounts != nil {
			t.Fatal("partial account set escaped on failure")
		}
	}
	for _, tc := range []struct {
		code int
		body string
	}{{204, ""}, {200, `{"AccountListResponse":{"Accounts":{"Account":[]}}}`}} {
		c := fixture(t, Sandbox, func(*http.Request) (*http.Response, error) { return response(tc.code, tc.body), nil })
		out, err := c.ListAccounts(context.Background(), access(c))
		if err != nil || out.Accounts == nil || len(out.Accounts) != 0 || !out.Synthetic {
			t.Fatal("explicit empty inventory not preserved", err)
		}
	}
}

func TestErrorsAndSecretsRemainRedacted(t *testing.T) {
	for status, code := range map[int]financial.ProviderErrorCode{400: financial.AuthorizationFailed, 401: financial.AuthorizationFailed, 403: financial.PermissionDenied, 429: financial.RateLimited, 503: financial.ProviderUnavailable, 302: financial.InvalidProviderResponse} {
		c := fixture(t, Production, func(*http.Request) (*http.Response, error) {
			return response(status, "private-token-and-provider-body"), nil
		})
		_, err := c.ListAccounts(context.Background(), access(c))
		requireCode(t, err, code)
	}
	c := fixture(t, Production, func(*http.Request) (*http.Response, error) { return nil, errors.New("secret-in-network-error") })
	_, err := c.ListAccounts(context.Background(), access(c))
	requireCode(t, err, financial.ProviderUnavailable)
	for _, value := range []any{Config{ConsumerKey: "private-key", ConsumerSecret: "private-secret"}, c, pending(c), access(c)} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		printed := fmt.Sprintf("%v %+v %#v %s", value, value, value, encoded)
		for _, secret := range []string{"private-key", "private-secret", "test-consumer", "test-request", "test-access"} {
			if strings.Contains(printed, secret) {
				t.Fatal("credential escaped standard JSON/formatting")
			}
		}
	}
}

func TestClientCannotFollowRedirectsOrInheritCookies(t *testing.T) {
	var calls atomic.Int32
	sink := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer sink.Close()
	jar, _ := cookiejar.New(nil)
	h := &http.Client{Jar: jar, Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		res := response(302, "")
		res.Header.Set("Location", sink.URL)
		return res, nil
	})}
	c, err := New(Config{Environment: Production, ConsumerKey: "test-consumer", ConsumerSecret: "test-secret"}, h)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.BeginAuthorization(context.Background())
	requireCode(t, err, financial.InvalidProviderResponse)
	if calls.Load() != 0 || c.http.Jar != nil || c.http.Timeout != 10*time.Second || h.CheckRedirect != nil || h.Timeout != 0 || h.Jar != jar {
		t.Fatal("redirect/cookie/timeout controls missing or caller client mutated")
	}
}

func TestFoundationRemainsUnwiredAndNonExecuting(t *testing.T) {
	if financial.DefaultRegistry()["etrade"].Availability != financial.Planned {
		t.Fatal("unfinished connector was advertised as connected")
	}
	want := []string{"AuthorizationURL", "BeginAuthorization", "Exchange", "GoString", "ListAccounts", "Renew", "String"}
	typeOf := reflect.TypeOf((*Client)(nil))
	var actual []string
	for i := 0; i < typeOf.NumMethod(); i++ {
		actual = append(actual, typeOf.Method(i).Name)
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatal("unexpected adapter surface", actual)
	}
}

func TestConfigurationMustBeExplicitAndCredentialComplete(t *testing.T) {
	for _, cfg := range []Config{
		{}, {Environment: "unknown", ConsumerKey: "test-key", ConsumerSecret: "test-secret"},
		{Environment: Production, ConsumerKey: "test-key"}, {Environment: Sandbox, ConsumerSecret: "test-secret"},
		{Environment: Production, ConsumerKey: "test-key\n", ConsumerSecret: "test-secret"},
		{Environment: Production, ConsumerKey: "test-key", ConsumerSecret: strings.Repeat("a", 4097)},
	} {
		_, err := New(cfg, nil)
		requireCode(t, err, financial.InvalidCredentialFormat)
	}
}

func TestResponseCrossingExpiryCannotCreateOrReturnUsableAuthorization(t *testing.T) {
	now := time.Date(2026, 9, 12, 3, 59, 59, 0, time.UTC)
	c := fixture(t, Production, func(*http.Request) (*http.Response, error) {
		now = now.Add(2 * time.Second)
		return response(200, "oauth_token=test-access&oauth_token_secret=test-access-secret"), nil
	})
	c.now = func() time.Time { return now }
	a, err := c.Exchange(context.Background(), pending(c), "test-verifier")
	requireCode(t, err, financial.AuthorizationExpired)
	if a != nil {
		t.Fatal("midnight-crossing exchange returned an authorization")
	}
}

func TestTransportDeadlineIsClassifiedWithoutRawCause(t *testing.T) {
	c := fixture(t, Production, func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("private transport context: %w", context.DeadlineExceeded)
	})
	_, err := c.BeginAuthorization(context.Background())
	requireCode(t, err, financial.Timeout)
}

func TestMalformedAccessTokenConsumesPendingAuthorization(t *testing.T) {
	calls := 0
	c := fixture(t, Production, func(*http.Request) (*http.Response, error) {
		calls++
		return response(200, "oauth_token=test-access&oauth_token_secret="), nil
	})
	p := pending(c)
	_, err := c.Exchange(context.Background(), p, "test-verifier")
	requireCode(t, err, financial.InvalidProviderResponse)
	_, err = c.Exchange(context.Background(), p, "test-verifier")
	requireCode(t, err, financial.AuthorizationFailed)
	if calls != 1 {
		t.Fatal("malformed access response allowed a retry")
	}
}
