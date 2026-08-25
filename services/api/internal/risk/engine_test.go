package risk

import (
	"reflect"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }
func fixture() (EvaluationContext, ProposedAction) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	m := &Mandate{ID: "m", UserID: "u", AccountID: "a", BucketID: "b", Status: "READY", AutomationType: "STRATEGY", AutonomyLevel: "CONFIRM_EACH", ExecutionMode: "LIVE", Version: 3, EffectiveFrom: now.Add(-time.Hour), AllowedSymbols: []string{"AAPL", "NVDA"}, MaxCapitalDeployed: ptr("9000"), MaxSinglePositionAmount: ptr("5000"), MaxSinglePositionPercentage: ptr("80"), MaxDailyLoss: ptr("1000"), MaxTradesPerDay: ptr(10), MinimumCashReserve: ptr("1000"), OptionsAllowed: true, MarginAllowed: true}
	c := EvaluationContext{UserID: "u", AccountOwned: true, FinancialEntitled: true, AutomationEntitled: true, ConnectionUsable: true, Mandate: m, Bucket: &CapitalBucket{ID: "b", UserID: "u", AccountID: "a", AllocationType: "FIXED_AMOUNT", AllocationValue: "10000.0000000001", ProtectedAmount: "0"}, Account: &AccountRiskSnapshot{AccountID: "a", Timestamp: now.Add(-time.Second), Currency: "USD", Cash: "15000", AvailableCash: "15000", BuyingPower: "20000", CurrentExposure: "4000", Positions: []Position{{Instrument: "AAPL", Exposure: "2000"}}, Options: CapabilitySupported, Margin: CapabilitySupported}, Activity: &RiskActivitySnapshot{Timestamp: now, RealizedDailyPL: ptr("-20"), ActionsToday: ptr(2)}, Now: now, MaxStaleness: time.Minute}
	a := ProposedAction{ID: "p", FinancialAccountID: "a", MandateID: ptr("m"), MandateVersion: ptr(3), Source: SourceUI, ActionType: ActionBuy, Instrument: "AAPL", Side: "BUY", Quantity: "1", Notional: "1000.0000000001", CreatedAt: now}
	return c, a
}
func reason(e RiskEvaluation, r ReasonCode) bool {
	for _, x := range e.ReasonCodes {
		if x == r {
			return true
		}
	}
	return false
}

func warning(e RiskEvaluation, r ReasonCode) bool {
	for _, x := range e.Warnings {
		if x == r {
			return true
		}
	}
	return false
}
func TestAllowIsDeterministicAndLiveNeverExecutable(t *testing.T) {
	c, a := fixture()
	e := NewEngine()
	x := e.Evaluate(c, a)
	y := e.Evaluate(c, a)
	if x.Decision != Allow || !x.ApprovalRequired || x.PlatformExecutionAvailable || !reason(x, Allowed) {
		t.Fatalf("unexpected evaluation: %#v", x)
	}
	x.ID, y.ID = "", ""
	if !reflect.DeepEqual(x, y) {
		t.Fatal("repeated evaluation changed deterministic policy result")
	}
}
func TestAuthorizationMandateAndBreakerFailures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*EvaluationContext, *ProposedAction)
		want   ReasonCode
	}{
		{"account ownership", func(c *EvaluationContext, a *ProposedAction) { c.AccountOwned = false }, AccountOwnershipMismatch},
		{"mandate ownership", func(c *EvaluationContext, a *ProposedAction) { c.Mandate.UserID = "other" }, MandateOwnershipMismatch},
		{"version", func(c *EvaluationContext, a *ProposedAction) { *a.MandateVersion = 2 }, MandateVersionMismatch},
		{"paused", func(c *EvaluationContext, a *ProposedAction) { c.Mandate.Status = "PAUSED" }, MandatePaused},
		{"disabled", func(c *EvaluationContext, a *ProposedAction) { c.Mandate.Status = "DISABLED" }, MandateNotReady},
		{"expired", func(c *EvaluationContext, a *ProposedAction) { c.Mandate.EffectiveUntil = ptr(c.Now) }, MandateExpired},
		{"global breaker", func(c *EvaluationContext, a *ProposedAction) {
			c.Breakers = []CircuitBreaker{{Scope: ScopeGlobal, State: BreakerOpen}}
		}, CircuitBreakerActive},
		{"user breaker", func(c *EvaluationContext, a *ProposedAction) {
			c.Breakers = []CircuitBreaker{{Scope: ScopeUser, ScopeID: ptr("u"), State: BreakerOpen}}
		}, CircuitBreakerActive},
		{"account breaker", func(c *EvaluationContext, a *ProposedAction) {
			c.Breakers = []CircuitBreaker{{Scope: ScopeAccount, ScopeID: ptr("a"), State: BreakerOpen}}
		}, CircuitBreakerActive},
		{"automation breaker", func(c *EvaluationContext, a *ProposedAction) {
			c.Breakers = []CircuitBreaker{{Scope: ScopeAutomation, ScopeID: ptr("m"), State: BreakerOpen}}
		}, CircuitBreakerActive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, a := fixture()
			tt.mutate(&c, &a)
			got := NewEngine().Evaluate(c, a)
			if got.Decision != Deny || !reason(got, tt.want) {
				t.Fatalf("got %#v", got)
			}
		})
	}
}
func TestAutonomy(t *testing.T) {
	tests := []struct {
		level  string
		source ActionSource
		want   Decision
	}{{"RESEARCH_ONLY", SourceUI, Deny}, {"SUGGEST", SourceAI, Deny}, {"CONFIRM_EACH", SourceAI, Allow}, {"STRATEGY_AUTONOMOUS", SourceAI, Deny}, {"FULL_AUTONOMOUS", SourceAI, Allow}}
	for _, tt := range tests {
		c, a := fixture()
		c.Mandate.AutonomyLevel = tt.level
		a.Source = tt.source
		if tt.level == "STRATEGY_AUTONOMOUS" {
			c.Mandate.StrategyIdentifier = ptr("wheel")
			a.StrategyIdentifier = ptr("wheel")
		}
		got := NewEngine().Evaluate(c, a)
		if got.Decision != tt.want {
			t.Fatalf("%s: %#v", tt.level, got)
		}
	}
	c, a := fixture()
	c.Mandate.AutonomyLevel = "STRATEGY_AUTONOMOUS"
	c.Mandate.StrategyIdentifier = ptr("wheel")
	a.StrategyIdentifier = ptr("wheel")
	a.Source = SourceStrategy
	if NewEngine().Evaluate(c, a).Decision != Allow {
		t.Fatal("valid deterministic strategy denied")
	}
}

