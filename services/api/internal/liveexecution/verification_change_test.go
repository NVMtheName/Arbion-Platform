package liveexecution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestVerificationComparisonReportsExactSameEvidence(t *testing.T) {
	now := time.Date(2026, 9, 8, 13, 0, 0, 123, time.UTC)
	report := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	comparison := CompareSafetyCaseVerificationReports(report, report)
	if comparison.ComparisonVersion != SafetyCaseVerificationComparisonVersion ||
		comparison.Status != VerificationComparisonSame || comparison.FailureInput != VerificationComparisonInputNone ||
		comparison.Failure != VerificationComparisonFailureNone || comparison.ExecutionAuthority ||
		comparison.ChangeCount != 0 || !comparison.BaselineEvaluatedAt.Equal(now) ||
		!comparison.CurrentEvaluatedAt.Equal(now) {
		t.Fatalf("same comparison changed or gained authority: %#v", comparison)
	}
	if comparison.EvaluationTime.State != VerificationChangeSame || len(comparison.Versions) != 3 ||
		len(comparison.Identities) != 12 || len(comparison.Digests) != 3 ||
		comparison.Assessment.State != VerificationChangeSame || len(comparison.Evidence) != 7 {
		t.Fatalf("comparison coverage is incomplete: %#v", comparison)
	}
	assertEveryVerificationChangeState(t, comparison, VerificationChangeSame)
}

func TestVerificationComparisonPreservesExactChangedEvidence(t *testing.T) {
	baselineTime := time.Date(2026, 9, 8, 13, 0, 0, 0, time.UTC)
	currentTime := baselineTime.Add(time.Minute)
	baselineCase := validSafetyCase(baselineTime)
	currentCase := validSafetyCase(currentTime)
	currentCase.Action.ID = "11111111-1111-4111-8111-111111111112"
	currentCase.ProviderCapability.Evidence.ID = "90000000-0000-4000-8000-000000000011"
	currentCase.ProviderCapability.Evidence.Digest = digestA
	baseline := verifiedSafetyCaseReport(t, baselineCase, baselineTime)
	current := verifiedSafetyCaseReport(t, currentCase, currentTime)
	comparison := CompareSafetyCaseVerificationReports(baseline, current)
	if comparison.Status != VerificationComparisonChanged || comparison.Failure != VerificationComparisonFailureNone ||
		comparison.FailureInput != VerificationComparisonInputNone || comparison.ExecutionAuthority || comparison.ChangeCount == 0 {
		t.Fatalf("changed comparison was not closed and exact: %#v", comparison)
	}
	if comparison.EvaluationTime.State != VerificationChangeChanged {
		t.Fatalf("evaluation-time change was omitted: %#v", comparison.EvaluationTime)
	}
	assertNamedChange(t, comparison.Versions, "report_version", VerificationChangeSame)
	assertNamedChange(t, comparison.Versions, "compiler_version", VerificationChangeSame)
	assertNamedChange(t, comparison.Versions, "contract_version", VerificationChangeSame)
	assertNamedChange(t, comparison.Identities, "action_id", VerificationChangeChanged)
	assertNamedChange(t, comparison.Identities, "assessment_key", VerificationChangeChanged)
	assertNamedChange(t, comparison.Identities, "user_id", VerificationChangeSame)
	for _, digest := range comparison.Digests {
		if digest.State != VerificationChangeChanged || digest.Before == digest.After {
			t.Fatalf("digest change was not exact: %#v", digest)
		}
	}
	if comparison.Evidence[0].Category != "PROVIDER_CAPABILITY" ||
		comparison.Evidence[0].State != VerificationChangeChanged ||
		comparison.Evidence[0].Before.ID == comparison.Evidence[0].After.ID ||
		comparison.Evidence[0].Before.Digest == comparison.Evidence[0].After.Digest {
		t.Fatalf("provider evidence change was omitted: %#v", comparison.Evidence[0])
	}
	for _, evidence := range comparison.Evidence[1:] {
		if evidence.State != VerificationChangeChanged {
			t.Fatalf("new exact evidence timestamp was not preserved for %s: %#v", evidence.Category, evidence)
		}
	}

	payload, err := json.Marshal(comparison)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"provider_order_id", "client_order_id", "request_payload", "response_payload",
		"credential_generation", "idempotency_key", "authorization_header", "execution_command",
	} {
		if strings.Contains(strings.ToLower(string(payload)), prohibited) {
			t.Fatalf("comparison contains protected or executable field %q: %s", prohibited, payload)
		}
	}
}

