package risk

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

type Rule interface {
	Code() ReasonCode
	Evaluate(*EvaluationContext, ProposedAction) RiskCheck
}
type rule struct {
	code ReasonCode
	fn   func(*EvaluationContext, ProposedAction) RiskCheck
}

func (r rule) Code() ReasonCode                                          { return r.code }
func (r rule) Evaluate(c *EvaluationContext, a ProposedAction) RiskCheck { return r.fn(c, a) }

type Engine struct{ rules []Rule }

func NewEngine() *Engine {
	return &Engine{rules: []Rule{
		rule{AuthorizationDenied, authorizationRule}, rule{CircuitBreakerActive, breakerRule}, rule{MandateNotReady, mandateRule}, rule{AutonomyDenied, autonomyRule}, rule{StaleAccountData, stalenessRule}, rule{SymbolNotAllowed, universeRule}, rule{OptionsNotAllowed, optionsRule}, rule{MarginNotAllowed, marginRule}, rule{CapitalLimitExceeded, capitalRule}, rule{PositionLimitExceeded, positionRule}, rule{DailyLossLimitExceeded, activityRule},
	}}
}
func check(code ReasonCode, ok bool, message string) RiskCheck {
	r := Pass
	if !ok {
		r = Fail
	}
	return RiskCheck{code, r, message}
}
func (e *Engine) Evaluate(c EvaluationContext, a ProposedAction) RiskEvaluation {
	if c.Now.IsZero() {
		panic("risk evaluation requires a trusted clock")
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("risk evaluation identifier unavailable")
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	id := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	out := RiskEvaluation{ID: id, UserID: c.UserID, AccountID: a.FinancialAccountID, Timestamp: c.Now, Decision: Allow, Checks: []RiskCheck{}, ReasonCodes: []ReasonCode{}, Warnings: []ReasonCode{}, PlatformExecutionAvailable: false}
	if a.MandateID != nil {
		out.MandateID = a.MandateID
		out.MandateVersion = a.MandateVersion
	}
	if c.Mandate != nil {
		out.Mode = c.Mandate.ExecutionMode
	}
	for _, r := range e.rules {
		x := r.Evaluate(&c, a)
		out.Checks = append(out.Checks, x)
		if x.Result == Fail {
			out.Decision = Deny
			out.ReasonCodes = append(out.ReasonCodes, x.Code)
			break
		}
		if x.Result == CheckWarn {
			out.Warnings = append(out.Warnings, x.Code)
		}
	}
	if out.Decision == Allow {
		out.ReasonCodes = append(out.ReasonCodes, Allowed)
		if c.Mandate != nil && c.Mandate.AutonomyLevel == "CONFIRM_EACH" {
			out.ApprovalRequired = true
			out.Warnings = append(out.Warnings, AutonomyRequiresApproval)
		}
		if out.Mode == "LIVE" {
			out.Warnings = append(out.Warnings, PlatformExecutionDisabled)
		}
	}
	return out
}
func authorizationRule(c *EvaluationContext, a ProposedAction) RiskCheck {
	if c.UserID == "" || !c.FinancialEntitled || !c.AccountOwned {
		code := AuthorizationDenied
		if c.UserID != "" && c.FinancialEntitled && !c.AccountOwned {
			code = AccountOwnershipMismatch
		}
		return check(code, false, "Authenticated ownership and financial entitlement could not be established.")
	}
	if a.MandateID != nil && !c.AutomationEntitled {
		return check(AuthorizationDenied, false, "Automation entitlement is required.")
	}
	if !c.ConnectionUsable {
		return check(ConnectionUnavailable, false, "The financial connection is not usable.")
	}
	return check(AuthorizationDenied, true, "Identity, entitlement, account ownership, and connection state are valid.")
}
func breakerRule(c *EvaluationContext, a ProposedAction) RiskCheck {
	for _, b := range c.Breakers {
		if b.State != BreakerOpen {
			continue
		}
		applies := b.Scope == ScopeGlobal || b.Scope == ScopeUser && (b.ScopeID == nil || *b.ScopeID == c.UserID) || b.Scope == ScopeAccount && b.ScopeID != nil && *b.ScopeID == a.FinancialAccountID || b.Scope == ScopeAutomation && a.MandateID != nil && b.ScopeID != nil && *b.ScopeID == *a.MandateID
		if applies {
			return check(CircuitBreakerActive, false, fmt.Sprintf("%s circuit breaker is active.", b.Scope))
		}
	}
	return check(CircuitBreakerActive, true, "No applicable circuit breaker is active.")
}
func mandateRule(c *EvaluationContext, a ProposedAction) RiskCheck {
	if a.MandateID == nil {
		return check(MandateNotReady, true, "No mandate is required for this manual proposal.")
	}
	m := c.Mandate
	if m == nil || m.ID != *a.MandateID {
		return check(MandateNotReady, false, "The exact mandate could not be loaded.")
	}
	if m.UserID != c.UserID {
		return check(MandateOwnershipMismatch, false, "The mandate does not belong to the user.")
	}
	if a.MandateVersion == nil || *a.MandateVersion != m.Version {
		return check(MandateVersionMismatch, false, "The proposed action is not bound to the exact mandate version.")
	}
	if m.AccountID != a.FinancialAccountID || c.Bucket == nil || c.Bucket.ID != m.BucketID || c.Bucket.UserID != c.UserID || c.Bucket.AccountID != a.FinancialAccountID {
		return check(MandateOwnershipMismatch, false, "Mandate, account, and capital bucket bindings do not match.")
	}
	if m.Status == "PAUSED" {
		return check(MandatePaused, false, "The mandate is paused.")
	}
	if m.Status != "READY" {
		return check(MandateNotReady, false, "The mandate status does not permit action evaluation.")
	}
	if c.Now.Before(m.EffectiveFrom) || (m.EffectiveUntil != nil && !c.Now.Before(*m.EffectiveUntil)) {
		return check(MandateExpired, false, "The mandate is outside its effective window.")
	}
	return check(MandateNotReady, true, "The exact immutable mandate version and bindings are valid.")
}
func autonomyRule(c *EvaluationContext, a ProposedAction) RiskCheck {
	if c.Mandate == nil {
		return check(AutonomyDenied, true, "Manual proposal approval is handled by the future trade workflow.")
	}
	m := c.Mandate
	switch m.AutonomyLevel {
	case "RESEARCH_ONLY":
		return check(AutonomyDenied, false, "Research-only mandates cannot authorize financial actions.")
	case "SUGGEST":
		if a.Source != SourceUI {
			return check(AutonomyDenied, false, "Suggested actions require user initiation.")
		}
	case "STRATEGY_AUTONOMOUS":
		if a.Source != SourceStrategy || m.StrategyIdentifier == nil || a.StrategyIdentifier == nil || *m.StrategyIdentifier != *a.StrategyIdentifier {
			return check(StrategyMismatch, false, "Autonomous action must come from the mandate's deterministic strategy.")
		}
	case "FULL_AUTONOMOUS":
	case "CONFIRM_EACH":
	default:
		return check(AutonomyDenied, false, "Unknown autonomy level fails closed.")
	}
	return check(AutonomyDenied, true, "The action source satisfies the mandate autonomy boundary.")
}
func stalenessRule(c *EvaluationContext, a ProposedAction) RiskCheck {
	if c.Account == nil || c.Account.AccountID != a.FinancialAccountID || c.MaxStaleness <= 0 || c.Account.Timestamp.After(c.Now) || c.Now.Sub(c.Account.Timestamp) > c.MaxStaleness {
		return check(StaleAccountData, false, "Required account data is unavailable or stale.")
	}
	return check(StaleAccountData, true, "Account data is within the configured freshness window.")
}
func universeRule(c *EvaluationContext, a ProposedAction) RiskCheck {
	if c.Mandate == nil {
		return check(SymbolNotAllowed, true, "No mandate universe applies.")
	}
	m := c.Mandate
	if len(m.UniverseIDs) > 0 {
		return check(UniverseUnsupported, false, "A trusted classifier is unavailable for the configured universe.")
	}
	s := strings.ToUpper(a.Instrument)
	for _, x := range m.ProhibitedSymbols {
		if strings.ToUpper(x) == s {
			return check(SymbolNotAllowed, false, "The instrument is explicitly prohibited.")
		}
	}
	if len(m.AllowedSymbols) > 0 {
		for _, x := range m.AllowedSymbols {
			if strings.ToUpper(x) == s {
				return check(SymbolNotAllowed, true, "The instrument is in the explicit allowed universe.")
			}
		}
		return check(SymbolNotAllowed, false, "The instrument is not in the explicit allowed universe.")
	}
	return check(SymbolNotAllowed, true, "The instrument is not prohibited.")
}
func optionsRule(c *EvaluationContext, a ProposedAction) RiskCheck {
	opt := a.ActionType == ActionOpenOption || a.ActionType == ActionCloseOption
	if !opt {
		return check(OptionsNotAllowed, true, "The action is not an option action.")
	}
	if c.Mandate == nil || !c.Mandate.OptionsAllowed {
		return check(OptionsNotAllowed, false, "Options are not permitted by the mandate.")
	}
	if a.Option == nil || a.Option.Underlying == "" || a.Option.Expiration == "" || (a.Option.PutCall != "PUT" && a.Option.PutCall != "CALL") || mustPositive(a.Option.Strike) == nil || a.Option.ContractMultiplier <= 0 {
		return check(InvalidAction, false, "Option contract fields are invalid.")
	}
	if c.Account.Options != CapabilitySupported {
		if c.Account.Options == CapabilityUnknown && c.Mandate.ExecutionMode == "PAPER" && c.Mandate.PaperOptionsSimulationAttested {
			return RiskCheck{Code: PaperOptionsSimulationAttested, Result: CheckWarn, Message: "Explicit owner attestation permits PAPER-only options simulation; broker options capability remains unverified."}
		}
		return check(OptionsCapabilityUnsupported, false, "Account options capability is not confirmed supported.")
	}
	return check(OptionsNotAllowed, true, "Options policy, structure, and capability are valid.")
}
func marginRule(c *EvaluationContext, a ProposedAction) RiskCheck {
	if !a.RequiresMargin {
		return check(MarginNotAllowed, true, "The normalized action does not require margin.")
	}
	if c.Mandate == nil || !c.Mandate.MarginAllowed {
		return check(MarginNotAllowed, false, "Margin is not permitted by the mandate.")
	}
	if c.Account.Margin != CapabilitySupported {
		return check(MarginCapabilityUnsupported, false, "Account margin capability is not confirmed supported.")
	}
	return check(MarginNotAllowed, true, "Margin policy and capability are valid.")
}
func rat(s string) *big.Rat {
	x, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil
	}
	return x
}
func mustPositive(s string) *big.Rat {
	x := rat(s)
	if x == nil || x.Sign() <= 0 {
		return nil
	}
	return x
}
func add(a, b *big.Rat) *big.Rat                 { return new(big.Rat).Add(a, b) }
func sub(a, b *big.Rat) *big.Rat                 { return new(big.Rat).Sub(a, b) }
func cmp(a, b *big.Rat) int                      { return a.Cmp(b) }
func proposedNotional(a ProposedAction) *big.Rat { return mustPositive(a.Notional) }
func capitalRule(c *EvaluationContext, a ProposedAction) RiskCheck {
	n := proposedNotional(a)
	if n == nil {
		return check(InvalidAction, false, "Action notional must be an exact positive decimal.")
	}
	if a.ActionType == ActionSell || a.ActionType == ActionCloseOption {
		return check(CapitalLimitExceeded, true, "Exposure-reducing action does not consume additional capital.")
	}
	if c.Account == nil || mustPositive(c.Account.BuyingPower) == nil {
		return check(InsufficientBuyingPower, false, "Buying power is unavailable.")
	}
	if cmp(n, mustPositive(c.Account.BuyingPower)) > 0 {
		return check(InsufficientBuyingPower, false, "Proposed notional exceeds current buying power.")
	}
	if c.Mandate == nil {
		return check(CapitalLimitExceeded, true, "No mandate capital bucket applies.")
	}
	b := c.Bucket
	allocation := rat(b.AllocationValue)
	if b.AllocationType != "FIXED_AMOUNT" {
		base := rat(c.Account.AvailableCash)
		if b.AllocationType == "PERCENT_OF_BUYING_POWER" {
			base = rat(c.Account.BuyingPower)
		}
		if base == nil || allocation == nil {
			return check(CapitalLimitExceeded, false, "Percentage allocation cannot be evaluated.")
		}
		allocation = new(big.Rat).Quo(new(big.Rat).Mul(base, allocation), big.NewRat(100, 1))
	}
	if b.AllocationLimit != nil {
		lim := rat(*b.AllocationLimit)
		if lim == nil {
			return check(CapitalLimitExceeded, false, "Capital limit is invalid.")
		}
		if allocation == nil || cmp(lim, allocation) < 0 {
			allocation = lim
		}
	}
	deployed := rat(c.Account.CurrentExposure)
	protected := rat(b.ProtectedAmount)
	reserve := rat("0")
	if c.Mandate.MinimumCashReserve != nil {
		reserve = rat(*c.Mandate.MinimumCashReserve)
	}
	if allocation == nil || deployed == nil || protected == nil || reserve == nil {
		return check(CapitalLimitExceeded, false, "Required capital inputs are unavailable.")
	}
	if cmp(add(deployed, n), sub(allocation, protected)) > 0 {
		return check(CapitalLimitExceeded, false, "Proposed action exceeds remaining capital bucket capacity.")
	}
	cash := rat(c.Account.AvailableCash)
	if cash == nil || cmp(sub(cash, n), reserve) < 0 {
		return check(ReserveViolation, false, "Proposed action would violate the required cash reserve.")
	}
	if c.Mandate.MaxCapitalDeployed != nil && cmp(add(deployed, n), rat(*c.Mandate.MaxCapitalDeployed)) > 0 {
		return check(CapitalLimitExceeded, false, "Proposed action exceeds maximum deployed capital.")
	}
	return check(CapitalLimitExceeded, true, "Capital bucket, buying power, deployment, and reserve limits pass.")
}
func positionRule(c *EvaluationContext, a ProposedAction) RiskCheck {
	if c.Mandate == nil {
		return check(PositionLimitExceeded, true, "No mandate position limit applies.")
	}
	current := rat("0")
	for _, p := range c.Account.Positions {
		if strings.EqualFold(p.Instrument, a.Instrument) {
			current = rat(p.Exposure)
			if current == nil {
				return check(PositionLimitExceeded, false, "Position exposure is unavailable.")
			}
		}
	}
	next := current
	n := proposedNotional(a)
	if a.ActionType == ActionSell || a.ActionType == ActionCloseOption {
		next = sub(current, n)
		if next.Sign() < 0 {
			next = rat("0")
		}
	} else {
		next = add(current, n)
	}
	if x := c.Mandate.MaxSinglePositionAmount; x != nil && cmp(next, rat(*x)) > 0 {
		return check(PositionLimitExceeded, false, "Resulting position exceeds the single-position amount limit.")
	}
	if x := c.Mandate.MaxSinglePositionPercentage; x != nil {
		total := rat(c.Account.CurrentExposure)
		if total == nil || total.Sign() <= 0 {
			return check(PositionLimitExceeded, false, "Exposure required for concentration is unavailable.")
		}
		projected := total
		if a.ActionType == ActionBuy || a.ActionType == ActionOpenOption {
			projected = add(total, n)
		}
		pct := new(big.Rat).Quo(new(big.Rat).Mul(next, big.NewRat(100, 1)), projected)
		if cmp(pct, rat(*x)) > 0 {
			return check(PositionLimitExceeded, false, "Resulting position exceeds the concentration limit.")
		}
	}
	return check(PositionLimitExceeded, true, "Resulting position and concentration are within limits.")
}
func activityRule(c *EvaluationContext, a ProposedAction) RiskCheck {
	if c.Mandate == nil || (c.Mandate.MaxDailyLoss == nil && c.Mandate.MaxTradesPerDay == nil) {
		return check(DailyLossLimitExceeded, true, "No daily activity limit applies.")
	}
	if c.Activity == nil {
		return check(ActivityDataUnavailable, false, "Required daily activity data is unavailable.")
	}
	if c.Mandate.MaxDailyLoss != nil {
		if c.Activity.RealizedDailyPL == nil {
			return check(ActivityDataUnavailable, false, "Daily P/L is unavailable.")
		}
		pl := rat(*c.Activity.RealizedDailyPL)
		limit := rat(*c.Mandate.MaxDailyLoss)
		if pl == nil || limit == nil || pl.Cmp(new(big.Rat).Neg(limit)) <= 0 {
			return check(DailyLossLimitExceeded, false, "The maximum daily loss is reached.")
		}
	}
	if c.Mandate.MaxTradesPerDay != nil {
		if c.Activity.ActionsToday == nil {
			return check(ActivityDataUnavailable, false, "Daily action count is unavailable.")
		}
		if *c.Activity.ActionsToday >= *c.Mandate.MaxTradesPerDay {
			return check(TradeCountLimitExceeded, false, "The maximum daily action count is reached.")
		}
	}
	return check(DailyLossLimitExceeded, true, "Daily loss and action-count limits pass.")
}
