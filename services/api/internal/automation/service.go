package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

var (
	ErrForbidden = errors.New("automation entitlement required")
	ErrInvalid   = errors.New("invalid automation configuration")
	ErrNotFound  = errors.New("automation resource not found")
	ErrConflict  = errors.New("automation version or dependency conflict")
)

type AccountFacts struct {
	Owned           bool
	Provider        string
	Options, Margin string
}
type AIFacts struct {
	Owned, Active, ModelValid bool
	Provider                  string
}
type Store interface {
	AccountFacts(context.Context, string, string) (AccountFacts, error)
	AIFacts(context.Context, string, string, string) (AIFacts, error)
	CreateBucket(context.Context, string, CreateBucketCommand) (CapitalBucket, error)
	FixedAllocated(context.Context, string, string) (*big.Rat, error)
	ListBuckets(context.Context, string) ([]CapitalBucket, error)
	GetBucket(context.Context, string, string) (CapitalBucket, error)
	UpdateBucket(context.Context, string, string, CreateBucketCommand) (CapitalBucket, error)
	DeleteBucket(context.Context, string, string) error
	CreateMandate(context.Context, string, MandateCommand, bool) (Mandate, error)
	ListMandates(context.Context, string) ([]Mandate, error)
	GetMandate(context.Context, string, string) (Mandate, error)
	UpdateMandate(context.Context, string, string, int, MandateCommand, bool, string) (Mandate, error)
	Transition(context.Context, string, string, int, string, string) (Mandate, error)
	Versions(context.Context, string, string) ([]Version, error)
	Version(context.Context, string, string, int) (Version, error)
}
type Auditor interface {
	Record(context.Context, *string, string, map[string]any) error
}
type Service struct {
	store Store
	audit Auditor
}

func NewService(s Store, a Auditor) *Service { return &Service{s, a} }
func allowed(p authorization.Principal) bool {
	return authorization.CanUseAutomation(p) && authorization.CanConnectFinancialAccounts(p)
}
func (s *Service) auditEvent(ctx context.Context, p authorization.Principal, a string, m map[string]any) {
	if s.audit != nil {
		_ = s.audit.Record(ctx, &p.UserID, a, m)
	}
}
func decimal(v string, positive bool) (*big.Rat, bool) {
	if !regexp.MustCompile(`^\d+(\.\d{1,10})?$`).MatchString(v) {
		return nil, false
	}
	r, ok := new(big.Rat).SetString(v)
	return r, ok && (!positive || r.Sign() > 0)
}

var strategySymbol = regexp.MustCompile(`^[A-Z][A-Z0-9.-]{0,9}$`)

var aiShadowModels = map[string]string{
	"gpt-5.6-luna":              "openai",
	"gpt-5.6-terra":             "openai",
	"gpt-5.6-sol":               "openai",
	"claude-haiku-4-5-20251001": "anthropic",
	"claude-sonnet-5":           "anthropic",
	"claude-opus-5":             "anthropic",
	"gemini-3.5-flash":          "gemini",
	"gemini-3.6-flash":          "gemini",
	"gemini-3.7-flash":          "gemini",
}

func ParseAIShadowParameters(raw json.RawMessage) (AIShadowParameters, error) {
	var parameters AIShadowParameters
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parameters); err != nil {
		return AIShadowParameters{}, ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return AIShadowParameters{}, ErrInvalid
	}
	parameters.Objective = strings.TrimSpace(parameters.Objective)
	maximum, ok := decimal(parameters.MaxProposalNotional, true)
	if parameters.Objective == "" || len([]byte(parameters.Objective)) > 500 || !ok || maximum.Cmp(big.NewRat(1_000_000_000, 1)) > 0 {
		return AIShadowParameters{}, ErrInvalid
	}
	return parameters, nil
}

func ValidateStrategyParameters(p StrategyParameters) error {
	if len(p.Symbols) == 0 || len(p.Symbols) > 10 || p.MinimumDTE < 0 || p.MaximumDTE < p.MinimumDTE || p.MaximumDTE > 730 || p.MaximumContracts < 1 || p.MaximumContracts > 100 {
		return ErrInvalid
	}
	lo, loOK := decimal(p.TargetDeltaMin, false)
	hi, hiOK := decimal(p.TargetDeltaMax, false)
	target, targetOK := decimal(p.TargetDelta, false)
	if !loOK || !hiOK || !targetOK || lo.Sign() < 0 || hi.Cmp(big.NewRat(1, 1)) > 0 || hi.Cmp(lo) < 0 || target.Cmp(lo) < 0 || target.Cmp(hi) > 0 || p.AssignmentHandlingPolicy != "continue_wheel" {
		return ErrInvalid
	}
	if p.MinimumPremium != nil {
		minimum, ok := decimal(*p.MinimumPremium, false)
		if !ok || minimum.Sign() < 0 {
			return ErrInvalid
		}
	}
	seen := map[string]bool{}
	for _, symbol := range p.Symbols {
		if !strategySymbol.MatchString(symbol) || seen[symbol] {
			return ErrInvalid
		}
		seen[symbol] = true
	}
	return nil
}

