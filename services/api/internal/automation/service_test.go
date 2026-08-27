package automation

import (
	"context"
	"math/big"
	"testing"

	"github.com/arbion/platform/services/api/internal/authorization"
)

type fakeStore struct {
	account          AccountFacts
	ai               AIFacts
	bucket           CapitalBucket
	fixed            *big.Rat
	created          Mandate
	transitionStatus string
	transitionSource string
	updatedCommand   MandateCommand
}

type fakeAuditor struct {
	action string
	data   map[string]any
}

func (f *fakeAuditor) Record(_ context.Context, _ *string, action string, data map[string]any) error {
	f.action = action
	f.data = data
	return nil
}

func (f *fakeStore) AccountFacts(context.Context, string, string) (AccountFacts, error) {
	return f.account, nil
}
func (f *fakeStore) AIFacts(context.Context, string, string, string) (AIFacts, error) {
	return f.ai, nil
}
func (f *fakeStore) CreateBucket(_ context.Context, u string, c CreateBucketCommand) (CapitalBucket, error) {
	return CapitalBucket{ID: "b", UserID: u, FinancialAccountID: c.FinancialAccountID, AllocationValue: c.AllocationValue, AllocationType: c.AllocationType, Currency: c.Currency, Status: "ACTIVE"}, nil
}
func (f *fakeStore) FixedAllocated(context.Context, string, string) (*big.Rat, error) {
	if f.fixed == nil {
		return new(big.Rat), nil
	}
	return f.fixed, nil
}
func (f *fakeStore) ListBuckets(context.Context, string) ([]CapitalBucket, error) {
	return []CapitalBucket{f.bucket}, nil
}
func (f *fakeStore) GetBucket(context.Context, string, string) (CapitalBucket, error) {
	return f.bucket, nil
}
func (f *fakeStore) UpdateBucket(context.Context, string, string, CreateBucketCommand) (CapitalBucket, error) {
	return f.bucket, nil
}
func (f *fakeStore) DeleteBucket(context.Context, string, string) error { return nil }
func (f *fakeStore) CreateMandate(_ context.Context, u string, c MandateCommand, w bool) (Mandate, error) {
	f.created = Mandate{ID: "m", UserID: u, FinancialAccountID: c.FinancialAccountID, AutomationType: c.AutomationType, CapitalBucketID: c.CapitalBucketID, AutonomyLevel: c.AutonomyLevel, ExecutionMode: c.ExecutionMode, Status: "DRAFT", CurrentVersion: 1, CapabilityUnverified: w, ExecutionCapable: false}
	return f.created, nil
}
func (f *fakeStore) ListMandates(context.Context, string) ([]Mandate, error) {
	return []Mandate{f.created}, nil
}
func (f *fakeStore) GetMandate(context.Context, string, string) (Mandate, error) {
	return f.created, nil
}
func (f *fakeStore) UpdateMandate(_ context.Context, _, _ string, _ int, command MandateCommand, unverified bool, _ string) (Mandate, error) {
	f.updatedCommand = command
	f.created.CurrentVersion++
	f.created.Status = "DRAFT"
	f.created.CapabilityUnverified = unverified
	f.created.PaperOptionsSimulationAttested = command.PaperOptionsSimulationAttested
	return f.created, nil
}
func (f *fakeStore) Transition(_ context.Context, _, _ string, _ int, status, source string) (Mandate, error) {
	f.transitionStatus = status
	f.transitionSource = source
	f.created.Status = status
	f.created.CurrentVersion++
	return f.created, nil
}
func (f *fakeStore) Versions(context.Context, string, string) ([]Version, error) {
	return []Version{{VersionNumber: 1}, {VersionNumber: 2}}, nil
}
func (f *fakeStore) Version(context.Context, string, string, int) (Version, error) {
	return Version{VersionNumber: 1}, nil
}

var founder = authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}

func intPointer(value int) *int { return &value }

