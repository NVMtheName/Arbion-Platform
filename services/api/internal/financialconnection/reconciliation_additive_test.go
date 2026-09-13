package financialconnection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

func additivePosition(symbol, total, available, unavailable string) ReconciliationPosition {
	a, u := financial.Decimal(available), financial.Decimal(unavailable)
	return ReconciliationPosition{Symbol: symbol, InstrumentType: "CRYPTO", Direction: "long", Quantity: financial.Decimal(total), AvailableQuantity: &a, UnavailableQuantity: &u, PerformanceStatus: "UNAVAILABLE"}
}

func TestAdditiveInventoryRequiresExactNondecreasingLongCryptoComponents(t *testing.T) {
	prior := additivePosition("USDC", "17807.60462", "260.75662", "17546.848")
	for _, test := range []struct {
		name   string
		before *ReconciliationPosition
		after  ReconciliationPosition
		want   bool
	}{
		{"observed USDC addition", &prior, additivePosition("USDC", "17813.66625", "266.13925", "17547.527"), true},
		{"new tiny BTC holding", nil, additivePosition("BTC", "0.00000202", "0.00000202", "0"), true},
		{"new unavailable holding", nil, additivePosition("ETH", "1", "0", "1"), true},
		{"withdrawal", &prior, additivePosition("USDC", "17806.60462", "259.75662", "17546.848"), false},
		{"availability redistribution", &prior, additivePosition("USDC", "17807.60462", "261.75662", "17545.848"), false},
		{"total up but unavailable reduced", &prior, additivePosition("USDC", "17808.60462", "262.75662", "17545.848"), false},
		{"total up but available reduced", &prior, additivePosition("USDC", "17808.60462", "259.75662", "17548.848"), false},
		{"inconsistent sum", &prior, additivePosition("USDC", "17813.66625", "266.13925", "17547.528"), false},
		{"negative component", nil, additivePosition("BTC", "1", "2", "-1"), false},
		{"invalid decimal", nil, additivePosition("BTC", "1e1", "10", "0"), false},
		{"zero appeared holding", nil, additivePosition("BTC", "0", "0", "0"), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := exactAdditiveInventoryChange(test.before, test.after); got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
	for _, field := range []string{"available", "unavailable", "short", "option", "prior sum"} {
		t.Run(field, func(t *testing.T) {
			before, after := prior, additivePosition("USDC", "17813.66625", "266.13925", "17547.527")
			switch field {
			case "available":
				after.AvailableQuantity = nil
			case "unavailable":
				before.UnavailableQuantity = nil
			case "short":
				after.Direction = "short"
			case "option":
				after.InstrumentType = "OPTION"
			case "prior sum":
				before.Quantity = "18000"
			}
			if exactAdditiveInventoryChange(&before, after) {
				t.Fatal("incomplete or unsafe evidence accepted")
			}
		})
	}
	for _, provider := range []string{"schwab", "unknown"} {
		changes := compareReconciliationPositions(provider, []ReconciliationPosition{prior}, []ReconciliationPosition{additivePosition("USDC", "17813.66625", "266.13925", "17547.527")})
		if len(changes) != 1 || changes[0].ControlImpact != reconciliationControlTradableInventory {
			t.Fatal("classification crossed provider boundary")
		}
	}
}

func TestAdditiveSnapshotRequiresSameAccountOrderedCompleteCashContext(t *testing.T) {
	now := time.Now().UTC()
	base := PortfolioReconciliation{FinancialAccountID: "account-1", Provider: "coinbase", BalancesStatus: "READY", PositionsStatus: "READY", ObservedAt: now}
	for _, test := range []struct {
		name   string
		mutate func(*PortfolioReconciliation, *PortfolioReconciliation)
	}{
		{"foreign account", func(_, current *PortfolioReconciliation) { current.FinancialAccountID = "account-2" }},
		{"foreign provider", func(_, current *PortfolioReconciliation) { current.Provider = "schwab" }},
		{"incomplete positions", func(prior, _ *PortfolioReconciliation) { prior.PositionsStatus = "UNAVAILABLE" }},
		{"incomplete balances", func(prior, _ *PortfolioReconciliation) { prior.BalancesStatus = "UNAVAILABLE" }},
		{"equal timestamps", func(prior, current *PortfolioReconciliation) { current.ObservedAt = prior.ObservedAt }},
		{"future prior", func(prior, current *PortfolioReconciliation) { prior.ObservedAt = current.ObservedAt.Add(time.Hour) }},
		{"missing prior time", func(prior, _ *PortfolioReconciliation) { prior.ObservedAt = time.Time{} }},
		{"cash decrease", func(prior, current *PortfolioReconciliation) {
			prior.Balances.Cash = &financial.Money{Amount: "25", Currency: "USD"}
			current.Balances.Cash = &financial.Money{Amount: "24", Currency: "USD"}
		}},
		{"missing current cash", func(prior, _ *PortfolioReconciliation) {
			prior.Balances.Cash = &financial.Money{Amount: "25", Currency: "USD"}
		}},
		{"available currency changed", func(prior, current *PortfolioReconciliation) {
			prior.Balances.AvailableCash = &financial.Money{Amount: "25", Currency: "USD"}
			current.Balances.AvailableCash = &financial.Money{Amount: "25", Currency: "EUR"}
		}},
		{"buying power reduced", func(prior, current *PortfolioReconciliation) {
			prior.Balances.BuyingPower = &financial.Money{Amount: "25", Currency: "USD"}
			current.Balances.BuyingPower = &financial.Money{Amount: "24", Currency: "USD"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			prior, current := base, base
			current.ObservedAt = now.Add(time.Hour)
			test.mutate(&prior, &current)
			if additiveSnapshotContext(prior, current) {
				t.Fatal("unsafe snapshot context accepted")
			}
		})
	}
	current := base
	current.ObservedAt = now.Add(time.Hour)
	base.Balances.AccountValue = &financial.Money{Amount: "1000", Currency: "USD"}
	current.Balances.AccountValue = &financial.Money{Amount: "900", Currency: "USD"}
	if !additiveSnapshotContext(base, current) {
		t.Fatal("market valuation loss was mistaken for inventory loss")
	}
}

func TestScheduledAdditionsRemainAutomaticWhileReductionAndExistingHoldsStayBlocked(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	position := additivePosition("USDC", "1000", "100", "900")
	provider := &coinbaseProviderFake{}
	setPosition := func(position ReconciliationPosition) {
		provider.positionData = []financial.Position{{Symbol: position.Symbol, InstrumentType: position.InstrumentType, Direction: position.Direction, Quantity: position.Quantity, AvailableQuantity: position.AvailableQuantity, UnavailableToTradeQuantity: position.UnavailableQuantity}}
	}
	setPosition(position)
	reports := &reconciliationStoreFake{}
	audit := &reconciliationAuditFake{}
	service := reconciliationServiceWithAudit(t, provider, reports, audit)
	if _, _, err := service.EnsureScheduledReconciliation(ctx, founder(), "account-1", now); err != nil {
		t.Fatal(err)
	}
	now = now.Add(30 * time.Minute)
	if _, _, err := service.EnsureScheduledReconciliation(ctx, founder(), "account-1", now); err != nil {
		t.Fatal(err)
	}
	for _, amount := range []struct{ total, available string }{{"1025", "125"}, {"1050", "150"}, {"1075", "175"}} {
		now = now.Add(12 * time.Hour)
		setPosition(additivePosition("USDC", amount.total, amount.available, "900"))
		id, review, err := service.EnsureScheduledReconciliation(ctx, founder(), "account-1", now)
		if err != nil || review {
			t.Fatalf("normal addition requires owner: %v", err)
		}
		latest := reports.items[len(reports.items)-1]
		if latest.ID != id || latest.ComparisonStatus != "MATCHED" || latest.BlocksNewActions || latest.BlockingChangeCount != 0 || latest.ChangeCount != 1 || latest.Changes[0].ControlImpact != reconciliationControlAdditiveOnly {
			t.Fatalf("addition not immutably recorded: %#v", latest)
		}
		if audit.metadata[len(audit.metadata)-1]["trigger"] != "SCHEDULED_FRESHNESS" {
			t.Fatal("automatic addition misrepresented as owner approval")
		}
	}
	// A reduction still holds, even if it is followed by an apparent restoration.
	now = now.Add(12 * time.Hour)
	setPosition(additivePosition("USDC", "1050", "150", "900"))
	driftID, review, err := service.EnsureScheduledReconciliation(ctx, founder(), "account-1", now)
	if err != nil || !review {
		t.Fatalf("reduction was not held: %v", err)
	}
	reads := provider.positions + provider.balances
	setPosition(additivePosition("USDC", "1100", "200", "900"))
	id, review, err := service.EnsureScheduledReconciliation(ctx, founder(), "account-1", now.Add(24*time.Hour))
	if err != nil || id != driftID || !review || reads != provider.positions+provider.balances {
		t.Fatal("addition silently cleared confirmed drift")
	}
	if _, err := service.RunReconciliation(ctx, founder(), "account-1"); !errors.Is(err, ErrReconciliationReviewRequired) {
		t.Fatal("owner review binding was bypassed")
	}
	if provider.orders != 0 || provider.fills != 0 || provider.previews != 0 || provider.disconnected != 0 {
		t.Fatal("reconciliation reached a broker action boundary")
	}
}

func TestAdditionsDoNotConcealDisappearedOrReducedPositions(t *testing.T) {
	prior := []ReconciliationPosition{additivePosition("USDC", "100", "100", "0"), additivePosition("BTC", "1", "1", "0")}
	current := []ReconciliationPosition{additivePosition("USDC", "200", "200", "0")}
	changes := compareReconciliationPositions("coinbase", prior, current)
	if len(changes) != 2 || changes[0].ControlImpact != reconciliationControlTradableInventory || changes[1].ControlImpact != reconciliationControlAdditiveOnly {
		t.Fatalf("addition masked disappeared inventory: %#v", changes)
	}
}
