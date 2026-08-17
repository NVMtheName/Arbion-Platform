package automation

import (
	"context"
	"math/big"
	"testing"

	"github.com/arbion/platform/services/api/internal/authorization"
)

type fakeStore struct {
	account          AccountFacts
	ai               AIFacts
	bucket           CapitalBucket
	fixed            *big.Rat
	created          Mandate
	transitionStatus string
	transitionSource string
}

func (f *fakeStore) AccountFacts(context.Context, string, string) (AccountFacts, error) {
	return f.account, nil
}
func (f *fakeStore) AIFacts(context.Context, string, string, string) (AIFacts, error) {
	return f.ai, nil
}
func (f *fakeStore) CreateBucket(_ context.Context, u string, c CreateBucketCommand) (CapitalBucket, error) {
	return CapitalBucket{ID: "b", UserID: u, FinancialAccountID: c.FinancialAccountID, AllocationValue: c.AllocationValue, AllocationType: c.AllocationType, Currency: c.Currency, Status: "ACTIVE"}, nil
}
func (f *fakeStore) FixedAllocated(context.Context, string, string) (*big.Rat, error) {
	if f.fixed == nil {
		return new(big.Rat), nil
	}
	return f.fixed, nil
}
func (f *fakeStore) ListBuckets(context.Context, string) ([]CapitalBucket, error) {
	return []CapitalBucket{f.bucket}, nil
}
func (f *fakeStore) GetBucket(context.Context, string, string) (CapitalBucket, error) {
	return f.bucket, nil
}
func (f *fakeStore) UpdateBucket(context.Context, string, string, CreateBucketCommand) (CapitalBucket, error) {
	return f.bucket, nil
}
func (f *fakeStore) DeleteBucket(context.Context, string, string) error { return nil }
func (f *fakeStore) CreateMandate(_ context.Context, u string, c MandateCommand, w bool) (Mandate, error) {
	f.created = Mandate{ID: "m", UserID: u, FinancialAccountID: c.FinancialAccountID, AutomationType: c.AutomationType, CapitalBucketID: c.CapitalBucketID, AutonomyLevel: c.AutonomyLevel, ExecutionMode: c.ExecutionMode, Status: "DRAFT", CurrentVersion: 1, CapabilityUnverified: w, ExecutionCapable: false}
	return f.created, nil
}
func (f *fakeStore) ListMandates(context.Context, string) ([]Mandate, error) {
	return []Mandate{f.created}, nil
}
func (f *fakeStore) GetMandate(context.Context, string, string) (Mandate, error) {
	return f.created, nil
}
func (f *fakeStore) UpdateMandate(context.Context, string, string, int, MandateCommand, bool, string) (Mandate, error) {
	f.created.CurrentVersion++
	return f.created, nil
}
func (f *fakeStore) Transition(_ context.Context, _, _ string, _ int, status, source string) (Mandate, error) {
	f.transitionStatus = status
	f.transitionSource = source
	f.created.Status = status
	f.created.CurrentVersion++
	return f.created, nil
}
func (f *fakeStore) Versions(context.Context, string, string) ([]Version, error) {
	return []Version{{VersionNumber: 1}, {VersionNumber: 2}}, nil
}
func (f *fakeStore) Version(context.Context, string, string, int) (Version, error) {
	return Version{VersionNumber: 1}, nil
}

var founder = authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}

func baseStore() *fakeStore {
	return &fakeStore{account: AccountFacts{Owned: true, Options: "SUPPORTED"}, bucket: CapitalBucket{ID: "b", FinancialAccountID: "a", Status: "ACTIVE"}, ai: AIFacts{Owned: true, Active: true, ModelValid: true}}
}
func baseCommand() MandateCommand {
	s := "wheel"
	return MandateCommand{FinancialAccountID: "a", AutomationType: "STRATEGY", StrategyIdentifier: &s, CapitalBucketID: "b", AutonomyLevel: "CONFIRM_EACH", ExecutionMode: "LIVE"}
}

