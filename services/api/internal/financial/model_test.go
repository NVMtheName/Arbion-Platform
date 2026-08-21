package financial

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRegistryIsConservative(t *testing.T) {
	r := DefaultRegistry()
	if r["schwab"].Availability != Implemented || r["etrade"].Availability != Planned || r["coinbase"].Availability != Implemented {
		t.Fatal("unexpected availability")
	}
	if r["schwab"].Capabilities.Orders || r["schwab"].Capabilities.Options {
		t.Fatal("read-only registry must not claim execution")
	}
	if !r["schwab"].Capabilities.MarketData {
		t.Fatal("implemented read-only market data was not advertised")
	}
	if r["coinbase"].AuthType != JWTKeyPair || !r["coinbase"].Capabilities.AccountDiscovery || r["coinbase"].Capabilities.Orders {
		t.Fatal("Coinbase must advertise a read-only key-pair connection")
	}
}
func TestFinancialModelsHideOpaqueIdentifiers(t *testing.T) {
	b, _ := json.Marshal(FinancialAccount{ProviderAccountID: "secret-account"})
	if strings.Contains(string(b), "secret-account") {
		t.Fatal("opaque account id leaked")
	}
	p, _ := json.Marshal(Position{ProviderInstrumentID: "cusip"})
	if strings.Contains(string(p), "cusip") {
		t.Fatal("provider instrument id leaked")
	}
}
func TestDecimalPreservesPrecision(t *testing.T) {
	d := Decimal("1234567890.12345678901234567890")
	b, _ := json.Marshal(d)
	if string(b) != "\"1234567890.12345678901234567890\"" {
		t.Fatalf("precision changed: %s", b)
	}
}
