package execution

import (
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

// ProviderPreflight is private normalized read/preview evidence. It never
// contains credentials and is not a browser command or authority to send.
type ProviderPreflight struct {
	PortfolioID, RequestDigest, ProductID, ProductType, BaseCurrency, QuoteCurrency, Status                      string
	BaseIncrement, PriceIncrement, BaseMinSize, BaseMaxSize, QuoteMinSize, QuoteMaxSize                          string
	BestBid, BestAsk, PreviewID, PreviewBaseSize, PreviewQuoteSize, PreviewFeeUSD, PreviewTotalUSD, PreviewPrice string
	CashUSD, AvailableCashUSD, TotalBase, AvailableBase                                                          string
	CanView, CanTrade, CanTransfer                                                                               bool
	Disabled, TradingDisabled, CancelOnly, PostOnly, AuctionMode, ViewOnly                                       bool
	StartedAt, CompletedAt, QuoteObservedAt                                                                      time.Time
}

// ExecutionPreflightProvider performs only reads and exact non-executing
// previews BEFORE claim. The credential is retrieved server-side for this
// operation and never supplied by the browser/model.
type ExecutionPreflightProvider interface {
	CollectExecutionPreflight(context.Context, *financial.Credentials, Order, string) (ProviderPreflight, error)
}

func validateProviderPreflight(p ProviderPreflight, o Order, portfolio string, now time.Time) error {
	digest, err := requestDigest(o.Request)
	if err != nil || digest != o.RequestDigest || p.RequestDigest != digest || !validUUID(portfolio) || p.PortfolioID != portfolio ||
		!p.CanView || !p.CanTrade || p.CanTransfer || p.ProductID != o.Request.ProductID || p.ProductType != "SPOT" ||
		p.BaseCurrency != strings.TrimSuffix(o.Request.ProductID, "-USD") || p.QuoteCurrency != "USD" || p.Status != "online" ||
		p.Disabled || p.TradingDisabled || p.CancelOnly || p.PostOnly || p.AuctionMode || p.ViewOnly || !validUUID(p.PreviewID) {
		return ErrNotAuthorized
	}
	if p.StartedAt.Before(o.CreatedAt) || p.StartedAt.IsZero() || p.CompletedAt.Before(p.StartedAt) || p.CompletedAt.After(now) ||
		now.Sub(p.StartedAt) > 30*time.Second || p.QuoteObservedAt.IsZero() || p.QuoteObservedAt.After(p.CompletedAt) ||
		now.Sub(p.QuoteObservedAt) > 10*time.Second {
		return ErrNotAuthorized
	}
	values := []string{o.Request.BaseSize, o.Request.LimitPrice, p.BaseIncrement, p.PriceIncrement, p.BaseMinSize, p.BaseMaxSize,
		p.QuoteMinSize, p.QuoteMaxSize, p.BestBid, p.BestAsk, p.PreviewBaseSize, p.PreviewQuoteSize, p.PreviewPrice}
	n := make([]*big.Rat, len(values))
	for i, value := range values {
		var ok bool
		n[i], ok = decimal(value, true)
		if !ok {
			return ErrNotAuthorized
		}
	}
	qty, price, baseStep, priceStep := n[0], n[1], n[2], n[3]
	notional := new(big.Rat).Mul(qty, price)
	if !new(big.Rat).Quo(qty, baseStep).IsInt() || !new(big.Rat).Quo(price, priceStep).IsInt() ||
		n[4].Cmp(n[5]) > 0 || n[6].Cmp(n[7]) > 0 || qty.Cmp(n[4]) < 0 || qty.Cmp(n[5]) > 0 ||
		notional.Cmp(n[6]) < 0 || notional.Cmp(n[7]) > 0 || n[11].Cmp(n[6]) < 0 || n[11].Cmp(n[7]) > 0 || n[8].Cmp(n[9]) > 0 || n[10].Cmp(qty) != 0 {
		return ErrNotAuthorized
	}
	fee, fok := decimal(p.PreviewFeeUSD, false)
	total, tok := decimal(p.PreviewTotalUSD, true)
	allowance, aok := decimal(o.Request.FeeAllowanceUSD, false)
	if !fok || !tok || !aok || fee.Cmp(allowance) > 0 {
		return ErrNotAuthorized
	}
	// Preview estimates are not settlement. Require consistent gross/price and
	// a bounded total; never reinterpret a partial/changed size as owner approval.
	if new(big.Rat).Mul(qty, n[12]).Cmp(n[11]) != 0 {
		return ErrNotAuthorized
	}
	if o.Request.Side == "BUY" {
		debit, _ := decimal(o.Request.MaximumDebitUSD, true)
		if n[12].Cmp(price) > 0 || n[9].Cmp(price) > 0 || new(big.Rat).Add(n[11], fee).Cmp(total) != 0 || total.Cmp(debit) > 0 {
			return ErrNotAuthorized
		}
	} else {
		if n[12].Cmp(price) < 0 || n[8].Cmp(price) < 0 || new(big.Rat).Sub(n[11], fee).Cmp(total) != 0 {
			return ErrNotAuthorized
		}
	}
	cash, cok := decimal(p.CashUSD, false)
	available, avok := decimal(p.AvailableCashUSD, false)
	base, bok := decimal(p.TotalBase, false)
	free, frok := decimal(p.AvailableBase, false)
	// External holds are not managed by this initial isolated-account pilot.
	if !cok || !avok || !bok || !frok || cash.Cmp(available) != 0 || base.Cmp(free) != 0 {
		return ErrNotAuthorized
	}
	return nil
}
