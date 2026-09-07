// Package liveexecution defines Arbion's provider-neutral safety contract for a
// possible future live execution boundary.
//
// This package is deliberately not imported by production routing, schedulers,
// provider clients, or the current PAPER and SHADOW runtimes. It exposes no
// submission method and its only boundary implementation always reports
// BLOCKED_UNIMPLEMENTED, even when supplied evidence is internally coherent.
package liveexecution

import (
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	ContractVersion = "live-execution-safety-v1"

	MaxActionAge         = 5 * time.Minute
	MaxProviderProofAge  = 24 * time.Hour
	MaxRiskAge           = 5 * time.Minute
	MaxReconciliationAge = 5 * time.Minute
	MaxKillSwitchAge     = 5 * time.Minute
)

type Availability string

const (
	Unavailable          Availability = "UNAVAILABLE"
	BlockedUnimplemented Availability = "BLOCKED_UNIMPLEMENTED"
)

type ReasonCode string

const (
	ReasonInvalidEvidence                ReasonCode = "INVALID_EVIDENCE"
	ReasonDuplicateEvidence              ReasonCode = "DUPLICATE_EVIDENCE"
	ReasonFutureEvidence                 ReasonCode = "FUTURE_EVIDENCE"
	ReasonStaleEvidence                  ReasonCode = "STALE_EVIDENCE"
	ReasonBindingMismatch                ReasonCode = "BINDING_MISMATCH"
	ReasonActionDigestMismatch           ReasonCode = "ACTION_DIGEST_MISMATCH"
	ReasonNonLiveMode                    ReasonCode = "NON_LIVE_MODE"
	ReasonProviderCapabilityUnavailable  ReasonCode = "PROVIDER_CAPABILITY_UNAVAILABLE"
	ReasonTransferPermissionPresent      ReasonCode = "TRANSFER_PERMISSION_PRESENT"
	ReasonOwnerAuthorizationUnavailable  ReasonCode = "OWNER_AUTHORIZATION_UNAVAILABLE"
	ReasonDeterministicRiskUnavailable   ReasonCode = "DETERMINISTIC_RISK_UNAVAILABLE"
	ReasonReconciliationUnavailable      ReasonCode = "RECONCILIATION_UNAVAILABLE"
	ReasonKillSwitchUnavailable          ReasonCode = "KILL_SWITCH_UNAVAILABLE"
	ReasonIdempotencyUnavailable         ReasonCode = "IDEMPOTENCY_UNAVAILABLE"
	ReasonLifecycleContractUnavailable   ReasonCode = "LIFECYCLE_CONTRACT_UNAVAILABLE"
	ReasonLiveRuntimeUnimplemented       ReasonCode = "LIVE_RUNTIME_UNIMPLEMENTED"
	ReasonLifecycleEvidenceInvalid       ReasonCode = "LIFECYCLE_EVIDENCE_INVALID"
	ReasonPostTradeReconciliationMissing ReasonCode = "POST_TRADE_RECONCILIATION_MISSING"
)

type Assessment struct {
	Availability Availability `json:"availability"`
	ReasonCodes  []ReasonCode `json:"reason_codes"`
	Structural   bool         `json:"structural_blocker"`
}

type ImmutableEvidenceRef struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Digest     string    `json:"digest_sha256"`
	RecordedAt time.Time `json:"recorded_at"`
}

type Action struct {
	ID                   string    `json:"id"`
	Digest               string    `json:"digest_sha256"`
	UserID               string    `json:"user_id"`
	FinancialAccountID   string    `json:"financial_account_id"`
	ProviderConnectionID string    `json:"provider_connection_id"`
	StrategyInstanceID   string    `json:"strategy_instance_id"`
	MandateID            string    `json:"mandate_id"`
	MandateVersion       int       `json:"mandate_version"`
	CapitalBucketID      string    `json:"capital_bucket_id"`
	CapitalReservationID string    `json:"capital_reservation_id"`
	Symbol               string    `json:"symbol"`
	Side                 string    `json:"side"`
	Quantity             string    `json:"quantity"`
	Notional             string    `json:"notional"`
	Currency             string    `json:"currency"`
	ExecutionMode        string    `json:"execution_mode"`
	CreatedAt            time.Time `json:"created_at"`
}

