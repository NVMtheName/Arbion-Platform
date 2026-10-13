package coinbase

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/financial"
)

// ScheduledMarketScope is immutable server composition, not a browser/model
// command or account authorization. FeeAllowanceUSD is an explicit all-in fee
// budget, not a provider fee estimate; final private preview remains mandatory.
type ScheduledMarketScope struct {
	OwnerID, AccountID, ProductID, FeeAllowanceUSD string
}

// ScheduledMarketCollector has no credentials, vault, signer, account API,
// preview or sender. It is deliberately separate from the financial Client.
// Production does not construct or schedule it until separately approved.
type ScheduledMarketCollector struct {
	scope ScheduledMarketScope
	base  url.URL
	http  *http.Client
	now   func() time.Time
}

var _ execution.ScheduledGenerationMarketCollector = (*ScheduledMarketCollector)(nil)

func validScheduledMarketScope(s ScheduledMarketScope) bool {
	base, usd := strings.CutSuffix(s.ProductID, "-USD")
	return executionUUID(s.OwnerID) && executionUUID(s.AccountID) && usd && base != "USD" &&
		previewSymbolPattern.MatchString(base) && previewDecimalPattern.MatchString(s.FeeAllowanceUSD)
}

// NewScheduledMarketCollector does no I/O and accepts no URL, transport, key,
// clock or runtime activation setting. The scope is copied by value.
func NewScheduledMarketCollector(scope ScheduledMarketScope) (*ScheduledMarketCollector, error) {
	if !validScheduledMarketScope(scope) {
		return nil, execution.ErrInvalid
	}
	c := newProductionExecutionClient()
	return &ScheduledMarketCollector{scope: scope, base: *c.base, http: c.http, now: c.now}, nil
}

func (c *ScheduledMarketCollector) CollectScheduledGenerationMarket(ctx context.Context, ownerID, accountID, productID string) (execution.ScheduledGenerationMarket, error) {
	fail := func(err error) (execution.ScheduledGenerationMarket, error) {
		return execution.ScheduledGenerationMarket{}, err
	}
	if c == nil || c.http == nil || c.now == nil || ctx == nil || !validScheduledMarketScope(c.scope) ||
		ownerID != c.scope.OwnerID || accountID != c.scope.AccountID || productID != c.scope.ProductID {
		return fail(execution.ErrNotAuthorized)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	started := c.now().UTC()
	var p executionProduct
	if err := c.publicMarketRequest(ctx, "/api/v3/brokerage/market/products/"+productID, &p); err != nil {
		return fail(err)
	}
	if p.ProductID == nil || *p.ProductID != productID || p.ProductType == nil || *p.ProductType != "SPOT" ||
		p.BaseCurrencyID == nil || *p.BaseCurrencyID != strings.TrimSuffix(productID, "-USD") || p.QuoteCurrencyID == nil || *p.QuoteCurrencyID != "USD" ||
		p.Status == nil || *p.Status != "online" || !requiredExecutionBools(p.IsDisabled, p.TradingDisabled, p.CancelOnly,
		p.LimitOnly, p.PostOnly, p.AuctionMode, p.ViewOnly) || *p.IsDisabled || *p.TradingDisabled || *p.CancelOnly || *p.PostOnly || *p.AuctionMode || *p.ViewOnly {
		return fail(invalidExecutionResponse())
	}
	for _, d := range []*executionDecimal{p.BaseIncrement, p.PriceIncrement, p.BaseMinSize, p.BaseMaxSize, p.QuoteMinSize, p.QuoteMaxSize} {
		if !executionPositive(d) {
			return fail(invalidExecutionResponse())
		}
	}
	var book executionBookResponse
	query := url.Values{"product_id": {productID}, "limit": {"1"}}
	if err := c.publicMarketRequest(ctx, "/api/v3/brokerage/market/product_book?"+query.Encode(), &book); err != nil {
		return fail(err)
	}
	b := book.Pricebook
	if b == nil || b.ProductID == nil || *b.ProductID != productID || b.Bids == nil || b.Asks == nil || len(*b.Bids) != 1 || len(*b.Asks) != 1 ||
		b.Time == nil || !executionPositive((*b.Bids)[0].Price) || !executionPositive((*b.Bids)[0].Size) ||
		!executionPositive((*b.Asks)[0].Price) || !executionPositive((*b.Asks)[0].Size) {
		return fail(invalidExecutionResponse())
	}
	m := execution.ScheduledGenerationMarket{
		ProductID: *p.ProductID, ProductType: *p.ProductType, BaseCurrency: *p.BaseCurrencyID, QuoteCurrency: *p.QuoteCurrencyID, Status: *p.Status,
		BaseIncrement: string(*p.BaseIncrement), PriceIncrement: string(*p.PriceIncrement), BaseMinSize: string(*p.BaseMinSize), BaseMaxSize: string(*p.BaseMaxSize),
		QuoteMinSize: string(*p.QuoteMinSize), QuoteMaxSize: string(*p.QuoteMaxSize), BestBid: string(*(*b.Bids)[0].Price), BestAsk: string(*(*b.Asks)[0].Price),
		FeeAllowanceUSD: c.scope.FeeAllowanceUSD, Disabled: *p.IsDisabled, TradingDisabled: *p.TradingDisabled, CancelOnly: *p.CancelOnly,
		PostOnly: *p.PostOnly, AuctionMode: *p.AuctionMode, ViewOnly: *p.ViewOnly, StartedAt: started, CompletedAt: c.now().UTC(), ObservedAt: b.Time.UTC(),
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if err := execution.ValidateScheduledGenerationMarket(productID, m, m.CompletedAt); err != nil {
		return fail(invalidExecutionResponse())
	}
	return m, nil
}

// Exact public GETs only. No private identifiers, credentials or cookies are
// supplied, and no cached snapshot, fallback source or retry is maintained.
func (c *ScheduledMarketCollector) publicMarketRequest(ctx context.Context, path string, output any) error {
	query := url.Values{"product_id": {c.scope.ProductID}, "limit": {"1"}}
	if path != "/api/v3/brokerage/market/products/"+c.scope.ProductID && path != "/api/v3/brokerage/market/product_book?"+query.Encode() {
		return invalidExecutionResponse()
	}
	u, err := c.base.Parse(path)
	if err != nil || u.Host != c.base.Host || u.Scheme != c.base.Scheme || u.User != nil || u.Fragment != "" {
		return invalidExecutionResponse()
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return invalidExecutionResponse()
	}
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Cache-Control", "no-cache")
	client := *c.http
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(r)
	if err != nil {
		var networkError net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
			return &financial.ProviderError{Code: financial.Timeout}
		}
		return &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	return decodeExecutionResponse(response, output)
}
