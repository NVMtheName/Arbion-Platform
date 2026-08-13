package schwab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

type Config struct{ ClientID, ClientSecret, RedirectURI, AuthorizationURL, TokenURL, TraderBaseURL string }
type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config, h *http.Client) *Client {
	if h == nil {
		h = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{cfg: cfg, http: h}
}
func (c *Client) AuthorizationURL(state string) (string, error) {
	u, e := url.Parse(c.cfg.AuthorizationURL)
	if e != nil {
		return "", e
	}
	q := u.Query()
	q.Set("client_id", c.cfg.ClientID)
	q.Set("redirect_uri", c.cfg.RedirectURI)
	q.Set("response_type", "code")
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func (c *Client) Exchange(ctx context.Context, code string) (financial.Credentials, error) {
	return c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {c.cfg.RedirectURI}})
}
func (c *Client) RefreshAuthorization(ctx context.Context, cr *financial.Credentials) error {
	fresh, e := c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {cr.RefreshToken}})
	if e != nil {
		return e
	}
	if fresh.RefreshToken == "" {
		fresh.RefreshToken = cr.RefreshToken
	}
	*cr = fresh
	return nil
}
func (c *Client) token(ctx context.Context, form url.Values) (financial.Credentials, error) {
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader(form.Encode()))
	if e != nil {
		return financial.Credentials{}, e
	}
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if e = c.do(req, &out); e != nil {
		return financial.Credentials{}, e
	}
	if out.AccessToken == "" {
		return financial.Credentials{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	return financial.Credentials{AccessToken: out.AccessToken, RefreshToken: out.RefreshToken, TokenType: out.TokenType, AccessExpiresAt: time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)}, nil
}
func (c *Client) VerifyConnection(ctx context.Context, cr *financial.Credentials) error {
	_, e := c.accountNumbers(ctx, cr)
	return e
}

type accountNumber struct {
	AccountNumber string `json:"accountNumber"`
	HashValue     string `json:"hashValue"`
}

func (c *Client) accountNumbers(ctx context.Context, cr *financial.Credentials) ([]accountNumber, error) {
	var v []accountNumber
	e := c.get(ctx, cr, "/accounts/accountNumbers", &v)
	return v, e
}
func mask(v string) string {
	if len(v) > 4 {
		return "••••" + v[len(v)-4:]
	}
	return "••••"
}
func (c *Client) ListAccounts(ctx context.Context, cr *financial.Credentials) ([]financial.FinancialAccount, error) {
	nums, e := c.accountNumbers(ctx, cr)
	if e != nil {
		return nil, e
	}
	out := make([]financial.FinancialAccount, 0, len(nums))
	for _, n := range nums {
		if n.HashValue == "" {
			return nil, &financial.ProviderError{Code: financial.InvalidProviderResponse}
		}
		out = append(out, financial.FinancialAccount{Provider: "schwab", ProviderAccountID: n.HashValue, DisplayName: "Schwab Brokerage " + mask(n.AccountNumber), MaskedIdentifier: mask(n.AccountNumber), AccountType: "brokerage", BaseCurrency: "USD", Status: "active", Capabilities: unknownCapabilities()})
	}
	return out, nil
}
func (c *Client) GetAccount(ctx context.Context, cr *financial.Credentials, id string) (financial.FinancialAccount, error) {
	var raw accountEnvelope
	e := c.get(ctx, cr, "/accounts/"+url.PathEscape(id), &raw)
	if e != nil {
		return financial.FinancialAccount{}, e
	}
	return financial.FinancialAccount{Provider: "schwab", ProviderAccountID: id, DisplayName: "Schwab " + raw.SecuritiesAccount.Type, AccountType: strings.ToLower(raw.SecuritiesAccount.Type), BaseCurrency: "USD", Status: "active", Capabilities: capabilities(raw.SecuritiesAccount.Type)}, nil
}

type decimal = json.Number
type accountEnvelope struct {
	SecuritiesAccount struct {
		Type            string `json:"type"`
		CurrentBalances struct {
			CashBalance      decimal `json:"cashBalance"`
			AvailableFunds   decimal `json:"availableFunds"`
			BuyingPower      decimal `json:"buyingPower"`
			LiquidationValue decimal `json:"liquidationValue"`
			Equity           decimal `json:"equity"`
			MarginBalance    decimal `json:"marginBalance"`
		} `json:"currentBalances"`
		Positions []struct {
			LongQuantity  decimal `json:"longQuantity"`
			ShortQuantity decimal `json:"shortQuantity"`
			MarketValue   decimal `json:"marketValue"`
			AveragePrice  decimal `json:"averagePrice"`
			Instrument    struct {
				AssetType string `json:"assetType"`
				Symbol    string `json:"symbol"`
				CUSIP     string `json:"cusip"`
			} `json:"instrument"`
		} `json:"positions"`
	} `json:"securitiesAccount"`
}