func ParseStrategyParameters(raw json.RawMessage) (StrategyParameters, error) {
	var parameters StrategyParameters
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parameters); err != nil {
		return StrategyParameters{}, ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return StrategyParameters{}, ErrInvalid
	}
	for index, symbol := range parameters.Symbols {
		parameters.Symbols[index] = strings.ToUpper(strings.TrimSpace(symbol))
	}
	if err := ValidateStrategyParameters(parameters); err != nil {
		return StrategyParameters{}, err
	}
	return parameters, nil
}

func ValidateScheduleConditions(schedule ScheduleConditions) error {
	if !schedule.Enabled {
		if schedule.IntervalMinutes != 0 || schedule.Session != "" || schedule.Notifications != (ScheduleNotifications{}) {
			return ErrInvalid
		}
		return nil
	}
	if schedule.IntervalMinutes < 30 || schedule.IntervalMinutes > 1440 || (schedule.Session != "US_EQUITIES_REGULAR" && schedule.Session != "CONTINUOUS") {
		return ErrInvalid
	}
	return nil
}

func ParseScheduleConditions(raw json.RawMessage) (ScheduleConditions, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var schedule ScheduleConditions
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&schedule); err != nil {
		return ScheduleConditions{}, ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ScheduleConditions{}, ErrInvalid
	}
	if err := ValidateScheduleConditions(schedule); err != nil {
		return ScheduleConditions{}, err
	}
	return schedule, nil
}
func (s *Service) CreateBucket(ctx context.Context, p authorization.Principal, c CreateBucketCommand) (CapitalBucket, error) {
	if !allowed(p) {
		return CapitalBucket{}, ErrForbidden
	}
	f, e := s.store.AccountFacts(ctx, p.UserID, c.FinancialAccountID)
	if e != nil || !f.Owned {
		return CapitalBucket{}, ErrNotFound
	}
	c.Name = strings.TrimSpace(c.Name)
	c.Currency = strings.ToUpper(c.Currency)
	v, ok := decimal(c.AllocationValue, true)
	if c.Name == "" || len(c.Name) > 100 || len(c.Currency) != 3 || !ok {
		return CapitalBucket{}, ErrInvalid
	}
	if c.AllocationType != "FIXED_AMOUNT" && c.AllocationType != "PERCENT_OF_AVAILABLE_CASH" && c.AllocationType != "PERCENT_OF_BUYING_POWER" {
		return CapitalBucket{}, ErrInvalid
	}
	if c.AllocationType != "FIXED_AMOUNT" && v.Cmp(big.NewRat(100, 1)) > 0 {
		return CapitalBucket{}, ErrInvalid
	}
	if _, ok = decimal(c.ProtectedAmount, false); !ok {
		return CapitalBucket{}, ErrInvalid
	}
	if c.AllocationType == "FIXED_AMOUNT" && c.AllocationLimit != nil {
		limit, lok := decimal(*c.AllocationLimit, true)
		used, e := s.store.FixedAllocated(ctx, p.UserID, c.FinancialAccountID)
		if !lok || e != nil || new(big.Rat).Add(used, v).Cmp(limit) > 0 {
			return CapitalBucket{}, ErrConflict
		}
	}
	b, e := s.store.CreateBucket(ctx, p.UserID, c)
	if e == nil {
		s.auditEvent(ctx, p, "capital_bucket.created", map[string]any{"capital_bucket_id": b.ID, "account_id": b.FinancialAccountID})
	}
	return b, e
}
func (s *Service) ListBuckets(c context.Context, p authorization.Principal) ([]CapitalBucket, error) {
	if !allowed(p) {
		return nil, ErrForbidden
	}
	return s.store.ListBuckets(c, p.UserID)
}
func (s *Service) GetBucket(c context.Context, p authorization.Principal, id string) (CapitalBucket, error) {
	if !allowed(p) {
		return CapitalBucket{}, ErrForbidden
	}
	return s.store.GetBucket(c, p.UserID, id)
}
func (s *Service) UpdateBucket(c context.Context, p authorization.Principal, id string, x CreateBucketCommand) (CapitalBucket, error) {
	if !allowed(p) {
		return CapitalBucket{}, ErrForbidden
	}
	old, e := s.store.GetBucket(c, p.UserID, id)
	if e != nil {
		return CapitalBucket{}, ErrNotFound
	}
	if x.FinancialAccountID != old.FinancialAccountID {
		return CapitalBucket{}, ErrInvalid
	}
	x.Name = strings.TrimSpace(x.Name)
	x.Currency = strings.ToUpper(x.Currency)
	v, ok := decimal(x.AllocationValue, true)
	if x.Name == "" || len(x.Name) > 100 || len(x.Currency) != 3 || !ok {
		return CapitalBucket{}, ErrInvalid
	}
	if x.AllocationType != "FIXED_AMOUNT" && x.AllocationType != "PERCENT_OF_AVAILABLE_CASH" && x.AllocationType != "PERCENT_OF_BUYING_POWER" {
		return CapitalBucket{}, ErrInvalid
	}
	if x.AllocationType != "FIXED_AMOUNT" && v.Cmp(big.NewRat(100, 1)) > 0 {
		return CapitalBucket{}, ErrInvalid
	}
	if _, ok = decimal(x.ProtectedAmount, false); !ok {
		return CapitalBucket{}, ErrInvalid
	}
	if x.AllocationLimit != nil {
		limit, valid := decimal(*x.AllocationLimit, true)
		if !valid {
			return CapitalBucket{}, ErrInvalid
		}
		if x.AllocationType == "FIXED_AMOUNT" {
			used, allocationErr := s.store.FixedAllocated(c, p.UserID, x.FinancialAccountID)
			oldAllocation, oldValid := decimal(old.AllocationValue, true)
			if allocationErr != nil || !oldValid {
				return CapitalBucket{}, ErrConflict
			}
			if old.Status == "ACTIVE" && old.AllocationType == "FIXED_AMOUNT" {
				used = new(big.Rat).Sub(used, oldAllocation)
			}
			if new(big.Rat).Add(used, v).Cmp(limit) > 0 {
				return CapitalBucket{}, ErrConflict
			}
		}
	}
	b, e := s.store.UpdateBucket(c, p.UserID, id, x)
	if e == nil {
		s.auditEvent(c, p, "capital_bucket.changed", map[string]any{"capital_bucket_id": id, "account_id": old.FinancialAccountID})
	}
	return b, e
}
func (s *Service) DeleteBucket(c context.Context, p authorization.Principal, id string) error {
	if !allowed(p) {
		return ErrForbidden
	}
	e := s.store.DeleteBucket(c, p.UserID, id)
	if e == nil {
		s.auditEvent(c, p, "capital_bucket.archived", map[string]any{"capital_bucket_id": id})
	}
	return e
}
func (s *Service) validate(ctx context.Context, p authorization.Principal, c MandateCommand, ready bool) (bool, error) {
	if !allowed(p) {
		return false, ErrForbidden
	}
	af, e := s.store.AccountFacts(ctx, p.UserID, c.FinancialAccountID)
	if e != nil || !af.Owned {
		return false, ErrNotFound
	}
	b, e := s.store.GetBucket(ctx, p.UserID, c.CapitalBucketID)
	if e != nil || b.FinancialAccountID != c.FinancialAccountID || b.Status != "ACTIVE" || b.IsReserve {
		return false, ErrInvalid
	}
	if _, ok := map[string]bool{"AI_AUTONOMOUS": true, "STRATEGY": true, "HYBRID": true}[c.AutomationType]; !ok {
		return false, ErrInvalid
	}
	strategy := c.AutomationType == "STRATEGY" || c.AutomationType == "HYBRID"
	ai := c.AutomationType == "AI_AUTONOMOUS" || c.AutomationType == "HYBRID"
	var meta Strategy
	if strategy {
		if c.StrategyIdentifier == nil {
			return false, ErrInvalid
		}
		var ok bool
		meta, ok = Strategies[*c.StrategyIdentifier]
		if !ok {
			return false, ErrInvalid
		}
	}
	if ai {
		if !authorization.CanUseNeuralEngine(p) || c.AIProviderConnectionID == nil || c.AIModelID == nil {
			return false, ErrForbidden
		}
		facts, e := s.store.AIFacts(ctx, p.UserID, *c.AIProviderConnectionID, *c.AIModelID)
		expectedProvider, supported := aiShadowModels[*c.AIModelID]
		if e != nil || !facts.Owned || !facts.Active || !facts.ModelValid || !supported || facts.Provider != expectedProvider {
			return false, ErrInvalid
		}
	}
	if _, ok := map[string]bool{"RESEARCH_ONLY": true, "SUGGEST": true, "CONFIRM_EACH": true, "STRATEGY_AUTONOMOUS": true, "FULL_AUTONOMOUS": true}[c.AutonomyLevel]; !ok {
		return false, ErrInvalid
	}
	if c.AutonomyLevel == "STRATEGY_AUTONOMOUS" && !strategy {
		return false, ErrInvalid
	}
	if ready && c.AutonomyLevel == "FULL_AUTONOMOUS" && c.AutomationType == "STRATEGY" {
		return false, ErrInvalid
	}
	if c.AutomationType == "AI_AUTONOMOUS" && (c.StrategyIdentifier != nil || (c.ExecutionMode != "PAPER" && c.ExecutionMode != "SHADOW") || c.AutonomyLevel != "FULL_AUTONOMOUS" || c.MarginAllowed || c.OptionsAllowed) {
		return false, ErrInvalid
	}
	if c.AutomationType == "AI_AUTONOMOUS" && (c.Risk.MaxTradesPerDay == nil || *c.Risk.MaxTradesPerDay < 1 || *c.Risk.MaxTradesPerDay > 48 || c.Risk.MaxDailyLoss != nil) {
		return false, ErrInvalid
	}
	if _, ok := map[string]bool{"BACKTEST": true, "PAPER": true, "SHADOW": true, "LIVE": true}[c.ExecutionMode]; !ok {
		return false, ErrInvalid
	}
	if c.PaperOptionsSimulationAttested && (c.AutomationType != "STRATEGY" || c.ExecutionMode != "PAPER" || !c.OptionsAllowed || !meta.OptionsRequired) {
		return false, ErrInvalid
	}
	if len(c.StrategyParameters) > 0 && !json.Valid(c.StrategyParameters) {
		return false, ErrInvalid
	}
	if ready && strategy {
		if _, e = ParseStrategyParameters(c.StrategyParameters); e != nil {
			return false, e
		}
	}
	if ready && c.AutomationType == "AI_AUTONOMOUS" {
		if _, e = ParseAIShadowParameters(c.StrategyParameters); e != nil || len(c.AllowedUniverse.Symbols) == 0 || len(c.AllowedUniverse.Symbols) > 8 || len(c.AllowedUniverse.UniverseIDs) != 0 {
			return false, ErrInvalid
		}
		seen := map[string]bool{}
		for _, symbol := range c.AllowedUniverse.Symbols {
			canonical := strings.ToUpper(strings.TrimSpace(symbol))
			if symbol != canonical || !strategySymbol.MatchString(canonical) || seen[canonical] {
				return false, ErrInvalid
			}
			seen[canonical] = true
		}
	}
	schedule, err := ParseScheduleConditions(c.ScheduleConditions)
	if err != nil {
		return false, err
	}
	if schedule.Notifications.ReconciliationReviewNeeded && (c.AutomationType != "AI_AUTONOMOUS" || c.ExecutionMode != "SHADOW") {
		return false, ErrInvalid
	}
	validStrategySchedule := c.AutomationType == "STRATEGY" && c.AutonomyLevel == "STRATEGY_AUTONOMOUS" && (c.ExecutionMode == "PAPER" || c.ExecutionMode == "SHADOW") && schedule.Session == "US_EQUITIES_REGULAR"
	validAIAutonomousSchedule := c.AutomationType == "AI_AUTONOMOUS" && c.AutonomyLevel == "FULL_AUTONOMOUS" && (c.ExecutionMode == "PAPER" || c.ExecutionMode == "SHADOW") && ((af.Provider == "coinbase" && schedule.Session == "CONTINUOUS") || (af.Provider == "schwab" && schedule.Session == "US_EQUITIES_REGULAR"))
	if schedule.Enabled && !validStrategySchedule && !validAIAutonomousSchedule {
		return false, ErrInvalid
	}
	for _, v := range []*string{c.Risk.MaxCapitalDeployed, c.Risk.MaxSinglePositionAmount, c.Risk.MaxSinglePositionPercentage, c.Risk.MaxDailyLoss, c.Risk.MinimumCashReserve} {
		if v != nil {
			r, ok := decimal(*v, false)
			if !ok || r.Sign() < 0 {
				return false, ErrInvalid
			}
		}
	}
	if c.Risk.MaxSinglePositionPercentage != nil {
		r, _ := decimal(*c.Risk.MaxSinglePositionPercentage, false)
		if r.Cmp(big.NewRat(100, 1)) > 0 {
			return false, ErrInvalid
		}
	}
	if c.AutomationType == "AI_AUTONOMOUS" && c.Risk.MaxCapitalDeployed != nil && c.Risk.MaxSinglePositionAmount != nil {
		capital, _ := decimal(*c.Risk.MaxCapitalDeployed, false)
		position, _ := decimal(*c.Risk.MaxSinglePositionAmount, false)
		if position.Cmp(capital) > 0 {
			return false, ErrInvalid
		}
	}
	if c.Risk.MaxTradesPerDay != nil && *c.Risk.MaxTradesPerDay < 0 {
		return false, ErrInvalid
	}
	unverified := false
	if meta.OptionsRequired {
		if af.Options == "UNSUPPORTED" {
			return false, ErrInvalid
		}
		if af.Options != "SUPPORTED" {
			unverified = true
		}
	}
	return unverified, nil
}
func (s *Service) Create(ctx context.Context, p authorization.Principal, c MandateCommand) (Mandate, error) {
	if !allowed(p) {
		return Mandate{}, ErrForbidden
	}
	if c.PaperOptionsSimulationAttested {
		return Mandate{}, ErrInvalid
	}
	u, e := s.validate(ctx, p, c, false)
	if e != nil {
		return Mandate{}, e
	}
	m, e := s.store.CreateMandate(ctx, p.UserID, c, u)
	if e == nil {
		s.auditEvent(ctx, p, "automation_mandate.created", map[string]any{"mandate_id": m.ID, "version": m.CurrentVersion, "account_id": m.FinancialAccountID, "automation_type": m.AutomationType, "capital_bucket_id": m.CapitalBucketID, "source": "UI"})
		s.auditEvent(ctx, p, "automation_mandate.version_created", map[string]any{"mandate_id": m.ID, "version": m.CurrentVersion, "source": "UI"})
	}
	return m, e
}
func (s *Service) List(c context.Context, p authorization.Principal) ([]Mandate, error) {
	if !allowed(p) {
		return nil, ErrForbidden
	}
	return s.store.ListMandates(c, p.UserID)
}
func (s *Service) Get(c context.Context, p authorization.Principal, id string) (Mandate, error) {
	if !allowed(p) {
		return Mandate{}, ErrForbidden
	}
	return s.store.GetMandate(c, p.UserID, id)
}
func (s *Service) Update(c context.Context, p authorization.Principal, id string, expected int, cmd MandateCommand) (Mandate, error) {
	if !allowed(p) {
		return Mandate{}, ErrForbidden
	}
	old, e := s.store.GetMandate(c, p.UserID, id)
	if e != nil {
		return Mandate{}, ErrNotFound
	}
	if cmd.PaperOptionsSimulationAttested != old.PaperOptionsSimulationAttested {
		return Mandate{}, ErrInvalid
	}
	u, e := s.validate(c, p, cmd, false)
	if e != nil {
		return Mandate{}, e
	}
	return s.store.UpdateMandate(c, p.UserID, id, expected, cmd, u, "UI")
}

