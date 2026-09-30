package execution

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSavedPreflightRequiresCompleteCanonicalFields(t *testing.T) {
	p := ProviderPreflight{}
	body, _ := json.Marshal(p)
	var decoded ProviderPreflight
	if !decodeProviderPreflight(body, &decoded) {
		t.Fatal("complete stored shape rejected")
	}
	for _, name := range []string{"CanTransfer", "Disabled", "ViewOnly", "CashUSD", "StartedAt"} {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		delete(fields, name)
		incomplete, _ := json.Marshal(fields)
		if decodeProviderPreflight(incomplete, &decoded) {
			t.Fatal("missing field accepted", name)
		}
		fields[name] = json.RawMessage("null")
		incomplete, _ = json.Marshal(fields)
		if decodeProviderPreflight(incomplete, &decoded) {
			t.Fatal("null field accepted", name)
		}
	}
}

func providerPreflightFixture(o Order, portfolio string, now time.Time) ProviderPreflight {
	total := "60.6"
	if o.Request.Side == "SELL" {
		total = "59.4"
	}
	return ProviderPreflight{PortfolioID: portfolio, RequestDigest: o.RequestDigest, ProductID: "BTC-USD", ProductType: "SPOT", BaseCurrency: "BTC", QuoteCurrency: "USD", Status: "online",
		BaseIncrement: "0.00000001", PriceIncrement: "0.01", BaseMinSize: "0.00001", BaseMaxSize: "1000", QuoteMinSize: "1", QuoteMaxSize: "100000000",
		BestBid: "60000", BestAsk: "60000", PreviewID: o.Request.ClientOrderID, PreviewBaseSize: "0.001", PreviewQuoteSize: "60", PreviewFeeUSD: "0.6", PreviewTotalUSD: total, PreviewPrice: "60000",
		CashUSD: "1000", AvailableCashUSD: "1000", TotalBase: "1", AvailableBase: "1", CanView: true, CanTrade: true, StartedAt: now, CompletedAt: now, QuoteObservedAt: now}
}

func TestProviderPreflightIsExactFreshAndNonLeveraged(t *testing.T) {
	now := time.Now().UTC()
	for _, side := range []string{"BUY", "SELL"} {
		r := requestFixture()
		r.Side = side
		if side == "SELL" {
			r.MaximumDebitUSD = "0"
		}
		d, _ := requestDigest(r)
		o := Order{Request: r, RequestDigest: d, CreatedAt: now.Add(-time.Second)}
		p := providerPreflightFixture(o, r.AccountID, now)
		if err := validateProviderPreflight(p, o, r.AccountID, now); err != nil {
			t.Fatal(side, err)
		}
		for name, change := range map[string]func(*ProviderPreflight){
			"transfer": func(p *ProviderPreflight) { p.CanTransfer = true }, "no trade": func(p *ProviderPreflight) { p.CanTrade = false },
			"foreign portfolio": func(p *ProviderPreflight) { p.PortfolioID = r.OwnerID }, "wrong product": func(p *ProviderPreflight) { p.ProductID = "ETH-USD" },
			"derivative": func(p *ProviderPreflight) { p.ProductType = "FUTURE" }, "auction": func(p *ProviderPreflight) { p.AuctionMode = true },
			"disabled": func(p *ProviderPreflight) { p.TradingDisabled = true }, "missing preview": func(p *ProviderPreflight) { p.PreviewID = "" },
			"base precision": func(p *ProviderPreflight) { p.BaseIncrement = "0.002" }, "price precision": func(p *ProviderPreflight) { p.PriceIncrement = "7" },
			"min order": func(p *ProviderPreflight) { p.QuoteMinSize = "61" }, "max order": func(p *ProviderPreflight) { p.BaseMaxSize = "0.0005" },
			"changed size": func(p *ProviderPreflight) { p.PreviewBaseSize = "0.002" }, "fee overrun": func(p *ProviderPreflight) { p.PreviewFeeUSD = "0.600000000000000001" },
			"changed price": func(p *ProviderPreflight) { p.PreviewPrice = "70000" }, "inconsistent total": func(p *ProviderPreflight) { p.PreviewTotalUSD = "70" },
			"crossed book": func(p *ProviderPreflight) { p.BestBid = "60001" }, "stale book": func(p *ProviderPreflight) { p.QuoteObservedAt = now.Add(-11 * time.Second) },
			"future book": func(p *ProviderPreflight) { p.QuoteObservedAt = now.Add(time.Second) }, "stale collection": func(p *ProviderPreflight) { p.StartedAt = now.Add(-31 * time.Second) },
			"future completion": func(p *ProviderPreflight) { p.CompletedAt = now.Add(time.Second) }, "cash hold": func(p *ProviderPreflight) { p.AvailableCashUSD = "999" },
			"base hold": func(p *ProviderPreflight) { p.AvailableBase = "0.5" }, "non decimal": func(p *ProviderPreflight) { p.PreviewFeeUSD = "1/2" },
		} {
			t.Run(side+"/"+name, func(t *testing.T) {
				bad := p
				change(&bad)
				if validateProviderPreflight(bad, o, r.AccountID, now) == nil {
					t.Fatal("unsafe evidence accepted")
				}
			})
		}
	}
}
