package coinbase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/financial"
)

var (
	preflightUUIDPattern   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	preflightDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

var _ execution.ExecutionPreflightProvider = (*Client)(nil)

// These private wire types intentionally require the fields needed for this
// boundary. Missing/null booleans must never acquire a permissive zero value.
type executionPermissions struct {
	CanView       *bool   `json:"can_view"`
	CanTrade      *bool   `json:"can_trade"`
	CanTransfer   *bool   `json:"can_transfer"`
	PortfolioUUID *string `json:"portfolio_uuid"`
}

type executionDecimal string

func (d *executionDecimal) UnmarshalJSON(b []byte) error {
	if len(b) > 64 {
		return errors.New("invalid decimal")
	}
	s := string(b)
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
	}
	if !previewDecimalPattern.MatchString(s) {
		return errors.New("invalid decimal")
	}
	*d = executionDecimal(s)
	return nil
}

type executionMoney struct {
	Value    *executionDecimal `json:"value"`
	Currency *string           `json:"currency"`
}

type executionAccount struct {
	UUID              *string         `json:"uuid"`
	Currency          *string         `json:"currency"`
	AvailableBalance  *executionMoney `json:"available_balance"`
	Hold              *executionMoney `json:"hold"`
	Active            *bool           `json:"active"`
	Ready             *bool           `json:"ready"`
	RetailPortfolioID *string         `json:"retail_portfolio_id"`
}

type executionAccountPage struct {
	Accounts *[]executionAccount `json:"accounts"`
	HasNext  *bool               `json:"has_next"`
	Cursor   *string             `json:"cursor"`
}

type executionProduct struct {
	ProductID       *string           `json:"product_id"`
	ProductType     *string           `json:"product_type"`
	BaseCurrencyID  *string           `json:"base_currency_id"`
	QuoteCurrencyID *string           `json:"quote_currency_id"`
	Status          *string           `json:"status"`
	BaseIncrement   *executionDecimal `json:"base_increment"`
	PriceIncrement  *executionDecimal `json:"price_increment"`
	BaseMinSize     *executionDecimal `json:"base_min_size"`
	BaseMaxSize     *executionDecimal `json:"base_max_size"`
	QuoteMinSize    *executionDecimal `json:"quote_min_size"`
	QuoteMaxSize    *executionDecimal `json:"quote_max_size"`
	IsDisabled      *bool             `json:"is_disabled"`
	TradingDisabled *bool             `json:"trading_disabled"`
	CancelOnly      *bool             `json:"cancel_only"`
	LimitOnly       *bool             `json:"limit_only"`
	PostOnly        *bool             `json:"post_only"`
	AuctionMode     *bool             `json:"auction_mode"`
	ViewOnly        *bool             `json:"view_only"`
}

type executionBookLevel struct {
	Price *executionDecimal `json:"price"`
	Size  *executionDecimal `json:"size"`
}

type executionBookResponse struct {
	Pricebook *struct {
		ProductID *string               `json:"product_id"`
		Bids      *[]executionBookLevel `json:"bids"`
		Asks      *[]executionBookLevel `json:"asks"`
		Time      *time.Time            `json:"time"`
	} `json:"pricebook"`
}

type executionPreviewRequest struct {
	ProductID          string `json:"product_id"`
	Side               string `json:"side"`
	RetailPortfolioID  string `json:"retail_portfolio_id"`
	OrderConfiguration struct {
		LimitIOC struct {
			BaseSize   string `json:"base_size"`
			LimitPrice string `json:"limit_price"`
		} `json:"sor_limit_ioc"`
	} `json:"order_configuration"`
}

type executionPreviewResponse struct {
	PreviewID       *string           `json:"preview_id"`
	BaseSize        *executionDecimal `json:"base_size"`
	QuoteSize       *executionDecimal `json:"quote_size"`
	CommissionTotal *executionDecimal `json:"commission_total"`
	OrderTotal      *executionDecimal `json:"order_total"`
	AveragePrice    *executionDecimal `json:"est_average_filled_price"`
	Errors          *[]string         `json:"errs"`
	Warnings        *[]string         `json:"warning"`
	IsMax           *bool             `json:"is_max"`
	Leverage        json.RawMessage   `json:"leverage"`
	MarginRatio     json.RawMessage   `json:"margin_ratio"`
}

