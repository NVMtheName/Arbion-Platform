package aiconnection

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/neural"
)

type memoryStore struct {
	items           map[string]Connection
	preference      *Preference
	dependencies    bool
	inUse           bool
	dependencyCalls int
	inUseCalls      int
	lockCalls       int
	lockDepth       int
	dependencyLock  bool
	inUseLock       bool
	statusLock      bool
	deleteLock      bool
	blobs           *blobs
}

func (ms *memoryStore) List(_ context.Context, user string) ([]Connection, error) {
	out := []Connection{}
	for _, c := range ms.items {
		out = append(out, c)
	}
	return out, nil
}
func (ms *memoryStore) Create(_ context.Context, user, p, n, h string) (Connection, error) {
	c := Connection{ID: "connection-1", Provider: p, DisplayName: n, Status: "pending", Enabled: true, CredentialHint: h, CreatedAt: time.Now(), UpdatedAt: time.Now(), CredentialGeneration: 1}
	ms.items[c.ID] = c
	return c, nil
}
func (ms *memoryStore) Get(_ context.Context, user, id string) (Connection, error) {
	c, ok := ms.items[id]
	if !ok {
		return Connection{}, ErrNotFound
	}
	return c, nil
}
func (ms *memoryStore) Rename(_ context.Context, user, id, n string) (Connection, error) {
	c, e := ms.Get(context.Background(), user, id)
	c.DisplayName = n
	ms.items[id] = c
	return c, e
}
func (ms *memoryStore) SetStatus(_ context.Context, user, id, status string) (Connection, error) {
	if status == "disabled" && ms.lockDepth > 0 {
		ms.statusLock = true
	}
	c, e := ms.Get(context.Background(), user, id)
	c.Status = status
	c.Enabled = status != "disabled"
	ms.items[id] = c
	return c, e
}
func (ms *memoryStore) SetVerification(_ context.Context, user, id, status string, verified bool, generation int64) (Connection, error) {
	current, e := ms.Get(context.Background(), user, id)
	if e != nil || current.CredentialGeneration != generation {
		return Connection{}, ErrNotFound
	}
	c, e := ms.SetStatus(context.Background(), user, id, status)
	if verified {
		now := time.Now()
		c.LastVerifiedAt = &now
		ms.items[id] = c
	}
	return c, e
}
func (ms *memoryStore) CommitStagedCredential(_ context.Context, user, id, token, hint, expectedStatus, nextStatus string, generation int64, verified bool) (Connection, error) {
	c, err := ms.Get(context.Background(), user, id)
	if err != nil || c.Status != expectedStatus || c.CredentialGeneration != generation || ms.blobs == nil {
		return Connection{}, ErrNotFound
	}
	pending, ok := ms.blobs.pending[id]
	if !ok || pending.token != token {
		return Connection{}, ErrNotFound
	}
	ms.blobs.data[id] = append([]byte(nil), pending.payload...)
	delete(ms.blobs.pending, id)
	c.Status = nextStatus
	c.Enabled = nextStatus != "disabled"
	c.CredentialHint = hint
	c.CredentialGeneration++
	if verified {
		now := time.Now()
		c.LastVerifiedAt = &now
	} else {
		c.LastVerifiedAt = nil
	}
	ms.items[id] = c
	return c, nil
}
func (ms *memoryStore) GetPreference(context.Context, string) (*Preference, error) {
	return ms.preference, nil
}
func (ms *memoryStore) SetPreference(_ context.Context, user, id, model string) (Preference, error) {
	pref := Preference{ConnectionID: id, ModelID: model, UpdatedAt: time.Now()}
	ms.preference = &pref
	return pref, nil
}
func (ms *memoryStore) Delete(_ context.Context, user, id string) error {
	if ms.lockDepth > 0 {
		ms.deleteLock = true
	}
	if _, ok := ms.items[id]; !ok {
		return ErrNotFound
	}
	delete(ms.items, id)
	return nil
}
func (ms *memoryStore) HasDependencies(context.Context, string, string) (bool, error) {
	ms.dependencyCalls++
	if ms.lockDepth > 0 {
		ms.dependencyLock = true
	}
	return ms.dependencies, nil
}
func (ms *memoryStore) ConnectionInUse(context.Context, string, string) (bool, error) {
	ms.inUseCalls++
	if ms.lockDepth > 0 {
		ms.inUseLock = true
	}
	return ms.inUse, nil
}
func (ms *memoryStore) WithLock(_ context.Context, _ string, fn func() error) error {
	ms.lockCalls++
	ms.lockDepth++
	defer func() { ms.lockDepth-- }()
	return fn()
}

