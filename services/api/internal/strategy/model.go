// Package strategy implements deterministic, provider-independent strategy state machines.
// It deliberately has no financial-provider or Neural Engine dependency.
package strategy

import (
	"encoding/json"
	"time"

	"github.com/arbion/platform/services/api/internal/automation"
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
	AIMonitoring  State = "AI_MONITORING"
)

type ExecutionMode string

const (
	Paper  ExecutionMode = "PAPER"
	Shadow ExecutionMode = "SHADOW"
)

type Instance struct {
	ID, UserID, AutomationMandateID, FinancialAccountID, CapitalBucketID, StrategyIdentifier string
	MandateVersion, DefinitionVersion, StateVersion                                          int
	ExecutionMode                                                                            ExecutionMode
	CurrentState                                                                             State
	Status                                                                                   string
	StartedAt, UpdatedAt                                                                     time.Time
	PausedAt, CompletedAt, LastEvaluatedAt                                                   *time.Time
}

type Parameters = automation.StrategyParameters
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
	Source         string
	InstrumentType string
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

type LifecycleCommand struct {
	EventID                string         `json:"event_id"`
	EventType              LifecycleEvent `json:"event_type"`
	ExpectedStateVersion   int            `json:"expected_state_version"`
	ConfirmPaperSimulation bool           `json:"confirm_paper_simulation"`
}

type LifecycleResult struct {
	ID                 string          `json:"id"`
	EventID            string          `json:"event_id"`
	StrategyInstanceID string          `json:"strategy_instance_id"`
	EventType          LifecycleEvent  `json:"event_type"`
	PreviousState      State           `json:"previous_state"`
	NewState           State           `json:"new_state"`
	StateVersion       int             `json:"state_version"`
	Metadata           json.RawMessage `json:"metadata"`
	OccurredAt         time.Time       `json:"occurred_at"`
	Duplicate          bool            `json:"duplicate"`
}

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
	Status        ExecutionStatus `json:"status"`
	Price         *string         `json:"price,omitempty"`
	Notional      *string         `json:"notional,omitempty"`
	ExpectedState State           `json:"expected_state,omitempty"`
	Reason        string          `json:"reason,omitempty"`
}

type ShadowOutcomeHorizon string

const (
	ShadowOutcomeOneHour         ShadowOutcomeHorizon = "ONE_HOUR"
	ShadowOutcomeTwentyFourHours ShadowOutcomeHorizon = "TWENTY_FOUR_HOURS"
)

type ShadowOutcomeCandidate struct {
	ExecutionRecordID string
	Horizon           ShadowOutcomeHorizon
	Symbol            string
	Side              string
	Quantity          string
	EntryPrice        string
	CreatedAt         time.Time
}

// ShadowOutcome is an immutable, non-live mark of one hypothetical action.
// It is directional evidence only: no fill, fee-adjusted return, or account
// performance is inferred.
type ShadowOutcome struct {
	ID                       string               `json:"id"`
	ExecutionRecordID        string               `json:"execution_record_id"`
	Horizon                  ShadowOutcomeHorizon `json:"horizon"`
	Symbol                   string               `json:"symbol"`
	Side                     string               `json:"side"`
	Quantity                 string               `json:"quantity"`
	EntryPrice               string               `json:"entry_price"`
	ObservedPrice            string               `json:"observed_price"`
	DirectionalChangeUSD     string               `json:"directional_change_usd"`
	DirectionalChangePercent string               `json:"directional_change_percent"`
	PricingBasis             string               `json:"pricing_basis"`
	MarketFeed               string               `json:"market_feed"`
	MarketQuality            string               `json:"market_quality"`
	MarketObservedAt         time.Time            `json:"market_observed_at"`
	EvaluatedAt              time.Time            `json:"evaluated_at"`
	ElapsedSeconds           int64                `json:"elapsed_seconds"`
}

const (
	ShadowScorecardMinimumSample           = 20
	ShadowEvidenceMinimumWindowHours int64 = 7 * 24
	ShadowRouteProvenanceExplicit          = "EXPLICIT"
	ShadowRouteProvenanceLegacy            = "UNATTRIBUTED_LEGACY"
)

// ShadowHorizonScore summarizes immutable hypothetical marks at one horizon.
// It is descriptive evidence only and never represents realized performance.
type ShadowHorizonScore struct {
	Horizon                            ShadowOutcomeHorizon `json:"horizon"`
	SampleSize                         int                  `json:"sample_size"`
	FavorableMarks                     int                  `json:"favorable_marks"`
	UnfavorableMarks                   int                  `json:"unfavorable_marks"`
	FlatMarks                          int                  `json:"flat_marks"`
	FavorableRatePercent               *string              `json:"favorable_rate_percent,omitempty"`
	AverageDirectionalChangePercent    *string              `json:"average_directional_change_percent,omitempty"`
	FirstEvaluatedAt                   *time.Time           `json:"first_evaluated_at,omitempty"`
	LastEvaluatedAt                    *time.Time           `json:"last_evaluated_at,omitempty"`
	Interpretation                     string               `json:"interpretation"`
	MinimumSampleForObservationalLabel int                  `json:"minimum_sample_for_observational_label"`
}

