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

// CapitalReservation is the durable, non-live capital claim attached to one
// strategy instance. A released reservation remains queryable as evidence.
type CapitalReservation struct {
	ID                     string        `json:"id"`
	StrategyInstanceID     string        `json:"strategy_instance_id"`
	FinancialAccountID     string        `json:"financial_account_id"`
	CapitalBucketID        string        `json:"capital_bucket_id"`
	ExecutionMode          ExecutionMode `json:"execution_mode"`
	ReservationAmount      *string       `json:"reservation_amount,omitempty"`
	Currency               string        `json:"currency"`
	ReservationBasis       string        `json:"reservation_basis"`
	AccountAllocationLimit *string       `json:"account_allocation_limit,omitempty"`
	Status                 string        `json:"status"`
	ReservedAt             time.Time     `json:"reserved_at"`
	ReleasedAt             *time.Time    `json:"released_at,omitempty"`
	ReleaseReason          *string       `json:"release_reason,omitempty"`
}

type capitalReservationClaim struct {
	Amount                 string
	Currency               string
	Basis                  string
	AccountAllocationLimit *string
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
type Position struct{ Symbol, Instrument, Quantity, AveragePrice string }
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
	MedianDirectionalChangePercent     *string              `json:"median_directional_change_percent,omitempty"`
	BestDirectionalChangePercent       *string              `json:"best_directional_change_percent,omitempty"`
	WorstDirectionalChangePercent      *string              `json:"worst_directional_change_percent,omitempty"`
	AverageDirectionalChangeUSD        *string              `json:"average_directional_change_usd,omitempty"`
	CumulativeDirectionalChangeUSD     *string              `json:"cumulative_directional_change_usd,omitempty"`
	FirstEvaluatedAt                   *time.Time           `json:"first_evaluated_at,omitempty"`
	LastEvaluatedAt                    *time.Time           `json:"last_evaluated_at,omitempty"`
	Interpretation                     string               `json:"interpretation"`
	MinimumSampleForObservationalLabel int                  `json:"minimum_sample_for_observational_label"`
}

type ShadowScorecard struct {
	StrategyInstanceID        string                `json:"strategy_instance_id"`
	TotalMarks                int                   `json:"total_marks"`
	Horizons                  []ShadowHorizonScore  `json:"horizons"`
	Behavior                  ShadowBehaviorScore   `json:"behavior"`
	EvidenceGate              ShadowEvidenceGate    `json:"evidence_gate"`
	EvidenceReviewFingerprint string                `json:"evidence_review_fingerprint"`
	LatestEvidenceReview      *ShadowEvidenceReview `json:"latest_evidence_review,omitempty"`
	CurrentEvidenceReviewed   bool                  `json:"current_evidence_reviewed"`
}

const (
	ShadowEvidenceReviewScope = "NON_LIVE_EVIDENCE_ONLY"
	ShadowExecutionBoundary   = "SHADOW_ONLY"
)

// ShadowEvidenceReview is an immutable MFA-backed acknowledgment that an owner
// inspected one exact non-live scorecard snapshot. It grants no trading,
// promotion, approval, order, or execution authority.
type ShadowEvidenceReview struct {
	ID                          string    `json:"id"`
	StrategyInstanceID          string    `json:"strategy_instance_id"`
	MandateID                   string    `json:"mandate_id"`
	MandateVersion              int       `json:"mandate_version"`
	EvidenceFingerprint         string    `json:"evidence_fingerprint"`
	GateStatus                  string    `json:"gate_status"`
	OneHourSampleSize           int       `json:"one_hour_sample_size"`
	TwentyFourHourSampleSize    int       `json:"twenty_four_hour_sample_size"`
	EvidenceWindowHours         int64     `json:"evidence_window_hours"`
	ScheduleHealthy             bool      `json:"schedule_healthy"`
	LastScheduleStatus          string    `json:"last_schedule_status"`
	ConsecutiveScheduleFailures int       `json:"consecutive_schedule_failures"`
	ExecutionBoundary           string    `json:"execution_boundary"`
	LiveExecutionAvailable      bool      `json:"live_execution_available"`
	ReviewScope                 string    `json:"review_scope"`
	MFAMethod                   string    `json:"mfa_method"`
	ReviewedAt                  time.Time `json:"reviewed_at"`
	CreatedAt                   time.Time `json:"created_at"`
}

