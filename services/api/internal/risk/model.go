// Package risk implements Arbion's provider-independent, deterministic action gate.
// It has no connector dependency and cannot submit, preview, cancel, or replace orders.
package risk

import "time"

type ActionSource string

const (
	SourceUI           ActionSource = "UI"
	SourceConversation ActionSource = "CONVERSATION"
	SourceStrategy     ActionSource = "STRATEGY"
	SourceAI           ActionSource = "AI"
	SourceHybrid       ActionSource = "HYBRID"
	SourceSystem       ActionSource = "SYSTEM"
)

type ActionType string

const (
	ActionBuy         ActionType = "BUY"
	ActionSell        ActionType = "SELL"
	ActionOpenOption  ActionType = "OPEN_OPTION"
	ActionCloseOption ActionType = "CLOSE_OPTION"
)

type OptionContract struct {
	Underlying, Expiration, PutCall string
	Strike                          string
	ContractMultiplier              int
}
type ProposedAction struct {
	ID, CorrelationID, FinancialAccountID                 string
	Source                                                ActionSource
	ActionType                                            ActionType
	MandateID                                             *string
	MandateVersion                                        *int
	Instrument, Side, Quantity, Notional                  string
	EstimatedPrice                                        *string
	Option                                                *OptionContract
	StrategyIdentifier, StrategyInstanceID, StrategyState *string
	RequiresMargin                                        bool
	CreatedAt                                             time.Time
}
type CapabilityState string

const (
	CapabilitySupported   CapabilityState = "SUPPORTED"
	CapabilityUnsupported CapabilityState = "UNSUPPORTED"
	CapabilityUnknown     CapabilityState = "UNKNOWN"
)

type Position struct{ Instrument, Exposure, AvailableQuantity string }
type AccountRiskSnapshot struct {
	AccountID, Currency                               string
	Timestamp                                         time.Time
	Cash, AvailableCash, BuyingPower, CurrentExposure string
	Positions                                         []Position
	Options, Margin                                   CapabilityState
}
type RiskActivitySnapshot struct {
	Timestamp                       time.Time
	RealizedDailyPL, CurrentDailyPL *string
	ActionsToday                    *int
}
type CapitalBucket struct {
	ID, UserID, AccountID, Name, AllocationType, AllocationValue, Currency, ProtectedAmount, Status string
	AllocationLimit                                                                                 *string
	IsReserve                                                                                       bool
}
type Mandate struct {
	ID, UserID, AccountID, BucketID, Status, AutomationType, AutonomyLevel, ExecutionMode                      string
	Version                                                                                                    int
	StrategyIdentifier                                                                                         *string
	EffectiveFrom                                                                                              time.Time
	EffectiveUntil                                                                                             *time.Time
	AllowedSymbols, ProhibitedSymbols, UniverseIDs                                                             []string
	MarginAllowed, OptionsAllowed, PaperOptionsSimulationAttested                                              bool
	MaxCapitalDeployed, MaxSinglePositionAmount, MaxSinglePositionPercentage, MaxDailyLoss, MinimumCashReserve *string
	MaxTradesPerDay                                                                                            *int
}
type EvaluationContext struct {
	UserID                                                                string
	AccountOwned, FinancialEntitled, AutomationEntitled, ConnectionUsable bool
	Mandate                                                               *Mandate
	Bucket                                                                *CapitalBucket
	Account                                                               *AccountRiskSnapshot
	Activity                                                              *RiskActivitySnapshot
	Breakers                                                              []CircuitBreaker
	Now                                                                   time.Time
	MaxStaleness                                                          time.Duration
}
type Decision string

const (
	Allow Decision = "ALLOW"
	Deny  Decision = "DENY"
	Warn  Decision = "WARN"
)

type CheckResult string

const (
	Pass      CheckResult = "PASS"
	Fail      CheckResult = "FAIL"
	CheckWarn CheckResult = "WARN"
)

type RiskCheck struct {
	Code    ReasonCode  `json:"code"`
	Result  CheckResult `json:"result"`
	Message string      `json:"message"`
}
type RiskEvaluation struct {
	ID, UserID, AccountID      string
	MandateID                  *string
	MandateVersion             *int
	Timestamp                  time.Time
	Decision                   Decision
	Checks                     []RiskCheck
	ReasonCodes                []ReasonCode
	Warnings                   []ReasonCode
	ApprovalRequired           bool
	Mode                       string
	PlatformExecutionAvailable bool
}

type BreakerScope string

const (
	ScopeAutomation BreakerScope = "AUTOMATION"
	ScopeAccount    BreakerScope = "ACCOUNT"
	ScopeUser       BreakerScope = "USER"
	ScopeGlobal     BreakerScope = "GLOBAL"
)

type BreakerState string

const (
	BreakerOpen   BreakerState = "OPEN"
	BreakerClosed BreakerState = "CLOSED"
)

type CircuitBreaker struct {
	ID               string       `json:"id"`
	Scope            BreakerScope `json:"scope"`
	ScopeID          *string      `json:"scope_id,omitempty"`
	State            BreakerState `json:"state"`
	Reason           string       `json:"reason"`
	Source           string       `json:"source"`
	EngagedByUserID  *string      `json:"engaged_by_user_id,omitempty"`
	EngagedAt        time.Time    `json:"engaged_at"`
	ReleasedByUserID *string      `json:"released_by_user_id,omitempty"`
	ReleasedAt       *time.Time   `json:"released_at,omitempty"`
}
