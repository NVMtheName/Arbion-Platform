package schwab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

func TestQuoteMetadataIsAllowlistedWithoutPromotingRealtime(t *testing.T) {
	for _, test := range []struct {
		name, fields, kind string
		flag               *bool
	}{
		{"fresh NFL", `"quoteType":"NFL","realtime":false,`, "NFL", func() *bool { v := false; return &v }()},
		{"NBBO unconfirmed", `"quoteType":"NBBO",`, "NBBO", nil},
		{"missing", "", "UNAVAILABLE", nil},
		{"untrusted", `"quoteType":"sensitive-unknown-field",`, "UNRECOGNIZED", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/SPY/quotes" {
					t.Error("unexpected provider action")
				}
				_, _ = w.Write([]byte(`{"SPY":{"symbol":"SPY","assetMainType":"EQUITY",` + test.fields + `"quote":{"bidPrice":600,"askPrice":601,"quoteTime":1789396196389}}}`))
			}))
			defer server.Close()
			client := New(Config{MarketDataBaseURL: server.URL}, server.Client())
			quote, err := client.GetQuote(context.Background(), &financial.Credentials{AccessToken: "fixture"}, "SPY")
			if err != nil || quote.QuoteType != test.kind || !quote.ProviderTimestamp.Equal(time.UnixMilli(1789396196389).UTC()) {
				t.Fatalf("metadata lost: %#v %v", quote, err)
			}
			if (quote.Realtime == nil) != (test.flag == nil) || (test.flag != nil && *quote.Realtime != *test.flag) {
				t.Fatal("provider flag inferred or changed")
			}
		})
	}
}
