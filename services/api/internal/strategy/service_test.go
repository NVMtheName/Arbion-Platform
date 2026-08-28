package strategy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
)

type journalPersistenceFake struct {
	entries            []JournalActivity
	scheduleRuns       []ScheduleRun
	requestedUser      string
	requestedLimit     int
	requestedAfter     *JournalCursor
	requestedRunAfter  *ScheduleRunCursor
	lifecycleID        string
	lifecycle          LifecycleCommand
	lifecycleAt        time.Time
	lifecycleResult    LifecycleResult
	lifecycleError     error
	instance           Instance
	getError           error
	portfolio          PaperPortfolio
	portfolioError     error
	portfolioCalls     int
	scorecard          ShadowScorecard
	scorecardError     error
	scorecardCalls     int
	requestedID        string
	initializedUser    string
	initializedCash    string
	initializedMandate automation.Mandate
	pausedUser         string
	pausedID           string
	pausedVersion      int
	pausedAt           time.Time
	pauseResult        Instance
	pauseError         error
	resumedUser        string
	resumedID          string
	resumedVersion     int
	resumedAt          time.Time
	resumeResult       Instance
	resumeError        error
	finishedUser       string
	finishedID         string
	finishedVersion    int
	finishedAt         time.Time
	finishResult       Instance
	finishError        error
}

func (f *journalPersistenceFake) Initialize(_ context.Context, userID string, mandate automation.Mandate, cash string, state State) (Instance, error) {
	f.initializedUser = userID
	f.initializedCash = cash
	f.initializedMandate = mandate
	return Instance{UserID: userID, AutomationMandateID: mandate.ID, FinancialAccountID: mandate.FinancialAccountID, CapitalBucketID: mandate.CapitalBucketID, CurrentState: state}, nil
}
func (f *journalPersistenceFake) Pause(_ context.Context, userID, instanceID string, expectedVersion int, at time.Time) (Instance, error) {
	f.pausedUser = userID
	f.pausedID = instanceID
	f.pausedVersion = expectedVersion
	f.pausedAt = at
	return f.pauseResult, f.pauseError
}
func (f *journalPersistenceFake) Resume(_ context.Context, userID, instanceID string, expectedVersion int, at time.Time) (Instance, error) {
	f.resumedUser = userID
	f.resumedID = instanceID
	f.resumedVersion = expectedVersion
	f.resumedAt = at
	return f.resumeResult, f.resumeError
}
func (f *journalPersistenceFake) Finish(_ context.Context, userID, instanceID string, expectedVersion int, at time.Time) (Instance, error) {
	f.finishedUser = userID
	f.finishedID = instanceID
	f.finishedVersion = expectedVersion
	f.finishedAt = at
	return f.finishResult, f.finishError
}
func (*journalPersistenceFake) List(context.Context, string) ([]Instance, error) { return nil, nil }
func (f *journalPersistenceFake) Get(_ context.Context, userID, instanceID string) (Instance, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	return f.instance, f.getError
}
func (*journalPersistenceFake) History(context.Context, string, string) ([]Transition, error) {
	return nil, nil
}
func (*journalPersistenceFake) Decisions(context.Context, string, string) ([]DecisionJournalEntry, error) {
	return nil, nil
}
func (*journalPersistenceFake) Executions(context.Context, string, string) ([]ExecutionRecord, error) {
	return nil, nil
}
func (*journalPersistenceFake) ShadowOutcomes(context.Context, string, string) ([]ShadowOutcome, error) {
	return nil, nil
}
func (f *journalPersistenceFake) ShadowScorecard(_ context.Context, userID, instanceID string) (ShadowScorecard, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	f.scorecardCalls++
	return f.scorecard, f.scorecardError
}
func (f *journalPersistenceFake) PaperPortfolio(_ context.Context, userID, instanceID string) (PaperPortfolio, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	f.portfolioCalls++
	return f.portfolio, f.portfolioError
}
func (f *journalPersistenceFake) Journal(_ context.Context, userID string, limit int, after *JournalCursor) ([]JournalActivity, error) {
	f.requestedUser = userID
	f.requestedLimit = limit
	f.requestedAfter = after
	if len(f.entries) < limit {
		limit = len(f.entries)
	}
	return f.entries[:limit], nil
}
func (*journalPersistenceFake) Schedule(context.Context, string, string) (ScheduleStatus, error) {
	return ScheduleStatus{}, nil
}
func (f *journalPersistenceFake) ScheduleRuns(_ context.Context, userID, instanceID string, limit int, after *ScheduleRunCursor) ([]ScheduleRun, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	f.requestedLimit = limit
	f.requestedRunAfter = after
	if len(f.scheduleRuns) < limit {
		limit = len(f.scheduleRuns)
	}
	return f.scheduleRuns[:limit], nil
}
func (f *journalPersistenceFake) RecordLifecycle(_ context.Context, userID, instanceID string, command LifecycleCommand, occurredAt time.Time) (LifecycleResult, error) {
	f.requestedUser = userID
	f.lifecycleID = instanceID
	f.lifecycle = command
	f.lifecycleAt = occurredAt
	return f.lifecycleResult, f.lifecycleError
}

