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

func TestSafetyEvidenceGapRemediationProgressVerificationReportIsIndependentDeterministicAndFixed(t *testing.T) {
	previous := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 14, 0, 0, 123, time.UTC), nil))
	current := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 15, 0, 0, 456, time.UTC), nil))
	observedAt := time.Date(2026, 9, 9, 15, 2, 0, 0, time.UTC)
	comparison := BuildSafetyEvidenceGapRemediationProgressComparison(previous, current, observedAt)
	first := BuildSafetyEvidenceGapRemediationProgressVerificationReport(previous, current, observedAt, comparison)
	second := BuildSafetyEvidenceGapRemediationProgressVerificationReport(previous, current, observedAt, comparison)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("progress verification replay changed: %#v %#v", first, second)
	}
	if first.ReportVersion != SafetyEvidenceGapRemediationProgressVerificationReportVersion ||
		first.Status != VerificationVerified || first.Failure != SafetyEvidenceGapRemediationProgressVerificationFailureNone ||
		first.ExecutionAuthority || !first.ComparisonVersion.Verified || !first.ObservedAt.Equal(observedAt) ||
		first.ExpectedCategoryCount != 7 || first.ClaimedUnchangedCount != 7 || first.RecomputedUnchangedCount != 7 ||
		first.ClaimedNewlyRequiredCount != 0 || first.RecomputedNewlyRequiredCount != 0 ||
		first.ClaimedResolvedCount != 0 || first.RecomputedResolvedCount != 0 || len(first.Requirements) != 7 ||
		!first.PreviousReportDigest.Verified || !first.CurrentReportDigest.Verified ||
		!first.PreviousArtifactDigest.Verified || !first.CurrentArtifactDigest.Verified ||
		!first.PreviousMatrixDigest.Verified || !first.CurrentMatrixDigest.Verified ||
		!first.ComparisonDigest.Verified || !digestPattern.MatchString(first.ReportDigest) {
		t.Fatalf("progress verification report is incomplete or authoritative: %#v", first)
	}
	for index, requirement := range first.Requirements {
		if !requirement.Verified || !reflect.DeepEqual(requirement.Claimed, requirement.Recomputed) ||
			requirement.Recomputed.Change != SafetyEvidenceGapRemediationUnchanged ||
			requirement.Recomputed.Category != independentRemediationExpectations()[index].category {
			t.Fatalf("requirement %d was not independently verified: %#v", index, requirement)
		}
	}
	if first.ReportDigest != "e21abde88a4b910f9a0e1dc745c0bc2c6a8372494d870d1d8382fee60a316224" {
		t.Fatalf("fixed progress verification report changed: %s", first.ReportDigest)
	}
	if first.ReportDigest == comparison.ComparisonDigest || first.ReportDigest == first.CurrentReportDigest.Claimed {
		t.Fatal("progress verification report digest was not domain-separated")
	}
	if err := VerifySafetyEvidenceGapRemediationProgressVerificationReport(previous, current, observedAt, comparison, first); err != nil {
		t.Fatalf("exact progress verification report did not verify: %v", err)
	}
}

