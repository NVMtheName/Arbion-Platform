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

func TestCompileSafetyCaseEnvelopeIsDeterministicCompleteAndRegistryCompatible(t *testing.T) {
	now := time.Date(2026, 9, 7, 20, 0, 0, 123, time.UTC)
	safetyCase := validSafetyCase(now)
	first, err := CompileSafetyCaseEnvelope(safetyCase, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompileSafetyCaseEnvelope(safetyCase, now)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("envelope replay changed: first=%#v second=%#v err=%v", first, second, err)
	}
	if first.CompilerVersion != SafetyCaseCompilerVersion || first.ContractVersion != ContractVersion ||
		!first.EvaluatedAt.Equal(now) || !digestPattern.MatchString(first.SafetyCaseDigest) ||
		!digestPattern.MatchString(first.RegistryInputDigest) || !digestPattern.MatchString(first.EnvelopeDigest) {
		t.Fatalf("envelope identity or digest changed: %#v", first)
	}
	if first.SafetyCaseDigest != "8f1a3d71826d558c6cfd3bf926681f323dbcfb7422373cb8e83cffc5d1608a98" ||
		first.RegistryInputDigest != "6cf38cd406fdfbc44b091d22d7f022ce8a7172eb01a2570fd94c7a34847824bf" ||
		first.EnvelopeDigest != "aca1c3388de2af996d2231c789b7a39ee407de55d7e4eb3c27131f75d431cc88" {
		t.Fatalf("fixed canonical envelope changed: %#v", first)
	}
	if first.RegistryInput.Assessment.Availability != BlockedUnimplemented ||
		!first.RegistryInput.Assessment.Structural {
		t.Fatalf("envelope gained executable authority: %#v", first.RegistryInput.Assessment)
	}
	if err = VerifySafetyCaseCompilationEnvelope(safetyCase, first); err != nil {
		t.Fatalf("exact envelope did not verify: %v", err)
	}
	prepared, _, _, err := prepareRegistryRecord(userID, first.RegistryInput, now)
	if err != nil || prepared.ContentDigest == "" || prepared.EvidenceDigest == "" {
		t.Fatalf("enveloped registry input is not compatible: %#v %v", prepared, err)
	}
	payload, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"provider_order_id", "client_order_id", "submit_order", "create_order",
		"request_payload", "execution_command", "authorization_header",
	} {
		if strings.Contains(strings.ToLower(string(payload)), prohibited) {
			t.Fatalf("envelope contains prohibited field %q: %s", prohibited, payload)
		}
	}
}

func TestSafetyCaseDigestBindsEveryTypedLeaf(t *testing.T) {
	now := time.Date(2026, 9, 7, 20, 0, 0, 0, time.UTC)
	original := validSafetyCase(now)
	originalDigest, err := canonicalSafetyCaseDigest(original)
	if err != nil {
		t.Fatal(err)
	}
	paths := safetyCaseLeafPaths(reflect.ValueOf(original), nil)
	if len(paths) < 60 {
		t.Fatalf("safety case leaf coverage unexpectedly small: %d", len(paths))
	}
	for _, path := range paths {
		mutated := original
		mutateSafetyCaseLeaf(reflect.ValueOf(&mutated).Elem(), path)
		digest, digestErr := canonicalSafetyCaseDigest(mutated)
		if digestErr != nil {
			t.Fatalf("mutated leaf %v could not be digested: %v", path, digestErr)
		}
		if digest == originalDigest {
			t.Fatalf("mutating typed safety-case leaf %v did not change digest", path)
		}
	}
}

