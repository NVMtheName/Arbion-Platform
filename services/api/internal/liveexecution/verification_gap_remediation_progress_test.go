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

func TestSafetyEvidenceGapRemediationProgressIsDeterministicCanonicalAndFixed(t *testing.T) {
	previous := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 1, 0, 0, 123, time.UTC), nil))
	current := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 2, 0, 0, 456, time.UTC), nil))
	observedAt := time.Date(2026, 9, 9, 2, 2, 0, 0, time.UTC)
	first := BuildSafetyEvidenceGapRemediationProgressComparison(previous, current, observedAt)
	second := BuildSafetyEvidenceGapRemediationProgressComparison(previous, current, observedAt)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("remediation progress replay changed: %#v %#v", first, second)
	}
	if first.ComparisonVersion != SafetyEvidenceGapRemediationProgressVersion ||
		first.Status != VerificationVerified || first.Failure != SafetyEvidenceGapRemediationProgressFailureNone ||
		first.ExecutionAuthority || !first.ObservedAt.Equal(observedAt) || first.ExpectedCategoryCount != 7 ||
		first.UnchangedCount != 7 || first.NewlyRequiredCount != 0 || first.ResolvedCount != 0 ||
		len(first.Requirements) != 7 || !digestPattern.MatchString(first.ComparisonDigest) {
		t.Fatalf("remediation progress is incomplete or authoritative: %#v", first)
	}
	for index, requirement := range first.Requirements {
		expectation := independentRemediationExpectations()[index]
		if requirement.Category != expectation.category || requirement.RequiredSource != expectation.source ||
			requirement.ResponsibleBoundary != expectation.boundary || requirement.SafeFollowUp != expectation.followUp ||
			requirement.Change != SafetyEvidenceGapRemediationUnchanged || requirement.PreviousRequired ||
			requirement.CurrentRequired || requirement.RequirementEvidenceChanged ||
			requirement.PreviousRequirement != nil || requirement.CurrentRequirement != nil {
			t.Fatalf("canonical requirement %d changed: %#v", index, requirement)
		}
	}
	if first.ComparisonDigest != "d67bf619f31c4c504c4c521942d2e7133e534873cf7a5e25870deb20b7c5e6d5" {
		t.Fatalf("fixed remediation progress changed: %s", first.ComparisonDigest)
	}
	for _, upstream := range []string{
		first.PreviousReportDigest, first.CurrentReportDigest,
		first.PreviousArtifactDigest, first.CurrentArtifactDigest,
		first.PreviousMatrixDigest, first.CurrentMatrixDigest,
	} {
		if first.ComparisonDigest == upstream {
			t.Fatal("remediation progress digest was not domain-separated")
		}
	}
	if err := VerifySafetyEvidenceGapRemediationProgressComparison(previous, current, observedAt, first); err != nil {
		t.Fatalf("exact remediation progress did not verify: %v", err)
	}
}

func TestSafetyEvidenceGapRemediationProgressClassifiesEveryChangeState(t *testing.T) {
	previous := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 3, 0, 0, 0, time.UTC), func(value *SafetyCase) {
		value.ProviderCapability.TradeAllowed = false
		value.OwnerAuthorization.MFAVerified = false
	}))
	current := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 4, 0, 0, 0, time.UTC), func(value *SafetyCase) {
		value.OwnerAuthorization.MFAVerified = false
		value.PreTradeReconciliation.BlocksNewActions = true
	}))
	comparison := BuildSafetyEvidenceGapRemediationProgressComparison(
		previous, current, time.Date(2026, 9, 9, 4, 2, 0, 0, time.UTC),
	)
	if comparison.Status != VerificationVerified || comparison.ExecutionAuthority ||
		comparison.UnchangedCount != 5 || comparison.NewlyRequiredCount != 1 || comparison.ResolvedCount != 1 {
		t.Fatalf("exact change counts not preserved: %#v", comparison)
	}
	expected := []struct {
		category        string
		change          SafetyEvidenceGapRemediationChange
		previous, now   bool
		evidenceChanged bool
	}{
		{"PROVIDER_CAPABILITY", SafetyEvidenceGapRemediationResolved, true, false, true},
		{"OWNER_AUTHORIZATION", SafetyEvidenceGapRemediationUnchanged, true, true, false},
		{"DETERMINISTIC_RISK", SafetyEvidenceGapRemediationUnchanged, false, false, false},
		{"ACCOUNT_RECONCILIATION", SafetyEvidenceGapRemediationNewlyRequired, false, true, true},
		{"BROKER_KILL_SWITCH", SafetyEvidenceGapRemediationUnchanged, false, false, false},
		{"IDEMPOTENCY_RESERVATION", SafetyEvidenceGapRemediationUnchanged, false, false, false},
		{"LIVE_LIFECYCLE_CONTRACT", SafetyEvidenceGapRemediationUnchanged, false, false, false},
	}
	for index, want := range expected {
		got := comparison.Requirements[index]
		if got.Category != want.category || got.Change != want.change || got.PreviousRequired != want.previous ||
			got.CurrentRequired != want.now || got.RequirementEvidenceChanged != want.evidenceChanged {
			t.Fatalf("requirement %d classification mismatch: %#v", index, got)
		}
		if got.PreviousRequired != (got.PreviousRequirement != nil) || got.CurrentRequired != (got.CurrentRequirement != nil) {
			t.Fatalf("requirement %d lost exact before/after evidence: %#v", index, got)
		}
	}
	owner := comparison.Requirements[1]
	if !reflect.DeepEqual(owner.PreviousRequirement, owner.CurrentRequirement) ||
		!reflect.DeepEqual(owner.CurrentRequirement.ReasonCodes, []ReasonCode{ReasonOwnerAuthorizationUnavailable}) {
		t.Fatalf("unchanged exact requirement was not preserved: %#v", owner)
	}
}