type blobs struct {
	data    map[string][]byte
	pending map[string]stagedBlob
}

type stagedBlob struct {
	payload []byte
	token   string
}

func (b *blobs) Put(_ context.Context, l credential.Locator, p []byte, create bool) error {
	if create {
		if _, ok := b.data[l.ConnectionID]; ok {
			return errors.New("exists")
		}
	}
	b.data[l.ConnectionID] = append([]byte(nil), p...)
	return nil
}
func (b *blobs) Get(_ context.Context, l credential.Locator) ([]byte, error) {
	p, ok := b.data[l.ConnectionID]
	if !ok {
		return nil, credential.ErrNotFound
	}
	return append([]byte(nil), p...), nil
}
func (b *blobs) Delete(_ context.Context, l credential.Locator) error {
	if _, ok := b.data[l.ConnectionID]; !ok {
		return credential.ErrNotFound
	}
	delete(b.data, l.ConnectionID)
	delete(b.pending, l.ConnectionID)
	return nil
}
func (b *blobs) PutStaged(_ context.Context, l credential.Locator, payload []byte, token string) error {
	if _, ok := b.data[l.ConnectionID]; !ok {
		return credential.ErrNotFound
	}
	b.pending[l.ConnectionID] = stagedBlob{payload: append([]byte(nil), payload...), token: token}
	return nil
}
func (b *blobs) DeleteStaged(_ context.Context, l credential.Locator, token string) error {
	pending, ok := b.pending[l.ConnectionID]
	if !ok || pending.token != token {
		return credential.ErrNotFound
	}
	delete(b.pending, l.ConnectionID)
	return nil
}

type audit struct{}

func (audit) Record(context.Context, *string, string, map[string]any) error { return nil }
func setup(t *testing.T) (*Service, *memoryStore, *blobs) {
	t.Helper()
	bs := &blobs{data: map[string][]byte{}, pending: map[string]stagedBlob{}}
	ms := &memoryStore{items: map[string]Connection{}, blobs: bs}
	v, e := credential.NewEncryptedVault(make([]byte, 32), bs)
	if e != nil {
		t.Fatal(e)
	}
	return NewService(ms, v, audit{}, DefaultRegistry(), nil, nil), ms, bs
}

type fakeNeural struct {
	err            error
	insight        neural.Insight
	seenCredential *string
	seenSecret     *[]byte
	seenSafetyID   *string
	seenProfile    *string
	onVerify       func()
}

type proposalNeural struct {
	fakeNeural
	proposal     neural.TradeProposal
	request      *neural.TradeProposalRequest
	seenSecret   *[]byte
	seenSafetyID *string
}

type shadowNeural struct {
	fakeNeural
	decision     neural.ShadowDecision
	request      *neural.ShadowDecisionRequest
	seenSecret   *[]byte
	seenSafetyID *string
}

func (fake shadowNeural) ProposeShadow(_ context.Context, _ string, credential []byte, request neural.ShadowDecisionRequest, safetyIdentifier string) (neural.ShadowDecision, error) {
	if fake.request != nil {
		*fake.request = request
	}
	if fake.seenSecret != nil {
		*fake.seenSecret = credential
	}
	if fake.seenSafetyID != nil {
		*fake.seenSafetyID = safetyIdentifier
	}
	return fake.decision, fake.err
}

func (fake proposalNeural) ProposeTrade(_ context.Context, _ string, credential []byte, request neural.TradeProposalRequest, safetyIdentifier string) (neural.TradeProposal, error) {
	if fake.request != nil {
		*fake.request = request
	}
	if fake.seenSecret != nil {
		*fake.seenSecret = credential
	}
	if fake.seenSafetyID != nil {
		*fake.seenSafetyID = safetyIdentifier
	}
	return fake.proposal, fake.err
}