type ShadowScorecard struct {
	StrategyInstanceID string               `json:"strategy_instance_id"`
	TotalMarks         int                  `json:"total_marks"`
	Horizons           []ShadowHorizonScore `json:"horizons"`
	Behavior           ShadowBehaviorScore  `json:"behavior"`
	EvidenceGate       ShadowEvidenceGate   `json:"evidence_gate"`
}

// ShadowBehaviorScore summarizes immutable AI decision behavior for one
// owner-scoped SHADOW strategy instance. Counts describe what Arbion recorded;
// they are not trading performance or model accuracy measures.
type ShadowBehaviorScore struct {
	TotalAIDecisions            int                    `json:"total_ai_decisions"`
	Abstentions                 int                    `json:"abstentions"`
	ProposedDecisions           int                    `json:"proposed_decisions"`
	RiskHeldDecisions           int                    `json:"risk_held_decisions"`
	RepeatActionCooldownHolds   int                    `json:"repeat_action_cooldown_holds"`
	WouldHaveSubmittedDecisions int                    `json:"would_have_submitted_decisions"`
	AttributedDecisions         int                    `json:"attributed_decisions"`
	UnattributedLegacyDecisions int                    `json:"unattributed_legacy_decisions"`
	AbstentionRatePercent       *string                `json:"abstention_rate_percent,omitempty"`
	ProposalRatePercent         *string                `json:"proposal_rate_percent,omitempty"`
	AverageDecisionIntervalMins *string                `json:"average_decision_interval_minutes,omitempty"`
	FirstDecisionAt             *time.Time             `json:"first_decision_at,omitempty"`
	LastDecisionAt              *time.Time             `json:"last_decision_at,omitempty"`
	Routes                      []ShadowRouteBehavior  `json:"routes"`
	Symbols                     []ShadowSymbolBehavior `json:"symbols"`
}

// ShadowRouteBehavior keeps model routes separate and never infers provenance
// for journal entries written before explicit route fields were recorded.
type ShadowRouteBehavior struct {
	AIProvider                  string  `json:"ai_provider,omitempty"`
	ModelID                     string  `json:"model_id,omitempty"`
	Profile                     string  `json:"profile,omitempty"`
	ProvenanceStatus            string  `json:"provenance_status"`
	TotalDecisions              int     `json:"total_decisions"`
	Abstentions                 int     `json:"abstentions"`
	ProposedDecisions           int     `json:"proposed_decisions"`
	RiskHeldDecisions           int     `json:"risk_held_decisions"`
	RepeatActionCooldownHolds   int     `json:"repeat_action_cooldown_holds"`
	WouldHaveSubmittedDecisions int     `json:"would_have_submitted_decisions"`
	OneHourOutcomeMarks         int     `json:"one_hour_outcome_marks"`
	TwentyFourHourOutcomeMarks  int     `json:"twenty_four_hour_outcome_marks"`
	MeasuredLatencyDecisions    int     `json:"measured_latency_decisions"`
	AverageLatencyMilliseconds  *string `json:"average_latency_milliseconds,omitempty"`
	MeteredUsageDecisions       int     `json:"metered_usage_decisions"`
	RecordedInputTokens         int64   `json:"recorded_input_tokens"`
	RecordedOutputTokens        int64   `json:"recorded_output_tokens"`
}

// ShadowSymbolBehavior describes proposal disposition and mark coverage by
// symbol. Abstentions intentionally have no symbol and are excluded.
type ShadowSymbolBehavior struct {
	Symbol                      string `json:"symbol"`
	ProposedDecisions           int    `json:"proposed_decisions"`
	RiskHeldDecisions           int    `json:"risk_held_decisions"`
	WouldHaveSubmittedDecisions int    `json:"would_have_submitted_decisions"`
	OneHourOutcomeMarks         int    `json:"one_hour_outcome_marks"`
	TwentyFourHourOutcomeMarks  int    `json:"twenty_four_hour_outcome_marks"`
}

// ShadowEvidenceGate determines only whether enough non-live evidence exists
// for owner review. It cannot authorize execution or represent performance.
type ShadowEvidenceGate struct {
	Status                      string   `json:"status"`
	Blockers                    []string `json:"blockers"`
	OneHourSampleSize           int      `json:"one_hour_sample_size"`
	TwentyFourHourSampleSize    int      `json:"twenty_four_hour_sample_size"`
	MinimumSamplePerHorizon     int      `json:"minimum_sample_per_horizon"`
	EvidenceWindowHours         int64    `json:"evidence_window_hours"`
	MinimumEvidenceWindowHours  int64    `json:"minimum_evidence_window_hours"`
	ScheduleHealthy             bool     `json:"schedule_healthy"`
	LastScheduleStatus          string   `json:"last_schedule_status,omitempty"`
	ConsecutiveScheduleFailures int      `json:"consecutive_schedule_failures"`
	ExecutionBoundary           string   `json:"execution_boundary"`
	LiveExecutionAvailable      bool     `json:"live_execution_available"`
}

