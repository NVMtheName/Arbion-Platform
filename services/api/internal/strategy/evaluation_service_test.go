package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
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
	instance          Instance
	facts             EvaluationFacts
	commits           int
	paperCommits      int
	commitErr         error
	abstains          int
	decision          Decision
	execution         ExecutionResult
	paperFill         AIPaperFill
	abstainRationale  json.RawMessage
	outcomeCandidates []ShadowOutcomeCandidate
	outcomeMarks      []ShadowOutcome
	outcomeErr        error
}

func (f *evaluationStoreFake) DueShadowOutcomes(context.Context, Instance, time.Time) ([]ShadowOutcomeCandidate, error) {
	return f.outcomeCandidates, f.outcomeErr
}
func (f *evaluationStoreFake) RecordShadowOutcome(_ context.Context, _ Instance, outcome ShadowOutcome) error {
	if f.outcomeErr != nil {
		return f.outcomeErr
	}
	f.outcomeMarks = append(f.outcomeMarks, outcome)
	return nil
}

func (f *evaluationStoreFake) CommitAIAbstention(_ context.Context, _ Instance, _ string, rationale json.RawMessage, _ time.Time) error {
	f.abstains++
	f.abstainRationale = append(json.RawMessage(nil), rationale...)
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
	batch     marketintelligence.CryptoMarketBatch
	stats     map[string]marketintelligence.CryptoVenueStats
	history   map[string]marketintelligence.CryptoCandleSeries
	liquidity map[string]marketintelligence.CryptoLiquiditySnapshot
	filings   map[string]marketintelligence.InsiderFilingBatch
}

func (f *evaluationMarketsFake) CryptoMarkets(context.Context, string, []string) (marketintelligence.CryptoMarketBatch, bool, error) {
	return f.batch, false, nil
}
func (f *evaluationMarketsFake) CryptoVenueStats(_ context.Context, symbol, _ string) (marketintelligence.CryptoVenueStats, bool, error) {
	stats, ok := f.stats[symbol]
	if !ok {
		return marketintelligence.CryptoVenueStats{}, false, errors.New("stats unavailable")
	}
	return stats, false, nil
}
func (f *evaluationMarketsFake) RecentCryptoCandles(_ context.Context, symbol, _ string, _, _ int) (marketintelligence.CryptoCandleSeries, bool, error) {
	series, ok := f.history[symbol]
	if !ok {
		return marketintelligence.CryptoCandleSeries{}, false, errors.New("history unavailable")
	}
	return series, false, nil
}
func (f *evaluationMarketsFake) CryptoLiquidity(_ context.Context, symbol, _ string, depth int) (marketintelligence.CryptoLiquiditySnapshot, bool, error) {
	snapshot, ok := f.liquidity[symbol]
	if !ok || depth != 10 {
		return marketintelligence.CryptoLiquiditySnapshot{}, false, errors.New("liquidity unavailable")
	}
	return snapshot, false, nil
}
func (f *evaluationMarketsFake) RecentInsiderFilingsForSymbol(_ context.Context, symbol string, limit int) (marketintelligence.InsiderFilingBatch, bool, error) {
	batch, ok := f.filings[symbol]
	if !ok || limit != aiEventsPerSymbol {
		return marketintelligence.InsiderFilingBatch{}, false, errors.New("filings unavailable")
	}
	return batch, false, nil
}
func (f *evaluationStoreFake) EvaluationFacts(context.Context, Instance, time.Time) (EvaluationFacts, error) {
	return f.facts, nil
}
func (f *evaluationStoreFake) CommitEvaluation(_ context.Context, _ Instance, _ int, decision Decision, _ risk.RiskEvaluation, result ExecutionResult, _ time.Time) error {
	f.commits++
	f.decision = decision
	f.execution = result
	return f.commitErr
}
func (f *evaluationStoreFake) CommitAIPaperEvaluation(_ context.Context, _ Instance, _ int, decision Decision, _ risk.RiskEvaluation, fill AIPaperFill, _ time.Time) error {
	f.paperCommits++
	f.decision = decision
	f.paperFill = fill
	return f.commitErr
}

type evaluationAutomationFake struct {
	mandate          automation.Mandate
	versionMandate   automation.Mandate
	versionRequested int
	bucket           automation.CapitalBucket
}