func money(n decimal) *financial.Money {
	if n == "" {
		return nil
	}
	return &financial.Money{Amount: financial.Decimal(n.String()), Currency: "USD"}
}
func (c *Client) fetch(ctx context.Context, cr *financial.Credentials, id string) (accountEnvelope, error) {
	var raw accountEnvelope
	e := c.get(ctx, cr, "/accounts/"+url.PathEscape(id)+"?fields=positions", &raw)
	return raw, e
}
func (c *Client) GetBalances(ctx context.Context, cr *financial.Credentials, id string) (financial.Balances, error) {
	r, e := c.fetch(ctx, cr, id)
	if e != nil {
		return financial.Balances{}, e
	}
	b := r.SecuritiesAccount.CurrentBalances
	return financial.Balances{Cash: money(b.CashBalance), AvailableCash: money(b.AvailableFunds), BuyingPower: money(b.BuyingPower), AccountValue: money(b.LiquidationValue), Equity: money(b.Equity)}, nil
}
func (c *Client) GetPositions(ctx context.Context, cr *financial.Credentials, id string) ([]financial.Position, error) {
	r, e := c.fetch(ctx, cr, id)
	if e != nil {
		return nil, e
	}
	out := make([]financial.Position, 0, len(r.SecuritiesAccount.Positions))
	for _, p := range r.SecuritiesAccount.Positions {
		q := p.LongQuantity
		dir := "long"
		if p.ShortQuantity != "" && p.ShortQuantity != "0" {
			q = p.ShortQuantity
			dir = "short"
		}
		out = append(out, financial.Position{AccountID: id, InstrumentType: p.Instrument.AssetType, Symbol: p.Instrument.Symbol, Quantity: financial.Decimal(q.String()), Direction: dir, MarketValue: money(p.MarketValue), CostBasis: money(p.AveragePrice), ProviderInstrumentID: p.Instrument.CUSIP})
	}
	return out, nil
}
func unknownCapabilities() financial.Capabilities {
	return financial.Capabilities{"equities": financial.Unknown, "options": financial.Unknown, "margin": financial.Unknown, "short_selling": financial.Unknown, "fractional_trading": financial.Unknown, "extended_hours": financial.Unknown, "market_data": financial.Unknown}
}
func capabilities(accountType string) financial.Capabilities {
	v := unknownCapabilities()
	if strings.EqualFold(accountType, "MARGIN") {
		v["margin"] = financial.Supported
	}
	return v
}
func (c *Client) GetCapabilities(ctx context.Context, cr *financial.Credentials, id string) (financial.Capabilities, error) {
	a, e := c.GetAccount(ctx, cr, id)
	return a.Capabilities, e
}
func (c *Client) Disconnect(context.Context, *financial.Credentials) error { return nil } // Schwab's portal does not document a safe API revocation endpoint; Arbion-side deletion is authoritative.
func (c *Client) get(ctx context.Context, cr *financial.Credentials, path string, out any) error {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.cfg.TraderBaseURL, "/")+path, nil)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+cr.AccessToken)
	return c.do(req, out)
}
func (c *Client) do(req *http.Request, out any) error {
	resp, e := c.http.Do(req)
	if e != nil {
		if errors.Is(e, context.DeadlineExceeded) {
			return &financial.ProviderError{Code: financial.Timeout}
		}
		return &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	defer resp.Body.Close()
	body, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if e != nil {
		return &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code := financial.ProviderUnavailable
		switch resp.StatusCode {
		case 400, 401:
			code = financial.AuthorizationExpired
		case 403:
			code = financial.PermissionDenied
		case 404:
			code = financial.AccountNotFound
		case 429:
			code = financial.RateLimited
		}
		return &financial.ProviderError{Code: code, Err: fmt.Errorf("provider status %d", resp.StatusCode)}
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if e = d.Decode(out); e != nil {
		return &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	return nil
}
