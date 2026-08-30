package strategy

import (
	"context"
	"encoding/json"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/marketintelligence"
	"github.com/arbion/platform/services/api/internal/neural"
	"github.com/arbion/platform/services/api/internal/risk"
)

type PaperEvaluationFacts struct {
	Cash, CurrentExposure string
	Positions             []Position
	RiskPositions         []risk.Position
}

type EvaluationFacts struct {
	Paper           *PaperEvaluationFacts
	Breakers        []risk.CircuitBreaker
	ActionsToday    int
	RecentActions   []risk.RecentAction
	RecentDecisions []neural.ShadowRecentDecision
	Reconciliation  *risk.ReconciliationSnapshot
}

type EvaluationStore interface {
	Repository
	Get(context.Context, string, string) (Instance, error)
	EvaluationFacts(context.Context, Instance, time.Time) (EvaluationFacts, error)
}

type EvaluationAutomation interface {
	Get(context.Context, authorization.Principal, string) (automation.Mandate, error)
	AtVersion(context.Context, authorization.Principal, string, int) (automation.Mandate, error)
	GetBucket(context.Context, authorization.Principal, string) (automation.CapitalBucket, error)
}

type EvaluationFinancial interface {
	GetAccount(context.Context, authorization.Principal, string) (financial.FinancialAccount, error)
	GetBalances(context.Context, authorization.Principal, string) (financial.Balances, error)
	GetPositions(context.Context, authorization.Principal, string) ([]financial.Position, error)
	GetQuote(context.Context, authorization.Principal, string, string) (financial.Quote, error)
	GetOptionChain(context.Context, authorization.Principal, string, financial.OptionChainRequest) (financial.OptionChain, error)
}

type EvaluationAI interface {
	GenerateShadowDecision(context.Context, authorization.Principal, string, string, neural.ShadowDecisionRequest) (neural.ShadowDecision, error)
}

type EvaluationMarkets interface {
	CryptoMarkets(context.Context, string, []string) (marketintelligence.CryptoMarketBatch, bool, error)
	CryptoVenueStats(context.Context, string, string) (marketintelligence.CryptoVenueStats, bool, error)
	RecentCryptoCandles(context.Context, string, string, int, int) (marketintelligence.CryptoCandleSeries, bool, error)
	CryptoLiquidity(context.Context, string, string, int) (marketintelligence.CryptoLiquiditySnapshot, bool, error)
	RecentInsiderFilingsForSymbol(context.Context, string, int) (marketintelligence.InsiderFilingBatch, bool, error)
}

const (
	aiHistoryGranularitySeconds = 900
	aiHistoryExpectedIntervals  = 96
	aiHistoryFreshness          = 30 * time.Minute
	aiDecisionMemoryWindow      = 6 * time.Hour
	aiDecisionMemoryLimit       = 6
	aiEventLookbackDays         = 30
	aiEventsPerSymbol           = 2
	aiEventResolverMaxAge       = 24 * time.Hour
	aiPaperFeeBasisPoints       = 50
	aiPaperSlippageBasisPoints  = 25
)

type AIAbstentionStore interface {
	CommitAIAbstention(context.Context, Instance, string, json.RawMessage, time.Time) error
}

type AIShadowOutcomeStore interface {
	DueShadowOutcomes(context.Context, Instance, time.Time) ([]ShadowOutcomeCandidate, error)
	RecordShadowOutcome(context.Context, Instance, ShadowOutcome) error
}

type EvaluationService struct {
	store        EvaluationStore
	automation   EvaluationAutomation
	financial    EvaluationFinancial
	now          func() time.Time
	orchestrator *Orchestrator
	ai           EvaluationAI
	markets      EvaluationMarkets
	outcomes     AIShadowOutcomeStore
}

func (s *EvaluationService) ConfigureAIShadow(ai EvaluationAI, markets EvaluationMarkets) {
	s.ai = ai
	s.markets = markets
}

func NewEvaluationService(store EvaluationStore, automations EvaluationAutomation, finances EvaluationFinancial) *EvaluationService {
	service := &EvaluationService{
		store:      store,
		automation: automations,
		financial:  finances,
		now:        func() time.Time { return time.Now().UTC() },
		orchestrator: &Orchestrator{
			Engine: NewEngine(),
			Risk:   risk.NewEngine(),
			Store:  store,
			Paper:  PaperExecutionAdapter{},
			Shadow: ShadowExecutionAdapter{},
		},
	}
	if outcomes, ok := store.(AIShadowOutcomeStore); ok {
		service.outcomes = outcomes
	}
	return service
}