func baseStore() *fakeStore {
	return &fakeStore{account: AccountFacts{Owned: true, Provider: "schwab", Options: "SUPPORTED"}, bucket: CapitalBucket{ID: "b", FinancialAccountID: "a", Status: "ACTIVE"}, ai: AIFacts{Owned: true, Active: true, ModelValid: true, Provider: "openai"}}
}
func baseCommand() MandateCommand {
	s := "wheel"
	return MandateCommand{FinancialAccountID: "a", AutomationType: "STRATEGY", StrategyIdentifier: &s, CapitalBucketID: "b", AutonomyLevel: "CONFIRM_EACH", ExecutionMode: "LIVE"}
}

func TestBucketExactDecimalPercentageReserveAndOverlap(t *testing.T) {
	f := baseStore()
	s := NewService(f, nil)
	limit := "20.00"
	f.fixed = big.NewRat(15, 1)
	_, e := s.CreateBucket(context.Background(), founder, CreateBucketCommand{FinancialAccountID: "a", Name: "Exact", AllocationType: "FIXED_AMOUNT", AllocationValue: "10.0000000001", Currency: "USD", ProtectedAmount: "0", AllocationLimit: &limit})
	if e != ErrConflict {
		t.Fatalf("expected overlap conflict, got %v", e)
	}
	_, e = s.CreateBucket(context.Background(), founder, CreateBucketCommand{FinancialAccountID: "a", Name: "Percent", AllocationType: "PERCENT_OF_BUYING_POWER", AllocationValue: "100.01", Currency: "USD", ProtectedAmount: "0"})
	if e != ErrInvalid {
		t.Fatal("percentage above 100 accepted")
	}
	f.fixed = new(big.Rat)
	b, e := s.CreateBucket(context.Background(), founder, CreateBucketCommand{FinancialAccountID: "a", Name: "Reserve", AllocationType: "FIXED_AMOUNT", AllocationValue: "0.0000000001", Currency: "USD", ProtectedAmount: "0", IsReserve: true})
	if e != nil || b.AllocationValue != "0.0000000001" {
		t.Fatal("exact decimal was not preserved")
	}
}
func TestMandateTypesCapabilitiesAutonomyAndNoExecution(t *testing.T) {
	ctx := context.Background()
	f := baseStore()
	s := NewService(f, nil)
	c := baseCommand()
	m, e := s.Create(ctx, founder, c)
	if e != nil || m.CurrentVersion != 1 || m.ExecutionMode != "LIVE" || m.ExecutionCapable {
		t.Fatalf("LIVE configuration must remain inert: %#v %v", m, e)
	}
	c.AutomationType = "AI_AUTONOMOUS"
	c.StrategyIdentifier = nil
	ai := "ai"
	model := "gpt-5.6-sol"
	c.AIProviderConnectionID = &ai
	c.AIModelID = &model
	c.ExecutionMode = "SHADOW"
	c.AutonomyLevel = "FULL_AUTONOMOUS"
	c.OptionsAllowed = false
	c.Risk.MaxTradesPerDay = intPointer(6)
	if _, e = s.Create(ctx, founder, c); e != nil {
		t.Fatalf("valid AI mandate rejected: %v", e)
	}
	c.AutomationType = "HYBRID"
	wheel := "wheel"
	c.StrategyIdentifier = &wheel
	if _, e = s.Create(ctx, founder, c); e != nil {
		t.Fatalf("valid hybrid rejected: %v", e)
	}
	f.ai.Active = false
	if _, e = s.Create(ctx, founder, c); e != ErrInvalid {
		t.Fatal("inactive AI connection accepted")
	}
	f.ai.Active = true
	c.AutomationType = "STRATEGY"
	c.AIProviderConnectionID = nil
	c.AIModelID = nil
	c.AutonomyLevel = "FULL_AUTONOMOUS"
	if _, e = s.validate(ctx, founder, c, true); e != ErrInvalid {
		t.Fatal("invalid autonomy/type combination accepted as ready")
	}
	f.account.Options = "UNSUPPORTED"
	if _, e = s.Create(ctx, founder, c); e != ErrInvalid {
		t.Fatal("unsupported options capability accepted")
	}
	f.account.Options = "UNKNOWN"
	c.AutonomyLevel = "CONFIRM_EACH"
	m, e = s.Create(ctx, founder, c)
	if e != nil || !m.CapabilityUnverified {
		t.Fatal("unknown capability did not produce warning")
	}
}