func (s *Service) UpdateAutonomy(c context.Context, p authorization.Principal, id string, expected int, autonomyLevel string) (Mandate, error) {
	if !allowed(p) {
		return Mandate{}, ErrForbidden
	}
	if autonomyLevel != "RESEARCH_ONLY" && autonomyLevel != "STRATEGY_AUTONOMOUS" {
		return Mandate{}, ErrInvalid
	}
	old, err := s.store.GetMandate(c, p.UserID, id)
	if err != nil {
		return Mandate{}, ErrNotFound
	}
	if old.AutomationType != "STRATEGY" || old.StrategyIdentifier == nil || (old.ExecutionMode != "PAPER" && old.ExecutionMode != "SHADOW") {
		return Mandate{}, ErrInvalid
	}
	cmd := commandFrom(old)
	cmd.AutonomyLevel = autonomyLevel
	unverified, err := s.validate(c, p, cmd, false)
	if err != nil {
		return Mandate{}, err
	}
	updated, err := s.store.UpdateMandate(c, p.UserID, id, expected, cmd, unverified, "UI")
	if err == nil {
		s.auditEvent(c, p, "automation_mandate.autonomy_changed", map[string]any{
			"mandate_id":     id,
			"version":        updated.CurrentVersion,
			"from_autonomy":  old.AutonomyLevel,
			"to_autonomy":    autonomyLevel,
			"execution_mode": old.ExecutionMode,
			"source":         "UI",
		})
	}
	return updated, err
}

