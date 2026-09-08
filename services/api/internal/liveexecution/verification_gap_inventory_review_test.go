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

func TestSafetyEvidenceGapInventoryVerificationReviewIsDeterministicCanonicalAndFixed(t *testing.T) {
	now := time.Date(2026, 9, 8, 20, 0, 0, 123, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, audit, review := verifiedComparisonReviewChain(t, input, input)
	inventory := CompileSafetyEvidenceGapInventory(input, input, envelope, audit, review)
	report := BuildSafetyEvidenceGapInventoryVerificationReport(input, input, envelope, audit, review, inventory)

	first := BuildSafetyEvidenceGapInventoryVerificationReview(input, input, envelope, audit, review, inventory, report)
	second := BuildSafetyEvidenceGapInventoryVerificationReview(input, input, envelope, audit, review, inventory, report)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("verification review replay changed: %#v %#v", first, second)
	}
	if first.ArtifactVersion != SafetyEvidenceGapInventoryVerificationReviewVersion ||
		first.Status != SafetyEvidenceGapInventoryVerificationReviewReady ||
		first.Failure != SafetyEvidenceGapInventoryVerificationReviewFailureNone || first.ExecutionAuthority ||
		first.Disposition != SafetyEvidenceGapInventoryVerificationReviewNoGaps ||
		first.OwnerAction != SafetyEvidenceGapInventoryVerificationReviewOwnerActionNone ||
		first.VerificationReportStatus != VerificationVerified ||
		first.VerificationReportFailure != SafetyEvidenceGapInventoryVerificationFailureNone ||
		!first.CurrentEvidenceAvailable || first.ExpectedCategoryCount != 7 || first.CurrentGapCount != 0 ||
		len(first.Categories) != 7 || first.VerificationReportDigest != report.ReportDigest ||
		!reflect.DeepEqual(first.ReviewDigest, report.ReviewDigest) ||
		!reflect.DeepEqual(first.InventoryDigest, report.InventoryDigest) ||
		!digestPattern.MatchString(first.ArtifactDigest) {
		t.Fatalf("verification review is incomplete or authoritative: %#v", first)
	}
	if first.ArtifactDigest != "5ac311f43312dd2116e51f94034061e8809d1a74b07c02683a5aa20b089ded06" {
		t.Fatalf("fixed verification review changed: %s", first.ArtifactDigest)
	}
	if first.ArtifactDigest == first.VerificationReportDigest || first.ArtifactDigest == first.InventoryDigest.Claimed {
		t.Fatal("review artifact was not domain-separated")
	}
	if err := VerifySafetyEvidenceGapInventoryVerificationReview(input, input, envelope, audit, review, inventory, report, first); err != nil {
		t.Fatalf("exact verification review did not verify: %v", err)
	}
}

func TestSafetyEvidenceGapInventoryVerificationReviewMapsUnavailableEvidenceToOwnerReview(t *testing.T) {
	baselineTime := time.Date(2026, 9, 8, 20, 0, 0, 0, time.UTC)
	currentTime := baselineTime.Add(time.Minute)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(baselineTime), baselineTime)
	currentCase := validSafetyCase(currentTime)
	currentCase.OwnerAuthorization.MFAVerified = false
	current := verifiedSafetyCaseReport(t, currentCase, currentTime)
	envelope, audit, review := verifiedComparisonReviewChain(t, baseline, current)
	inventory := CompileSafetyEvidenceGapInventory(baseline, current, envelope, audit, review)
	report := BuildSafetyEvidenceGapInventoryVerificationReport(baseline, current, envelope, audit, review, inventory)
	artifact := BuildSafetyEvidenceGapInventoryVerificationReview(baseline, current, envelope, audit, review, inventory, report)

	if report.Status != VerificationRejected || report.Failure != SafetyEvidenceGapInventoryVerificationFailureUnavailable ||
		artifact.Status != SafetyEvidenceGapInventoryVerificationReviewReady ||
		artifact.Failure != SafetyEvidenceGapInventoryVerificationReviewFailureNone || artifact.ExecutionAuthority ||
		artifact.Disposition != SafetyEvidenceGapInventoryVerificationReviewOwnerReview ||
		artifact.OwnerAction != SafetyEvidenceGapInventoryVerificationReviewOwnerActionReviewGaps ||
		artifact.VerificationReportStatus != VerificationRejected ||
		artifact.VerificationReportFailure != SafetyEvidenceGapInventoryVerificationFailureUnavailable ||
		artifact.CurrentEvidenceAvailable || artifact.CurrentGapCount != 1 || !digestPattern.MatchString(artifact.ArtifactDigest) {
		t.Fatalf("unavailable evidence was not mapped to an exact owner review: %#v", artifact)
	}
	if err := VerifySafetyEvidenceGapInventoryVerificationReview(baseline, current, envelope, audit, review, inventory, report, artifact); err != nil {
		t.Fatalf("exact unavailable-evidence review did not verify: %v", err)
	}
}