// PaperPortfolio is the owner-facing, provider-independent projection of one
// simulated ledger. It deliberately excludes provider and account identifiers.
type PaperPortfolio struct {
	StrategyInstanceID string          `json:"strategy_instance_id"`
	Currency           string          `json:"currency"`
	StartingCash       string          `json:"starting_cash"`
	Cash               string          `json:"cash"`
	Version            int64           `json:"version"`
	Positions          []PaperPosition `json:"positions"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// PaperPosition contains only normalized simulated position facts.
type PaperPosition struct {
	Symbol       string    `json:"symbol"`
	Instrument   string    `json:"instrument"`
	OptionType   string    `json:"option_type,omitempty"`
	Strike       string    `json:"strike,omitempty"`
	Expiration   string    `json:"expiration,omitempty"`
	Quantity     string    `json:"quantity"`
	AveragePrice string    `json:"average_price"`
	IsOpen       bool      `json:"is_open"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// JournalCursor identifies a stable point in the reverse-chronological
// decision feed. IDs disambiguate entries that share the same timestamp.
type JournalCursor struct {
	CreatedAt time.Time
	ID        string
}

// JournalActivity is the credential-free, owner-facing projection of one
// durable strategy decision and its linked risk/execution evidence.
type JournalActivity struct {
	ID                  string          `json:"id"`
	CreatedAt           time.Time       `json:"created_at"`
	StrategyInstanceID  string          `json:"strategy_instance_id"`
	FinancialAccountID  string          `json:"financial_account_id"`
	AccountDisplayName  string          `json:"account_display_name"`
	MandateID           string          `json:"mandate_id"`
	MandateVersion      int             `json:"mandate_version"`
	StrategyIdentifier  string          `json:"strategy_identifier"`
	ExecutionMode       ExecutionMode   `json:"execution_mode"`
	StrategyState       string          `json:"strategy_state"`
	ResultingState      *string         `json:"resulting_state,omitempty"`
	Source              string          `json:"source"`
	DecisionType        string          `json:"decision_type"`
	StructuredRationale json.RawMessage `json:"structured_rationale"`
	RiskDecision        *string         `json:"risk_decision,omitempty"`
	ApprovalRequired    *bool           `json:"approval_required,omitempty"`
	RiskReasonCodes     json.RawMessage `json:"risk_reason_codes,omitempty"`
	RiskChecks          json.RawMessage `json:"risk_checks,omitempty"`
	ExecutionStatus     *string         `json:"execution_status,omitempty"`
	Symbol              *string         `json:"symbol,omitempty"`
	Instrument          *string         `json:"instrument,omitempty"`
	Side                *string         `json:"side,omitempty"`
	Quantity            *string         `json:"quantity,omitempty"`
	Price               *string         `json:"price,omitempty"`
	Notional            *string         `json:"notional,omitempty"`
}

type JournalPage struct {
	Entries    []JournalActivity
	NextCursor *JournalCursor
}

type ScheduleStatus struct {
	Enabled             bool       `json:"enabled"`
	StrategyInstanceID  string     `json:"strategy_instance_id"`
	MandateID           string     `json:"mandate_id,omitempty"`
	MandateVersion      int        `json:"mandate_version,omitempty"`
	IntervalMinutes     int        `json:"interval_minutes,omitempty"`
	Session             string     `json:"session,omitempty"`
	NextRunAt           *time.Time `json:"next_run_at,omitempty"`
	LastStartedAt       *time.Time `json:"last_started_at,omitempty"`
	LastCompletedAt     *time.Time `json:"last_completed_at,omitempty"`
	LastStatus          *string    `json:"last_status,omitempty"`
	LastErrorCode       *string    `json:"last_error_code,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
}

type ScheduledRun struct {
	StrategyInstanceID               string
	UserID                           string
	FinancialAccountID               string
	OwnerEmail                       string
	OwnerEmailVerified               bool
	MandateID                        string
	MandateVersion                   int
	ExecutionMode                    ExecutionMode
	CurrentState                     State
	IntervalMinutes                  int
	Session                          string
	ScheduledFor                     time.Time
	LeaseToken                       string
	NotifyEvaluation                 bool
	NotifyLifecycle                  bool
	NotifyFirstFailure               bool
	NotifyReconciliationReview       bool
	LastReconciliationNotificationID *string
	PreviousErrorCode                *string
	ConsecutiveFailures              int
}

type ScheduleCompletion struct {
	Status                       string
	ErrorCode                    string
	CompletedAt                  time.Time
	NextRunAt                    time.Time
	ReconciliationID             string
	ReconciliationReviewRequired bool
}
type EvaluationOutcome struct {
	Execution              ExecutionResult   `json:"execution"`
	RiskDecision           risk.Decision     `json:"risk_decision"`
	RiskReasonCodes        []risk.ReasonCode `json:"risk_reason_codes"`
	RiskChecks             []risk.RiskCheck  `json:"risk_checks"`
	ApprovalRequired       bool              `json:"approval_required"`
	LiveExecutionAvailable bool              `json:"live_execution_available"`
	AIDecision             string            `json:"ai_decision,omitempty"`
	Confidence             string            `json:"confidence,omitempty"`
}
