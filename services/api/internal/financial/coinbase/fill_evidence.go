package coinbase

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

// privateFillPage is stricter than the historical display projection. Missing
// identity cannot make display facts disappear, but cannot enter the ledger.
func privateFillPage(raw fillPage, portfolio, account string, now time.Time) *financial.FillEvidencePage {
	accountRef, err := financial.CorrelationReference("coinbase", account, "account", account)
	if err != nil || raw.Fills == nil || raw.ProofTokenRequired {
		return nil
	}
	page := &financial.FillEvidencePage{Provider: "coinbase", Feed: "advanced_trade_fills", AccountReference: accountRef, ObservedAt: now, HasMore: raw.Cursor != "", Fills: []financial.FillEvidence{}}
	for _, r := range raw.Fills {
		if r.RetailPortfolioID != portfolio || r.TradeType != "FILL" || r.SizeInQuote == nil {
			return nil
		}
		entry, err := financial.CorrelationReference("coinbase", account, "entry", r.EntryID)
		if err != nil {
			return nil
		}
		trade, err := financial.CorrelationReference("coinbase", account, "trade", r.TradeID)
		if err != nil {
			return nil
		}
		order, err := financial.CorrelationReference("coinbase", account, "order", r.OrderID)
		if err != nil {
			return nil
		}
		product := strings.ToUpper(r.ProductID)
		parts := strings.Split(product, "-")
		if len(parts) != 2 {
			return nil
		}
		unit := parts[0]
		if *r.SizeInQuote {
			unit = parts[1]
		}
		liquidity, ok := normalizedLiquidity(r.LiquidityIndicator)
		if !ok {
			return nil
		}
		e, err := financial.NormalizeFillEvidence(financial.FillEvidence{AccountReference: accountRef, EntryReference: entry, TradeReference: trade, OrderReference: order, SequenceTime: r.SequenceTimestamp, CommissionCurrencyStatus: "UNAVAILABLE", Fill: financial.TradeFill{ProductID: product, BaseAsset: parts[0], QuoteCurrency: parts[1], Side: r.Side, Price: financial.Decimal(r.Price), Size: financial.Decimal(r.Size), SizeUnit: unit, Commission: financial.Money{Amount: financial.Decimal(r.Commission)}, TradeTime: r.TradeTime, Liquidity: liquidity}}, now)
		if err != nil {
			return nil
		}
		page.Fills = append(page.Fills, e)
	}
	return page
}

// ReadFillEvidence reads at most ten 100-row pages for a fixed <=24h window.
// It never returns a usable partial result on errors/conflicting identities.
// Cursor exhaustion is not proof of provider snapshot isolation/completeness.
// This method is not wired into the scheduler or the owner display read.
func (c *Client) ReadFillEvidence(ctx context.Context, credentials *financial.Credentials, id string, window financial.FillEvidenceWindow) (financial.FillEvidenceScan, error) {
	invalid := func() (financial.FillEvidenceScan, error) {
		return financial.FillEvidenceScan{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	now := c.now().UTC()
	if window.From.IsZero() || !window.To.After(window.From) || window.To.After(now) || window.To.Sub(window.From) > 24*time.Hour {
		return invalid()
	}
	if err := validateAccountID(credentials, id); err != nil {
		return financial.FillEvidenceScan{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := c.VerifyConnection(ctx, credentials); err != nil {
		return financial.FillEvidenceScan{}, err
	}
	accountRef, err := financial.CorrelationReference("coinbase", id, "account", id)
	if err != nil {
		return invalid()
	}
	scan := financial.FillEvidenceScan{Window: window, Page: financial.FillEvidencePage{Provider: "coinbase", Feed: "advanced_trade_fills", AccountReference: accountRef, ObservedAt: now, Fills: []financial.FillEvidence{}}}
	seen := map[string]string{}
	cursors := map[string]bool{}
	cursor := ""
	for i := 0; i < 10; i++ {
		query := url.Values{"limit": {"100"}, "product_types": {"SPOT"}, "start_sequence_timestamp": {window.From.UTC().Format(time.RFC3339Nano)}, "end_sequence_timestamp": {window.To.UTC().Format(time.RFC3339Nano)}}
		// Do not opt into PRICE/TRADE_TIME sorting: documented pagination is
		// unstable under those sorts. Fixed windows still aren't snapshots.
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var response fillPage
		if err := c.get(ctx, credentials, "/api/v3/brokerage/orders/historical/fills?"+query.Encode(), &response); err != nil {
			return financial.FillEvidenceScan{}, err
		}
		if response.ProofTokenRequired {
			return financial.FillEvidenceScan{}, &financial.ProviderError{Code: financial.PermissionDenied}
		}
		if len(response.Fills) > 100 || len(response.Cursor) > 1024 || !safeCursor(response.Cursor) || response.Cursor != strings.TrimSpace(response.Cursor) {
			return invalid()
		}
		page := privateFillPage(response, credentials.PortfolioID, id, now)
		if page == nil {
			return invalid()
		}
		for _, e := range page.Fills {
			if e.Fill.TradeTime.Before(window.From) || e.Fill.TradeTime.After(window.To) {
				return invalid()
			}
			digest, err := financial.FillEvidenceDigest(e, now)
			if err != nil {
				return invalid()
			}
			if prior, ok := seen[e.EntryReference]; ok {
				if prior != digest {
					return invalid()
				}
				continue
			}
			seen[e.EntryReference] = digest
			scan.Page.Fills = append(scan.Page.Fills, e)
		}
		scan.Pages++
		cursor = response.Cursor
		if cursor == "" {
			scan.PaginationExhausted = true
			break
		}
		if cursors[cursor] {
			return invalid()
		}
		cursors[cursor] = true
	}
	scan.Page.HasMore = !scan.PaginationExhausted
	sort.Slice(scan.Page.Fills, func(i, j int) bool {
		a, b := scan.Page.Fills[i], scan.Page.Fills[j]
		if a.Fill.TradeTime.Equal(b.Fill.TradeTime) {
			return a.EntryReference < b.EntryReference
		}
		return a.Fill.TradeTime.Before(b.Fill.TradeTime)
	})
	return scan, nil
}
