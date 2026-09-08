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

func TestSafetyEvidenceGapInventoryVerificationReportIsIndependentDeterministicAndFixed(t *testing.T) {
	now := time.Date(2026, 9, 8, 19, 0, 0, 123, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, audit, review := verifiedComparisonReviewChain(t, input, input)
	inventory := CompileSafetyEvidenceGapInventory(input, input, envelope, audit, review)

	first := BuildSafetyEvidenceGapInventoryVerificationReport(input, input, envelope, audit, review, inventory)
	second := BuildSafetyEvidenceGapInventoryVerificationReport(input, input, envelope, audit, review, inventory)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("verification report replay changed: %#v %#v", first, second)
	}
	if first.ReportVersion != SafetyEvidenceGapInventoryVerificationReportVersion ||
		first.Status != VerificationVerified || first.Failure != SafetyEvidenceGapInventoryVerificationFailureNone ||
		first.ExecutionAuthority || !first.CurrentEvidenceAvailable ||
		!first.InventoryVersion.Verified || !first.ReviewDigest.Verified || !first.InventoryDigest.Verified ||
		first.ExpectedCategoryCount != 7 || first.ClaimedCategoryCount != 7 ||
		first.RecomputedCurrentGapCount != 0 || first.ClaimedCurrentGapCount != 0 ||
		first.RecomputedNonUnchangedCategoryCount != 0 || first.ClaimedNonUnchangedCategoryCount != 0 ||
		len(first.Categories) != 7 || !digestPattern.MatchString(first.ReportDigest) {
		t.Fatalf("verification report is incomplete or authoritative: %#v", first)
	}
	for index, category := range first.Categories {
		expected := verificationEvidenceExpectations()[index]
		if category.Category != expected.category || category.Source != expected.source || !category.Verified ||
			category.ClaimedState != SafetyEvidenceGapUnchanged || category.RecomputedState != SafetyEvidenceGapUnchanged ||
			category.ClaimedCurrentUnavailable || category.RecomputedCurrentUnavailable {
			t.Fatalf("category %d was not independently verified: %#v", index, category)
		}
	}
	independentDigest, err := independentlyCanonicalSafetyEvidenceGapInventoryDigest(inventory)
	if err != nil || independentDigest != inventory.InventoryDigest || first.InventoryDigest.Recomputed != inventory.InventoryDigest {
		t.Fatalf("independent canonical inventory digest mismatch: %s %s %v", independentDigest, inventory.InventoryDigest, err)
	}
	if first.ReportDigest != "a2f5cab94b5d3f661066046f8b1fec56f1335059a0e0ee4c850e906003f261ee" {
		t.Fatalf("fixed verification report changed: %s", first.ReportDigest)
	}
	if first.ReportDigest == inventory.InventoryDigest {
		t.Fatal("report and inventory domains produced the same digest")
	}
	if err = VerifySafetyEvidenceGapInventoryVerificationReport(input, input, envelope, audit, review, inventory, first); err != nil {
		t.Fatalf("exact verification report did not verify: %v", err)
	}
}