func TestVerificationComparisonRejectsExactInvalidInput(t *testing.T) {
	now := time.Date(2026, 9, 8, 13, 0, 0, 0, time.UTC)
	valid := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	tests := []struct {
		name     string
		input    VerificationComparisonInput
		expected VerificationComparisonFailure
		mutate   func(*SafetyCaseVerificationReport)
	}{
		{"secret-like", VerificationComparisonInputCurrent, VerificationComparisonFailureSecretInput, func(value *SafetyCaseVerificationReport) { value.Identity.ProviderName = "api_key_hidden" }},
		{"report version", VerificationComparisonInputCurrent, VerificationComparisonFailureReportVersion, func(value *SafetyCaseVerificationReport) { value.ReportVersion = "future-report-v2" }},
		{"status", VerificationComparisonInputCurrent, VerificationComparisonFailureStatus, func(value *SafetyCaseVerificationReport) { value.Status = VerificationRejected }},
		{"authority", VerificationComparisonInputCurrent, VerificationComparisonFailureAuthority, func(value *SafetyCaseVerificationReport) { value.ExecutionAuthority = true }},
		{"time", VerificationComparisonInputCurrent, VerificationComparisonFailureTime, func(value *SafetyCaseVerificationReport) { value.EvaluatedAt = time.Time{} }},
		{"version", VerificationComparisonInputCurrent, VerificationComparisonFailureVersion, func(value *SafetyCaseVerificationReport) { value.CompilerVersion.Verified = false }},
		{"identity", VerificationComparisonInputCurrent, VerificationComparisonFailureIdentity, func(value *SafetyCaseVerificationReport) { value.Identity.MandateID = "bad" }},
		{"assessment", VerificationComparisonInputCurrent, VerificationComparisonFailureAssessment, func(value *SafetyCaseVerificationReport) { value.AssessmentVerified = false }},
		{"evidence", VerificationComparisonInputCurrent, VerificationComparisonFailureEvidence, func(value *SafetyCaseVerificationReport) { value.Evidence[0].MatchesEnvelope = false }},
		{"digest", VerificationComparisonInputCurrent, VerificationComparisonFailureDigest, func(value *SafetyCaseVerificationReport) { value.Digests.Envelope.Recomputed = digestB }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := cloneSafetyCaseReport(t, valid)
			test.mutate(&current)
			comparison := CompareSafetyCaseVerificationReports(valid, current)
			if comparison.Status != VerificationComparisonRejected || comparison.FailureInput != test.input ||
				comparison.Failure != test.expected || comparison.ExecutionAuthority || comparison.ChangeCount != 0 {
				t.Fatalf("invalid input did not fail closed: %#v", comparison)
			}
		})
	}

	baseline := cloneSafetyCaseReport(t, valid)
	baseline.Identity.UserID = "bad"
	comparison := CompareSafetyCaseVerificationReports(baseline, valid)
	if comparison.FailureInput != VerificationComparisonInputBaseline || comparison.Failure != VerificationComparisonFailureIdentity {
		t.Fatalf("baseline failure was not attributed exactly: %#v", comparison)
	}

	earlier := verifiedSafetyCaseReport(t, validSafetyCase(now.Add(-time.Minute)), now.Add(-time.Minute))
	comparison = CompareSafetyCaseVerificationReports(valid, earlier)
	if comparison.FailureInput != VerificationComparisonInputCurrent || comparison.Failure != VerificationComparisonFailureTimeOrder {
		t.Fatalf("time regression was not rejected exactly: %#v", comparison)
	}
}

func TestVerificationComparisonDoesNotEchoSecretLikeInput(t *testing.T) {
	now := time.Date(2026, 9, 8, 13, 0, 0, 0, time.UTC)
	valid := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	unsafe := cloneSafetyCaseReport(t, valid)
	unsafe.Identity.ProviderName = "access_token_do_not_echo"
	comparison := CompareSafetyCaseVerificationReports(valid, unsafe)
	payload, err := json.Marshal(comparison)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Failure != VerificationComparisonFailureSecretInput ||
		strings.Contains(string(payload), "access_token_do_not_echo") {
		t.Fatalf("secret-like comparison input was echoed: %s", payload)
	}
}

func TestVerificationComparisonHasNoPersistenceOrNetworkSurface(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("verification comparison test path unavailable")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "verification_change.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"context.Context", "NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient", "SubmitOrder", "CancelOrder",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("comparison unexpectedly contains persistence, provider, or broker surface %q", prohibited)
		}
	}
}

func verifiedSafetyCaseReport(t *testing.T, safetyCase SafetyCase, evaluatedAt time.Time) SafetyCaseVerificationReport {
	t.Helper()
	envelope, err := CompileSafetyCaseEnvelope(safetyCase, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	report := BuildSafetyCaseVerificationReport(safetyCase, envelope)
	if report.Status != VerificationVerified {
		t.Fatalf("test report did not verify: %#v", report)
	}
	return report
}

func cloneSafetyCaseReport(t *testing.T, value SafetyCaseVerificationReport) SafetyCaseVerificationReport {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyCaseVerificationReport
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func assertEveryVerificationChangeState(t *testing.T, comparison SafetyCaseVerificationComparison, expected VerificationChangeState) {
	t.Helper()
	for _, changes := range [][]VerificationValueChange{comparison.Versions, comparison.Identities, comparison.Digests} {
		for _, change := range changes {
			if change.State != expected {
				t.Fatalf("unexpected %s state for %s: %#v", change.State, change.Field, change)
			}
		}
	}
	for _, evidence := range comparison.Evidence {
		if evidence.State != expected {
			t.Fatalf("unexpected %s evidence state: %#v", evidence.State, evidence)
		}
	}
}

func assertNamedChange(t *testing.T, changes []VerificationValueChange, field string, expected VerificationChangeState) {
	t.Helper()
	for _, change := range changes {
		if change.Field == field {
			if change.State != expected {
				t.Fatalf("%s state = %s, want %s: %#v", field, change.State, expected, change)
			}
			return
		}
	}
	t.Fatalf("missing change field %s", field)
}
