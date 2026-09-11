// Package etrade contains an unwired OAuth 1.0a and account-discovery foundation.
// It has no order, transfer, quote, refresh-loop, persistence, or HTTP-route surface.
package etrade

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // Midnight Eastern must remain DST-correct in minimal images.

	"github.com/arbion/platform/services/api/internal/financial"
)

type Environment string

const (
	Production Environment = "production"
	Sandbox    Environment = "sandbox"
	maxBody                = 1 << 20
	requestTTL             = 5 * time.Minute
)

// Config secrets are server-side only. Neither JSON nor standard formatting
// exposes them. No arbitrary endpoint configuration is accepted.
type Config struct {
	Environment    Environment `json:"environment"`
	ConsumerKey    string      `json:"-"`
	ConsumerSecret string      `json:"-"`
}

func (Config) String() string     { return "E*TRADE configuration [redacted]" }
func (c Config) GoString() string { return c.String() }

type Client struct {
	environment Environment
	key, secret string
	binding     [32]byte
	http        *http.Client
	eastern     *time.Location
	now         func() time.Time
}

func (*Client) String() string     { return "E*TRADE client [redacted]" }
func (c *Client) GoString() string { return c.String() }

// New requires an explicit environment; sandbox and production share E*TRADE's
// authorization server but use distinct keys and data hosts. The HTTP client is
// copied: redirects and cookies are disabled, and timeout is bounded.
func New(cfg Config, h *http.Client) (*Client, error) {
	if cfg.Environment != Production && cfg.Environment != Sandbox || !validSecret(cfg.ConsumerKey) || !validSecret(cfg.ConsumerSecret) {
		return nil, failure(financial.InvalidCredentialFormat)
	}
	eastern, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, failure(financial.InternalError)
	}
	client := http.Client{Timeout: 10 * time.Second}
	if h != nil {
		client = *h
	}
	if client.Timeout <= 0 || client.Timeout > 10*time.Second {
		client.Timeout = 10 * time.Second
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{environment: cfg.Environment, key: cfg.ConsumerKey, secret: cfg.ConsumerSecret,
		binding: sha256.Sum256([]byte(string(cfg.Environment) + "\x00" + cfg.ConsumerKey + "\x00" + cfg.ConsumerSecret)),
		http:    &client, eastern: eastern, now: time.Now}, nil
}

// PendingAuthorization is opaque and one-use, even if exchange fails. A future
// control-plane integration must bind it to the initiating owner, store it only
// in encrypted/ephemeral server-side state, and consume that state atomically.
// It is deliberately not a financial.Credentials or an access authorization.
type PendingAuthorization struct {
	mu                  sync.Mutex
	used                bool
	token, secret       string
	binding             [32]byte
	issuedAt, expiresAt time.Time
	callbackConfirmed   bool
}

func (*PendingAuthorization) String() string     { return "E*TRADE pending authorization [redacted]" }
func (p *PendingAuthorization) GoString() string { return p.String() }

type PendingStatus struct {
	ExpiresAt         time.Time `json:"expires_at"`
	CallbackConfirmed bool      `json:"callback_confirmed"`
}

func (p *PendingAuthorization) Status() PendingStatus {
	if p == nil {
		return PendingStatus{}
	}
	return PendingStatus{ExpiresAt: p.expiresAt, CallbackConfirmed: p.callbackConfirmed}
}

// Authorization contains an access token, never a request token. Private fields
// prevent accidental browser/AI serialization. There is no persistence codec in
// this milestone; no consumer may infer that midnight expiry is renewable.
type Authorization struct {
	token, secret       string
	binding             [32]byte
	issuedAt, expiresAt time.Time
}

func (*Authorization) String() string     { return "E*TRADE authorization [redacted]" }
func (a *Authorization) GoString() string { return a.String() }

func (a *Authorization) ExpiresAt() time.Time {
	if a == nil {
		return time.Time{}
	}
	return a.expiresAt
}

func (c *Client) BeginAuthorization(ctx context.Context) (*PendingAuthorization, error) {
	now := c.now()
	body, _, err := c.get(ctx, "https://api.etrade.com/oauth/request_token", "", "", url.Values{"oauth_callback": {"oob"}}, "application/x-www-form-urlencoded")
	if err != nil {
		return nil, err
	}
	token, secret, values, err := parseToken(body)
	if err != nil {
		return nil, err
	}
	confirmed := strings.ToLower(values.Get("oauth_callback_confirmed"))
	if len(values["oauth_callback_confirmed"]) != 1 || confirmed != "true" && confirmed != "false" {
		return nil, failure(financial.InvalidProviderResponse)
	}
	if !c.now().Before(now.Add(requestTTL)) {
		return nil, failure(financial.AuthorizationExpired)
	}
	return &PendingAuthorization{token: token, secret: secret, binding: c.binding,
		issuedAt: now, expiresAt: now.Add(requestTTL), callbackConfirmed: confirmed == "true"}, nil
}