type ProviderCapabilityProof struct {
	Evidence             ImmutableEvidenceRef `json:"evidence"`
	Provider             string               `json:"provider"`
	ProviderConnectionID string               `json:"provider_connection_id"`
	FinancialAccountID   string               `json:"financial_account_id"`
	CredentialGeneration int64                `json:"credential_generation"`
	PermissionSource     string               `json:"permission_source"`
	TradeAllowed         bool                 `json:"trade_allowed"`
	TransferAllowed      bool                 `json:"transfer_allowed"`
	ObservedAt           time.Time            `json:"observed_at"`
	ExpiresAt            time.Time            `json:"expires_at"`
}

type OwnerAuthorizationProof struct {
	Evidence       ImmutableEvidenceRef `json:"evidence"`
	UserID         string               `json:"user_id"`
	MandateID      string               `json:"mandate_id"`
	MandateVersion int                  `json:"mandate_version"`
	ActionDigest   string               `json:"action_digest_sha256"`
	MFAMethod      string               `json:"mfa_method"`
	MFAVerified    bool                 `json:"mfa_verified"`
	ApprovedAt     time.Time            `json:"approved_at"`
	ExpiresAt      time.Time            `json:"expires_at"`
}

type DeterministicRiskProof struct {
	Evidence                   ImmutableEvidenceRef `json:"evidence"`
	EvaluationID               string               `json:"evaluation_id"`
	UserID                     string               `json:"user_id"`
	FinancialAccountID         string               `json:"financial_account_id"`
	StrategyInstanceID         string               `json:"strategy_instance_id"`
	MandateID                  string               `json:"mandate_id"`
	MandateVersion             int                  `json:"mandate_version"`
	CapitalBucketID            string               `json:"capital_bucket_id"`
	CapitalReservationID       string               `json:"capital_reservation_id"`
	ActionDigest               string               `json:"action_digest_sha256"`
	Decision                   string               `json:"decision"`
	ExecutionMode              string               `json:"execution_mode"`
	PlatformExecutionAvailable bool                 `json:"platform_execution_available"`
	EvaluatedAt                time.Time            `json:"evaluated_at"`
}

type ReconciliationProof struct {
	Evidence             ImmutableEvidenceRef `json:"evidence"`
	Provider             string               `json:"provider"`
	ProviderConnectionID string               `json:"provider_connection_id"`
	FinancialAccountID   string               `json:"financial_account_id"`
	ComparisonStatus     string               `json:"comparison_status"`
	BalancesStatus       string               `json:"balances_status"`
	PositionsStatus      string               `json:"positions_status"`
	BlocksNewActions     bool                 `json:"blocks_new_actions"`
	ObservedAt           time.Time            `json:"observed_at"`
}

type KillSwitchProof struct {
	Evidence               ImmutableEvidenceRef `json:"evidence"`
	UserID                 string               `json:"user_id"`
	FinancialAccountID     string               `json:"financial_account_id"`
	StrategyInstanceID     string               `json:"strategy_instance_id"`
	ProviderConnectionID   string               `json:"provider_connection_id"`
	State                  string               `json:"state"`
	BrokerEnforcementReady bool                 `json:"broker_enforcement_ready"`
	ObservedAt             time.Time            `json:"observed_at"`
}

type IdempotencyProof struct {
	Evidence       ImmutableEvidenceRef `json:"evidence"`
	Key            string               `json:"key"`
	ActionDigest   string               `json:"action_digest_sha256"`
	ReservationID  string               `json:"reservation_id"`
	ReplayDetected bool                 `json:"replay_detected"`
	ReservedAt     time.Time            `json:"reserved_at"`
}

type LifecycleContract struct {
	Version                        string `json:"version"`
	RequireProviderAcknowledgment  bool   `json:"require_provider_acknowledgment"`
	RequirePartialFillEvidence     bool   `json:"require_partial_fill_evidence"`
	RequireCancelReplaceEvidence   bool   `json:"require_cancel_replace_evidence"`
	RequirePostTradeReconciliation bool   `json:"require_post_trade_reconciliation"`
	RequireImmutableEventIDs       bool   `json:"require_immutable_event_ids"`
}

