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

func TestSafetyCaseVerificationReportEnumeratesExactClosedEvidence(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 123, time.UTC)
	safetyCase := validSafetyCase(now)
	envelope, err := CompileSafetyCaseEnvelope(safetyCase, now)
	if err != nil {
		t.Fatal(err)
	}
	report := BuildSafetyCaseVerificationReport(safetyCase, envelope)
	if report.ReportVersion != SafetyCaseVerificationReportVersion ||
		report.Status != VerificationVerified || report.Failure != VerificationFailureNone ||
		report.ExecutionAuthority || !report.EvaluatedAt.Equal(now) {
		t.Fatalf("verification report is not closed and verified: %#v", report)
	}
	if !report.CompilerVersion.Verified || report.CompilerVersion.Expected != SafetyCaseCompilerVersion ||
		report.CompilerVersion.Claimed != SafetyCaseCompilerVersion ||
		!report.ContractVersion.Verified || report.ContractVersion.Expected != ContractVersion ||
		report.ContractVersion.Claimed != ContractVersion {
		t.Fatalf("version verification is incomplete: %#v %#v", report.CompilerVersion, report.ContractVersion)
	}
	identity := report.Identity
	if identity.ActionID != actionID || identity.AssessmentKey != "live-safety-11111111111141118111111111111111" ||
		identity.UserID != userID || identity.FinancialAccountID != accountID ||
		identity.ProviderConnectionID != connectionID || identity.ProviderName != "coinbase" ||
		identity.StrategyInstanceID != strategyID || identity.MandateID != mandateID ||
		identity.MandateVersion != 4 || identity.CapitalBucketID != bucketID ||
		identity.CapitalReservationID != reservationID || identity.ActionDigest != digestA ||
		!identity.BindingsVerified {
		t.Fatalf("identity verification is incomplete: %#v", identity)
	}
	if !report.AssessmentVerified || report.Assessment.Availability != BlockedUnimplemented ||
		!report.Assessment.Structural {
		t.Fatalf("assessment verification changed: %#v", report.Assessment)
	}
	assertReasons(t, report.Assessment.ReasonCodes, ReasonLiveRuntimeUnimplemented)

	expectedCategories := []string{
		"PROVIDER_CAPABILITY",
		"OWNER_AUTHORIZATION",
		"DETERMINISTIC_RISK",
		"ACCOUNT_RECONCILIATION",
		"BROKER_KILL_SWITCH",
		"IDEMPOTENCY_RESERVATION",
		"LIVE_LIFECYCLE_CONTRACT",
	}
	if len(report.Evidence) != len(expectedCategories) {
		t.Fatalf("evidence count = %d, want %d", len(report.Evidence), len(expectedCategories))
	}
	for index, item := range report.Evidence {
		if item.Category != expectedCategories[index] || item.ID == "" || item.Digest != digestB ||
			item.Source == "" || item.ObservedAt.IsZero() || !item.MatchesEnvelope {
			t.Fatalf("evidence category %d is incomplete: %#v", index, item)
		}
	}
	for name, digest := range map[string]DigestVerification{
		"safety case": report.Digests.SafetyCase,
		"registry":    report.Digests.RegistryInput,
		"envelope":    report.Digests.Envelope,
	} {
		if !digest.Verified || digest.Claimed == "" || digest.Claimed != digest.Recomputed ||
			digest.Claimed != digest.Expected {
			t.Fatalf("%s digest is not exactly verified: %#v", name, digest)
		}
	}

	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"provider_order_id", "client_order_id", "request_payload", "response_payload",
		"credential_generation", "idempotency_key", "authorization_header", "execution_command",
	} {
		if strings.Contains(strings.ToLower(string(payload)), prohibited) {
			t.Fatalf("verification report contains protected or executable field %q: %s", prohibited, payload)
		}
	}
}

