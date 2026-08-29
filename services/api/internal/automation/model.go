package automation

import (
	"encoding/json"
	"time"
)

const ExecutionCapable = false

type CapitalBucket struct {
	ID, UserID, FinancialAccountID, Name      string
	AllocationType, AllocationValue, Currency string
	IsReserve                                 bool
	ProtectedAmount                           string
	AllocationLimit                           *string
	Status                                    string
	CreatedAt, UpdatedAt                      time.Time
}

type RiskPolicy struct {
	MaxCapitalDeployed          *string `json:"max_capital_deployed,omitempty"`
	MaxSinglePositionAmount     *string `json:"max_single_position_amount,omitempty"`
	MaxSinglePositionPercentage *string `json:"max_single_position_percentage,omitempty"`
	MaxDailyLoss                *string `json:"max_daily_loss,omitempty"`
	MaxTradesPerDay             *int    `json:"max_trades_per_day,omitempty"`
	MinimumCashReserve          *string `json:"minimum_cash_reserve,omitempty"`
}

type StrategyParameters struct {
	Symbols                  []string `json:"symbols"`
	MinimumDTE               int      `json:"minimum_dte"`
	MaximumDTE               int      `json:"maximum_dte"`
	TargetDelta              string   `json:"target_delta"`
	TargetDeltaMin           string   `json:"target_delta_min"`
	TargetDeltaMax           string   `json:"target_delta_max"`
	MinimumPremium           *string  `json:"minimum_premium,omitempty"`
	MaximumContracts         int      `json:"maximum_contracts"`
	AssignmentHandlingPolicy string   `json:"assignment_handling_policy"`
}

// AIShadowParameters is the deliberately small, immutable decision envelope for
// non-live autonomous AI evaluation. The model may choose one symbol and
// BUY/SELL, but it can never exceed MaxProposalNotional and the execution mode
// remains PAPER or SHADOW.
type AIShadowParameters struct {
	Objective           string `json:"objective"`
	MaxProposalNotional string `json:"max_proposal_notional"`
}

// AIShadowScenarioDraftCommand can only revise the two bounded research
// limits exposed by the non-executing simulation workbench. The service
// preserves every other mandate field and always creates an immutable DRAFT.
type AIShadowScenarioDraftCommand struct {
	ExpectedVersion     int    `json:"expected_version"`
	MaxProposalNotional string `json:"max_proposal_notional"`
	MaxTradesPerDay     int    `json:"max_trades_per_day"`
	Confirm             bool   `json:"confirm"`
}

type ScheduleNotifications struct {
	EvaluationCompleted        bool `json:"evaluation_completed,omitempty"`
	LifecycleRequired          bool `json:"lifecycle_required,omitempty"`
	FirstFailure               bool `json:"first_failure,omitempty"`
	ReconciliationReviewNeeded bool `json:"reconciliation_review_required,omitempty"`
}

// ScheduleConditions is intentionally narrow. A schedule only requests a
// non-live evaluation cadence and optional informational email; it never grants
// authority beyond the immutable mandate version that contains it.
type ScheduleConditions struct {
	Enabled         bool                  `json:"enabled"`
	IntervalMinutes int                   `json:"interval_minutes,omitempty"`
	Session         string                `json:"session,omitempty"`
	Notifications   ScheduleNotifications `json:"notifications,omitempty"`
}

type Universe struct {
	Symbols     []string `json:"symbols"`
	UniverseIDs []string `json:"universe_ids,omitempty"`
}

type Mandate struct {
	ID, UserID, FinancialAccountID, AutomationType        string
	StrategyIdentifier                                    *string
	AIProviderConnectionID, AIModelID                     *string
	CapitalBucketID, AutonomyLevel, ExecutionMode, Status string
	CurrentVersion                                        int
	StrategyParameters                                    json.RawMessage
	Risk                                                  RiskPolicy
	AllowedUniverse, ProhibitedUniverse                   Universe
	MarginAllowed, OptionsAllowed, CapabilityUnverified   bool
	PaperOptionsSimulationAttested                        bool
	ScheduleConditions                                    json.RawMessage
	EffectiveFrom                                         time.Time
	EffectiveUntil                                        *time.Time
	CreatedAt, UpdatedAt                                  time.Time
	ExecutionCapable                                      bool
}

