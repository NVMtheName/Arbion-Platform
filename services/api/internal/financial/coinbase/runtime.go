package coinbase

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/arbion/platform/services/api/internal/aiconnection"
	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/financialconnection"
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

// NewProductionScheduledWorkflow composes only existing reviewed boundaries.
// The AI service retains its own saved AI connection; the market collector has
// no vault and the financial adapters never receive the AI service. No I/O,
// scheduler, startup registration, commissioning or activation occurs here.
func NewProductionScheduledWorkflow(store *execution.PostgresStore, scope execution.ScheduledScope, feeAllowanceUSD string, stepUp *auth.Service, vault *credential.EncryptedVault, model *aiconnection.Service, refresh *financialconnection.Service) (*execution.ScheduledWorkflow, error) {
	if store == nil || stepUp == nil || vault == nil || model == nil || refresh == nil {
		return nil, execution.ErrNotAuthorized
	}
	market, err := NewScheduledMarketCollector(ScheduledMarketScope{OwnerID: scope.Owner.OwnerID, AccountID: scope.Owner.AccountID, ProductID: scope.Owner.ProductID, FeeAllowanceUSD: feeAllowanceUSD})
	if err != nil {
		return nil, err
	}
	client := newProductionExecutionClient()
	adapter := NewExecutionAdapter(client)
	return execution.NewScheduledWorkflow(store, scope, execution.OwnerWorkflowDependencies{
		StepUp: stepUp, Vault: vault, Preflight: client, Sender: adapter,
		Lookup: adapter, Observation: adapter, Cancellation: adapter, Settlement: adapter,
	}, market, model, refresh)
}

// NewProductionOwnerRuntime composes commissioning, owner controls and one
// worker from the same fixed terms. No independently supplied worker scope can
// drift from what the owner reviews. It neither reads credentials nor calls a
// provider, mounts routes, starts execution or chooses activation terms.
func NewProductionOwnerRuntime(store *execution.PostgresStore, terms *execution.OwnerCommissioningTerms, feeAllowanceUSD string, stepUp *auth.Service, vault *credential.EncryptedVault, model *aiconnection.Service, refresh *financialconnection.Service) (*execution.OwnerRuntime, error) {
	if terms == nil {
		return nil, nil
	}
	if store == nil || stepUp == nil || vault == nil || model == nil || refresh == nil {
		return nil, execution.ErrNotAuthorized
	}
	selected := *terms
	market, err := NewScheduledMarketCollector(ScheduledMarketScope{OwnerID: selected.Pilot.OwnerID,
		AccountID: selected.Pilot.AccountID, ProductID: selected.Pilot.ProductID, FeeAllowanceUSD: feeAllowanceUSD})
	if err != nil {
		return nil, err
	}
	client := newProductionExecutionClient()
	adapter := NewExecutionAdapter(client)
	return execution.NewOwnerRuntime(store, &selected, execution.OwnerWorkflowDependencies{
		StepUp: stepUp, Vault: vault, Preflight: client, Sender: adapter,
		Lookup: adapter, Observation: adapter, Cancellation: adapter, Settlement: adapter,
	}, market, model, refresh)
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