func TestSafetyEvidenceGapInventoryVerificationReportRejectsEveryCategoryMutation(t *testing.T) {
	now := time.Date(2026, 9, 8, 19, 0, 0, 0, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, audit, review := verifiedComparisonReviewChain(t, input, input)
	original := CompileSafetyEvidenceGapInventory(input, input, envelope, audit, review)
	for index := range original.Categories {
		mutations := []struct {
			name   string
			mutate func(*SafetyEvidenceGap)
		}{
			{"category", func(value *SafetyEvidenceGap) { value.Category += "_CHANGED" }},
			{"source", func(value *SafetyEvidenceGap) { value.Source = EvidenceSource("CHANGED") }},
			{"state", func(value *SafetyEvidenceGap) { value.State = SafetyEvidenceGapChanged }},
			{"baseline unavailable", func(value *SafetyEvidenceGap) { value.BaselineUnavailable = true }},
			{"current unavailable", func(value *SafetyEvidenceGap) { value.CurrentUnavailable = true }},
			{"baseline reasons", func(value *SafetyEvidenceGap) { value.BaselineReasonCodes = []ReasonCode{ReasonInvalidEvidence} }},
			{"current reasons", func(value *SafetyEvidenceGap) { value.CurrentReasonCodes = []ReasonCode{ReasonInvalidEvidence} }},
			{"baseline evidence", func(value *SafetyEvidenceGap) {
				if value.Baseline.Digest == digestA {
					value.Baseline.Digest = digestB
				} else {
					value.Baseline.Digest = digestA
				}
			}},
			{"current evidence", func(value *SafetyEvidenceGap) {
				if value.Current.Digest == digestA {
					value.Current.Digest = digestB
				} else {
					value.Current.Digest = digestA
				}
			}},
		}
		for _, mutation := range mutations {
			t.Run(verificationEvidenceExpectations()[index].category+"/"+mutation.name, func(t *testing.T) {
				inventory := cloneSafetyEvidenceGapInventory(t, original)
				mutation.mutate(&inventory.Categories[index])
				report := BuildSafetyEvidenceGapInventoryVerificationReport(input, input, envelope, audit, review, inventory)
				if report.Status != VerificationRejected || report.Failure != SafetyEvidenceGapInventoryVerificationFailureCategory ||
					report.ExecutionAuthority || report.ReportDigest != "" {
					t.Fatalf("category mutation was not rejected: %#v", report)
				}
			})
		}
	}
}

func TestSafetyEvidenceGapInventoryVerificationReportBindsEveryInput(t *testing.T) {
	now := time.Date(2026, 9, 8, 19, 0, 0, 0, time.UTC)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	current := cloneSafetyCaseReport(t, baseline)
	envelope, audit, review := verifiedComparisonReviewChain(t, baseline, current)
	inventory := CompileSafetyEvidenceGapInventory(baseline, current, envelope, audit, review)
	report := BuildSafetyEvidenceGapInventoryVerificationReport(baseline, current, envelope, audit, review, inventory)

	changedBaseline := cloneSafetyCaseReport(t, baseline)
	changedBaseline.EvaluatedAt = changedBaseline.EvaluatedAt.Add(time.Nanosecond)
	if !errors.Is(VerifySafetyEvidenceGapInventoryVerificationReport(changedBaseline, current, envelope, audit, review, inventory, report), ErrSafetyEvidenceGapInventoryVerificationReport) {
		t.Fatal("changed baseline verified against prior report")
	}
	changedEnvelope := cloneVerificationComparisonEnvelope(t, envelope)
	changedEnvelope.EnvelopeDigest = digestB
	if !errors.Is(VerifySafetyEvidenceGapInventoryVerificationReport(baseline, current, changedEnvelope, audit, review, inventory, report), ErrSafetyEvidenceGapInventoryVerificationReport) {
		t.Fatal("changed envelope verified against prior report")
	}
	changedAudit := cloneVerificationComparisonEnvelopeReport(t, audit)
	changedAudit.Digests.Envelope.Verified = false
	if !errors.Is(VerifySafetyEvidenceGapInventoryVerificationReport(baseline, current, envelope, changedAudit, review, inventory, report), ErrSafetyEvidenceGapInventoryVerificationReport) {
		t.Fatal("changed audit verified against prior report")
	}
	changedReview := cloneVerificationComparisonReviewArtifact(t, review)
	changedReview.ReviewDigest = digestB
	if !errors.Is(VerifySafetyEvidenceGapInventoryVerificationReport(baseline, current, envelope, audit, changedReview, inventory, report), ErrSafetyEvidenceGapInventoryVerificationReport) {
		t.Fatal("changed review verified against prior report")
	}
	changedInventory := cloneSafetyEvidenceGapInventory(t, inventory)
	changedInventory.InventoryDigest = digestB
	if !errors.Is(VerifySafetyEvidenceGapInventoryVerificationReport(baseline, current, envelope, audit, review, changedInventory, report), ErrSafetyEvidenceGapInventoryVerificationReport) {
		t.Fatal("changed inventory verified against prior report")
	}
}

func TestSafetyEvidenceGapInventoryVerificationReportFailsClosedOnUnavailableEvidence(t *testing.T) {
	baselineTime := time.Date(2026, 9, 8, 19, 0, 0, 0, time.UTC)
	currentTime := baselineTime.Add(time.Minute)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(baselineTime), baselineTime)
	currentCase := validSafetyCase(currentTime)
	currentCase.ProviderCapability.TradeAllowed = false
	current := verifiedSafetyCaseReport(t, currentCase, currentTime)
	envelope, audit, review := verifiedComparisonReviewChain(t, baseline, current)
	inventory := CompileSafetyEvidenceGapInventory(baseline, current, envelope, audit, review)
	report := BuildSafetyEvidenceGapInventoryVerificationReport(baseline, current, envelope, audit, review, inventory)

	if report.Status != VerificationRejected || report.Failure != SafetyEvidenceGapInventoryVerificationFailureUnavailable ||
		report.ExecutionAuthority || report.CurrentEvidenceAvailable ||
		report.ClaimedCurrentGapCount != 1 || report.RecomputedCurrentGapCount != 1 ||
		len(report.Categories) != 7 || report.Categories[0].Category != "PROVIDER_CAPABILITY" ||
		report.Categories[0].RecomputedState != SafetyEvidenceGapNewlyUnavailable ||
		!report.Categories[0].RecomputedCurrentUnavailable || !report.Categories[0].Verified ||
		!digestPattern.MatchString(report.ReportDigest) {
		t.Fatalf("unavailable evidence was not preserved closed: %#v", report)
	}
	if err := VerifySafetyEvidenceGapInventoryVerificationReport(baseline, current, envelope, audit, review, inventory, report); err != nil {
		t.Fatalf("exact closed unavailable report did not verify: %v", err)
	}
}

