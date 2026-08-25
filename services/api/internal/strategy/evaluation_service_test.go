package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/marketintelligence"
	"github.com/arbion/platform/services/api/internal/neural"
	"github.com/arbion/platform/services/api/internal/risk"
)

type evaluationStoreFake struct {
	instance  Instance
	facts     EvaluationFacts
	commits   int
	commitErr error
	abstains  int
}

func (f *evaluationStoreFake) CommitAIAbstention(_ context.Context, _ Instance, _ string, _ json.RawMessage, _ time.Time) error {
	f.abstains++
	return f.commitErr
}

func (f *evaluationStoreFake) Get(context.Context, string, string) (Instance, error) {
	return f.instance, nil
}

type evaluationAIFake struct {
	decision neural.ShadowDecision
	request  neural.ShadowDecisionRequest
}

func (f *evaluationAIFake) GenerateShadowDecision(_ context.Context, _ authorization.Principal, _, _ string, request neural.ShadowDecisionRequest) (neural.ShadowDecision, error) {
	f.request = request
	return f.decision, nil
}

type evaluationMarketsFake struct {
	batch marketintelligence.CryptoMarketBatch
}

func (f *evaluationMarketsFake) CryptoMarkets(context.Context, string, []string) (marketintelligence.CryptoMarketBatch, bool, error) {
	return f.batch, false, nil
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
	contractTime  *time.Time
	emptyChain    bool
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
	if f.emptyChain {
		return financial.OptionChain{Symbol: request.Symbol, ProviderTimestamp: f.timestamp, Contracts: []financial.OptionContract{}}, nil
	}
	bid, ask, delta := financial.Decimal("1.25"), financial.Decimal("1.35"), financial.Decimal("-0.30")
	contractTime := f.timestamp
	if f.contractTime != nil {
		contractTime = *f.contractTime
	}
	return financial.OptionChain{Symbol: request.Symbol, ProviderTimestamp: f.timestamp, Contracts: []financial.OptionContract{{Underlying: request.Symbol, PutCall: "PUT", Strike: "190", Expiration: "2026-01-31", Bid: &bid, Ask: &ask, Delta: &delta, ProviderTimestamp: contractTime}}}, nil
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
	if !errors.Is(err, ErrEvaluationConfigurationChanged) || store.commits != 0 || finances.quoteCalls != 0 {
		t.Fatalf("stale mandate version was not rejected before provider reads: %v", err)
	}
}