func TestAIShadowReadyMandateBindsProviderSessionAndBoundedParameters(t *testing.T) {
	connection, model := "ai", "gpt-5.6-sol"
	command := MandateCommand{FinancialAccountID: "a", AutomationType: "AI_AUTONOMOUS", CapitalBucketID: "b", AutonomyLevel: "FULL_AUTONOMOUS", ExecutionMode: "SHADOW", AIProviderConnectionID: &connection, AIModelID: &model, StrategyParameters: []byte(`{"objective":"Preserve capital.","max_proposal_notional":"1"}`), Risk: RiskPolicy{MaxTradesPerDay: intPointer(6)}, AllowedUniverse: Universe{Symbols: []string{"BTC"}}, ScheduleConditions: []byte(`{"enabled":true,"interval_minutes":60,"session":"US_EQUITIES_REGULAR"}`)}
	store := baseStore()
	service := NewService(store, nil)
	if _, err := service.validate(context.Background(), founder, command, true); err != nil {
		t.Fatalf("valid Schwab AI shadow mandate rejected: %v", err)
	}
	store.account.Provider = "coinbase"
	command.ScheduleConditions = []byte(`{"enabled":true,"interval_minutes":60,"session":"CONTINUOUS"}`)
	if _, err := service.validate(context.Background(), founder, command, true); err != nil {
		t.Fatalf("valid Coinbase AI shadow mandate rejected: %v", err)
	}
	command.ScheduleConditions = []byte(`{"enabled":true,"interval_minutes":60,"session":"US_EQUITIES_REGULAR"}`)
	if _, err := service.validate(context.Background(), founder, command, true); err != ErrInvalid {
		t.Fatalf("Coinbase equities session mismatch accepted: %v", err)
	}
	command.ScheduleConditions = []byte(`{"enabled":false}`)
	command.StrategyParameters = []byte(`{"objective":"Preserve capital.","max_proposal_notional":"0"}`)
	if _, err := service.validate(context.Background(), founder, command, true); err != ErrInvalid {
		t.Fatalf("zero AI proposal ceiling accepted: %v", err)
	}
}

func TestAIShadowReadyMandateAcceptsOnlyMatchingClaudeProviderAndModel(t *testing.T) {
	connection, model := "ai", "claude-sonnet-5"
	command := MandateCommand{FinancialAccountID: "a", AutomationType: "AI_AUTONOMOUS", CapitalBucketID: "b", AutonomyLevel: "FULL_AUTONOMOUS", ExecutionMode: "SHADOW", AIProviderConnectionID: &connection, AIModelID: &model, StrategyParameters: []byte(`{"objective":"Preserve capital.","max_proposal_notional":"1"}`), Risk: RiskPolicy{MaxTradesPerDay: intPointer(6)}, AllowedUniverse: Universe{Symbols: []string{"BTC"}}, ScheduleConditions: []byte(`{"enabled":false}`)}
	store := baseStore()
	store.ai.Provider = "anthropic"
	service := NewService(store, nil)
	if _, err := service.validate(context.Background(), founder, command, true); err != nil {
		t.Fatalf("valid Claude AI shadow mandate rejected: %v", err)
	}
	store.ai.Provider = "openai"
	if _, err := service.validate(context.Background(), founder, command, true); err != ErrInvalid {
		t.Fatalf("mismatched Claude model/provider accepted: %v", err)
	}
	unknown := "claude-unapproved"
	command.AIModelID = &unknown
	store.ai.Provider = "anthropic"
	if _, err := service.validate(context.Background(), founder, command, true); err != ErrInvalid {
		t.Fatalf("unapproved Claude model accepted: %v", err)
	}
}

