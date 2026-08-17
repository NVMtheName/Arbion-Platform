package schwab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

type Config struct {
	ClientID, ClientSecret, RedirectURI, AuthorizationURL, TokenURL, TraderBaseURL, MarketDataBaseURL string
}
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
		long, e := nonZero(p.LongQuantity)
		if e != nil {
			return nil, e
		}
		short, e := nonZero(p.ShortQuantity)
		if e != nil || long && short {
			return nil, &financial.ProviderError{Code: financial.InvalidProviderResponse}
		}
		if !long && !short {
			continue
		}
		q, dir := p.LongQuantity, "long"
		if short {
			q = p.ShortQuantity
			dir = "short"
		}
		out = append(out, financial.Position{AccountID: id, InstrumentType: p.Instrument.AssetType, Symbol: p.Instrument.Symbol, Quantity: financial.Decimal(q.String()), Direction: dir, MarketValue: money(p.MarketValue), CostBasis: money(p.AveragePrice), ProviderInstrumentID: p.Instrument.CUSIP})
	}
	return out, nil
}
func nonZero(n decimal) (bool, error) {
	if n == "" {
		return false, nil
	}
	v, ok := new(big.Rat).SetString(n.String())
	if !ok {
		return false, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	return v.Sign() != 0, nil
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

type quoteEnvelope struct {
	AssetMainType string `json:"assetMainType"`
	Symbol        string `json:"symbol"`
	Quote         struct {
		BidPrice, AskPrice, Mark, LastPrice decimal
		QuoteTime, TradeTime                int64
	} `json:"quote"`
}

func (c *Client) GetQuote(ctx context.Context, cr *financial.Credentials, symbol string) (financial.Quote, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return financial.Quote{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	var raw map[string]quoteEnvelope
	path := "/" + url.PathEscape(symbol) + "/quotes?fields=" + url.QueryEscape("quote,reference")
	if err := c.marketGet(ctx, cr, path, &raw); err != nil {
		return financial.Quote{}, err
	}
	value, ok := raw[symbol]
	if !ok {
		for key, candidate := range raw {
			if strings.EqualFold(key, symbol) {
				value, ok = candidate, true
				break
			}
		}
	}
	if !ok || !strings.EqualFold(value.Symbol, symbol) {
		return financial.Quote{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	bid, err := providerDecimal(value.Quote.BidPrice)
	if err != nil {
		return financial.Quote{}, err
	}
	ask, err := providerDecimal(value.Quote.AskPrice)
	if err != nil {
		return financial.Quote{}, err
	}
	mark, err := providerDecimal(value.Quote.Mark)
	if err != nil {
		return financial.Quote{}, err
	}
	last, err := providerDecimal(value.Quote.LastPrice)
	if err != nil {
		return financial.Quote{}, err
	}
	if bid == nil && ask == nil && mark == nil && last == nil {
		return financial.Quote{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	quoteTime := value.Quote.QuoteTime
	if quoteTime == 0 {
		quoteTime = value.Quote.TradeTime
	}
	return financial.Quote{Symbol: symbol, AssetType: value.AssetMainType, Bid: bid, Ask: ask, Mark: mark, Last: last, ProviderTimestamp: providerTime(quoteTime)}, nil
}

type rawOptionContract struct {
	PutCall         string  `json:"putCall"`
	Symbol          string  `json:"symbol"`
	BidPrice        decimal `json:"bidPrice"`
	AskPrice        decimal `json:"askPrice"`
	MarkPrice       decimal `json:"markPrice"`
	Volatility      decimal `json:"volatility"`
	Delta           decimal `json:"delta"`
	StrikePrice     decimal `json:"strikePrice"`
	ExpirationDate  string  `json:"expirationDate"`
	QuoteTimeInLong int64   `json:"quoteTimeInLong"`
	TradeTimeInLong int64   `json:"tradeTimeInLong"`
	OpenInterest    decimal `json:"openInterest"`
	TotalVolume     decimal `json:"totalVolume"`
	Multiplier      decimal `json:"multiplier"`
	IsMini          bool    `json:"isMini"`
	IsNonStandard   bool    `json:"isNonStandard"`
}

type rawOptionContracts []rawOptionContract

func (contracts *rawOptionContracts) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*contracts = nil
		return nil
	}
	if data[0] == '[' {
		return json.Unmarshal(data, (*[]rawOptionContract)(contracts))
	}
	if data[0] == '{' {
		var contract rawOptionContract
		if err := json.Unmarshal(data, &contract); err != nil {
			return err
		}
		*contracts = []rawOptionContract{contract}
		return nil
	}
	return errors.New("invalid option contract collection")
}

type rawOptionChain struct {
	Symbol          string  `json:"symbol"`
	Status          string  `json:"status"`
	UnderlyingPrice decimal `json:"underlyingPrice"`
	Underlying      struct {
		QuoteTime int64 `json:"quoteTime"`
		TradeTime int64 `json:"tradeTime"`
	} `json:"underlying"`
	CallExpDateMap map[string]map[string]rawOptionContracts `json:"callExpDateMap"`
	PutExpDateMap  map[string]map[string]rawOptionContracts `json:"putExpDateMap"`
}

func (c *Client) GetOptionChain(ctx context.Context, cr *financial.Credentials, request financial.OptionChainRequest) (financial.OptionChain, error) {
	request.Symbol = strings.ToUpper(strings.TrimSpace(request.Symbol))
	request.ContractType = strings.ToUpper(request.ContractType)
	if request.Symbol == "" || (request.ContractType != "PUT" && request.ContractType != "CALL") || request.StrikeCount < 1 || request.StrikeCount > 100 || request.FromDate.IsZero() || request.ToDate.Before(request.FromDate) || request.ToDate.Sub(request.FromDate) > 730*24*time.Hour {
		return financial.OptionChain{}, &financial.ProviderError{Code: financial.InvalidProviderResponse, Err: errors.New("invalid option chain request")}
	}
	query := url.Values{}
	query.Set("symbol", request.Symbol)
	query.Set("contractType", request.ContractType)
	query.Set("strikeCount", fmt.Sprintf("%d", request.StrikeCount))
	query.Set("includeUnderlyingQuote", "true")
	query.Set("strategy", "SINGLE")
	query.Set("fromDate", request.FromDate.Format("2006-01-02"))
	query.Set("toDate", request.ToDate.Format("2006-01-02"))
	var raw rawOptionChain
	if err := c.marketGet(ctx, cr, "/chains?"+query.Encode(), &raw); err != nil {
		return financial.OptionChain{}, err
	}
	if !strings.EqualFold(raw.Symbol, request.Symbol) || (raw.Status != "" && !strings.EqualFold(raw.Status, "SUCCESS")) {
		return financial.OptionChain{}, &financial.ProviderError{Code: financial.InvalidProviderResponse, Err: errors.New("option chain response identity or status mismatch")}
	}
	underlyingPrice, err := providerDecimal(raw.UnderlyingPrice)
	if err != nil {
		return financial.OptionChain{}, err
	}
	providerTimestamp := providerTime(raw.Underlying.QuoteTime)
	if providerTimestamp.IsZero() {
		providerTimestamp = providerTime(raw.Underlying.TradeTime)
	}
	selectedMap := raw.PutExpDateMap
	if request.ContractType == "CALL" {
		selectedMap = raw.CallExpDateMap
	}
	contracts := []financial.OptionContract{}
	for _, strikes := range selectedMap {
		for _, candidates := range strikes {
			for _, candidate := range candidates {
				normalized, ok, normalizeErr := normalizeOptionContract(request.Symbol, request.ContractType, candidate, providerTimestamp)
				if normalizeErr != nil {
					return financial.OptionChain{}, normalizeErr
				}
				if ok {
					contracts = append(contracts, normalized)
				}
			}
		}
	}
	return financial.OptionChain{Symbol: request.Symbol, UnderlyingPrice: underlyingPrice, ProviderTimestamp: providerTimestamp, Contracts: contracts}, nil
}

func normalizeOptionContract(underlying, contractType string, raw rawOptionContract, fallbackTime time.Time) (financial.OptionContract, bool, error) {
	multiplier, err := providerInt(raw.Multiplier)
	if err != nil {
		return financial.OptionContract{}, false, err
	}
	if raw.IsMini || raw.IsNonStandard || multiplier == nil || *multiplier != 100 || !strings.EqualFold(raw.PutCall, contractType) {
		return financial.OptionContract{}, false, nil
	}
	strike, err := providerDecimal(raw.StrikePrice)
	if err != nil || strike == nil {
		if err != nil {
			return financial.OptionContract{}, false, err
		}
		return financial.OptionContract{}, false, nil
	}
	strikeValue, strikeOK := new(big.Rat).SetString(string(*strike))
	if !strikeOK || strikeValue.Sign() <= 0 {
		return financial.OptionContract{}, false, nil
	}
	if _, err = time.Parse("2006-01-02", raw.ExpirationDate); err != nil {
		return financial.OptionContract{}, false, nil
	}
	bid, err := providerDecimal(raw.BidPrice)
	if err != nil {
		return financial.OptionContract{}, false, err
	}
	ask, err := providerDecimal(raw.AskPrice)
	if err != nil {
		return financial.OptionContract{}, false, err
	}
	mark, err := providerDecimal(raw.MarkPrice)
	if err != nil {
		return financial.OptionContract{}, false, err
	}
	delta, err := providerDecimal(raw.Delta)
	if err != nil {
		return financial.OptionContract{}, false, err
	}
	volatility, err := providerDecimal(raw.Volatility)
	if err != nil {
		return financial.OptionContract{}, false, err
	}
	openInterest, err := providerInt(raw.OpenInterest)
	if err != nil {
		return financial.OptionContract{}, false, err
	}
	volume, err := providerInt(raw.TotalVolume)
	if err != nil {
		return financial.OptionContract{}, false, err
	}
	timestamp := providerTime(raw.QuoteTimeInLong)
	if timestamp.IsZero() {
		timestamp = providerTime(raw.TradeTimeInLong)
	}
	if timestamp.IsZero() {
		timestamp = fallbackTime
	}
	return financial.OptionContract{Symbol: raw.Symbol, Underlying: underlying, PutCall: contractType, Expiration: raw.ExpirationDate, Strike: *strike, Bid: bid, Ask: ask, Mark: mark, Delta: delta, ImpliedVolatility: volatility, OpenInterest: openInterest, Volume: volume, ProviderTimestamp: timestamp}, true, nil
}

func providerDecimal(value decimal) (*financial.Decimal, error) {
	if value == "" {
		return nil, nil
	}
	if _, ok := new(big.Rat).SetString(value.String()); !ok {
		return nil, &financial.ProviderError{Code: financial.InvalidProviderResponse, Err: errors.New("provider decimal is malformed")}
	}
	result := financial.Decimal(value.String())
	return &result, nil
}

func providerInt(value decimal) (*int, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := value.Int64()
	if err != nil || parsed < 0 || int64(int(parsed)) != parsed {
		return nil, &financial.ProviderError{Code: financial.InvalidProviderResponse, Err: errors.New("provider integer is malformed")}
	}
	result := int(parsed)
	return &result, nil
}

func providerTime(milliseconds int64) time.Time {
	if milliseconds <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(milliseconds).UTC()
}

func (c *Client) marketGet(ctx context.Context, cr *financial.Credentials, path string, out any) error {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.cfg.MarketDataBaseURL, "/")+path, nil)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+cr.AccessToken)
	return c.do(req, out)
}

func (c *Client) get(ctx context.Context, cr *financial.Credentials, path string, out any) error {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.cfg.TraderBaseURL, "/")+path, nil)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+cr.AccessToken)
	return c.do(req, out)
}

var _ financial.MarketDataProvider = (*Client)(nil)

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
		return &financial.ProviderError{Code: financial.InvalidProviderResponse, Err: fmt.Errorf("decode provider response: %w", e)}
	}
	return nil
}