type ShadowEvidenceReviewCommand struct {
	EvidenceFingerprint  string `json:"evidence_fingerprint"`
	ConfirmNonLiveReview bool   `json:"confirm_non_live_review"`
	MFACode              string `json:"mfa_code"`
}

// ShadowEvidenceReviewCursor identifies a stable point in the immutable,
// reverse-chronological owner review ledger.
type ShadowEvidenceReviewCursor struct {
	ReviewedAt time.Time
	ID         string
}

type ShadowEvidenceReviewPage struct {
	Reviews    []ShadowEvidenceReview
	NextCursor *ShadowEvidenceReviewCursor
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
	Symbol                      string                        `json:"symbol"`
	ProposedDecisions           int                           `json:"proposed_decisions"`
	RiskHeldDecisions           int                           `json:"risk_held_decisions"`
	WouldHaveSubmittedDecisions int                           `json:"would_have_submitted_decisions"`
	OneHourOutcomeMarks         int                           `json:"one_hour_outcome_marks"`
	TwentyFourHourOutcomeMarks  int                           `json:"twenty_four_hour_outcome_marks"`
	Horizons                    []ShadowSymbolHorizonBehavior `json:"horizons"`
}

// ShadowSymbolHorizonBehavior keeps asset evidence separated by horizon. Its
// marked values are hypothetical and cannot represent portfolio performance.
type ShadowSymbolHorizonBehavior struct {
	Horizon                         ShadowOutcomeHorizon `json:"horizon"`
	SampleSize                      int                  `json:"sample_size"`
	FavorableMarks                  int                  `json:"favorable_marks"`
	UnfavorableMarks                int                  `json:"unfavorable_marks"`
	FlatMarks                       int                  `json:"flat_marks"`
	FavorableRatePercent            *string              `json:"favorable_rate_percent,omitempty"`
	AverageDirectionalChangePercent *string              `json:"average_directional_change_percent,omitempty"`
	AverageDirectionalChangeUSD     *string              `json:"average_directional_change_usd,omitempty"`
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
	StrategyInstanceID string               `json:"strategy_instance_id"`
	Currency           string               `json:"currency"`
	StartingCash       string               `json:"starting_cash"`
	Cash               string               `json:"cash"`
	Version            int64                `json:"version"`
	Positions          []PaperPosition      `json:"positions"`
	RealizedOutcome    PaperRealizedOutcome `json:"realized_outcome"`
	ExecutionCosts     PaperExecutionCosts  `json:"execution_costs"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

// PaperRealizedOutcome is an exact, read-only average-cost projection derived
// from the complete immutable simulation fill chain. UNAVAILABLE is explicit:
// legacy or inconsistent evidence is never repaired or inferred.
type PaperRealizedOutcome struct {
	Status                  string                       `json:"status"`
	CalculationMethod       string                       `json:"calculation_method,omitempty"`
	HistoricalCoverage      string                       `json:"historical_coverage,omitempty"`
	TotalRealizedProfitLoss string                       `json:"total_realized_profit_loss,omitempty"`
	FillCount               int                          `json:"fill_count"`
	SellFillCount           int                          `json:"sell_fill_count"`
	FirstFillAt             *time.Time                   `json:"first_fill_at,omitempty"`
	LastFillAt              *time.Time                   `json:"last_fill_at,omitempty"`
	Symbols                 []PaperRealizedSymbolOutcome `json:"symbols"`
}

type PaperRealizedSymbolOutcome struct {
	Symbol                 string `json:"symbol"`
	Instrument             string `json:"instrument"`
	RealizedProfitLoss     string `json:"realized_profit_loss"`
	BuyFillCount           int    `json:"buy_fill_count"`
	SellFillCount          int    `json:"sell_fill_count"`
	TotalFees              string `json:"total_fees"`
	EndingPositionQuantity string `json:"ending_position_quantity"`
	EndingAverageCost      string `json:"ending_average_cost,omitempty"`
}

// PaperExecutionCosts attributes the explicit simulated execution costs in the
// complete immutable fill chain. Fees and adverse slippage are deliberately
// separate from realized, unrealized, and total simulated outcomes.
type PaperExecutionCosts struct {
	Status                      string                     `json:"status"`
	CalculationMethod           string                     `json:"calculation_method,omitempty"`
	HistoricalCoverage          string                     `json:"historical_coverage,omitempty"`
	TotalFees                   string                     `json:"total_fees,omitempty"`
	TotalAdverseSlippage        string                     `json:"total_adverse_slippage,omitempty"`
	TotalExplicitCost           string                     `json:"total_explicit_cost,omitempty"`
	ProviderReferenceNotional   string                     `json:"provider_reference_notional,omitempty"`
	GrossNotional               string                     `json:"gross_notional,omitempty"`
	AllInCostRateBPS            string                     `json:"all_in_cost_rate_bps,omitempty"`
	FillNotionalResidual        string                     `json:"fill_notional_residual,omitempty"`
	MaximumAbsoluteFillResidual string                     `json:"maximum_absolute_fill_residual,omitempty"`
	ResidualBoundPerFill        string                     `json:"residual_bound_per_fill,omitempty"`
	FillCount                   int                        `json:"fill_count"`
	BuyFillCount                int                        `json:"buy_fill_count"`
	SellFillCount               int                        `json:"sell_fill_count"`
	FirstFillAt                 *time.Time                 `json:"first_fill_at,omitempty"`
	LastFillAt                  *time.Time                 `json:"last_fill_at,omitempty"`
	MarketProviders             []string                   `json:"market_providers"`
	MarketFeeds                 []string                   `json:"market_feeds"`
	MarketQualities             []string                   `json:"market_qualities"`
	Sides                       []PaperExecutionSideCost   `json:"sides"`
	Symbols                     []PaperExecutionSymbolCost `json:"symbols"`
	TimelineSampleCount         int                        `json:"timeline_sample_count"`
	TimelineCapped              bool                       `json:"timeline_capped"`
	Timeline                    []PaperExecutionCheckpoint `json:"timeline"`
}

type PaperExecutionSymbolCost struct {
	Symbol                    string `json:"symbol"`
	Instrument                string `json:"instrument"`
	TotalFees                 string `json:"total_fees"`
	AdverseSlippage           string `json:"adverse_slippage"`
	TotalExplicitCost         string `json:"total_explicit_cost"`
	ProviderReferenceNotional string `json:"provider_reference_notional"`
	GrossNotional             string `json:"gross_notional"`
	AllInCostRateBPS          string `json:"all_in_cost_rate_bps"`
	FillCount                 int    `json:"fill_count"`
	BuyFillCount              int    `json:"buy_fill_count"`
	SellFillCount             int    `json:"sell_fill_count"`
}

type PaperExecutionSideCost struct {
	Side                      string `json:"side"`
	TotalFees                 string `json:"total_fees"`
	AdverseSlippage           string `json:"adverse_slippage"`
	TotalExplicitCost         string `json:"total_explicit_cost"`
	ProviderReferenceNotional string `json:"provider_reference_notional"`
	GrossNotional             string `json:"gross_notional"`
	AllInCostRateBPS          string `json:"all_in_cost_rate_bps"`
	FillCount                 int    `json:"fill_count"`
}

// PaperExecutionCheckpoint is one bounded chronological checkpoint from the
// complete immutable Paper fill replay. Change direction compares the saved
// fixed-decimal cumulative rates and never implies performance or causality.
type PaperExecutionCheckpoint struct {
	Sequence                            int       `json:"sequence"`
	FillID                              string    `json:"fill_id"`
	ExecutionRecordID                   string    `json:"execution_record_id"`
	ProposedActionID                    string    `json:"proposed_action_id"`
	RiskEvaluationID                    string    `json:"risk_evaluation_id"`
	Symbol                              string    `json:"symbol"`
	Instrument                          string    `json:"instrument"`
	Side                                string    `json:"side"`
	Fee                                 string    `json:"fee"`
	AdverseSlippage                     string    `json:"adverse_slippage"`
	ExplicitCost                        string    `json:"explicit_cost"`
	ProviderReferenceNotional           string    `json:"provider_reference_notional"`
	GrossNotional                       string    `json:"gross_notional"`
	FillNotionalResidual                string    `json:"fill_notional_residual"`
	CumulativeFees                      string    `json:"cumulative_fees"`
	CumulativeAdverseSlippage           string    `json:"cumulative_adverse_slippage"`
	CumulativeExplicitCost              string    `json:"cumulative_explicit_cost"`
	CumulativeProviderReferenceNotional string    `json:"cumulative_provider_reference_notional"`
	CumulativeGrossNotional             string    `json:"cumulative_gross_notional"`
	CumulativeAllInCostRateBPS          string    `json:"cumulative_all_in_cost_rate_bps"`
	CumulativeRateChange                string    `json:"cumulative_rate_change"`
	MarketProvider                      string    `json:"market_provider"`
	MarketFeed                          string    `json:"market_feed"`
	MarketQuality                       string    `json:"market_quality"`
	MarketObservedAt                    time.Time `json:"market_observed_at"`
	SimulatedAt                         time.Time `json:"simulated_at"`
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

// AIPaperSpotFill is the owner-facing projection of one immutable simulated
// spot fill. All monetary values remain exact decimal strings and the market
// fields identify the provider-derived reference used by the simulator.
type AIPaperSpotFill struct {
	ID                        string    `json:"id"`
	StrategyInstanceID        string    `json:"strategy_instance_id"`
	ExecutionRecordID         string    `json:"execution_record_id"`
	ProposedActionID          string    `json:"proposed_action_id"`
	RiskEvaluationID          string    `json:"risk_evaluation_id"`
	Symbol                    string    `json:"symbol"`
	Instrument                string    `json:"instrument"`
	Side                      string    `json:"side"`
	Quantity                  string    `json:"quantity"`
	RequestedNotional         string    `json:"requested_notional"`
	ReferencePrice            string    `json:"reference_price"`
	FillPrice                 string    `json:"fill_price"`
	GrossNotional             string    `json:"gross_notional"`
	Fee                       string    `json:"fee"`
	PreviousCash              string    `json:"previous_cash"`
	PreviousPositionQuantity  string    `json:"previous_position_quantity"`
	ResultingCash             string    `json:"resulting_cash"`
	ResultingPositionQuantity string    `json:"resulting_position_quantity"`
	PricingBasis              string    `json:"pricing_basis"`
	MarketProvider            string    `json:"market_provider"`
	MarketFeed                string    `json:"market_feed"`
	MarketQuality             string    `json:"market_quality"`
	MarketObservedAt          time.Time `json:"market_observed_at"`
	SimulatedAt               time.Time `json:"simulated_at"`
	SimulationOnly            bool      `json:"simulation_only"`
}

type AIPaperSpotFillCursor struct {
	SimulatedAt time.Time
	ID          string
}

type AIPaperSpotFillPage struct {
	Fills      []AIPaperSpotFill
	NextCursor *AIPaperSpotFillCursor
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

// ScheduleRun is credential-free, immutable evidence of one completed
// non-live scheduler claim. It records control-plane disposition only and
// cannot represent broker submission or execution.
type ScheduleRun struct {
	ID                           string        `json:"id"`
	StrategyInstanceID           string        `json:"strategy_instance_id"`
	MandateID                    string        `json:"mandate_id"`
	MandateVersion               int           `json:"mandate_version"`
	ExecutionMode                ExecutionMode `json:"execution_mode"`
	StrategyState                State         `json:"strategy_state"`
	ScheduledFor                 time.Time     `json:"scheduled_for"`
	StartedAt                    time.Time     `json:"started_at"`
	CompletedAt                  time.Time     `json:"completed_at"`
	NextRunAt                    time.Time     `json:"next_run_at"`
	Status                       string        `json:"status"`
	ErrorCode                    *string       `json:"error_code,omitempty"`
	AIDecision                   *string       `json:"ai_decision,omitempty"`
	ExecutionStatus              *string       `json:"execution_status,omitempty"`
	DuplicateRecovered           bool          `json:"duplicate_recovered"`
	ReconciliationID             *string       `json:"reconciliation_id,omitempty"`
	ReconciliationReviewRequired bool          `json:"reconciliation_review_required"`
	ConsecutiveFailures          int           `json:"consecutive_failures"`
}

type ScheduleRunCursor struct {
	ScheduledFor time.Time
	ID           string
}

type ScheduleRunPage struct {
	Runs       []ScheduleRun
	NextCursor *ScheduleRunCursor
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
	StartedAt                        time.Time
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
	EvidenceGateStatus           string
	CompletedAt                  time.Time
	NextRunAt                    time.Time
	ReconciliationID             string
	ReconciliationReviewRequired bool
	AIDecision                   string
	ExecutionStatus              ExecutionStatus
	DuplicateRecovered           bool
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
