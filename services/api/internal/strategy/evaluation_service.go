package strategy

import (
	"context"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/risk"
)

type PaperEvaluationFacts struct {
	Cash, CurrentExposure string
	Positions             []Position
	RiskPositions         []risk.Position
}

type EvaluationFacts struct {
	Paper        *PaperEvaluationFacts
	Breakers     []risk.CircuitBreaker
	ActionsToday int
}

type EvaluationStore interface {
	Repository
	Get(context.Context, string, string) (Instance, error)
	EvaluationFacts(context.Context, Instance, time.Time) (EvaluationFacts, error)
}

type EvaluationAutomation interface {
	Get(context.Context, authorization.Principal, string) (automation.Mandate, error)
	GetBucket(context.Context, authorization.Principal, string) (automation.CapitalBucket, error)
}

type EvaluationFinancial interface {
	GetAccount(context.Context, authorization.Principal, string) (financial.FinancialAccount, error)
	GetBalances(context.Context, authorization.Principal, string) (financial.Balances, error)
	GetPositions(context.Context, authorization.Principal, string) ([]financial.Position, error)
	GetQuote(context.Context, authorization.Principal, string, string) (financial.Quote, error)
	GetOptionChain(context.Context, authorization.Principal, string, financial.OptionChainRequest) (financial.OptionChain, error)
}

type EvaluationService struct {
	store        EvaluationStore
	automation   EvaluationAutomation
	financial    EvaluationFinancial
	now          func() time.Time
	orchestrator *Orchestrator
}

func NewEvaluationService(store EvaluationStore, automations EvaluationAutomation, finances EvaluationFinancial) *EvaluationService {
	return &EvaluationService{
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
	if instance.Status != "ACTIVE" || (instance.ExecutionMode != Paper && instance.ExecutionMode != Shadow) {
		return EvaluationOutcome{}, ErrInvalid
	}
	mandate, err := s.automation.Get(ctx, principal, instance.AutomationMandateID)
	if err != nil {
		return EvaluationOutcome{}, ErrNotFound
	}
	if mandate.ID != instance.AutomationMandateID || mandate.UserID != principal.UserID || mandate.FinancialAccountID != instance.FinancialAccountID || mandate.CapitalBucketID != instance.CapitalBucketID || mandate.CurrentVersion != instance.MandateVersion || mandate.Status != "READY" || mandate.StrategyIdentifier == nil || *mandate.StrategyIdentifier != instance.StrategyIdentifier || mandate.ExecutionMode != string(instance.ExecutionMode) {
		return EvaluationOutcome{}, ErrInvalid
	}
	parameters, err := ParseParameters(mandate.StrategyParameters)
	if err != nil {
		return EvaluationOutcome{}, err
	}
	bucket, err := s.automation.GetBucket(ctx, principal, instance.CapitalBucketID)
	if err != nil || bucket.UserID != principal.UserID || bucket.FinancialAccountID != instance.FinancialAccountID || bucket.Status != "ACTIVE" || bucket.IsReserve {
		return EvaluationOutcome{}, ErrInvalid
	}
	account, err := s.financial.GetAccount(ctx, principal, instance.FinancialAccountID)
	if err != nil || account.ID != instance.FinancialAccountID || account.UserID != principal.UserID || account.Status != "active" {
		if err != nil {
			return EvaluationOutcome{}, err
		}
		return EvaluationOutcome{}, ErrInvalid
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
		ID:                          mandate.ID,
		UserID:                      mandate.UserID,
		AccountID:                   mandate.FinancialAccountID,
		BucketID:                    instance.CapitalBucketID,
		Status:                      mandate.Status,
		AutomationType:              mandate.AutomationType,
		AutonomyLevel:               mandate.AutonomyLevel,
		ExecutionMode:               mandate.ExecutionMode,
		Version:                     mandate.CurrentVersion,
		StrategyIdentifier:          mandate.StrategyIdentifier,
		EffectiveFrom:               mandate.EffectiveFrom,
		EffectiveUntil:              mandate.EffectiveUntil,
		AllowedSymbols:              mandate.AllowedUniverse.Symbols,
		ProhibitedSymbols:           mandate.ProhibitedUniverse.Symbols,
		UniverseIDs:                 mandate.AllowedUniverse.UniverseIDs,
		MarginAllowed:               mandate.MarginAllowed,
		OptionsAllowed:              mandate.OptionsAllowed,
		MaxCapitalDeployed:          mandate.Risk.MaxCapitalDeployed,
		MaxSinglePositionAmount:     mandate.Risk.MaxSinglePositionAmount,
		MaxSinglePositionPercentage: mandate.Risk.MaxSinglePositionPercentage,
		MaxDailyLoss:                mandate.Risk.MaxDailyLoss,
		MinimumCashReserve:          mandate.Risk.MinimumCashReserve,
		MaxTradesPerDay:             mandate.Risk.MaxTradesPerDay,
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
	for index, symbol := range parameters.Symbols {
		quote, err := s.financial.GetQuote(ctx, principal, instance.FinancialAccountID, symbol)
		if err != nil {
			return MarketSnapshot{}, err
		}
		if !freshMarketTimestamp(quote.ProviderTimestamp, now) {
			return MarketSnapshot{}, ErrInvalid
		}
		chain, err := s.financial.GetOptionChain(ctx, principal, instance.FinancialAccountID, financial.OptionChainRequest{Symbol: symbol, ContractType: kind, StrikeCount: 50, FromDate: now.AddDate(0, 0, parameters.MinimumDTE), ToDate: now.AddDate(0, 0, parameters.MaximumDTE)})
		if err != nil {
			return MarketSnapshot{}, err
		}
		if !freshMarketTimestamp(chain.ProviderTimestamp, now) {
			return MarketSnapshot{}, ErrInvalid
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
			if !strings.EqualFold(contract.Underlying, symbol) || !strings.EqualFold(contract.PutCall, kind) || !freshMarketTimestamp(contract.ProviderTimestamp, now) {
				continue
			}
			market.Options = append(market.Options, OptionCandidate{Underlying: strings.ToUpper(contract.Underlying), OptionType: strings.ToUpper(contract.PutCall), Strike: string(contract.Strike), Expiration: contract.Expiration, Bid: decimalPointer(contract.Bid), Ask: decimalPointer(contract.Ask), Mark: decimalPointer(contract.Mark), Delta: decimalPointer(contract.Delta), ImpliedVolatility: decimalPointer(contract.ImpliedVolatility), OpenInterest: contract.OpenInterest, Volume: contract.Volume, Timestamp: contract.ProviderTimestamp})
		}
	}
	if len(market.Options) == 0 {
		return MarketSnapshot{}, ErrInvalid
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
			return AccountSnapshot{}, risk.AccountRiskSnapshot{}, ErrInvalid
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
		return AccountSnapshot{}, risk.AccountRiskSnapshot{}, ErrInvalid
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
		riskPositions = append(riskPositions, risk.Position{Instrument: strings.ToUpper(position.Symbol), Exposure: exposure.FloatString(10)})
	}
	return AccountSnapshot{Timestamp: now, AvailableCash: available, Positions: strategyPositions}, risk.AccountRiskSnapshot{AccountID: account.ID, Currency: account.BaseCurrency, Timestamp: now, Cash: cash, AvailableCash: available, BuyingPower: buyingPower, CurrentExposure: totalExposure.FloatString(10), Positions: riskPositions, Options: capabilities("options"), Margin: capabilities("margin")}, nil
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
