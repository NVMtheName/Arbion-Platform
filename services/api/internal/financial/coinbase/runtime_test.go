package coinbase

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/jackc/pgx/v5"
)

type runtimeNoDatabase struct{}

func (runtimeNoDatabase) Begin(context.Context) (pgx.Tx, error) { panic("unexpected database I/O") }
func (runtimeNoDatabase) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("unexpected database I/O")
}

type runtimeNoBlobStore struct{}

func (runtimeNoBlobStore) Put(context.Context, credential.Locator, []byte, bool) error {
	panic("unexpected credential I/O")
}
func (runtimeNoBlobStore) Get(context.Context, credential.Locator) ([]byte, error) {
	panic("unexpected credential I/O")
}
func (runtimeNoBlobStore) Delete(context.Context, credential.Locator) error {
	panic("unexpected credential I/O")
}

func TestProductionOwnerWorkflowCompositionHasNoIOAndRejectsMissingDependencies(t *testing.T) {
	store := execution.NewPostgresStore(runtimeNoDatabase{})
	stepUp := auth.NewService(nil, nil, nil, nil, time.Hour)
	vault, err := credential.NewEncryptedVault(bytes.Repeat([]byte{17}, 32), runtimeNoBlobStore{})
	if err != nil {
		t.Fatal(err)
	}
	scope := execution.OwnerScope{
		OwnerID: "10000000-0000-4000-8000-000000000001", AccountID: "20000000-0000-4000-8000-000000000001",
		ConnectionID: "30000000-0000-4000-8000-000000000001", CapitalBucketID: "40000000-0000-4000-8000-000000000001", ProductID: "BTC-USD",
		PilotLimits: execution.OwnerPilotLimits{MaximumOrderUSD: "100", ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	if workflow, err := NewProductionOwnerWorkflow(store, scope, stepUp, vault); err != nil || workflow == nil {
		t.Fatalf("inert concrete composition failed: %v", err)
	}
	for _, test := range []struct {
		name   string
		store  *execution.PostgresStore
		scope  execution.OwnerScope
		stepUp *auth.Service
		vault  *credential.EncryptedVault
	}{
		{"store", nil, scope, stepUp, vault},
		{"database", execution.NewPostgresStore(nil), scope, stepUp, vault},
		{"step-up", store, scope, nil, vault},
		{"vault", store, scope, stepUp, nil},
		{"scope", store, execution.OwnerScope{}, stepUp, vault},
	} {
		t.Run(test.name, func(t *testing.T) {
			workflow, err := NewProductionOwnerWorkflow(test.store, test.scope, test.stepUp, test.vault)
			if workflow != nil || !errors.Is(err, execution.ErrNotAuthorized) {
				t.Fatalf("missing trusted dependency accepted: %v", err)
			}
		})
	}
}

func TestProductionExecutionTransportIgnoresDefaultsAndProxyEnvironment(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	oldTransport, oldClient := http.DefaultTransport, http.DefaultClient
	t.Cleanup(func() { http.DefaultTransport, http.DefaultClient = oldTransport, oldClient })
	trap := &submissionRetryTransport{}
	http.DefaultTransport = trap
	http.DefaultClient = &http.Client{Transport: trap}
	client := newProductionExecutionClient()
	if client.base.String() != "https://api.coinbase.com" || client.http == http.DefaultClient || client.http.Jar != nil || client.http.Timeout != 10*time.Second || client.now == nil {
		t.Fatal("runtime destination or client is not fixed and privately owned")
	}
	assertProductionTransport(t, client.http.Transport)
	if err := client.http.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatal("runtime allows redirects")
	}
	isolated, closeTransport, err := NewExecutionAdapter(client).isolatedClient()
	if err != nil {
		t.Fatal(err)
	}
	defer closeTransport()
	assertProductionTransport(t, isolated.http.Transport)
	if isolated.http.Transport == client.http.Transport || trap.calls != 0 {
		t.Fatal("execution operation inherited shared state or performed I/O")
	}
	other := newProductionExecutionClient()
	if other.http.Transport == client.http.Transport || other.base == client.base || other.http.Transport.(*http.Transport).TLSClientConfig == client.http.Transport.(*http.Transport).TLSClientConfig {
		t.Fatal("runtime instances share mutable transport state")
	}
}

func assertProductionTransport(t *testing.T, roundTripper http.RoundTripper) {
	t.Helper()
	transport, ok := roundTripper.(*http.Transport)
	if !ok || transport.Proxy != nil || transport.DialContext == nil || transport.Dial != nil || transport.DialTLS != nil || transport.DialTLSContext != nil ||
		!transport.DisableKeepAlives || !transport.DisableCompression || transport.ForceAttemptHTTP2 || len(transport.TLSNextProto) != 0 ||
		transport.Protocols == nil || !transport.Protocols.HTTP1() || transport.Protocols.HTTP2() || transport.Protocols.UnencryptedHTTP2() ||
		transport.TLSHandshakeTimeout != 5*time.Second || transport.ResponseHeaderTimeout != 5*time.Second || transport.MaxResponseHeaderBytes != 32<<10 {
		t.Fatal("runtime transport has an unsafe override, replay path, or unbounded handshake/headers")
	}
	cfg := transport.TLSClientConfig
	if cfg == nil || cfg.MinVersion != tls.VersionTLS12 || cfg.InsecureSkipVerify || cfg.ServerName != "api.coinbase.com" ||
		cfg.RootCAs != nil || cfg.VerifyConnection != nil || cfg.VerifyPeerCertificate != nil || cfg.GetClientCertificate != nil || len(cfg.Certificates) != 0 ||
		len(cfg.NextProtos) != 1 || cfg.NextProtos[0] != "http/1.1" {
		t.Fatal("runtime TLS trust or peer identity is configurable/insecure")
	}
}

func TestProductionExecutionTransportRejectsUntrustedAndWrongHostTLS(t *testing.T) {
	var handled atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { handled.Add(1) }))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	client := newProductionExecutionClient()
	// Only this private test addresses loopback directly. The production factory
	// exposes neither this client nor an alternate base/destination parameter.
	response, err := client.http.Get(server.URL)
	if response != nil {
		response.Body.Close()
	}
	if err == nil || handled.Load() != 0 {
		t.Fatal("runtime accepted an untrusted local certificate")
	}
	// Trust this fixture's certificate in the test only; the fixed Coinbase peer
	// name must still reject it. Never disable certificate verification.
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client.http.Transport.(*http.Transport).TLSClientConfig.RootCAs = roots
	response, err = client.http.Get(server.URL)
	if response != nil {
		response.Body.Close()
	}
	if err == nil || handled.Load() != 0 {
		t.Fatal("runtime accepted a trusted certificate for a different peer")
	}
}

func TestProductionExecutionTransportDoesNotFollowRedirectOrReplayLostResponse(t *testing.T) {
	var redirects, requests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirects.Add(1) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
			return
		}
		if r.ProtoMajor != 1 || !r.Close {
			t.Error("runtime used an HTTP/2 or reusable connection")
		}
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = connection.Close()
	}))
	defer server.Close()
	client := newProductionExecutionClient()
	response, err := client.http.Post(server.URL+"/redirect", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusTemporaryRedirect || redirects.Load() != 0 || requests.Load() != 1 {
		t.Fatal("runtime followed/replayed a redirect")
	}
	response, err = client.http.Post(server.URL+"/lost", "application/json", bytes.NewBufferString(`{}`))
	if response != nil {
		response.Body.Close()
	}
	if err == nil || requests.Load() != 2 {
		t.Fatal("runtime retried a request after response loss")
	}
}
