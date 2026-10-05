package execution

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
)

// Every method panics: denied entry/scope tests must never read credentials,
// consume MFA, refresh provider evidence or attempt broker I/O.
type ownerNoIO struct{}

func (ownerNoIO) VerifyExecutionStepUp(context.Context, string, string) (string, time.Time, error) {
	panic("unexpected MFA access")
}
func (ownerNoIO) RetrieveFinancialVersion(context.Context, credential.Locator) ([]byte, int64, error) {
	panic("unexpected credential access")
}
func (ownerNoIO) CollectExecutionPreflight(context.Context, *financial.Credentials, Order, string) (ProviderPreflight, error) {
	panic("unexpected preview")
}
func (ownerNoIO) SubmitOnce(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
	panic("unexpected send")
}
func (ownerNoIO) LookupSubmission(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (SubmissionAcknowledgement, error) {
	panic("unexpected lookup")
}
func (ownerNoIO) CollectExecutionObservation(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (BrokerObservation, error) {
	panic("unexpected observation")
}
func (ownerNoIO) CancelOnce(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (CancellationAcknowledgement, error) {
	panic("unexpected cancellation")
}
func (ownerNoIO) CollectAccountSettlement(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (AccountSettlementEvidence, error) {
	panic("unexpected settlement")
}

func ownerNoIODependencies() OwnerWorkflowDependencies {
	p := ownerNoIO{}
	return OwnerWorkflowDependencies{p, p, p, p, p, p, p, p}
}

type ownerNoDatabase struct{}

func (ownerNoDatabase) Begin(context.Context) (pgx.Tx, error) { panic("unexpected database access") }
func (ownerNoDatabase) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("unexpected database access")
}

func ownerScopeFor(r Request) OwnerScope {
	return OwnerScope{OwnerID: r.OwnerID, AccountID: r.AccountID, ConnectionID: r.ConnectionID,
		CapitalBucketID: r.CapitalBucketID, ProductID: r.ProductID, PilotLimits: *r.PilotLimits}
}

func ownerCommands(w *OwnerWorkflow, p authorization.Principal, id string) map[string]func(context.Context) error {
	return map[string]func(context.Context) error{
		"get":       func(c context.Context) error { _, e := w.Get(c, p, id); return e },
		"approve":   func(c context.Context) error { _, e := w.Approve(c, p, id, OwnerApproveCommand{}); return e },
		"revoke":    func(c context.Context) error { _, e := w.Revoke(c, p, id); return e },
		"preflight": func(c context.Context) error { _, e := w.Capture(c, p, id); return e },
		"send":      func(c context.Context) error { _, e := w.Send(c, p, id, OwnerSendCommand{}); return e },
		"recover":   func(c context.Context) error { _, e := w.Recover(c, p, id); return e },
		"reconcile": func(c context.Context) error { _, e := w.Reconcile(c, p, id); return e },
		"cancel":    func(c context.Context) error { _, e := w.Cancel(c, p, id); return e },
		"settle":    func(c context.Context) error { _, e := w.Settle(c, p, id); return e },
	}
}

type ownerPresentationDatabase struct{ allowed bool }

func (d ownerPresentationDatabase) Begin(context.Context) (pgx.Tx, error) {
	panic("presentation attempted a write")
}
func (d ownerPresentationDatabase) QueryRow(context.Context, string, ...any) pgx.Row { return d }
func (d ownerPresentationDatabase) Scan(out ...any) error                            { *out[0].(*bool) = d.allowed; return nil }

func TestOwnerPresentationBindsEveryScopeFieldWithoutFinancialIO(t *testing.T) {
	r := requestFixture()
	scope := ownerScopeFor(r)
	p := authorization.Principal{UserID: r.OwnerID, Entitlement: authorization.EntitlementFounder}
	read := func(s OwnerScope, allowed bool) OwnerPresentation {
		t.Helper()
		w, err := NewOwnerWorkflow(NewPostgresStore(ownerPresentationDatabase{allowed}), s, ownerNoIODependencies())
		if err != nil {
			t.Fatal(err)
		}
		view, err := w.Presentation(context.Background(), p)
		if !allowed {
			if !errors.Is(err, ErrNotAuthorized) || view != (OwnerPresentation{}) {
				t.Fatal("inactive owner received context")
			}
		} else if s.OwnerID == p.UserID && err != nil {
			t.Fatal(err)
		}
		return view
	}
	first := read(scope, true)
	if first.ProductID != r.ProductID || first.AccountLabel != "Dedicated Coinbase portfolio" || len(first.ScopeID) != 64 || strings.Contains(first.ScopeID, r.AccountID) || first != read(scope, true) {
		t.Fatal("unstable or unsafe display context")
	}
	read(scope, false)
	for _, mutate := range []func(*OwnerScope){
		func(s *OwnerScope) { s.AccountID = r.ClientOrderID },
		func(s *OwnerScope) { s.ConnectionID = r.ClientOrderID },
		func(s *OwnerScope) { s.CapitalBucketID = r.ClientOrderID },
		func(s *OwnerScope) { s.ProductID = "ETH-USD" },
		func(s *OwnerScope) { s.PilotLimits.MaximumOrderUSD = "10" },
		func(s *OwnerScope) { s.PilotLimits.ExpiresAt = s.PilotLimits.ExpiresAt.Add(-time.Hour) },
	} {
		changed := scope
		mutate(&changed)
		if read(changed, true).ScopeID == first.ScopeID {
			t.Fatal("display binding ignored changed fixed scope")
		}
	}
	p.UserID = r.ClientOrderID
	scope.OwnerID = p.UserID
	if read(scope, true).ScopeID == first.ScopeID {
		t.Fatal("display binding ignored owner")
	}
}

func TestOwnerWorkflowRejectsForeignPrincipalBeforeAnyDependency(t *testing.T) {
	r := requestFixture()
	w, err := NewOwnerWorkflow(NewPostgresStore(ownerNoDatabase{}), ownerScopeFor(r), ownerNoIODependencies())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []authorization.Principal{{}, {UserID: r.AccountID, Entitlement: authorization.EntitlementFounder}, {UserID: r.OwnerID}} {
		commands := ownerCommands(w, p, r.ClientOrderID)
		commands["presentation"] = func(c context.Context) error { _, e := w.Presentation(c, p); return e }
		commands["prepare"] = func(c context.Context) error { _, e := w.Prepare(c, p, OwnerPrepareCommand{}); return e }
		for name, command := range commands {
			t.Run(name, func(t *testing.T) {
				if err := command(context.Background()); !errors.Is(err, ErrNotAuthorized) {
					t.Fatal("foreign principal accepted", err)
				}
			})
		}
	}
}

func TestOwnerWorkflowPrivateIdempotencyIdentity(t *testing.T) {
	r := requestFixture()
	id := ownerClientID(r.OwnerID, r.ClientOrderID)
	if !validUUID(id) || id == r.ClientOrderID || id != ownerClientID(r.OwnerID, r.ClientOrderID) ||
		id == ownerClientID(r.AccountID, r.ClientOrderID) || id == ownerClientID(r.OwnerID, r.AccountID) {
		t.Fatal("request key did not produce stable private owner-bound identity")
	}
	if _, err := NewOwnerWorkflow(NewPostgresStore(ownerNoDatabase{}), OwnerScope{}, ownerNoIODependencies()); !errors.Is(err, ErrNotAuthorized) {
		t.Fatal("unselected scope accepted")
	}
	if _, err := NewOwnerWorkflow(NewPostgresStore(ownerNoDatabase{}), ownerScopeFor(r), OwnerWorkflowDependencies{}); !errors.Is(err, ErrNotAuthorized) {
		t.Fatal("missing server dependencies accepted")
	}
}