type instanceMandatesFake struct {
	mandate automation.Mandate
	bucket  automation.CapitalBucket
}

type strategyAuditFake struct {
	userID   *string
	action   string
	metadata map[string]any
}

func (f *strategyAuditFake) Record(_ context.Context, userID *string, action string, metadata map[string]any) error {
	f.userID = userID
	f.action = action
	f.metadata = metadata
	return nil
}

func (f *instanceMandatesFake) Get(context.Context, authorization.Principal, string) (automation.Mandate, error) {
	return f.mandate, nil
}

func (f *instanceMandatesFake) GetBucket(context.Context, authorization.Principal, string) (automation.CapitalBucket, error) {
	return f.bucket, nil
}

func TestPaperInitializationIsBoundToProtectedBucketCapacity(t *testing.T) {
	wheel := "wheel"
	mandates := &instanceMandatesFake{
		mandate: automation.Mandate{ID: "mandate", UserID: "owner", FinancialAccountID: "account", CapitalBucketID: "bucket", AutomationType: "STRATEGY", StrategyIdentifier: &wheel, Status: "READY", ExecutionMode: "PAPER"},
		bucket:  automation.CapitalBucket{ID: "bucket", UserID: "owner", FinancialAccountID: "account", AllocationType: "FIXED_AMOUNT", AllocationValue: "100", ProtectedAmount: "20", Status: "ACTIVE"},
	}
	store := &journalPersistenceFake{}
	service := NewInstanceService(store, mandates)
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}

	instance, err := service.Initialize(context.Background(), principal, "mandate", "80")
	if err != nil || store.initializedCash != "80" || instance.CapitalBucketID != "bucket" {
		t.Fatalf("authorized paper capacity was not bound: instance=%#v store=%#v err=%v", instance, store, err)
	}
	store.initializedCash = ""
	if _, err = service.Initialize(context.Background(), principal, "mandate", "80.0000000001"); !errors.Is(err, ErrCapitalLimit) || store.initializedCash != "" {
		t.Fatalf("over-capacity paper cash reached persistence: cash=%q err=%v", store.initializedCash, err)
	}

	limit := "75"
	mandates.bucket.AllocationLimit = &limit
	if _, err = service.Initialize(context.Background(), principal, "mandate", "55.0000000001"); !errors.Is(err, ErrCapitalLimit) {
		t.Fatalf("absolute bucket limit was not enforced: %v", err)
	}
	mandates.bucket.AllocationType = "PERCENT_OF_AVAILABLE_CASH"
	mandates.bucket.AllocationLimit = nil
	if _, err = service.Initialize(context.Background(), principal, "mandate", "1"); !errors.Is(err, ErrCapitalLimit) {
		t.Fatalf("unbounded percentage bucket initialized PAPER: %v", err)
	}
}

func TestShadowInitializationBindsBucketWithoutCreatingPaperCash(t *testing.T) {
	wheel := "wheel"
	mandates := &instanceMandatesFake{
		mandate: automation.Mandate{ID: "mandate", UserID: "owner", FinancialAccountID: "account", CapitalBucketID: "bucket", AutomationType: "STRATEGY", StrategyIdentifier: &wheel, Status: "READY", ExecutionMode: "SHADOW"},
		bucket:  automation.CapitalBucket{ID: "bucket", UserID: "owner", FinancialAccountID: "account", AllocationType: "PERCENT_OF_AVAILABLE_CASH", AllocationValue: "25", ProtectedAmount: "0", Status: "ACTIVE"},
	}
	store := &journalPersistenceFake{}
	service := NewInstanceService(store, mandates)
	instance, err := service.Initialize(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "mandate", "untrusted")
	if err != nil || store.initializedCash != "" || instance.CapitalBucketID != "bucket" {
		t.Fatalf("shadow bucket binding changed: instance=%#v cash=%q err=%v", instance, store.initializedCash, err)
	}
}