func TestBucketExactDecimalPercentageReserveAndOverlap(t *testing.T) {
	f := baseStore()
	s := NewService(f, nil)
	limit := "20.00"
	f.fixed = big.NewRat(15, 1)
	_, e := s.CreateBucket(context.Background(), founder, CreateBucketCommand{FinancialAccountID: "a", Name: "Exact", AllocationType: "FIXED_AMOUNT", AllocationValue: "10.0000000001", Currency: "USD", ProtectedAmount: "0", AllocationLimit: &limit})
	if e != ErrConflict {
		t.Fatalf("expected overlap conflict, got %v", e)
	}
	_, e = s.CreateBucket(context.Background(), founder, CreateBucketCommand{FinancialAccountID: "a", Name: "Percent", AllocationType: "PERCENT_OF_BUYING_POWER", AllocationValue: "100.01", Currency: "USD", ProtectedAmount: "0"})
	if e != ErrInvalid {
		t.Fatal("percentage above 100 accepted")
	}
	f.fixed = new(big.Rat)
	b, e := s.CreateBucket(context.Background(), founder, CreateBucketCommand{FinancialAccountID: "a", Name: "Reserve", AllocationType: "FIXED_AMOUNT", AllocationValue: "0.0000000001", Currency: "USD", ProtectedAmount: "0", IsReserve: true})
	if e != nil || b.AllocationValue != "0.0000000001" {
		t.Fatal("exact decimal was not preserved")
	}
}
func TestMandateTypesCapabilitiesAutonomyAndNoExecution(t *testing.T) {
	ctx := context.Background()
	f := baseStore()
	s := NewService(f, nil)
	c := baseCommand()
	m, e := s.Create(ctx, founder, c)
	if e != nil || m.CurrentVersion != 1 || m.ExecutionMode != "LIVE" || m.ExecutionCapable {
		t.Fatalf("LIVE configuration must remain inert: %#v %v", m, e)
	}
	c.AutomationType = "AI_AUTONOMOUS"
	c.StrategyIdentifier = nil
	ai := "ai"
	model := "model"
	c.AIProviderConnectionID = &ai
	c.AIModelID = &model
	if _, e = s.Create(ctx, founder, c); e != nil {
		t.Fatalf("valid AI mandate rejected: %v", e)
	}
	c.AutomationType = "HYBRID"
	wheel := "wheel"
	c.StrategyIdentifier = &wheel
	if _, e = s.Create(ctx, founder, c); e != nil {
		t.Fatalf("valid hybrid rejected: %v", e)
	}
	f.ai.Active = false
	if _, e = s.Create(ctx, founder, c); e != ErrInvalid {
		t.Fatal("inactive AI connection accepted")
	}
	f.ai.Active = true
	c.AutomationType = "STRATEGY"
	c.AIProviderConnectionID = nil
	c.AIModelID = nil
	c.AutonomyLevel = "FULL_AUTONOMOUS"
	if _, e = s.validate(ctx, founder, c, true); e != ErrInvalid {
		t.Fatal("invalid autonomy/type combination accepted as ready")
	}
	f.account.Options = "UNSUPPORTED"
	if _, e = s.Create(ctx, founder, c); e != ErrInvalid {
		t.Fatal("unsupported options capability accepted")
	}
	f.account.Options = "UNKNOWN"
	c.AutonomyLevel = "CONFIRM_EACH"
	m, e = s.Create(ctx, founder, c)
	if e != nil || !m.CapabilityUnverified {
		t.Fatal("unknown capability did not produce warning")
	}
}
func TestOwnershipEntitlementReserveAndDurability(t *testing.T) {
	f := baseStore()
	s := NewService(f, nil)
	c := baseCommand()
	if _, e := s.Create(context.Background(), authorization.Principal{UserID: "u", Role: authorization.RoleSuperadmin, Entitlement: authorization.EntitlementFree}, c); e != ErrForbidden {
		t.Fatal("admin role granted product entitlement")
	}
	f.account.Owned = false
	if _, e := s.Create(context.Background(), founder, c); e != ErrNotFound {
		t.Fatal("cross-user account accepted")
	}
	f.account.Owned = true
	f.bucket.IsReserve = true
	if _, e := s.Create(context.Background(), founder, c); e != ErrInvalid {
		t.Fatal("reserve attached to mandate")
	}
	f.bucket.IsReserve = false
	m, _ := s.Create(context.Background(), founder, c)
	before := m.CurrentVersion
	f.created = m
	updated, _ := s.Update(context.Background(), founder, m.ID, before, c)
	if updated.CurrentVersion != 2 {
		t.Fatal("update did not create version 2")
	}
	versions, _ := s.Versions(context.Background(), founder, m.ID)
	if len(versions) != 2 {
		t.Fatal("durable history missing")
	}
}

func TestReadyPaperWheelAllowsUnknownOptionsCapability(t *testing.T) {
	f := baseStore()
	f.account.Options = "UNKNOWN"
	wheel := "wheel"
	f.created = Mandate{
		ID:                   "m",
		UserID:               founder.UserID,
		FinancialAccountID:   "a",
		AutomationType:       "STRATEGY",
		StrategyIdentifier:   &wheel,
		CapitalBucketID:      "b",
		AutonomyLevel:        "RESEARCH_ONLY",
		ExecutionMode:        "PAPER",
		Status:               "DRAFT",
		CurrentVersion:       1,
		StrategyParameters:   []byte(`{}`),
		ScheduleConditions:   []byte(`{}`),
		OptionsAllowed:       true,
		CapabilityUnverified: true,
	}

	ready, err := NewService(f, nil).Transition(context.Background(), founder, f.created.ID, 1, "READY")
	if err != nil {
		t.Fatalf("safe PAPER wheel readiness was rejected: %v", err)
	}
	if ready.Status != "READY" || ready.CurrentVersion != 2 {
		t.Fatalf("unexpected READY transition: %#v", ready)
	}
	if f.transitionStatus != "READY" || f.transitionSource != "UI" {
		t.Fatalf("unexpected transition metadata: status=%q source=%q", f.transitionStatus, f.transitionSource)
	}
}
