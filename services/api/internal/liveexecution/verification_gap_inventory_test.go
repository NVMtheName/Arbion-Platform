package liveexecution

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSafetyEvidenceGapInventoryIsDeterministicCanonicalAndFixed(t *testing.T) {
	now := time.Date(2026, 9, 8, 17, 0, 0, 123, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, audit, review := verifiedComparisonReviewChain(t, input, input)
	first := CompileSafetyEvidenceGapInventory(input, input, envelope, audit, review)
	second := CompileSafetyEvidenceGapInventory(input, input, envelope, audit, review)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("gap inventory replay changed: %#v %#v", first, second)
	}
	if first.InventoryVersion != SafetyEvidenceGapInventoryVersion ||
		first.Status != SafetyEvidenceGapInventoryCompiled ||
		first.Failure != SafetyEvidenceGapInventoryFailureNone || first.ExecutionAuthority ||
		first.ReviewDigest != review.ReviewDigest || first.ComparisonStatus != VerificationComparisonSame ||
		first.ReviewDisposition != VerificationComparisonReviewNoChange ||
		first.OwnerAction != VerificationComparisonOwnerActionNone ||
		first.CurrentGapCount != 0 || first.NonUnchangedCategoryCount != 0 ||
		len(first.Categories) != len(verificationEvidenceExpectations()) ||
		!digestPattern.MatchString(first.InventoryDigest) {
		t.Fatalf("gap inventory is incomplete or authoritative: %#v", first)
	}
	for index, expected := range verificationEvidenceExpectations() {
		gap := first.Categories[index]
		if gap.Category != expected.category || gap.Source != expected.source ||
			gap.State != SafetyEvidenceGapUnchanged || gap.BaselineUnavailable || gap.CurrentUnavailable ||
			len(gap.BaselineReasonCodes) != 0 || len(gap.CurrentReasonCodes) != 0 ||
			!reflect.DeepEqual(gap.Baseline, gap.Current) {
			t.Fatalf("category %d is not canonical and unchanged: %#v", index, gap)
		}
	}
	if first.InventoryDigest != "0e77b5bff44aadacec3ebdb2235671fc0e6d21f8087fa37185804b71fb5d09e7" {
		t.Fatalf("fixed gap inventory changed: %s", first.InventoryDigest)
	}
	if err := VerifySafetyEvidenceGapInventory(input, input, envelope, audit, review, first); err != nil {
		t.Fatalf("exact gap inventory did not verify: %v", err)
	}
}

func TestSafetyEvidenceGapInventoryClassifiesExactUnavailableReasonTransitions(t *testing.T) {
	baselineTime := time.Date(2026, 9, 8, 17, 0, 0, 0, time.UTC)
	currentTime := baselineTime.Add(time.Minute)
	availableBaseline := verifiedSafetyCaseReport(t, validSafetyCase(baselineTime), baselineTime)
	unavailableCurrentCase := validSafetyCase(currentTime)
	unavailableCurrentCase.ProviderCapability.TradeAllowed = false
	unavailableCurrent := verifiedSafetyCaseReport(t, unavailableCurrentCase, currentTime)
	envelope, audit, review := verifiedComparisonReviewChain(t, availableBaseline, unavailableCurrent)
	inventory := CompileSafetyEvidenceGapInventory(availableBaseline, unavailableCurrent, envelope, audit, review)
	if inventory.Status != SafetyEvidenceGapInventoryCompiled || inventory.CurrentGapCount != 1 {
		t.Fatalf("provider gap did not compile exactly: %#v", inventory)
	}
	providerGap := inventory.Categories[0]
	if providerGap.Category != "PROVIDER_CAPABILITY" || providerGap.State != SafetyEvidenceGapNewlyUnavailable ||
		providerGap.BaselineUnavailable || !providerGap.CurrentUnavailable ||
		!reflect.DeepEqual(providerGap.CurrentReasonCodes, []ReasonCode{ReasonProviderCapabilityUnavailable}) {
		t.Fatalf("new provider gap was not classified exactly: %#v", providerGap)
	}
	if inventory.Assessment.After.Availability != Unavailable ||
		!containsReason(inventory.Assessment.After.ReasonCodes, ReasonProviderCapabilityUnavailable) {
		t.Fatalf("exact current assessment was not preserved: %#v", inventory.Assessment)
	}

	unavailableBaselineCase := validSafetyCase(baselineTime)
	unavailableBaselineCase.ProviderCapability.TradeAllowed = false
	unavailableBaseline := verifiedSafetyCaseReport(t, unavailableBaselineCase, baselineTime)
	envelope, audit, review = verifiedComparisonReviewChain(t, unavailableBaseline, unavailableCurrent)
	still := CompileSafetyEvidenceGapInventory(unavailableBaseline, unavailableCurrent, envelope, audit, review)
	if still.Categories[0].State != SafetyEvidenceGapStillUnavailable || still.CurrentGapCount != 1 {
		t.Fatalf("persistent provider gap was not classified exactly: %#v", still.Categories[0])
	}

	availableCurrent := verifiedSafetyCaseReport(t, validSafetyCase(currentTime), currentTime)
	envelope, audit, review = verifiedComparisonReviewChain(t, unavailableBaseline, availableCurrent)
	resolved := CompileSafetyEvidenceGapInventory(unavailableBaseline, availableCurrent, envelope, audit, review)
	if resolved.Categories[0].State != SafetyEvidenceGapResolved || resolved.CurrentGapCount != 0 {
		t.Fatalf("resolved provider gap was not classified exactly: %#v", resolved.Categories[0])
	}
}