func TestSafetyEvidenceGapInventoryVerificationReviewRejectsInvalidReportsExactly(t *testing.T) {
	now := time.Date(2026, 9, 8, 20, 0, 0, 0, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, audit, review := verifiedComparisonReviewChain(t, input, input)
	inventory := CompileSafetyEvidenceGapInventory(input, input, envelope, audit, review)
	original := BuildSafetyEvidenceGapInventoryVerificationReport(input, input, envelope, audit, review, inventory)
	tests := []struct {
		name    string
		failure SafetyEvidenceGapInventoryVerificationReviewFailure
		mutate  func(*SafetyEvidenceGapInventoryVerificationReport)
	}{
		{"version", SafetyEvidenceGapInventoryVerificationReviewFailureReportVersion, func(value *SafetyEvidenceGapInventoryVerificationReport) { value.ReportVersion = "report-v2" }},
		{"authority", SafetyEvidenceGapInventoryVerificationReviewFailureAuthority, func(value *SafetyEvidenceGapInventoryVerificationReport) { value.ExecutionAuthority = true }},
		{"status", SafetyEvidenceGapInventoryVerificationReviewFailureReportStatus, func(value *SafetyEvidenceGapInventoryVerificationReport) { value.CurrentEvidenceAvailable = false }},
		{"report digest", SafetyEvidenceGapInventoryVerificationReviewFailureReport, func(value *SafetyEvidenceGapInventoryVerificationReport) { value.ReportDigest = digestB }},
		{"digest evidence", SafetyEvidenceGapInventoryVerificationReviewFailureReport, func(value *SafetyEvidenceGapInventoryVerificationReport) { value.InventoryDigest.Verified = false }},
		{"count", SafetyEvidenceGapInventoryVerificationReviewFailureReport, func(value *SafetyEvidenceGapInventoryVerificationReport) { value.ClaimedCategoryCount++ }},
		{"category", SafetyEvidenceGapInventoryVerificationReviewFailureReport, func(value *SafetyEvidenceGapInventoryVerificationReport) { value.Categories[0].Verified = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := cloneGapInventoryVerificationReport(t, original)
			test.mutate(&report)
			artifact := BuildSafetyEvidenceGapInventoryVerificationReview(input, input, envelope, audit, review, inventory, report)
			if artifact.Status != SafetyEvidenceGapInventoryVerificationReviewRejected || artifact.Failure != test.failure ||
				artifact.ExecutionAuthority || artifact.ArtifactDigest != "" {
				t.Fatalf("invalid report was not rejected exactly: %#v", artifact)
			}
		})
	}
}

func TestSafetyEvidenceGapInventoryVerificationReviewRejectsSecretWithoutEcho(t *testing.T) {
	now := time.Date(2026, 9, 8, 20, 0, 0, 0, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, audit, review := verifiedComparisonReviewChain(t, input, input)
	inventory := CompileSafetyEvidenceGapInventory(input, input, envelope, audit, review)
	report := BuildSafetyEvidenceGapInventoryVerificationReport(input, input, envelope, audit, review, inventory)
	report.ReportVersion = "authorization_header_do_not_copy"
	artifact := BuildSafetyEvidenceGapInventoryVerificationReview(input, input, envelope, audit, review, inventory, report)
	if artifact.Status != SafetyEvidenceGapInventoryVerificationReviewRejected ||
		artifact.Failure != SafetyEvidenceGapInventoryVerificationReviewFailureSecretInput ||
		artifact.ExecutionAuthority || artifact.ArtifactDigest != "" || len(artifact.Categories) != 0 {
		t.Fatalf("secret-like input produced attributable review output: %#v", artifact)
	}
	payload, err := json.Marshal(artifact)
	if err != nil || strings.Contains(string(payload), "authorization_header_do_not_copy") {
		t.Fatalf("secret-like input was echoed: %s %v", payload, err)
	}
}

func TestSafetyEvidenceGapInventoryVerificationReviewRejectsEveryOutputTamper(t *testing.T) {
	now := time.Date(2026, 9, 8, 20, 0, 0, 0, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, audit, review := verifiedComparisonReviewChain(t, input, input)
	inventory := CompileSafetyEvidenceGapInventory(input, input, envelope, audit, review)
	report := BuildSafetyEvidenceGapInventoryVerificationReport(input, input, envelope, audit, review, inventory)
	original := BuildSafetyEvidenceGapInventoryVerificationReview(input, input, envelope, audit, review, inventory, report)
	mutations := []struct {
		name   string
		mutate func(*SafetyEvidenceGapInventoryVerificationReview)
	}{
		{"version", func(value *SafetyEvidenceGapInventoryVerificationReview) { value.ArtifactVersion = "artifact-v2" }},
		{"status", func(value *SafetyEvidenceGapInventoryVerificationReview) {
			value.Status = SafetyEvidenceGapInventoryVerificationReviewRejected
		}},
		{"failure", func(value *SafetyEvidenceGapInventoryVerificationReview) {
			value.Failure = SafetyEvidenceGapInventoryVerificationReviewFailureCount
		}},
		{"authority", func(value *SafetyEvidenceGapInventoryVerificationReview) { value.ExecutionAuthority = true }},
		{"disposition", func(value *SafetyEvidenceGapInventoryVerificationReview) {
			value.Disposition = SafetyEvidenceGapInventoryVerificationReviewOwnerReview
		}},
		{"owner action", func(value *SafetyEvidenceGapInventoryVerificationReview) {
			value.OwnerAction = SafetyEvidenceGapInventoryVerificationReviewOwnerActionReviewGaps
		}},
		{"report status", func(value *SafetyEvidenceGapInventoryVerificationReview) {
			value.VerificationReportStatus = VerificationRejected
		}},
		{"report failure", func(value *SafetyEvidenceGapInventoryVerificationReview) {
			value.VerificationReportFailure = SafetyEvidenceGapInventoryVerificationFailureUnavailable
		}},
		{"time", func(value *SafetyEvidenceGapInventoryVerificationReview) {
			value.CurrentEvaluatedAt = value.CurrentEvaluatedAt.Add(time.Nanosecond)
		}},
		{"availability", func(value *SafetyEvidenceGapInventoryVerificationReview) { value.CurrentEvidenceAvailable = false }},
		{"expected count", func(value *SafetyEvidenceGapInventoryVerificationReview) { value.ExpectedCategoryCount++ }},
		{"gap count", func(value *SafetyEvidenceGapInventoryVerificationReview) { value.CurrentGapCount++ }},
		{"change count", func(value *SafetyEvidenceGapInventoryVerificationReview) { value.NonUnchangedCategoryCount++ }},
		{"review digest", func(value *SafetyEvidenceGapInventoryVerificationReview) { value.ReviewDigest.Expected = digestB }},
		{"inventory digest", func(value *SafetyEvidenceGapInventoryVerificationReview) { value.InventoryDigest.Claimed = digestB }},
		{"category", func(value *SafetyEvidenceGapInventoryVerificationReview) { value.Categories[0].Verified = false }},
		{"category order", func(value *SafetyEvidenceGapInventoryVerificationReview) {
			value.Categories[0], value.Categories[1] = value.Categories[1], value.Categories[0]
		}},
		{"report digest", func(value *SafetyEvidenceGapInventoryVerificationReview) { value.VerificationReportDigest = digestB }},
		{"artifact digest", func(value *SafetyEvidenceGapInventoryVerificationReview) { value.ArtifactDigest = digestB }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			artifact := cloneGapInventoryVerificationReview(t, original)
			mutation.mutate(&artifact)
			if !errors.Is(VerifySafetyEvidenceGapInventoryVerificationReview(input, input, envelope, audit, review, inventory, report, artifact), ErrSafetyEvidenceGapInventoryVerificationReview) {
				t.Fatalf("tampered review verified: %#v", artifact)
			}
		})
	}
}

func TestSafetyEvidenceGapInventoryVerificationReviewBindsEveryInput(t *testing.T) {
	now := time.Date(2026, 9, 8, 20, 0, 0, 0, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, audit, review := verifiedComparisonReviewChain(t, input, input)
	inventory := CompileSafetyEvidenceGapInventory(input, input, envelope, audit, review)
	report := BuildSafetyEvidenceGapInventoryVerificationReport(input, input, envelope, audit, review, inventory)
	artifact := BuildSafetyEvidenceGapInventoryVerificationReview(input, input, envelope, audit, review, inventory, report)

	changedInput := cloneSafetyCaseReport(t, input)
	changedInput.EvaluatedAt = changedInput.EvaluatedAt.Add(time.Nanosecond)
	if !errors.Is(VerifySafetyEvidenceGapInventoryVerificationReview(changedInput, input, envelope, audit, review, inventory, report, artifact), ErrSafetyEvidenceGapInventoryVerificationReview) {
		t.Fatal("changed baseline verified against prior review")
	}
	changedInventory := cloneSafetyEvidenceGapInventory(t, inventory)
	changedInventory.InventoryDigest = digestB
	if !errors.Is(VerifySafetyEvidenceGapInventoryVerificationReview(input, input, envelope, audit, review, changedInventory, report, artifact), ErrSafetyEvidenceGapInventoryVerificationReview) {
		t.Fatal("changed inventory verified against prior review")
	}
	changedReport := cloneGapInventoryVerificationReport(t, report)
	changedReport.ReportDigest = digestB
	if !errors.Is(VerifySafetyEvidenceGapInventoryVerificationReview(input, input, envelope, audit, review, inventory, changedReport, artifact), ErrSafetyEvidenceGapInventoryVerificationReview) {
		t.Fatal("changed report verified against prior review")
	}
}

func TestSafetyEvidenceGapInventoryVerificationReviewHasNoRuntimeSurfaceOrProductionCaller(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("gap verification review test path unavailable")
	}
	directory := filepath.Dir(filename)
	source, err := os.ReadFile(filepath.Join(directory, "verification_gap_inventory_review.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"context.Context", "NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient", "SubmitOrder", "CancelOrder",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("verification review unexpectedly contains runtime surface %q", prohibited)
		}
	}
	apiRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	for _, symbol := range []string{"BuildSafetyEvidenceGapInventoryVerificationReview(", "VerifySafetyEvidenceGapInventoryVerificationReview("} {
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
				t.Fatalf("verification review has a production caller in %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func cloneGapInventoryVerificationReview(t *testing.T, value SafetyEvidenceGapInventoryVerificationReview) SafetyEvidenceGapInventoryVerificationReview {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyEvidenceGapInventoryVerificationReview
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