func (s *Service) UpdatePaperOptionsSimulationAttestation(c context.Context, p authorization.Principal, id string, expected int, attested bool) (Mandate, error) {
	if !allowed(p) {
		return Mandate{}, ErrForbidden
	}
	old, err := s.store.GetMandate(c, p.UserID, id)
	if err != nil {
		return Mandate{}, ErrNotFound
	}
	if old.AutomationType != "STRATEGY" || old.StrategyIdentifier == nil || old.ExecutionMode != "PAPER" || !old.OptionsAllowed {
		return Mandate{}, ErrInvalid
	}
	strategy, ok := Strategies[*old.StrategyIdentifier]
	if !ok || !strategy.OptionsRequired {
		return Mandate{}, ErrInvalid
	}
	if attested == old.PaperOptionsSimulationAttested {
		return Mandate{}, ErrInvalid
	}
	if attested {
		facts, factsErr := s.store.AccountFacts(c, p.UserID, old.FinancialAccountID)
		if factsErr != nil || !facts.Owned || facts.Options != "UNKNOWN" {
			return Mandate{}, ErrInvalid
		}
	}
	cmd := commandFrom(old)
	cmd.PaperOptionsSimulationAttested = attested
	unverified, err := s.validate(c, p, cmd, false)
	if err != nil {
		return Mandate{}, err
	}
	updated, err := s.store.UpdateMandate(c, p.UserID, id, expected, cmd, unverified, "UI")
	if err == nil {
		s.auditEvent(c, p, "automation_mandate.paper_options_simulation_attestation_changed", map[string]any{
			"mandate_id":                id,
			"version":                   updated.CurrentVersion,
			"from_attested":             old.PaperOptionsSimulationAttested,
			"to_attested":               attested,
			"execution_mode":            old.ExecutionMode,
			"broker_capability_changed": false,
			"source":                    "UI",
		})
	}
	return updated, err
}

