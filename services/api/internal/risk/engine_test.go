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
