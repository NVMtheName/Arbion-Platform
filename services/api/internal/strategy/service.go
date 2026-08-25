package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
)

var (
	ErrForbidden    = errors.New("strategy entitlement required")
	ErrNotFound     = errors.New("strategy instance not found")
	ErrConflict     = errors.New("strategy instance conflict")
	ErrCapitalLimit = errors.New("paper starting cash exceeds capital bucket capacity")
	ErrAccountInUse = errors.New("financial account already has an active non-live strategy")
	ErrOpenExposure = errors.New("paper strategy still has open simulated positions")
	ErrMandateStale = errors.New("strategy mandate is not current and ready")
)

type Persistence interface {
	Initialize(context.Context, string, automation.Mandate, string, State) (Instance, error)
	Pause(context.Context, string, string, int, time.Time) (Instance, error)
	Resume(context.Context, string, string, int, time.Time) (Instance, error)
	Finish(context.Context, string, string, int, time.Time) (Instance, error)
	List(context.Context, string) ([]Instance, error)
	Get(context.Context, string, string) (Instance, error)
	History(context.Context, string, string) ([]Transition, error)
	Decisions(context.Context, string, string) ([]DecisionJournalEntry, error)
	Executions(context.Context, string, string) ([]ExecutionRecord, error)
	PaperPortfolio(context.Context, string, string) (PaperPortfolio, error)
	Journal(context.Context, string, int, *JournalCursor) ([]JournalActivity, error)
	Schedule(context.Context, string, string) (ScheduleStatus, error)
	RecordLifecycle(context.Context, string, string, LifecycleCommand, time.Time) (LifecycleResult, error)
}
type Auditor interface {
	Record(context.Context, *string, string, map[string]any) error
}
type Mandates interface {
	Get(context.Context, authorization.Principal, string) (automation.Mandate, error)
	GetBucket(context.Context, authorization.Principal, string) (automation.CapitalBucket, error)
}
type DecisionJournalEntry struct {
	ID, StrategyInstanceID, StrategyState, Source, DecisionType           string
	StructuredRationale                                                   json.RawMessage
	ProposedActionID, RiskEvaluationID, ExecutionRecordID, ResultingState *string
	RiskDecision                                                          *string
	ApprovalRequired                                                      *bool
	RiskReasonCodes, RiskChecks                                           json.RawMessage
	ExecutionStatus, Symbol, Instrument, Side, Quantity, Price, Notional  *string
	CreatedAt                                                             time.Time
}
type InstanceService struct {
	store    Persistence
	mandates Mandates
	audit    Auditor
	now      func() time.Time
}

func NewInstanceService(s Persistence, m Mandates, auditors ...Auditor) *InstanceService {
	var audit Auditor
	if len(auditors) > 0 {
		audit = auditors[0]
	}
	return &InstanceService{store: s, mandates: m, audit: audit, now: func() time.Time { return time.Now().UTC() }}
}
func entitled(p authorization.Principal) bool {
	return authorization.CanUseAutomation(p) && authorization.CanConnectFinancialAccounts(p)
}

var paperCashPattern = regexp.MustCompile(`^\d+(\.\d{1,10})?$`)

func paperCashCapacity(bucket automation.CapitalBucket) (*big.Rat, bool) {
	if bucket.Status != "ACTIVE" || bucket.IsReserve {
		return nil, false
	}
	allocation, ok := new(big.Rat).SetString(bucket.AllocationValue)
	if !ok || allocation.Sign() <= 0 {
		return nil, false
	}
	capacity := new(big.Rat).Set(allocation)
	if bucket.AllocationType != "FIXED_AMOUNT" {
		if bucket.AllocationLimit == nil {
			return nil, false
		}
		capacity, ok = new(big.Rat).SetString(*bucket.AllocationLimit)
		if !ok || capacity.Sign() <= 0 {
			return nil, false
		}
	} else if bucket.AllocationLimit != nil {
		limit, valid := new(big.Rat).SetString(*bucket.AllocationLimit)
		if !valid || limit.Sign() <= 0 {
			return nil, false
		}
		if limit.Cmp(capacity) < 0 {
			capacity = limit
		}
	}
	protected, ok := new(big.Rat).SetString(bucket.ProtectedAmount)
	if !ok || protected.Sign() < 0 {
		return nil, false
	}
	capacity = new(big.Rat).Sub(capacity, protected)
	return capacity, capacity.Sign() > 0
}