func (s *Service) UpdateStrategyParameters(c context.Context, p authorization.Principal, id string, expected int, parameters StrategyParameters) (Mandate, error) {
	if !allowed(p) {
		return Mandate{}, ErrForbidden
	}
	for index, symbol := range parameters.Symbols {
		parameters.Symbols[index] = strings.ToUpper(strings.TrimSpace(symbol))
	}
	if err := ValidateStrategyParameters(parameters); err != nil {
		return Mandate{}, err
	}
	old, err := s.store.GetMandate(c, p.UserID, id)
	if err != nil {
		return Mandate{}, ErrNotFound
	}
	if (old.AutomationType != "STRATEGY" || old.StrategyIdentifier == nil) && old.AutomationType != "AI_AUTONOMOUS" {
		return Mandate{}, ErrInvalid
	}
	raw, err := json.Marshal(parameters)
	if err != nil {
		return Mandate{}, err
	}
	cmd := commandFrom(old)
	cmd.StrategyParameters = raw
	unverified, err := s.validate(c, p, cmd, false)
	if err != nil {
		return Mandate{}, err
	}
	updated, err := s.store.UpdateMandate(c, p.UserID, id, expected, cmd, unverified, "UI")
	if err == nil {
		s.auditEvent(c, p, "automation_mandate.strategy_parameters_changed", map[string]any{"mandate_id": id, "version": updated.CurrentVersion, "source": "UI"})
	}
	return updated, err
}