func TestAIShadowInitializationCreatesMonitoringStateWithoutPaperCash(t *testing.T) {
	connection, model := "ai", "gpt-5.6-sol"
	mandates := &instanceMandatesFake{
		mandate: automation.Mandate{ID: "ai-mandate", UserID: "owner", FinancialAccountID: "account", CapitalBucketID: "bucket", AutomationType: "AI_AUTONOMOUS", AIProviderConnectionID: &connection, AIModelID: &model, Status: "READY", AutonomyLevel: "FULL_AUTONOMOUS", ExecutionMode: "SHADOW", StrategyParameters: []byte(`{"objective":"Preserve capital.","max_proposal_notional":"1"}`)},
		bucket:  automation.CapitalBucket{ID: "bucket", UserID: "owner", FinancialAccountID: "account", AllocationType: "FIXED_AMOUNT", AllocationValue: "10", ProtectedAmount: "0", Status: "ACTIVE"},
	}
	store := &journalPersistenceFake{}
	instance, err := NewInstanceService(store, mandates).Initialize(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "ai-mandate", "999")
	if err != nil || instance.CurrentState != AIMonitoring || store.initializedCash != "" {
		t.Fatalf("AI shadow initialization was not isolated: instance=%#v cash=%q err=%v", instance, store.initializedCash, err)
	}
}

func TestFinishRequiresExplicitConfirmationAndAuditsReleasedClaim(t *testing.T) {
	now := time.Date(2026, 8, 19, 13, 0, 0, 0, time.UTC)
	store := &journalPersistenceFake{finishResult: Instance{ID: "instance", UserID: "owner", AutomationMandateID: "mandate", FinancialAccountID: "account", ExecutionMode: Paper, Status: "COMPLETED", StateVersion: 4}}
	audit := &strategyAuditFake{}
	service := NewInstanceService(store, nil, audit)
	service.now = func() time.Time { return now }
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}

	if _, err := service.Finish(context.Background(), principal, "instance", 3, false); !errors.Is(err, ErrInvalid) || store.finishedID != "" {
		t.Fatalf("unconfirmed finish reached persistence: id=%q err=%v", store.finishedID, err)
	}
	instance, err := service.Finish(context.Background(), principal, "instance", 3, true)
	if err != nil || instance.Status != "COMPLETED" || store.finishedUser != "owner" || store.finishedID != "instance" || store.finishedVersion != 3 || !store.finishedAt.Equal(now) {
		t.Fatalf("confirmed finish was not owner-scoped: instance=%#v store=%#v err=%v", instance, store, err)
	}
	if audit.userID == nil || *audit.userID != "owner" || audit.action != "strategy_instance.completed" || audit.metadata["strategy_instance_id"] != "instance" || audit.metadata["account_id"] != "account" {
		t.Fatalf("finish audit evidence is incomplete: %#v", audit)
	}
}