func (f fakeNeural) Verify(_ context.Context, _ string, credential []byte) error {
	if f.seenSecret != nil {
		*f.seenSecret = append((*f.seenSecret)[:0], credential...)
	}
	if f.onVerify != nil {
		f.onVerify()
	}
	return f.err
}
func (f fakeNeural) Models(context.Context, string, []byte) ([]neural.Model, error) {
	return []neural.Model{{ID: "model-1", Provider: "openai"}}, f.err
}
func (f fakeNeural) Analyze(_ context.Context, _, profile string, credential []byte, _ string, safetyID string) (neural.Insight, error) {
	if f.seenCredential != nil {
		*f.seenCredential = string(credential)
	}
	if f.seenSecret != nil {
		*f.seenSecret = credential
	}
	if f.seenSafetyID != nil {
		*f.seenSafetyID = safetyID
	}
	if f.seenProfile != nil {
		*f.seenProfile = profile
	}
	return f.insight, f.err
}

type fakeLimiter struct {
	allowed bool
	err     error
	calls   *int
	cost    *int
}

func (f fakeLimiter) AllowWeighted(_ context.Context, _ string, cost, _ int, _ time.Duration) (bool, error) {
	if f.calls != nil {
		(*f.calls)++
	}
	if f.cost != nil {
		*f.cost = cost
	}
	return f.allowed, f.err
}

type recordedAudit struct {
	actions  []string
	metadata []map[string]any
}

func (a *recordedAudit) Record(_ context.Context, _ *string, action string, metadata map[string]any) error {
	a.actions = append(a.actions, action)
	a.metadata = append(a.metadata, metadata)
	return nil
}
func TestVerificationTransitionsAndPreservesCredential(t *testing.T) {
	s, ms, bs := setup(t)
	s.neural = fakeNeural{}
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	c, e := s.Create(context.Background(), p, "openai", "Mine", []byte("secret-value"))
	if e != nil {
		t.Fatal(e)
	}
	verified, e := s.Verify(context.Background(), p, c.ID)
	if e != nil {
		t.Fatal(e)
	}
	if verified.Status != "active" || verified.LastVerifiedAt == nil {
		t.Fatal("verification did not activate and timestamp connection")
	}
	cipher := string(bs.data[c.ID])
	s.neural = fakeNeural{err: &neural.ProviderError{Code: neural.AuthenticationFailed}}
	_, e = s.Verify(context.Background(), p, c.ID)
	if neural.Code(e) != neural.AuthenticationFailed || ms.items[c.ID].Status != "error" {
		t.Fatal("failed verification was not normalized")
	}
	if string(bs.data[c.ID]) != cipher {
		t.Fatal("failed verification removed credential")
	}
}
func TestDisabledVerificationRejected(t *testing.T) {
	s, _, _ := setup(t)
	s.neural = fakeNeural{}
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	c, _ := s.Create(context.Background(), p, "openai", "Mine", []byte("secret-value"))
	_, _ = s.SetEnabled(context.Background(), p, c.ID, false)
	if _, e := s.Verify(context.Background(), p, c.ID); !errors.Is(e, ErrDisabled) {
		t.Fatal("disabled connection verified")
	}
}
func TestPreferenceRequiresActiveDiscoveredModel(t *testing.T) {
	s, _, _ := setup(t)
	s.neural = fakeNeural{}
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	c, _ := s.Create(context.Background(), p, "openai", "Mine", []byte("secret-value"))
	if _, e := s.SetPreference(context.Background(), p, c.ID, "model-1"); !errors.Is(e, ErrInactive) {
		t.Fatal("pending connection selected")
	}
	_, _ = s.Verify(context.Background(), p, c.ID)
	pref, e := s.SetPreference(context.Background(), p, c.ID, "model-1")
	if e != nil || pref.ModelID != "model-1" {
		t.Fatal("active model preference rejected")
	}
}
func TestEntitlementIsIndependentOfAdminRole(t *testing.T) {
	s, _, _ := setup(t)
	admin := authorization.Principal{UserID: "u", Role: authorization.RoleAdmin, Entitlement: authorization.EntitlementFree}
	if _, e := s.Create(context.Background(), admin, "openai", "Mine", []byte("secret-value")); !errors.Is(e, ErrForbidden) {
		t.Fatal("admin role granted product access")
	}
	founder := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	if _, e := s.Create(context.Background(), founder, "openai", "Mine", []byte("secret-value")); e != nil {
		t.Fatalf("founder rejected: %v", e)
	}
}
func TestCredentialLifecycleIsEncryptedAndSafe(t *testing.T) {
	s, ms, bs := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	secret := []byte("plaintext-secret-AB12")
	c, e := s.Create(context.Background(), p, "openai", "Mine", secret)
	if e != nil {
		t.Fatal(e)
	}
	cipher := append([]byte(nil), bs.data[c.ID]...)
	if string(cipher) == string(secret) {
		t.Fatal("plaintext persisted")
	}
	if c.CredentialHint != "••••••••AB12" {
		t.Fatalf("unsafe hint: %q", c.CredentialHint)
	}
	if _, e = s.SetEnabled(context.Background(), p, c.ID, false); e != nil {
		t.Fatal(e)
	}
	if string(bs.data[c.ID]) != string(cipher) {
		t.Fatal("disable changed vault credential")
	}
	if _, e = s.Replace(context.Background(), p, c.ID, []byte("replacement-ZZ99")); e != nil {
		t.Fatal(e)
	}
	if string(bs.data[c.ID]) == string(cipher) {
		t.Fatal("replacement did not change ciphertext")
	}
	if e = s.Delete(context.Background(), p, c.ID); e != nil {
		t.Fatal(e)
	}
	if _, ok := bs.data[c.ID]; ok {
		t.Fatal("delete retained vault secret")
	}
	if _, ok := ms.items[c.ID]; ok {
		t.Fatal("delete retained connection")
	}
	if ms.lockCalls != 2 || !ms.inUseLock || !ms.statusLock || !ms.dependencyLock || !ms.deleteLock {
		t.Fatalf("destructive lifecycle checks and writes were not serialized: locks=%d in_use=%t status=%t dependency=%t delete=%t", ms.lockCalls, ms.inUseLock, ms.statusLock, ms.dependencyLock, ms.deleteLock)
	}
}