func (s *Service) UpdateAIShadowParameters(c context.Context, p authorization.Principal, id string, expected int, parameters AIShadowParameters) (Mandate, error) {
	if !allowed(p) {
		return Mandate{}, ErrForbidden
	}
	old, err := s.store.GetMandate(c, p.UserID, id)
	if err != nil {
		return Mandate{}, ErrNotFound
	}
	if old.AutomationType != "AI_AUTONOMOUS" || (old.ExecutionMode != "PAPER" && old.ExecutionMode != "SHADOW") {
		return Mandate{}, ErrInvalid
	}
	raw, err := json.Marshal(parameters)
	if err != nil {
		return Mandate{}, err
	}
	parsed, err := ParseAIShadowParameters(raw)
	if err != nil {
		return Mandate{}, err
	}
	raw, err = json.Marshal(parsed)
	if err != nil {
		return Mandate{}, err
	}
	cmd := commandFrom(old)
	cmd.StrategyParameters = raw
	unverified, err := s.validate(c, p, cmd, false)
	if err != nil {
		return Mandate{}, err
	}
	updated, err := s.store.UpdateMandate(c, p.UserID, id, expected, cmd, unverified, "UI")
	if err == nil {
		s.auditEvent(c, p, "automation_mandate.ai_shadow_parameters_changed", map[string]any{"mandate_id": id, "version": updated.CurrentVersion, "source": "UI"})
	}
	return updated, err
}