func TestSafetyEvidenceGapRemediationProgressVerificationReportPreservesEveryChangeState(t *testing.T) {
	previous := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 16, 0, 0, 0, time.UTC), func(value *SafetyCase) {
		value.ProviderCapability.TradeAllowed = false
		value.OwnerAuthorization.MFAVerified = false
	}))
	current := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 17, 0, 0, 0, time.UTC), func(value *SafetyCase) {
		value.OwnerAuthorization.MFAVerified = false
		value.PreTradeReconciliation.BlocksNewActions = true
	}))
	observedAt := time.Date(2026, 9, 9, 17, 2, 0, 0, time.UTC)
	comparison := BuildSafetyEvidenceGapRemediationProgressComparison(previous, current, observedAt)
	report := BuildSafetyEvidenceGapRemediationProgressVerificationReport(previous, current, observedAt, comparison)
	if report.Status != VerificationVerified || report.ExecutionAuthority ||
		report.RecomputedUnchangedCount != 5 || report.RecomputedNewlyRequiredCount != 1 || report.RecomputedResolvedCount != 1 {
		t.Fatalf("change classifications were not verified: %#v", report)
	}
	want := []SafetyEvidenceGapRemediationChange{
		SafetyEvidenceGapRemediationResolved,
		SafetyEvidenceGapRemediationUnchanged,
		SafetyEvidenceGapRemediationUnchanged,
		SafetyEvidenceGapRemediationNewlyRequired,
		SafetyEvidenceGapRemediationUnchanged,
		SafetyEvidenceGapRemediationUnchanged,
		SafetyEvidenceGapRemediationUnchanged,
	}
	for index, verification := range report.Requirements {
		if !verification.Verified || verification.Recomputed.Change != want[index] ||
			!reflect.DeepEqual(verification.Claimed, verification.Recomputed) {
			t.Fatalf("change %d was not exact: %#v", index, verification)
		}
	}
}

func TestSafetyEvidenceGapRemediationProgressVerificationReportClassifiesFailures(t *testing.T) {
	previous := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 18, 0, 0, 0, time.UTC), func(value *SafetyCase) {
		value.ProviderCapability.TradeAllowed = false
	}))
	current := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 19, 0, 0, 0, time.UTC), func(value *SafetyCase) {
		value.OwnerAuthorization.MFAVerified = false
	}))
	observedAt := time.Date(2026, 9, 9, 19, 2, 0, 0, time.UTC)
	original := BuildSafetyEvidenceGapRemediationProgressComparison(previous, current, observedAt)
	tests := []struct {
		name       string
		failure    SafetyEvidenceGapRemediationProgressVerificationFailure
		previous   SafetyEvidenceGapRemediationOwnerReviewChain
		current    SafetyEvidenceGapRemediationOwnerReviewChain
		observedAt time.Time
		mutate     func(*SafetyEvidenceGapRemediationProgressComparison)
	}{
		{"observed at", SafetyEvidenceGapRemediationProgressVerificationFailureObservedAt, previous, current, observedAt.In(time.FixedZone("offset", 3600)), nil},
		{"future evidence", SafetyEvidenceGapRemediationProgressVerificationFailureTimeOrder, previous, current, current.OwnerReviewReport.CurrentEvaluatedAt.Add(-time.Nanosecond), nil},
		{"previous chain", SafetyEvidenceGapRemediationProgressVerificationFailurePreviousChain, mutateProgressChain(t, previous, func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
			value.OwnerReviewReport.ReportDigest = digestB
		}), current, observedAt, nil},
		{"current chain", SafetyEvidenceGapRemediationProgressVerificationFailureCurrentChain, previous, mutateProgressChain(t, current, func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
			value.OwnerReviewReport.ReportDigest = digestB
		}), observedAt, nil},
		{"version", SafetyEvidenceGapRemediationProgressVerificationFailureComparisonVersion, previous, current, observedAt, func(value *SafetyEvidenceGapRemediationProgressComparison) { value.ComparisonVersion += "-changed" }},
		{"status", SafetyEvidenceGapRemediationProgressVerificationFailureComparisonStatus, previous, current, observedAt, func(value *SafetyEvidenceGapRemediationProgressComparison) { value.Status = VerificationRejected }},
		{"digest evidence", SafetyEvidenceGapRemediationProgressVerificationFailureDigestEvidence, previous, current, observedAt, func(value *SafetyEvidenceGapRemediationProgressComparison) { value.CurrentReportDigest = digestB }},
		{"count", SafetyEvidenceGapRemediationProgressVerificationFailureCount, previous, current, observedAt, func(value *SafetyEvidenceGapRemediationProgressComparison) { value.NewlyRequiredCount++ }},
		{"requirement", SafetyEvidenceGapRemediationProgressVerificationFailureRequirement, previous, current, observedAt, func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.Requirements[0].Change = SafetyEvidenceGapRemediationUnchanged
		}},
		{"requirement reordering", SafetyEvidenceGapRemediationProgressVerificationFailureRequirement, previous, current, observedAt, func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.Requirements[0], value.Requirements[1] = value.Requirements[1], value.Requirements[0]
		}},
		{"digest malformed", SafetyEvidenceGapRemediationProgressVerificationFailureDigestMalformed, previous, current, observedAt, func(value *SafetyEvidenceGapRemediationProgressComparison) { value.ComparisonDigest = "not-a-digest" }},
		{"digest mismatch", SafetyEvidenceGapRemediationProgressVerificationFailureDigestMismatch, previous, current, observedAt, func(value *SafetyEvidenceGapRemediationProgressComparison) { value.ComparisonDigest = digestB }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			comparison := cloneRemediationProgress(t, original)
			if test.mutate != nil {
				test.mutate(&comparison)
			}
			report := BuildSafetyEvidenceGapRemediationProgressVerificationReport(test.previous, test.current, test.observedAt, comparison)
			if report.Status != VerificationRejected || report.Failure != test.failure ||
				report.ExecutionAuthority || report.ReportDigest != "" {
				t.Fatalf("failure was not classified exactly: %#v", report)
			}
		})
	}

	other := completeRemediationProgressChain(buildRemediationMatrixChainForCases(t, time.Date(2026, 9, 9, 20, 0, 0, 0, time.UTC), func(value *SafetyCase) {
		value.Action.ID = "10000000-0000-4000-8000-000000000099"
	}))
	report := BuildSafetyEvidenceGapRemediationProgressVerificationReport(previous, other, time.Date(2026, 9, 9, 20, 2, 0, 0, time.UTC), original)
	if report.Failure != SafetyEvidenceGapRemediationProgressVerificationFailureComparable {
		t.Fatalf("noncomparable chains were not rejected: %#v", report)
	}
}

