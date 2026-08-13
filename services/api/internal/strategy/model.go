// Package strategy implements deterministic, provider-independent strategy state machines.
// It deliberately has no financial-provider or Neural Engine dependency.
package strategy

import (
	"encoding/json"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

type State string

const (
	ReadyForPut   State = "READY_FOR_PUT"
	PutProposed   State = "PUT_PROPOSED"
	ShortPutOpen  State = "SHORT_PUT_OPEN"
	PutExpired    State = "PUT_EXPIRED"
	Assigned      State = "ASSIGNED"
	LongShares    State = "LONG_SHARES"
	ReadyForCall  State = "READY_FOR_CALL"
	CallProposed  State = "CALL_PROPOSED"
	ShortCallOpen State = "SHORT_CALL_OPEN"
	CallExpired   State = "CALL_EXPIRED"
	CalledAway    State = "CALLED_AWAY"
	Cash          State = "CASH"
	Completed     State = "COMPLETED"
	Paused        State = "PAUSED"
	Error         State = "ERROR"
)

type ExecutionMode string

const (
	Paper  ExecutionMode = "PAPER"
	Shadow ExecutionMode = "SHADOW"
)

type Instance struct {
	ID, UserID, AutomationMandateID, FinancialAccountID, StrategyIdentifier string
	MandateVersion, DefinitionVersion, StateVersion                         int
	ExecutionMode                                                           ExecutionMode
	CurrentState                                                            State
	Status                                                                  string
	StartedAt, UpdatedAt                                                    time.Time
	PausedAt, CompletedAt, LastEvaluatedAt                                  *time.Time
}

type Parameters struct {
	Symbols                                     []string `json:"symbols"`
	MinimumDTE, MaximumDTE                      int
	TargetDelta, TargetDeltaMin, TargetDeltaMax string
	MinimumPremium                              *string
	MaximumContracts                            int
	AssignmentHandlingPolicy                    string
}
type OptionCandidate struct {
	Underlying, OptionType, Strike, Expiration string
	Bid, Ask, Mark, Delta, ImpliedVolatility   *string
	OpenInterest, Volume                       *int
	Timestamp                                  time.Time
}
type MarketSnapshot struct {
	Symbol                    string
	Timestamp                 time.Time
	UnderlyingPrice, Bid, Ask *string
	Options                   []OptionCandidate
}
type Position struct{ Symbol, Instrument, Quantity string }
type AccountSnapshot struct {
	Timestamp     time.Time
	AvailableCash string
	Positions     []Position
}
type MandateSnapshot struct {
	ID                 string
	Version            int
	StrategyIdentifier string
}
type EvaluationInput struct {
	EventID           string
	Timestamp         time.Time
	Account           AccountSnapshot
	Parameters        Parameters
	Mandate           MandateSnapshot
	Market            MarketSnapshot
	ExistingPositions []Position
	PriorState        State
}
type Decision struct {
	ProposedAction *risk.ProposedAction
	ProposedState  State
	CandidateCount int
	Selected       *OptionCandidate
	Reason         string
	Rationale      json.RawMessage
}
type LifecycleEvent string

const (
	ExpireWorthless LifecycleEvent = "EXPIRE_WORTHLESS"
	Assignment      LifecycleEvent = "ASSIGNED"
	CallAway        LifecycleEvent = "CALLED_AWAY"
)

type Transition struct {
	ID, StrategyInstanceID, Trigger                       string
	PreviousState, NewState                               State
	StateVersion                                          int
	ProposedActionID, RiskEvaluationID, ExecutionRecordID *string
	Metadata                                              json.RawMessage
	OccurredAt                                            time.Time
}

type ExecutionStatus string

const (
	ExecutionProposed  ExecutionStatus = "PROPOSED"
	RiskDenied         ExecutionStatus = "RISK_DENIED"
	SimulatedFilled    ExecutionStatus = "SIMULATED_FILLED"
	SimulatedRejected  ExecutionStatus = "SIMULATED_REJECTED"
	WouldHaveSubmitted ExecutionStatus = "WOULD_HAVE_SUBMITTED"
	ExecutionCanceled  ExecutionStatus = "CANCELED"
	ExecutionError     ExecutionStatus = "ERROR"
)

type ExecutionRecord struct {
	ID, IdempotencyKey, UserID, StrategyInstanceID, MandateID, ProposedActionID, RiskEvaluationID string
	MandateVersion                                                                                int
	Mode                                                                                          ExecutionMode
	Status                                                                                        ExecutionStatus
	Symbol, Instrument, Side, Quantity                                                            string
	Price, Notional                                                                               *string
	Metadata                                                                                      json.RawMessage
	CreatedAt                                                                                     time.Time
}
type ExecutionResult struct {
	Status          ExecutionStatus
	Price, Notional *string
	ExpectedState   State
	Reason          string
}
