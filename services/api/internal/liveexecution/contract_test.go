package liveexecution

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	actionID      = "11111111-1111-4111-8111-111111111111"
	userID        = "22222222-2222-4222-8222-222222222222"
	accountID     = "33333333-3333-4333-8333-333333333333"
	connectionID  = "44444444-4444-4444-8444-444444444444"
	strategyID    = "55555555-5555-4555-8555-555555555555"
	mandateID     = "66666666-6666-4666-8666-666666666666"
	bucketID      = "77777777-7777-4777-8777-777777777777"
	reservationID = "88888888-8888-4888-8888-888888888888"
	digestA       = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	digestB       = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestUnavailableBoundaryPreservesStructuralBlockForCoherentEvidence(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	assessment := (UnavailableBoundary{}).Assess(validSafetyCase(now), now)
	if assessment.Availability != BlockedUnimplemented || !assessment.Structural {
		t.Fatalf("coherent evidence did not preserve structural block: %+v", assessment)
	}
	assertReasons(t, assessment.ReasonCodes, ReasonLiveRuntimeUnimplemented)
}

func TestSafetyCaseFailsClosedForMalformedDuplicateStaleAndInconsistentEvidence(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		mutate  func(*SafetyCase)
		reasons []ReasonCode
	}{
		{
			name: "malformed exact decimal",
			mutate: func(safetyCase *SafetyCase) {
				safetyCase.Action.Notional = "1e2"
			},
			reasons: []ReasonCode{ReasonInvalidEvidence},
		},
		{
			name: "duplicate immutable evidence identity",
			mutate: func(safetyCase *SafetyCase) {
				safetyCase.Risk.Evidence.ID = safetyCase.ProviderCapability.Evidence.ID
			},
			reasons: []ReasonCode{ReasonDuplicateEvidence, ReasonBindingMismatch},
		},
		{
			name: "stale provider and reconciliation snapshots",
			mutate: func(safetyCase *SafetyCase) {
				safetyCase.ProviderCapability.ObservedAt = now.Add(-MaxProviderProofAge - time.Second)
				safetyCase.PreTradeReconciliation.ObservedAt = now.Add(-MaxReconciliationAge - time.Second)
			},
			reasons: []ReasonCode{ReasonStaleEvidence},
		},
		{
			name: "future evidence",
			mutate: func(safetyCase *SafetyCase) {
				safetyCase.KillSwitch.ObservedAt = now.Add(time.Second)
			},
			reasons: []ReasonCode{ReasonFutureEvidence},
		},
		{
			name: "cross-account binding",
			mutate: func(safetyCase *SafetyCase) {
				safetyCase.Risk.FinancialAccountID = "99999999-9999-4999-8999-999999999999"
			},
			reasons: []ReasonCode{ReasonBindingMismatch},
		},
		{
			name: "action digest mismatch",
			mutate: func(safetyCase *SafetyCase) {
				safetyCase.OwnerAuthorization.ActionDigest = digestB
			},
			reasons: []ReasonCode{ReasonActionDigestMismatch},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			safetyCase := validSafetyCase(now)
			test.mutate(&safetyCase)
			assessment := (UnavailableBoundary{}).Assess(safetyCase, now)
			if assessment.Availability != Unavailable || !assessment.Structural {
				t.Fatalf("invalid evidence was not unavailable: %+v", assessment)
			}
			assertContainsReasons(t, assessment.ReasonCodes, test.reasons...)
		})
	}
}

