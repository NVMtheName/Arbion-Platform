package liveexecution

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type remediationMatrixChain struct {
	baseline           SafetyCaseVerificationReport
	current            SafetyCaseVerificationReport
	envelope           SafetyCaseVerificationComparisonEnvelope
	audit              SafetyCaseVerificationComparisonEnvelopeReport
	review             SafetyCaseVerificationComparisonReviewArtifact
	inventory          SafetyEvidenceGapInventory
	report             SafetyEvidenceGapInventoryVerificationReport
	verificationReview SafetyEvidenceGapInventoryVerificationReview
}

func TestSafetyEvidenceGapRemediationMatrixIsDeterministicCanonicalAndFixed(t *testing.T) {
	now := time.Date(2026, 9, 8, 21, 0, 0, 123, time.UTC)
	chain := buildRemediationMatrixChain(t, now, nil)
	first := compileRemediationMatrix(chain)
	second := compileRemediationMatrix(chain)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("remediation matrix replay changed: %#v %#v", first, second)
	}
	if first.MatrixVersion != SafetyEvidenceGapRemediationMatrixVersion ||
		first.Status != SafetyEvidenceGapRemediationMatrixCompiled ||
		first.Failure != SafetyEvidenceGapRemediationMatrixFailureNone || first.ExecutionAuthority ||
		first.ReviewDisposition != SafetyEvidenceGapInventoryVerificationReviewNoGaps ||
		first.ReviewOwnerAction != SafetyEvidenceGapInventoryVerificationReviewOwnerActionNone ||
		first.ExpectedCategoryCount != 7 || first.CurrentGapCount != 0 || len(first.Requirements) != 0 ||
		first.VerificationReportDigest != chain.report.ReportDigest ||
		first.VerificationReviewDigest != chain.verificationReview.ArtifactDigest ||
		first.InventoryDigest != chain.inventory.InventoryDigest || first.UpstreamReviewDigest != chain.review.ReviewDigest ||
		!digestPattern.MatchString(first.MatrixDigest) {
		t.Fatalf("remediation matrix is incomplete or authoritative: %#v", first)
	}
	if first.MatrixDigest != "de7b6f1eff7eaa625677b9997ad2f654c6028a84ec1e8f425da014eaa38dcd28" {
		t.Fatalf("fixed remediation matrix changed: %s", first.MatrixDigest)
	}
	if first.MatrixDigest == first.VerificationReportDigest || first.MatrixDigest == first.VerificationReviewDigest ||
		first.MatrixDigest == first.InventoryDigest || first.MatrixDigest == first.UpstreamReviewDigest {
		t.Fatal("remediation matrix digest was not domain-separated")
	}
	if err := verifyRemediationMatrix(chain, first); err != nil {
		t.Fatalf("exact remediation matrix did not verify: %v", err)
	}
}

