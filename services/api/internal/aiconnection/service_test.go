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
	items      map[string]Connection
	preference *Preference
}

func (ms *memoryStore) List(_ context.Context, user string) ([]Connection, error) {
	out := []Connection{}
	for _, c := range ms.items {
		out = append(out, c)
	}
	return out, nil
}
func (ms *memoryStore) Create(_ context.Context, user, p, n, h string) (Connection, error) {
	c := Connection{ID: "connection-1", Provider: p, DisplayName: n, Status: "pending", Enabled: true, CredentialHint: h, CreatedAt: time.Now(), UpdatedAt: time.Now()}
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
	c, e := ms.Get(context.Background(), user, id)
	c.Status = status
	c.Enabled = status != "disabled"
	ms.items[id] = c
	return c, e
}
func (ms *memoryStore) SetCredentialPending(_ context.Context, user, id, hint string) (Connection, error) {
	c, e := ms.SetStatus(context.Background(), user, id, "pending")
	c.CredentialHint = hint
	ms.items[id] = c
	return c, e
}
func (ms *memoryStore) SetVerification(_ context.Context, user, id, status string, verified bool) (Connection, error) {
	c, e := ms.SetStatus(context.Background(), user, id, status)
	if verified {
		now := time.Now()
		c.LastVerifiedAt = &now
		ms.items[id] = c
	}
	return c, e
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
	if _, ok := ms.items[id]; !ok {
		return ErrNotFound
	}
	delete(ms.items, id)
	return nil
}
func (ms *memoryStore) HasDependencies(context.Context, string, string) (bool, error) {
	return false, nil
}

type blobs struct{ data map[string][]byte }

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
	return nil
}

type audit struct{}

func (audit) Record(context.Context, *string, string, map[string]any) error { return nil }
func setup(t *testing.T) (*Service, *memoryStore, *blobs) {
	t.Helper()
	ms := &memoryStore{items: map[string]Connection{}}
	bs := &blobs{data: map[string][]byte{}}
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
}

func (f fakeNeural) Verify(context.Context, string, []byte) error { return f.err }
func (f fakeNeural) Models(context.Context, string, []byte) ([]neural.Model, error) {
	return []neural.Model{{ID: "model-1", Provider: "openai"}}, f.err
}
func (f fakeNeural) Analyze(_ context.Context, _, _ string, credential []byte, _ string, safetyID string) (neural.Insight, error) {
	if f.seenCredential != nil {
		*f.seenCredential = string(credential)
	}
	if f.seenSecret != nil {
		*f.seenSecret = credential
	}
	if f.seenSafetyID != nil {
		*f.seenSafetyID = safetyID
	}
	return f.insight, f.err
}

type fakeLimiter struct {
	allowed bool
	err     error
}

func (f fakeLimiter) Allow(context.Context, string, int, time.Duration) (bool, error) {
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
	seenCredential, seenSafetyID := "", ""
	var seenSecret []byte
	s.neural = fakeNeural{
		insight: neural.Insight{
			Summary:  "Diversification reduces concentration risk.",
			Metadata: neural.InsightMetadata{Provider: "openai", Model: "model-1", InputUsage: &inputUsage, OutputUsage: &outputUsage, RequestID: "resp-safe"},
		},
		seenCredential: &seenCredential,
		seenSecret:     &seenSecret,
		seenSafetyID:   &seenSafetyID,
	}
	s.limiter = fakeLimiter{allowed: true}
	recorded := &recordedAudit{}
	s.audit = recorded
	prompt := "Explain diversification without live data."
	result, err := s.Analyze(context.Background(), p, prompt)
	if err != nil || result.Summary == "" {
		t.Fatalf("analysis failed: %#v %v", result, err)
	}
	if seenCredential != "secret-value" || len(seenSafetyID) != 64 {
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
	if _, err := s.Analyze(context.Background(), p, "question"); !errors.Is(err, ErrRateLimit) {
		t.Fatalf("rate limit did not fail closed: %v", err)
	}
}

func TestAnalyzeRejectsMissingPreferenceAndOversizedInput(t *testing.T) {
	s, _, _ := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	if _, err := s.Analyze(context.Background(), p, "question"); !errors.Is(err, ErrInactive) {
		t.Fatalf("missing preference accepted: %v", err)
	}
	if _, err := s.Analyze(context.Background(), p, strings.Repeat("x", MaxPromptBytes+1)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized prompt accepted: %v", err)
	}
}