func TestManualEvaluationRequiresImmutableCapitalBucketBinding(t *testing.T) {
	service, store, finances, principal := evaluationFixture()
	service.automation.(*evaluationAutomationFake).mandate.CapitalBucketID = "different-bucket"
	_, err := service.Evaluate(context.Background(), principal, "instance", "manual:different-bucket")
	if !errors.Is(err, ErrEvaluationConfigurationChanged) || store.commits != 0 || finances.quoteCalls != 0 {
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
	if !errors.Is(err, ErrEvaluationMarketDataStale) || store.commits != 0 {
		t.Fatalf("stale market data was not rejected before persistence: %v", err)
	}
}

func TestManualEvaluationReportsInactiveStrategy(t *testing.T) {
	service, store, finances, principal := evaluationFixture()
	store.instance.Status = "PAUSED"
	_, err := service.Evaluate(context.Background(), principal, "instance", "manual:paused")
	if !errors.Is(err, ErrEvaluationInactive) || store.commits != 0 || finances.quoteCalls != 0 {
		t.Fatalf("inactive strategy was not identified before provider reads: %v", err)
	}
}

func TestManualPaperEvaluationReportsMissingPaperState(t *testing.T) {
	service, store, finances, principal := evaluationFixture()
	store.facts.Paper = nil
	_, err := service.Evaluate(context.Background(), principal, "instance", "manual:missing-paper")
	if !errors.Is(err, ErrEvaluationPaperStateUnavailable) || store.commits != 0 || finances.quoteCalls != 0 {
		t.Fatalf("missing PAPER state was not identified before provider reads: %v", err)
	}
}

func TestManualEvaluationReportsNoEligibleContracts(t *testing.T) {
	service, store, finances, principal := evaluationFixture()
	finances.emptyChain = true
	_, err := service.Evaluate(context.Background(), principal, "instance", "manual:no-contracts")
	if !errors.Is(err, ErrEvaluationNoEligibleContracts) || store.commits != 0 || finances.quoteCalls != 1 || finances.chainCalls != 1 {
		t.Fatalf("empty option chain was not identified safely: %v", err)
	}
}

func TestManualEvaluationReportsStaleContractMarketData(t *testing.T) {
	service, store, finances, principal := evaluationFixture()
	stale := service.now().Add(-marketDataMaxAge - time.Second)
	finances.contractTime = &stale
	_, err := service.Evaluate(context.Background(), principal, "instance", "manual:stale-contract")
	if !errors.Is(err, ErrEvaluationMarketDataStale) || store.commits != 0 || finances.quoteCalls != 1 || finances.chainCalls != 1 {
		t.Fatalf("stale option contract was not identified safely: %v", err)
	}
}

func TestManualEvaluationReportsInvalidSavedParameters(t *testing.T) {
	service, store, finances, principal := evaluationFixture()
	service.automation.(*evaluationAutomationFake).mandate.StrategyParameters = []byte(`{"symbols":[]}`)
	_, err := service.Evaluate(context.Background(), principal, "instance", "manual:invalid-parameters")
	if !errors.Is(err, ErrEvaluationParametersInvalid) || store.commits != 0 || finances.quoteCalls != 0 {
		t.Fatalf("invalid saved parameters were not identified before provider reads: %v", err)
	}
}

func aiEvaluationFixture(provider string, decision neural.ShadowDecision) (*EvaluationService, *evaluationStoreFake, *evaluationFinancialFake, *evaluationAIFake, authorization.Principal) {
	now := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	instance := Instance{ID: "ai-instance", UserID: "user", AutomationMandateID: "ai-mandate", FinancialAccountID: "account", CapitalBucketID: "bucket", StrategyIdentifier: "ai_shadow", MandateVersion: 2, ExecutionMode: Shadow, CurrentState: AIMonitoring, StateVersion: 1, Status: "ACTIVE"}
	connection, model := "ai-connection", "gpt-5.6-sol"
	mandate := automation.Mandate{ID: "ai-mandate", UserID: "user", FinancialAccountID: "account", AutomationType: "AI_AUTONOMOUS", AIProviderConnectionID: &connection, AIModelID: &model, CapitalBucketID: "bucket", AutonomyLevel: "FULL_AUTONOMOUS", ExecutionMode: "SHADOW", Status: "READY", CurrentVersion: 2, StrategyParameters: json.RawMessage(`{"objective":"Preserve capital while finding cautious opportunities.","max_proposal_notional":"1"}`), AllowedUniverse: automation.Universe{Symbols: []string{"BTC"}}, EffectiveFrom: now.Add(-time.Hour)}
	store := &evaluationStoreFake{instance: instance, facts: EvaluationFacts{Breakers: []risk.CircuitBreaker{}}}
	automations := &evaluationAutomationFake{mandate: mandate, bucket: automation.CapitalBucket{ID: "bucket", UserID: "user", FinancialAccountID: "account", Name: "AI budget", AllocationType: "FIXED_AMOUNT", AllocationValue: "10", Currency: "USD", ProtectedAmount: "0", Status: "ACTIVE"}}
	cash, available, buyingPower := financial.Money{Amount: "100", Currency: "USD"}, financial.Money{Amount: "100", Currency: "USD"}, financial.Money{Amount: "100", Currency: "USD"}
	finances := &evaluationFinancialFake{account: financial.FinancialAccount{ID: "account", UserID: "user", Provider: provider, Status: "active", BaseCurrency: "USD", Capabilities: financial.Capabilities{"options": financial.Unsupported, "margin": financial.Unsupported}}, balances: financial.Balances{Cash: &cash, AvailableCash: &available, BuyingPower: &buyingPower}, positions: []financial.Position{}, timestamp: now}
	ai := &evaluationAIFake{decision: decision}
	markets := &evaluationMarketsFake{batch: marketintelligence.CryptoMarketBatch{Markets: []marketintelligence.CryptoMarketObservation{{Symbol: "BTC", Currency: "USD", CurrentPrice: "100", Bid: marketDecimalPointer("99"), Ask: marketDecimalPointer("101"), Provenance: marketintelligence.Provenance{Provider: "coinbase", Feed: "exchange", Quality: marketintelligence.RealTimeSingleVenue, ProviderTimestamp: now, ReceivedAt: now}}}}}
	service := NewEvaluationService(store, automations, finances)
	service.ConfigureAIShadow(ai, markets)
	service.now = func() time.Time { return now }
	return service, store, finances, ai, authorization.Principal{UserID: "user", Entitlement: authorization.EntitlementFounder}
}

func marketDecimalPointer(value string) *marketintelligence.Decimal {
	decimal := marketintelligence.Decimal(value)
	return &decimal
}

func TestSchwabAIShadowProposalUsesBrokerQuoteAndDeterministicRiskGate(t *testing.T) {
	decision := neural.ShadowDecision{Decision: "PROPOSE", Symbol: "BTC", Side: "BUY", ProposedNotional: "1", Confidence: "LOW", Thesis: "Bounded test", RiskFlags: []string{"Volatility"}, Limitations: []string{"No news"}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep"}}
	service, store, finances, ai, principal := aiEvaluationFixture("schwab", decision)
	outcome, err := service.Evaluate(context.Background(), principal, "ai-instance", "manual-ai:schwab")
	if err != nil || outcome.AIDecision != "PROPOSE" || outcome.Execution.Status != WouldHaveSubmitted || outcome.RiskDecision != risk.Allow {
		t.Fatalf("unexpected Schwab AI shadow outcome: %#v %v", outcome, err)
	}
	if store.commits != 1 || store.abstains != 0 || finances.quoteCalls != 1 || ai.request.Markets[0].Feed != "schwab_market_data" {
		t.Fatalf("unexpected Schwab boundaries: commits=%d abstains=%d quotes=%d request=%#v", store.commits, store.abstains, finances.quoteCalls, ai.request)
	}
}

func TestCoinbaseAIShadowAbstentionWritesJournalWithoutExecution(t *testing.T) {
	decision := neural.ShadowDecision{Decision: "ABSTAIN", Symbol: "NONE", Side: "NONE", ProposedNotional: "0", Confidence: "LOW", Thesis: "Insufficient evidence", RiskFlags: []string{}, Limitations: []string{"No edge"}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep"}}
	service, store, finances, ai, principal := aiEvaluationFixture("coinbase", decision)
	outcome, err := service.Evaluate(context.Background(), principal, "ai-instance", "manual-ai:coinbase")
	if err != nil || outcome.AIDecision != "ABSTAIN" || outcome.Execution.Status != ExecutionCanceled {
		t.Fatalf("unexpected Coinbase abstention: %#v %v", outcome, err)
	}
	if store.abstains != 1 || store.commits != 0 || finances.quoteCalls != 0 || ai.request.Markets[0].Feed != "exchange" {
		t.Fatalf("unexpected Coinbase boundaries: commits=%d abstains=%d quotes=%d request=%#v", store.commits, store.abstains, finances.quoteCalls, ai.request)
	}
}

func TestAIShadowRejectsModelOutputAboveImmutableCeilingBeforeRisk(t *testing.T) {
	decision := neural.ShadowDecision{Decision: "PROPOSE", Symbol: "BTC", Side: "BUY", ProposedNotional: "1.01", Confidence: "HIGH", Thesis: "Unsafe", RiskFlags: []string{}, Limitations: []string{}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep"}}
	service, store, _, _, principal := aiEvaluationFixture("coinbase", decision)
	_, err := service.Evaluate(context.Background(), principal, "ai-instance", "manual-ai:over-ceiling")
	if !errors.Is(err, ErrInvalid) || store.commits != 0 || store.abstains != 0 {
		t.Fatalf("over-ceiling model output reached persistence: commits=%d abstains=%d err=%v", store.commits, store.abstains, err)
	}
}