type SafetyCase struct {
	Version                string                  `json:"version"`
	Action                 Action                  `json:"action"`
	ProviderCapability     ProviderCapabilityProof `json:"provider_capability"`
	OwnerAuthorization     OwnerAuthorizationProof `json:"owner_authorization"`
	Risk                   DeterministicRiskProof  `json:"deterministic_risk"`
	PreTradeReconciliation ReconciliationProof     `json:"pre_trade_reconciliation"`
	KillSwitch             KillSwitchProof         `json:"kill_switch"`
	Idempotency            IdempotencyProof        `json:"idempotency"`
	Lifecycle              LifecycleContract       `json:"lifecycle_contract"`
}

// Boundary intentionally has no submit, cancel, replace, refresh, or provider
// method. Its only responsibility is to report whether the evidence contract is
// internally coherent and then preserve the structural implementation block.
type Boundary interface {
	Assess(SafetyCase, time.Time) Assessment
}

type UnavailableBoundary struct{}

func (UnavailableBoundary) Assess(safetyCase SafetyCase, now time.Time) Assessment {
	reasons := validateSafetyCase(safetyCase, now)
	if len(reasons) > 0 {
		return Assessment{Availability: Unavailable, ReasonCodes: reasons, Structural: true}
	}
	return Assessment{
		Availability: BlockedUnimplemented,
		ReasonCodes:  []ReasonCode{ReasonLiveRuntimeUnimplemented},
		Structural:   true,
	}
}

