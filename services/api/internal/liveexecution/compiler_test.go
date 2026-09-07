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

func TestCompileSafetyCaseAssessmentIsCanonicalCompleteAndNonExecutable(t *testing.T) {
	now := time.Date(2026, 9, 7, 19, 0, 0, 123, time.UTC)
	first, err := CompileSafetyCaseAssessment(validSafetyCase(now), now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompileSafetyCaseAssessment(validSafetyCase(now), now)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("semantic replay did not compile deterministically: first=%#v second=%#v err=%v", first, second, err)
	}
	if first.AssessmentKey != "live-safety-11111111111141118111111111111111" ||
		first.FinancialAccountID != accountID || first.ProviderConnectionID != connectionID ||
		first.ProviderName != "coinbase" || first.StrategyInstanceID != strategyID ||
		first.MandateID != mandateID || first.MandateVersion != 4 ||
		first.CapitalBucketID != bucketID || first.CapitalReservationID != reservationID ||
		first.ActionDigest != digestA || !first.ObservedAt.Equal(now) {
		t.Fatalf("compiled identity changed: %#v", first)
	}
	if first.Assessment.Availability != BlockedUnimplemented || !first.Assessment.Structural {
		t.Fatalf("compiler produced executable authority: %#v", first.Assessment)
	}
	assertReasons(t, first.Assessment.ReasonCodes, ReasonLiveRuntimeUnimplemented)

	expected := map[string]EvidenceSource{
		"PROVIDER_CAPABILITY":     EvidenceProviderVerified,
		"OWNER_AUTHORIZATION":     EvidenceOwnerMFA,
		"DETERMINISTIC_RISK":      EvidenceDeterministicControl,
		"ACCOUNT_RECONCILIATION":  EvidenceDatabase,
		"BROKER_KILL_SWITCH":      EvidenceDeterministicControl,
		"IDEMPOTENCY_RESERVATION": EvidenceDatabase,
		"LIVE_LIFECYCLE_CONTRACT": EvidenceDesignContract,
	}
	if len(first.Evidence.Items) != len(expected) {
		t.Fatalf("compiled evidence count = %d, want %d", len(first.Evidence.Items), len(expected))
	}
	seen := map[string]bool{}
	for _, item := range first.Evidence.Items {
		if seen[item.Kind] || expected[item.Kind] != item.Source || item.ID == "" || item.Digest != digestB || item.ObservedAt.IsZero() {
			t.Fatalf("compiled evidence category changed: %#v", item)
		}
		seen[item.Kind] = true
	}
	prepared, _, _, err := prepareRegistryRecord(userID, first, now)
	if err != nil || prepared.Assessment.Availability != BlockedUnimplemented {
		t.Fatalf("compiler output is not registry compatible: %#v %v", prepared, err)
	}
	payload, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{"provider_order_id", "client_order_id", "submit_order", "create_order", "credential", "request_payload", "execution_command"} {
		if strings.Contains(strings.ToLower(string(payload)), prohibited) {
			t.Fatalf("compiled registry input contains executable or protected field %q: %s", prohibited, payload)
		}
	}
}

func TestCompileSafetyCaseAssessmentPreservesUnavailableWithoutGrantingAuthority(t *testing.T) {
	now := time.Date(2026, 9, 7, 19, 0, 0, 0, time.UTC)
	safetyCase := validSafetyCase(now)
	safetyCase.ProviderCapability.TradeAllowed = false
	compiled, err := CompileSafetyCaseAssessment(safetyCase, now)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Assessment.Availability != Unavailable || !compiled.Assessment.Structural {
		t.Fatalf("unavailable case gained authority: %#v", compiled.Assessment)
	}
	assertContainsReasons(t, compiled.Assessment.ReasonCodes, ReasonProviderCapabilityUnavailable)
}

func TestCompileSafetyCaseAssessmentRejectsUnregistrableEvidence(t *testing.T) {
	now := time.Date(2026, 9, 7, 19, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*SafetyCase)
	}{
		{"missing lifecycle evidence", func(s *SafetyCase) { s.Lifecycle.Evidence = ImmutableEvidenceRef{} }},
		{"duplicate evidence", func(s *SafetyCase) { s.Lifecycle.Evidence.ID = s.Risk.Evidence.ID }},
		{"cross-bound owner", func(s *SafetyCase) { s.OwnerAuthorization.UserID = accountID }},
		{"action digest mismatch", func(s *SafetyCase) { s.Idempotency.ActionDigest = digestB }},
		{"future evidence", func(s *SafetyCase) { s.Lifecycle.Evidence.RecordedAt = now.Add(time.Second) }},
		{"malformed evidence digest", func(s *SafetyCase) { s.KillSwitch.Evidence.Digest = "bad" }},
		{"category mismatch", func(s *SafetyCase) { s.ProviderCapability.Evidence.Kind = "OWNER_AUTHORIZATION" }},
		{"secret-like category", func(s *SafetyCase) { s.ProviderCapability.Evidence.Kind = "API_KEY_PROOF" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			safetyCase := validSafetyCase(now)
			test.mutate(&safetyCase)
			compiled, err := CompileSafetyCaseAssessment(safetyCase, now)
			if !errors.Is(err, ErrSafetyCaseCompilation) || !reflect.DeepEqual(compiled, RegistryInput{}) {
				t.Fatalf("unregistrable evidence did not fail closed: %#v %v", compiled, err)
			}
		})
	}
}

func TestLifecycleContractRequiresExactImmutableEvidence(t *testing.T) {
	now := time.Date(2026, 9, 7, 19, 0, 0, 0, time.UTC)
	for name, mutate := range map[string]func(*SafetyCase){
		"missing":    func(s *SafetyCase) { s.Lifecycle.Evidence = ImmutableEvidenceRef{} },
		"wrong kind": func(s *SafetyCase) { s.Lifecycle.Evidence.Kind = "DESIGN_CONTRACT" },
		"pre-action": func(s *SafetyCase) { s.Lifecycle.Evidence.RecordedAt = s.Action.CreatedAt.Add(-time.Second) },
	} {
		t.Run(name, func(t *testing.T) {
			safetyCase := validSafetyCase(now)
			mutate(&safetyCase)
			assessment := (UnavailableBoundary{}).Assess(safetyCase, now)
			if assessment.Availability != Unavailable {
				t.Fatalf("invalid lifecycle evidence was accepted: %#v", assessment)
			}
			assertContainsReasons(t, assessment.ReasonCodes, ReasonLifecycleContractUnavailable)
		})
	}
}

func TestSafetyCaseCompilerHasNoPersistenceOrNetworkSurface(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("compiler test path unavailable")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "compiler.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("compiler unexpectedly contains persistence or network surface %q", prohibited)
		}
	}
}