func TestSafetyCaseEnvelopeNormalizesSemanticTimeAndRegistryOrdering(t *testing.T) {
	now := time.Date(2026, 9, 7, 20, 0, 0, 123, time.UTC)
	utcCase := validSafetyCase(now)
	offsetCase := utcCase
	offset := time.FixedZone("review-offset", -4*60*60)
	setSafetyCaseTimeLocation(reflect.ValueOf(&offsetCase).Elem(), offset)
	utcEnvelope, err := CompileSafetyCaseEnvelope(utcCase, now)
	if err != nil {
		t.Fatal(err)
	}
	offsetEnvelope, err := CompileSafetyCaseEnvelope(offsetCase, now.In(offset))
	if err != nil {
		t.Fatal(err)
	}
	if utcEnvelope.SafetyCaseDigest != offsetEnvelope.SafetyCaseDigest ||
		utcEnvelope.RegistryInputDigest != offsetEnvelope.RegistryInputDigest ||
		utcEnvelope.EnvelopeDigest != offsetEnvelope.EnvelopeDigest {
		t.Fatalf("equivalent instants changed canonical digests: %#v %#v", utcEnvelope, offsetEnvelope)
	}

	unordered := utcEnvelope.RegistryInput
	for left, right := 0, len(unordered.Evidence.Items)-1; left < right; left, right = left+1, right-1 {
		unordered.Evidence.Items[left], unordered.Evidence.Items[right] = unordered.Evidence.Items[right], unordered.Evidence.Items[left]
	}
	canonicalDigest, err := canonicalRegistryInputDigest(utcEnvelope.RegistryInput)
	if err != nil {
		t.Fatal(err)
	}
	unorderedDigest, err := canonicalRegistryInputDigest(unordered)
	if err != nil || canonicalDigest != unorderedDigest {
		t.Fatalf("semantic evidence ordering changed digest: %s %s %v", canonicalDigest, unorderedDigest, err)
	}
}

func TestVerifySafetyCaseCompilationEnvelopeRejectsEveryEnvelopeTamper(t *testing.T) {
	now := time.Date(2026, 9, 7, 20, 0, 0, 0, time.UTC)
	safetyCase := validSafetyCase(now)
	original, err := CompileSafetyCaseEnvelope(safetyCase, now)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*SafetyCaseCompilationEnvelope)
	}{
		{"compiler version", func(value *SafetyCaseCompilationEnvelope) { value.CompilerVersion = "live-safety-case-compiler-v2" }},
		{"contract version", func(value *SafetyCaseCompilationEnvelope) { value.ContractVersion = "live-execution-safety-v2" }},
		{"evaluation time", func(value *SafetyCaseCompilationEnvelope) { value.EvaluatedAt = value.EvaluatedAt.Add(time.Nanosecond) }},
		{"noncanonical evaluation zone", func(value *SafetyCaseCompilationEnvelope) {
			value.EvaluatedAt = value.EvaluatedAt.In(time.FixedZone("offset", -4*60*60))
		}},
		{"safety digest", func(value *SafetyCaseCompilationEnvelope) { value.SafetyCaseDigest = digestB }},
		{"registry digest", func(value *SafetyCaseCompilationEnvelope) { value.RegistryInputDigest = digestB }},
		{"envelope digest", func(value *SafetyCaseCompilationEnvelope) { value.EnvelopeDigest = digestB }},
		{"registry identity", func(value *SafetyCaseCompilationEnvelope) { value.RegistryInput.FinancialAccountID = strategyID }},
		{"registry assessment", func(value *SafetyCaseCompilationEnvelope) { value.RegistryInput.Assessment.Availability = Unavailable }},
		{"registry reason", func(value *SafetyCaseCompilationEnvelope) {
			value.RegistryInput.Assessment.ReasonCodes[0] = ReasonInvalidEvidence
		}},
		{"registry evidence", func(value *SafetyCaseCompilationEnvelope) { value.RegistryInput.Evidence.Items[0].Digest = digestA }},
		{"registry order", func(value *SafetyCaseCompilationEnvelope) {
			value.RegistryInput.Evidence.Items[0], value.RegistryInput.Evidence.Items[1] = value.RegistryInput.Evidence.Items[1], value.RegistryInput.Evidence.Items[0]
		}},
		{"registry observed time", func(value *SafetyCaseCompilationEnvelope) {
			value.RegistryInput.ObservedAt = value.RegistryInput.ObservedAt.Add(time.Nanosecond)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := original
			mutated.RegistryInput.Assessment.ReasonCodes = append([]ReasonCode(nil), original.RegistryInput.Assessment.ReasonCodes...)
			mutated.RegistryInput.Evidence.Items = append([]EvidenceItem(nil), original.RegistryInput.Evidence.Items...)
			test.mutate(&mutated)
			if !errors.Is(VerifySafetyCaseCompilationEnvelope(safetyCase, mutated), ErrSafetyCaseEnvelope) {
				t.Fatalf("tampered envelope verified: %#v", mutated)
			}
		})
	}
}