func TestAutonomousAISellCannotExceedAvailableHolding(t *testing.T) {
	c, action := fixture()
	c.Mandate.AutomationType = "AI_AUTONOMOUS"
	c.Mandate.AutonomyLevel = "FULL_AUTONOMOUS"
	c.Mandate.ExecutionMode = "SHADOW"
	c.Mandate.MaxSinglePositionPercentage = nil
	c.Mandate.MaxDailyLoss = nil
	c.Mandate.MaxTradesPerDay = nil
	c.Account.CurrentExposure = "0"
	c.Account.Positions = []Position{{Instrument: "AAPL", Exposure: "2000", AvailableQuantity: "2"}}
	action.Source = SourceAI
	action.ActionType = ActionSell
	action.Side = "SELL"
	action.Quantity = "3"
	action.Notional = "300"
	denied := NewEngine().Evaluate(c, action)
	if denied.Decision != Deny || !reason(denied, InsufficientPosition) {
		t.Fatalf("AI sell exceeded the provider-reported holding: %#v", denied)
	}
	action.Quantity = "2"
	if allowed := NewEngine().Evaluate(c, action); allowed.Decision != Allow {
		t.Fatalf("covered AI shadow sell was denied: %#v", allowed)
	}
}
func TestCapitalPositionActivityUniverseAndStaleness(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*EvaluationContext, *ProposedAction)
		want   ReasonCode
	}{
		{"fixed bucket", func(c *EvaluationContext, a *ProposedAction) { a.Notional = "7000" }, CapitalLimitExceeded},
		{"reserve", func(c *EvaluationContext, a *ProposedAction) { c.Account.AvailableCash = "1500"; a.Notional = "1000" }, ReserveViolation},
		{"buying power", func(c *EvaluationContext, a *ProposedAction) { c.Account.BuyingPower = "100" }, InsufficientBuyingPower},
		{"single position", func(c *EvaluationContext, a *ProposedAction) { a.Notional = "4000" }, PositionLimitExceeded},
		{"daily loss", func(c *EvaluationContext, a *ProposedAction) { c.Activity.RealizedDailyPL = ptr("-1000") }, DailyLossLimitExceeded},
		{"trade count", func(c *EvaluationContext, a *ProposedAction) { c.Activity.ActionsToday = ptr(10) }, TradeCountLimitExceeded},
		{"prohibited", func(c *EvaluationContext, a *ProposedAction) { c.Mandate.ProhibitedSymbols = []string{"AAPL"} }, SymbolNotAllowed},
		{"not allowed", func(c *EvaluationContext, a *ProposedAction) { a.Instrument = "TSLA" }, SymbolNotAllowed},
		{"stale", func(c *EvaluationContext, a *ProposedAction) { c.Account.Timestamp = c.Now.Add(-time.Hour) }, StaleAccountData},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, a := fixture()
			tt.mutate(&c, &a)
			g := NewEngine().Evaluate(c, a)
			if !reason(g, tt.want) {
				t.Fatalf("got %#v", g)
			}
		})
	}
	c, a := fixture()
	a.ActionType = ActionSell
	a.Notional = "1900"
	if g := NewEngine().Evaluate(c, a); g.Decision != Allow {
		t.Fatalf("risk-reducing sell denied: %#v", g)
	}
}
func TestOptionsAndMarginFailClosed(t *testing.T) {
	c, a := fixture()
	a.ActionType = ActionOpenOption
	a.Option = &OptionContract{Underlying: "AAPL", Expiration: "2027-01-15", PutCall: "CALL", Strike: "200", ContractMultiplier: 100}
	c.Mandate.OptionsAllowed = false
	if !reason(NewEngine().Evaluate(c, a), OptionsNotAllowed) {
		t.Fatal("options policy bypass")
	}
	c.Mandate.OptionsAllowed = true
	c.Account.Options = CapabilityUnknown
	if !reason(NewEngine().Evaluate(c, a), OptionsCapabilityUnsupported) {
		t.Fatal("unknown options allowed")
	}
	c.Account.Options = CapabilitySupported
	a.RequiresMargin = true
	c.Mandate.MarginAllowed = false
	if !reason(NewEngine().Evaluate(c, a), MarginNotAllowed) {
		t.Fatal("margin policy bypass")
	}
	c.Mandate.MarginAllowed = true
	c.Account.Margin = CapabilityUnknown
	if !reason(NewEngine().Evaluate(c, a), MarginCapabilityUnsupported) {
		t.Fatal("unknown margin allowed")
	}
}