func (s *Service) CreateAIShadowScenarioDraft(c context.Context, p authorization.Principal, id string, command AIShadowScenarioDraftCommand) (Mandate, error) {
	if !allowed(p) {
		return Mandate{}, ErrForbidden
	}
	if !command.Confirm || command.ExpectedVersion < 1 || command.MaxTradesPerDay < 1 || command.MaxTradesPerDay > 48 {
		return Mandate{}, ErrInvalid
	}
	old, err := s.store.GetMandate(c, p.UserID, id)
	if err != nil {
		return Mandate{}, ErrNotFound
	}
	if old.AutomationType != "AI_AUTONOMOUS" || old.ExecutionMode != "SHADOW" || old.Status == "ARCHIVED" {
		return Mandate{}, ErrInvalid
	}
	parameters, err := ParseAIShadowParameters(old.StrategyParameters)
	if err != nil {
		return Mandate{}, err
	}
	previousMaximumText := parameters.MaxProposalNotional
	previousMaximum, _ := decimal(parameters.MaxProposalNotional, true)
	parameters.MaxProposalNotional = strings.TrimSpace(command.MaxProposalNotional)
	candidateRaw, err := json.Marshal(parameters)
	if err != nil {
		return Mandate{}, err
	}
	parameters, err = ParseAIShadowParameters(candidateRaw)
	if err != nil {
		return Mandate{}, err
	}
	nextMaximum, _ := decimal(parameters.MaxProposalNotional, true)
	unchangedDailyLimit := old.Risk.MaxTradesPerDay != nil && *old.Risk.MaxTradesPerDay == command.MaxTradesPerDay
	if previousMaximum.Cmp(nextMaximum) == 0 && unchangedDailyLimit {
		return Mandate{}, ErrInvalid
	}
	raw, err := json.Marshal(parameters)
	if err != nil {
		return Mandate{}, err
	}
	cmd := commandFrom(old)
	cmd.StrategyParameters = raw
	cmd.Risk.MaxTradesPerDay = &command.MaxTradesPerDay
	unverified, err := s.validate(c, p, cmd, false)
	if err != nil {
		return Mandate{}, err
	}
	updated, err := s.store.UpdateMandate(c, p.UserID, id, command.ExpectedVersion, cmd, unverified, "UI")
	if err == nil {
		previousDailyLimit := 0
		if old.Risk.MaxTradesPerDay != nil {
			previousDailyLimit = *old.Risk.MaxTradesPerDay
		}
		s.auditEvent(c, p, "automation_mandate.ai_shadow_scenario_draft_created", map[string]any{
			"mandate_id":                 id,
			"version":                    updated.CurrentVersion,
			"from_max_proposal_notional": previousMaximumText,
			"to_max_proposal_notional":   parameters.MaxProposalNotional,
			"from_max_trades_per_day":    previousDailyLimit,
			"to_max_trades_per_day":      command.MaxTradesPerDay,
			"execution_mode":             old.ExecutionMode,
			"live_execution_available":   false,
			"review_required":            true,
			"source":                     "UI",
		})
	}
	return updated, err
}

func (s *Service) UpdateSchedule(c context.Context, p authorization.Principal, id string, expected int, schedule ScheduleConditions) (Mandate, error) {
	if !allowed(p) {
		return Mandate{}, ErrForbidden
	}
	if err := ValidateScheduleConditions(schedule); err != nil {
		return Mandate{}, err
	}
	old, err := s.store.GetMandate(c, p.UserID, id)
	if err != nil {
		return Mandate{}, ErrNotFound
	}
	validType := (old.AutomationType == "STRATEGY" && old.StrategyIdentifier != nil) ||
		(old.AutomationType == "AI_AUTONOMOUS" && old.StrategyIdentifier == nil)
	if !validType {
		return Mandate{}, ErrInvalid
	}
	raw, err := json.Marshal(schedule)
	if err != nil {
		return Mandate{}, err
	}
	cmd := commandFrom(old)
	legacyDailyActionLimitApplied := false
	if old.AutomationType == "AI_AUTONOMOUS" && cmd.Risk.MaxTradesPerDay == nil {
		conservativeLimit := 1
		cmd.Risk.MaxTradesPerDay = &conservativeLimit
		legacyDailyActionLimitApplied = true
	}
	cmd.ScheduleConditions = raw
	unverified, err := s.validate(c, p, cmd, false)
	if err != nil {
		return Mandate{}, err
	}
	updated, err := s.store.UpdateMandate(c, p.UserID, id, expected, cmd, unverified, "UI")
	if err == nil {
		s.auditEvent(c, p, "automation_mandate.schedule_changed", map[string]any{"mandate_id": id, "version": updated.CurrentVersion, "enabled": schedule.Enabled, "legacy_daily_action_limit_applied": legacyDailyActionLimitApplied, "source": "UI"})
	}
	return updated, err
}
func (s *Service) Transition(c context.Context, p authorization.Principal, id string, expected int, status string) (Mandate, error) {
	if !allowed(p) {
		return Mandate{}, ErrForbidden
	}
	old, e := s.store.GetMandate(c, p.UserID, id)
	if e != nil {
		return Mandate{}, e
	}
	if status == "READY" {
		cmd := commandFrom(old)
		if _, e = s.validate(c, p, cmd, true); e != nil {
			return Mandate{}, e
		}
	}
	m, e := s.store.Transition(c, p.UserID, id, expected, status, "UI")
	if e == nil {
		s.auditEvent(c, p, "automation_mandate."+strings.ToLower(status), map[string]any{"mandate_id": id, "version": m.CurrentVersion, "source": "UI"})
	}
	return m, e
}
func commandFrom(m Mandate) MandateCommand {
	return MandateCommand{
		FinancialAccountID:             m.FinancialAccountID,
		AutomationType:                 m.AutomationType,
		CapitalBucketID:                m.CapitalBucketID,
		AutonomyLevel:                  m.AutonomyLevel,
		ExecutionMode:                  m.ExecutionMode,
		StrategyIdentifier:             m.StrategyIdentifier,
		AIProviderConnectionID:         m.AIProviderConnectionID,
		AIModelID:                      m.AIModelID,
		StrategyParameters:             m.StrategyParameters,
		Risk:                           m.Risk,
		AllowedUniverse:                m.AllowedUniverse,
		ProhibitedUniverse:             m.ProhibitedUniverse,
		MarginAllowed:                  m.MarginAllowed,
		OptionsAllowed:                 m.OptionsAllowed,
		PaperOptionsSimulationAttested: m.PaperOptionsSimulationAttested,
		ScheduleConditions:             m.ScheduleConditions,
		EffectiveFrom:                  &m.EffectiveFrom,
		EffectiveUntil:                 m.EffectiveUntil,
	}
}