// AuthorizationURL is the only intentional token-in-URL surface, mandated by
// E*TRADE's browser consent flow. It contains no token secret/access token and
// must not be logged, sent to analytics, or persisted as public metadata.
func (c *Client) AuthorizationURL(p *PendingAuthorization) (string, error) {
	if p == nil {
		return "", failure(financial.AuthorizationFailed)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := c.pendingValid(p); err != nil {
		return "", err
	}
	return "https://us.etrade.com/e/t/etws/authorize?" + url.Values{"key": {c.key}, "token": {p.token}}.Encode(), nil
}

func (c *Client) pendingValid(p *PendingAuthorization) error {
	if p.used || p.binding != c.binding || !validSecret(p.token) || !validSecret(p.secret) {
		return failure(financial.AuthorizationFailed)
	}
	if now := c.now(); now.Before(p.issuedAt) || !now.Before(p.expiresAt) {
		return failure(financial.AuthorizationExpired)
	}
	return nil
}

func (c *Client) Exchange(ctx context.Context, p *PendingAuthorization, verifier string) (*Authorization, error) {
	if p == nil || !validSecret(verifier) {
		return nil, failure(financial.AuthorizationFailed)
	}
	p.mu.Lock()
	err := c.pendingValid(p)
	if err != nil {
		p.mu.Unlock()
		return nil, err
	}
	p.used = true
	token, secret := p.token, p.secret
	p.token, p.secret = "", ""
	p.mu.Unlock()
	now := c.now()
	body, _, err := c.get(ctx, "https://api.etrade.com/oauth/access_token", token, secret, url.Values{"oauth_verifier": {verifier}}, "application/x-www-form-urlencoded")
	if err != nil {
		return nil, err
	}
	access, accessSecret, _, err := parseToken(body)
	if err != nil {
		return nil, err
	}
	day := now.In(c.eastern)
	expires := time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, c.eastern).UTC()
	if !c.now().Before(expires) {
		return nil, failure(financial.AuthorizationExpired)
	}
	return &Authorization{token: access, secret: accessSecret, binding: c.binding, issuedAt: now, expiresAt: expires}, nil
}

func (c *Client) accessValid(a *Authorization) error {
	if a == nil || a.binding != c.binding || !validSecret(a.token) || !validSecret(a.secret) {
		return failure(financial.AuthorizationFailed)
	}
	if now := c.now(); now.Before(a.issuedAt) || !now.Before(a.expiresAt) {
		return failure(financial.AuthorizationExpired)
	}
	return nil
}

// Renew reactivates an inactive access token within the same Eastern calendar
// day. It does not extend expiry, exchange credentials, or retry any data call.
func (c *Client) Renew(ctx context.Context, a *Authorization) error {
	if err := c.accessValid(a); err != nil {
		return err
	}
	body, _, err := c.get(ctx, "https://api.etrade.com/oauth/renew_access_token", a.token, a.secret, nil, "text/plain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(body)) != "Access Token has been renewed" {
		return failure(financial.InvalidProviderResponse)
	}
	return c.accessValid(a)
}

func parseToken(body []byte) (string, string, url.Values, error) {
	values, err := url.ParseQuery(string(body))
	if err != nil || len(values["oauth_token"]) != 1 || len(values["oauth_token_secret"]) != 1 ||
		!validSecret(values.Get("oauth_token")) || !validSecret(values.Get("oauth_token_secret")) {
		return "", "", nil, failure(financial.InvalidProviderResponse)
	}
	return values.Get("oauth_token"), values.Get("oauth_token_secret"), values, nil
}

func validSecret(value string) bool {
	if value == "" || len(value) > 4096 {
		return false
	}
	for _, b := range []byte(value) {
		if b <= 32 || b >= 127 {
			return false
		}
	}
	return true
}

func failure(code financial.ProviderErrorCode) error { return &financial.ProviderError{Code: code} }

// get has a private fixed-endpoint caller surface. HTTP status/transport errors
// never retain a raw body, URL error, Authorization header, or provider message.
func (c *Client) get(ctx context.Context, endpoint, token, secret string, extra url.Values, accept string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, failure(financial.InternalError)
	}
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return nil, 0, failure(financial.InternalError)
	}
	header, err := authorizationHeader(req.URL, c.key, c.secret, token, secret, hex.EncodeToString(nonce[:]), c.now().Unix(), extra)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", header)
	req.Header.Set("Accept", accept)
	resp, err := c.http.Do(req)
	if err != nil {
		code := financial.ProviderUnavailable
		var networkError net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkError) && networkError.Timeout() {
			code = financial.Timeout
		}
		return nil, 0, failure(code)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		code := financial.InvalidProviderResponse
		switch {
		case resp.StatusCode == 401 || resp.StatusCode == 400:
			code = financial.AuthorizationFailed
		case resp.StatusCode == 403:
			code = financial.PermissionDenied
		case resp.StatusCode == 429:
			code = financial.RateLimited
		case resp.StatusCode >= 500:
			code = financial.ProviderUnavailable
		}
		return nil, resp.StatusCode, failure(code)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return nil, resp.StatusCode, failure(financial.InvalidProviderResponse)
	}
	return body, resp.StatusCode, nil
}
