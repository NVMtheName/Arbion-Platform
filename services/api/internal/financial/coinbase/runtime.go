package coinbase

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/execution"
)

// NewProductionOwnerWorkflow composes the concrete owner workflow without any
// provider or credential I/O. Scope must come from reviewed server composition,
// not browser/model input. This constructor does not mount, activate, or schedule
// execution. Production startup remains disconnected until separately approved.
// Unlike New, it accepts no provider URL, HTTP client, transport, or clock.
func NewProductionOwnerWorkflow(store *execution.PostgresStore, scope execution.OwnerScope, stepUp *auth.Service, vault *credential.EncryptedVault) (*execution.OwnerWorkflow, error) {
	// Check concrete pointers before interface conversion: typed nil dependencies
	// must not become apparently present capabilities in OwnerWorkflowDependencies.
	if store == nil || stepUp == nil || vault == nil {
		return nil, execution.ErrNotAuthorized
	}
	client := newProductionExecutionClient()
	adapter := NewExecutionAdapter(client)
	return execution.NewOwnerWorkflow(store, scope, execution.OwnerWorkflowDependencies{
		StepUp: stepUp, Vault: vault, Preflight: client, Sender: adapter,
		Lookup: adapter, Observation: adapter, Cancellation: adapter, Settlement: adapter,
	})
}

// Own every transport setting. Cloning http.DefaultTransport or accepting the
// existing financial Client could inherit proxies, insecure TLS, custom dialers,
// or retry wrappers. Normal platform trust roots and hostname verification apply;
// no provider certificate pin or credential is embedded in the binary.
func newProductionExecutionClient() *Client {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	transport := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second, KeepAlive: -1,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12, ServerName: "api.coinbase.com", NextProtos: []string{"http/1.1"},
		},
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
		MaxResponseHeaderBytes: 32 << 10,
		DisableCompression:     true,
		DisableKeepAlives:      true,
		ForceAttemptHTTP2:      false,
		TLSNextProto:           map[string]func(string, *tls.Conn) http.RoundTripper{},
		Protocols:              protocols,
	}
	return &Client{
		base: &url.URL{Scheme: "https", Host: "api.coinbase.com"},
		now:  time.Now,
		http: &http.Client{
			Transport: transport, Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}
