package financialconnection

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/financial"
)

type fillEvidenceStoreFake struct {
	connectionStoreFake
	calls         int
	failure       error
	user, account string
}

func (s *fillEvidenceStoreFake) CaptureFillEvidence(ctx context.Context, user, account string, _ financial.FillEvidencePage) error {
	s.calls++
	s.user, s.account = user, account
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("capture has no deadline")
	}
	return s.failure
}

func TestFillCaptureDoesNotInterruptReadsOrChangeAccountState(t *testing.T) {
	for name, failure := range map[string]error{"saved": nil, "unavailable": errors.New("private database failure"), "conflict": ErrFillEvidenceConflict} {
		t.Run(name, func(t *testing.T) {
			store := &fillEvidenceStoreFake{failure: failure}
			store.connection.Status = "active"
			audit := &reconciliationAuditFake{}
			s := NewService(store, nil, nil, nil, audit)
			page := financial.TradeFillPage{Provider: "coinbase", Fills: []financial.TradeFill{{ProductID: "BTC-USD"}}, Evidence: &financial.FillEvidencePage{}}
			s.captureFillPage(context.Background(), "owner-a", "account-a", &page)
			want := "SAVED"
			if failure != nil {
				want = "UNAVAILABLE"
			}
			if errors.Is(failure, ErrFillEvidenceConflict) {
				want = "CONFLICT"
			}
			if page.EvidenceCaptureStatus != want || len(page.Fills) != 1 || store.calls != 1 || store.user != "owner-a" || store.account != "account-a" || store.connection.Status != "active" {
				t.Fatal("read/account changed", page, store)
			}
			encoded, _ := json.Marshal(audit.metadata)
			if strings.Contains(string(encoded), "private database") {
				t.Fatal("private error escaped audit")
			}
		})
	}
	store := &fillEvidenceStoreFake{}
	s := NewService(store, nil, nil, nil, nil)
	page := financial.TradeFillPage{Provider: "coinbase"}
	s.captureFillPage(context.Background(), "a", "b", &page)
	if store.calls != 0 || page.EvidenceCaptureStatus != "UNAVAILABLE" {
		t.Fatal("missing identities treated as saved")
	}
}

func privateTestPage(providerAccount string, now time.Time) financial.FillEvidencePage {
	ref, _ := financial.CorrelationReference("coinbase", providerAccount, "account", providerAccount)
	entry, _ := financial.CorrelationReference("coinbase", providerAccount, "entry", "entry-one")
	trade, _ := financial.CorrelationReference("coinbase", providerAccount, "trade", "trade-one")
	order, _ := financial.CorrelationReference("coinbase", providerAccount, "order", "order-one")
	return financial.FillEvidencePage{Provider: "coinbase", Feed: "advanced_trade_fills", AccountReference: ref, ObservedAt: now, Fills: []financial.FillEvidence{{AccountReference: ref, EntryReference: entry, TradeReference: trade, OrderReference: order, SequenceTime: now.Add(-time.Minute), CommissionCurrencyStatus: "UNAVAILABLE", Fill: financial.TradeFill{ProductID: "BTC-USD", BaseAsset: "BTC", QuoteCurrency: "USD", Side: "BUY", Price: "60000.000", Size: "0.0000100", SizeUnit: "BTC", Commission: financial.Money{Amount: "0.01"}, TradeTime: now.Add(-2 * time.Minute), Liquidity: "MAKER"}}}}
}

type privateHistoryProviderFake struct{ coinbaseProviderFake }

func (p *privateHistoryProviderFake) GetTradeFills(ctx context.Context, credentials *financial.Credentials, account string, limit int) (financial.TradeFillPage, error) {
	page, err := p.coinbaseProviderFake.GetTradeFills(ctx, credentials, account, limit)
	evidence := privateTestPage(account, time.Now().UTC())
	page.Evidence = &evidence
	return page, err
}

func TestOwnerHistoryServiceCapturesOnceAndStripsPrivateEvidence(t *testing.T) {
	store := &fillEvidenceStoreFake{}
	provider := &privateHistoryProviderFake{}
	service := NewService(store, &vaultFake{}, nil, nil, nil, NamedProvider{ID: "coinbase", Provider: provider})
	if _, err := service.ConnectAPIKey(context.Background(), founder(), "coinbase", "organizations/org/apiKeys/key", "private-key"); err != nil {
		t.Fatal(err)
	}
	page, err := service.GetTradeFills(context.Background(), founder(), "account-1")
	if err != nil || page.Evidence != nil || page.EvidenceCaptureStatus != "SAVED" || store.calls != 1 || provider.fills != 1 || store.user != founder().UserID || store.account != "account-1" {
		t.Fatal("owner read lost capture or exposed private projection", page, err)
	}
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "entry_reference") || strings.Contains(string(encoded), "AccountReference") {
		t.Fatal("private identity escaped", string(encoded))
	}
	free := authorization.Principal{UserID: "free-user", Entitlement: authorization.EntitlementFree}
	if _, err := service.GetTradeFills(context.Background(), free, "account-1"); !errors.Is(err, ErrForbidden) || provider.fills != 1 || store.calls != 1 {
		t.Fatal("entitlement bypassed", err)
	}
}
