package strategy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/risk"
)

type evaluationStoreFake struct {
	instance  Instance
	facts     EvaluationFacts
	commits   int
	commitErr error
}

func (f *evaluationStoreFake) Get(context.Context, string, string) (Instance, error) {
	return f.instance, nil
}
func (f *evaluationStoreFake) EvaluationFacts(context.Context, Instance, time.Time) (EvaluationFacts, error) {
	return f.facts, nil
}
func (f *evaluationStoreFake) CommitEvaluation(_ context.Context, _ Instance, _ int, _ Decision, _ risk.RiskEvaluation, _ ExecutionResult, _ time.Time) error {
	f.commits++
	return f.commitErr
}

type evaluationAutomationFake struct {
	mandate automation.Mandate
	bucket  automation.CapitalBucket
}

func (f *evaluationAutomationFake) Get(context.Context, authorization.Principal, string) (automation.Mandate, error) {
	return f.mandate, nil
}
func (f *evaluationAutomationFake) GetBucket(context.Context, authorization.Principal, string) (automation.CapitalBucket, error) {
	return f.bucket, nil
}

type evaluationFinancialFake struct {
	account       financial.FinancialAccount
	balances      financial.Balances
	positions     []financial.Position
	quoteCalls    int
	chainCalls    int
	balanceCalls  int
	positionCalls int
	timestamp     time.Time
}

func (f *evaluationFinancialFake) GetAccount(context.Context, authorization.Principal, string) (financial.FinancialAccount, error) {
	return f.account, nil
}
func (f *evaluationFinancialFake) GetBalances(context.Context, authorization.Principal, string) (financial.Balances, error) {
	f.balanceCalls++
	return f.balances, nil
}
func (f *evaluationFinancialFake) GetPositions(context.Context, authorization.Principal, string) ([]financial.Position, error) {
	f.positionCalls++
	return f.positions, nil
}
func (f *evaluationFinancialFake) GetQuote(_ context.Context, _ authorization.Principal, _ string, symbol string) (financial.Quote, error) {
	f.quoteCalls++
	bid, ask := financial.Decimal("199.90"), financial.Decimal("200.10")
	return financial.Quote{Symbol: symbol, Bid: &bid, Ask: &ask, ProviderTimestamp: f.timestamp}, nil
}
func (f *evaluationFinancialFake) GetOptionChain(_ context.Context, _ authorization.Principal, _ string, request financial.OptionChainRequest) (financial.OptionChain, error) {
	f.chainCalls++
	bid, ask, delta := financial.Decimal("1.25"), financial.Decimal("1.35"), financial.Decimal("-0.30")
	return financial.OptionChain{Symbol: request.Symbol, ProviderTimestamp: f.timestamp, Contracts: []financial.OptionContract{{Underlying: request.Symbol, PutCall: "PUT", Strike: "190", Expiration: "2026-01-31", Bid: &bid, Ask: &ask, Delta: &delta, ProviderTimestamp: f.timestamp}}}, nil
}