func TestSafetyEvidenceGapRemediationMatrixMapsEveryExactUnavailableCategory(t *testing.T) {
	tests := []struct {
		name       string
		category   string
		source     EvidenceSource
		reason     ReasonCode
		boundary   SafetyEvidenceGapResponsibleBoundary
		followUp   SafetyEvidenceGapFollowUp
		mutateCase func(*SafetyCase)
	}{
		{"provider", "PROVIDER_CAPABILITY", EvidenceProviderVerified, ReasonProviderCapabilityUnavailable, SafetyEvidenceGapBoundaryFinancialProvider, SafetyEvidenceGapFollowUpVerifyProvider, func(value *SafetyCase) { value.ProviderCapability.TradeAllowed = false }},
		{"owner authorization", "OWNER_AUTHORIZATION", EvidenceOwnerMFA, ReasonOwnerAuthorizationUnavailable, SafetyEvidenceGapBoundaryOwnerAuthorization, SafetyEvidenceGapFollowUpRecordAuthorization, func(value *SafetyCase) { value.OwnerAuthorization.MFAVerified = false }},
		{"risk", "DETERMINISTIC_RISK", EvidenceDeterministicControl, ReasonDeterministicRiskUnavailable, SafetyEvidenceGapBoundaryRiskControl, SafetyEvidenceGapFollowUpVerifyRisk, func(value *SafetyCase) { value.Risk.PlatformExecutionAvailable = false }},
		{"reconciliation", "ACCOUNT_RECONCILIATION", EvidenceDatabase, ReasonReconciliationUnavailable, SafetyEvidenceGapBoundaryReconciliation, SafetyEvidenceGapFollowUpVerifyReconciliation, func(value *SafetyCase) { value.PreTradeReconciliation.BlocksNewActions = true }},
		{"kill switch", "BROKER_KILL_SWITCH", EvidenceDeterministicControl, ReasonKillSwitchUnavailable, SafetyEvidenceGapBoundaryBrokerControl, SafetyEvidenceGapFollowUpVerifyKillSwitch, func(value *SafetyCase) { value.KillSwitch.BrokerEnforcementReady = false }},
		{"idempotency", "IDEMPOTENCY_RESERVATION", EvidenceDatabase, ReasonIdempotencyUnavailable, SafetyEvidenceGapBoundaryIdempotency, SafetyEvidenceGapFollowUpVerifyIdempotency, func(value *SafetyCase) { value.Idempotency.ReplayDetected = true }},
		{"lifecycle", "LIVE_LIFECYCLE_CONTRACT", EvidenceDesignContract, ReasonLifecycleContractUnavailable, SafetyEvidenceGapBoundaryLifecycleContract, SafetyEvidenceGapFollowUpVerifyLifecycle, func(value *SafetyCase) { value.Lifecycle.RequirePostTradeReconciliation = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 21, 5, 0, 0, time.UTC), test.mutateCase)
			matrix := compileRemediationMatrix(chain)
			if matrix.Status != SafetyEvidenceGapRemediationMatrixCompiled ||
				matrix.ReviewDisposition != SafetyEvidenceGapInventoryVerificationReviewOwnerReview ||
				matrix.ReviewOwnerAction != SafetyEvidenceGapInventoryVerificationReviewOwnerActionReviewGaps ||
				matrix.CurrentGapCount != 1 || len(matrix.Requirements) != 1 || matrix.ExecutionAuthority {
				t.Fatalf("single unavailable category did not compile exactly: %#v", matrix)
			}
			requirement := matrix.Requirements[0]
			if requirement.Category != test.category || requirement.RequiredSource != test.source ||
				requirement.GapState != SafetyEvidenceGapNewlyUnavailable ||
				!reflect.DeepEqual(requirement.ReasonCodes, []ReasonCode{test.reason}) ||
				requirement.ResponsibleBoundary != test.boundary || requirement.SafeFollowUp != test.followUp {
				t.Fatalf("category mapping changed: %#v", requirement)
			}
			if err := verifyRemediationMatrix(chain, matrix); err != nil {
				t.Fatalf("exact category matrix did not verify: %v", err)
			}
		})
	}
}

func TestSafetyEvidenceGapRemediationMatrixPreservesCanonicalGapOrder(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 21, 10, 0, 0, time.UTC), func(value *SafetyCase) {
		value.ProviderCapability.TradeAllowed = false
		value.Risk.PlatformExecutionAvailable = false
		value.Idempotency.ReplayDetected = true
	})
	matrix := compileRemediationMatrix(chain)
	want := []string{"PROVIDER_CAPABILITY", "DETERMINISTIC_RISK", "IDEMPOTENCY_RESERVATION"}
	if matrix.CurrentGapCount != len(want) || len(matrix.Requirements) != len(want) {
		t.Fatalf("multi-gap matrix count changed: %#v", matrix)
	}
	for index, category := range want {
		if matrix.Requirements[index].Category != category {
			t.Fatalf("requirement[%d]=%s, want %s", index, matrix.Requirements[index].Category, category)
		}
	}
}

func TestSafetyEvidenceGapRemediationMatrixRejectsSecretAndAuthorityWithoutEcho(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 21, 15, 0, 0, time.UTC), nil)
	secret := cloneGapInventoryVerificationReview(t, chain.verificationReview)
	secret.ArtifactVersion = "authorization_header_do_not_copy"
	matrix := CompileSafetyEvidenceGapRemediationMatrix(chain.baseline, chain.current, chain.envelope, chain.audit, chain.review, chain.inventory, chain.report, secret)
	if matrix.Status != SafetyEvidenceGapRemediationMatrixRejected ||
		matrix.Failure != SafetyEvidenceGapRemediationMatrixFailureSecretInput || matrix.ExecutionAuthority ||
		matrix.MatrixDigest != "" || len(matrix.Requirements) != 0 {
		t.Fatalf("secret-like input was not rejected closed: %#v", matrix)
	}
	payload, err := json.Marshal(matrix)
	if err != nil || strings.Contains(string(payload), "authorization_header_do_not_copy") {
		t.Fatalf("secret-like input was echoed: %s %v", payload, err)
	}

	authoritative := cloneGapInventoryVerificationReview(t, chain.verificationReview)
	authoritative.ExecutionAuthority = true
	matrix = CompileSafetyEvidenceGapRemediationMatrix(chain.baseline, chain.current, chain.envelope, chain.audit, chain.review, chain.inventory, chain.report, authoritative)
	if matrix.Failure != SafetyEvidenceGapRemediationMatrixFailureAuthority || matrix.MatrixDigest != "" {
		t.Fatalf("authority-bearing input was not rejected: %#v", matrix)
	}
}