func TestSafetyCaseRequiresAuthoritativeProviderScopeAndEverySafetyControl(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*SafetyCase)
		reason ReasonCode
	}{
		{
			name: "self asserted provider scope",
			mutate: func(safetyCase *SafetyCase) {
				safetyCase.ProviderCapability.PermissionSource = "OWNER_ASSERTED"
			},
			reason: ReasonProviderCapabilityUnavailable,
		},
		{
			name: "transfer permission present",
			mutate: func(safetyCase *SafetyCase) {
				safetyCase.ProviderCapability.TransferAllowed = true
			},
			reason: ReasonTransferPermissionPresent,
		},
		{
			name: "owner mfa missing",
			mutate: func(safetyCase *SafetyCase) {
				safetyCase.OwnerAuthorization.MFAVerified = false
			},
			reason: ReasonOwnerAuthorizationUnavailable,
		},
		{
			name: "risk cannot execute",
			mutate: func(safetyCase *SafetyCase) {
				safetyCase.Risk.PlatformExecutionAvailable = false
			},
			reason: ReasonDeterministicRiskUnavailable,
		},
		{
			name: "reconciliation blocks",
			mutate: func(safetyCase *SafetyCase) {
				safetyCase.PreTradeReconciliation.BlocksNewActions = true
			},
			reason: ReasonReconciliationUnavailable,
		},
		{
			name: "broker kill switch unproven",
			mutate: func(safetyCase *SafetyCase) {
				safetyCase.KillSwitch.BrokerEnforcementReady = false
			},
			reason: ReasonKillSwitchUnavailable,
		},
		{
			name: "idempotency replay",
			mutate: func(safetyCase *SafetyCase) {
				safetyCase.Idempotency.ReplayDetected = true
			},
			reason: ReasonIdempotencyUnavailable,
		},
		{
			name: "incomplete lifecycle contract",
			mutate: func(safetyCase *SafetyCase) {
				safetyCase.Lifecycle.RequirePostTradeReconciliation = false
			},
			reason: ReasonLifecycleContractUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			safetyCase := validSafetyCase(now)
			test.mutate(&safetyCase)
			assessment := (UnavailableBoundary{}).Assess(safetyCase, now)
			if assessment.Availability != Unavailable {
				t.Fatalf("unproven control did not fail closed: %+v", assessment)
			}
			assertContainsReasons(t, assessment.ReasonCodes, test.reason)
		})
	}
}

func TestCurrentPaperAndShadowModesCannotCrossBoundary(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	for _, mode := range []string{"PAPER", "SHADOW"} {
		t.Run(mode, func(t *testing.T) {
			safetyCase := validSafetyCase(now)
			safetyCase.Action.ExecutionMode = mode
			safetyCase.Risk.ExecutionMode = mode
			safetyCase.Risk.PlatformExecutionAvailable = false
			assessment := (UnavailableBoundary{}).Assess(safetyCase, now)
			if assessment.Availability != Unavailable {
				t.Fatalf("%s crossed the live boundary: %+v", mode, assessment)
			}
			assertContainsReasons(t, assessment.ReasonCodes, ReasonNonLiveMode, ReasonDeterministicRiskUnavailable)
		})
	}
}

func TestLifecycleEvidenceSupportsPartialFillAndExactPostTradeReconciliation(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	safetyCase := validSafetyCase(now)
	outcome := OutcomeEvidence{
		ActionDigest: digestA,
		Events: []LifecycleEvent{
			lifecycleEvent("10000000-0000-4000-8000-000000000001", Acknowledged, "PROVIDER_VERIFIED", "provider-order-1", "", "0", now.Add(-4*time.Minute)),
			lifecycleEvent("10000000-0000-4000-8000-000000000002", PartiallyFilled, "PROVIDER_VERIFIED", "provider-order-1", "", "0.4000000000", now.Add(-3*time.Minute)),
			lifecycleEvent("10000000-0000-4000-8000-000000000003", Filled, "PROVIDER_VERIFIED", "provider-order-1", "", "1.0000000000", now.Add(-2*time.Minute)),
		},
		PostTradeReconciliation: reconciliationProof("10000000-0000-4000-8000-000000000004", now.Add(-time.Minute)),
	}
	assessment := ValidateOutcome(safetyCase, outcome, now)
	if assessment.Availability != BlockedUnimplemented {
		t.Fatalf("coherent saved lifecycle evidence was not recognized: %+v", assessment)
	}
	assertReasons(t, assessment.ReasonCodes, ReasonLiveRuntimeUnimplemented)
}