func (f *evaluationAutomationFake) Get(context.Context, authorization.Principal, string) (automation.Mandate, error) {
	return f.mandate, nil
}
func (f *evaluationAutomationFake) AtVersion(_ context.Context, _ authorization.Principal, _ string, version int) (automation.Mandate, error) {
	f.versionRequested = version
	if f.versionMandate.ID != "" {
		return f.versionMandate, nil
	}
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

func TestManualEvaluationContinuesOnPinnedVersionWhileReplacementIsDraft(t *testing.T) {
	service, store, finances, principal := evaluationFixture()
	automations := service.automation.(*evaluationAutomationFake)
	automations.versionMandate = automations.mandate
	automations.mandate.CurrentVersion = 3
	automations.mandate.Status = "DRAFT"
	automations.mandate.FinancialAccountID = "replacement-account"
	automations.mandate.CapitalBucketID = "replacement-bucket"
	outcome, err := service.Evaluate(context.Background(), principal, "instance", "manual:pinned")
	if err != nil || outcome.Execution.Status != RiskDenied || store.commits != 1 || finances.quoteCalls != 1 || automations.versionRequested != 2 {
		t.Fatalf("pinned reviewed version did not continue while replacement was a draft: %#v %v", outcome, err)
	}
}

func TestManualEvaluationRequiresImmutableCapitalBucketBinding(t *testing.T) {
	service, store, finances, principal := evaluationFixture()
	automations := service.automation.(*evaluationAutomationFake)
	automations.versionMandate = automations.mandate
	automations.versionMandate.CapitalBucketID = "different-bucket"
	_, err := service.Evaluate(context.Background(), principal, "instance", "manual:different-bucket")
	if !errors.Is(err, ErrEvaluationConfigurationChanged) || store.commits != 0 || finances.quoteCalls != 0 {
		t.Fatalf("changed capital bucket was not rejected before provider reads: %v", err)
	}
}

func TestManualEvaluationStopsWhenCurrentMandateIsExplicitlyDisabled(t *testing.T) {
	service, store, finances, principal := evaluationFixture()
	automations := service.automation.(*evaluationAutomationFake)
	automations.versionMandate = automations.mandate
	automations.mandate.Status = "DISABLED"
	_, err := service.Evaluate(context.Background(), principal, "instance", "manual:disabled")
	if !errors.Is(err, ErrEvaluationConfigurationChanged) || store.commits != 0 || finances.quoteCalls != 0 || automations.versionRequested != 0 {
		t.Fatalf("explicit mandate disable did not stop before immutable version or provider reads: %v", err)
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
	store := &evaluationStoreFake{instance: instance, facts: EvaluationFacts{Breakers: []risk.CircuitBreaker{}, Reconciliation: &risk.ReconciliationSnapshot{
		AccountID: "account", ComparisonStatus: "MATCHED", BalancesStatus: "READY", PositionsStatus: "READY",
		AutonomySignal: "CLEAR", AutonomyEnforcementActive: true, ObservedAt: now.Add(-time.Minute),
	}}}
	automations := &evaluationAutomationFake{mandate: mandate, bucket: automation.CapitalBucket{ID: "bucket", UserID: "user", FinancialAccountID: "account", Name: "AI budget", AllocationType: "FIXED_AMOUNT", AllocationValue: "10", Currency: "USD", ProtectedAmount: "0", Status: "ACTIVE"}}
	cash, available, buyingPower := financial.Money{Amount: "100", Currency: "USD"}, financial.Money{Amount: "100", Currency: "USD"}, financial.Money{Amount: "100", Currency: "USD"}
	finances := &evaluationFinancialFake{account: financial.FinancialAccount{ID: "account", UserID: "user", Provider: provider, Status: "active", BaseCurrency: "USD", Capabilities: financial.Capabilities{"options": financial.Unsupported, "margin": financial.Unsupported}}, balances: financial.Balances{Cash: &cash, AvailableCash: &available, BuyingPower: &buyingPower}, positions: []financial.Position{}, timestamp: now}
	ai := &evaluationAIFake{decision: decision}
	markets := &evaluationMarketsFake{
		batch:     marketintelligence.CryptoMarketBatch{Markets: []marketintelligence.CryptoMarketObservation{{Symbol: "BTC", Currency: "USD", CurrentPrice: "100", Bid: marketDecimalPointer("99"), Ask: marketDecimalPointer("101"), Provenance: marketintelligence.Provenance{Provider: "coinbase", Feed: "exchange", Quality: marketintelligence.RealTimeSingleVenue, ProviderTimestamp: now, ReceivedAt: now}}}},
		stats:     map[string]marketintelligence.CryptoVenueStats{"BTC": {Symbol: "BTC", Currency: "USD", Open: "80", Last: "100", Volume24H: "2500"}},
		history:   map[string]marketintelligence.CryptoCandleSeries{"BTC": aiHistorySeries(now, aiHistoryExpectedIntervals)},
		liquidity: map[string]marketintelligence.CryptoLiquiditySnapshot{"BTC": aiLiquiditySnapshot(now)},
	}
	service := NewEvaluationService(store, automations, finances)
	service.ConfigureAIShadow(ai, markets)
	service.now = func() time.Time { return now }
	return service, store, finances, ai, authorization.Principal{UserID: "user", Entitlement: authorization.EntitlementFounder}
}

func aiLiquiditySnapshot(now time.Time) marketintelligence.CryptoLiquiditySnapshot {
	return marketintelligence.CryptoLiquiditySnapshot{
		Symbol: "BTC", Currency: "USD", ProductID: "BTC-USD", Depth: 10,
		Bids: []marketintelligence.CryptoBookLevel{{Price: "99", Size: "2"}, {Price: "98", Size: "1"}},
		Asks: []marketintelligence.CryptoBookLevel{{Price: "101", Size: "3"}, {Price: "102", Size: "1"}},
		Last: "100", MidMarket: "100", SpreadBPS: "200", SpreadAbsolute: "2",
		Provenance: marketintelligence.Provenance{Provider: "coinbase", Feed: "advanced_trade_public_book", Quality: marketintelligence.RealTimeSingleVenue, ProviderTimestamp: now, ReceivedAt: now},
	}
}

func aiFilingBatch(now time.Time) marketintelligence.InsiderFilingBatch {
	issuer := marketintelligence.IssuerReferenceObservation{
		Symbol: "BTC", IssuerCIK: "0000320193", Name: "Bounded fixture issuer",
		Receipt: marketintelligence.SourceReceipt{Provider: "sec_edgar", Role: marketintelligence.ReferenceData, Feed: "company_tickers", Quality: marketintelligence.AggregatedReference, ReceivedAt: now.Add(-time.Minute)},
	}
	filing := func(accession, form string, filedAt time.Time) marketintelligence.InsiderFilingObservation {
		return marketintelligence.InsiderFilingObservation{
			IssuerCIK: issuer.IssuerCIK, AccessionNumber: accession, Form: form,
			IsAmendment: strings.HasSuffix(form, "/A"), FiledAt: filedAt,
			ReportDate: filedAt.Format("2006-01-02"), PrimaryDocument: "ownership.xml",
			SourceURL:  "https://www.sec.gov/Archives/edgar/data/320193/000032019326000001/ownership.xml",
			Provenance: marketintelligence.Provenance{Provider: "sec_edgar", Role: marketintelligence.PrimaryFiling, Feed: "submissions", Quality: marketintelligence.Filing, ProviderTimestamp: filedAt, ReceivedAt: now.Add(-time.Minute)},
		}
	}
	return marketintelligence.InsiderFilingBatch{Issuer: issuer, Filings: []marketintelligence.InsiderFilingObservation{
		filing("0000320193-26-000001", "4", now.Add(-24*time.Hour)),
		filing("0000320193-26-000002", "4/A", now.AddDate(0, 0, -31)),
	}}
}

func aiHistorySeries(now time.Time, intervals int) marketintelligence.CryptoCandleSeries {
	candles := make([]marketintelligence.CryptoCandle, intervals)
	for index := range candles {
		closeValue := marketintelligence.Decimal("100")
		if index == len(candles)-1 {
			closeValue = "104"
		}
		candles[index] = marketintelligence.CryptoCandle{
			Start: now.Add(time.Duration(index-(intervals-1)) * 15 * time.Minute),
			Low:   "99", High: "105", Open: "100", Close: closeValue, Volume: "1",
		}
	}
	return marketintelligence.CryptoCandleSeries{
		Symbol: "BTC", Currency: "USD", GranularitySeconds: aiHistoryGranularitySeconds, ExpectedIntervals: aiHistoryExpectedIntervals, Candles: candles,
		Provenance: marketintelligence.Provenance{Provider: "coinbase", Feed: "rest_candles", Quality: marketintelligence.RealTimeSingleVenue, ProviderTimestamp: now, ReceivedAt: now},
	}
}

func marketDecimalPointer(value string) *marketintelligence.Decimal {
	decimal := marketintelligence.Decimal(value)
	return &decimal
}

func TestSchwabAIShadowProposalUsesBrokerQuoteAndDeterministicRiskGate(t *testing.T) {
	inputUsage, outputUsage, latencyMS := 30, 45, 120
	decision := neural.ShadowDecision{Decision: "PROPOSE", Symbol: "BTC", Side: "BUY", ProposedNotional: "1", Confidence: "LOW", Thesis: "Bounded test", RiskFlags: []string{"Volatility"}, Limitations: []string{"No news"}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep", InputUsage: &inputUsage, OutputUsage: &outputUsage, LatencyMS: &latencyMS}}
	service, store, finances, ai, principal := aiEvaluationFixture("schwab", decision)
	quantity := financial.Decimal("2")
	dayPercent, openPercent := financial.Decimal("1.9417475728"), financial.Decimal("5")
	finances.positions = []financial.Position{{
		Symbol: "BTC", InstrumentType: "COLLECTIVE_INVESTMENT", Direction: "long", Quantity: quantity,
		MarketValue: &financial.Money{Amount: "210", Currency: "USD"}, CostBasis: &financial.Money{Amount: "100", Currency: "USD"},
		CurrentPrice: &financial.Money{Amount: "105", Currency: "USD"}, DayProfitLoss: &financial.Money{Amount: "4", Currency: "USD"}, DayProfitLossPercent: &dayPercent,
		OpenProfitLoss: &financial.Money{Amount: "10", Currency: "USD"}, OpenProfitLossPercent: &openPercent,
		PriceBasis: aiPositionPriceBasis, ProviderInstrumentID: "must-not-cross-neural-boundary",
	}}
	outcome, err := service.Evaluate(context.Background(), principal, "ai-instance", "manual-ai:schwab")
	if err != nil || outcome.AIDecision != "PROPOSE" || outcome.Execution.Status != WouldHaveSubmitted || outcome.RiskDecision != risk.Allow {
		t.Fatalf("unexpected Schwab AI shadow outcome: %#v %v", outcome, err)
	}
	if store.commits != 1 || store.abstains != 0 || finances.quoteCalls != 1 || ai.request.BudgetScope != "ai-instance" || ai.request.Markets[0].Feed != "schwab_market_data" || ai.request.Markets[0].HistoryStatus != "UNAVAILABLE" || len(ai.request.MarketEventCoverage) != 1 || ai.request.MarketEventCoverage[0].Status != "UNAVAILABLE" || len(ai.request.MarketEvents) != 0 || len(ai.request.Positions) != 1 {
		t.Fatalf("unexpected Schwab boundaries: commits=%d abstains=%d quotes=%d request=%#v", store.commits, store.abstains, finances.quoteCalls, ai.request)
	}
	position := ai.request.Positions[0]
	if position.Instrument != "EQUITY" || position.PerformanceStatus != "AVAILABLE" || position.AveragePriceUSD != "100" || position.CurrentPriceUSD != "105" || position.DayProfitLossUSD != "4" || position.DayProfitLossPercent != "1.9417475728" || position.OpenProfitLossUSD != "10" || position.OpenProfitLossPercent != "5" || position.PriceBasis != aiPositionPriceBasis {
		t.Fatalf("Schwab position performance was not normalized: %#v", position)
	}
	encoded, err := json.Marshal(ai.request)
	if err != nil || strings.Contains(string(encoded), "must-not-cross-neural-boundary") || strings.Contains(string(encoded), "ai-instance") {
		t.Fatalf("provider instrument identifier crossed Neural boundary: %s err=%v", encoded, err)
	}
	assertAIInputEvidence(t, store.decision.Rationale, "schwab", "100", 1, 1, 0)
	var journal struct {
		InputEvidence struct {
			Positions []neural.ShadowPositionFact `json:"positions"`
		} `json:"input_evidence"`
	}
	if err = json.Unmarshal(store.decision.Rationale, &journal); err != nil || len(journal.InputEvidence.Positions) != 1 || journal.InputEvidence.Positions[0] != position || strings.Contains(string(store.decision.Rationale), "must-not-cross-neural-boundary") {
		t.Fatalf("immutable Schwab position evidence was incomplete: %#v err=%v", journal.InputEvidence.Positions, err)
	}
}

func TestSchwabAIPositionPerformanceDropsIncompletePairsAndUnknownPriceBasis(t *testing.T) {
	dayProfitLoss := financial.Money{Amount: "4", Currency: "USD"}
	currentPrice := financial.Money{Amount: "105", Currency: "USD"}
	fact := neural.ShadowPositionFact{PerformanceStatus: "UNAVAILABLE"}
	addAIPositionPerformance(&fact, financial.Position{DayProfitLoss: &dayProfitLoss, CurrentPrice: &currentPrice, PriceBasis: "UNVERIFIED"})

	if fact.PerformanceStatus != "UNAVAILABLE" || fact.DayProfitLossUSD != "" || fact.DayProfitLossPercent != "" || fact.CurrentPriceUSD != "" || fact.PriceBasis != "" {
		t.Fatalf("incomplete or unproven performance crossed the AI boundary: %#v", fact)
	}
}

func TestSchwabAIShadowUsesBoundedSECEventIdentityWithoutFilingContents(t *testing.T) {
	decision := neural.ShadowDecision{Decision: "ABSTAIN", Symbol: "NONE", Side: "NONE", ProposedNotional: "0", Confidence: "LOW", Thesis: "No cautious action", RiskFlags: []string{}, Limitations: []string{}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep"}}
	service, store, _, ai, principal := aiEvaluationFixture("schwab", decision)
	service.markets.(*evaluationMarketsFake).filings = map[string]marketintelligence.InsiderFilingBatch{"BTC": aiFilingBatch(service.now())}

	outcome, err := service.Evaluate(context.Background(), principal, "ai-instance", "manual-ai:sec-events")
	if err != nil || outcome.AIDecision != "ABSTAIN" || store.abstains != 1 {
		t.Fatalf("unexpected SEC-context outcome: %#v err=%v", outcome, err)
	}
	if len(ai.request.MarketEventCoverage) != 1 || ai.request.MarketEventCoverage[0].Status != "AVAILABLE" || ai.request.MarketEventCoverage[0].EventCount != 1 || ai.request.MarketEventCoverage[0].ResolverFeed != "company_tickers" || len(ai.request.MarketEvents) != 1 {
		t.Fatalf("SEC event coverage was not bounded: %#v %#v", ai.request.MarketEventCoverage, ai.request.MarketEvents)
	}
	event := ai.request.MarketEvents[0]
	if event.EventType != "SEC_OWNERSHIP_FILING" || event.Form != "4" || event.EvidenceID != "0000320193-26-000001" || event.Feed != "submissions" || event.Quality != "FILING" {
		t.Fatalf("SEC event identity was incomplete: %#v", event)
	}
	encoded, err := json.Marshal(ai.request)
	if err != nil || strings.Contains(string(encoded), "ownership.xml") || strings.Contains(string(encoded), "Bounded fixture issuer") || strings.Contains(string(encoded), "sec.gov/Archives") {
		t.Fatalf("filing contents or issuer prose crossed the Neural boundary: %s err=%v", encoded, err)
	}
	var journal struct {
		InputEvidence struct {
			MarketEventCoverage []neural.ShadowMarketEventCoverage `json:"market_event_coverage"`
			MarketEvents        []neural.ShadowMarketEventFact     `json:"market_events"`
		} `json:"input_evidence"`
	}
	if err = json.Unmarshal(store.abstainRationale, &journal); err != nil || len(journal.InputEvidence.MarketEventCoverage) != 1 || len(journal.InputEvidence.MarketEvents) != 1 || journal.InputEvidence.MarketEvents[0].EvidenceID != event.EvidenceID {
		t.Fatalf("immutable SEC evidence was not preserved: %#v err=%v", journal.InputEvidence, err)
	}
}

func TestSchwabAIShadowDistinguishesNoRecentFilingsFromUnavailableSEC(t *testing.T) {
	decision := neural.ShadowDecision{Decision: "ABSTAIN", Symbol: "NONE", Side: "NONE", ProposedNotional: "0", Confidence: "LOW", Thesis: "No cautious action", Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep"}}
	service, _, _, _, _ := aiEvaluationFixture("schwab", decision)
	batch := aiFilingBatch(service.now())
	batch.Filings = []marketintelligence.InsiderFilingObservation{}
	service.markets.(*evaluationMarketsFake).filings = map[string]marketintelligence.InsiderFilingBatch{"BTC": batch}

	coverage, events := service.aiMarketEventFacts(context.Background(), financial.FinancialAccount{Provider: "schwab"}, []string{"BTC"}, service.now())
	if len(coverage) != 1 || coverage[0].Status != "AVAILABLE" || coverage[0].EventCount != 0 || len(events) != 0 || coverage[0].ResolverReceivedAt == nil {
		t.Fatalf("authoritative empty SEC window was treated as unavailable: %#v %#v", coverage, events)
	}
}

func TestCoinbaseAIShadowAbstentionWritesJournalWithoutExecution(t *testing.T) {
	decision := neural.ShadowDecision{Decision: "ABSTAIN", Symbol: "NONE", Side: "NONE", ProposedNotional: "0", Confidence: "LOW", Thesis: "Insufficient evidence", RiskFlags: []string{}, Limitations: []string{"No edge"}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep"}}
	service, store, finances, ai, principal := aiEvaluationFixture("coinbase", decision)
	quantity, available := financial.Decimal("0.01"), financial.Decimal("0.01")
	finances.positions = []financial.Position{{Symbol: "BTC", InstrumentType: "crypto", Direction: "long", Quantity: quantity, AvailableQuantity: &available}}
	outcome, err := service.Evaluate(context.Background(), principal, "ai-instance", "manual-ai:coinbase")
	if err != nil || outcome.AIDecision != "ABSTAIN" || outcome.Execution.Status != ExecutionCanceled {
		t.Fatalf("unexpected Coinbase abstention: %#v %v", outcome, err)
	}
	if store.abstains != 1 || store.commits != 0 || finances.quoteCalls != 0 || ai.request.Markets[0].Feed != "exchange" || ai.request.Markets[0].ChangePercent1H != "4.0000000000" || ai.request.Markets[0].ChangePercent6H != "4.0000000000" || ai.request.Markets[0].ChangePercent24H != "25.0000000000" || ai.request.Markets[0].Volume24H != "2500" || ai.request.Markets[0].HistoryStatus != "COMPLETE" || ai.request.Markets[0].HistoryContiguousIntervals != 96 || ai.request.Markets[0].HistoryFeed != "rest_candles" || ai.request.Markets[0].LiquidityStatus != "AVAILABLE" || ai.request.Markets[0].SpreadBPS != "200" || ai.request.Markets[0].BidDepthUSD != "296.0000000000" || ai.request.Markets[0].AskDepthUSD != "405.0000000000" || ai.request.Markets[0].BidLevels != 2 || ai.request.Markets[0].AskLevels != 2 || ai.request.Markets[0].LiquidityFeed != "advanced_trade_public_book" || len(ai.request.Positions) != 1 || ai.request.Positions[0].MarketValueUSD != "1.0000000000" || ai.request.Positions[0].PerformanceStatus != "UNAVAILABLE" || ai.request.Positions[0].AveragePriceUSD != "" {
		t.Fatalf("unexpected Coinbase boundaries: commits=%d abstains=%d quotes=%d request=%#v", store.commits, store.abstains, finances.quoteCalls, ai.request)
	}
	assertAIInputEvidence(t, store.abstainRationale, "coinbase", "100", 1, 1, 0)
}

func TestCoinbaseAIPaperProposalUsesOnlyIsolatedPortfolioAndSimulatesAtomically(t *testing.T) {
	decision := neural.ShadowDecision{Decision: "PROPOSE", Symbol: "BTC", Side: "BUY", ProposedNotional: "100", Confidence: "MEDIUM", Thesis: "Bounded paper candidate", RiskFlags: []string{}, Limitations: []string{"Simulation only"}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep"}}
	service, store, finances, ai, principal := aiEvaluationFixture("coinbase", decision)
	store.instance.ExecutionMode = Paper
	store.facts = EvaluationFacts{Paper: &PaperEvaluationFacts{
		Cash: "900.0000000000", CurrentExposure: "100.0000000000",
		Positions:     []Position{{Symbol: "BTC", Instrument: "CRYPTO", Quantity: "1.0000000000", AveragePrice: "100.0000000000"}},
		RiskPositions: []risk.Position{{Instrument: "BTC", Exposure: "100.0000000000", AvailableQuantity: "1.0000000000"}},
	}, Breakers: []risk.CircuitBreaker{}}
	automations := service.automation.(*evaluationAutomationFake)
	automations.mandate.ExecutionMode = "PAPER"
	automations.mandate.StrategyParameters = json.RawMessage(`{"objective":"Preserve simulated capital.","max_proposal_notional":"100"}`)
	automations.bucket.AllocationValue = "1000"

	outcome, err := service.Evaluate(context.Background(), principal, "ai-instance", "manual-ai:paper-buy")
	if err != nil || outcome.AIDecision != "PROPOSE" || outcome.Execution.Status != SimulatedFilled || outcome.RiskDecision != risk.Allow {
		t.Fatalf("unexpected AI Paper outcome: %#v err=%v", outcome, err)
	}
	if store.paperCommits != 1 || store.commits != 0 || store.abstains != 0 || finances.balanceCalls != 0 || finances.positionCalls != 0 || finances.quoteCalls != 0 {
		t.Fatalf("AI Paper crossed an isolated boundary: paper=%d generic=%d abstain=%d balances=%d positions=%d quotes=%d", store.paperCommits, store.commits, store.abstains, finances.balanceCalls, finances.positionCalls, finances.quoteCalls)
	}
	if ai.request.AvailableCashUSD != "900.0000000000" || ai.request.BuyingPowerUSD != "900.0000000000" || len(ai.request.Positions) != 1 || ai.request.Positions[0].AveragePriceUSD != "100.0000000000" || ai.request.Positions[0].CurrentPriceUSD != "100.0000000000" || ai.request.Positions[0].AvailableQuantity != "1.0000000000" || ai.request.Positions[0].PriceBasis != "PROVIDER_MARKET_REFERENCE" {
		t.Fatalf("isolated paper context was not supplied to the model: %#v", ai.request)
	}
	if !store.paperFill.SimulationOnly || store.paperFill.MarketProvider != "coinbase" || store.paperFill.PricingBasis != "ASK" || store.paperFill.RequestedNotional != "100.0000000000" {
		t.Fatalf("simulated fill provenance was incomplete: %#v", store.paperFill)
	}
	debit, debitOK := new(big.Rat).SetString(strings.TrimPrefix(store.paperFill.CashDelta, "-"))
	if !debitOK || debit.Cmp(big.NewRat(100, 1)) > 0 {
		t.Fatalf("paper costs exceeded the model's bounded proposal: %#v", store.paperFill)
	}
	if len(store.outcomeMarks) != 0 {
		t.Fatalf("Paper evaluation wrote Shadow outcome marks: %#v", store.outcomeMarks)
	}
	if !strings.Contains(string(store.decision.Rationale), `"execution_mode":"PAPER"`) || !strings.Contains(string(store.decision.Rationale), `"portfolio_source":"arbion_isolated_paper_ledger"`) {
		t.Fatalf("Paper simulation contract was not journaled: %s", store.decision.Rationale)
	}
}

func TestAIShadowRepeatProposalIsDeniedBeforeShadowExecution(t *testing.T) {
	decision := neural.ShadowDecision{Decision: "PROPOSE", Symbol: "BTC", Side: "BUY", ProposedNotional: "1", Confidence: "MEDIUM", Thesis: "Repeat the prior direction", RiskFlags: []string{}, Limitations: []string{}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep"}}
	service, store, _, ai, principal := aiEvaluationFixture("coinbase", decision)
	store.facts.RecentActions = []risk.RecentAction{{Instrument: "BTC", Side: "BUY", OccurredAt: service.now().Add(-30 * time.Minute)}}
	store.facts.RecentDecisions = []neural.ShadowRecentDecision{{Decision: "PROPOSE", Symbol: "BTC", Side: "BUY", Disposition: "WOULD_HAVE_SUBMITTED", OccurredAt: service.now().Add(-30 * time.Minute)}}

	outcome, err := service.Evaluate(context.Background(), principal, "ai-instance", "manual-ai:repeat")
	if err != nil || outcome.AIDecision != "PROPOSE" || outcome.Execution.Status != RiskDenied || outcome.RiskDecision != risk.Deny || len(outcome.RiskReasonCodes) != 1 || outcome.RiskReasonCodes[0] != risk.RepeatActionCooldownActive {
		t.Fatalf("repeat proposal did not stop at the deterministic risk gate: %#v %v", outcome, err)
	}
	if store.commits != 1 || store.abstains != 0 {
		t.Fatalf("repeat proposal produced the wrong immutable evidence: commits=%d abstains=%d", store.commits, store.abstains)
	}
	if len(ai.request.RecentDecisions) != 1 || ai.request.RecentDecisions[0].Disposition != "WOULD_HAVE_SUBMITTED" {
		t.Fatalf("bounded recent decision memory was not supplied: %#v", ai.request.RecentDecisions)
	}
	assertAIInputEvidence(t, store.decision.Rationale, "coinbase", "100", 0, 1, 1)
}

func TestAIShadowProposalIsHeldWithoutMatchedBrokerReconciliation(t *testing.T) {
	decision := neural.ShadowDecision{Decision: "PROPOSE", Symbol: "BTC", Side: "BUY", ProposedNotional: "1", Confidence: "MEDIUM", Thesis: "Bounded candidate", RiskFlags: []string{}, Limitations: []string{}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep"}}
	service, store, _, _, principal := aiEvaluationFixture("coinbase", decision)
	store.facts.Reconciliation = nil

	outcome, err := service.Evaluate(context.Background(), principal, "ai-instance", "manual-ai:reconciliation-required")
	if err != nil || outcome.AIDecision != "PROPOSE" || outcome.Execution.Status != RiskDenied || outcome.RiskDecision != risk.Deny || len(outcome.RiskReasonCodes) != 1 || outcome.RiskReasonCodes[0] != risk.ReconciliationRequired {
		t.Fatalf("missing broker reconciliation did not hold the AI proposal: %#v %v", outcome, err)
	}
	if store.commits != 1 || store.abstains != 0 {
		t.Fatalf("reconciliation hold did not create exactly one immutable decision: commits=%d abstains=%d", store.commits, store.abstains)
	}
}

func TestAIHistoryFactsNeverFillProviderCandleGaps(t *testing.T) {
	now := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	series := aiHistorySeries(now, 24)
	series.Candles[len(series.Candles)-4].Start = series.Candles[len(series.Candles)-4].Start.Add(-15 * time.Minute)
	fact := neural.ShadowMarketFact{HistoryStatus: "UNAVAILABLE", HistoryGranularitySeconds: aiHistoryGranularitySeconds, HistoryExpectedIntervals: aiHistoryExpectedIntervals}

	addAIHistoryFacts(&fact, series, now)

	if fact.HistoryStatus != "UNAVAILABLE" || fact.HistoryContiguousIntervals != 3 || fact.ChangePercent1H != "" || fact.ChangePercent6H != "" {
		t.Fatalf("missing Coinbase bucket was synthesized: %#v", fact)
	}
}

func TestAIHistoryFactsRejectStaleSeriesWithoutDerivedChanges(t *testing.T) {
	now := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	series := aiHistorySeries(now.Add(-31*time.Minute), aiHistoryExpectedIntervals)
	fact := neural.ShadowMarketFact{HistoryStatus: "UNAVAILABLE", HistoryGranularitySeconds: aiHistoryGranularitySeconds, HistoryExpectedIntervals: aiHistoryExpectedIntervals}

	addAIHistoryFacts(&fact, series, now)

	if fact.HistoryStatus != "STALE" || fact.HistoryContiguousIntervals != 0 || fact.ChangePercent1H != "" || fact.ChangePercent6H != "" {
		t.Fatalf("stale Coinbase history produced a trend signal: %#v", fact)
	}
}

func TestAILiquidityFactsRejectStaleBookWithoutDerivedDepth(t *testing.T) {
	now := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	snapshot := aiLiquiditySnapshot(now.Add(-marketDataMaxAge - time.Second))
	fact := neural.ShadowMarketFact{Symbol: "BTC", Currency: "USD", LiquidityStatus: "UNAVAILABLE"}

	addAILiquidityFacts(&fact, snapshot, now)

	if fact.LiquidityStatus != "STALE" || fact.SpreadBPS != "" || fact.BidDepthUSD != "" || fact.AskDepthUSD != "" || fact.LiquidityObservedAt == nil {
		t.Fatalf("stale Coinbase book produced liquidity values: %#v", fact)
	}
}

func TestAILiquidityFactsRejectMalformedLevelWithoutPartialEvidence(t *testing.T) {
	now := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	snapshot := aiLiquiditySnapshot(now)
	snapshot.Bids[0].Size = "not-a-decimal"
	fact := neural.ShadowMarketFact{Symbol: "BTC", Currency: "USD", LiquidityStatus: "UNAVAILABLE"}

	addAILiquidityFacts(&fact, snapshot, now)

	if fact.LiquidityStatus != "UNAVAILABLE" || fact.LiquidityFeed != "" || fact.LiquidityObservedAt != nil {
		t.Fatalf("malformed Coinbase book leaked partial liquidity evidence: %#v", fact)
	}
}

func assertAIInputEvidence(t *testing.T, rationale json.RawMessage, provider, buyingPower string, positionCount, marketCount, recentDecisionCount int) {
	t.Helper()
	var payload struct {
		AIProvider    string `json:"ai_provider"`
		ModelID       string `json:"model_id"`
		Profile       string `json:"profile"`
		InputUsage    *int   `json:"input_usage"`
		OutputUsage   *int   `json:"output_usage"`
		LatencyMS     *int   `json:"latency_ms"`
		InputEvidence struct {
			Provider        string                        `json:"provider"`
			AvailableCash   string                        `json:"available_cash_usd"`
			BuyingPower     string                        `json:"buying_power_usd"`
			Positions       []neural.ShadowPositionFact   `json:"positions"`
			Markets         []neural.ShadowMarketFact     `json:"markets"`
			RecentDecisions []neural.ShadowRecentDecision `json:"recent_decisions"`
			ObservedAt      time.Time                     `json:"observed_at"`
		} `json:"input_evidence"`
	}
	if err := json.Unmarshal(rationale, &payload); err != nil {
		t.Fatalf("AI input evidence was not valid JSON: %v", err)
	}
	evidence := payload.InputEvidence
	if payload.AIProvider != "openai" || payload.ModelID != "gpt-5.6-sol" || payload.Profile != "deep" || evidence.Provider != provider || evidence.AvailableCash != "100" || evidence.BuyingPower != buyingPower || len(evidence.Positions) != positionCount || len(evidence.Markets) != marketCount || len(evidence.RecentDecisions) != recentDecisionCount || evidence.ObservedAt.IsZero() {
		t.Fatalf("AI input evidence was incomplete: %#v", evidence)
	}
	if provider == "schwab" && (payload.InputUsage == nil || *payload.InputUsage != 30 || payload.OutputUsage == nil || *payload.OutputUsage != 45 || payload.LatencyMS == nil || *payload.LatencyMS != 120) {
		t.Fatalf("AI route telemetry was not preserved: %#v", payload)
	}
	if provider == "coinbase" && (len(evidence.Markets) != 1 || evidence.Markets[0].LiquidityStatus != "AVAILABLE" || evidence.Markets[0].SpreadBPS != "200" || evidence.Markets[0].LiquidityObservedAt == nil) {
		t.Fatalf("immutable Coinbase liquidity evidence was not preserved: %#v", evidence.Markets)
	}
}

func TestCoinbaseAIShadowRecordsConservativeOneHourSellOutcomeBeforeNextDecision(t *testing.T) {
	decision := neural.ShadowDecision{Decision: "ABSTAIN", Symbol: "NONE", Side: "NONE", ProposedNotional: "0", Confidence: "LOW", Thesis: "No new edge", RiskFlags: []string{}, Limitations: []string{}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep"}}
	service, store, _, ai, principal := aiEvaluationFixture("coinbase", decision)
	store.outcomeCandidates = []ShadowOutcomeCandidate{{ExecutionRecordID: "shadow-record", Horizon: ShadowOutcomeOneHour, Symbol: "BTC", Side: "SELL", Quantity: "0.0100000000", EntryPrice: "100.0000000000", CreatedAt: service.now().Add(-2 * time.Hour)}}

	outcome, err := service.Evaluate(context.Background(), principal, "ai-instance", "scheduled:outcome")
	if err != nil || outcome.AIDecision != "ABSTAIN" || len(store.outcomeMarks) != 1 || ai.request.Objective == "" {
		t.Fatalf("outcome tracking disrupted the next decision: outcome=%#v marks=%#v request=%#v err=%v", outcome, store.outcomeMarks, ai.request, err)
	}
	mark := store.outcomeMarks[0]
	if mark.ExecutionRecordID != "shadow-record" || mark.PricingBasis != "ASK_TO_CLOSE" || mark.ObservedPrice != "101.0000000000" || mark.DirectionalChangeUSD != "-0.0100000000" || mark.DirectionalChangePercent != "-1.0000000000" || mark.ElapsedSeconds != 7200 {
		t.Fatalf("conservative SELL outcome was incorrect: %#v", mark)
	}
}

func TestSchwabAIShadowRecordsConservativeOneHourBuyOutcomeBeforeNextDecision(t *testing.T) {
	decision := neural.ShadowDecision{Decision: "ABSTAIN", Symbol: "NONE", Side: "NONE", ProposedNotional: "0", Confidence: "LOW", Thesis: "No new edge", RiskFlags: []string{}, Limitations: []string{}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep"}}
	service, store, _, ai, principal := aiEvaluationFixture("schwab", decision)
	store.outcomeCandidates = []ShadowOutcomeCandidate{{ExecutionRecordID: "shadow-record", Horizon: ShadowOutcomeOneHour, Symbol: "BTC", Side: "BUY", Quantity: "0.0100000000", EntryPrice: "200.0000000000", CreatedAt: service.now().Add(-time.Hour)}}

	outcome, err := service.Evaluate(context.Background(), principal, "ai-instance", "scheduled:schwab-outcome")
	if err != nil || outcome.AIDecision != "ABSTAIN" || len(store.outcomeMarks) != 1 || ai.request.Objective == "" {
		t.Fatalf("Schwab outcome tracking disrupted the next decision: outcome=%#v marks=%#v request=%#v err=%v", outcome, store.outcomeMarks, ai.request, err)
	}
	mark := store.outcomeMarks[0]
	if mark.ExecutionRecordID != "shadow-record" || mark.PricingBasis != "BID_TO_CLOSE" || mark.ObservedPrice != "199.9000000000" || mark.DirectionalChangeUSD != "-0.0010000000" || mark.DirectionalChangePercent != "-0.0500000000" || mark.MarketFeed != "schwab_market_data" || mark.MarketQuality != "BROKER_REALTIME" || mark.ElapsedSeconds != 3600 {
		t.Fatalf("conservative Schwab BUY outcome was incorrect: %#v", mark)
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
