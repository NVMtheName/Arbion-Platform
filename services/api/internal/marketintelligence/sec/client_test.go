package sec

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/marketintelligence"
)

func config(baseURL string) Config {
	return Config{UserAgent: "Arbion market intelligence admin@arbion.ai", BaseURL: baseURL, Timeout: time.Second, RateInterval: 100 * time.Millisecond, MaxFutureSkew: time.Second}
}

func TestRecentInsiderFilingsUsesDeclaredAgentAndPrimaryEvidence(t *testing.T) {
	accepted := time.Now().UTC().Add(-time.Minute)
	filingDate := accepted.Format("2006-01-02")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/submissions/CIK0000320193.json" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("User-Agent") != "Arbion market intelligence admin@arbion.ai" || request.Header.Get("Accept") != "application/json" {
			t.Fatal("SEC fair-access headers missing")
		}
		_, _ = writer.Write([]byte(`{"filings":{"recent":{
			"accessionNumber":["0001140361-26-032884","0000320193-26-000002","0001140361-26-025622"],
			"filingDate":["` + filingDate + `","` + filingDate + `","` + filingDate + `"],
			"reportDate":["` + filingDate + `","` + filingDate + `","` + filingDate + `"],
			"acceptanceDateTime":["` + accepted.Format(time.RFC3339Nano) + `","` + accepted.Format(time.RFC3339Nano) + `","` + accepted.Format(time.RFC3339Nano) + `"],
			"form":["4","10-K","4/A"],
			"primaryDocument":["xslF345X05/ownership.xml","annual.htm","ownership-amendment.xml"]
		}}}`))
	}))
	defer server.Close()

	client, err := New(config(server.URL), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	filings, err := client.RecentInsiderFilings(t.Context(), "320193", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(filings) != 2 || filings[0].Form != "4" || !filings[1].IsAmendment {
		t.Fatalf("ownership forms not preserved: %+v", filings)
	}
	if filings[0].IssuerCIK != "0000320193" || filings[0].SourceURL != "https://www.sec.gov/Archives/edgar/data/320193/000114036126032884/xslF345X05/ownership.xml" || filings[0].Provenance.Quality != marketintelligence.Filing {
		t.Fatalf("SEC evidence missing: %+v", filings[0])
	}
}

func TestNewEnforcesSECIdentityAndFairAccessRate(t *testing.T) {
	for _, test := range []Config{
		{},
		{UserAgent: "anonymous bot", BaseURL: "https://data.sec.gov", Timeout: time.Second, RateInterval: 100 * time.Millisecond},
		{UserAgent: "Arbion admin@arbion.ai", BaseURL: "https://example.com", Timeout: time.Second, RateInterval: 100 * time.Millisecond},
		{UserAgent: "Arbion admin@arbion.ai", BaseURL: "https://data.sec.gov", Timeout: time.Second, RateInterval: 99 * time.Millisecond},
	} {
		if _, err := New(test, http.DefaultClient); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("unsafe configuration accepted: %+v err=%v", test, err)
		}
	}
}

func TestRecentInsiderFilingsRejectsInvalidInputsAndSchemas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"filings":{"recent":{"accessionNumber":["0000320193-26-000001"],"filingDate":[],"reportDate":[],"acceptanceDateTime":[],"form":[],"primaryDocument":[]}}}`))
	}))
	defer server.Close()
	client, err := New(config(server.URL), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.RecentInsiderFilings(t.Context(), "not-a-cik", 10); !errors.Is(err, marketintelligence.ErrInvalidObservation) {
		t.Fatalf("invalid CIK accepted: %v", err)
	}
	if _, err = client.RecentInsiderFilings(t.Context(), "320193", 101); !errors.Is(err, marketintelligence.ErrInvalidObservation) {
		t.Fatalf("invalid limit accepted: %v", err)
	}
	if _, err = client.RecentInsiderFilings(t.Context(), "320193", 10); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("misaligned schema accepted: %v", err)
	}
}

func TestRecentInsiderFilingsClassifiesProviderFailuresWithoutBodies(t *testing.T) {
	for _, test := range []struct {
		status int
		want   error
	}{
		{http.StatusNotFound, ErrNotFound},
		{http.StatusForbidden, ErrRateLimited},
		{http.StatusTooManyRequests, ErrRateLimited},
		{http.StatusBadGateway, ErrUnavailable},
	} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(`{"secret":"body"}`))
			}))
			defer server.Close()
			client, err := New(config(server.URL), server.Client())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.RecentInsiderFilings(t.Context(), "320193", 10); !errors.Is(err, test.want) || strings.Contains(err.Error(), "body") {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestPacingCancellationDoesNotReserveARequestSlot(t *testing.T) {
	client, err := New(config("http://127.0.0.1:1"), http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.wait(context.Background()); err != nil {
		t.Fatal(err)
	}

	canceled, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err = client.wait(canceled); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("paced request did not honor cancellation: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	available, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if err = client.wait(available); err != nil {
		t.Fatalf("canceled request retained a future slot: %v", err)
	}
}