func TestLifecycleEvidenceSupportsExplicitCancelAndReplaceSemantics(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	safetyCase := validSafetyCase(now)
	outcome := OutcomeEvidence{
		ActionDigest: digestA,
		Events: []LifecycleEvent{
			lifecycleEvent("20000000-0000-4000-8000-000000000001", Acknowledged, "PROVIDER_VERIFIED", "provider-order-1", "", "0", now.Add(-4*time.Minute)),
			lifecycleEvent("20000000-0000-4000-8000-000000000002", ReplaceRequested, "PLATFORM_COMMAND", "provider-order-1", "", "0", now.Add(-210*time.Second)),
			lifecycleEvent("20000000-0000-4000-8000-000000000003", Replaced, "PROVIDER_VERIFIED", "provider-order-2", "provider-order-1", "0", now.Add(-3*time.Minute)),
			lifecycleEvent("20000000-0000-4000-8000-000000000004", Acknowledged, "PROVIDER_VERIFIED", "provider-order-2", "", "0", now.Add(-150*time.Second)),
			lifecycleEvent("20000000-0000-4000-8000-000000000005", CancelRequested, "PLATFORM_COMMAND", "provider-order-2", "", "0", now.Add(-2*time.Minute)),
			lifecycleEvent("20000000-0000-4000-8000-000000000006", Canceled, "PROVIDER_VERIFIED", "provider-order-2", "", "0", now.Add(-90*time.Second)),
		},
		PostTradeReconciliation: reconciliationProof("20000000-0000-4000-8000-000000000007", now.Add(-time.Minute)),
	}
	assessment := ValidateOutcome(safetyCase, outcome, now)
	if assessment.Availability != BlockedUnimplemented {
		t.Fatalf("coherent replace/cancel evidence was not recognized: %+v", assessment)
	}
}

func TestLifecycleEvidenceFailsClosedOnDuplicateRegressionAndMissingReconciliation(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	safetyCase := validSafetyCase(now)
	eventID := "30000000-0000-4000-8000-000000000001"
	outcome := OutcomeEvidence{
		ActionDigest: digestA,
		Events: []LifecycleEvent{
			lifecycleEvent(eventID, Acknowledged, "PROVIDER_VERIFIED", "provider-order-1", "", "0", now.Add(-4*time.Minute)),
			lifecycleEvent(eventID, PartiallyFilled, "PROVIDER_VERIFIED", "provider-order-1", "", "0.8000000000", now.Add(-3*time.Minute)),
			lifecycleEvent("30000000-0000-4000-8000-000000000003", PartiallyFilled, "PROVIDER_VERIFIED", "provider-order-1", "", "0.7000000000", now.Add(-2*time.Minute)),
		},
	}
	assessment := ValidateOutcome(safetyCase, outcome, now)
	if assessment.Availability != Unavailable {
		t.Fatalf("invalid lifecycle did not fail closed: %+v", assessment)
	}
	assertContainsReasons(t, assessment.ReasonCodes, ReasonDuplicateEvidence, ReasonLifecycleEvidenceInvalid, ReasonPostTradeReconciliationMissing)
}

func TestProductionPackagesDoNotImportLiveExecutionContract(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller path unavailable")
	}
	packageDir := filepath.Dir(filename)
	internalDir := filepath.Dir(packageDir)
	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || filepath.Ext(path) != ".go" || strings.HasPrefix(path, packageDir+string(os.PathSeparator)) {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(contents), "/internal/liveexecution") {
			t.Errorf("production package imports the non-executable live contract: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan production packages: %v", err)
	}
}