func TestSafetyEvidenceGapClassifierUsesOnlyExactGapAndEvidenceFacts(t *testing.T) {
	tests := []struct {
		baseline bool
		current  bool
		evidence VerificationChangeState
		expected SafetyEvidenceGapState
	}{
		{false, false, VerificationChangeSame, SafetyEvidenceGapUnchanged},
		{false, false, VerificationChangeChanged, SafetyEvidenceGapChanged},
		{false, true, VerificationChangeSame, SafetyEvidenceGapNewlyUnavailable},
		{true, true, VerificationChangeChanged, SafetyEvidenceGapStillUnavailable},
		{true, false, VerificationChangeSame, SafetyEvidenceGapResolved},
	}
	for _, test := range tests {
		if actual := classifySafetyEvidenceGap(test.baseline, test.current, test.evidence); actual != test.expected {
			t.Fatalf("classify(%t,%t,%s)=%s, want %s", test.baseline, test.current, test.evidence, actual, test.expected)
		}
	}
}

func TestSafetyEvidenceGapInventoryMapsOnlyExactCategoryReasons(t *testing.T) {
	tests := []struct {
		category string
		reasons  []ReasonCode
		expected []ReasonCode
	}{
		{"PROVIDER_CAPABILITY", []ReasonCode{ReasonLiveRuntimeUnimplemented, ReasonProviderCapabilityUnavailable, ReasonTransferPermissionPresent}, []ReasonCode{ReasonProviderCapabilityUnavailable, ReasonTransferPermissionPresent}},
		{"OWNER_AUTHORIZATION", []ReasonCode{ReasonOwnerAuthorizationUnavailable, ReasonStaleEvidence}, []ReasonCode{ReasonOwnerAuthorizationUnavailable}},
		{"DETERMINISTIC_RISK", []ReasonCode{ReasonDeterministicRiskUnavailable, ReasonNonLiveMode}, []ReasonCode{ReasonDeterministicRiskUnavailable}},
		{"ACCOUNT_RECONCILIATION", []ReasonCode{ReasonReconciliationUnavailable}, []ReasonCode{ReasonReconciliationUnavailable}},
		{"BROKER_KILL_SWITCH", []ReasonCode{ReasonKillSwitchUnavailable}, []ReasonCode{ReasonKillSwitchUnavailable}},
		{"IDEMPOTENCY_RESERVATION", []ReasonCode{ReasonIdempotencyUnavailable}, []ReasonCode{ReasonIdempotencyUnavailable}},
		{"LIVE_LIFECYCLE_CONTRACT", []ReasonCode{ReasonLifecycleContractUnavailable, ReasonLiveRuntimeUnimplemented}, []ReasonCode{ReasonLifecycleContractUnavailable}},
	}
	for _, test := range tests {
		actual, ok := categoryUnavailableReasons(test.category, test.reasons)
		if !ok || !reflect.DeepEqual(actual, test.expected) {
			t.Fatalf("category %s reasons=%v, want %v (ok=%t)", test.category, actual, test.expected, ok)
		}
	}
	if reasons, ok := categoryUnavailableReasons("UNKNOWN", []ReasonCode{ReasonInvalidEvidence}); ok || reasons != nil {
		t.Fatalf("unknown category returned reasons: %v %t", reasons, ok)
	}
}