func TestAIShadowReadyMandateAcceptsOnlyMatchingGeminiProviderAndModel(t *testing.T) {
	connection, model := "ai", "gemini-3.6-flash"
	command := MandateCommand{FinancialAccountID: "a", AutomationType: "AI_AUTONOMOUS", CapitalBucketID: "b", AutonomyLevel: "FULL_AUTONOMOUS", ExecutionMode: "SHADOW", AIProviderConnectionID: &connection, AIModelID: &model, StrategyParameters: []byte(`{"objective":"Preserve capital.","max_proposal_notional":"1"}`), Risk: RiskPolicy{MaxTradesPerDay: intPointer(6)}, AllowedUniverse: Universe{Symbols: []string{"BTC"}}, ScheduleConditions: []byte(`{"enabled":false}`)}
	store := baseStore()
	store.ai.Provider = "gemini"
	service := NewService(store, nil)
	if _, err := service.validate(context.Background(), founder, command, true); err != nil {
		t.Fatalf("valid Gemini AI shadow mandate rejected: %v", err)
	}
	store.ai.Provider = "openai"
	if _, err := service.validate(context.Background(), founder, command, true); err != ErrInvalid {
		t.Fatalf("mismatched Gemini model/provider accepted: %v", err)
	}
}

func TestAIShadowMandateRequiresBoundedAuditableGuardrails(t *testing.T) {
	connection, model := "ai", "gpt-5.6-sol"
	command := MandateCommand{FinancialAccountID: "a", AutomationType: "AI_AUTONOMOUS", CapitalBucketID: "b", AutonomyLevel: "FULL_AUTONOMOUS", ExecutionMode: "SHADOW", AIProviderConnectionID: &connection, AIModelID: &model, StrategyParameters: []byte(`{"objective":"Preserve capital.","max_proposal_notional":"1"}`), AllowedUniverse: Universe{Symbols: []string{"BTC"}}, ScheduleConditions: []byte(`{"enabled":false}`)}
	service := NewService(baseStore(), nil)
	if _, err := service.validate(context.Background(), founder, command, false); err != ErrInvalid {
		t.Fatalf("AI mandate without a daily action limit was accepted: %v", err)
	}
	command.Risk.MaxTradesPerDay = intPointer(49)
	if _, err := service.validate(context.Background(), founder, command, false); err != ErrInvalid {
		t.Fatalf("AI mandate with an unbounded daily action limit was accepted: %v", err)
	}
	command.Risk.MaxTradesPerDay = intPointer(6)
	dailyLoss := "10"
	command.Risk.MaxDailyLoss = &dailyLoss
	if _, err := service.validate(context.Background(), founder, command, false); err != ErrInvalid {
		t.Fatalf("AI mandate accepted a daily loss limit without authoritative realized P/L: %v", err)
	}
	command.Risk.MaxDailyLoss = nil
	capital, position := "100", "101"
	command.Risk.MaxCapitalDeployed = &capital
	command.Risk.MaxSinglePositionAmount = &position
	if _, err := service.validate(context.Background(), founder, command, false); err != ErrInvalid {
		t.Fatalf("single-position limit above total deployment limit was accepted: %v", err)
	}
	position = "25"
	if _, err := service.validate(context.Background(), founder, command, false); err != nil {
		t.Fatalf("bounded AI guardrails were rejected: %v", err)
	}
}
func TestOwnershipEntitlementReserveAndDurability(t *testing.T) {
	f := baseStore()
	s := NewService(f, nil)
	c := baseCommand()
	if _, e := s.Create(context.Background(), authorization.Principal{UserID: "u", Role: authorization.RoleSuperadmin, Entitlement: authorization.EntitlementFree}, c); e != ErrForbidden {
		t.Fatal("admin role granted product entitlement")
	}
	f.account.Owned = false
	if _, e := s.Create(context.Background(), founder, c); e != ErrNotFound {
		t.Fatal("cross-user account accepted")
	}
	f.account.Owned = true
	f.bucket.IsReserve = true
	if _, e := s.Create(context.Background(), founder, c); e != ErrInvalid {
		t.Fatal("reserve attached to mandate")
	}
	f.bucket.IsReserve = false
	m, _ := s.Create(context.Background(), founder, c)
	before := m.CurrentVersion
	f.created = m
	updated, _ := s.Update(context.Background(), founder, m.ID, before, c)
	if updated.CurrentVersion != 2 {
		t.Fatal("update did not create version 2")
	}
	versions, _ := s.Versions(context.Background(), founder, m.ID)
	if len(versions) != 2 {
		t.Fatal("durable history missing")
	}
}