func TestPauseAndResumeAreOwnerScopedAndAudited(t *testing.T) {
	now := time.Date(2026, 8, 19, 13, 30, 0, 0, time.UTC)
	store := &journalPersistenceFake{
		pauseResult:  Instance{ID: "instance", UserID: "owner", AutomationMandateID: "mandate", FinancialAccountID: "account", ExecutionMode: Paper, Status: "PAUSED", StateVersion: 4},
		resumeResult: Instance{ID: "instance", UserID: "owner", AutomationMandateID: "mandate", FinancialAccountID: "account", ExecutionMode: Paper, Status: "ACTIVE", StateVersion: 5},
	}
	audit := &strategyAuditFake{}
	service := NewInstanceService(store, nil, audit)
	service.now = func() time.Time { return now }
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}

	paused, err := service.Pause(context.Background(), principal, "instance", 3)
	if err != nil || paused.Status != "PAUSED" || store.pausedUser != "owner" || store.pausedID != "instance" || store.pausedVersion != 3 || !store.pausedAt.Equal(now) {
		t.Fatalf("pause was not owner-scoped: instance=%#v store=%#v err=%v", paused, store, err)
	}
	if audit.userID == nil || *audit.userID != "owner" || audit.action != "strategy_instance.paused" || audit.metadata["strategy_instance_id"] != "instance" {
		t.Fatalf("pause audit evidence is incomplete: %#v", audit)
	}
	if _, err = service.Resume(context.Background(), principal, "instance", 4, false); !errors.Is(err, ErrInvalid) || store.resumedID != "" {
		t.Fatalf("unconfirmed resume reached persistence: id=%q err=%v", store.resumedID, err)
	}
	resumed, err := service.Resume(context.Background(), principal, "instance", 4, true)
	if err != nil || resumed.Status != "ACTIVE" || store.resumedUser != "owner" || store.resumedID != "instance" || store.resumedVersion != 4 || !store.resumedAt.Equal(now) {
		t.Fatalf("resume was not owner-scoped: instance=%#v store=%#v err=%v", resumed, store, err)
	}
	if audit.action != "strategy_instance.resumed" || audit.metadata["account_id"] != "account" {
		t.Fatalf("resume audit evidence is incomplete: %#v", audit)
	}
}

func TestJournalIsOwnerScopedAndBuildsStableNextCursor(t *testing.T) {
	now := time.Date(2026, 8, 18, 18, 0, 0, 0, time.UTC)
	store := &journalPersistenceFake{entries: []JournalActivity{
		{ID: "11111111-1111-4111-8111-111111111111", CreatedAt: now},
		{ID: "22222222-2222-4222-8222-222222222222", CreatedAt: now.Add(-time.Minute)},
		{ID: "33333333-3333-4333-8333-333333333333", CreatedAt: now.Add(-2 * time.Minute)},
	}}
	service := NewInstanceService(store, nil)
	page, err := service.Journal(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if store.requestedUser != "owner" || store.requestedLimit != 3 {
		t.Fatalf("owner or lookahead was not preserved: user=%q limit=%d", store.requestedUser, store.requestedLimit)
	}
	if len(page.Entries) != 2 || page.NextCursor == nil || page.NextCursor.ID != page.Entries[1].ID || !page.NextCursor.CreatedAt.Equal(page.Entries[1].CreatedAt) {
		t.Fatalf("unexpected page: %#v", page)
	}
}

func TestJournalRequiresAutomationEntitlement(t *testing.T) {
	store := &journalPersistenceFake{}
	service := NewInstanceService(store, nil)
	_, err := service.Journal(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, 25, nil)
	if !errors.Is(err, ErrForbidden) || store.requestedUser != "" {
		t.Fatalf("unentitled journal request was not rejected: %v", err)
	}
}

func TestScheduleRunsAreOwnerScopedAndBuildStableNextCursor(t *testing.T) {
	now := time.Date(2026, 8, 28, 1, 0, 0, 0, time.UTC)
	store := &journalPersistenceFake{
		instance: Instance{ID: "instance-1", UserID: "owner"},
		scheduleRuns: []ScheduleRun{
			{ID: "11111111-1111-4111-8111-111111111111", ScheduledFor: now},
			{ID: "22222222-2222-4222-8222-222222222222", ScheduledFor: now.Add(-time.Hour)},
			{ID: "33333333-3333-4333-8333-333333333333", ScheduledFor: now.Add(-2 * time.Hour)},
		},
	}
	service := NewInstanceService(store, nil)
	page, err := service.ScheduleRuns(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if store.requestedUser != "owner" || store.requestedID != "instance-1" || store.requestedLimit != 3 {
		t.Fatalf("schedule-run owner boundary changed: %#v", store)
	}
	if len(page.Runs) != 2 || page.NextCursor == nil || page.NextCursor.ID != page.Runs[1].ID || !page.NextCursor.ScheduledFor.Equal(page.Runs[1].ScheduledFor) {
		t.Fatalf("unexpected schedule-run page: %#v", page)
	}
}

func TestScheduleRunsRequireAutomationEntitlementAndValidBounds(t *testing.T) {
	store := &journalPersistenceFake{instance: Instance{ID: "instance-1"}}
	service := NewInstanceService(store, nil)
	if _, err := service.ScheduleRuns(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, "instance-1", 20, nil); !errors.Is(err, ErrForbidden) || store.requestedID != "" {
		t.Fatalf("unentitled schedule-run request reached persistence: id=%q err=%v", store.requestedID, err)
	}
	if _, err := service.ScheduleRuns(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", 0, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid schedule-run limit was accepted: %v", err)
	}
}

func TestPaperPortfolioIsOwnerScopedAndPaperOnly(t *testing.T) {
	store := &journalPersistenceFake{
		instance:  Instance{ID: "instance-1", ExecutionMode: Paper},
		portfolio: PaperPortfolio{StrategyInstanceID: "instance-1", Currency: "USD", Cash: "20125.0000000000", Positions: []PaperPosition{}},
	}
	service := NewInstanceService(store, nil)
	portfolio, err := service.PaperPortfolio(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1")
	if err != nil {
		t.Fatal(err)
	}
	if portfolio.StrategyInstanceID != "instance-1" || store.requestedUser != "owner" || store.requestedID != "instance-1" || store.portfolioCalls != 1 {
		t.Fatalf("paper portfolio boundary changed: portfolio=%#v store=%#v", portfolio, store)
	}

	store.instance.ExecutionMode = Shadow
	if _, err = service.PaperPortfolio(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("shadow portfolio was not rejected: %v", err)
	}
	if store.portfolioCalls != 1 {
		t.Fatal("shadow portfolio request reached the PAPER ledger")
	}

	if _, err = service.PaperPortfolio(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, "instance-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unentitled portfolio request was accepted: %v", err)
	}
}

func TestShadowScorecardIsOwnerScopedAndEntitled(t *testing.T) {
	store := &journalPersistenceFake{scorecard: ShadowScorecard{
		StrategyInstanceID: "instance-1", TotalMarks: 1,
		Horizons: []ShadowHorizonScore{{Horizon: ShadowOutcomeOneHour, SampleSize: 1}},
	}}
	service := NewInstanceService(store, nil)
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}
	scorecard, err := service.ShadowScorecard(context.Background(), principal, "instance-1")
	if err != nil || scorecard.TotalMarks != 1 || store.requestedUser != "owner" || store.requestedID != "instance-1" || store.scorecardCalls != 1 {
		t.Fatalf("shadow scorecard owner boundary changed: scorecard=%#v store=%#v err=%v", scorecard, store, err)
	}
	if _, err = service.ShadowScorecard(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, "instance-1"); !errors.Is(err, ErrForbidden) || store.scorecardCalls != 1 {
		t.Fatalf("unentitled shadow scorecard request reached persistence: calls=%d err=%v", store.scorecardCalls, err)
	}
}