func TestSafetyEvidenceGapRemediationProgressClassifiesInputFailures(t *testing.T) {
	previous := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 5, 0, 0, 0, time.UTC), nil))
	current := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 6, 0, 0, 0, time.UTC), nil))
	observedAt := time.Date(2026, 9, 9, 6, 2, 0, 0, time.UTC)
	tests := []struct {
		name       string
		failure    SafetyEvidenceGapRemediationProgressFailure
		previous   SafetyEvidenceGapRemediationOwnerReviewChain
		current    SafetyEvidenceGapRemediationOwnerReviewChain
		observedAt time.Time
	}{
		{"observed at", SafetyEvidenceGapRemediationProgressFailureObservedAt, previous, current, observedAt.In(time.FixedZone("offset", 3600))},
		{"previous chain", SafetyEvidenceGapRemediationProgressFailurePreviousChain, mutateProgressChain(t, previous, func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
			value.OwnerReviewReport.ReportDigest = digestB
		}), current, observedAt},
		{"current chain", SafetyEvidenceGapRemediationProgressFailureCurrentChain, previous, mutateProgressChain(t, current, func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
			value.OwnerReviewReport.ReportDigest = digestB
		}), observedAt},
		{"time order", SafetyEvidenceGapRemediationProgressFailureTimeOrder, previous, current, current.OwnerReviewReport.CurrentEvaluatedAt.Add(-time.Nanosecond)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			comparison := BuildSafetyEvidenceGapRemediationProgressComparison(test.previous, test.current, test.observedAt)
			if comparison.Status != VerificationRejected || comparison.Failure != test.failure ||
				comparison.ExecutionAuthority || comparison.ComparisonDigest != "" {
				t.Fatalf("input failure was not exact and closed: %#v", comparison)
			}
		})
	}

	otherAction := "10000000-0000-4000-8000-000000000099"
	other := completeRemediationProgressChain(buildRemediationMatrixChainForCases(
		t, time.Date(2026, 9, 9, 7, 0, 0, 0, time.UTC), func(value *SafetyCase) { value.Action.ID = otherAction },
	))
	comparison := BuildSafetyEvidenceGapRemediationProgressComparison(previous, other, time.Date(2026, 9, 9, 7, 2, 0, 0, time.UTC))
	if comparison.Failure != SafetyEvidenceGapRemediationProgressFailureComparable {
		t.Fatalf("different safety identity was compared: %#v", comparison)
	}
}

func TestSafetyEvidenceGapRemediationProgressRejectsSecretsAndAuthorityWithoutEcho(t *testing.T) {
	previous := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 8, 0, 0, 0, time.UTC), nil))
	current := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 9, 0, 0, 0, time.UTC), nil))
	observedAt := time.Date(2026, 9, 9, 9, 2, 0, 0, time.UTC)
	secret := mutateProgressChain(t, previous, func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
		value.OwnerReviewReport.ReportVersion = "authorization_header_do_not_copy"
	})
	comparison := BuildSafetyEvidenceGapRemediationProgressComparison(secret, current, observedAt)
	payload, err := json.Marshal(comparison)
	if err != nil || comparison.Failure != SafetyEvidenceGapRemediationProgressFailureSecret ||
		comparison.ComparisonDigest != "" || len(comparison.Requirements) != 0 ||
		strings.Contains(string(payload), "authorization_header_do_not_copy") {
		t.Fatalf("secret-like input was retained: %s %v", payload, err)
	}
	authoritative := mutateProgressChain(t, previous, func(value *SafetyEvidenceGapRemediationOwnerReviewChain) {
		value.OwnerReviewReport.ExecutionAuthority = true
	})
	comparison = BuildSafetyEvidenceGapRemediationProgressComparison(authoritative, current, observedAt)
	if comparison.Failure != SafetyEvidenceGapRemediationProgressFailureAuthority || comparison.ExecutionAuthority {
		t.Fatalf("authority-bearing input was not rejected: %#v", comparison)
	}
}