type Version struct {
	ID, MandateID           string
	VersionNumber           int
	CreatedAt               time.Time
	CreatedByUserID, Source string
	Snapshot                json.RawMessage
	ChangeSummary           json.RawMessage
}

type Strategy struct {
	ID, DisplayName, Description                            string
	OptionsRequired, MarginRelevant, ExistingSharesRelevant bool
	ParameterSchemaID                                       string
	DefinitionVersion                                       int
	InitialState                                            string
	Implemented                                             bool
}

var Strategies = map[string]Strategy{
	"wheel":            {"wheel", "Wheel", "Deterministic cash-secured-put and covered-call cycle.", true, true, false, "wheel.v1", 1, "READY_FOR_PUT", true},
	"covered_call":     {"covered_call", "Covered Call", "Deterministic covered-call state machine.", true, false, true, "covered_call.v1", 1, "READY_FOR_CALL", true},
	"cash_secured_put": {"cash_secured_put", "Cash-Secured Put", "Deterministic cash-secured-put state machine.", true, false, false, "cash_secured_put.v1", 1, "READY_FOR_PUT", true},
	"collar":           {"collar", "Collar", "Configure a future protective Collar strategy.", true, false, true, "collar.v1", 1, "", false},
}

type CreateBucketCommand struct {
	FinancialAccountID string  `json:"financial_account_id"`
	Name               string  `json:"name"`
	AllocationType     string  `json:"allocation_type"`
	AllocationValue    string  `json:"allocation_value"`
	Currency           string  `json:"currency"`
	ProtectedAmount    string  `json:"protected_amount"`
	AllocationLimit    *string `json:"allocation_limit,omitempty"`
	IsReserve          bool    `json:"is_reserve"`
}
type MandateCommand struct {
	FinancialAccountID             string          `json:"financial_account_id"`
	AutomationType                 string          `json:"automation_type"`
	CapitalBucketID                string          `json:"capital_bucket_id"`
	AutonomyLevel                  string          `json:"autonomy_level"`
	ExecutionMode                  string          `json:"execution_mode"`
	StrategyIdentifier             *string         `json:"strategy_identifier,omitempty"`
	AIProviderConnectionID         *string         `json:"ai_provider_connection_id,omitempty"`
	AIModelID                      *string         `json:"ai_model_id,omitempty"`
	StrategyParameters             json.RawMessage `json:"strategy_parameters"`
	Risk                           RiskPolicy      `json:"risk_parameters"`
	AllowedUniverse                Universe        `json:"allowed_universe"`
	ProhibitedUniverse             Universe        `json:"prohibited_universe"`
	MarginAllowed                  bool            `json:"margin_allowed"`
	OptionsAllowed                 bool            `json:"options_allowed"`
	PaperOptionsSimulationAttested bool            `json:"paper_options_simulation_attested"`
	ScheduleConditions             json.RawMessage `json:"schedule_conditions"`
	EffectiveFrom                  *time.Time      `json:"effective_from,omitempty"`
	EffectiveUntil                 *time.Time      `json:"effective_until,omitempty"`
}

type ScheduleCommand struct {
	ExpectedVersion    int                `json:"expected_version"`
	ScheduleConditions ScheduleConditions `json:"schedule_conditions"`
}

type AutonomyCommand struct {
	ExpectedVersion int    `json:"expected_version"`
	AutonomyLevel   string `json:"autonomy_level"`
}

type PaperOptionsSimulationAttestationCommand struct {
	ExpectedVersion int  `json:"expected_version"`
	Attested        bool `json:"attested"`
}