func TestSafetyEvidenceGapInventoryPreservesRejectedComparison(t *testing.T) {
	now := time.Date(2026, 9, 8, 17, 0, 0, 0, time.UTC)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	rejectedCurrent := cloneSafetyCaseReport(t, baseline)
	rejectedCurrent.Status = VerificationRejected
	envelope, audit, review := verifiedComparisonReviewChain(t, baseline, rejectedCurrent)
	inventory := CompileSafetyEvidenceGapInventory(baseline, rejectedCurrent, envelope, audit, review)
	if inventory.Status != SafetyEvidenceGapInventoryRejected ||
		inventory.Failure != SafetyEvidenceGapInventoryFailureComparisonRejected ||
		inventory.ComparisonStatus != VerificationComparisonRejected ||
		inventory.ComparisonFailureInput != VerificationComparisonInputCurrent ||
		inventory.ComparisonFailure != VerificationComparisonFailureStatus ||
		inventory.ReviewDisposition != VerificationComparisonReviewInputRejected ||
		inventory.OwnerAction != VerificationComparisonOwnerActionReviewRejection ||
		inventory.ExecutionAuthority || len(inventory.Categories) != 0 || inventory.InventoryDigest != "" {
		t.Fatalf("rejected comparison was not preserved closed: %#v", inventory)
	}
}