func TestSafetyEvidenceGapInventoryVerificationReportRejectsInventoryMismatchesExactly(t *testing.T) {
	now := time.Date(2026, 9, 8, 19, 0, 0, 0, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, audit, review := verifiedComparisonReviewChain(t, input, input)
	original := CompileSafetyEvidenceGapInventory(input, input, envelope, audit, review)
	tests := []struct {
		name    string
		failure SafetyEvidenceGapInventoryVerificationFailure
		mutate  func(*SafetyEvidenceGapInventory)
	}{
		{"version", SafetyEvidenceGapInventoryVerificationFailureInventoryVersion, func(value *SafetyEvidenceGapInventory) { value.InventoryVersion = "inventory-v2" }},
		{"status", SafetyEvidenceGapInventoryVerificationFailureInventoryStatus, func(value *SafetyEvidenceGapInventory) { value.Status = SafetyEvidenceGapInventoryRejected }},
		{"authority", SafetyEvidenceGapInventoryVerificationFailureAuthority, func(value *SafetyEvidenceGapInventory) { value.ExecutionAuthority = true }},
		{"review digest", SafetyEvidenceGapInventoryVerificationFailureInventoryMismatch, func(value *SafetyEvidenceGapInventory) { value.ReviewDigest = digestB }},
		{"category count", SafetyEvidenceGapInventoryVerificationFailureCount, func(value *SafetyEvidenceGapInventory) { value.Categories = value.Categories[:6] }},
		{"gap count", SafetyEvidenceGapInventoryVerificationFailureCount, func(value *SafetyEvidenceGapInventory) { value.CurrentGapCount++ }},
		{"category fact", SafetyEvidenceGapInventoryVerificationFailureCategory, func(value *SafetyEvidenceGapInventory) { value.Categories[0].State = SafetyEvidenceGapChanged }},
		{"category order", SafetyEvidenceGapInventoryVerificationFailureCategory, func(value *SafetyEvidenceGapInventory) {
			value.Categories[0], value.Categories[1] = value.Categories[1], value.Categories[0]
		}},
		{"digest malformed", SafetyEvidenceGapInventoryVerificationFailureDigestMalformed, func(value *SafetyEvidenceGapInventory) { value.InventoryDigest = "not-a-digest" }},
		{"digest mismatch", SafetyEvidenceGapInventoryVerificationFailureDigestMismatch, func(value *SafetyEvidenceGapInventory) { value.InventoryDigest = digestB }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inventory := cloneSafetyEvidenceGapInventory(t, original)
			test.mutate(&inventory)
			report := BuildSafetyEvidenceGapInventoryVerificationReport(input, input, envelope, audit, review, inventory)
			if report.Status != VerificationRejected || report.Failure != test.failure ||
				report.ExecutionAuthority || report.ReportDigest != "" {
				t.Fatalf("mismatch was not classified exactly: %#v", report)
			}
			if !errors.Is(VerifySafetyEvidenceGapInventoryVerificationReport(input, input, envelope, audit, review, inventory, report), ErrSafetyEvidenceGapInventoryVerificationReport) {
				t.Fatalf("invalid report verified: %#v", report)
			}
		})
	}
}