func TestActiveCredentialRotationKeepsCurrentKeyUntilCandidateVerificationSucceeds(t *testing.T) {
	s, ms, bs := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	c, err := s.Create(context.Background(), p, "openai", "Mine", []byte("original-secret-AB12"))
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), bs.data[c.ID]...)
	ms.items[c.ID] = Connection{ID: c.ID, Provider: "openai", Status: "active", Enabled: true, CredentialHint: "••••••••AB12", CredentialGeneration: 1}
	ms.inUse = true
	currentDuringVerification := ""
	var candidateDuringVerification []byte
	s.neural = fakeNeural{seenSecret: &candidateDuringVerification, onVerify: func() {
		current, retrieveErr := s.vault.Retrieve(context.Background(), credential.Locator{ConnectionID: c.ID, UserID: p.UserID, Class: credential.AI})
		if retrieveErr != nil {
			t.Fatal(retrieveErr)
		}
		defer clear(current)
		currentDuringVerification = string(current)
	}}

	rotated, err := s.Replace(context.Background(), p, c.ID, []byte("replacement-ZZ99"))
	if err != nil {
		t.Fatal(err)
	}
	if currentDuringVerification != "original-secret-AB12" {
		t.Fatalf("candidate replaced the runtime credential before verification: %q", currentDuringVerification)
	}
	if string(candidateDuringVerification) != "replacement-ZZ99" {
		t.Fatalf("provider verified the wrong credential: %q", candidateDuringVerification)
	}
	if string(bs.data[c.ID]) == string(before) || rotated.Status != "active" || rotated.CredentialHint != "••••••••ZZ99" || rotated.CredentialGeneration != 2 || rotated.LastVerifiedAt == nil {
		t.Fatalf("verified candidate was not atomically activated: %#v", rotated)
	}
	if len(bs.pending) != 0 || ms.inUseCalls != 0 {
		t.Fatalf("rotation retained staged material or required downtime: pending=%d in_use_checks=%d", len(bs.pending), ms.inUseCalls)
	}
	if _, err = s.SetEnabled(context.Background(), p, c.ID, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("in-use disable was not blocked: %v", err)
	}
	if ms.items[c.ID].Status != "active" || !ms.items[c.ID].Enabled {
		t.Fatal("blocked disable changed the credential or connection state")
	}
	if ms.inUseCalls != 1 {
		t.Fatalf("expected disable to check runtime use once, got %d calls", ms.inUseCalls)
	}
	if ms.lockCalls != 1 || !ms.inUseLock {
		t.Fatalf("runtime dependency check did not run under the lifecycle lock: locks=%d locked=%t", ms.lockCalls, ms.inUseLock)
	}
}