var evaluationEventID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,199}$`)

const (
	marketDataMaxAge          = 15 * time.Minute
	marketDataMaxFutureOffset = time.Minute
)

func (s *EvaluationService) Evaluate(ctx context.Context, principal authorization.Principal, instanceID, eventID string) (EvaluationOutcome, error) {
	if !entitled(principal) {
		return EvaluationOutcome{}, ErrForbidden
	}
	if !evaluationEventID.MatchString(eventID) {
		return EvaluationOutcome{}, ErrInvalid
	}
	instance, err := s.store.Get(ctx, principal.UserID, instanceID)
	if err != nil {
		return EvaluationOutcome{}, ErrNotFound
	}
	if instance.Status != "ACTIVE" {
		return EvaluationOutcome{}, ErrEvaluationInactive
	}
	if instance.ExecutionMode != Paper && instance.ExecutionMode != Shadow {
		return EvaluationOutcome{}, ErrInvalid
	}
	currentMandate, err := s.automation.Get(ctx, principal, instance.AutomationMandateID)
	if err != nil {
		return EvaluationOutcome{}, ErrNotFound
	}
	if currentMandate.ID != instance.AutomationMandateID || currentMandate.UserID != principal.UserID || currentMandate.CurrentVersion < instance.MandateVersion || (currentMandate.Status != "READY" && currentMandate.Status != "DRAFT") {
		return EvaluationOutcome{}, ErrEvaluationConfigurationChanged
	}
	mandate, err := s.automation.AtVersion(ctx, principal, instance.AutomationMandateID, instance.MandateVersion)
	if err != nil {
		return EvaluationOutcome{}, ErrEvaluationConfigurationChanged
	}
	if mandate.ID != instance.AutomationMandateID || mandate.UserID != principal.UserID || mandate.FinancialAccountID != instance.FinancialAccountID || mandate.CapitalBucketID != instance.CapitalBucketID || mandate.CurrentVersion != instance.MandateVersion || mandate.Status != "READY" || mandate.ExecutionMode != string(instance.ExecutionMode) {
		return EvaluationOutcome{}, ErrEvaluationConfigurationChanged
	}
	if instance.StrategyIdentifier == "ai_shadow" {
		return s.evaluateAIAutonomous(ctx, principal, instance, mandate, eventID)
	}
	if mandate.StrategyIdentifier == nil || *mandate.StrategyIdentifier != instance.StrategyIdentifier {
		return EvaluationOutcome{}, ErrEvaluationConfigurationChanged
	}
	parameters, err := ParseParameters(mandate.StrategyParameters)
	if err != nil {
		return EvaluationOutcome{}, ErrEvaluationParametersInvalid
	}
	bucket, err := s.automation.GetBucket(ctx, principal, instance.CapitalBucketID)
	if err != nil || bucket.UserID != principal.UserID || bucket.FinancialAccountID != instance.FinancialAccountID || bucket.Status != "ACTIVE" || bucket.IsReserve {
		return EvaluationOutcome{}, ErrEvaluationConfigurationChanged
	}
	account, err := s.financial.GetAccount(ctx, principal, instance.FinancialAccountID)
	if err != nil || account.ID != instance.FinancialAccountID || account.UserID != principal.UserID || account.Status != "active" {
		if err != nil {
			return EvaluationOutcome{}, err
		}
		return EvaluationOutcome{}, ErrEvaluationConfigurationChanged
	}

	now := s.now().UTC()
	facts, err := s.store.EvaluationFacts(ctx, instance, now)
	if err != nil {
		return EvaluationOutcome{}, err
	}
	strategyAccount, riskAccount, err := s.accountSnapshots(ctx, principal, instance, account, facts, now)
	if err != nil {
		return EvaluationOutcome{}, err
	}
	market, err := s.marketSnapshot(ctx, principal, instance, parameters, now)
	if err != nil {
		return EvaluationOutcome{}, err
	}

	riskMandate := risk.Mandate{
		ID:                             mandate.ID,
		UserID:                         mandate.UserID,
		AccountID:                      mandate.FinancialAccountID,
		BucketID:                       instance.CapitalBucketID,
		Status:                         mandate.Status,
		AutomationType:                 mandate.AutomationType,
		AutonomyLevel:                  mandate.AutonomyLevel,
		ExecutionMode:                  mandate.ExecutionMode,
		Version:                        mandate.CurrentVersion,
		StrategyIdentifier:             mandate.StrategyIdentifier,
		EffectiveFrom:                  mandate.EffectiveFrom,
		EffectiveUntil:                 mandate.EffectiveUntil,
		AllowedSymbols:                 mandate.AllowedUniverse.Symbols,
		ProhibitedSymbols:              mandate.ProhibitedUniverse.Symbols,
		UniverseIDs:                    mandate.AllowedUniverse.UniverseIDs,
		MarginAllowed:                  mandate.MarginAllowed,
		OptionsAllowed:                 mandate.OptionsAllowed,
		PaperOptionsSimulationAttested: mandate.PaperOptionsSimulationAttested,
		MaxCapitalDeployed:             mandate.Risk.MaxCapitalDeployed,
		MaxSinglePositionAmount:        mandate.Risk.MaxSinglePositionAmount,
		MaxSinglePositionPercentage:    mandate.Risk.MaxSinglePositionPercentage,
		MaxDailyLoss:                   mandate.Risk.MaxDailyLoss,
		MinimumCashReserve:             mandate.Risk.MinimumCashReserve,
		MaxTradesPerDay:                mandate.Risk.MaxTradesPerDay,
	}
	riskBucket := risk.CapitalBucket{ID: bucket.ID, UserID: bucket.UserID, AccountID: bucket.FinancialAccountID, AllocationType: bucket.AllocationType, AllocationValue: bucket.AllocationValue, ProtectedAmount: bucket.ProtectedAmount, AllocationLimit: bucket.AllocationLimit}
	actionsToday := facts.ActionsToday
	activity := &risk.RiskActivitySnapshot{Timestamp: now, ActionsToday: &actionsToday}
	evaluationInput := EvaluationInput{
		EventID:           eventID,
		Timestamp:         now,
		Account:           strategyAccount,
		Parameters:        parameters,
		Mandate:           MandateSnapshot{ID: mandate.ID, Version: mandate.CurrentVersion, StrategyIdentifier: *mandate.StrategyIdentifier},
		Market:            market,
		ExistingPositions: strategyAccount.Positions,
		PriorState:        instance.CurrentState,
	}
	riskContext := risk.EvaluationContext{
		UserID:             principal.UserID,
		AccountOwned:       true,
		FinancialEntitled:  authorization.CanConnectFinancialAccounts(principal),
		AutomationEntitled: authorization.CanUseAutomation(principal),
		ConnectionUsable:   true,
		Mandate:            &riskMandate,
		Bucket:             &riskBucket,
		Account:            &riskAccount,
		Activity:           activity,
		Breakers:           facts.Breakers,
		Now:                now,
		MaxStaleness:       2 * time.Minute,
	}
	return s.orchestrator.Evaluate(ctx, instance, evaluationInput, riskContext)
}

func (s *EvaluationService) marketSnapshot(ctx context.Context, principal authorization.Principal, instance Instance, parameters Parameters, now time.Time) (MarketSnapshot, error) {
	kind := "PUT"
	if instance.CurrentState == ReadyForCall || instance.CurrentState == LongShares {
		kind = "CALL"
	}
	market := MarketSnapshot{Options: []OptionCandidate{}}
	staleContractObserved := false
	for index, symbol := range parameters.Symbols {
		quote, err := s.financial.GetQuote(ctx, principal, instance.FinancialAccountID, symbol)
		if err != nil {
			return MarketSnapshot{}, err
		}
		if !freshMarketTimestamp(quote.ProviderTimestamp, now) {
			return MarketSnapshot{}, ErrEvaluationMarketDataStale
		}
		chain, err := s.financial.GetOptionChain(ctx, principal, instance.FinancialAccountID, financial.OptionChainRequest{Symbol: symbol, ContractType: kind, StrikeCount: 50, FromDate: now.AddDate(0, 0, parameters.MinimumDTE), ToDate: now.AddDate(0, 0, parameters.MaximumDTE)})
		if err != nil {
			return MarketSnapshot{}, err
		}
		if !freshMarketTimestamp(chain.ProviderTimestamp, now) {
			return MarketSnapshot{}, ErrEvaluationMarketDataStale
		}
		if index == 0 {
			market.Symbol = symbol
			market.Timestamp = olderTimestamp(quote.ProviderTimestamp, chain.ProviderTimestamp)
			market.UnderlyingPrice = decimalPointer(chain.UnderlyingPrice)
			market.Bid = decimalPointer(quote.Bid)
			market.Ask = decimalPointer(quote.Ask)
		} else {
			market.Timestamp = olderTimestamp(market.Timestamp, olderTimestamp(quote.ProviderTimestamp, chain.ProviderTimestamp))
		}
		for _, contract := range chain.Contracts {
			if !strings.EqualFold(contract.Underlying, symbol) || !strings.EqualFold(contract.PutCall, kind) {
				continue
			}
			if !freshMarketTimestamp(contract.ProviderTimestamp, now) {
				staleContractObserved = true
				continue
			}
			market.Options = append(market.Options, OptionCandidate{Underlying: strings.ToUpper(contract.Underlying), OptionType: strings.ToUpper(contract.PutCall), Strike: string(contract.Strike), Expiration: contract.Expiration, Bid: decimalPointer(contract.Bid), Ask: decimalPointer(contract.Ask), Mark: decimalPointer(contract.Mark), Delta: decimalPointer(contract.Delta), ImpliedVolatility: decimalPointer(contract.ImpliedVolatility), OpenInterest: contract.OpenInterest, Volume: contract.Volume, Timestamp: contract.ProviderTimestamp})
		}
	}
	if len(market.Options) == 0 {
		if staleContractObserved {
			return MarketSnapshot{}, ErrEvaluationMarketDataStale
		}
		return MarketSnapshot{}, ErrEvaluationNoEligibleContracts
	}
	return market, nil
}

func freshMarketTimestamp(observedAt, now time.Time) bool {
	return !observedAt.IsZero() && !observedAt.After(now.Add(marketDataMaxFutureOffset)) && now.Sub(observedAt) <= marketDataMaxAge
}

func olderTimestamp(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func decimalPointer(value *financial.Decimal) *string {
	if value == nil {
		return nil
	}
	result := string(*value)
	return &result
}

func (s *EvaluationService) accountSnapshots(ctx context.Context, principal authorization.Principal, instance Instance, account financial.FinancialAccount, facts EvaluationFacts, now time.Time) (AccountSnapshot, risk.AccountRiskSnapshot, error) {
	capabilities := func(name string) risk.CapabilityState {
		switch account.Capabilities[name] {
		case financial.Supported:
			return risk.CapabilitySupported
		case financial.Unsupported:
			return risk.CapabilityUnsupported
		default:
			return risk.CapabilityUnknown
		}
	}
	if instance.ExecutionMode == Paper {
		if facts.Paper == nil {
			return AccountSnapshot{}, risk.AccountRiskSnapshot{}, ErrEvaluationPaperStateUnavailable
		}
		return AccountSnapshot{Timestamp: now, AvailableCash: facts.Paper.Cash, Positions: facts.Paper.Positions}, risk.AccountRiskSnapshot{AccountID: account.ID, Currency: account.BaseCurrency, Timestamp: now, Cash: facts.Paper.Cash, AvailableCash: facts.Paper.Cash, BuyingPower: facts.Paper.Cash, CurrentExposure: facts.Paper.CurrentExposure, Positions: facts.Paper.RiskPositions, Options: capabilities("options"), Margin: capabilities("margin")}, nil
	}
	balances, err := s.financial.GetBalances(ctx, principal, account.ID)
	if err != nil {
		return AccountSnapshot{}, risk.AccountRiskSnapshot{}, err
	}
	positions, err := s.financial.GetPositions(ctx, principal, account.ID)
	if err != nil {
		return AccountSnapshot{}, risk.AccountRiskSnapshot{}, err
	}
	available, ok := moneyAmount(balances.AvailableCash, account.BaseCurrency)
	if !ok {
		return AccountSnapshot{}, risk.AccountRiskSnapshot{}, ErrInvalid
	}
	cash, ok := moneyAmount(balances.Cash, account.BaseCurrency)
	if !ok {
		cash = available
	}
	buyingPower, ok := moneyAmount(balances.BuyingPower, account.BaseCurrency)
	if !ok {
		buyingPower = available
	}
	strategyPositions := make([]Position, 0, len(positions))
	riskPositions := make([]risk.Position, 0, len(positions))
	totalExposure := new(big.Rat)
	for _, position := range positions {
		quantity := string(position.Quantity)
		if position.Direction == "short" && !strings.HasPrefix(quantity, "-") {
			quantity = "-" + quantity
		}
		strategyPositions = append(strategyPositions, Position{Symbol: strings.ToUpper(position.Symbol), Instrument: position.InstrumentType, Quantity: quantity})
		if position.MarketValue == nil || position.MarketValue.Currency != account.BaseCurrency {
			continue
		}
		exposure, valid := new(big.Rat).SetString(string(position.MarketValue.Amount))
		if !valid {
			return AccountSnapshot{}, risk.AccountRiskSnapshot{}, ErrInvalid
		}
		if exposure.Sign() < 0 {
			exposure.Neg(exposure)
		}
		totalExposure.Add(totalExposure, exposure)
		availableQuantity := quantity
		if position.AvailableQuantity != nil {
			availableQuantity = string(*position.AvailableQuantity)
		}
		riskPositions = append(riskPositions, risk.Position{Instrument: strings.ToUpper(position.Symbol), Exposure: exposure.FloatString(10), AvailableQuantity: availableQuantity})
	}
	return AccountSnapshot{Timestamp: now, AvailableCash: available, Positions: strategyPositions}, risk.AccountRiskSnapshot{AccountID: account.ID, Currency: account.BaseCurrency, Timestamp: now, Cash: cash, AvailableCash: available, BuyingPower: buyingPower, CurrentExposure: totalExposure.FloatString(10), Positions: riskPositions, Options: capabilities("options"), Margin: capabilities("margin")}, nil
}

func (s *EvaluationService) evaluateAIAutonomous(ctx context.Context, principal authorization.Principal, instance Instance, mandate automation.Mandate, eventID string) (EvaluationOutcome, error) {
	if s.ai == nil || mandate.AutomationType != "AI_AUTONOMOUS" || mandate.AutonomyLevel != "FULL_AUTONOMOUS" || (mandate.ExecutionMode != "PAPER" && mandate.ExecutionMode != "SHADOW") || mandate.StrategyIdentifier != nil || mandate.AIProviderConnectionID == nil || mandate.AIModelID == nil || instance.CurrentState != AIMonitoring {
		return EvaluationOutcome{}, ErrEvaluationConfigurationChanged
	}
	parameters, err := automation.ParseAIShadowParameters(mandate.StrategyParameters)
	if err != nil || len(mandate.AllowedUniverse.Symbols) == 0 || len(mandate.AllowedUniverse.Symbols) > 8 {
		return EvaluationOutcome{}, ErrEvaluationParametersInvalid
	}
	bucket, err := s.automation.GetBucket(ctx, principal, instance.CapitalBucketID)
	if err != nil || bucket.UserID != principal.UserID || bucket.FinancialAccountID != instance.FinancialAccountID || bucket.Status != "ACTIVE" || bucket.IsReserve {
		return EvaluationOutcome{}, ErrEvaluationConfigurationChanged
	}
	account, err := s.financial.GetAccount(ctx, principal, instance.FinancialAccountID)
	if err != nil || account.ID != instance.FinancialAccountID || account.UserID != principal.UserID || account.Status != "active" || (account.Provider != "coinbase" && account.Provider != "schwab") || account.BaseCurrency != "USD" {
		if err != nil {
			return EvaluationOutcome{}, err
		}
		return EvaluationOutcome{}, ErrEvaluationConfigurationChanged
	}
	now := s.now().UTC()
	facts, err := s.store.EvaluationFacts(ctx, instance, now)
	if err != nil {
		return EvaluationOutcome{}, err
	}
	markets, err := s.aiMarketFacts(ctx, principal, account, mandate.AllowedUniverse.Symbols, now)
	if err != nil {
		return EvaluationOutcome{}, err
	}
	if instance.ExecutionMode == Shadow {
		if err = s.recordDueAIShadowOutcomes(ctx, instance, markets, now); err != nil {
			return EvaluationOutcome{}, err
		}
	}
	eventCoverage, marketEvents := s.aiMarketEventFacts(ctx, account, mandate.AllowedUniverse.Symbols, now)
	var request neural.ShadowDecisionRequest
	var riskAccount risk.AccountRiskSnapshot
	var paperPortfolio AIPaperPortfolioSnapshot
	if instance.ExecutionMode == Paper {
		request, riskAccount, paperPortfolio, err = aiPaperAccountFacts(account, facts.Paper, mandate.AllowedUniverse.Symbols, markets, now)
	} else {
		request, riskAccount, err = s.aiAccountFacts(ctx, principal, account, mandate.AllowedUniverse.Symbols, markets, now)
	}
	if err != nil {
		return EvaluationOutcome{}, err
	}
	request.Profile = ""
	request.BudgetScope = instance.ID
	request.Objective = parameters.Objective
	request.AllowedSymbols = append([]string(nil), mandate.AllowedUniverse.Symbols...)
	request.MaxProposalNotional = parameters.MaxProposalNotional
	request.Markets = markets
	request.MarketEventCoverage = eventCoverage
	request.MarketEvents = marketEvents
	request.RecentDecisions = append([]neural.ShadowRecentDecision{}, facts.RecentDecisions...)
	request.ObservedAt = now
	decision, err := s.ai.GenerateShadowDecision(ctx, principal, *mandate.AIProviderConnectionID, *mandate.AIModelID, request)
	if err != nil {
		return EvaluationOutcome{}, err
	}
	if !validAIShadowDecision(decision, mandate.AllowedUniverse.Symbols, parameters.MaxProposalNotional) {
		return EvaluationOutcome{}, ErrInvalid
	}
	rationaleFields := map[string]any{
		"decision": decision.Decision, "symbol": decision.Symbol, "side": decision.Side,
		"proposed_notional": decision.ProposedNotional, "confidence": decision.Confidence,
		"thesis": decision.Thesis, "risk_flags": decision.RiskFlags, "limitations": decision.Limitations,
		"ai_provider": decision.Metadata.Provider, "model_id": decision.Metadata.Model,
		"profile": decision.Metadata.Profile, "input_usage": decision.Metadata.InputUsage,
		"output_usage": decision.Metadata.OutputUsage, "latency_ms": decision.Metadata.LatencyMS,
		"objective": parameters.Objective, "market_observed_at": oldestAIMarketTimestamp(markets),
		"execution_mode": mandate.ExecutionMode,
		"input_evidence": map[string]any{
			"provider": account.Provider, "available_cash_usd": request.AvailableCashUSD,
			"buying_power_usd": request.BuyingPowerUSD, "positions": request.Positions,
			"markets": request.Markets, "market_event_coverage": request.MarketEventCoverage,
			"market_events": request.MarketEvents, "recent_decisions": request.RecentDecisions,
			"observed_at": request.ObservedAt,
		},
	}
	if instance.ExecutionMode == Paper {
		rationaleFields["simulation"] = map[string]any{
			"model": "ai_paper_spot_v1", "simulation_only": true,
			"fee_basis_points":      aiPaperFeeBasisPoints,
			"slippage_basis_points": aiPaperSlippageBasisPoints,
			"portfolio_source":      "arbion_isolated_paper_ledger",
		}
	}
	rationale, err := json.Marshal(rationaleFields)
	if err != nil {
		return EvaluationOutcome{}, err
	}
	if decision.Decision == "ABSTAIN" {
		store, ok := s.store.(AIAbstentionStore)
		if !ok {
			return EvaluationOutcome{}, ErrInvalid
		}
		if err := store.CommitAIAbstention(ctx, instance, eventID, rationale, now); err != nil {
			return EvaluationOutcome{}, err
		}
		return EvaluationOutcome{Execution: ExecutionResult{Status: ExecutionCanceled, Reason: "ai_abstained_no_order_was_sent"}, LiveExecutionAvailable: false, AIDecision: "ABSTAIN", Confidence: decision.Confidence}, nil
	}
	market, ok := findAIMarket(markets, decision.Symbol)
	if !ok || (decision.Side != "BUY" && decision.Side != "SELL") {
		return EvaluationOutcome{}, ErrInvalid
	}
	price, pricingBasis, priceAvailable := aiExecutionPrice(market, decision.Side)
	priceRat, priceOK := new(big.Rat).SetString(price)
	notionalRat, notionalOK := new(big.Rat).SetString(decision.ProposedNotional)
	if !priceAvailable || !priceOK || priceRat.Sign() <= 0 || !notionalOK || notionalRat.Sign() <= 0 {
		return EvaluationOutcome{}, ErrInvalid
	}
	quantityDivisor := new(big.Rat).Set(priceRat)
	if instance.ExecutionMode == Paper && decision.Side == "BUY" {
		slippage := new(big.Rat).Add(big.NewRat(1, 1), new(big.Rat).SetFrac64(aiPaperSlippageBasisPoints, aiPaperBasisPointDenominator))
		fee := new(big.Rat).Add(big.NewRat(1, 1), new(big.Rat).SetFrac64(aiPaperFeeBasisPoints, aiPaperBasisPointDenominator))
		quantityDivisor.Mul(quantityDivisor, new(big.Rat).Mul(slippage, fee))
	}
	quantity := floorRat(new(big.Rat).Quo(notionalRat, quantityDivisor), 10)
	if quantity == "0.0000000000" {
		return EvaluationOutcome{}, ErrInvalid
	}
	actionType := risk.ActionBuy
	if decision.Side == "SELL" {
		actionType = risk.ActionSell
	}
	state := string(instance.CurrentState)
	actionID := instance.ID + ":" + eventID
	action := risk.ProposedAction{ID: actionID, CorrelationID: eventID, FinancialAccountID: instance.FinancialAccountID, Source: risk.SourceAI, ActionType: actionType, MandateID: &instance.AutomationMandateID, MandateVersion: &instance.MandateVersion, Instrument: decision.Symbol, Side: decision.Side, Quantity: quantity, Notional: decision.ProposedNotional, EstimatedPrice: &price, StrategyInstanceID: &instance.ID, StrategyState: &state, CreatedAt: now}
	riskMandate := risk.Mandate{ID: mandate.ID, UserID: mandate.UserID, AccountID: mandate.FinancialAccountID, BucketID: instance.CapitalBucketID, Status: mandate.Status, AutomationType: mandate.AutomationType, AutonomyLevel: mandate.AutonomyLevel, ExecutionMode: mandate.ExecutionMode, Version: mandate.CurrentVersion, EffectiveFrom: mandate.EffectiveFrom, EffectiveUntil: mandate.EffectiveUntil, AllowedSymbols: mandate.AllowedUniverse.Symbols, ProhibitedSymbols: mandate.ProhibitedUniverse.Symbols, UniverseIDs: mandate.AllowedUniverse.UniverseIDs, MarginAllowed: false, OptionsAllowed: false, MaxCapitalDeployed: mandate.Risk.MaxCapitalDeployed, MaxSinglePositionAmount: mandate.Risk.MaxSinglePositionAmount, MaxSinglePositionPercentage: mandate.Risk.MaxSinglePositionPercentage, MaxDailyLoss: mandate.Risk.MaxDailyLoss, MinimumCashReserve: mandate.Risk.MinimumCashReserve, MaxTradesPerDay: mandate.Risk.MaxTradesPerDay}
	riskBucket := risk.CapitalBucket{ID: bucket.ID, UserID: bucket.UserID, AccountID: bucket.FinancialAccountID, Name: bucket.Name, AllocationType: bucket.AllocationType, AllocationValue: bucket.AllocationValue, Currency: bucket.Currency, ProtectedAmount: bucket.ProtectedAmount, AllocationLimit: bucket.AllocationLimit, Status: bucket.Status, IsReserve: bucket.IsReserve}
	actionsToday := facts.ActionsToday
	riskContext := risk.EvaluationContext{UserID: principal.UserID, AccountOwned: true, FinancialEntitled: authorization.CanConnectFinancialAccounts(principal), AutomationEntitled: authorization.CanUseAutomation(principal), ConnectionUsable: true, Mandate: &riskMandate, Bucket: &riskBucket, Account: &riskAccount, Activity: &risk.RiskActivitySnapshot{Timestamp: now, ActionsToday: &actionsToday, RecentActions: append([]risk.RecentAction(nil), facts.RecentActions...)}, Reconciliation: facts.Reconciliation, Breakers: facts.Breakers, Now: now, MaxStaleness: 2 * time.Minute}
	instrument := "EQUITY"
	if account.Provider == "coinbase" {
		instrument = "CRYPTO"
	}
	decisionRecord := Decision{ProposedAction: &action, Source: "AI", InstrumentType: instrument, ProposedState: AIMonitoring, Reason: "bounded_ai_nonlive_decision", Rationale: rationale}
	var outcome EvaluationOutcome
	if instance.ExecutionMode == Paper {
		outcome, err = s.orchestrator.EvaluateAIPaperDecision(ctx, instance, decisionRecord, riskContext, paperPortfolio, AIPaperMarketReference{
			Symbol: decision.Symbol, Price: price, Basis: pricingBasis, Provider: account.Provider,
			Feed: market.Feed, Quality: market.Quality, ObservedAt: market.ObservedAt,
		}, AIPaperSimulationConfig{FeeBasisPoints: aiPaperFeeBasisPoints, SlippageBasisPoints: aiPaperSlippageBasisPoints}, now)
	} else {
		outcome, err = s.orchestrator.EvaluateDecision(ctx, instance, decisionRecord, riskContext, now)
	}
	outcome.AIDecision = "PROPOSE"
	outcome.Confidence = decision.Confidence
	return outcome, err
}

func (s *EvaluationService) aiMarketEventFacts(ctx context.Context, account financial.FinancialAccount, symbols []string, now time.Time) ([]neural.ShadowMarketEventCoverage, []neural.ShadowMarketEventFact) {
	if account.Provider != "schwab" {
		return []neural.ShadowMarketEventCoverage{}, []neural.ShadowMarketEventFact{}
	}
	coverage := make([]neural.ShadowMarketEventCoverage, 0, len(symbols))
	events := make([]neural.ShadowMarketEventFact, 0, len(symbols)*aiEventsPerSymbol)
	for _, rawSymbol := range symbols {
		symbol := strings.ToUpper(strings.TrimSpace(rawSymbol))
		item := neural.ShadowMarketEventCoverage{Symbol: symbol, Status: "UNAVAILABLE", LookbackDays: aiEventLookbackDays}
		if s.markets == nil {
			coverage = append(coverage, item)
			continue
		}
		batch, _, err := s.markets.RecentInsiderFilingsForSymbol(ctx, symbol, aiEventsPerSymbol)
		if err != nil || batch.Issuer.Symbol != symbol || marketintelligence.ValidateIssuerReference(batch.Issuer, now, aiEventResolverMaxAge, marketDataMaxFutureOffset) != nil {
			coverage = append(coverage, item)
			continue
		}
		bounded := make([]neural.ShadowMarketEventFact, 0, aiEventsPerSymbol)
		validBatch := true
		for _, filing := range batch.Filings {
			if filing.IssuerCIK != batch.Issuer.IssuerCIK || marketintelligence.ValidateInsiderFiling(filing, now, marketDataMaxFutureOffset) != nil {
				validBatch = false
				break
			}
			if filing.FiledAt.Before(now.AddDate(0, 0, -aiEventLookbackDays)) {
				continue
			}
			bounded = append(bounded, neural.ShadowMarketEventFact{
				Symbol: symbol, EventType: "SEC_OWNERSHIP_FILING", Form: filing.Form,
				IsAmendment: filing.IsAmendment, EvidenceID: filing.AccessionNumber,
				IssuerCIK: filing.IssuerCIK, OccurredAt: filing.FiledAt,
				Provider: filing.Provenance.Provider, Feed: filing.Provenance.Feed,
				Quality: string(filing.Provenance.Quality),
			})
		}
		if !validBatch {
			coverage = append(coverage, item)
			continue
		}
		receivedAt := batch.Issuer.Receipt.ReceivedAt
		item.Status = "AVAILABLE"
		item.EventCount = len(bounded)
		item.ResolverProvider = batch.Issuer.Receipt.Provider
		item.ResolverFeed = batch.Issuer.Receipt.Feed
		item.ResolverQuality = string(batch.Issuer.Receipt.Quality)
		item.ResolverReceivedAt = &receivedAt
		coverage = append(coverage, item)
		events = append(events, bounded...)
	}
	sort.Slice(events, func(left, right int) bool {
		if events[left].OccurredAt.Equal(events[right].OccurredAt) {
			if events[left].Symbol == events[right].Symbol {
				return events[left].EvidenceID < events[right].EvidenceID
			}
			return events[left].Symbol < events[right].Symbol
		}
		return events[left].OccurredAt.After(events[right].OccurredAt)
	})
	return coverage, events
}

func (s *EvaluationService) recordDueAIShadowOutcomes(ctx context.Context, instance Instance, markets []neural.ShadowMarketFact, now time.Time) error {
	if s.outcomes == nil {
		return nil
	}
	candidates, err := s.outcomes.DueShadowOutcomes(ctx, instance, now)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		market, ok := findAIMarket(markets, candidate.Symbol)
		if !ok {
			return ErrEvaluationMarketDataStale
		}
		outcome, err := buildAIShadowOutcome(candidate, market, now)
		if err != nil {
			return err
		}
		if err = s.outcomes.RecordShadowOutcome(ctx, instance, outcome); err != nil {
			return err
		}
	}
	return nil
}

func buildAIShadowOutcome(candidate ShadowOutcomeCandidate, market neural.ShadowMarketFact, now time.Time) (ShadowOutcome, error) {
	minimumAge := time.Hour
	if candidate.Horizon == ShadowOutcomeTwentyFourHours {
		minimumAge = 24 * time.Hour
	} else if candidate.Horizon != ShadowOutcomeOneHour {
		return ShadowOutcome{}, ErrInvalid
	}
	elapsed := now.Sub(candidate.CreatedAt)
	if candidate.ExecutionRecordID == "" || !strings.EqualFold(candidate.Symbol, market.Symbol) || elapsed < minimumAge || now.IsZero() || !freshMarketTimestamp(market.ObservedAt, now) {
		return ShadowOutcome{}, ErrInvalid
	}

	var prices []struct {
		value, basis string
	}
	switch candidate.Side {
	case "BUY":
		prices = []struct{ value, basis string }{{market.Bid, "BID_TO_CLOSE"}, {market.Mark, "MARK_FALLBACK"}, {market.Last, "LAST_FALLBACK"}}
	case "SELL":
		prices = []struct{ value, basis string }{{market.Ask, "ASK_TO_CLOSE"}, {market.Mark, "MARK_FALLBACK"}, {market.Last, "LAST_FALLBACK"}}
	default:
		return ShadowOutcome{}, ErrInvalid
	}
	observedPrice, pricingBasis := "", ""
	for _, price := range prices {
		value, ok := new(big.Rat).SetString(price.value)
		if ok && value.Sign() > 0 {
			observedPrice, pricingBasis = price.value, price.basis
			break
		}
	}
	entry, entryOK := new(big.Rat).SetString(candidate.EntryPrice)
	observed, observedOK := new(big.Rat).SetString(observedPrice)
	quantity, quantityOK := new(big.Rat).SetString(candidate.Quantity)
	if !entryOK || entry.Sign() <= 0 || !observedOK || observed.Sign() <= 0 || !quantityOK || quantity.Sign() <= 0 || market.Feed == "" || market.Quality == "" {
		return ShadowOutcome{}, ErrInvalid
	}
	difference := new(big.Rat)
	if candidate.Side == "BUY" {
		difference.Sub(observed, entry)
	} else {
		difference.Sub(entry, observed)
	}
	change := new(big.Rat).Mul(difference, quantity)
	entryNotional := new(big.Rat).Mul(new(big.Rat).Set(entry), quantity)
	changePercent := new(big.Rat).Mul(new(big.Rat).Quo(change, entryNotional), big.NewRat(100, 1))
	return ShadowOutcome{
		ExecutionRecordID:        candidate.ExecutionRecordID,
		Horizon:                  candidate.Horizon,
		Symbol:                   strings.ToUpper(candidate.Symbol),
		Side:                     candidate.Side,
		Quantity:                 quantity.FloatString(10),
		EntryPrice:               entry.FloatString(10),
		ObservedPrice:            observed.FloatString(10),
		DirectionalChangeUSD:     change.FloatString(10),
		DirectionalChangePercent: changePercent.FloatString(10),
		PricingBasis:             pricingBasis,
		MarketFeed:               market.Feed,
		MarketQuality:            market.Quality,
		MarketObservedAt:         market.ObservedAt,
		EvaluatedAt:              now,
		ElapsedSeconds:           int64(elapsed / time.Second),
	}, nil
}

func (s *EvaluationService) aiAccountFacts(ctx context.Context, principal authorization.Principal, account financial.FinancialAccount, allowed []string, markets []neural.ShadowMarketFact, now time.Time) (neural.ShadowDecisionRequest, risk.AccountRiskSnapshot, error) {
	balances, err := s.financial.GetBalances(ctx, principal, account.ID)
	if err != nil {
		return neural.ShadowDecisionRequest{}, risk.AccountRiskSnapshot{}, err
	}
	positions, err := s.financial.GetPositions(ctx, principal, account.ID)
	if err != nil {
		return neural.ShadowDecisionRequest{}, risk.AccountRiskSnapshot{}, err
	}
	available, ok := moneyAmount(balances.AvailableCash, "USD")
	if !ok {
		return neural.ShadowDecisionRequest{}, risk.AccountRiskSnapshot{}, ErrInvalid
	}
	cash, ok := moneyAmount(balances.Cash, "USD")
	if !ok {
		cash = available
	}
	buyingPower, ok := moneyAmount(balances.BuyingPower, "USD")
	if !ok {
		buyingPower = available
	}
	allowedSet := map[string]bool{}
	for _, symbol := range allowed {
		allowedSet[strings.ToUpper(symbol)] = true
	}
	request := neural.ShadowDecisionRequest{AvailableCashUSD: available, BuyingPowerUSD: buyingPower, Positions: []neural.ShadowPositionFact{}}
	riskPositions := []risk.Position{}
	for _, position := range positions {
		symbol := strings.ToUpper(position.Symbol)
		if !allowedSet[symbol] {
			continue
		}
		quantity := string(position.Quantity)
		if position.Direction == "short" && !strings.HasPrefix(quantity, "-") {
			quantity = "-" + quantity
		}
		availableQuantity := "0"
		if position.AvailableQuantity != nil {
			availableQuantity = string(*position.AvailableQuantity)
		} else if !strings.HasPrefix(quantity, "-") {
			availableQuantity = quantity
		}
		marketValue := "0"
		if account.Provider == "coinbase" {
			if price, found := aiMarketPrice(markets, symbol); found {
				quantityValue, quantityOK := new(big.Rat).SetString(quantity)
				priceValue, priceOK := new(big.Rat).SetString(price)
				if quantityOK && priceOK && priceValue.Sign() > 0 {
					if quantityValue.Sign() < 0 {
						quantityValue.Neg(quantityValue)
					}
					marketValue = new(big.Rat).Mul(quantityValue, priceValue).FloatString(10)
				}
			}
		} else if position.MarketValue != nil && position.MarketValue.Currency == "USD" {
			marketValue = string(position.MarketValue.Amount)
		}
		instrument := strings.ToUpper(position.InstrumentType)
		if account.Provider == "schwab" {
			instrument = "EQUITY"
		} else if account.Provider == "coinbase" {
			instrument = "CRYPTO"
		}
		fact := neural.ShadowPositionFact{Symbol: symbol, Instrument: instrument, Quantity: quantity, AvailableQuantity: availableQuantity, MarketValueUSD: marketValue, PerformanceStatus: "UNAVAILABLE"}
		if account.Provider == "schwab" {
			addAIPositionPerformance(&fact, position)
		}
		request.Positions = append(request.Positions, fact)
		exposure := marketValue
		if strings.HasPrefix(exposure, "-") {
			exposure = strings.TrimPrefix(exposure, "-")
		}
		riskPositions = append(riskPositions, risk.Position{Instrument: symbol, Exposure: exposure, AvailableQuantity: availableQuantity})
	}
	capability := func(name string) risk.CapabilityState {
		if account.Capabilities[name] == financial.Supported {
			return risk.CapabilitySupported
		}
		if account.Capabilities[name] == financial.Unsupported {
			return risk.CapabilityUnsupported
		}
		return risk.CapabilityUnknown
	}
	riskAccount := risk.AccountRiskSnapshot{AccountID: account.ID, Currency: "USD", Timestamp: now, Cash: cash, AvailableCash: available, BuyingPower: buyingPower, CurrentExposure: "0", Positions: riskPositions, Options: capability("options"), Margin: capability("margin")}
	return request, riskAccount, nil
}

// aiPaperAccountFacts exposes only the isolated Arbion paper ledger to the
// model and risk engine. The connected account remains the source of current
// market references, never cash, positions, buying power, or execution state.
func aiPaperAccountFacts(account financial.FinancialAccount, paper *PaperEvaluationFacts, allowed []string, markets []neural.ShadowMarketFact, now time.Time) (neural.ShadowDecisionRequest, risk.AccountRiskSnapshot, AIPaperPortfolioSnapshot, error) {
	if paper == nil {
		return neural.ShadowDecisionRequest{}, risk.AccountRiskSnapshot{}, AIPaperPortfolioSnapshot{}, ErrEvaluationPaperStateUnavailable
	}
	cash, cashOK := new(big.Rat).SetString(paper.Cash)
	if !cashOK || cash.Sign() < 0 || account.BaseCurrency != "USD" || (account.Provider != "coinbase" && account.Provider != "schwab") {
		return neural.ShadowDecisionRequest{}, risk.AccountRiskSnapshot{}, AIPaperPortfolioSnapshot{}, ErrInvalid
	}
	allowedSet := map[string]bool{}
	for _, symbol := range allowed {
		allowedSet[symbol] = true
	}
	expectedInstrument := "EQUITY"
	if account.Provider == "coinbase" {
		expectedInstrument = "CRYPTO"
	}
	request := neural.ShadowDecisionRequest{AvailableCashUSD: paper.Cash, BuyingPowerUSD: paper.Cash, Positions: []neural.ShadowPositionFact{}}
	portfolio := AIPaperPortfolioSnapshot{Currency: "USD", Cash: paper.Cash, Positions: map[string]string{}}
	riskPositions := []risk.Position{}
	totalExposure := new(big.Rat)
	for _, position := range paper.Positions {
		symbol := strings.ToUpper(strings.TrimSpace(position.Symbol))
		quantity, quantityOK := new(big.Rat).SetString(position.Quantity)
		if symbol != position.Symbol || !allowedSet[symbol] || position.Instrument != expectedInstrument || !quantityOK || quantity.Sign() < 0 {
			return neural.ShadowDecisionRequest{}, risk.AccountRiskSnapshot{}, AIPaperPortfolioSnapshot{}, ErrInvalid
		}
		if _, duplicate := portfolio.Positions[symbol]; duplicate {
			return neural.ShadowDecisionRequest{}, risk.AccountRiskSnapshot{}, AIPaperPortfolioSnapshot{}, ErrInvalid
		}
		portfolio.Positions[symbol] = quantity.FloatString(aiPaperDecimalPlaces)
		if quantity.Sign() == 0 {
			continue
		}
		averagePrice, averageOK := new(big.Rat).SetString(position.AveragePrice)
		currentPriceText, currentOK := aiMarketPrice(markets, symbol)
		currentPrice, currentPriceOK := new(big.Rat).SetString(currentPriceText)
		if !averageOK || averagePrice.Sign() <= 0 || !currentOK || !currentPriceOK || currentPrice.Sign() <= 0 {
			return neural.ShadowDecisionRequest{}, risk.AccountRiskSnapshot{}, AIPaperPortfolioSnapshot{}, ErrInvalid
		}
		marketValue := new(big.Rat).Mul(new(big.Rat).Set(quantity), currentPrice)
		openProfitLoss := new(big.Rat).Mul(new(big.Rat).Sub(currentPrice, averagePrice), quantity)
		openProfitLossPercent := new(big.Rat).Mul(new(big.Rat).Quo(new(big.Rat).Sub(currentPrice, averagePrice), averagePrice), big.NewRat(100, 1))
		totalExposure.Add(totalExposure, marketValue)
		request.Positions = append(request.Positions, neural.ShadowPositionFact{
			Symbol: symbol, Instrument: expectedInstrument, Quantity: quantity.FloatString(10),
			AvailableQuantity: quantity.FloatString(10), MarketValueUSD: marketValue.FloatString(10),
			PerformanceStatus: "PARTIAL", AveragePriceUSD: averagePrice.FloatString(10),
			CurrentPriceUSD: currentPrice.FloatString(10), OpenProfitLossUSD: openProfitLoss.FloatString(10),
			OpenProfitLossPercent: openProfitLossPercent.FloatString(10), PriceBasis: "PROVIDER_MARKET_REFERENCE",
		})
		riskPositions = append(riskPositions, risk.Position{Instrument: symbol, Exposure: marketValue.FloatString(10), AvailableQuantity: quantity.FloatString(10)})
	}
	riskAccount := risk.AccountRiskSnapshot{
		AccountID: account.ID, Currency: "USD", Timestamp: now,
		Cash: paper.Cash, AvailableCash: paper.Cash, BuyingPower: paper.Cash,
		CurrentExposure: totalExposure.FloatString(10), Positions: riskPositions,
		Options: risk.CapabilityUnsupported, Margin: risk.CapabilityUnsupported,
	}
	return request, riskAccount, portfolio, nil
}

const aiPositionPriceBasis = "PROVIDER_POSITION_MARKET_VALUE_PER_UNIT"

func addAIPositionPerformance(fact *neural.ShadowPositionFact, position financial.Position) {
	if value, ok := positiveMoneyAmount(position.CostBasis, "USD"); ok {
		fact.AveragePriceUSD = value
	}
	if position.PriceBasis == aiPositionPriceBasis {
		if value, ok := positiveMoneyAmount(position.CurrentPrice, "USD"); ok {
			fact.CurrentPriceUSD = value
			fact.PriceBasis = aiPositionPriceBasis
		}
	}
	if amount, amountOK := moneyAmount(position.DayProfitLoss, "USD"); amountOK {
		if percent, percentOK := decimalAmount(position.DayProfitLossPercent); percentOK {
			fact.DayProfitLossUSD = amount
			fact.DayProfitLossPercent = percent
		}
	}
	if amount, amountOK := moneyAmount(position.OpenProfitLoss, "USD"); amountOK {
		if percent, percentOK := decimalAmount(position.OpenProfitLossPercent); percentOK {
			fact.OpenProfitLossUSD = amount
			fact.OpenProfitLossPercent = percent
		}
	}
	complete := fact.AveragePriceUSD != "" && fact.CurrentPriceUSD != "" && fact.DayProfitLossUSD != "" && fact.DayProfitLossPercent != "" && fact.OpenProfitLossUSD != "" && fact.OpenProfitLossPercent != ""
	if complete {
		fact.PerformanceStatus = "AVAILABLE"
	} else if fact.AveragePriceUSD != "" || fact.CurrentPriceUSD != "" || fact.DayProfitLossUSD != "" || fact.OpenProfitLossUSD != "" {
		fact.PerformanceStatus = "PARTIAL"
	}
}

func positiveMoneyAmount(value *financial.Money, currency string) (string, bool) {
	amount, ok := moneyAmount(value, currency)
	if !ok {
		return "", false
	}
	parsed, ok := new(big.Rat).SetString(amount)
	return amount, ok && parsed.Sign() > 0
}

func decimalAmount(value *financial.Decimal) (string, bool) {
	if value == nil {
		return "", false
	}
	amount := string(*value)
	_, ok := new(big.Rat).SetString(amount)
	return amount, ok
}

func (s *EvaluationService) aiMarketFacts(ctx context.Context, principal authorization.Principal, account financial.FinancialAccount, symbols []string, now time.Time) ([]neural.ShadowMarketFact, error) {
	facts := make([]neural.ShadowMarketFact, 0, len(symbols))
	if account.Provider == "schwab" {
		for _, symbol := range symbols {
			quote, err := s.financial.GetQuote(ctx, principal, account.ID, symbol)
			if err != nil {
				return nil, err
			}
			if !freshMarketTimestamp(quote.ProviderTimestamp, now) {
				return nil, ErrEvaluationMarketDataStale
			}
			facts = append(facts, neural.ShadowMarketFact{Symbol: strings.ToUpper(symbol), AssetClass: "EQUITY", Currency: "USD", Bid: financialDecimal(quote.Bid), Ask: financialDecimal(quote.Ask), Mark: financialDecimal(quote.Mark), Last: financialDecimal(quote.Last), Feed: "schwab_market_data", Quality: "BROKER_REALTIME", ObservedAt: quote.ProviderTimestamp, HistoryStatus: "UNAVAILABLE", LiquidityStatus: "UNAVAILABLE"})
		}
		return facts, nil
	}
	if s.markets == nil {
		return nil, ErrInvalid
	}
	batch, _, err := s.markets.CryptoMarkets(ctx, "USD", symbols)
	if err != nil {
		return nil, err
	}
	if len(batch.UnavailableSymbols) > 0 || len(batch.Markets) != len(symbols) {
		return nil, ErrEvaluationNoEligibleContracts
	}
	for _, market := range batch.Markets {
		if !freshMarketTimestamp(market.Provenance.ProviderTimestamp, now) {
			return nil, ErrEvaluationMarketDataStale
		}
		fact := neural.ShadowMarketFact{Symbol: strings.ToUpper(market.Symbol), AssetClass: "CRYPTO", Currency: "USD", Bid: marketDecimal(market.Bid), Ask: marketDecimal(market.Ask), Mark: string(market.CurrentPrice), Last: string(market.CurrentPrice), ChangePercent24H: marketDecimal(market.ChangePercent24H), Volume24H: marketDecimal(market.Volume24H), Feed: market.Provenance.Feed, Quality: string(market.Provenance.Quality), ObservedAt: market.Provenance.ProviderTimestamp, HistoryStatus: "UNAVAILABLE", HistoryGranularitySeconds: aiHistoryGranularitySeconds, HistoryExpectedIntervals: aiHistoryExpectedIntervals, LiquidityStatus: "UNAVAILABLE"}
		if stats, _, statsErr := s.markets.CryptoVenueStats(ctx, fact.Symbol, "USD"); statsErr == nil {
			fact.Volume24H = string(stats.Volume24H)
			fact.ChangePercent24H = percentageChange(string(stats.Open), string(stats.Last))
		}
		if series, _, historyErr := s.markets.RecentCryptoCandles(ctx, fact.Symbol, "USD", aiHistoryGranularitySeconds, aiHistoryExpectedIntervals); historyErr == nil {
			addAIHistoryFacts(&fact, series, now)
		}
		if liquidity, _, liquidityErr := s.markets.CryptoLiquidity(ctx, fact.Symbol, "USD", 10); liquidityErr == nil {
			addAILiquidityFacts(&fact, liquidity, now)
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

// addAILiquidityFacts preserves only bounded, point-in-time Coinbase book
// evidence. The aggregate depths are exact price-times-size sums across the
// returned levels; they are context, not a promise that those levels execute.
func addAILiquidityFacts(fact *neural.ShadowMarketFact, snapshot marketintelligence.CryptoLiquiditySnapshot, now time.Time) {
	if fact == nil || !strings.EqualFold(snapshot.Symbol, fact.Symbol) || !strings.EqualFold(snapshot.Currency, fact.Currency) || snapshot.Depth != 10 || len(snapshot.Bids) == 0 || len(snapshot.Bids) > 10 || len(snapshot.Asks) == 0 || len(snapshot.Asks) > 10 {
		return
	}
	fact.LiquidityFeed = snapshot.Provenance.Feed
	fact.LiquidityQuality = string(snapshot.Provenance.Quality)
	observedAt := snapshot.Provenance.ProviderTimestamp
	fact.LiquidityObservedAt = &observedAt
	if !freshMarketTimestamp(observedAt, now) {
		fact.LiquidityStatus = "STALE"
		return
	}
	bidDepth, bidOK := aiBookDepthUSD(snapshot.Bids)
	askDepth, askOK := aiBookDepthUSD(snapshot.Asks)
	if !bidOK || !askOK {
		fact.LiquidityStatus = "UNAVAILABLE"
		fact.LiquidityFeed = ""
		fact.LiquidityQuality = ""
		fact.LiquidityObservedAt = nil
		return
	}
	fact.LiquidityStatus = "AVAILABLE"
	fact.SpreadBPS = string(snapshot.SpreadBPS)
	fact.BidDepthUSD = bidDepth
	fact.AskDepthUSD = askDepth
	fact.BidLevels = len(snapshot.Bids)
	fact.AskLevels = len(snapshot.Asks)
}

func aiBookDepthUSD(levels []marketintelligence.CryptoBookLevel) (string, bool) {
	total := new(big.Rat)
	for _, level := range levels {
		price, priceOK := new(big.Rat).SetString(string(level.Price))
		size, sizeOK := new(big.Rat).SetString(string(level.Size))
		if !priceOK || !sizeOK || price.Sign() <= 0 || size.Sign() <= 0 {
			return "", false
		}
		total.Add(total, new(big.Rat).Mul(price, size))
	}
	return total.FloatString(10), total.Sign() > 0
}

// addAIHistoryFacts derives only fully covered trailing windows from exact
// provider candles. It never fills a missing bucket or treats a partial window
// as complete market history.
func addAIHistoryFacts(fact *neural.ShadowMarketFact, series marketintelligence.CryptoCandleSeries, now time.Time) {
	if fact == nil || series.GranularitySeconds != aiHistoryGranularitySeconds || series.ExpectedIntervals != aiHistoryExpectedIntervals || len(series.Candles) == 0 {
		return
	}
	fact.HistoryFeed = series.Provenance.Feed
	fact.HistoryQuality = string(series.Provenance.Quality)
	observedAt := series.Provenance.ProviderTimestamp
	fact.HistoryObservedAt = &observedAt

	latest := series.Candles[len(series.Candles)-1]
	if latest.Start.Before(now.Add(-aiHistoryFreshness)) || latest.Start.After(now.Add(marketDataMaxFutureOffset)) {
		fact.HistoryStatus = "STALE"
		return
	}
	contiguous := 1
	step := time.Duration(series.GranularitySeconds) * time.Second
	for index := len(series.Candles) - 1; index > 0; index-- {
		if !series.Candles[index-1].Start.Equal(series.Candles[index].Start.Add(-step)) {
			break
		}
		contiguous++
	}
	fact.HistoryContiguousIntervals = contiguous
	if contiguous < 4 {
		return
	}
	fact.HistoryStatus = "PARTIAL"
	fact.ChangePercent1H = percentageChange(string(series.Candles[len(series.Candles)-4].Open), string(latest.Close))
	if contiguous >= 24 {
		fact.ChangePercent6H = percentageChange(string(series.Candles[len(series.Candles)-24].Open), string(latest.Close))
	}
	if contiguous >= aiHistoryExpectedIntervals {
		fact.HistoryStatus = "COMPLETE"
	}
}

func aiMarketPrice(markets []neural.ShadowMarketFact, symbol string) (string, bool) {
	for _, market := range markets {
		if !strings.EqualFold(market.Symbol, symbol) {
			continue
		}
		for _, candidate := range []string{market.Mark, market.Last, market.Bid, market.Ask} {
			value, ok := new(big.Rat).SetString(candidate)
			if ok && value.Sign() > 0 {
				return candidate, true
			}
		}
	}
	return "", false
}

func percentageChange(open, last string) string {
	openValue, openOK := new(big.Rat).SetString(open)
	lastValue, lastOK := new(big.Rat).SetString(last)
	if !openOK || !lastOK || openValue.Sign() <= 0 || lastValue.Sign() <= 0 {
		return ""
	}
	change := new(big.Rat).Sub(lastValue, openValue)
	change.Mul(change, big.NewRat(100, 1))
	return change.Quo(change, openValue).FloatString(10)
}

func financialDecimal(value *financial.Decimal) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
func marketDecimal(value *marketintelligence.Decimal) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
func findAIMarket(markets []neural.ShadowMarketFact, symbol string) (neural.ShadowMarketFact, bool) {
	for _, market := range markets {
		if strings.EqualFold(market.Symbol, symbol) {
			return market, true
		}
	}
	return neural.ShadowMarketFact{}, false
}

func aiExecutionPrice(market neural.ShadowMarketFact, side string) (string, string, bool) {
	type candidate struct{ value, basis string }
	prices := []candidate{{market.Ask, "ASK"}, {market.Mark, "MARK_FALLBACK"}, {market.Last, "LAST_FALLBACK"}}
	if side == "SELL" {
		prices = []candidate{{market.Bid, "BID"}, {market.Mark, "MARK_FALLBACK"}, {market.Last, "LAST_FALLBACK"}}
	} else if side != "BUY" {
		return "", "", false
	}
	for _, price := range prices {
		value, ok := new(big.Rat).SetString(price.value)
		if ok && value.Sign() > 0 {
			return price.value, price.basis, true
		}
	}
	return "", "", false
}
func oldestAIMarketTimestamp(markets []neural.ShadowMarketFact) time.Time {
	var result time.Time
	for _, market := range markets {
		if result.IsZero() || market.ObservedAt.Before(result) {
			result = market.ObservedAt
		}
	}
	return result
}

func validAIShadowDecision(decision neural.ShadowDecision, allowed []string, maximum string) bool {
	if decision.Confidence != "LOW" && decision.Confidence != "MEDIUM" && decision.Confidence != "HIGH" {
		return false
	}
	if strings.TrimSpace(decision.Thesis) == "" || len([]byte(decision.Thesis)) > 1000 || len(decision.RiskFlags) > 8 || len(decision.Limitations) > 8 {
		return false
	}
	for _, values := range [][]string{decision.RiskFlags, decision.Limitations} {
		seen := map[string]bool{}
		for _, value := range values {
			if strings.TrimSpace(value) == "" || len([]byte(value)) > 500 || seen[value] {
				return false
			}
			seen[value] = true
		}
	}
	if decision.Decision == "ABSTAIN" {
		return decision.Symbol == "NONE" && decision.Side == "NONE" && decision.ProposedNotional == "0"
	}
	if decision.Decision != "PROPOSE" || (decision.Side != "BUY" && decision.Side != "SELL") {
		return false
	}
	notional, ok := new(big.Rat).SetString(decision.ProposedNotional)
	limit, limitOK := new(big.Rat).SetString(maximum)
	if !ok || !limitOK || notional.Sign() <= 0 || notional.Cmp(limit) > 0 {
		return false
	}
	for _, symbol := range allowed {
		if symbol == decision.Symbol {
			return true
		}
	}
	return false
}

func validAIRecentDecision(decision neural.ShadowRecentDecision) bool {
	if decision.OccurredAt.IsZero() {
		return false
	}
	if decision.Decision == "ABSTAIN" {
		return decision.Symbol == "NONE" && decision.Side == "NONE" && decision.Disposition == "ABSTAINED"
	}
	if decision.Decision != "PROPOSE" || (decision.Side != "BUY" && decision.Side != "SELL") || (decision.Disposition != "WOULD_HAVE_SUBMITTED" && decision.Disposition != "SIMULATED_FILLED" && decision.Disposition != "SIMULATED_REJECTED" && decision.Disposition != "HELD_BY_CONTROLS") || len(decision.Symbol) == 0 || len(decision.Symbol) > 16 {
		return false
	}
	for index, value := range []byte(decision.Symbol) {
		if (value >= 'A' && value <= 'Z') || (index > 0 && value >= '0' && value <= '9') || (index > 0 && (value == '.' || value == '-')) {
			continue
		}
		return false
	}
	return true
}

func floorRat(value *big.Rat, precision int) string {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(precision)), nil)
	scaled := new(big.Rat).Mul(value, new(big.Rat).SetInt(scale))
	integer := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	return new(big.Rat).Quo(new(big.Rat).SetInt(integer), new(big.Rat).SetInt(scale)).FloatString(precision)
}

func moneyAmount(value *financial.Money, currency string) (string, bool) {
	if value == nil || value.Currency != currency {
		return "", false
	}
	if _, ok := new(big.Rat).SetString(string(value.Amount)); !ok {
		return "", false
	}
	return string(value.Amount), true
}