func TestSafetyEvidenceGapRemediationMatrixRejectsEveryOutputTamper(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 21, 20, 0, 0, time.UTC), func(value *SafetyCase) {
		value.ProviderCapability.TradeAllowed = false
		value.OwnerAuthorization.MFAVerified = false
	})
	original := compileRemediationMatrix(chain)
	mutations := []struct {
		name   string
		mutate func(*SafetyEvidenceGapRemediationMatrix)
	}{
		{"version", func(value *SafetyEvidenceGapRemediationMatrix) { value.MatrixVersion = "matrix-v2" }},
		{"status", func(value *SafetyEvidenceGapRemediationMatrix) {
			value.Status = SafetyEvidenceGapRemediationMatrixRejected
		}},
		{"failure", func(value *SafetyEvidenceGapRemediationMatrix) {
			value.Failure = SafetyEvidenceGapRemediationMatrixFailureCount
		}},
		{"authority", func(value *SafetyEvidenceGapRemediationMatrix) { value.ExecutionAuthority = true }},
		{"disposition", func(value *SafetyEvidenceGapRemediationMatrix) {
			value.ReviewDisposition = SafetyEvidenceGapInventoryVerificationReviewNoGaps
		}},
		{"owner action", func(value *SafetyEvidenceGapRemediationMatrix) {
			value.ReviewOwnerAction = SafetyEvidenceGapInventoryVerificationReviewOwnerActionNone
		}},
		{"baseline time", func(value *SafetyEvidenceGapRemediationMatrix) {
			value.BaselineEvaluatedAt = value.BaselineEvaluatedAt.Add(time.Nanosecond)
		}},
		{"current time", func(value *SafetyEvidenceGapRemediationMatrix) {
			value.CurrentEvaluatedAt = value.CurrentEvaluatedAt.Add(time.Nanosecond)
		}},
		{"expected count", func(value *SafetyEvidenceGapRemediationMatrix) { value.ExpectedCategoryCount++ }},
		{"gap count", func(value *SafetyEvidenceGapRemediationMatrix) { value.CurrentGapCount++ }},
		{"verification report digest", func(value *SafetyEvidenceGapRemediationMatrix) { value.VerificationReportDigest = digestB }},
		{"verification review digest", func(value *SafetyEvidenceGapRemediationMatrix) { value.VerificationReviewDigest = digestB }},
		{"inventory digest", func(value *SafetyEvidenceGapRemediationMatrix) { value.InventoryDigest = digestB }},
		{"upstream review digest", func(value *SafetyEvidenceGapRemediationMatrix) { value.UpstreamReviewDigest = digestB }},
		{"requirement category", func(value *SafetyEvidenceGapRemediationMatrix) { value.Requirements[0].Category += "_CHANGED" }},
		{"requirement source", func(value *SafetyEvidenceGapRemediationMatrix) {
			value.Requirements[0].RequiredSource = EvidenceDatabase
		}},
		{"requirement state", func(value *SafetyEvidenceGapRemediationMatrix) {
			value.Requirements[0].GapState = SafetyEvidenceGapResolved
		}},
		{"requirement reasons", func(value *SafetyEvidenceGapRemediationMatrix) {
			value.Requirements[0].ReasonCodes[0] = ReasonInvalidEvidence
		}},
		{"requirement boundary", func(value *SafetyEvidenceGapRemediationMatrix) {
			value.Requirements[0].ResponsibleBoundary = SafetyEvidenceGapBoundaryLifecycleContract
		}},
		{"requirement follow-up", func(value *SafetyEvidenceGapRemediationMatrix) {
			value.Requirements[0].SafeFollowUp = SafetyEvidenceGapFollowUpVerifyLifecycle
		}},
		{"requirement order", func(value *SafetyEvidenceGapRemediationMatrix) {
			value.Requirements[0], value.Requirements[1] = value.Requirements[1], value.Requirements[0]
		}},
		{"requirement removed", func(value *SafetyEvidenceGapRemediationMatrix) { value.Requirements = value.Requirements[:1] }},
		{"digest", func(value *SafetyEvidenceGapRemediationMatrix) { value.MatrixDigest = digestB }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			matrix := cloneRemediationMatrix(t, original)
			mutation.mutate(&matrix)
			if !errors.Is(verifyRemediationMatrix(chain, matrix), ErrSafetyEvidenceGapRemediationMatrix) {
				t.Fatalf("tampered matrix verified: %#v", matrix)
			}
		})
	}
}