func TestFailedActiveCredentialCandidatePreservesVerifiedConnection(t *testing.T) {
	s, ms, bs := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	c, err := s.Create(context.Background(), p, "openai", "Mine", []byte("original-secret-AB12"))
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), bs.data[c.ID]...)
	verifiedAt := time.Now().Add(-time.Hour)
	ms.items[c.ID] = Connection{ID: c.ID, Provider: "openai", Status: "active", Enabled: true, CredentialHint: "••••••••AB12", CredentialGeneration: 1, LastVerifiedAt: &verifiedAt}
	s.neural = fakeNeural{err: &neural.ProviderError{Code: neural.AuthenticationFailed}}

	if _, err = s.Replace(context.Background(), p, c.ID, []byte("rejected-key-ZZ99")); neural.Code(err) != neural.AuthenticationFailed {
		t.Fatalf("failed candidate was not normalized: %v", err)
	}
	retained := ms.items[c.ID]
	if string(bs.data[c.ID]) != string(before) || retained.Status != "active" || retained.CredentialHint != "••••••••AB12" || retained.CredentialGeneration != 1 || retained.LastVerifiedAt == nil || !retained.LastVerifiedAt.Equal(verifiedAt) {
		t.Fatalf("failed candidate changed the verified connection: %#v", retained)
	}
	if len(bs.pending) != 0 {
		t.Fatal("failed candidate retained encrypted staging material")
	}
}

func TestStaleVerificationCannotOverwriteNewerCredentialGeneration(t *testing.T) {
	s, ms, _ := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	c, err := s.Create(context.Background(), p, "openai", "Mine", []byte("original-secret-AB12"))
	if err != nil {
		t.Fatal(err)
	}
	s.neural = fakeNeural{onVerify: func() {
		newer := ms.items[c.ID]
		newer.Status = "active"
		newer.CredentialHint = "••••••••NEW2"
		newer.CredentialGeneration = 2
		ms.items[c.ID] = newer
	}}

	if _, err = s.Verify(context.Background(), p, c.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale verification did not fail with a state conflict: %v", err)
	}
	retained := ms.items[c.ID]
	if retained.Status != "active" || retained.CredentialHint != "••••••••NEW2" || retained.CredentialGeneration != 2 {
		t.Fatalf("stale verification overwrote newer credential state: %#v", retained)
	}
}

func TestDeleteFailsClosedForDurableDependencies(t *testing.T) {
	s, ms, bs := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	c, err := s.Create(context.Background(), p, "openai", "Mine", []byte("original-secret-AB12"))
	if err != nil {
		t.Fatal(err)
	}
	ms.dependencies = true
	if err = s.Delete(context.Background(), p, c.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("dependent connection deletion was not blocked: %v", err)
	}
	if bs.data[c.ID] == nil {
		t.Fatal("blocked deletion removed the credential")
	}
	if _, ok := ms.items[c.ID]; !ok {
		t.Fatal("blocked deletion removed the connection")
	}
	if ms.dependencyCalls != 1 {
		t.Fatalf("expected one durable dependency check, got %d", ms.dependencyCalls)
	}
	if ms.lockCalls != 1 || !ms.dependencyLock {
		t.Fatalf("durable dependency check did not run under the lifecycle lock: locks=%d locked=%t", ms.lockCalls, ms.dependencyLock)
	}
}
func TestOwnershipUsesNotFound(t *testing.T) {
	s, ms, _ := setup(t)
	ms.items["other"] = Connection{ID: "other", Provider: "openai"}
	delete(ms.items, "other") // an ownership-filtered store reveals no row
	p := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}
	if _, e := s.Rename(context.Background(), p, "other", "x"); !errors.Is(e, ErrNotFound) {
		t.Fatal("foreign identifier was revealed")
	}
	if e := s.Delete(context.Background(), p, "other"); !errors.Is(e, ErrNotFound) {
		t.Fatal("foreign delete was not hidden")
	}
}