func TestRecordLifecycleRequiresExplicitPaperConfirmationAndOwnerEntitlement(t *testing.T) {
	store := &journalPersistenceFake{}
	service := NewInstanceService(store, nil)
	command := LifecycleCommand{EventID: "manual-lifecycle:event-1", EventType: Assignment, ExpectedStateVersion: 2}
	if _, err := service.RecordLifecycle(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", command); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unconfirmed paper event was accepted: %v", err)
	}
	command.ConfirmPaperSimulation = true
	if _, err := service.RecordLifecycle(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, "instance-1", command); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unentitled paper event was accepted: %v", err)
	}
	if store.lifecycleID != "" {
		t.Fatal("rejected lifecycle command reached persistence")
	}
}

func TestRecordLifecycleIsOwnerScopedAndUsesServerTime(t *testing.T) {
	now := time.Date(2026, 8, 18, 20, 0, 0, 0, time.UTC)
	store := &journalPersistenceFake{lifecycleResult: LifecycleResult{ID: "event-record-1", EventType: ExpireWorthless}}
	service := NewInstanceService(store, nil)
	service.now = func() time.Time { return now }
	command := LifecycleCommand{EventID: "manual-lifecycle:event-1", EventType: ExpireWorthless, ExpectedStateVersion: 2, ConfirmPaperSimulation: true}
	result, err := service.RecordLifecycle(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", command)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "event-record-1" || store.requestedUser != "owner" || store.lifecycleID != "instance-1" || store.lifecycle != command || !store.lifecycleAt.Equal(now) {
		t.Fatalf("lifecycle command boundary changed: result=%#v store=%#v", result, store)
	}
}