func TestSafetyEvidenceGapRemediationMatrixBindsEveryInput(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 21, 25, 0, 0, time.UTC), nil)
	matrix := compileRemediationMatrix(chain)
	tests := []struct {
		name   string
		mutate func(*remediationMatrixChain)
	}{
		{"baseline", func(value *remediationMatrixChain) {
			value.baseline.EvaluatedAt = value.baseline.EvaluatedAt.Add(time.Nanosecond)
		}},
		{"current", func(value *remediationMatrixChain) {
			value.current.EvaluatedAt = value.current.EvaluatedAt.Add(time.Nanosecond)
		}},
		{"envelope", func(value *remediationMatrixChain) { value.envelope.EnvelopeDigest = digestB }},
		{"audit", func(value *remediationMatrixChain) { value.audit.Digests.Envelope.Claimed = digestB }},
		{"review", func(value *remediationMatrixChain) { value.review.ReviewDigest = digestB }},
		{"inventory", func(value *remediationMatrixChain) { value.inventory.InventoryDigest = digestB }},
		{"report", func(value *remediationMatrixChain) { value.report.ReportDigest = digestB }},
		{"verification review", func(value *remediationMatrixChain) { value.verificationReview.ArtifactDigest = digestB }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := cloneRemediationMatrixChain(t, chain)
			test.mutate(&changed)
			if !errors.Is(verifyRemediationMatrix(changed, matrix), ErrSafetyEvidenceGapRemediationMatrix) {
				t.Fatalf("changed %s verified against prior matrix", test.name)
			}
		})
	}
}

func TestSafetyEvidenceGapRemediationMatrixHasNoRuntimeSurfaceOrProductionCaller(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("remediation matrix test path unavailable")
	}
	directory := filepath.Dir(filename)
	source, err := os.ReadFile(filepath.Join(directory, "verification_gap_remediation_matrix.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"context.Context", "NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient", "SubmitOrder", "CancelOrder",
		"RunCycle", "Execute", "Refresh", "Reconnect", "Sync", "PlaceOrder",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("remediation matrix unexpectedly contains runtime surface %q", prohibited)
		}
	}
	apiRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	for _, symbol := range []string{"CompileSafetyEvidenceGapRemediationMatrix(", "VerifySafetyEvidenceGapRemediationMatrix("} {
		err = filepath.WalkDir(apiRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
				strings.HasPrefix(path, directory+string(filepath.Separator)) {
				return nil
			}
			payload, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if strings.Contains(string(payload), symbol) {
				t.Fatalf("remediation matrix has a production caller in %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func buildRemediationMatrixChain(t *testing.T, now time.Time, mutateCurrent func(*SafetyCase)) remediationMatrixChain {
	t.Helper()
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	currentCase := validSafetyCase(now.Add(time.Minute))
	if mutateCurrent != nil {
		mutateCurrent(&currentCase)
	}
	current := verifiedSafetyCaseReport(t, currentCase, now.Add(time.Minute))
	envelope, audit, review := verifiedComparisonReviewChain(t, baseline, current)
	inventory := CompileSafetyEvidenceGapInventory(baseline, current, envelope, audit, review)
	report := BuildSafetyEvidenceGapInventoryVerificationReport(baseline, current, envelope, audit, review, inventory)
	verificationReview := BuildSafetyEvidenceGapInventoryVerificationReview(baseline, current, envelope, audit, review, inventory, report)
	return remediationMatrixChain{baseline, current, envelope, audit, review, inventory, report, verificationReview}
}

func compileRemediationMatrix(chain remediationMatrixChain) SafetyEvidenceGapRemediationMatrix {
	return CompileSafetyEvidenceGapRemediationMatrix(
		chain.baseline, chain.current, chain.envelope, chain.audit, chain.review,
		chain.inventory, chain.report, chain.verificationReview,
	)
}

func verifyRemediationMatrix(chain remediationMatrixChain, matrix SafetyEvidenceGapRemediationMatrix) error {
	return VerifySafetyEvidenceGapRemediationMatrix(
		chain.baseline, chain.current, chain.envelope, chain.audit, chain.review,
		chain.inventory, chain.report, chain.verificationReview, matrix,
	)
}

func cloneRemediationMatrix(t *testing.T, value SafetyEvidenceGapRemediationMatrix) SafetyEvidenceGapRemediationMatrix {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyEvidenceGapRemediationMatrix
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func cloneRemediationMatrixChain(t *testing.T, value remediationMatrixChain) remediationMatrixChain {
	t.Helper()
	return remediationMatrixChain{
		baseline:           cloneSafetyCaseReport(t, value.baseline),
		current:            cloneSafetyCaseReport(t, value.current),
		envelope:           cloneVerificationComparisonEnvelope(t, value.envelope),
		audit:              cloneVerificationComparisonEnvelopeReport(t, value.audit),
		review:             cloneVerificationComparisonReviewArtifact(t, value.review),
		inventory:          cloneSafetyEvidenceGapInventory(t, value.inventory),
		report:             cloneGapInventoryVerificationReport(t, value.report),
		verificationReview: cloneGapInventoryVerificationReview(t, value.verificationReview),
	}
}