func invalidExecutionResponse() error {
	return &financial.ProviderError{Code: financial.InvalidProviderResponse}
}

func executionUUID(s string) bool {
	return preflightUUIDPattern.MatchString(s) && s != "00000000-0000-0000-0000-000000000000"
}

func executionPositive(d *executionDecimal) bool {
	return d != nil && positiveDecimal(financial.Decimal(*d))
}

func requiredExecutionBools(values ...*bool) bool {
	for _, v := range values {
		if v == nil {
			return false
		}
	}
	return true
}

func optionalExecutionDecimal(raw json.RawMessage, expected financial.Decimal) bool {
	if len(raw) == 0 {
		return true
	}
	var value executionDecimal
	return json.Unmarshal(raw, &value) == nil && compareDecimal(financial.Decimal(value), expected) == 0
}

// CollectExecutionPreflight is an unwired private read/preview collector, not
// execution authority. Its only POST is the non-executing preview endpoint.
// The control plane separately verifies exact economics, age, capital and the
// credential generation before saving this evidence and again when claiming.
func (c *Client) CollectExecutionPreflight(ctx context.Context, credentials *financial.Credentials, order execution.Order, portfolioID string) (execution.ProviderPreflight, error) {
	fail := func(err error) (execution.ProviderPreflight, error) { return execution.ProviderPreflight{}, err }
	r := order.Request
	base, usd := strings.CutSuffix(r.ProductID, "-USD")
	if credentials == nil || !executionUUID(portfolioID) || credentials.PortfolioID != portfolioID ||
		!preflightDigestPattern.MatchString(order.RequestDigest) || !usd || !previewSymbolPattern.MatchString(base) || base == "USD" ||
		(r.Side != "BUY" && r.Side != "SELL") || !previewDecimalPattern.MatchString(r.BaseSize) || !positiveDecimal(financial.Decimal(r.BaseSize)) ||
		!previewDecimalPattern.MatchString(r.LimitPrice) || !positiveDecimal(financial.Decimal(r.LimitPrice)) || !previewDecimalPattern.MatchString(r.FeeAllowanceUSD) {
		return fail(invalidExecutionResponse())
	}
	// Never mutate the caller's credential or rely on cached trade permission.
	key := *credentials
	if err := normalizeCredentials(&key); err != nil {
		return fail(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	p := execution.ProviderPreflight{PortfolioID: portfolioID, RequestDigest: order.RequestDigest, StartedAt: c.now().UTC()}
	var permissions executionPermissions
	if err := c.executionRequest(ctx, &key, http.MethodGet, "/api/v3/brokerage/key_permissions", nil, &permissions); err != nil {
		return fail(err)
	}
	if !requiredExecutionBools(permissions.CanView, permissions.CanTrade, permissions.CanTransfer) || permissions.PortfolioUUID == nil {
		return fail(invalidExecutionResponse())
	}
	if !*permissions.CanView || !*permissions.CanTrade || *permissions.CanTransfer || *permissions.PortfolioUUID != portfolioID {
		return fail(&financial.ProviderError{Code: financial.PermissionDenied})
	}
	p.CanView, p.CanTrade, p.CanTransfer = *permissions.CanView, *permissions.CanTrade, *permissions.CanTransfer
	if err := c.executionAccounts(ctx, &key, base, &p); err != nil {
		return fail(err)
	}
	var product executionProduct
	if err := c.executionRequest(ctx, &key, http.MethodGet, "/api/v3/brokerage/products/"+r.ProductID, nil, &product); err != nil {
		return fail(err)
	}
	if product.ProductID == nil || *product.ProductID != r.ProductID || product.ProductType == nil || *product.ProductType != "SPOT" ||
		product.BaseCurrencyID == nil || *product.BaseCurrencyID != base || product.QuoteCurrencyID == nil || *product.QuoteCurrencyID != "USD" ||
		product.Status == nil || *product.Status != "online" || !requiredExecutionBools(product.IsDisabled, product.TradingDisabled, product.CancelOnly,
		product.LimitOnly, product.PostOnly, product.AuctionMode, product.ViewOnly) || *product.IsDisabled || *product.TradingDisabled ||
		*product.CancelOnly || *product.PostOnly || *product.AuctionMode || *product.ViewOnly {
		return fail(invalidExecutionResponse())
	}
	for _, d := range []*executionDecimal{product.BaseIncrement, product.PriceIncrement, product.BaseMinSize, product.BaseMaxSize, product.QuoteMinSize, product.QuoteMaxSize} {
		if !executionPositive(d) {
			return fail(invalidExecutionResponse())
		}
	}
	p.ProductID, p.ProductType, p.BaseCurrency, p.QuoteCurrency, p.Status = *product.ProductID, *product.ProductType, *product.BaseCurrencyID, *product.QuoteCurrencyID, *product.Status
	p.BaseIncrement, p.PriceIncrement = string(*product.BaseIncrement), string(*product.PriceIncrement)
	p.BaseMinSize, p.BaseMaxSize, p.QuoteMinSize, p.QuoteMaxSize = string(*product.BaseMinSize), string(*product.BaseMaxSize), string(*product.QuoteMinSize), string(*product.QuoteMaxSize)
	p.Disabled, p.TradingDisabled, p.CancelOnly = *product.IsDisabled, *product.TradingDisabled, *product.CancelOnly
	p.PostOnly, p.AuctionMode, p.ViewOnly = *product.PostOnly, *product.AuctionMode, *product.ViewOnly
	var book executionBookResponse
	query := url.Values{"product_id": {r.ProductID}, "limit": {"1"}}
	if err := c.executionRequest(ctx, &key, http.MethodGet, "/api/v3/brokerage/product_book?"+query.Encode(), nil, &book); err != nil {
		return fail(err)
	}
	b := book.Pricebook
	if b == nil || b.ProductID == nil || *b.ProductID != r.ProductID || b.Bids == nil || b.Asks == nil || len(*b.Bids) != 1 || len(*b.Asks) != 1 ||
		b.Time == nil || b.Time.IsZero() || !executionPositive((*b.Bids)[0].Price) || !executionPositive((*b.Bids)[0].Size) ||
		!executionPositive((*b.Asks)[0].Price) || !executionPositive((*b.Asks)[0].Size) {
		return fail(invalidExecutionResponse())
	}
	p.BestBid, p.BestAsk, p.QuoteObservedAt = string(*(*b.Bids)[0].Price), string(*(*b.Asks)[0].Price), b.Time.UTC()
	if compareDecimal(financial.Decimal(p.BestBid), financial.Decimal(p.BestAsk)) > 0 || p.QuoteObservedAt.After(c.now()) || c.now().Sub(p.QuoteObservedAt) > 10*time.Second {
		return fail(invalidExecutionResponse())
	}
	request := executionPreviewRequest{ProductID: r.ProductID, Side: r.Side, RetailPortfolioID: portfolioID}
	request.OrderConfiguration.LimitIOC.BaseSize, request.OrderConfiguration.LimitIOC.LimitPrice = r.BaseSize, r.LimitPrice
	var preview executionPreviewResponse
	if err := c.executionRequest(ctx, &key, http.MethodPost, "/api/v3/brokerage/orders/preview", request, &preview); err != nil {
		return fail(err)
	}
	if preview.PreviewID == nil || !executionUUID(*preview.PreviewID) || preview.Errors == nil || len(*preview.Errors) != 0 ||
		preview.Warnings == nil || len(*preview.Warnings) != 0 || preview.IsMax == nil || *preview.IsMax ||
		!executionPositive(preview.BaseSize) || !executionPositive(preview.QuoteSize) || !executionPositive(preview.OrderTotal) ||
		!executionPositive(preview.AveragePrice) || preview.CommissionTotal == nil ||
		compareDecimal(financial.Decimal(*preview.BaseSize), financial.Decimal(r.BaseSize)) != 0 ||
		compareDecimal(financial.Decimal(*preview.CommissionTotal), financial.Decimal(r.FeeAllowanceUSD)) > 0 ||
		!optionalExecutionDecimal(preview.Leverage, "1") || !optionalExecutionDecimal(preview.MarginRatio, "0") {
		return fail(invalidExecutionResponse())
	}
	p.PreviewID, p.PreviewBaseSize, p.PreviewQuoteSize = *preview.PreviewID, string(*preview.BaseSize), string(*preview.QuoteSize)
	p.PreviewFeeUSD, p.PreviewTotalUSD, p.PreviewPrice = string(*preview.CommissionTotal), string(*preview.OrderTotal), string(*preview.AveragePrice)
	p.CompletedAt = c.now().UTC()
	if p.CompletedAt.Before(p.StartedAt) || p.CompletedAt.Sub(p.StartedAt) > 30*time.Second || p.QuoteObservedAt.After(p.CompletedAt) || p.CompletedAt.Sub(p.QuoteObservedAt) > 10*time.Second {
		return fail(invalidExecutionResponse())
	}
	return p, nil
}

func (c *Client) executionAccounts(ctx context.Context, credentials *financial.Credentials, base string, evidence *execution.ProviderPreflight) error {
	cash, inventory := new(big.Rat), new(big.Rat)
	seenAccounts, seenCursors := map[string]bool{}, map[string]bool{}
	cursor := ""
	for page := 0; page < maxPages; page++ {
		query := url.Values{"limit": {"250"}, "retail_portfolio_id": {evidence.PortfolioID}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var response executionAccountPage
		if err := c.executionRequest(ctx, credentials, http.MethodGet, "/api/v3/brokerage/accounts?"+query.Encode(), nil, &response); err != nil {
			return err
		}
		if response.Accounts == nil || response.HasNext == nil || response.Cursor == nil || len(*response.Accounts) > 250 ||
			len(*response.Cursor) > 1024 || !safeCursor(*response.Cursor) || strings.TrimSpace(*response.Cursor) != *response.Cursor {
			return invalidExecutionResponse()
		}
		for _, a := range *response.Accounts {
			if a.UUID == nil || !executionUUID(*a.UUID) || seenAccounts[*a.UUID] || a.Currency == nil || !currencyPattern.MatchString(*a.Currency) ||
				a.RetailPortfolioID == nil || *a.RetailPortfolioID != evidence.PortfolioID || !requiredExecutionBools(a.Active, a.Ready) || !*a.Active || !*a.Ready ||
				a.AvailableBalance == nil || a.AvailableBalance.Value == nil || a.AvailableBalance.Currency == nil || *a.AvailableBalance.Currency != *a.Currency ||
				a.Hold == nil || a.Hold.Value == nil || a.Hold.Currency == nil || *a.Hold.Currency != *a.Currency {
				return invalidExecutionResponse()
			}
			seenAccounts[*a.UUID] = true
			available, ok := new(big.Rat).SetString(string(*a.AvailableBalance.Value))
			hold, holdOK := new(big.Rat).SetString(string(*a.Hold.Value))
			if !ok || !holdOK || hold.Sign() != 0 {
				return invalidExecutionResponse()
			}
			switch *a.Currency {
			case "USD":
				cash.Add(cash, available)
			case base:
				inventory.Add(inventory, available)
			default:
				if available.Sign() != 0 {
					return invalidExecutionResponse()
				}
			}
		}
		if !*response.HasNext {
			if len(seenAccounts) == 0 {
				return invalidExecutionResponse()
			}
			// Holds were explicitly zero for every account. Preserve separate
			// total/available fields; do not substitute a portfolio valuation.
			evidence.CashUSD, evidence.AvailableCashUSD = executionAmount(cash), executionAmount(cash)
			evidence.TotalBase, evidence.AvailableBase = executionAmount(inventory), executionAmount(inventory)
			if !previewDecimalPattern.MatchString(evidence.CashUSD) || !previewDecimalPattern.MatchString(evidence.TotalBase) {
				return invalidExecutionResponse()
			}
			return nil
		}
		next := *response.Cursor
		if next == "" || seenCursors[next] || len(*response.Accounts) == 0 {
			return invalidExecutionResponse()
		}
		seenCursors[next], cursor = true, next
	}
	return invalidExecutionResponse()
}

func executionAmount(n *big.Rat) string {
	return strings.TrimRight(strings.TrimRight(n.FloatString(18), "0"), ".")
}

// executionRequest reuses the client's JWT and transport, not its permissive
// display decoder or redirect policy. It has no retry and allows no send/cancel
// endpoint. A bounded whole-body read catches oversized trailing whitespace.
func (c *Client) executionRequest(ctx context.Context, credentials *financial.Credentials, method, path string, input, output any) error {
	u, err := c.base.Parse(path)
	if err != nil || u.Host != c.base.Host || u.Scheme != c.base.Scheme || u.User != nil || u.Fragment != "" ||
		(method != http.MethodGet && method != http.MethodPost) || (method == http.MethodPost && (u.Path != "/api/v3/brokerage/orders/preview" || input == nil)) ||
		(method == http.MethodGet && input != nil) {
		return invalidExecutionResponse()
	}
	if method == http.MethodGet && u.Path != "/api/v3/brokerage/key_permissions" && u.Path != "/api/v3/brokerage/accounts" &&
		u.Path != "/api/v3/brokerage/product_book" && !strings.HasPrefix(u.Path, "/api/v3/brokerage/products/") {
		return invalidExecutionResponse()
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil || len(encoded) > 16*1024 {
			return invalidExecutionResponse()
		}
		body = bytes.NewReader(encoded)
	}
	token, err := c.jwt(credentials, method, u.Path)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return invalidExecutionResponse()
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := *c.http
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		var networkError net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
			return &financial.ProviderError{Code: financial.Timeout}
		}
		return &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		code := financial.InvalidProviderResponse
		switch response.StatusCode {
		case http.StatusUnauthorized:
			code = financial.AuthorizationFailed
		case http.StatusForbidden:
			code = financial.PermissionDenied
		case http.StatusTooManyRequests:
			code = financial.RateLimited
		default:
			if response.StatusCode >= 500 {
				code = financial.ProviderUnavailable
			}
		}
		return &financial.ProviderError{Code: code}
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes+1))
	if err != nil || len(encoded) > maxBodyBytes || !executionUniqueJSON(encoded) || json.Unmarshal(encoded, output) != nil {
		return invalidExecutionResponse()
	}
	return nil
}

// Check nested keys before struct decoding: encoding/json otherwise accepts
// last-key-wins (including case aliases). Depth and token counts are bounded.
func executionUniqueJSON(body []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	count := 0
	var value func(int) bool
	value = func(depth int) bool {
		count++
		if depth > 32 || count > 50000 {
			return false
		}
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		delimiter, nested := token.(json.Delim)
		if !nested {
			return true
		}
		switch delimiter {
		case '{':
			keys := map[string]bool{}
			for decoder.More() {
				token, err := decoder.Token()
				key, ok := token.(string)
				// encoding/json folds Unicode (including long-s) when matching
				// struct fields. Contract keys are ASCII; reject aliases before
				// any decoder can overwrite a security-relevant field.
				for _, r := range key {
					if r > 0x7f {
						return false
					}
				}
				key = strings.ToLower(key)
				if err != nil || !ok || keys[key] || !value(depth+1) {
					return false
				}
				keys[key] = true
			}
			token, err = decoder.Token()
			return err == nil && token == json.Delim('}')
		case '[':
			for decoder.More() {
				if !value(depth + 1) {
					return false
				}
			}
			token, err = decoder.Token()
			return err == nil && token == json.Delim(']')
		default:
			return false
		}
	}
	if !value(0) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}