func TestSafetyCaseVerificationReportReturnsExactFailureCategories(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	baseCase := validSafetyCase(now)
	baseEnvelope, err := CompileSafetyCaseEnvelope(baseCase, now)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name           string
		expected       VerificationFailure
		mutateCase     func(*SafetyCase)
		mutateEnvelope func(*SafetyCaseCompilationEnvelope)
	}{
		{"secret-like safety input", VerificationFailureSecretLikeInput, func(value *SafetyCase) { value.Idempotency.Key = "access_token_hidden" }, nil},
		{"secret-like envelope input", VerificationFailureSecretLikeInput, nil, func(value *SafetyCaseCompilationEnvelope) { value.CompilerVersion = "private_key_hidden" }},
		{"compiler version", VerificationFailureCompilerVersion, nil, func(value *SafetyCaseCompilationEnvelope) { value.CompilerVersion = "live-safety-case-compiler-v2" }},
		{"contract version", VerificationFailureContractVersion, nil, func(value *SafetyCaseCompilationEnvelope) { value.ContractVersion = "live-execution-safety-v2" }},
		{"missing evaluation time", VerificationFailureEvaluationTimeMissing, nil, func(value *SafetyCaseCompilationEnvelope) { value.EvaluatedAt = time.Time{} }},
		{"noncanonical evaluation time", VerificationFailureEvaluationTimeCanonical, nil, func(value *SafetyCaseCompilationEnvelope) {
			value.EvaluatedAt = value.EvaluatedAt.In(time.FixedZone("offset", -4*60*60))
		}},
		{"mismatched evaluation time", VerificationFailureEvaluationTimeMismatch, nil, func(value *SafetyCaseCompilationEnvelope) {
			value.RegistryInput.ObservedAt = value.RegistryInput.ObservedAt.Add(time.Nanosecond)
		}},
		{"malformed safety digest", VerificationFailureSafetyDigestMalformed, nil, func(value *SafetyCaseCompilationEnvelope) { value.SafetyCaseDigest = "bad" }},
		{"malformed registry digest", VerificationFailureRegistryDigestMalformed, nil, func(value *SafetyCaseCompilationEnvelope) { value.RegistryInputDigest = "bad" }},
		{"malformed envelope digest", VerificationFailureEnvelopeDigestMalformed, nil, func(value *SafetyCaseCompilationEnvelope) { value.EnvelopeDigest = "bad" }},
		{"compilation rejected", VerificationFailureCompilationRejected, func(value *SafetyCase) { value.Action.ID = "bad" }, nil},
		{"safety digest mismatch", VerificationFailureSafetyDigestMismatch, func(value *SafetyCase) { value.Action.Symbol = "ETH" }, nil},
		{"registry digest mismatch", VerificationFailureRegistryDigestMismatch, nil, func(value *SafetyCaseCompilationEnvelope) { value.RegistryInput.FinancialAccountID = strategyID }},
		{"registry input mismatch", VerificationFailureRegistryInputMismatch, nil, func(value *SafetyCaseCompilationEnvelope) {
			value.RegistryInput.Evidence.Items[0], value.RegistryInput.Evidence.Items[1] = value.RegistryInput.Evidence.Items[1], value.RegistryInput.Evidence.Items[0]
		}},
		{"envelope digest mismatch", VerificationFailureEnvelopeDigestMismatch, nil, func(value *SafetyCaseCompilationEnvelope) { value.EnvelopeDigest = digestB }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			safetyCase := baseCase
			envelope := cloneSafetyCaseEnvelope(t, baseEnvelope)
			if test.mutateCase != nil {
				test.mutateCase(&safetyCase)
			}
			if test.mutateEnvelope != nil {
				test.mutateEnvelope(&envelope)
			}
			report := BuildSafetyCaseVerificationReport(safetyCase, envelope)
			if report.Status != VerificationRejected || report.Failure != test.expected || report.ExecutionAuthority {
				t.Fatalf("failure category = %s/%s, want REJECTED/%s: %#v", report.Status, report.Failure, test.expected, report)
			}
			if !errors.Is(VerifySafetyCaseCompilationEnvelope(safetyCase, envelope), ErrSafetyCaseEnvelope) {
				t.Fatalf("rejected report passed the envelope verifier: %#v", report)
			}
		})
	}
}

func TestSecretLikeVerificationReportDoesNotEchoInput(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	safetyCase := validSafetyCase(now)
	envelope, err := CompileSafetyCaseEnvelope(safetyCase, now)
	if err != nil {
		t.Fatal(err)
	}
	safetyCase.Idempotency.Key = "access_token_do_not_echo"
	envelope.CompilerVersion = "private_key_do_not_echo"
	report := BuildSafetyCaseVerificationReport(safetyCase, envelope)
	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if report.Failure != VerificationFailureSecretLikeInput ||
		strings.Contains(string(payload), "access_token_do_not_echo") ||
		strings.Contains(string(payload), "private_key_do_not_echo") {
		t.Fatalf("secret-like input was not safely redacted: %s", payload)
	}
}

func TestSafetyCaseVerificationReportHasNoPersistenceOrNetworkSurface(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("verification report test path unavailable")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "verification_report.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"context.Context", "NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient", "SubmitOrder", "CancelOrder",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("verification report unexpectedly contains persistence, provider, or broker surface %q", prohibited)
		}
	}
}

func cloneSafetyCaseEnvelope(t *testing.T, value SafetyCaseCompilationEnvelope) SafetyCaseCompilationEnvelope {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyCaseCompilationEnvelope
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(value, cloned) {
		t.Fatalf("envelope clone changed value: %#v %#v", value, cloned)
	}
	return cloned
}
