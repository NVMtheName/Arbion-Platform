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

func TestSafetyEvidenceGapRemediationMatrixVerificationReviewIsDeterministicCanonicalAndFixed(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 23, 30, 0, 123, time.UTC), nil)
	matrix := compileRemediationMatrix(chain)
	report := buildRemediationMatrixVerificationReport(chain, matrix)
	first := buildRemediationMatrixVerificationReview(chain, matrix, report)
	second := buildRemediationMatrixVerificationReview(chain, matrix, report)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("matrix verification review replay changed: %#v %#v", first, second)
	}
	if first.ArtifactVersion != SafetyEvidenceGapRemediationMatrixVerificationReviewVersion ||
		first.Status != SafetyEvidenceGapRemediationMatrixVerificationReviewReady ||
		first.Failure != SafetyEvidenceGapRemediationMatrixVerificationReviewFailureNone || first.ExecutionAuthority ||
		first.Disposition != SafetyEvidenceGapRemediationMatrixVerificationReviewNoRemediation ||
		first.OwnerAction != SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionNone ||
		first.VerificationReportStatus != VerificationVerified ||
		first.VerificationReportFailure != SafetyEvidenceGapRemediationMatrixVerificationFailureNone ||
		!first.CurrentEvidenceAvailable || first.ExpectedCategoryCount != 7 || first.CurrentGapCount != 0 ||
		first.RequirementCount != 0 || len(first.Requirements) != 0 ||
		first.VerificationReportDigest != report.ReportDigest || !reflect.DeepEqual(first.MatrixDigest, report.MatrixDigest) ||
		!digestPattern.MatchString(first.ArtifactDigest) {
		t.Fatalf("matrix verification review is incomplete or authoritative: %#v", first)
	}
	if first.ArtifactDigest != "03dd20f229f06ee04f667d03a3c68e241b36662d34e7b0e07ab1028c624c1cf2" {
		t.Fatalf("fixed matrix verification review changed: %s", first.ArtifactDigest)
	}
	if first.ArtifactDigest == first.VerificationReportDigest || first.ArtifactDigest == first.MatrixDigest.Claimed {
		t.Fatal("matrix verification review digest was not domain-separated")
	}
	if err := verifyRemediationMatrixVerificationReview(chain, matrix, report, first); err != nil {
		t.Fatalf("exact matrix verification review did not verify: %v", err)
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReviewRequiresExactOwnerReview(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 23, 35, 0, 0, time.UTC), func(value *SafetyCase) {
		value.OwnerAuthorization.MFAVerified = false
	})
	matrix := compileRemediationMatrix(chain)
	report := buildRemediationMatrixVerificationReport(chain, matrix)
	artifact := buildRemediationMatrixVerificationReview(chain, matrix, report)
	if artifact.Status != SafetyEvidenceGapRemediationMatrixVerificationReviewReady ||
		artifact.Failure != SafetyEvidenceGapRemediationMatrixVerificationReviewFailureNone || artifact.ExecutionAuthority ||
		artifact.Disposition != SafetyEvidenceGapRemediationMatrixVerificationReviewRequired ||
		artifact.OwnerAction != SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionReview ||
		artifact.CurrentEvidenceAvailable || artifact.CurrentGapCount != 1 || artifact.RequirementCount != 1 ||
		len(artifact.Requirements) != 1 || artifact.Requirements[0].Category != "OWNER_AUTHORIZATION" ||
		artifact.Requirements[0].ResponsibleBoundary != SafetyEvidenceGapBoundaryOwnerAuthorization ||
		artifact.Requirements[0].SafeFollowUp != SafetyEvidenceGapFollowUpRecordAuthorization {
		t.Fatalf("exact owner review was not preserved: %#v", artifact)
	}
	if err := verifyRemediationMatrixVerificationReview(chain, matrix, report, artifact); err != nil {
		t.Fatalf("exact owner-review artifact did not verify: %v", err)
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReviewRejectsSecretWithoutEcho(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 23, 40, 0, 0, time.UTC), nil)
	matrix := compileRemediationMatrix(chain)
	report := buildRemediationMatrixVerificationReport(chain, matrix)
	report.ReportVersion = "client_secret_do_not_copy"
	artifact := buildRemediationMatrixVerificationReview(chain, matrix, report)
	if artifact.Status != SafetyEvidenceGapRemediationMatrixVerificationReviewRejected ||
		artifact.Failure != SafetyEvidenceGapRemediationMatrixVerificationReviewFailureSecret ||
		artifact.ExecutionAuthority || artifact.ArtifactDigest != "" || len(artifact.Requirements) != 0 {
		t.Fatalf("secret-like input produced attributable output: %#v", artifact)
	}
	payload, err := json.Marshal(artifact)
	if err != nil || strings.Contains(string(payload), "client_secret_do_not_copy") {
		t.Fatalf("secret-like input was echoed: %s %v", payload, err)
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReviewRejectsEveryOutputTamper(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 23, 45, 0, 0, time.UTC), func(value *SafetyCase) {
		value.ProviderCapability.TradeAllowed = false
	})
	matrix := compileRemediationMatrix(chain)
	report := buildRemediationMatrixVerificationReport(chain, matrix)
	original := buildRemediationMatrixVerificationReview(chain, matrix, report)
	mutations := []struct {
		name   string
		mutate func(*SafetyEvidenceGapRemediationMatrixVerificationReview)
	}{
		{"version", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.ArtifactVersion = "artifact-v2"
		}},
		{"status", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.Status = SafetyEvidenceGapRemediationMatrixVerificationReviewRejected
		}},
		{"failure", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.Failure = SafetyEvidenceGapRemediationMatrixVerificationReviewFailureEvidence
		}},
		{"authority", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) { value.ExecutionAuthority = true }},
		{"disposition", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.Disposition = SafetyEvidenceGapRemediationMatrixVerificationReviewNoRemediation
		}},
		{"owner action", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.OwnerAction = SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionNone
		}},
		{"report status", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.VerificationReportStatus = VerificationRejected
		}},
		{"report failure", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.VerificationReportFailure = SafetyEvidenceGapRemediationMatrixVerificationFailureCount
		}},
		{"baseline time", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.BaselineEvaluatedAt = value.BaselineEvaluatedAt.Add(time.Nanosecond)
		}},
		{"current time", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.CurrentEvaluatedAt = value.CurrentEvaluatedAt.Add(time.Nanosecond)
		}},
		{"availability", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.CurrentEvidenceAvailable = true
		}},
		{"category count", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) { value.ExpectedCategoryCount++ }},
		{"gap count", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) { value.CurrentGapCount++ }},
		{"requirement count", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) { value.RequirementCount++ }},
		{"report digest", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.VerificationReportDigest = digestB
		}},
		{"matrix digest", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.MatrixDigest.Claimed = digestB
		}},
		{"requirement", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) {
			value.Requirements[0].Category += "_CHANGED"
		}},
		{"artifact digest", func(value *SafetyEvidenceGapRemediationMatrixVerificationReview) { value.ArtifactDigest = digestB }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			artifact := cloneRemediationMatrixVerificationReview(t, original)
			mutation.mutate(&artifact)
			if !errors.Is(verifyRemediationMatrixVerificationReview(chain, matrix, report, artifact), ErrSafetyEvidenceGapRemediationMatrixVerificationReview) {
				t.Fatalf("tampered owner review verified: %#v", artifact)
			}
		})
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReviewBindsEveryInput(t *testing.T) {
	chain := buildRemediationMatrixChain(t, time.Date(2026, 9, 8, 23, 50, 0, 0, time.UTC), nil)
	matrix := compileRemediationMatrix(chain)
	report := buildRemediationMatrixVerificationReport(chain, matrix)
	artifact := buildRemediationMatrixVerificationReview(chain, matrix, report)
	tests := []struct {
		name   string
		mutate func(*remediationMatrixChain, *SafetyEvidenceGapRemediationMatrix, *SafetyEvidenceGapRemediationMatrixVerificationReport)
	}{
		{"baseline", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.baseline.EvaluatedAt = value.baseline.EvaluatedAt.Add(time.Nanosecond)
		}},
		{"current", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.current.EvaluatedAt = value.current.EvaluatedAt.Add(time.Nanosecond)
		}},
		{"envelope", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.envelope.EnvelopeDigest = digestB
		}},
		{"audit", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.audit.Digests.Envelope.Verified = false
		}},
		{"comparison review", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.review.ReviewDigest = digestB
		}},
		{"inventory", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.inventory.InventoryDigest = digestB
		}},
		{"inventory report", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.report.ReportDigest = digestB
		}},
		{"inventory review", func(value *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.verificationReview.ArtifactDigest = digestB
		}},
		{"matrix", func(_ *remediationMatrixChain, value *SafetyEvidenceGapRemediationMatrix, _ *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.MatrixDigest = digestB
		}},
		{"matrix report", func(_ *remediationMatrixChain, _ *SafetyEvidenceGapRemediationMatrix, value *SafetyEvidenceGapRemediationMatrixVerificationReport) {
			value.ReportDigest = digestB
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changedChain := cloneRemediationMatrixChain(t, chain)
			changedMatrix := cloneRemediationMatrix(t, matrix)
			changedReport := cloneRemediationMatrixVerificationReport(t, report)
			test.mutate(&changedChain, &changedMatrix, &changedReport)
			if !errors.Is(verifyRemediationMatrixVerificationReview(changedChain, changedMatrix, changedReport, artifact), ErrSafetyEvidenceGapRemediationMatrixVerificationReview) {
				t.Fatalf("changed %s verified against prior owner review", test.name)
			}
		})
	}
}