func TestReadyPaperWheelAllowsUnknownOptionsCapability(t *testing.T) {
	f := baseStore()
	f.account.Options = "UNKNOWN"
	wheel := "wheel"
	f.created = Mandate{
		ID:                   "m",
		UserID:               founder.UserID,
		FinancialAccountID:   "a",
		AutomationType:       "STRATEGY",
		StrategyIdentifier:   &wheel,
		CapitalBucketID:      "b",
		AutonomyLevel:        "RESEARCH_ONLY",
		ExecutionMode:        "PAPER",
		Status:               "DRAFT",
		CurrentVersion:       1,
		StrategyParameters:   []byte(`{"symbols":["AAPL"],"minimum_dte":20,"maximum_dte":60,"target_delta":"0.30","target_delta_min":"0.20","target_delta_max":"0.40","maximum_contracts":1,"assignment_handling_policy":"continue_wheel"}`),
		ScheduleConditions:   []byte(`{}`),
		OptionsAllowed:       true,
		CapabilityUnverified: true,
	}

	ready, err := NewService(f, nil).Transition(context.Background(), founder, f.created.ID, 1, "READY")
	if err != nil {
		t.Fatalf("safe PAPER wheel readiness was rejected: %v", err)
	}
	if ready.Status != "READY" || ready.CurrentVersion != 2 {
		t.Fatalf("unexpected READY transition: %#v", ready)
	}
	if f.transitionStatus != "READY" || f.transitionSource != "UI" {
		t.Fatalf("unexpected transition metadata: status=%q source=%q", f.transitionStatus, f.transitionSource)
	}
}

func TestAutonomyUpdateCreatesNonLiveDraftAndPreservesConfiguration(t *testing.T) {
	f := baseStore()
	wheel := "wheel"
	f.created = Mandate{
		ID:                 "m",
		UserID:             founder.UserID,
		FinancialAccountID: "a",
		AutomationType:     "STRATEGY",
		StrategyIdentifier: &wheel,
		CapitalBucketID:    "b",
		AutonomyLevel:      "RESEARCH_ONLY",
		ExecutionMode:      "PAPER",
		Status:             "READY",
		CurrentVersion:     4,
		StrategyParameters: []byte(`{"symbols":["AAPL"]}`),
		ScheduleConditions: []byte(`{}`),
		OptionsAllowed:     true,
	}

	updated, err := NewService(f, nil).UpdateAutonomy(context.Background(), founder, "m", 4, "STRATEGY_AUTONOMOUS")
	if err != nil || updated.Status != "DRAFT" || updated.CurrentVersion != 5 {
		t.Fatalf("autonomy update did not create a draft version: %#v %v", updated, err)
	}
	if f.updatedCommand.AutonomyLevel != "STRATEGY_AUTONOMOUS" || f.updatedCommand.ExecutionMode != "PAPER" || string(f.updatedCommand.ScheduleConditions) != `{}` {
		t.Fatalf("autonomy update changed unrelated safety settings: %#v", f.updatedCommand)
	}
}

func TestAutonomyUpdateRejectsLiveAndNonStrategyMandates(t *testing.T) {
	f := baseStore()
	wheel := "wheel"
	f.created = Mandate{ID: "m", UserID: founder.UserID, FinancialAccountID: "a", AutomationType: "STRATEGY", StrategyIdentifier: &wheel, CapitalBucketID: "b", AutonomyLevel: "RESEARCH_ONLY", ExecutionMode: "LIVE", CurrentVersion: 1, ScheduleConditions: []byte(`{}`)}
	service := NewService(f, nil)
	if _, err := service.UpdateAutonomy(context.Background(), founder, "m", 1, "STRATEGY_AUTONOMOUS"); err != ErrInvalid {
		t.Fatalf("live autonomy update was accepted: %v", err)
	}
	f.created.AutomationType = "AI_AUTONOMOUS"
	f.created.StrategyIdentifier = nil
	f.created.ExecutionMode = "PAPER"
	if _, err := service.UpdateAutonomy(context.Background(), founder, "m", 1, "STRATEGY_AUTONOMOUS"); err != ErrInvalid {
		t.Fatalf("non-strategy autonomy update was accepted: %v", err)
	}
	if _, err := service.UpdateAutonomy(context.Background(), founder, "m", 1, "FULL_AUTONOMOUS"); err != ErrInvalid {
		t.Fatalf("unsupported autonomy update was accepted: %v", err)
	}
}