func TestSafetyEvidenceGapRemediationProgressVerificationReportRejectsSecretsAndAuthorityWithoutEcho(t *testing.T) {
	previous := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 21, 0, 0, 0, time.UTC), nil))
	current := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 22, 0, 0, 0, time.UTC), nil))
	observedAt := time.Date(2026, 9, 9, 22, 2, 0, 0, time.UTC)
	comparison := BuildSafetyEvidenceGapRemediationProgressComparison(previous, current, observedAt)
	secret := cloneRemediationProgress(t, comparison)
	secret.ComparisonVersion = "authorization_header_do_not_copy"
	report := BuildSafetyEvidenceGapRemediationProgressVerificationReport(previous, current, observedAt, secret)
	payload, err := json.Marshal(report)
	if err != nil || report.Failure != SafetyEvidenceGapRemediationProgressVerificationFailureSecret ||
		report.ReportDigest != "" || len(report.Requirements) != 0 ||
		strings.Contains(string(payload), "authorization_header_do_not_copy") {
		t.Fatalf("secret-like input was retained: %s %v", payload, err)
	}
	authoritative := cloneRemediationProgress(t, comparison)
	authoritative.ExecutionAuthority = true
	report = BuildSafetyEvidenceGapRemediationProgressVerificationReport(previous, current, observedAt, authoritative)
	if report.Failure != SafetyEvidenceGapRemediationProgressVerificationFailureAuthority || report.ExecutionAuthority {
		t.Fatalf("authority-bearing comparison was not rejected: %#v", report)
	}
}

