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

func TestSafetyEvidenceGapRemediationMatrixVerificationReviewReportIsIndependentDeterministicAndFixed(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 0, 10, 0, 123, time.UTC), nil)
	matrix, matrixReport, artifact := completeRemediationOwnerReviewChain(chain)
	first := buildRemediationOwnerReviewReport(chain, matrix, matrixReport, artifact)
	second := buildRemediationOwnerReviewReport(chain, matrix, matrixReport, artifact)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("owner-review report replay changed: %#v %#v", first, second)
	}
	if first.ReportVersion != SafetyEvidenceGapRemediationMatrixVerificationReviewReportVersion ||
		first.Status != VerificationVerified ||
		first.Failure != SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureNone || first.ExecutionAuthority ||
		!first.ArtifactVersion.Verified ||
		first.ClaimedDisposition != SafetyEvidenceGapRemediationMatrixVerificationReviewNoRemediation ||
		first.RecomputedDisposition != SafetyEvidenceGapRemediationMatrixVerificationReviewNoRemediation ||
		first.ClaimedOwnerAction != SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionNone ||
		first.RecomputedOwnerAction != SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionNone ||
		!first.CurrentEvidenceAvailable || first.ExpectedCategoryCount != 7 ||
		first.ClaimedCurrentGapCount != 0 || first.RecomputedCurrentGapCount != 0 ||
		first.ClaimedRequirementCount != 0 || first.RecomputedRequirementCount != 0 || len(first.Requirements) != 0 ||
		!first.VerificationReportDigest.Verified || !first.MatrixDigest.Verified || !first.ArtifactDigest.Verified ||
		!digestPattern.MatchString(first.ReportDigest) {
		t.Fatalf("owner-review report is incomplete or authoritative: %#v", first)
	}
	if first.ReportDigest != "3e7ff185a8f9610fd4fe68e7f4da6c316068ae6f7cadbefba36dc97c46a52d4c" {
		t.Fatalf("fixed owner-review report changed: %s", first.ReportDigest)
	}
	if first.ReportDigest == first.ArtifactDigest.Claimed || first.ReportDigest == first.VerificationReportDigest.Claimed {
		t.Fatal("owner-review report digest was not domain-separated")
	}
	if err := verifyRemediationOwnerReviewReport(chain, matrix, matrixReport, artifact, first); err != nil {
		t.Fatalf("exact owner-review report did not verify: %v", err)
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReviewReportPreservesAllExactReviewRequirements(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 0, 15, 0, 0, time.UTC), func(value *SafetyCase) {
		value.ProviderCapability.TradeAllowed = false
		value.OwnerAuthorization.MFAVerified = false
		value.Risk.PlatformExecutionAvailable = false
		value.PreTradeReconciliation.BlocksNewActions = true
		value.KillSwitch.BrokerEnforcementReady = false
		value.Idempotency.ReplayDetected = true
		value.Lifecycle.RequirePostTradeReconciliation = false
	})
	matrix, matrixReport, artifact := completeRemediationOwnerReviewChain(chain)
	report := buildRemediationOwnerReviewReport(chain, matrix, matrixReport, artifact)
	if report.Status != VerificationVerified || report.Failure != SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureNone ||
		report.ExecutionAuthority || report.CurrentEvidenceAvailable ||
		report.ClaimedDisposition != SafetyEvidenceGapRemediationMatrixVerificationReviewRequired ||
		report.RecomputedDisposition != SafetyEvidenceGapRemediationMatrixVerificationReviewRequired ||
		report.ClaimedOwnerAction != SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionReview ||
		report.RecomputedOwnerAction != SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionReview ||
		report.ClaimedCurrentGapCount != 7 || report.RecomputedCurrentGapCount != 7 ||
		report.ClaimedRequirementCount != 7 || report.RecomputedRequirementCount != 7 || len(report.Requirements) != 7 {
		t.Fatalf("all exact owner-review requirements were not preserved: %#v", report)
	}
	for index, requirement := range report.Requirements {
		if !requirement.Verified || !reflect.DeepEqual(requirement.Claimed, requirement.Recomputed) ||
			!reflect.DeepEqual(requirement.Recomputed, matrix.Requirements[index]) {
			t.Fatalf("requirement %d was not independently verified: %#v", index, requirement)
		}
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReviewReportClassifiesArtifactFailures(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 0, 20, 0, 0, time.UTC), func(value *SafetyCase) {
		value.OwnerAuthorization.MFAVerified = false
	})
	matrix, matrixReport, original := completeRemediationOwnerReviewChain(chain)
	tests := []struct {
		name    string
		failure SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure
		mutate  func(*SafetyEvidenceGapRemediationMatrixVerificationReview)
	}{
		{"version", SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureArtifactVersion, func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.ArtifactVersion = "artifact-v2"
		}},
		{"status", SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureArtifactStatus, func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.Status = SafetyEvidenceGapRemediationMatrixVerificationReviewRejected
		}},
		{"authority", SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureAuthority, func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) { value.ExecutionAuthority = true }},
		{"disposition", SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureConclusion, func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.Disposition = SafetyEvidenceGapRemediationMatrixVerificationReviewNoRemediation
		}},
		{"count", SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureCount, func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) { value.CurrentGapCount++ }},
		{"requirement", SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureRequirement, func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.Requirements[0].Category += "_CHANGED"
		}},
		{"report digest", SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureReportDigest, func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.VerificationReportDigest = digestB
		}},
		{"matrix digest", SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureMatrixDigest, func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.MatrixDigest.Claimed = digestB
		}},
		{"digest malformed", SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureDigestMalformed, func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.ArtifactDigest = "not-a-digest"
		}},
		{"digest mismatch", SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureDigestMismatch, func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) { value.ArtifactDigest = digestB }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifact := cloneRemediationMatrixVerificationReview(t, original)
			test.mutate(&artifact)
			report := buildRemediationOwnerReviewReport(chain, matrix, matrixReport, artifact)
			if report.Status != VerificationRejected || report.Failure != test.failure ||
				report.ExecutionAuthority || report.ReportDigest != "" {
				t.Fatalf("artifact failure was not classified exactly: %#v", report)
			}
		})
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReviewReportRejectsSecretWithoutEcho(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 0, 25, 0, 0, time.UTC), nil)
	matrix, matrixReport, artifact := completeRemediationOwnerReviewChain(chain)
	artifact.ArtifactVersion = "authorization_header_do_not_copy"
	report := buildRemediationOwnerReviewReport(chain, matrix, matrixReport, artifact)
	if report.Status != VerificationRejected ||
		report.Failure != SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureSecret ||
		report.ExecutionAuthority || report.ReportDigest != "" || len(report.Requirements) != 0 {
		t.Fatalf("secret-like input produced attributable output: %#v", report)
	}
	payload, err := json.Marshal(report)
	if err != nil || strings.Contains(string(payload), "authorization_header_do_not_copy") {
		t.Fatalf("secret-like input was echoed: %s %v", payload, err)
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReviewReportRejectsEveryOutputTamper(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 0, 30, 0, 0, time.UTC), func(value *SafetyCase) {
		value.ProviderCapability.TradeAllowed = false
	})
	matrix, matrixReport, artifact := completeRemediationOwnerReviewChain(chain)
	original := buildRemediationOwnerReviewReport(chain, matrix, matrixReport, artifact)
	mutations := []struct {
		name   string
		mutate func(*SafetyEvidenceGapRemediationMatrixVerificationReviewReport)
	}{
		{"version", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.ReportVersion = "report-v2"
		}},
		{"status", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.Status = VerificationRejected
		}},
		{"failure", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.Failure = SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureCount
		}},
		{"authority", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.ExecutionAuthority = true
		}},
		{"artifact version", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.ArtifactVersion.Verified = false
		}},
		{"claimed disposition", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.ClaimedDisposition = SafetyEvidenceGapRemediationMatrixVerificationReviewNoRemediation
		}},
		{"recomputed disposition", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.RecomputedDisposition = SafetyEvidenceGapRemediationMatrixVerificationReviewNoRemediation
		}},
		{"claimed action", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.ClaimedOwnerAction = SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionNone
		}},
		{"recomputed action", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.RecomputedOwnerAction = SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionNone
		}},
		{"baseline time", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.BaselineEvaluatedAt = value.BaselineEvaluatedAt.Add(time.Nanosecond)
		}},
		{"current time", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.CurrentEvaluatedAt = value.CurrentEvaluatedAt.Add(time.Nanosecond)
		}},
		{"availability", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.CurrentEvidenceAvailable = true
		}},
		{"category count", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) { value.ExpectedCategoryCount++ }},
		{"claimed gaps", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.ClaimedCurrentGapCount++
		}},
		{"recomputed gaps", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.RecomputedCurrentGapCount++
		}},
		{"claimed requirements", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.ClaimedRequirementCount++
		}},
		{"recomputed requirements", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.RecomputedRequirementCount++
		}},
		{"report digest evidence", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.VerificationReportDigest.Verified = false
		}},
		{"matrix digest evidence", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.MatrixDigest.Expected = digestB
		}},
		{"artifact digest evidence", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.ArtifactDigest.Recomputed = digestB
		}},
		{"requirement claimed", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.Requirements[0].Claimed.Category += "_CHANGED"
		}},
		{"requirement recomputed", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.Requirements[0].Recomputed.Category += "_CHANGED"
		}},
		{"requirement verified", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
			value.Requirements[0].Verified = false
		}},
		{"report digest", func(value *SafetyEvidenceGapRemediationMatrixVerificationReviewReport) { value.ReportDigest = digestB }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			report := cloneRemediationOwnerReviewReport(t, original)
			mutation.mutate(&report)
			if !errors.Is(verifyRemediationOwnerReviewReport(chain, matrix, matrixReport, artifact, report), ErrSafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
				t.Fatalf("tampered report verified: %#v", report)
			}
		})
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReviewReportBindsEveryInput(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 0, 35, 0, 0, time.UTC), nil)
	matrix, matrixReport, artifact := completeRemediationOwnerReviewChain(chain)
	report := buildRemediationOwnerReviewReport(chain, matrix, matrixReport, artifact)
	tests := []struct {
		name   string
		mutate func(*remediationMatrixChain, *SafetyEvidenceGapRemediationMatrix, *SafetyEvidenceGapRemediationMatrixVerificationReport, *SafetyEvidenceGapRemediationMatrixVerificationReview)
	}{
		{"baseline", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport, _ *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.baseline.EvaluatedAt = value.baseline.EvaluatedAt.Add(time.Nanosecond)
		}},
		{"current", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport, _ *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.current.EvaluatedAt = value.current.EvaluatedAt.Add(time.Nanosecond)
		}},
		{"envelope", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport, _ *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.envelope.EnvelopeDigest = digestB
		}},
		{"audit", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport, _ *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.audit.Digests.Envelope.Verified = false
		}},
		{"comparison review", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport, _ *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.review.ReviewDigest = digestB
		}},
		{"inventory", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport, _ *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.inventory.InventoryDigest = digestB
		}},
		{"inventory report", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport, _ *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.report.ReportDigest = digestB
		}},
		{"inventory review", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport, _ *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.verificationReview.ArtifactDigest = digestB
		}},
		{"matrix", func(_ *remediationMatrixChain, value *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport, _ *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.MatrixDigest = digestB
		}},
		{"matrix report", func(_ *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, value *SafetyEvidenceGapRemediationMatrixVerificationReport, _ *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.ReportDigest = digestB
		}},
		{"owner review", func(_ *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport, value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.ArtifactDigest = digestB
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changedChain := cloneRemediationMatrixChain(t, chain)
			changedMatrix := cloneRemediationMatrix(t, matrix)
			changedMatrixReport := cloneRemediationMatrixVerificationReport(t, matrixReport)
			changedArtifact := cloneRemediationMatrixVerificationReview(t, artifact)
			test.mutate(&changedChain, &changedMatrix, &changedMatrixReport, &changedArtifact)
			if !errors.Is(verifyRemediationOwnerReviewReport(changedChain, changedMatrix, changedMatrixReport, changedArtifact, report), ErrSafetyEvidenceGapRemediationMatrixVerificationReviewReport) {
				t.Fatalf("changed %s verified against prior report", test.name)
			}
		})
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReviewReportIsIndependentAndHasNoRuntimeSurface(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("owner-review report test path unavailable")
	}
	directory := filepath.Dir(filename)
	source, err := os.ReadFile(filepath.Join(directory, "verification_gap_remediation_matrix_review_report.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"context.Context", "NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient", "SubmitOrder", "CancelOrder",
		"RunCycle", "Execute", "Refresh", "Reconnect", "Sync", "PlaceOrder",
		"BuildSafetyEvidenceGapRemediationMatrixVerificationReview(",
		"VerifySafetyEvidenceGapRemediationMatrixVerificationReview(",
		"remediationMatrixVerificationReviewConclusion(",
		"canonicalSafetyEvidenceGapRemediationMatrixVerificationReviewDigest(",
		"exactMatrixDigestVerification(",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("owner-review report unexpectedly contains shared or runtime surface %q", prohibited)
		}
	}
	apiRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	for _, symbol := range []string{"BuildSafetyEvidenceGapRemediationMatrixVerificationReviewReport(", "VerifySafetyEvidenceGapRemediationMatrixVerificationReviewReport("} {
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
				t.Fatalf("owner-review report has a production caller in %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func completeRemediationOwnerReviewChain(chain remediationMatrixChain) (
	SafetyEvidenceGapRemediationMatrix,
	SafetyEvidenceGapRemediationMatrixVerificationReport,
	SafetyEvidenceGapRemediationMatrixVerificationReview,
) {
	matrix := compileRemediationMatrix(chain)
	matrixReport := buildRemediationMatrixVerificationReport(chain, matrix)
	artifact := buildRemediationMatrixVerificationReview(chain, matrix, matrixReport)
	return matrix, matrixReport, artifact
}

func buildRemediationOwnerReviewReport(
	chain remediationMatrixChain,
	matrix SafetyEvidenceGapRemediationMatrix,
	matrixReport SafetyEvidenceGapRemediationMatrixVerificationReport,
	artifact SafetyEvidenceGapRemediationMatrixVerificationReview,
) SafetyEvidenceGapRemediationMatrixVerificationReviewReport {
	return BuildSafetyEvidenceGapRemediationMatrixVerificationReviewReport(
		chain.baseline, chain.current, chain.envelope, chain.audit, chain.review, chain.inventory,
		chain.report, chain.verificationReview, matrix, matrixReport, artifact,
	)
}

func verifyRemediationOwnerReviewReport(
	chain remediationMatrixChain,
	matrix SafetyEvidenceGapRemediationMatrix,
	matrixReport SafetyEvidenceGapRemediationMatrixVerificationReport,
	artifact SafetyEvidenceGapRemediationMatrixVerificationReview,
	report SafetyEvidenceGapRemediationMatrixVerificationReviewReport,
) error {
	return VerifySafetyEvidenceGapRemediationMatrixVerificationReviewReport(
		chain.baseline, chain.current, chain.envelope, chain.audit, chain.review, chain.inventory,
		chain.report, chain.verificationReview, matrix, matrixReport, artifact, report,
	)
}

func cloneRemediationOwnerReviewReport(
	t *testing.T,
	value SafetyEvidenceGapRemediationMatrixVerificationReviewReport,
) SafetyEvidenceGapRemediationMatrixVerificationReviewReport {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyEvidenceGapRemediationMatrixVerificationReviewReport
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