func TestAnalyzeUsesActivePreferenceAndAuditsMetadataOnly(t *testing.T) {
	s, ms, bs := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	s.neural = fakeNeural{}
	c, err := s.Create(context.Background(), p, "openai", "Mine", []byte("secret-value"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Verify(context.Background(), p, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetPreference(context.Background(), p, c.ID, "model-1"); err != nil {
		t.Fatal(err)
	}
	inputUsage, outputUsage := 30, 45
	seenCredential, seenSafetyID, seenProfile := "", "", ""
	var seenSecret []byte
	s.neural = fakeNeural{
		insight: neural.Insight{
			Summary:  "Diversification reduces concentration risk.",
			Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-luna", Profile: "fast", InputUsage: &inputUsage, OutputUsage: &outputUsage, RequestID: "resp-safe"},
		},
		seenCredential: &seenCredential,
		seenSecret:     &seenSecret,
		seenSafetyID:   &seenSafetyID,
		seenProfile:    &seenProfile,
	}
	s.limiter = fakeLimiter{allowed: true}
	recorded := &recordedAudit{}
	s.audit = recorded
	prompt := "Explain diversification without live data."
	result, err := s.Analyze(context.Background(), p, prompt, "fast")
	if err != nil || result.Summary == "" {
		t.Fatalf("analysis failed: %#v %v", result, err)
	}
	if seenCredential != "secret-value" || len(seenSafetyID) != 64 || seenProfile != "fast" {
		t.Fatal("credential or privacy-preserving safety identifier was not passed correctly")
	}
	for _, value := range seenSecret {
		if value != 0 {
			t.Fatal("retrieved plaintext credential was not cleared after use")
		}
	}
	if strings.Contains(string(bs.data[c.ID]), "secret-value") {
		t.Fatal("vault persisted plaintext credential")
	}
	if ms.preference == nil || ms.preference.ModelID != "model-1" {
		t.Fatal("saved preference changed during analysis")
	}
	raw, err := json.Marshal(recorded.metadata)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), prompt) || strings.Contains(string(raw), "secret-value") || !strings.Contains(string(raw), "resp-safe") {
		t.Fatalf("audit metadata was unsafe or incomplete: %s", raw)
	}
}

func TestAnalyzeFailsClosedOnRateLimit(t *testing.T) {
	s, ms, _ := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	ms.items["connection-1"] = Connection{ID: "connection-1", Provider: "openai", Status: "active"}
	ms.preference = &Preference{ConnectionID: "connection-1", ModelID: "model-1"}
	s.neural = fakeNeural{}
	s.limiter = fakeLimiter{allowed: false}
	if _, err := s.Analyze(context.Background(), p, "question", "fast"); !errors.Is(err, ErrRateLimit) {
		t.Fatalf("rate limit did not fail closed: %v", err)
	}
}

func TestAnalyzeRejectsMissingPreferenceAndOversizedInput(t *testing.T) {
	s, _, _ := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	if _, err := s.Analyze(context.Background(), p, "question", ""); !errors.Is(err, ErrInactive) {
		t.Fatalf("missing preference accepted: %v", err)
	}
	if _, err := s.Analyze(context.Background(), p, strings.Repeat("x", MaxPromptBytes+1), "fast"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized prompt accepted: %v", err)
	}
	if _, err := s.Analyze(context.Background(), p, "question", "unknown"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown profile accepted: %v", err)
	}
}

func TestAnalyzeProfilesConsumeWeightedCredits(t *testing.T) {
	for _, test := range []struct {
		name    string
		profile string
		model   string
		units   int
	}{
		{name: "default", profile: "", model: "gpt-5.6-luna", units: 1},
		{name: "fast", profile: "fast", model: "gpt-5.6-luna", units: 1},
		{name: "core", profile: "core", model: "gpt-5.6-terra", units: 3},
		{name: "deep", profile: "deep", model: "gpt-5.6-sol", units: 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _, _ := setup(t)
			p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
			s.neural = fakeNeural{}
			connection, err := s.Create(context.Background(), p, "openai", "Mine", []byte("secret-value"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Verify(context.Background(), p, connection.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = s.SetPreference(context.Background(), p, connection.ID, "model-1"); err != nil {
				t.Fatal(err)
			}
			calls := 0
			cost := 0
			s.limiter = fakeLimiter{allowed: true, calls: &calls, cost: &cost}
			resolvedProfile := test.profile
			if resolvedProfile == "" {
				resolvedProfile = "fast"
			}
			s.neural = fakeNeural{insight: neural.Insight{Summary: "ok", Metadata: neural.InsightMetadata{Provider: "openai", Model: test.model, Profile: resolvedProfile}}}

			if _, err := s.Analyze(context.Background(), p, "question", test.profile); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || cost != test.units {
				t.Fatalf("profile used %d limiter calls with cost %d, want one call with cost %d", calls, cost, test.units)
			}
		})
	}
}

func TestAnalyzeRejectsMismatchedRouteMetadata(t *testing.T) {
	s, _, _ := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	s.neural = fakeNeural{}
	connection, err := s.Create(context.Background(), p, "openai", "Mine", []byte("secret-value"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Verify(context.Background(), p, connection.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetPreference(context.Background(), p, connection.ID, "model-1"); err != nil {
		t.Fatal(err)
	}
	s.limiter = fakeLimiter{allowed: true}
	s.neural = fakeNeural{insight: neural.Insight{Summary: "unsafe mismatch", Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "fast"}}}
	recorded := &recordedAudit{}
	s.audit = recorded

	if _, err = s.Analyze(context.Background(), p, "question", "fast"); neural.Code(err) != neural.InternalError {
		t.Fatalf("mismatched route metadata was accepted: %v", err)
	}
	if len(recorded.actions) != 1 || recorded.actions[0] != "neural_insight.failed" {
		t.Fatalf("mismatched route was not audited safely: %#v", recorded.actions)
	}
}

func TestGenerateTradeProposalReusesEncryptedPreferenceAndAuditsMetadataOnly(t *testing.T) {
	s, _, bs := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	s.neural = fakeNeural{}
	connection, err := s.Create(context.Background(), p, "openai", "Mine", []byte("secret-value"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Verify(context.Background(), p, connection.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetPreference(context.Background(), p, connection.ID, "model-1"); err != nil {
		t.Fatal(err)
	}
	seenRequest := neural.TradeProposalRequest{}
	seenSecret := []byte{}
	seenSafetyID := ""
	s.neural = proposalNeural{fakeNeural: fakeNeural{}, proposal: neural.TradeProposal{
		Decision: "PROPOSE", RequestedSize: "25.50", Confidence: "LOW", Thesis: "Bounded proposal.",
		Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-terra", Profile: "core", RequestID: "resp-proposal"},
	}, request: &seenRequest, seenSecret: &seenSecret, seenSafetyID: &seenSafetyID}
	s.limiter = fakeLimiter{allowed: true}
	recorded := &recordedAudit{}
	s.audit = recorded
	input := neural.TradeProposalRequest{Profile: "core", Objective: "private objective", Symbol: "BTC", Side: "BUY", MaxSize: "50", MaxSizeUnit: "USD", AvailableCash: "200", PositionQuantity: "0.012", PositionAvailableQuantity: "0.01", ObservedAt: time.Now().UTC()}
	proposal, err := s.GenerateTradeProposal(context.Background(), p, input)
	if err != nil || proposal.Decision != "PROPOSE" || seenRequest.Objective != input.Objective || len(seenSafetyID) != 64 {
		t.Fatalf("trade proposal failed: proposal=%#v request=%#v err=%v", proposal, seenRequest, err)
	}
	for _, value := range seenSecret {
		if value != 0 {
			t.Fatal("retrieved plaintext credential was not cleared after use")
		}
	}
	if strings.Contains(string(bs.data[connection.ID]), "secret-value") {
		t.Fatal("vault persisted plaintext credential")
	}
	raw, err := json.Marshal(recorded.metadata)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), input.Objective) || strings.Contains(string(raw), "secret-value") || !strings.Contains(string(raw), "resp-proposal") {
		t.Fatalf("proposal audit metadata was unsafe or incomplete: %s", raw)
	}
}

func TestGenerateShadowDecisionUsesExactMandateConnectionAndModelWithoutPreference(t *testing.T) {
	s, ms, _ := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	s.neural = fakeNeural{}
	connection, err := s.Create(context.Background(), p, "openai", "Mandate model", []byte("secret-value"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Verify(context.Background(), p, connection.ID); err != nil {
		t.Fatal(err)
	}
	ms.preference = nil
	seenRequest := neural.ShadowDecisionRequest{}
	seenSecret := []byte{}
	seenSafetyID := ""
	s.neural = shadowNeural{decision: neural.ShadowDecision{Decision: "ABSTAIN", Symbol: "NONE", Side: "NONE", ProposedNotional: "0", Confidence: "LOW", Thesis: "No edge", Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep", RequestID: "resp-shadow"}}, request: &seenRequest, seenSecret: &seenSecret, seenSafetyID: &seenSafetyID}
	s.limiter = fakeLimiter{allowed: true}
	input := neural.ShadowDecisionRequest{Objective: "private objective", AllowedSymbols: []string{"BTC"}, MaxProposalNotional: "1", AvailableCashUSD: "100", BuyingPowerUSD: "100", ObservedAt: time.Now().UTC()}
	decision, err := s.GenerateShadowDecision(context.Background(), p, connection.ID, "gpt-5.6-sol", input)
	if err != nil || decision.Decision != "ABSTAIN" || seenRequest.Profile != "deep" || len(seenSafetyID) != 64 {
		t.Fatalf("shadow decision failed: decision=%#v request=%#v err=%v", decision, seenRequest, err)
	}
	for _, value := range seenSecret {
		if value != 0 {
			t.Fatal("retrieved plaintext credential was not cleared")
		}
	}
	if _, err = s.GenerateShadowDecision(context.Background(), p, connection.ID, "unapproved-model", input); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unapproved mandate model was accepted: %v", err)
	}
}

func TestGenerateShadowDecisionRoutesExactClaudeMandateModel(t *testing.T) {
	s, ms, _ := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	s.neural = fakeNeural{}
	connection, err := s.Create(context.Background(), p, "anthropic", "Mandate model", []byte("secret-value"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Verify(context.Background(), p, connection.ID); err != nil {
		t.Fatal(err)
	}
	seenRequest := neural.ShadowDecisionRequest{}
	s.neural = shadowNeural{decision: neural.ShadowDecision{Decision: "ABSTAIN", Symbol: "NONE", Side: "NONE", ProposedNotional: "0", Confidence: "LOW", Thesis: "No edge", Metadata: neural.InsightMetadata{Provider: "anthropic", Model: "claude-sonnet-5", Profile: "core", RequestID: "msg-shadow"}}, request: &seenRequest}
	s.limiter = fakeLimiter{allowed: true}
	input := neural.ShadowDecisionRequest{Objective: "private objective", AllowedSymbols: []string{"BTC"}, MaxProposalNotional: "1", AvailableCashUSD: "100", BuyingPowerUSD: "100", ObservedAt: time.Now().UTC()}
	decision, err := s.GenerateShadowDecision(context.Background(), p, connection.ID, "claude-sonnet-5", input)
	if err != nil || decision.Decision != "ABSTAIN" || seenRequest.Profile != "core" {
		t.Fatalf("Claude shadow decision failed: decision=%#v request=%#v err=%v", decision, seenRequest, err)
	}
	ms.items[connection.ID] = Connection{ID: connection.ID, Provider: "openai", Status: "active"}
	if _, err = s.GenerateShadowDecision(context.Background(), p, connection.ID, "claude-sonnet-5", input); neural.Code(err) != neural.Unsupported {
		t.Fatalf("provider/model mismatch was accepted: %v", err)
	}
}

func TestGenerateShadowDecisionRoutesExactGeminiMandateModel(t *testing.T) {
	s, ms, _ := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	s.neural = fakeNeural{}
	connection, err := s.Create(context.Background(), p, "gemini", "Mandate model", []byte("secret-value"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Verify(context.Background(), p, connection.ID); err != nil {
		t.Fatal(err)
	}
	seenRequest := neural.ShadowDecisionRequest{}
	s.neural = shadowNeural{decision: neural.ShadowDecision{Decision: "ABSTAIN", Symbol: "NONE", Side: "NONE", ProposedNotional: "0", Confidence: "LOW", Thesis: "No edge", Metadata: neural.InsightMetadata{Provider: "gemini", Model: "gemini-3.6-flash", Profile: "core", RequestID: "interaction-shadow"}}, request: &seenRequest}
	s.limiter = fakeLimiter{allowed: true}
	input := neural.ShadowDecisionRequest{Objective: "private objective", AllowedSymbols: []string{"BTC"}, MaxProposalNotional: "1", AvailableCashUSD: "100", BuyingPowerUSD: "100", ObservedAt: time.Now().UTC()}
	decision, err := s.GenerateShadowDecision(context.Background(), p, connection.ID, "gemini-3.6-flash", input)
	if err != nil || decision.Decision != "ABSTAIN" || seenRequest.Profile != "core" {
		t.Fatalf("Gemini shadow decision failed: decision=%#v request=%#v err=%v", decision, seenRequest, err)
	}
	ms.items[connection.ID] = Connection{ID: connection.ID, Provider: "anthropic", Status: "active"}
	if _, err = s.GenerateShadowDecision(context.Background(), p, connection.ID, "gemini-3.6-flash", input); neural.Code(err) != neural.Unsupported {
		t.Fatalf("provider/model mismatch was accepted: %v", err)
	}
}