func TestPaperOptionsSimulationAttestationIsNarrowAndAuditable(t *testing.T) {
	optionAction := func() (EvaluationContext, ProposedAction) {
		c, a := fixture()
		c.Mandate.ExecutionMode = "PAPER"
		c.Mandate.PaperOptionsSimulationAttested = true
		c.Account.Options = CapabilityUnknown
		a.ActionType = ActionOpenOption
		a.Option = &OptionContract{Underlying: "AAPL", Expiration: "2027-01-15", PutCall: "PUT", Strike: "200", ContractMultiplier: 100}
		return c, a
	}

	c, a := optionAction()
	evaluation := NewEngine().Evaluate(c, a)
	if evaluation.Decision != Allow || !warning(evaluation, PaperOptionsSimulationAttested) {
		t.Fatalf("attested PAPER simulation did not produce an explicit warning: %#v", evaluation)
	}
	foundWarningCheck := false
	for _, item := range evaluation.Checks {
		if item.Code == PaperOptionsSimulationAttested && item.Result == CheckWarn {
			foundWarningCheck = true
		}
	}
	if !foundWarningCheck {
		t.Fatalf("durable risk checks omitted attestation evidence: %#v", evaluation.Checks)
	}

	for _, test := range []struct {
		name       string
		mode       string
		capability CapabilityState
	}{
		{"shadow", "SHADOW", CapabilityUnknown},
		{"live", "LIVE", CapabilityUnknown},
		{"unsupported", "PAPER", CapabilityUnsupported},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, a := optionAction()
			c.Mandate.ExecutionMode = test.mode
			c.Account.Options = test.capability
			got := NewEngine().Evaluate(c, a)
			if got.Decision != Deny || !reason(got, OptionsCapabilityUnsupported) {
				t.Fatalf("unsafe attestation scope was accepted: %#v", got)
			}
		})
	}

	c, a = optionAction()
	c.Account.Options = CapabilitySupported
	evaluation = NewEngine().Evaluate(c, a)
	if evaluation.Decision != Allow || warning(evaluation, PaperOptionsSimulationAttested) {
		t.Fatalf("known broker support did not use the normal capability path: %#v", evaluation)
	}
}

func TestAttestedPaperOptionStillFailsOneDollarCapitalGate(t *testing.T) {
	c, a := fixture()
	c.Mandate.ExecutionMode = "PAPER"
	c.Mandate.PaperOptionsSimulationAttested = true
	c.Account.Options = CapabilityUnknown
	c.Account.BuyingPower = "1"
	c.Account.AvailableCash = "1"
	a.ActionType = ActionOpenOption
	a.Notional = "30500"
	a.Option = &OptionContract{Underlying: "AAPL", Expiration: "2026-09-25", PutCall: "PUT", Strike: "305", ContractMultiplier: 100}

	evaluation := NewEngine().Evaluate(c, a)
	if evaluation.Decision != Deny || !reason(evaluation, InsufficientBuyingPower) {
		t.Fatalf("$1 capital gate did not deny the simulated option: %#v", evaluation)
	}
	if !warning(evaluation, PaperOptionsSimulationAttested) {
		t.Fatalf("attestation evidence was lost before the capital denial: %#v", evaluation)
	}
}