func TestPaperOptionsSimulationAttestationCreatesAuditedDraft(t *testing.T) {
	f := baseStore()
	f.account.Options = "UNKNOWN"
	wheel := "wheel"
	f.created = Mandate{
		ID:                   "m",
		UserID:               founder.UserID,
		FinancialAccountID:   "a",
		AutomationType:       "STRATEGY",
		StrategyIdentifier:   &wheel,
		CapitalBucketID:      "b",
		AutonomyLevel:        "STRATEGY_AUTONOMOUS",
		ExecutionMode:        "PAPER",
		Status:               "READY",
		CurrentVersion:       6,
		StrategyParameters:   []byte(`{"symbols":["AAPL"]}`),
		ScheduleConditions:   []byte(`{}`),
		OptionsAllowed:       true,
		CapabilityUnverified: true,
	}
	audit := &fakeAuditor{}

	updated, err := NewService(f, audit).UpdatePaperOptionsSimulationAttestation(context.Background(), founder, "m", 6, true)
	if err != nil || updated.Status != "DRAFT" || updated.CurrentVersion != 7 || !updated.PaperOptionsSimulationAttested {
		t.Fatalf("attestation did not create a new draft version: %#v %v", updated, err)
	}
	if !f.updatedCommand.PaperOptionsSimulationAttested || f.updatedCommand.ExecutionMode != "PAPER" || string(f.updatedCommand.ScheduleConditions) != `{}` {
		t.Fatalf("attestation changed unrelated safety settings: %#v", f.updatedCommand)
	}
	if audit.action != "automation_mandate.paper_options_simulation_attestation_changed" || audit.data["broker_capability_changed"] != false || audit.data["to_attested"] != true {
		t.Fatalf("attestation audit evidence is incomplete: %q %#v", audit.action, audit.data)
	}
}

func TestPaperOptionsSimulationAttestationRejectsUnsafeScope(t *testing.T) {
	for _, test := range []struct {
		name       string
		capability string
		mode       string
		automation string
		strategy   string
		options    bool
	}{
		{"supported capability", "SUPPORTED", "PAPER", "STRATEGY", "wheel", true},
		{"unsupported capability", "UNSUPPORTED", "PAPER", "STRATEGY", "wheel", true},
		{"shadow mode", "UNKNOWN", "SHADOW", "STRATEGY", "wheel", true},
		{"live mode", "UNKNOWN", "LIVE", "STRATEGY", "wheel", true},
		{"backtest mode", "UNKNOWN", "BACKTEST", "STRATEGY", "wheel", true},
		{"hybrid automation", "UNKNOWN", "PAPER", "HYBRID", "wheel", true},
		{"non-options strategy policy", "UNKNOWN", "PAPER", "STRATEGY", "wheel", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := baseStore()
			f.account.Options = test.capability
			strategy := test.strategy
			f.created = Mandate{ID: "m", UserID: founder.UserID, FinancialAccountID: "a", AutomationType: test.automation, StrategyIdentifier: &strategy, CapitalBucketID: "b", AutonomyLevel: "STRATEGY_AUTONOMOUS", ExecutionMode: test.mode, CurrentVersion: 1, OptionsAllowed: test.options, ScheduleConditions: []byte(`{}`)}
			if _, err := NewService(f, nil).UpdatePaperOptionsSimulationAttestation(context.Background(), founder, "m", 1, true); err != ErrInvalid {
				t.Fatalf("unsafe attestation was accepted: %v", err)
			}
		})
	}
}