func TestSafetyEvidenceGapInventoryRejectsSecretWithoutEcho(t *testing.T) {
	now := time.Date(2026, 9, 8, 17, 0, 0, 0, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, audit, review := verifiedComparisonReviewChain(t, input, input)
	review.OwnerAction = VerificationComparisonOwnerAction("client_secret_do_not_copy")
	inventory := CompileSafetyEvidenceGapInventory(input, input, envelope, audit, review)
	if inventory.Status != SafetyEvidenceGapInventoryRejected ||
		inventory.Failure != SafetyEvidenceGapInventoryFailureSecretInput ||
		inventory.ExecutionAuthority || inventory.ReviewDigest != "" ||
		len(inventory.Categories) != 0 || inventory.InventoryDigest != "" {
		t.Fatalf("secret-like review produced inventory output: %#v", inventory)
	}
	payload, marshalErr := json.Marshal(inventory)
	if marshalErr != nil || strings.Contains(string(payload), "client_secret_do_not_copy") {
		t.Fatalf("secret-like input was echoed: %s %v", payload, marshalErr)
	}
}

func TestSafetyEvidenceGapInventoryRejectsChangedInputChain(t *testing.T) {
	now := time.Date(2026, 9, 8, 17, 0, 0, 0, time.UTC)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	current := cloneSafetyCaseReport(t, baseline)
	envelope, audit, review := verifiedComparisonReviewChain(t, baseline, current)
	changedCurrent := cloneSafetyCaseReport(t, current)
	changedCurrent.EvaluatedAt = changedCurrent.EvaluatedAt.Add(time.Nanosecond)
	inventory := CompileSafetyEvidenceGapInventory(baseline, changedCurrent, envelope, audit, review)
	if inventory.Status != SafetyEvidenceGapInventoryRejected ||
		inventory.Failure != SafetyEvidenceGapInventoryFailureReviewMismatch || inventory.InventoryDigest != "" {
		t.Fatalf("changed input chain was not rejected: %#v", inventory)
	}
	changedReview := cloneVerificationComparisonReviewArtifact(t, review)
	changedReview.Digests.Envelope.Verified = false
	inventory = CompileSafetyEvidenceGapInventory(baseline, current, envelope, audit, changedReview)
	if inventory.Failure != SafetyEvidenceGapInventoryFailureReviewMismatch {
		t.Fatalf("changed review evidence was not rejected: %#v", inventory)
	}
}

func TestSafetyEvidenceGapInventoryRejectsEveryOutputTamper(t *testing.T) {
	now := time.Date(2026, 9, 8, 17, 0, 0, 0, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, audit, review := verifiedComparisonReviewChain(t, input, input)
	original := CompileSafetyEvidenceGapInventory(input, input, envelope, audit, review)
	mutations := []struct {
		name   string
		mutate func(*SafetyEvidenceGapInventory)
	}{
		{"version", func(value *SafetyEvidenceGapInventory) { value.InventoryVersion = "inventory-v2" }},
		{"status", func(value *SafetyEvidenceGapInventory) { value.Status = SafetyEvidenceGapInventoryRejected }},
		{"failure", func(value *SafetyEvidenceGapInventory) { value.Failure = SafetyEvidenceGapInventoryFailureCategory }},
		{"authority", func(value *SafetyEvidenceGapInventory) { value.ExecutionAuthority = true }},
		{"review digest", func(value *SafetyEvidenceGapInventory) { value.ReviewDigest = digestB }},
		{"baseline time", func(value *SafetyEvidenceGapInventory) {
			value.BaselineEvaluatedAt = value.BaselineEvaluatedAt.Add(time.Nanosecond)
		}},
		{"comparison status", func(value *SafetyEvidenceGapInventory) { value.ComparisonStatus = VerificationComparisonChanged }},
		{"review disposition", func(value *SafetyEvidenceGapInventory) { value.ReviewDisposition = VerificationComparisonReviewChanged }},
		{"owner action", func(value *SafetyEvidenceGapInventory) {
			value.OwnerAction = VerificationComparisonOwnerActionReviewChanges
		}},
		{"assessment", func(value *SafetyEvidenceGapInventory) { value.Assessment.After.Availability = Unavailable }},
		{"category state", func(value *SafetyEvidenceGapInventory) { value.Categories[0].State = SafetyEvidenceGapChanged }},
		{"category reason", func(value *SafetyEvidenceGapInventory) {
			value.Categories[0].CurrentReasonCodes = []ReasonCode{ReasonProviderCapabilityUnavailable}
		}},
		{"category fact", func(value *SafetyEvidenceGapInventory) { value.Categories[0].Current.Digest = digestA }},
		{"category order", func(value *SafetyEvidenceGapInventory) {
			value.Categories[0], value.Categories[1] = value.Categories[1], value.Categories[0]
		}},
		{"gap count", func(value *SafetyEvidenceGapInventory) { value.CurrentGapCount++ }},
		{"state count", func(value *SafetyEvidenceGapInventory) { value.NonUnchangedCategoryCount++ }},
		{"inventory digest", func(value *SafetyEvidenceGapInventory) { value.InventoryDigest = digestB }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			inventory := cloneSafetyEvidenceGapInventory(t, original)
			mutation.mutate(&inventory)
			if !errors.Is(VerifySafetyEvidenceGapInventory(input, input, envelope, audit, review, inventory), ErrSafetyEvidenceGapInventory) {
				t.Fatalf("tampered gap inventory verified: %#v", inventory)
			}
		})
	}
}

func TestSafetyEvidenceGapInventoryHasNoPersistenceOrNetworkSurface(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("gap inventory test path unavailable")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "verification_gap_inventory.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"context.Context", "NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient", "SubmitOrder", "CancelOrder",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("gap inventory unexpectedly contains persistence, provider, or broker surface %q", prohibited)
		}
	}
}

func verifiedComparisonReviewChain(t *testing.T, baseline, current SafetyCaseVerificationReport) (SafetyCaseVerificationComparisonEnvelope, SafetyCaseVerificationComparisonEnvelopeReport, SafetyCaseVerificationComparisonReviewArtifact) {
	t.Helper()
	envelope, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, current)
	if err != nil {
		t.Fatal(err)
	}
	audit := BuildSafetyCaseVerificationComparisonEnvelopeReport(baseline, current, envelope)
	if audit.Status != VerificationVerified {
		t.Fatalf("test audit did not verify: %#v", audit)
	}
	review := BuildSafetyCaseVerificationComparisonReviewArtifact(baseline, current, envelope, audit)
	if review.Status != VerificationComparisonReviewReady {
		t.Fatalf("test review did not compile: %#v", review)
	}
	return envelope, audit, review
}

func containsReason(reasons []ReasonCode, expected ReasonCode) bool {
	for _, reason := range reasons {
		if reason == expected {
			return true
		}
	}
	return false
}

func cloneSafetyEvidenceGapInventory(t *testing.T, value SafetyEvidenceGapInventory) SafetyEvidenceGapInventory {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyEvidenceGapInventory
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