func (s *InstanceService) Initialize(ctx context.Context, p authorization.Principal, mandateID, startingCash string) (Instance, error) {
	if !entitled(p) {
		return Instance{}, ErrForbidden
	}
	m, e := s.mandates.Get(ctx, p, mandateID)
	if e != nil {
		return Instance{}, ErrNotFound
	}
	if m.AutomationType == "HYBRID" {
		return Instance{}, ErrInvalid
	}
	if m.AutomationType == "AI_AUTONOMOUS" {
		if m.StrategyIdentifier != nil || m.AIProviderConnectionID == nil || m.AIModelID == nil || m.Status != "READY" || m.ExecutionMode != "SHADOW" || m.AutonomyLevel != "FULL_AUTONOMOUS" {
			return Instance{}, ErrInvalid
		}
		if _, e = automation.ParseAIShadowParameters(m.StrategyParameters); e != nil {
			return Instance{}, ErrInvalid
		}
		bucket, bucketErr := s.mandates.GetBucket(ctx, p, m.CapitalBucketID)
		if bucketErr != nil || bucket.UserID != p.UserID || bucket.FinancialAccountID != m.FinancialAccountID || bucket.Status != "ACTIVE" || bucket.IsReserve {
			return Instance{}, ErrInvalid
		}
		return s.store.Initialize(ctx, p.UserID, m, "", AIMonitoring)
	}
	if m.AutomationType != "STRATEGY" || m.StrategyIdentifier == nil || m.Status != "READY" || (*m.StrategyIdentifier != "wheel" && *m.StrategyIdentifier != "covered_call" && *m.StrategyIdentifier != "cash_secured_put") {
		return Instance{}, ErrInvalid
	}
	if m.ExecutionMode != "PAPER" && m.ExecutionMode != "SHADOW" {
		return Instance{}, ErrInvalid
	}
	bucket, e := s.mandates.GetBucket(ctx, p, m.CapitalBucketID)
	if e != nil || bucket.UserID != p.UserID || bucket.FinancialAccountID != m.FinancialAccountID || bucket.Status != "ACTIVE" || bucket.IsReserve {
		return Instance{}, ErrInvalid
	}
	if m.ExecutionMode == "PAPER" {
		if !paperCashPattern.MatchString(startingCash) {
			return Instance{}, ErrInvalid
		}
		x, ok := new(big.Rat).SetString(startingCash)
		if !ok || x.Sign() <= 0 {
			return Instance{}, ErrInvalid
		}
		capacity, available := paperCashCapacity(bucket)
		if !available || x.Cmp(capacity) > 0 {
			return Instance{}, ErrCapitalLimit
		}
	} else {
		startingCash = ""
	}
	definition := automation.Strategies[*m.StrategyIdentifier]
	return s.store.Initialize(ctx, p.UserID, m, startingCash, State(definition.InitialState))
}
func (s *InstanceService) List(c context.Context, p authorization.Principal) ([]Instance, error) {
	if !entitled(p) {
		return nil, ErrForbidden
	}
	return s.store.List(c, p.UserID)
}
func (s *InstanceService) Get(c context.Context, p authorization.Principal, id string) (Instance, error) {
	if !entitled(p) {
		return Instance{}, ErrForbidden
	}
	return s.store.Get(c, p.UserID, id)
}

func (s *InstanceService) Pause(c context.Context, p authorization.Principal, id string, expectedStateVersion int) (Instance, error) {
	if !entitled(p) {
		return Instance{}, ErrForbidden
	}
	if id == "" || expectedStateVersion < 1 {
		return Instance{}, ErrInvalid
	}
	instance, err := s.store.Pause(c, p.UserID, id, expectedStateVersion, s.now().UTC())
	if err != nil {
		return Instance{}, err
	}
	s.auditInstanceStatus(c, p.UserID, instance, "paused")
	return instance, nil
}

func (s *InstanceService) Resume(c context.Context, p authorization.Principal, id string, expectedStateVersion int, confirmed bool) (Instance, error) {
	if !entitled(p) {
		return Instance{}, ErrForbidden
	}
	if id == "" || expectedStateVersion < 1 || !confirmed {
		return Instance{}, ErrInvalid
	}
	instance, err := s.store.Resume(c, p.UserID, id, expectedStateVersion, s.now().UTC())
	if err != nil {
		return Instance{}, err
	}
	s.auditInstanceStatus(c, p.UserID, instance, "resumed")
	return instance, nil
}