func TestSafetyEvidenceGapRemediationProgressRejectsEveryOutputTamper(t *testing.T) {
	previous := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC), func(value *SafetyCase) {
		value.ProviderCapability.TradeAllowed = false
	}))
	current := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 11, 0, 0, 0, time.UTC), func(value *SafetyCase) {
		value.OwnerAuthorization.MFAVerified = false
	}))
	observedAt := time.Date(2026, 9, 9, 11, 2, 0, 0, time.UTC)
	original := BuildSafetyEvidenceGapRemediationProgressComparison(previous, current, observedAt)
	mutations := []struct {
		name   string
		mutate func(*SafetyEvidenceGapRemediationProgressComparison)
	}{
		{"version", func(value *SafetyEvidenceGapRemediationProgressComparison) { value.ComparisonVersion += "-changed" }},
		{"status", func(value *SafetyEvidenceGapRemediationProgressComparison) { value.Status = VerificationRejected }},
		{"failure", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.Failure = SafetyEvidenceGapRemediationProgressFailureCount
		}},
		{"authority", func(value *SafetyEvidenceGapRemediationProgressComparison) { value.ExecutionAuthority = true }},
		{"observed", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.ObservedAt = value.ObservedAt.Add(time.Nanosecond)
		}},
		{"previous baseline", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.PreviousBaselineAt = value.PreviousBaselineAt.Add(time.Nanosecond)
		}},
		{"previous current", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.PreviousCurrentAt = value.PreviousCurrentAt.Add(time.Nanosecond)
		}},
		{"current baseline", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.CurrentBaselineAt = value.CurrentBaselineAt.Add(time.Nanosecond)
		}},
		{"current current", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.CurrentCurrentAt = value.CurrentCurrentAt.Add(time.Nanosecond)
		}},
		{"previous report digest", func(value *SafetyEvidenceGapRemediationProgressComparison) { value.PreviousReportDigest = digestB }},
		{"current report digest", func(value *SafetyEvidenceGapRemediationProgressComparison) { value.CurrentReportDigest = digestB }},
		{"previous artifact digest", func(value *SafetyEvidenceGapRemediationProgressComparison) { value.PreviousArtifactDigest = digestB }},
		{"current artifact digest", func(value *SafetyEvidenceGapRemediationProgressComparison) { value.CurrentArtifactDigest = digestB }},
		{"previous matrix digest", func(value *SafetyEvidenceGapRemediationProgressComparison) { value.PreviousMatrixDigest = digestB }},
		{"current matrix digest", func(value *SafetyEvidenceGapRemediationProgressComparison) { value.CurrentMatrixDigest = digestB }},
		{"category count", func(value *SafetyEvidenceGapRemediationProgressComparison) { value.ExpectedCategoryCount++ }},
		{"unchanged count", func(value *SafetyEvidenceGapRemediationProgressComparison) { value.UnchangedCount++ }},
		{"new count", func(value *SafetyEvidenceGapRemediationProgressComparison) { value.NewlyRequiredCount++ }},
		{"resolved count", func(value *SafetyEvidenceGapRemediationProgressComparison) { value.ResolvedCount++ }},
		{"category", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.Requirements[0].Category += "_CHANGED"
		}},
		{"source", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.Requirements[0].RequiredSource = EvidenceDatabase
		}},
		{"boundary", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.Requirements[0].ResponsibleBoundary = SafetyEvidenceGapBoundaryRiskControl
		}},
		{"follow up", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.Requirements[0].SafeFollowUp = SafetyEvidenceGapFollowUpVerifyRisk
		}},
		{"change", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.Requirements[0].Change = SafetyEvidenceGapRemediationUnchanged
		}},
		{"previous required", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.Requirements[0].PreviousRequired = false
		}},
		{"current required", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.Requirements[0].CurrentRequired = true
		}},
		{"evidence changed", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.Requirements[0].RequirementEvidenceChanged = false
		}},
		{"previous requirement", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.Requirements[0].PreviousRequirement.Category += "_CHANGED"
		}},
		{"current requirement", func(value *SafetyEvidenceGapRemediationProgressComparison) {
			value.Requirements[1].CurrentRequirement.Category += "_CHANGED"
		}},
		{"digest", func(value *SafetyEvidenceGapRemediationProgressComparison) { value.ComparisonDigest = digestB }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := cloneRemediationProgress(t, original)
			mutation.mutate(&changed)
			if !errors.Is(VerifySafetyEvidenceGapRemediationProgressComparison(previous, current, observedAt, changed), ErrSafetyEvidenceGapRemediationProgress) {
				t.Fatalf("tampered progress comparison verified: %#v", changed)
			}
		})
	}
}