func evaluationFixture() (*EvaluationService, *evaluationStoreFake, *evaluationFinancialFake, authorization.Principal) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	identifier := "wheel"
	instance := Instance{ID: "instance", UserID: "user", AutomationMandateID: "mandate", FinancialAccountID: "account", CapitalBucketID: "bucket", StrategyIdentifier: identifier, MandateVersion: 2, ExecutionMode: Paper, CurrentState: ReadyForPut, StateVersion: 1, Status: "ACTIVE"}
	parameters := []byte(`{"symbols":["AAPL"],"minimum_dte":20,"maximum_dte":60,"target_delta":"0.30","target_delta_min":"0.20","target_delta_max":"0.40","maximum_contracts":1,"assignment_handling_policy":"continue_wheel"}`)
	mandate := automation.Mandate{ID: "mandate", UserID: "user", FinancialAccountID: "account", AutomationType: "STRATEGY", StrategyIdentifier: &identifier, CapitalBucketID: "bucket", AutonomyLevel: "RESEARCH_ONLY", ExecutionMode: "PAPER", Status: "READY", CurrentVersion: 2, StrategyParameters: parameters, OptionsAllowed: true, EffectiveFrom: now.Add(-time.Hour)}
	store := &evaluationStoreFake{instance: instance, facts: EvaluationFacts{Paper: &PaperEvaluationFacts{Cash: "1.0000000000", CurrentExposure: "0.0000000000", Positions: []Position{}, RiskPositions: []risk.Position{}}, Breakers: []risk.CircuitBreaker{}}}
	automations := &evaluationAutomationFake{mandate: mandate, bucket: automation.CapitalBucket{ID: "bucket", UserID: "user", FinancialAccountID: "account", AllocationType: "FIXED_AMOUNT", AllocationValue: "1.0000000000", ProtectedAmount: "0", Status: "ACTIVE"}}
	finances := &evaluationFinancialFake{account: financial.FinancialAccount{ID: "account", UserID: "user", Status: "active", BaseCurrency: "USD", Capabilities: financial.Capabilities{"options": financial.Unknown, "margin": financial.Unknown}}, timestamp: now}
	service := NewEvaluationService(store, automations, finances)
	service.now = func() time.Time { return now }
	return service, store, finances, authorization.Principal{UserID: "user", Entitlement: authorization.EntitlementFounder}
}

func TestManualPaperEvaluationUsesPaperCashAndRecordsRiskDenial(t *testing.T) {
	service, store, finances, principal := evaluationFixture()
	outcome, err := service.Evaluate(context.Background(), principal, "instance", "manual:one")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Execution.Status != RiskDenied || outcome.RiskDecision != risk.Deny || len(outcome.RiskReasonCodes) == 0 || outcome.RiskReasonCodes[0] != risk.AutonomyDenied {
		t.Fatalf("unexpected fail-closed outcome: %#v", outcome)
	}
	if store.commits != 1 || finances.quoteCalls != 1 || finances.chainCalls != 1 || finances.balanceCalls != 0 || finances.positionCalls != 0 {
		t.Fatalf("unexpected boundaries: commits=%d quote=%d chain=%d balances=%d positions=%d", store.commits, finances.quoteCalls, finances.chainCalls, finances.balanceCalls, finances.positionCalls)
	}
}

func TestManualEvaluationRequiresCurrentImmutableMandateVersion(t *testing.T) {
	service, store, finances, principal := evaluationFixture()
	service.automation.(*evaluationAutomationFake).mandate.CurrentVersion = 3
	_, err := service.Evaluate(context.Background(), principal, "instance", "manual:stale")
	if !errors.Is(err, ErrInvalid) || store.commits != 0 || finances.quoteCalls != 0 {
		t.Fatalf("stale mandate version was not rejected before provider reads: %v", err)
	}
}

func TestManualEvaluationRequiresImmutableCapitalBucketBinding(t *testing.T) {
	service, store, finances, principal := evaluationFixture()
	service.automation.(*evaluationAutomationFake).mandate.CapitalBucketID = "different-bucket"
	_, err := service.Evaluate(context.Background(), principal, "instance", "manual:different-bucket")
	if !errors.Is(err, ErrInvalid) || store.commits != 0 || finances.quoteCalls != 0 {
		t.Fatalf("changed capital bucket was not rejected before provider reads: %v", err)
	}
}

func TestManualEvaluationPropagatesAtomicDuplicate(t *testing.T) {
	service, store, _, principal := evaluationFixture()
	store.commitErr = ErrDuplicate
	_, err := service.Evaluate(context.Background(), principal, "instance", "manual:duplicate")
	if !errors.Is(err, ErrDuplicate) || store.commits != 1 {
		t.Fatalf("duplicate result was not preserved: %v", err)
	}
}

func TestManualEvaluationRejectsStaleMarketData(t *testing.T) {
	service, store, finances, principal := evaluationFixture()
	finances.timestamp = service.now().Add(-marketDataMaxAge - time.Second)
	_, err := service.Evaluate(context.Background(), principal, "instance", "manual:stale-market")
	if !errors.Is(err, ErrInvalid) || store.commits != 0 {
		t.Fatalf("stale market data was not rejected before persistence: %v", err)
	}
}