func TestSafetyEvidenceGapRemediationProgressVerificationReportRejectsEveryOutputTamper(t *testing.T) {
	previous := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 23, 0, 0, 0, time.UTC), func(value *SafetyCase) { value.ProviderCapability.TradeAllowed = false }))
	current := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), func(value *SafetyCase) { value.OwnerAuthorization.MFAVerified = false }))
	observedAt := time.Date(2026, 9, 10, 0, 2, 0, 0, time.UTC)
	comparison := BuildSafetyEvidenceGapRemediationProgressComparison(previous, current, observedAt)
	original := BuildSafetyEvidenceGapRemediationProgressVerificationReport(previous, current, observedAt, comparison)
	mutations := []struct {
		name   string
		mutate func(*SafetyEvidenceGapRemediationProgressVerificationReport)
	}{
		{"version", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) { value.ReportVersion += "-changed" }},
		{"status", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.Status = VerificationRejected
		}},
		{"failure", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.Failure = SafetyEvidenceGapRemediationProgressVerificationFailureCount
		}},
		{"authority", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) { value.ExecutionAuthority = true }},
		{"comparison version", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.ComparisonVersion.Verified = false
		}},
		{"observed", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.ObservedAt = value.ObservedAt.Add(time.Nanosecond)
		}},
		{"previous baseline", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.PreviousBaselineAt = value.PreviousBaselineAt.Add(time.Nanosecond)
		}},
		{"previous current", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.PreviousCurrentAt = value.PreviousCurrentAt.Add(time.Nanosecond)
		}},
		{"current baseline", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.CurrentBaselineAt = value.CurrentBaselineAt.Add(time.Nanosecond)
		}},
		{"current current", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.CurrentCurrentAt = value.CurrentCurrentAt.Add(time.Nanosecond)
		}},
		{"previous report", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.PreviousReportDigest.Verified = false
		}},
		{"current report", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.CurrentReportDigest.Expected = digestB
		}},
		{"previous artifact", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.PreviousArtifactDigest.Recomputed = digestB
		}},
		{"current artifact", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.CurrentArtifactDigest.Claimed = digestB
		}},
		{"previous matrix", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.PreviousMatrixDigest.Verified = false
		}},
		{"current matrix", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.CurrentMatrixDigest.Expected = digestB
		}},
		{"category count", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) { value.ExpectedCategoryCount++ }},
		{"claimed unchanged", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) { value.ClaimedUnchangedCount++ }},
		{"recomputed unchanged", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) { value.RecomputedUnchangedCount++ }},
		{"claimed new", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) { value.ClaimedNewlyRequiredCount++ }},
		{"recomputed new", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.RecomputedNewlyRequiredCount++
		}},
		{"claimed resolved", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) { value.ClaimedResolvedCount++ }},
		{"recomputed resolved", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) { value.RecomputedResolvedCount++ }},
		{"requirement claimed", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.Requirements[0].Claimed.Category += "_CHANGED"
		}},
		{"requirement recomputed", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.Requirements[0].Recomputed.Category += "_CHANGED"
		}},
		{"requirement verified", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.Requirements[0].Verified = false
		}},
		{"requirement reordering", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.Requirements[0], value.Requirements[1] = value.Requirements[1], value.Requirements[0]
		}},
		{"comparison digest", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) {
			value.ComparisonDigest.Expected = digestB
		}},
		{"report digest", func(value *SafetyEvidenceGapRemediationProgressVerificationReport) { value.ReportDigest = digestB }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := cloneRemediationProgressVerificationReport(t, original)
			mutation.mutate(&changed)
			if !errors.Is(VerifySafetyEvidenceGapRemediationProgressVerificationReport(previous, current, observedAt, comparison, changed), ErrSafetyEvidenceGapRemediationProgressVerificationReport) {
				t.Fatalf("tampered progress verification report verified: %#v", changed)
			}
		})
	}
}

