package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
)

var (
	ErrForbidden = errors.New("strategy entitlement required")
	ErrNotFound  = errors.New("strategy instance not found")
	ErrConflict  = errors.New("strategy instance conflict")
)

type Persistence interface {
	Initialize(context.Context, string, automation.Mandate, string, State) (Instance, error)
	List(context.Context, string) ([]Instance, error)
	Get(context.Context, string, string) (Instance, error)
	History(context.Context, string, string) ([]Transition, error)
	Decisions(context.Context, string, string) ([]DecisionJournalEntry, error)
	Executions(context.Context, string, string) ([]ExecutionRecord, error)
	Journal(context.Context, string, int, *JournalCursor) ([]JournalActivity, error)
	Schedule(context.Context, string, string) (ScheduleStatus, error)
	RecordLifecycle(context.Context, string, string, LifecycleCommand, time.Time) (LifecycleResult, error)
}
type Mandates interface {
	Get(context.Context, authorization.Principal, string) (automation.Mandate, error)
}
type DecisionJournalEntry struct {
	ID, StrategyInstanceID, StrategyState, Source, DecisionType           string
	StructuredRationale                                                   json.RawMessage
	ProposedActionID, RiskEvaluationID, ExecutionRecordID, ResultingState *string
	CreatedAt                                                             time.Time
}
type InstanceService struct {
	store    Persistence
	mandates Mandates
	now      func() time.Time
}

func NewInstanceService(s Persistence, m Mandates) *InstanceService {
	return &InstanceService{store: s, mandates: m, now: func() time.Time { return time.Now().UTC() }}
}
func entitled(p authorization.Principal) bool {
	return authorization.CanUseAutomation(p) && authorization.CanConnectFinancialAccounts(p)
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
	if m.AutomationType != "STRATEGY" || m.StrategyIdentifier == nil || m.Status != "READY" || (*m.StrategyIdentifier != "wheel" && *m.StrategyIdentifier != "covered_call" && *m.StrategyIdentifier != "cash_secured_put") {
		return Instance{}, ErrInvalid
	}
	if m.ExecutionMode != "PAPER" && m.ExecutionMode != "SHADOW" {
		return Instance{}, ErrInvalid
	}
	if m.ExecutionMode == "PAPER" {
		x, ok := new(big.Rat).SetString(startingCash)
		if !ok || x.Sign() <= 0 {
			return Instance{}, ErrInvalid
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