func TestSafetyEvidenceGapInventoryVerificationReportRejectsSecretWithoutEcho(t *testing.T) {
	now := time.Date(2026, 9, 8, 19, 0, 0, 0, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, audit, review := verifiedComparisonReviewChain(t, input, input)
	inventory := CompileSafetyEvidenceGapInventory(input, input, envelope, audit, review)
	inventory.OwnerAction = VerificationComparisonOwnerAction("client_secret_do_not_copy")
	report := BuildSafetyEvidenceGapInventoryVerificationReport(input, input, envelope, audit, review, inventory)
	if report.Status != VerificationRejected || report.Failure != SafetyEvidenceGapInventoryVerificationFailureSecretInput ||
		report.ExecutionAuthority || report.ReportDigest != "" || len(report.Categories) != 0 {
		t.Fatalf("secret-like input produced attributable output: %#v", report)
	}
	payload, err := json.Marshal(report)
	if err != nil || strings.Contains(string(payload), "client_secret_do_not_copy") {
		t.Fatalf("secret-like input was echoed: %s %v", payload, err)
	}
}

func TestSafetyEvidenceGapInventoryVerificationReportRejectsEveryOutputTamper(t *testing.T) {
	now := time.Date(2026, 9, 8, 19, 0, 0, 0, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, audit, review := verifiedComparisonReviewChain(t, input, input)
	inventory := CompileSafetyEvidenceGapInventory(input, input, envelope, audit, review)
	original := BuildSafetyEvidenceGapInventoryVerificationReport(input, input, envelope, audit, review, inventory)
	mutations := []struct {
		name   string
		mutate func(*SafetyEvidenceGapInventoryVerificationReport)
	}{
		{"version", func(value *SafetyEvidenceGapInventoryVerificationReport) { value.ReportVersion = "report-v2" }},
		{"status", func(value *SafetyEvidenceGapInventoryVerificationReport) { value.Status = VerificationRejected }},
		{"failure", func(value *SafetyEvidenceGapInventoryVerificationReport) {
			value.Failure = SafetyEvidenceGapInventoryVerificationFailureUnavailable
		}},
		{"authority", func(value *SafetyEvidenceGapInventoryVerificationReport) { value.ExecutionAuthority = true }},
		{"inventory version", func(value *SafetyEvidenceGapInventoryVerificationReport) { value.InventoryVersion.Verified = false }},
		{"baseline time", func(value *SafetyEvidenceGapInventoryVerificationReport) {
			value.BaselineEvaluatedAt = value.BaselineEvaluatedAt.Add(time.Nanosecond)
		}},
		{"review digest", func(value *SafetyEvidenceGapInventoryVerificationReport) { value.ReviewDigest.Claimed = digestB }},
		{"inventory digest", func(value *SafetyEvidenceGapInventoryVerificationReport) { value.InventoryDigest.Expected = digestB }},
		{"expected count", func(value *SafetyEvidenceGapInventoryVerificationReport) { value.ExpectedCategoryCount++ }},
		{"claimed count", func(value *SafetyEvidenceGapInventoryVerificationReport) { value.ClaimedCategoryCount++ }},
		{"gap count", func(value *SafetyEvidenceGapInventoryVerificationReport) { value.RecomputedCurrentGapCount++ }},
		{"change count", func(value *SafetyEvidenceGapInventoryVerificationReport) { value.ClaimedNonUnchangedCategoryCount++ }},
		{"availability", func(value *SafetyEvidenceGapInventoryVerificationReport) { value.CurrentEvidenceAvailable = false }},
		{"category fact", func(value *SafetyEvidenceGapInventoryVerificationReport) { value.Categories[0].Verified = false }},
		{"category order", func(value *SafetyEvidenceGapInventoryVerificationReport) {
			value.Categories[0], value.Categories[1] = value.Categories[1], value.Categories[0]
		}},
		{"report digest", func(value *SafetyEvidenceGapInventoryVerificationReport) { value.ReportDigest = digestB }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			report := cloneGapInventoryVerificationReport(t, original)
			mutation.mutate(&report)
			if !errors.Is(VerifySafetyEvidenceGapInventoryVerificationReport(input, input, envelope, audit, review, inventory, report), ErrSafetyEvidenceGapInventoryVerificationReport) {
				t.Fatalf("tampered verification report verified: %#v", report)
			}
		})
	}
}

func TestSafetyEvidenceGapInventoryVerificationReportHasNoRuntimeSurfaceOrProductionCaller(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("gap inventory verification report test path unavailable")
	}
	directory := filepath.Dir(filename)
	source, err := os.ReadFile(filepath.Join(directory, "verification_gap_inventory_report.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"context.Context", "NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient", "SubmitOrder", "CancelOrder",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("verification report unexpectedly contains runtime surface %q", prohibited)
		}
	}

	apiRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	for _, symbol := range []string{"BuildSafetyEvidenceGapInventoryVerificationReport(", "VerifySafetyEvidenceGapInventoryVerificationReport("} {
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
				t.Fatalf("verification report has a production caller in %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func cloneGapInventoryVerificationReport(t *testing.T, value SafetyEvidenceGapInventoryVerificationReport) SafetyEvidenceGapInventoryVerificationReport {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyEvidenceGapInventoryVerificationReport
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