func TestSafetyEvidenceGapRemediationProgressVerificationReportBindsEveryInput(t *testing.T) {
	previous := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 10, 1, 0, 0, 0, time.UTC), nil))
	current := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 10, 2, 0, 0, 0, time.UTC), nil))
	observedAt := time.Date(2026, 9, 10, 2, 2, 0, 0, time.UTC)
	comparison := BuildSafetyEvidenceGapRemediationProgressComparison(previous, current, observedAt)
	report := BuildSafetyEvidenceGapRemediationProgressVerificationReport(previous, current, observedAt, comparison)
	mutations := []struct {
		name   string
		mutate func(*SafetyEvidenceGapRemediationOwnerReviewChain)
	}{
		{"baseline", func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
			value.BaselineReport.EvaluatedAt = value.BaselineReport.EvaluatedAt.Add(time.Nanosecond)
		}},
		{"current", func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
			value.CurrentReport.EvaluatedAt = value.CurrentReport.EvaluatedAt.Add(time.Nanosecond)
		}},
		{"envelope", func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
			value.ComparisonEnvelope.EnvelopeDigest = digestB
		}},
		{"comparison report", func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
			value.ComparisonReport.Digests.Envelope.Verified = false
		}},
		{"comparison review", func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
			value.ComparisonReview.ReviewDigest = digestB
		}},
		{"inventory", func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
			value.GapInventory.InventoryDigest = digestB
		}},
		{"inventory report", func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
			value.GapInventoryReport.ReportDigest = digestB
		}},
		{"inventory review", func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
			value.GapInventoryReview.ArtifactDigest = digestB
		}},
		{"matrix", func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
			value.RemediationMatrix.MatrixDigest = digestB
		}},
		{"matrix report", func(value *SafetyEvidenceGapRemediationOwnerReviewChain) { value.MatrixReport.ReportDigest = digestB }},
		{"owner review", func(value *SafetyEvidenceGapRemediationOwnerReviewChain) { value.OwnerReview.ArtifactDigest = digestB }},
		{"owner report", func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
			value.OwnerReviewReport.ReportDigest = digestB
		}},
	}
	for _, side := range []string{"previous", "current"} {
		for _, mutation := range mutations {
			t.Run(side+" "+mutation.name, func(t *testing.T) {
				changedPrevious := cloneRemediationProgressChain(t, previous)
				changedCurrent := cloneRemediationProgressChain(t, current)
				if side == "previous" {
					mutation.mutate(&changedPrevious)
				} else {
					mutation.mutate(&changedCurrent)
				}
				if !errors.Is(VerifySafetyEvidenceGapRemediationProgressVerificationReport(changedPrevious, changedCurrent, observedAt, comparison, report), ErrSafetyEvidenceGapRemediationProgressVerificationReport) {
					t.Fatalf("changed %s %s verified", side, mutation.name)
				}
			})
		}
	}
	if !errors.Is(VerifySafetyEvidenceGapRemediationProgressVerificationReport(previous, current, observedAt.Add(time.Nanosecond), comparison, report), ErrSafetyEvidenceGapRemediationProgressVerificationReport) {
		t.Fatal("changed observation time verified")
	}
	changedComparison := cloneRemediationProgress(t, comparison)
	changedComparison.ComparisonDigest = digestB
	if !errors.Is(VerifySafetyEvidenceGapRemediationProgressVerificationReport(previous, current, observedAt, changedComparison, report), ErrSafetyEvidenceGapRemediationProgressVerificationReport) {
		t.Fatal("changed comparison verified")
	}
}

func TestSafetyEvidenceGapRemediationProgressVerificationReportHasNoRuntimeSurface(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("progress verification source path unavailable")
	}
	directory := filepath.Dir(filename)
	source, err := os.ReadFile(filepath.Join(directory, "verification_gap_remediation_progress_report.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"context.Context", "NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient", "SubmitOrder", "CancelOrder",
		"RunCycle", "Execute", "Refresh", "Reconnect", "Sync", "PlaceOrder",
		"BuildSafetyEvidenceGapRemediationProgressComparison(",
		"VerifySafetyEvidenceGapRemediationProgressComparison(",
		"canonicalSafetyEvidenceGapRemediationProgressDigest(",
		"exactRemediationProgressRequirements(",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("progress verification report unexpectedly contains shared or runtime surface %q", prohibited)
		}
	}
	apiRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	for _, symbol := range []string{
		"BuildSafetyEvidenceGapRemediationProgressVerificationReport(",
		"VerifySafetyEvidenceGapRemediationProgressVerificationReport(",
	} {
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
				t.Fatalf("progress verification report has a production caller in %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func cloneRemediationProgressVerificationReport(
	t *testing.T,
	value SafetyEvidenceGapRemediationProgressVerificationReport,
) SafetyEvidenceGapRemediationProgressVerificationReport {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyEvidenceGapRemediationProgressVerificationReport
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