func TestGenericMandateUpdateCannotBypassPaperAttestationBoundary(t *testing.T) {
	f := baseStore()
	f.account.Options = "UNKNOWN"
	command := baseCommand()
	command.ExecutionMode = "PAPER"
	command.PaperOptionsSimulationAttested = true
	if _, err := NewService(f, nil).Create(context.Background(), founder, command); err != ErrInvalid {
		t.Fatalf("generic create bypassed the dedicated attestation command: %v", err)
	}
	wheel := "wheel"
	f.created = Mandate{ID: "m", UserID: founder.UserID, FinancialAccountID: "a", AutomationType: "STRATEGY", StrategyIdentifier: &wheel, CapitalBucketID: "b", AutonomyLevel: "STRATEGY_AUTONOMOUS", ExecutionMode: "PAPER", CurrentVersion: 1, OptionsAllowed: true, ScheduleConditions: []byte(`{}`)}
	if _, err := NewService(f, nil).Update(context.Background(), founder, "m", 1, command); err != ErrInvalid {
		t.Fatalf("generic update bypassed the dedicated attestation command: %v", err)
	}
}

func TestExistingPaperAttestationCanBePreservedAfterCapabilityBecomesSupported(t *testing.T) {
	f := baseStore()
	wheel := "wheel"
	f.created = Mandate{ID: "m", UserID: founder.UserID, FinancialAccountID: "a", AutomationType: "STRATEGY", StrategyIdentifier: &wheel, CapitalBucketID: "b", AutonomyLevel: "STRATEGY_AUTONOMOUS", ExecutionMode: "PAPER", Status: "READY", CurrentVersion: 7, OptionsAllowed: true, PaperOptionsSimulationAttested: true, ScheduleConditions: []byte(`{}`)}
	command := commandFrom(f.created)
	updated, err := NewService(f, nil).Update(context.Background(), founder, "m", 7, command)
	if err != nil || !updated.PaperOptionsSimulationAttested {
		t.Fatalf("existing attestation could not be preserved after broker support became known: %#v %v", updated, err)
	}
}

func TestReadyStrategyRequiresCompleteParameters(t *testing.T) {
	f := baseStore()
	wheel := "wheel"
	f.created = Mandate{ID: "m", UserID: founder.UserID, FinancialAccountID: "a", AutomationType: "STRATEGY", StrategyIdentifier: &wheel, CapitalBucketID: "b", AutonomyLevel: "RESEARCH_ONLY", ExecutionMode: "PAPER", Status: "DRAFT", CurrentVersion: 1, StrategyParameters: []byte(`{}`), OptionsAllowed: true}
	if _, err := NewService(f, nil).Transition(context.Background(), founder, f.created.ID, 1, "READY"); err != ErrInvalid {
		t.Fatalf("incomplete strategy parameters reached READY: %v", err)
	}
}