func (s *InstanceService) Finish(c context.Context, p authorization.Principal, id string, expectedStateVersion int, confirmed bool) (Instance, error) {
	if !entitled(p) {
		return Instance{}, ErrForbidden
	}
	if id == "" || expectedStateVersion < 1 || !confirmed {
		return Instance{}, ErrInvalid
	}
	instance, err := s.store.Finish(c, p.UserID, id, expectedStateVersion, s.now().UTC())
	if err != nil {
		return Instance{}, err
	}
	s.auditInstanceStatus(c, p.UserID, instance, "completed")
	return instance, nil
}

func (s *InstanceService) auditInstanceStatus(c context.Context, userID string, instance Instance, action string) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(c, &userID, "strategy_instance."+action, map[string]any{
		"strategy_instance_id": instance.ID,
		"mandate_id":           instance.AutomationMandateID,
		"account_id":           instance.FinancialAccountID,
		"mode":                 instance.ExecutionMode,
		"source":               "UI",
	})
}
func (s *InstanceService) History(c context.Context, p authorization.Principal, id string) ([]Transition, error) {
	if !entitled(p) {
		return nil, ErrForbidden
	}
	return s.store.History(c, p.UserID, id)
}
func (s *InstanceService) Decisions(c context.Context, p authorization.Principal, id string) ([]DecisionJournalEntry, error) {
	if !entitled(p) {
		return nil, ErrForbidden
	}
	return s.store.Decisions(c, p.UserID, id)
}
func (s *InstanceService) Executions(c context.Context, p authorization.Principal, id string) ([]ExecutionRecord, error) {
	if !entitled(p) {
		return nil, ErrForbidden
	}
	return s.store.Executions(c, p.UserID, id)
}

func (s *InstanceService) PaperPortfolio(c context.Context, p authorization.Principal, id string) (PaperPortfolio, error) {
	if !entitled(p) {
		return PaperPortfolio{}, ErrForbidden
	}
	instance, err := s.store.Get(c, p.UserID, id)
	if err != nil {
		return PaperPortfolio{}, ErrNotFound
	}
	if instance.ExecutionMode != Paper {
		return PaperPortfolio{}, ErrInvalid
	}
	portfolio, err := s.store.PaperPortfolio(c, p.UserID, id)
	if err != nil {
		return PaperPortfolio{}, ErrNotFound
	}
	return portfolio, nil
}

func (s *InstanceService) Journal(c context.Context, p authorization.Principal, limit int, cursor *JournalCursor) (JournalPage, error) {
	if !entitled(p) {
		return JournalPage{}, ErrForbidden
	}
	if limit < 1 || limit > 100 || (cursor != nil && (cursor.CreatedAt.IsZero() || cursor.ID == "")) {
		return JournalPage{}, ErrInvalid
	}
	entries, err := s.store.Journal(c, p.UserID, limit+1, cursor)
	if err != nil {
		return JournalPage{}, err
	}
	page := JournalPage{Entries: entries}
	if len(entries) > limit {
		page.Entries = entries[:limit]
		last := page.Entries[len(page.Entries)-1]
		page.NextCursor = &JournalCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

func (s *InstanceService) Schedule(c context.Context, p authorization.Principal, id string) (ScheduleStatus, error) {
	if !entitled(p) {
		return ScheduleStatus{}, ErrForbidden
	}
	status, err := s.store.Schedule(c, p.UserID, id)
	if err != nil {
		return ScheduleStatus{}, ErrNotFound
	}
	return status, nil
}

func (s *InstanceService) RecordLifecycle(c context.Context, p authorization.Principal, id string, command LifecycleCommand) (LifecycleResult, error) {
	if !entitled(p) {
		return LifecycleResult{}, ErrForbidden
	}
	if id == "" || !evaluationEventID.MatchString(command.EventID) || command.ExpectedStateVersion < 1 || !command.ConfirmPaperSimulation {
		return LifecycleResult{}, ErrInvalid
	}
	switch command.EventType {
	case ExpireWorthless, Assignment, CallAway:
	default:
		return LifecycleResult{}, ErrInvalid
	}
	result, err := s.store.RecordLifecycle(c, p.UserID, id, command, s.now().UTC())
	if err != nil {
		return LifecycleResult{}, err
	}
	return result, nil
}