func TestCompileSafetyCaseEnvelopeRejectsIncompleteOrSecretLikeInput(t *testing.T) {
	now := time.Date(2026, 9, 7, 20, 0, 0, 0, time.UTC)
	for name, mutate := range map[string]func(*SafetyCase){
		"missing lifecycle evidence": func(value *SafetyCase) { value.Lifecycle.Evidence = ImmutableEvidenceRef{} },
		"missing action identity":    func(value *SafetyCase) { value.Action.ID = "" },
		"future evidence":            func(value *SafetyCase) { value.Risk.Evidence.RecordedAt = now.Add(time.Second) },
		"secret-like mfa value":      func(value *SafetyCase) { value.OwnerAuthorization.MFAMethod = "API_KEY_PROOF" },
		"secret-like idempotency":    func(value *SafetyCase) { value.Idempotency.Key = "authorization_header_01" },
	} {
		t.Run(name, func(t *testing.T) {
			safetyCase := validSafetyCase(now)
			mutate(&safetyCase)
			envelope, compileErr := CompileSafetyCaseEnvelope(safetyCase, now)
			if !errors.Is(compileErr, ErrSafetyCaseEnvelope) || !reflect.DeepEqual(envelope, SafetyCaseCompilationEnvelope{}) {
				t.Fatalf("unsafe envelope input did not fail closed: %#v %v", envelope, compileErr)
			}
		})
	}
}

func safetyCaseLeafPaths(value reflect.Value, prefix []int) [][]int {
	if value.Type() == reflect.TypeOf(time.Time{}) {
		return [][]int{append([]int(nil), prefix...)}
	}
	if value.Kind() != reflect.Struct {
		return [][]int{append([]int(nil), prefix...)}
	}
	paths := [][]int{}
	for index := 0; index < value.NumField(); index++ {
		paths = append(paths, safetyCaseLeafPaths(value.Field(index), append(append([]int(nil), prefix...), index))...)
	}
	return paths
}

func mutateSafetyCaseLeaf(root reflect.Value, path []int) {
	value := root
	for _, index := range path {
		value = value.Field(index)
	}
	if value.Type() == reflect.TypeOf(time.Time{}) {
		value.Set(reflect.ValueOf(value.Interface().(time.Time).Add(time.Nanosecond)))
		return
	}
	switch value.Kind() {
	case reflect.String:
		value.SetString(value.String() + "x")
	case reflect.Bool:
		value.SetBool(!value.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value.SetInt(value.Int() + 1)
	default:
		panic("unhandled safety-case leaf kind: " + value.Kind().String())
	}
}

func setSafetyCaseTimeLocation(value reflect.Value, location *time.Location) {
	if value.Type() == reflect.TypeOf(time.Time{}) {
		value.Set(reflect.ValueOf(value.Interface().(time.Time).In(location)))
		return
	}
	if value.Kind() != reflect.Struct {
		return
	}
	for index := 0; index < value.NumField(); index++ {
		setSafetyCaseTimeLocation(value.Field(index), location)
	}
}