func TestScheduleConditionsAreStrictAndNonLiveOnly(t *testing.T) {
	for name, raw := range map[string][]byte{
		"too frequent":          []byte(`{"enabled":true,"interval_minutes":15,"session":"US_EQUITIES_REGULAR"}`),
		"wrong session":         []byte(`{"enabled":true,"interval_minutes":60,"session":"ALWAYS"}`),
		"unknown field":         []byte(`{"enabled":false,"live":true}`),
		"disabled data":         []byte(`{"enabled":false,"interval_minutes":60}`),
		"disabled notification": []byte(`{"enabled":false,"notifications":{"first_failure":true}}`),
		"unknown notification":  []byte(`{"enabled":true,"interval_minutes":60,"session":"US_EQUITIES_REGULAR","notifications":{"execute":true}}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseScheduleConditions(raw); err != ErrInvalid {
				t.Fatalf("unsafe schedule accepted: %v", err)
			}
		})
	}
	valid, err := ParseScheduleConditions([]byte(`{"enabled":true,"interval_minutes":60,"session":"US_EQUITIES_REGULAR","notifications":{"evaluation_completed":true,"lifecycle_required":true,"first_failure":true}}`))
	if err != nil || !valid.Enabled || valid.IntervalMinutes != 60 || !valid.Notifications.EvaluationCompleted || !valid.Notifications.LifecycleRequired || !valid.Notifications.FirstFailure {
		t.Fatalf("valid schedule rejected: %#v %v", valid, err)
	}
}

func TestScheduleUpdateCreatesDraftAndPreservesNonLiveBoundary(t *testing.T) {
	f := baseStore()
	wheel := "wheel"
	f.created = Mandate{ID: "m", UserID: founder.UserID, FinancialAccountID: "a", AutomationType: "STRATEGY", StrategyIdentifier: &wheel, CapitalBucketID: "b", AutonomyLevel: "STRATEGY_AUTONOMOUS", ExecutionMode: "PAPER", Status: "READY", CurrentVersion: 4, StrategyParameters: []byte(`{}`), ScheduleConditions: []byte(`{}`)}
	service := NewService(f, nil)
	updated, err := service.UpdateSchedule(context.Background(), founder, "m", 4, ScheduleConditions{Enabled: true, IntervalMinutes: 60, Session: "US_EQUITIES_REGULAR"})
	if err != nil || updated.Status != "DRAFT" || updated.CurrentVersion != 5 {
		t.Fatalf("schedule did not create a draft version: %#v %v", updated, err)
	}
	parsed, err := ParseScheduleConditions(f.updatedCommand.ScheduleConditions)
	if err != nil || !parsed.Enabled {
		t.Fatalf("schedule was not stored in the immutable command: %#v %v", parsed, err)
	}
	f.created.ExecutionMode = "LIVE"
	if _, err = service.UpdateSchedule(context.Background(), founder, "m", 5, parsed); err != ErrInvalid {
		t.Fatalf("live schedule was accepted: %v", err)
	}
}

func TestAIShadowScheduleUpdateCreatesDraftAndEnforcesProviderSession(t *testing.T) {
	connection, model := "ai", "gpt-5.6-sol"
	f := baseStore()
	f.created = Mandate{
		ID:                             "m",
		UserID:                         founder.UserID,
		FinancialAccountID:             "a",
		AutomationType:                 "AI_AUTONOMOUS",
		AIProviderConnectionID:         &connection,
		AIModelID:                      &model,
		CapitalBucketID:                "b",
		AutonomyLevel:                  "FULL_AUTONOMOUS",
		ExecutionMode:                  "SHADOW",
		Status:                         "READY",
		CurrentVersion:                 2,
		StrategyParameters:             []byte(`{"objective":"Preserve capital.","max_proposal_notional":"1"}`),
		Risk:                           RiskPolicy{MaxTradesPerDay: intPointer(6)},
		AllowedUniverse:                Universe{Symbols: []string{"SPY"}},
		ScheduleConditions:             []byte(`{"enabled":false}`),
		PaperOptionsSimulationAttested: false,
	}
	service := NewService(f, nil)

	updated, err := service.UpdateSchedule(context.Background(), founder, "m", 2, ScheduleConditions{Enabled: true, IntervalMinutes: 60, Session: "US_EQUITIES_REGULAR"})
	if err != nil || updated.Status != "DRAFT" || updated.CurrentVersion != 3 {
		t.Fatalf("Schwab AI shadow schedule did not create a draft version: %#v %v", updated, err)
	}
	parsed, err := ParseScheduleConditions(f.updatedCommand.ScheduleConditions)
	if err != nil || !parsed.Enabled || parsed.IntervalMinutes != 60 || parsed.Session != "US_EQUITIES_REGULAR" {
		t.Fatalf("Schwab AI shadow schedule was not preserved: %#v %v", parsed, err)
	}

	if _, err = service.UpdateSchedule(context.Background(), founder, "m", 3, ScheduleConditions{Enabled: true, IntervalMinutes: 60, Session: "CONTINUOUS"}); err != ErrInvalid {
		t.Fatalf("Schwab continuous session mismatch accepted: %v", err)
	}
}