func validSafetyCase(now time.Time) SafetyCase {
	recordedAt := now.Add(-30 * time.Second)
	return SafetyCase{
		Version: ContractVersion,
		Action: Action{
			ID: actionID, Digest: digestA, UserID: userID, FinancialAccountID: accountID,
			ProviderConnectionID: connectionID, StrategyInstanceID: strategyID, MandateID: mandateID,
			MandateVersion: 4, CapitalBucketID: bucketID, CapitalReservationID: reservationID,
			Symbol: "BTC", Side: "BUY", Quantity: "1.0000000000", Notional: "100.0000000000",
			Currency: "USD", ExecutionMode: "LIVE", CreatedAt: now.Add(-time.Minute),
		},
		ProviderCapability: ProviderCapabilityProof{
			Evidence: evidenceRef("90000000-0000-4000-8000-000000000001", "PROVIDER_CAPABILITY", recordedAt),
			Provider: "coinbase", ProviderConnectionID: connectionID, FinancialAccountID: accountID,
			CredentialGeneration: 2, PermissionSource: "PROVIDER_VERIFIED", TradeAllowed: true,
			TransferAllowed: false, ObservedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour),
		},
		OwnerAuthorization: OwnerAuthorizationProof{
			Evidence: evidenceRef("90000000-0000-4000-8000-000000000002", "OWNER_AUTHORIZATION", recordedAt),
			UserID:   userID, MandateID: mandateID, MandateVersion: 4, ActionDigest: digestA,
			MFAMethod: "TOTP", MFAVerified: true, ApprovedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
		},
		Risk: DeterministicRiskProof{
			Evidence:     evidenceRef("90000000-0000-4000-8000-000000000003", "DETERMINISTIC_RISK", recordedAt),
			EvaluationID: "90000000-0000-4000-8000-000000000003", UserID: userID,
			FinancialAccountID: accountID, StrategyInstanceID: strategyID, MandateID: mandateID,
			MandateVersion: 4, CapitalBucketID: bucketID, CapitalReservationID: reservationID,
			ActionDigest: digestA, Decision: "ALLOW", ExecutionMode: "LIVE", PlatformExecutionAvailable: true,
			EvaluatedAt: now.Add(-time.Minute),
		},
		PreTradeReconciliation: reconciliationProof("90000000-0000-4000-8000-000000000004", now.Add(-time.Minute)),
		KillSwitch: KillSwitchProof{
			Evidence: evidenceRef("90000000-0000-4000-8000-000000000005", "BROKER_KILL_SWITCH", recordedAt),
			UserID:   userID, FinancialAccountID: accountID, StrategyInstanceID: strategyID,
			ProviderConnectionID: connectionID, State: "ARMED", BrokerEnforcementReady: true,
			ObservedAt: now.Add(-time.Minute),
		},
		Idempotency: IdempotencyProof{
			Evidence: evidenceRef("90000000-0000-4000-8000-000000000006", "IDEMPOTENCY_RESERVATION", recordedAt),
			Key:      "live-contract-test-0001", ActionDigest: digestA, ReservationID: reservationID,
			ReplayDetected: false, ReservedAt: now.Add(-time.Minute),
		},
		Lifecycle: LifecycleContract{
			Evidence: evidenceRef("90000000-0000-4000-8000-000000000007", "LIVE_LIFECYCLE_CONTRACT", recordedAt),
			Version:  ContractVersion, RequireProviderAcknowledgment: true, RequirePartialFillEvidence: true,
			RequireCancelReplaceEvidence: true, RequirePostTradeReconciliation: true, RequireImmutableEventIDs: true,
		},
	}
}

func evidenceRef(id, kind string, recordedAt time.Time) ImmutableEvidenceRef {
	return ImmutableEvidenceRef{ID: id, Kind: kind, Digest: digestB, RecordedAt: recordedAt}
}

func reconciliationProof(id string, observedAt time.Time) ReconciliationProof {
	return ReconciliationProof{
		Evidence: evidenceRef(id, "ACCOUNT_RECONCILIATION", observedAt.Add(10*time.Second)),
		Provider: "coinbase", ProviderConnectionID: connectionID, FinancialAccountID: accountID,
		ComparisonStatus: "EXACT_MATCH", BalancesStatus: "EXACT_MATCH", PositionsStatus: "EXACT_MATCH",
		BlocksNewActions: false, ObservedAt: observedAt,
	}
}

func lifecycleEvent(id string, state LifecycleState, source, providerOrderID, replaces, filled string, occurredAt time.Time) LifecycleEvent {
	return LifecycleEvent{
		Evidence: evidenceRef(id, "PROVIDER_ORDER_LIFECYCLE", occurredAt.Add(time.Second)),
		State:    state, Source: source, ProviderOrderID: providerOrderID,
		ReplacesProviderOrderID: replaces, CumulativeFilledQuantity: filled, OccurredAt: occurredAt,
	}
}

func assertReasons(t *testing.T, actual []ReasonCode, expected ...ReasonCode) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("reason count = %d (%v), want %d (%v)", len(actual), actual, len(expected), expected)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("reason[%d] = %s, want %s (all: %v)", index, actual[index], expected[index], actual)
		}
	}
}

func assertContainsReasons(t *testing.T, actual []ReasonCode, expected ...ReasonCode) {
	t.Helper()
	for _, expectedReason := range expected {
		found := false
		for _, actualReason := range actual {
			if actualReason == expectedReason {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("reasons %v do not contain %s", actual, expectedReason)
		}
	}
}