func TestSafetyEvidenceGapRemediationMatrixVerificationReviewHasNoRuntimeSurfaceOrProductionCaller(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("matrix verification review test path unavailable")
	}
	directory := filepath.Dir(filename)
	source, err := os.ReadFile(filepath.Join(directory, "verification_gap_remediation_matrix_review.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"context.Context", "NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient", "SubmitOrder", "CancelOrder",
		"RunCycle", "Execute", "Refresh", "Reconnect", "Sync", "PlaceOrder",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("matrix verification review unexpectedly contains runtime surface %q", prohibited)
		}
	}
	apiRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	for _, symbol := range []string{"BuildSafetyEvidenceGapRemediationMatrixVerificationReview(", "VerifySafetyEvidenceGapRemediationMatrixVerificationReview("} {
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
				t.Fatalf("matrix verification review has a production caller in %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func buildRemediationMatrixVerificationReview(
	chain remediationMatrixChain,
	matrix SafetyEvidenceGapRemediationMatrix,
	report SafetyEvidenceGapRemediationMatrixVerificationReport,
) SafetyEvidenceGapRemediationMatrixVerificationReview {
	return BuildSafetyEvidenceGapRemediationMatrixVerificationReview(
		chain.baseline, chain.current, chain.envelope, chain.audit, chain.review, chain.inventory,
		chain.report, chain.verificationReview, matrix, report,
	)
}

func verifyRemediationMatrixVerificationReview(
	chain remediationMatrixChain,
	matrix SafetyEvidenceGapRemediationMatrix,
	report SafetyEvidenceGapRemediationMatrixVerificationReport,
	artifact SafetyEvidenceGapRemediationMatrixVerificationReview,
) error {
	return VerifySafetyEvidenceGapRemediationMatrixVerificationReview(
		chain.baseline, chain.current, chain.envelope, chain.audit, chain.review, chain.inventory,
		chain.report, chain.verificationReview, matrix, report, artifact,
	)
}

func cloneRemediationMatrixVerificationReview(
	t *testing.T,
	value SafetyEvidenceGapRemediationMatrixVerificationReview,
) SafetyEvidenceGapRemediationMatrixVerificationReview {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyEvidenceGapRemediationMatrixVerificationReview
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
