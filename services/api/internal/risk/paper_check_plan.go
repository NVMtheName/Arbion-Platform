package risk

// DeterministicCheckStage is one ordered stage in the fail-closed AI Paper
// risk plan. CanonicalCode is the PASS code; AcceptedCodes also includes the
// exact fail or warning codes that the same rule may emit.
type DeterministicCheckStage struct {
	CanonicalCode ReasonCode
	AcceptedCodes []ReasonCode
}

// AIAutonomousPaperCheckPlan returns a defensive copy of the exact ordered
// deterministic rule families used for AI_AUTONOMOUS PAPER proposals. A
// denied evaluation is complete when it contains the exact prefix through its
// first FAIL; an allowed evaluation must contain every stage.
func AIAutonomousPaperCheckPlan() []DeterministicCheckStage {
	plan := []DeterministicCheckStage{
		{AuthorizationDenied, []ReasonCode{AuthorizationDenied, AccountOwnershipMismatch, ConnectionUnavailable}},
		{CircuitBreakerActive, []ReasonCode{CircuitBreakerActive}},
		{MandateNotReady, []ReasonCode{MandateNotReady, MandateOwnershipMismatch, MandateVersionMismatch, MandatePaused, MandateExpired}},
		{CapitalPolicyRequired, []ReasonCode{CapitalPolicyRequired}},
		{AutonomyDenied, []ReasonCode{AutonomyDenied, StrategyMismatch}},
		{StaleAccountData, []ReasonCode{StaleAccountData}},
		{SymbolNotAllowed, []ReasonCode{SymbolNotAllowed, UniverseUnsupported}},
		{OptionsNotAllowed, []ReasonCode{OptionsNotAllowed, InvalidAction, OptionsCapabilityUnsupported, PaperOptionsSimulationAttested}},
		{MarginNotAllowed, []ReasonCode{MarginNotAllowed, MarginCapabilityUnsupported}},
		{InsufficientPosition, []ReasonCode{InsufficientPosition}},
		{CapitalLimitExceeded, []ReasonCode{CapitalLimitExceeded, InvalidAction, InsufficientBuyingPower, ReserveViolation}},
		{PositionLimitExceeded, []ReasonCode{PositionLimitExceeded}},
		{DailyLossLimitExceeded, []ReasonCode{DailyLossLimitExceeded, ActivityDataUnavailable, TradeCountLimitExceeded}},
		{RepeatActionCooldownActive, []ReasonCode{RepeatActionCooldownActive, ActivityDataUnavailable}},
	}
	result := make([]DeterministicCheckStage, len(plan))
	for index, stage := range plan {
		result[index] = DeterministicCheckStage{CanonicalCode: stage.CanonicalCode, AcceptedCodes: append([]ReasonCode(nil), stage.AcceptedCodes...)}
	}
	return result
}