// MandateFromVersion reconstructs the immutable reviewed mandate used by a
// pinned strategy instance. Mutable identity and ownership come only from the
// owner-scoped current record; every configurable field comes from the version
// snapshot.
func MandateFromVersion(current Mandate, version Version) (Mandate, error) {
	if current.ID == "" || current.UserID == "" || version.MandateID != current.ID || version.VersionNumber < 1 {
		return Mandate{}, ErrInvalid
	}
	var stored struct {
		MandateCommand
		Status               string `json:"status"`
		CapabilityUnverified bool   `json:"capability_unverified"`
		ExecutionCapable     bool   `json:"execution_capable"`
	}
	if err := json.Unmarshal(version.Snapshot, &stored); err != nil || stored.Status == "" || stored.ExecutionCapable {
		return Mandate{}, ErrInvalid
	}
	return Mandate{
		ID:                             current.ID,
		UserID:                         current.UserID,
		FinancialAccountID:             stored.FinancialAccountID,
		AutomationType:                 stored.AutomationType,
		StrategyIdentifier:             stored.StrategyIdentifier,
		AIProviderConnectionID:         stored.AIProviderConnectionID,
		AIModelID:                      stored.AIModelID,
		CapitalBucketID:                stored.CapitalBucketID,
		AutonomyLevel:                  stored.AutonomyLevel,
		ExecutionMode:                  stored.ExecutionMode,
		Status:                         stored.Status,
		CurrentVersion:                 version.VersionNumber,
		StrategyParameters:             stored.StrategyParameters,
		Risk:                           stored.Risk,
		AllowedUniverse:                stored.AllowedUniverse,
		ProhibitedUniverse:             stored.ProhibitedUniverse,
		MarginAllowed:                  stored.MarginAllowed,
		OptionsAllowed:                 stored.OptionsAllowed,
		CapabilityUnverified:           stored.CapabilityUnverified,
		PaperOptionsSimulationAttested: stored.PaperOptionsSimulationAttested,
		ScheduleConditions:             stored.ScheduleConditions,
		EffectiveFrom:                  timeOrZero(stored.EffectiveFrom),
		EffectiveUntil:                 stored.EffectiveUntil,
		CreatedAt:                      current.CreatedAt,
		UpdatedAt:                      version.CreatedAt,
		ExecutionCapable:               false,
	}, nil
}

func timeOrZero(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func (s *Service) AtVersion(c context.Context, p authorization.Principal, id string, versionNumber int) (Mandate, error) {
	if !allowed(p) {
		return Mandate{}, ErrForbidden
	}
	current, err := s.store.GetMandate(c, p.UserID, id)
	if err != nil {
		return Mandate{}, ErrNotFound
	}
	version, err := s.store.Version(c, p.UserID, id, versionNumber)
	if err != nil {
		return Mandate{}, ErrNotFound
	}
	return MandateFromVersion(current, version)
}

func (s *Service) Versions(c context.Context, p authorization.Principal, id string) ([]Version, error) {
	if !allowed(p) {
		return nil, ErrForbidden
	}
	return s.store.Versions(c, p.UserID, id)
}
func (s *Service) Version(c context.Context, p authorization.Principal, id string, v int) (Version, error) {
	if !allowed(p) {
		return Version{}, ErrForbidden
	}
	return s.store.Version(c, p.UserID, id, v)
}