func TestSafetyEvidenceGapRemediationProgressBindsEveryInput(t *testing.T) {
	previous := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC), nil))
	current := completeRemediationProgressChain(buildRemediationMatrixChain(t, time.Date(2026, 9, 9, 13, 0, 0, 0, time.UTC), nil))
	observedAt := time.Date(2026, 9, 9, 13, 2, 0, 0, time.UTC)
	original := BuildSafetyEvidenceGapRemediationProgressComparison(previous, current, observedAt)
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
				if !errors.Is(VerifySafetyEvidenceGapRemediationProgressComparison(changedPrevious, changedCurrent, observedAt, original), ErrSafetyEvidenceGapRemediationProgress) {
					t.Fatalf("changed %s %s verified", side, mutation.name)
				}
			})
		}
	}
}

func TestSafetyEvidenceGapRemediationProgressHasNoRuntimeSurface(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("remediation progress source path unavailable")
	}
	directory := filepath.Dir(filename)
	source, err := os.ReadFile(filepath.Join(directory, "verification_gap_remediation_progress.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"context.Context", "NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient", "SubmitOrder", "CancelOrder",
		"RunCycle", "Execute", "Refresh", "Reconnect", "Sync", "PlaceOrder",
		"BuildSafetyEvidenceGapRemediationMatrixVerificationReviewReport(",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("remediation progress unexpectedly contains runtime or self-asserted surface %q", prohibited)
		}
	}
	apiRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	for _, symbol := range []string{"BuildSafetyEvidenceGapRemediationProgressComparison(", "VerifySafetyEvidenceGapRemediationProgressComparison("} {
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
				t.Fatalf("remediation progress has a production caller in %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func completeRemediationProgressChain(chain remediationMatrixChain) SafetyEvidenceGapRemediationOwnerReviewChain {
	matrix, matrixReport, ownerReview := completeRemediationOwnerReviewChain(chain)
	ownerReport := buildRemediationOwnerReviewReport(chain, matrix, matrixReport, ownerReview)
	return SafetyEvidenceGapRemediationOwnerReviewChain{
		BaselineReport: chain.baseline, CurrentReport: chain.current,
		ComparisonEnvelope: chain.envelope, ComparisonReport: chain.audit, ComparisonReview: chain.review,
		GapInventory: chain.inventory, GapInventoryReport: chain.report, GapInventoryReview: chain.verificationReview,
		RemediationMatrix: matrix, MatrixReport: matrixReport, OwnerReview: ownerReview, OwnerReviewReport: ownerReport,
	}
}

func buildRemediationMatrixChainForCases(t *testing.T, now time.Time, mutate func(*SafetyCase)) remediationMatrixChain {
	t.Helper()
	baselineCase := validSafetyCase(now)
	currentCase := validSafetyCase(now.Add(time.Minute))
	mutate(&baselineCase)
	mutate(&currentCase)
	baseline := verifiedSafetyCaseReport(t, baselineCase, now)
	current := verifiedSafetyCaseReport(t, currentCase, now.Add(time.Minute))
	envelope, audit, review := verifiedComparisonReviewChain(t, baseline, current)
	inventory := CompileSafetyEvidenceGapInventory(baseline, current, envelope, audit, review)
	report := BuildSafetyEvidenceGapInventoryVerificationReport(baseline, current, envelope, audit, review, inventory)
	verificationReview := BuildSafetyEvidenceGapInventoryVerificationReview(baseline, current, envelope, audit, review, inventory, report)
	return remediationMatrixChain{baseline, current, envelope, audit, review, inventory, report, verificationReview}
}

func mutateProgressChain(
	t *testing.T,
	value SafetyEvidenceGapRemediationOwnerReviewChain,
	mutate func(*SafetyEvidenceGapRemediationOwnerReviewChain),
) SafetyEvidenceGapRemediationOwnerReviewChain {
	t.Helper()
	cloned := cloneRemediationProgressChain(t, value)
	mutate(&cloned)
	return cloned
}

func cloneRemediationProgressChain(t *testing.T, value SafetyEvidenceGapRemediationOwnerReviewChain) SafetyEvidenceGapRemediationOwnerReviewChain {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyEvidenceGapRemediationOwnerReviewChain
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func cloneRemediationProgress(t *testing.T, value SafetyEvidenceGapRemediationProgressComparison) SafetyEvidenceGapRemediationProgressComparison {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyEvidenceGapRemediationProgressComparison
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
