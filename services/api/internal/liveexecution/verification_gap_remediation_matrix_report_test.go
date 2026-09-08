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

func TestSafetyEvidenceGapRemediationMatrixVerificationReportIsIndependentDeterministicAndFixed(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 22, 0, 0, 123, time.UTC), nil)
	matrix := compileRemediationMatrix(chain)
	first := buildRemediationMatrixVerificationReport(chain, matrix)
	second := buildRemediationMatrixVerificationReport(chain, matrix)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("matrix verification report replay changed: %#v %#v", first, second)
	}
	if first.ReportVersion != SafetyEvidenceGapRemediationMatrixVerificationReportVersion ||
		first.Status != VerificationVerified || first.Failure != SafetyEvidenceGapRemediationMatrixVerificationFailureNone ||
		first.ExecutionAuthority || !first.MatrixVersion.Verified ||
		first.ReviewDisposition != SafetyEvidenceGapInventoryVerificationReviewNoGaps ||
		first.ReviewOwnerAction != SafetyEvidenceGapInventoryVerificationReviewOwnerActionNone ||
		!first.VerificationReportDigest.Verified || !first.VerificationReviewDigest.Verified ||
		!first.InventoryDigest.Verified || !first.UpstreamReviewDigest.Verified || !first.MatrixDigest.Verified ||
		first.ExpectedCategoryCount != 7 || first.ClaimedCurrentGapCount != 0 || first.RecomputedCurrentGapCount != 0 ||
		first.ClaimedRequirementCount != 0 || first.ExpectedRequirementCount != 0 ||
		!first.CurrentEvidenceAvailable || len(first.Requirements) != 0 || !digestPattern.MatchString(first.ReportDigest) {
		t.Fatalf("matrix verification report is incomplete or authoritative: %#v", first)
	}
	if first.ReportDigest != "1ab63984464a7f948b032e1029183bf378889257de6904dabec090b4e6667cf9" {
		t.Fatalf("fixed matrix verification report changed: %s", first.ReportDigest)
	}
	if first.ReportDigest == first.MatrixDigest.Claimed || first.ReportDigest == first.VerificationReviewDigest.Claimed {
		t.Fatal("matrix verification report digest was not domain-separated")
	}
	if err := verifyRemediationMatrixVerificationReport(chain, matrix, first); err != nil {
		t.Fatalf("exact matrix verification report did not verify: %v", err)
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReportPreservesOwnerReviewWithoutReadinessClaim(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 22, 5, 0, 0, time.UTC), func(value *SafetyCase) {
		value.OwnerAuthorization.MFAVerified = false
	})
	matrix := compileRemediationMatrix(chain)
	report := buildRemediationMatrixVerificationReport(chain, matrix)
	if report.Status != VerificationVerified || report.Failure != SafetyEvidenceGapRemediationMatrixVerificationFailureNone ||
		report.ExecutionAuthority || report.CurrentEvidenceAvailable ||
		report.ReviewDisposition != SafetyEvidenceGapInventoryVerificationReviewOwnerReview ||
		report.ReviewOwnerAction != SafetyEvidenceGapInventoryVerificationReviewOwnerActionReviewGaps ||
		report.ClaimedCurrentGapCount != 1 || report.RecomputedCurrentGapCount != 1 ||
		report.ClaimedRequirementCount != 1 || report.ExpectedRequirementCount != 1 || len(report.Requirements) != 1 {
		t.Fatalf("owner-review evidence was not preserved exactly: %#v", report)
	}
	requirement := report.Requirements[0]
	if !requirement.Verified || requirement.Claimed.Category != "OWNER_AUTHORIZATION" ||
		!reflect.DeepEqual(requirement.Claimed, requirement.Recomputed) {
		t.Fatalf("owner-review requirement did not verify independently: %#v", requirement)
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReportRejectsEveryRequirementTamper(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 22, 10, 0, 0, time.UTC), func(value *SafetyCase) {
		value.ProviderCapability.TradeAllowed = false
		value.OwnerAuthorization.MFAVerified = false
		value.Risk.PlatformExecutionAvailable = false
		value.PreTradeReconciliation.BlocksNewActions = true
		value.KillSwitch.BrokerEnforcementReady = false
		value.Idempotency.ReplayDetected = true
		value.Lifecycle.RequirePostTradeReconciliation = false
	})
	original := compileRemediationMatrix(chain)
	if len(original.Requirements) != len(independentRemediationExpectations()) {
		t.Fatalf("test matrix does not contain every requirement: %#v", original)
	}
	for index := range original.Requirements {
		mutations := []struct {
			name   string
			mutate func(*SafetyEvidenceGapRemediationRequirement)
		}{
			{"category", func(value *SafetyEvidenceGapRemediationRequirement) { value.Category += "_CHANGED" }},
			{"source", func(value *SafetyEvidenceGapRemediationRequirement) { value.RequiredSource = EvidenceSource("CHANGED") }},
			{"state", func(value *SafetyEvidenceGapRemediationRequirement) { value.GapState = SafetyEvidenceGapResolved }},
			{"reason", func(value *SafetyEvidenceGapRemediationRequirement) { value.ReasonCodes[0] = ReasonInvalidEvidence }},
			{"boundary", func(value *SafetyEvidenceGapRemediationRequirement) {
				value.ResponsibleBoundary = SafetyEvidenceGapResponsibleBoundary("CHANGED")
			}},
			{"follow-up", func(value *SafetyEvidenceGapRemediationRequirement) {
				value.SafeFollowUp = SafetyEvidenceGapFollowUp("CHANGED")
			}},
		}
		for _, mutation := range mutations {
			t.Run(independentRemediationExpectations()[index].category+"/"+mutation.name, func(t *testing.T) {
				matrix := cloneRemediationMatrix(t, original)
				mutation.mutate(&matrix.Requirements[index])
				report := buildRemediationMatrixVerificationReport(chain, matrix)
				if report.Status != VerificationRejected ||
					report.Failure != SafetyEvidenceGapRemediationMatrixVerificationFailureCategory ||
					report.ExecutionAuthority || report.ReportDigest != "" {
					t.Fatalf("requirement tamper was not rejected: %#v", report)
				}
			})
		}
	}
	swapped := cloneRemediationMatrix(t, original)
	swapped.Requirements[0], swapped.Requirements[1] = swapped.Requirements[1], swapped.Requirements[0]
	if report := buildRemediationMatrixVerificationReport(chain, swapped); report.Failure != SafetyEvidenceGapRemediationMatrixVerificationFailureCategory {
		t.Fatalf("requirement order tamper was not rejected: %#v", report)
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReportClassifiesMatrixFailures(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 22, 15, 0, 0, time.UTC), nil)
	original := compileRemediationMatrix(chain)
	tests := []struct {
		name    string
		failure SafetyEvidenceGapRemediationMatrixVerificationFailure
		mutate  func(*SafetyEvidenceGapRemediationMatrix)
	}{
		{"version", SafetyEvidenceGapRemediationMatrixVerificationFailureMatrixVersion, func(value *SafetyEvidenceGapRemediationMatrix) { value.MatrixVersion = "matrix-v2" }},
		{"status", SafetyEvidenceGapRemediationMatrixVerificationFailureMatrixStatus, func(value *SafetyEvidenceGapRemediationMatrix) {
			value.Status = SafetyEvidenceGapRemediationMatrixRejected
		}},
		{"authority", SafetyEvidenceGapRemediationMatrixVerificationFailureAuthority, func(value *SafetyEvidenceGapRemediationMatrix) { value.ExecutionAuthority = true }},
		{"category count", SafetyEvidenceGapRemediationMatrixVerificationFailureCount, func(value *SafetyEvidenceGapRemediationMatrix) { value.ExpectedCategoryCount++ }},
		{"gap count", SafetyEvidenceGapRemediationMatrixVerificationFailureCount, func(value *SafetyEvidenceGapRemediationMatrix) { value.CurrentGapCount++ }},
		{"requirement count", SafetyEvidenceGapRemediationMatrixVerificationFailureCount, func(value *SafetyEvidenceGapRemediationMatrix) {
			value.Requirements = append(value.Requirements, SafetyEvidenceGapRemediationRequirement{})
		}},
		{"verification report digest", SafetyEvidenceGapRemediationMatrixVerificationFailureMatrixMismatch, func(value *SafetyEvidenceGapRemediationMatrix) { value.VerificationReportDigest = digestB }},
		{"verification review digest", SafetyEvidenceGapRemediationMatrixVerificationFailureMatrixMismatch, func(value *SafetyEvidenceGapRemediationMatrix) { value.VerificationReviewDigest = digestB }},
		{"inventory digest", SafetyEvidenceGapRemediationMatrixVerificationFailureMatrixMismatch, func(value *SafetyEvidenceGapRemediationMatrix) { value.InventoryDigest = digestB }},
		{"review digest", SafetyEvidenceGapRemediationMatrixVerificationFailureMatrixMismatch, func(value *SafetyEvidenceGapRemediationMatrix) { value.UpstreamReviewDigest = digestB }},
		{"digest malformed", SafetyEvidenceGapRemediationMatrixVerificationFailureDigestMalformed, func(value *SafetyEvidenceGapRemediationMatrix) { value.MatrixDigest = "not-a-digest" }},
		{"digest mismatch", SafetyEvidenceGapRemediationMatrixVerificationFailureDigestMismatch, func(value *SafetyEvidenceGapRemediationMatrix) { value.MatrixDigest = digestB }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matrix := cloneRemediationMatrix(t, original)
			test.mutate(&matrix)
			report := buildRemediationMatrixVerificationReport(chain, matrix)
			if report.Status != VerificationRejected || report.Failure != test.failure ||
				report.ExecutionAuthority || report.ReportDigest != "" {
				t.Fatalf("matrix failure was not classified exactly: %#v", report)
			}
		})
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReportRejectsSecretWithoutEcho(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 22, 20, 0, 0, time.UTC), nil)
	matrix := compileRemediationMatrix(chain)
	matrix.MatrixVersion = "client_secret_do_not_copy"
	report := buildRemediationMatrixVerificationReport(chain, matrix)
	if report.Status != VerificationRejected ||
		report.Failure != SafetyEvidenceGapRemediationMatrixVerificationFailureSecretInput ||
		report.ExecutionAuthority || report.ReportDigest != "" || len(report.Requirements) != 0 {
		t.Fatalf("secret-like input produced attributable output: %#v", report)
	}
	payload, err := json.Marshal(report)
	if err != nil || strings.Contains(string(payload), "client_secret_do_not_copy") {
		t.Fatalf("secret-like input was echoed: %s %v", payload, err)
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReportRejectsEveryOutputTamper(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 22, 25, 0, 0, time.UTC), func(value *SafetyCase) {
		value.ProviderCapability.TradeAllowed = false
	})
	matrix := compileRemediationMatrix(chain)
	original := buildRemediationMatrixVerificationReport(chain, matrix)
	mutations := []struct {
		name   string
		mutate func(*SafetyEvidenceGapRemediationMatrixVerificationReport)
	}{
		{"version", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) { value.ReportVersion = "report-v2" }},
		{"status", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) { value.Status = VerificationRejected }},
		{"failure", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.Failure = SafetyEvidenceGapRemediationMatrixVerificationFailureCount
		}},
		{"authority", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) { value.ExecutionAuthority = true }},
		{"matrix version", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.MatrixVersion.Verified = false
		}},
		{"disposition", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.ReviewDisposition = SafetyEvidenceGapInventoryVerificationReviewNoGaps
		}},
		{"owner action", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.ReviewOwnerAction = SafetyEvidenceGapInventoryVerificationReviewOwnerActionNone
		}},
		{"baseline time", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.BaselineEvaluatedAt = value.BaselineEvaluatedAt.Add(time.Nanosecond)
		}},
		{"current time", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.CurrentEvaluatedAt = value.CurrentEvaluatedAt.Add(time.Nanosecond)
		}},
		{"report digest evidence", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.VerificationReportDigest.Verified = false
		}},
		{"review digest evidence", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.VerificationReviewDigest.Claimed = digestB
		}},
		{"inventory digest evidence", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.InventoryDigest.Expected = digestB
		}},
		{"upstream digest evidence", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.UpstreamReviewDigest.Recomputed = digestB
		}},
		{"matrix digest evidence", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) { value.MatrixDigest.Verified = false }},
		{"category count", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) { value.ExpectedCategoryCount++ }},
		{"claimed gaps", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) { value.ClaimedCurrentGapCount++ }},
		{"recomputed gaps", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) { value.RecomputedCurrentGapCount++ }},
		{"claimed requirements", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) { value.ClaimedRequirementCount++ }},
		{"expected requirements", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) { value.ExpectedRequirementCount++ }},
		{"availability", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.CurrentEvidenceAvailable = true
		}},
		{"requirement claimed", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.Requirements[0].Claimed.Category += "_CHANGED"
		}},
		{"requirement recomputed", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.Requirements[0].Recomputed.Category += "_CHANGED"
		}},
		{"requirement verified", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.Requirements[0].Verified = false
		}},
		{"report digest", func(value *SafetyEvidenceGapRemediationMatrixVerificationReport) { value.ReportDigest = digestB }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			report := cloneRemediationMatrixVerificationReport(t, original)
			mutation.mutate(&report)
			if !errors.Is(verifyRemediationMatrixVerificationReport(chain, matrix, report), ErrSafetyEvidenceGapRemediationMatrixVerificationReport) {
				t.Fatalf("tampered report verified: %#v", report)
			}
		})
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReportBindsEveryInput(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 22, 30, 0, 0, time.UTC), nil)
	matrix := compileRemediationMatrix(chain)
	report := buildRemediationMatrixVerificationReport(chain, matrix)
	tests := []struct {
		name   string
		mutate func(*remediationMatrixChain, *SafetyEvidenceGapRemediationMatrix)
	}{
		{"baseline", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix) {
			value.baseline.EvaluatedAt = value.baseline.EvaluatedAt.Add(time.Nanosecond)
		}},
		{"current", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix) {
			value.current.EvaluatedAt = value.current.EvaluatedAt.Add(time.Nanosecond)
		}},
		{"envelope", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix) {
			value.envelope.EnvelopeDigest = digestB
		}},
		{"audit", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix) {
			value.audit.Digests.Envelope.Verified = false
		}},
		{"review", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix) {
			value.review.ReviewDigest = digestB
		}},
		{"inventory", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix) {
			value.inventory.InventoryDigest = digestB
		}},
		{"gap report", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix) {
			value.report.ReportDigest = digestB
		}},
		{"review artifact", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix) {
			value.verificationReview.ArtifactDigest = digestB
		}},
		{"matrix", func(_ *remediationMatrixChain, value *SafetyEvidenceGapRemediationMatrix) {
			value.MatrixDigest = digestB
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changedChain := cloneRemediationMatrixChain(t, chain)
			changedMatrix := cloneRemediationMatrix(t, matrix)
			test.mutate(&changedChain, &changedMatrix)
			if !errors.Is(verifyRemediationMatrixVerificationReport(changedChain, changedMatrix, report), ErrSafetyEvidenceGapRemediationMatrixVerificationReport) {
				t.Fatalf("changed %s verified against prior report", test.name)
			}
		})
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReportHasNoRuntimeSurfaceOrProductionCaller(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("matrix verification report test path unavailable")
	}
	directory := filepath.Dir(filename)
	source, err := os.ReadFile(filepath.Join(directory, "verification_gap_remediation_matrix_report.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"context.Context", "NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient", "SubmitOrder", "CancelOrder",
		"RunCycle", "Execute", "Refresh", "Reconnect", "Sync", "PlaceOrder",
		"CompileSafetyEvidenceGapRemediationMatrix(", "VerifySafetyEvidenceGapRemediationMatrix(",
		"safetyEvidenceGapRemediationContract(", "canonicalSafetyEvidenceGapRemediationMatrixDigest(",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("matrix verification report unexpectedly contains shared or runtime surface %q", prohibited)
		}
	}
	apiRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	for _, symbol := range []string{"BuildSafetyEvidenceGapRemediationMatrixVerificationReport(", "VerifySafetyEvidenceGapRemediationMatrixVerificationReport("} {
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
				t.Fatalf("matrix verification report has a production caller in %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func buildRemediationMatrixVerificationReport(chain remediationMatrixChain, matrix SafetyEvidenceGapRemediationMatrix) SafetyEvidenceGapRemediationMatrixVerificationReport {
	return BuildSafetyEvidenceGapRemediationMatrixVerificationReport(
		chain.baseline, chain.current, chain.envelope, chain.audit, chain.review,
		chain.inventory, chain.report, chain.verificationReview, matrix,
	)
}

func verifyRemediationMatrixVerificationReport(chain remediationMatrixChain, matrix SafetyEvidenceGapRemediationMatrix, report SafetyEvidenceGapRemediationMatrixVerificationReport) error {
	return VerifySafetyEvidenceGapRemediationMatrixVerificationReport(
		chain.baseline, chain.current, chain.envelope, chain.audit, chain.review,
		chain.inventory, chain.report, chain.verificationReview, matrix, report,
	)
}

func cloneRemediationMatrixVerificationReport(t *testing.T, value SafetyEvidenceGapRemediationMatrixVerificationReport) SafetyEvidenceGapRemediationMatrixVerificationReport {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyEvidenceGapRemediationMatrixVerificationReport
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