var (
	uuidPattern        = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	digestPattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	idempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
	providerPattern    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	symbolPattern      = regexp.MustCompile(`^[A-Z][A-Z0-9.-]{0,15}$`)
	decimalPattern     = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]{1,10})?$`)
)

func validateSafetyCase(safetyCase SafetyCase, now time.Time) []ReasonCode {
	reasons := map[ReasonCode]struct{}{}
	add := func(reason ReasonCode) { reasons[reason] = struct{}{} }

	if now.IsZero() || safetyCase.Version != ContractVersion || !validAction(safetyCase.Action) {
		add(ReasonInvalidEvidence)
	}
	if !safetyCase.Action.CreatedAt.IsZero() && (safetyCase.Action.CreatedAt.After(now) || now.Sub(safetyCase.Action.CreatedAt) > MaxActionAge) {
		if safetyCase.Action.CreatedAt.After(now) {
			add(ReasonFutureEvidence)
		} else {
			add(ReasonStaleEvidence)
		}
	}
	if safetyCase.Action.ExecutionMode != "LIVE" {
		add(ReasonNonLiveMode)
	}

	refs := []ImmutableEvidenceRef{
		safetyCase.ProviderCapability.Evidence,
		safetyCase.OwnerAuthorization.Evidence,
		safetyCase.Risk.Evidence,
		safetyCase.PreTradeReconciliation.Evidence,
		safetyCase.KillSwitch.Evidence,
		safetyCase.Idempotency.Evidence,
	}
	seenEvidence := map[string]struct{}{}
	for _, ref := range refs {
		if !validEvidenceRef(ref) {
			add(ReasonInvalidEvidence)
		}
		if _, exists := seenEvidence[ref.ID]; exists {
			add(ReasonDuplicateEvidence)
		}
		seenEvidence[ref.ID] = struct{}{}
		if ref.RecordedAt.After(now) {
			add(ReasonFutureEvidence)
		}
	}

	action := safetyCase.Action
	provider := safetyCase.ProviderCapability
	if !providerPattern.MatchString(provider.Provider) || provider.ProviderConnectionID != action.ProviderConnectionID || provider.FinancialAccountID != action.FinancialAccountID || provider.CredentialGeneration < 1 {
		add(ReasonBindingMismatch)
	}
	if provider.PermissionSource != "PROVIDER_VERIFIED" || !provider.TradeAllowed || provider.ObservedAt.IsZero() || provider.ExpiresAt.IsZero() || !provider.ExpiresAt.After(now) {
		add(ReasonProviderCapabilityUnavailable)
	}
	if !validProofRecord(provider.Evidence, "PROVIDER_CAPABILITY", provider.ObservedAt) || !provider.ExpiresAt.After(provider.ObservedAt) {
		add(ReasonProviderCapabilityUnavailable)
	}
	validateAge(provider.ObservedAt, MaxProviderProofAge, now, add)
	if provider.TransferAllowed {
		add(ReasonTransferPermissionPresent)
	}

	authorization := safetyCase.OwnerAuthorization
	if authorization.UserID != action.UserID || authorization.MandateID != action.MandateID || authorization.MandateVersion != action.MandateVersion {
		add(ReasonBindingMismatch)
	}
	if authorization.ActionDigest != action.Digest {
		add(ReasonActionDigestMismatch)
	}
	if !authorization.MFAVerified || strings.TrimSpace(authorization.MFAMethod) == "" || authorization.ApprovedAt.IsZero() || authorization.ApprovedAt.After(now) || !authorization.ExpiresAt.After(now) {
		add(ReasonOwnerAuthorizationUnavailable)
	}
	if !validProofRecord(authorization.Evidence, "OWNER_AUTHORIZATION", authorization.ApprovedAt) || !authorization.ExpiresAt.After(authorization.ApprovedAt) {
		add(ReasonOwnerAuthorizationUnavailable)
	}

	risk := safetyCase.Risk
	if risk.EvaluationID != risk.Evidence.ID || risk.UserID != action.UserID || risk.FinancialAccountID != action.FinancialAccountID || risk.StrategyInstanceID != action.StrategyInstanceID || risk.MandateID != action.MandateID || risk.MandateVersion != action.MandateVersion || risk.CapitalBucketID != action.CapitalBucketID || risk.CapitalReservationID != action.CapitalReservationID {
		add(ReasonBindingMismatch)
	}
	if risk.ActionDigest != action.Digest {
		add(ReasonActionDigestMismatch)
	}
	if risk.Decision != "ALLOW" || risk.ExecutionMode != "LIVE" || !risk.PlatformExecutionAvailable || risk.EvaluatedAt.IsZero() {
		add(ReasonDeterministicRiskUnavailable)
	}
	if !validProofRecord(risk.Evidence, "DETERMINISTIC_RISK", risk.EvaluatedAt) || risk.EvaluatedAt.Before(action.CreatedAt) {
		add(ReasonDeterministicRiskUnavailable)
	}
	validateAge(risk.EvaluatedAt, MaxRiskAge, now, add)

	reconciliation := safetyCase.PreTradeReconciliation
	if reconciliation.Provider != provider.Provider || reconciliation.ProviderConnectionID != action.ProviderConnectionID || reconciliation.FinancialAccountID != action.FinancialAccountID {
		add(ReasonBindingMismatch)
	}
	if reconciliation.ComparisonStatus != "EXACT_MATCH" || reconciliation.BalancesStatus != "EXACT_MATCH" || reconciliation.PositionsStatus != "EXACT_MATCH" || reconciliation.BlocksNewActions || reconciliation.ObservedAt.IsZero() {
		add(ReasonReconciliationUnavailable)
	}
	if !validProofRecord(reconciliation.Evidence, "ACCOUNT_RECONCILIATION", reconciliation.ObservedAt) {
		add(ReasonReconciliationUnavailable)
	}
	validateAge(reconciliation.ObservedAt, MaxReconciliationAge, now, add)

	killSwitch := safetyCase.KillSwitch
	if killSwitch.UserID != action.UserID || killSwitch.FinancialAccountID != action.FinancialAccountID || killSwitch.StrategyInstanceID != action.StrategyInstanceID || killSwitch.ProviderConnectionID != action.ProviderConnectionID {
		add(ReasonBindingMismatch)
	}
	if killSwitch.State != "ARMED" || !killSwitch.BrokerEnforcementReady || killSwitch.ObservedAt.IsZero() {
		add(ReasonKillSwitchUnavailable)
	}
	if !validProofRecord(killSwitch.Evidence, "BROKER_KILL_SWITCH", killSwitch.ObservedAt) {
		add(ReasonKillSwitchUnavailable)
	}
	validateAge(killSwitch.ObservedAt, MaxKillSwitchAge, now, add)

	idempotency := safetyCase.Idempotency
	if !idempotencyPattern.MatchString(idempotency.Key) || !uuidPattern.MatchString(idempotency.ReservationID) || idempotency.ActionDigest != action.Digest || idempotency.ReplayDetected || idempotency.ReservedAt.IsZero() {
		add(ReasonIdempotencyUnavailable)
	}
	if !validProofRecord(idempotency.Evidence, "IDEMPOTENCY_RESERVATION", idempotency.ReservedAt) || idempotency.ReservedAt.Before(action.CreatedAt) {
		add(ReasonIdempotencyUnavailable)
	}
	if idempotency.ActionDigest != action.Digest {
		add(ReasonActionDigestMismatch)
	}
	validateAge(idempotency.ReservedAt, MaxActionAge, now, add)

	lifecycle := safetyCase.Lifecycle
	if lifecycle.Version != ContractVersion || !lifecycle.RequireProviderAcknowledgment || !lifecycle.RequirePartialFillEvidence || !lifecycle.RequireCancelReplaceEvidence || !lifecycle.RequirePostTradeReconciliation || !lifecycle.RequireImmutableEventIDs {
		add(ReasonLifecycleContractUnavailable)
	}

	ordered := make([]ReasonCode, 0, len(reasons))
	for reason := range reasons {
		ordered = append(ordered, reason)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	return ordered
}

func validAction(action Action) bool {
	quantity, quantityOK := exactPositiveDecimal(action.Quantity)
	notional, notionalOK := exactPositiveDecimal(action.Notional)
	return uuidPattern.MatchString(action.ID) && digestPattern.MatchString(action.Digest) &&
		uuidPattern.MatchString(action.UserID) && uuidPattern.MatchString(action.FinancialAccountID) &&
		uuidPattern.MatchString(action.ProviderConnectionID) && uuidPattern.MatchString(action.StrategyInstanceID) &&
		uuidPattern.MatchString(action.MandateID) && action.MandateVersion > 0 &&
		uuidPattern.MatchString(action.CapitalBucketID) && uuidPattern.MatchString(action.CapitalReservationID) &&
		symbolPattern.MatchString(action.Symbol) && (action.Side == "BUY" || action.Side == "SELL") &&
		quantityOK && quantity.Sign() > 0 && notionalOK && notional.Sign() > 0 &&
		action.Currency == "USD" && !action.CreatedAt.IsZero()
}

func validEvidenceRef(ref ImmutableEvidenceRef) bool {
	return uuidPattern.MatchString(ref.ID) && strings.TrimSpace(ref.Kind) != "" && len(ref.Kind) <= 80 && digestPattern.MatchString(ref.Digest) && !ref.RecordedAt.IsZero()
}

func validProofRecord(ref ImmutableEvidenceRef, kind string, observedAt time.Time) bool {
	return ref.Kind == kind && !observedAt.IsZero() && !ref.RecordedAt.Before(observedAt)
}

func validateAge(observedAt time.Time, maximum time.Duration, now time.Time, add func(ReasonCode)) {
	if observedAt.After(now) {
		add(ReasonFutureEvidence)
		return
	}
	if !observedAt.IsZero() && now.Sub(observedAt) > maximum {
		add(ReasonStaleEvidence)
	}
}

func exactPositiveDecimal(value string) (*big.Rat, bool) {
	if !decimalPattern.MatchString(value) {
		return nil, false
	}
	number, ok := new(big.Rat).SetString(value)
	return number, ok
}

type LifecycleState string

const (
	Acknowledged     LifecycleState = "ACKNOWLEDGED"
	PartiallyFilled  LifecycleState = "PARTIALLY_FILLED"
	Filled           LifecycleState = "FILLED"
	CancelRequested  LifecycleState = "CANCEL_REQUESTED"
	Canceled         LifecycleState = "CANCELED"
	ReplaceRequested LifecycleState = "REPLACE_REQUESTED"
	Replaced         LifecycleState = "REPLACED"
	Rejected         LifecycleState = "REJECTED"
)

type LifecycleEvent struct {
	Evidence                 ImmutableEvidenceRef `json:"evidence"`
	State                    LifecycleState       `json:"state"`
	Source                   string               `json:"source"`
	ProviderOrderID          string               `json:"provider_order_id"`
	ReplacesProviderOrderID  string               `json:"replaces_provider_order_id,omitempty"`
	CumulativeFilledQuantity string               `json:"cumulative_filled_quantity"`
	OccurredAt               time.Time            `json:"occurred_at"`
}

type OutcomeEvidence struct {
	ActionDigest            string              `json:"action_digest_sha256"`
	Events                  []LifecycleEvent    `json:"events"`
	PostTradeReconciliation ReconciliationProof `json:"post_trade_reconciliation"`
}

// ValidateOutcome validates saved lifecycle and post-trade evidence only. It
// has no mechanism for creating an order or changing provider state.
func ValidateOutcome(safetyCase SafetyCase, outcome OutcomeEvidence, now time.Time) Assessment {
	reasons := validateOutcome(safetyCase, outcome, now)
	if len(reasons) > 0 {
		return Assessment{Availability: Unavailable, ReasonCodes: reasons, Structural: true}
	}
	return Assessment{Availability: BlockedUnimplemented, ReasonCodes: []ReasonCode{ReasonLiveRuntimeUnimplemented}, Structural: true}
}

func validateOutcome(safetyCase SafetyCase, outcome OutcomeEvidence, now time.Time) []ReasonCode {
	reasons := map[ReasonCode]struct{}{}
	add := func(reason ReasonCode) { reasons[reason] = struct{}{} }
	for _, reason := range validateSafetyCase(safetyCase, now) {
		add(reason)
	}
	if outcome.ActionDigest != safetyCase.Action.Digest || !digestPattern.MatchString(outcome.ActionDigest) {
		add(ReasonActionDigestMismatch)
	}
	requested, requestedOK := exactPositiveDecimal(safetyCase.Action.Quantity)
	if !requestedOK || len(outcome.Events) == 0 || len(outcome.Events) > 100 {
		add(ReasonLifecycleEvidenceInvalid)
	}
	seen := map[string]struct{}{
		safetyCase.ProviderCapability.Evidence.ID:     {},
		safetyCase.OwnerAuthorization.Evidence.ID:     {},
		safetyCase.Risk.Evidence.ID:                   {},
		safetyCase.PreTradeReconciliation.Evidence.ID: {},
		safetyCase.KillSwitch.Evidence.ID:             {},
		safetyCase.Idempotency.Evidence.ID:            {},
	}
	var previous *LifecycleEvent
	var previousFilled *big.Rat
	for index := range outcome.Events {
		event := &outcome.Events[index]
		filled, filledOK := exactNonnegativeDecimal(event.CumulativeFilledQuantity)
		if !validEvidenceRef(event.Evidence) || event.Evidence.Kind != "PROVIDER_ORDER_LIFECYCLE" || !filledOK || strings.TrimSpace(event.ProviderOrderID) == "" || len(event.ProviderOrderID) > 200 || event.OccurredAt.IsZero() || event.OccurredAt.After(now) || event.Evidence.RecordedAt.Before(event.OccurredAt) || event.Evidence.RecordedAt.After(now) {
			add(ReasonLifecycleEvidenceInvalid)
			continue
		}
		if _, exists := seen[event.Evidence.ID]; exists {
			add(ReasonDuplicateEvidence)
		}
		seen[event.Evidence.ID] = struct{}{}
		if filled.Cmp(requested) > 0 || (previousFilled != nil && filled.Cmp(previousFilled) < 0) {
			add(ReasonLifecycleEvidenceInvalid)
		}
		if !validLifecycleSource(event.State, event.Source) || !validLifecycleQuantity(event.State, filled, requested) {
			add(ReasonLifecycleEvidenceInvalid)
		}
		if previous == nil {
			if event.State != Acknowledged && event.State != Rejected {
				add(ReasonLifecycleEvidenceInvalid)
			}
		} else {
			if !event.OccurredAt.After(previous.OccurredAt) || !validTransition(previous.State, event.State) {
				add(ReasonLifecycleEvidenceInvalid)
			}
			if event.State != Replaced && event.ProviderOrderID != previous.ProviderOrderID {
				add(ReasonLifecycleEvidenceInvalid)
			}
			if event.State == Replaced && (event.ReplacesProviderOrderID == "" || event.ReplacesProviderOrderID != previous.ProviderOrderID || event.ProviderOrderID == previous.ProviderOrderID) {
				add(ReasonLifecycleEvidenceInvalid)
			}
		}
		previous = event
		previousFilled = filled
	}
	if previous == nil || !terminalLifecycleState(previous.State) {
		add(ReasonLifecycleEvidenceInvalid)
	}
	postTrade := outcome.PostTradeReconciliation
	if _, exists := seen[postTrade.Evidence.ID]; exists {
		add(ReasonDuplicateEvidence)
	}
	if !validEvidenceRef(postTrade.Evidence) || !validProofRecord(postTrade.Evidence, "ACCOUNT_RECONCILIATION", postTrade.ObservedAt) || postTrade.Provider != safetyCase.ProviderCapability.Provider || postTrade.ProviderConnectionID != safetyCase.Action.ProviderConnectionID || postTrade.FinancialAccountID != safetyCase.Action.FinancialAccountID || postTrade.ComparisonStatus != "EXACT_MATCH" || postTrade.BalancesStatus != "EXACT_MATCH" || postTrade.PositionsStatus != "EXACT_MATCH" || postTrade.BlocksNewActions || postTrade.ObservedAt.IsZero() || postTrade.ObservedAt.After(now) || (previous != nil && postTrade.ObservedAt.Before(previous.OccurredAt)) {
		add(ReasonPostTradeReconciliationMissing)
	}
	validateAge(postTrade.ObservedAt, MaxReconciliationAge, now, add)
	ordered := make([]ReasonCode, 0, len(reasons))
	for reason := range reasons {
		ordered = append(ordered, reason)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	return ordered
}

func exactNonnegativeDecimal(value string) (*big.Rat, bool) {
	if !decimalPattern.MatchString(value) {
		return nil, false
	}
	number, ok := new(big.Rat).SetString(value)
	return number, ok && number.Sign() >= 0
}

func validLifecycleSource(state LifecycleState, source string) bool {
	switch state {
	case CancelRequested, ReplaceRequested:
		return source == "PLATFORM_COMMAND"
	case Acknowledged, PartiallyFilled, Filled, Canceled, Replaced, Rejected:
		return source == "PROVIDER_VERIFIED"
	default:
		return false
	}
}

func validLifecycleQuantity(state LifecycleState, filled, requested *big.Rat) bool {
	switch state {
	case Acknowledged, Rejected:
		return filled.Sign() == 0
	case PartiallyFilled:
		return filled.Sign() > 0 && filled.Cmp(requested) < 0
	case Filled:
		return filled.Cmp(requested) == 0
	case CancelRequested, Canceled, ReplaceRequested, Replaced:
		return filled.Cmp(requested) <= 0
	default:
		return false
	}
}

func validTransition(previous, next LifecycleState) bool {
	switch previous {
	case Acknowledged:
		return next == PartiallyFilled || next == Filled || next == CancelRequested || next == ReplaceRequested || next == Rejected
	case PartiallyFilled:
		return next == PartiallyFilled || next == Filled || next == CancelRequested || next == ReplaceRequested
	case CancelRequested:
		return next == PartiallyFilled || next == Filled || next == Canceled
	case ReplaceRequested:
		return next == PartiallyFilled || next == Filled || next == Canceled || next == Replaced
	case Replaced:
		return next == Acknowledged || next == PartiallyFilled || next == Filled || next == CancelRequested || next == ReplaceRequested || next == Rejected
	default:
		return false
	}
}

func terminalLifecycleState(state LifecycleState) bool {
	return state == Filled || state == Canceled || state == Rejected
}