func manualFixture() (EvaluationContext, ProposedAction) {
	now := time.Date(2026, 8, 22, 3, 0, 0, 0, time.UTC)
	context := EvaluationContext{
		UserID: "u", AccountOwned: true, FinancialEntitled: true, ConnectionUsable: true,
		Bucket:       &CapitalBucket{ID: "b", UserID: "u", AccountID: "a", Name: "Coinbase manual", AllocationType: "FIXED_AMOUNT", AllocationValue: "100", Currency: "USD", ProtectedAmount: "10", Status: "ACTIVE"},
		Reservations: &CapitalReservationSnapshot{Timestamp: now, AccountReservedCash: "0", BucketReservedCash: "0", TargetInstrument: "BTC", TargetReservedQuantity: "0"},
		Account:      &AccountRiskSnapshot{AccountID: "a", Currency: "USD", Timestamp: now, Cash: "250", AvailableCash: "200", BuyingPower: "200", CurrentExposure: "0", Positions: []Position{{Instrument: "BTC", AvailableQuantity: "0.01"}}},
		Now:          now, MaxStaleness: 15 * time.Second,
	}
	action := ProposedAction{ID: "intent", FinancialAccountID: "a", Source: SourceUI, ActionType: ActionBuy, Instrument: "BTC", Side: "BUY", Quantity: "0.0004", Notional: "25.15", CreatedAt: now}
	return context, action
}

func TestManualProposalAccountsForConcurrentCapitalReservations(t *testing.T) {
	context, action := manualFixture()
	context.Reservations.AccountReservedCash = "175"
	context.Reservations.BucketReservedCash = "70"
	if denied := NewEngine().Evaluate(context, action); denied.Decision != Deny || !reason(denied, InsufficientBuyingPower) {
		t.Fatalf("manual proposal reused reserved cash: %#v", denied)
	}

	context, action = manualFixture()
	context.Reservations.BucketReservedCash = "70"
	if denied := NewEngine().Evaluate(context, action); denied.Decision != Deny || !reason(denied, CapitalLimitExceeded) {
		t.Fatalf("manual proposal reused reserved bucket capacity: %#v", denied)
	}
}

func TestManualProposalRequiresBoundedCapitalPolicyAndOwnerApproval(t *testing.T) {
	context, action := manualFixture()
	evaluation := NewEngine().Evaluate(context, action)
	if evaluation.Decision != Allow || !evaluation.ApprovalRequired || evaluation.Mode != "MANUAL_PROPOSAL" || evaluation.PlatformExecutionAvailable || !reason(evaluation, Allowed) {
		t.Fatalf("manual policy did not allow a bounded non-executable proposal: %#v", evaluation)
	}

	context, action = manualFixture()
	context.Bucket = nil
	if denied := NewEngine().Evaluate(context, action); denied.Decision != Deny || !reason(denied, CapitalPolicyRequired) {
		t.Fatalf("manual proposal without a capital policy was accepted: %#v", denied)
	}

	context, action = manualFixture()
	action.Notional = "91"
	if denied := NewEngine().Evaluate(context, action); denied.Decision != Deny || !reason(denied, CapitalLimitExceeded) {
		t.Fatalf("manual proposal exceeded protected bucket capacity: %#v", denied)
	}
}

func TestManualSellRequiresCurrentHoldingButNoAdditionalCapital(t *testing.T) {
	context, action := manualFixture()
	action.ActionType = ActionSell
	action.Side = "SELL"
	action.Quantity = "0.005"
	action.Notional = "300"
	if evaluation := NewEngine().Evaluate(context, action); evaluation.Decision != Allow {
		t.Fatalf("covered manual sell was denied: %#v", evaluation)
	}

	action.Quantity = "0.02"
	if denied := NewEngine().Evaluate(context, action); denied.Decision != Deny || !reason(denied, InsufficientPosition) {
		t.Fatalf("uncovered manual sell was accepted: %#v", denied)
	}

	context, action = manualFixture()
	context.Reservations.TargetReservedQuantity = "0.006"
	action.ActionType = ActionSell
	action.Side = "SELL"
	action.Quantity = "0.005"
	if denied := NewEngine().Evaluate(context, action); denied.Decision != Deny || !reason(denied, InsufficientPosition) {
		t.Fatalf("manual sell reused reserved holdings: %#v", denied)
	}
}
