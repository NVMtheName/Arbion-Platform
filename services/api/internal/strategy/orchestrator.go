package strategy

import (
	"context"
	"errors"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

var ErrDuplicate = errors.New("evaluation already processed")

type RiskEvaluator interface {
	Evaluate(risk.EvaluationContext, risk.ProposedAction) risk.RiskEvaluation
}
type Repository interface {
	CommitEvaluation(context.Context, Instance, int, Decision, risk.RiskEvaluation, ExecutionResult, time.Time) error
}
type Orchestrator struct {
	Engine *Engine
	Risk   RiskEvaluator
	Store  Repository
	Paper  ExecutionAdapter
	Shadow ExecutionAdapter
}

func (o *Orchestrator) Evaluate(ctx context.Context, i Instance, in EvaluationInput, rc risk.EvaluationContext) (EvaluationOutcome, error) {
	d, err := o.Engine.Evaluate(i, in)
	if err != nil {
		return EvaluationOutcome{}, err
	}
	re := o.Risk.Evaluate(rc, *d.ProposedAction)
	var adapter ExecutionAdapter
	switch i.ExecutionMode {
	case Paper:
		adapter = o.Paper
	case Shadow:
		adapter = o.Shadow
	default:
		return EvaluationOutcome{}, ErrInvalid
	}
	result, err := adapter.Execute(*d.ProposedAction, re, in.Market, d.ProposedState)
	if err != nil {
		return EvaluationOutcome{Execution: result}, err
	}
	// The repository claims the event inside the same transaction that persists risk
	// evidence, execution, journal, paper mutations, and any optimistic state change.
	// Pure evaluation may therefore run more than once for a duplicate request, while
	// durable effects are committed exactly once and no abandoned CLAIMED row is left.
	if err = o.Store.CommitEvaluation(ctx, i, i.StateVersion, d, re, result, in.Timestamp); err != nil {
		return EvaluationOutcome{Execution: result}, err
	}
	return EvaluationOutcome{Execution: result, RiskDecision: re.Decision, RiskReasonCodes: re.ReasonCodes, RiskChecks: re.Checks, ApprovalRequired: re.ApprovalRequired, LiveExecutionAvailable: false}, nil
}

func (o *Orchestrator) EvaluateDecision(ctx context.Context, i Instance, d Decision, rc risk.EvaluationContext, evaluatedAt time.Time) (EvaluationOutcome, error) {
	if d.ProposedAction == nil || i.ExecutionMode != Shadow {
		return EvaluationOutcome{}, ErrInvalid
	}
	re := o.Risk.Evaluate(rc, *d.ProposedAction)
	result, err := o.Shadow.Execute(*d.ProposedAction, re, MarketSnapshot{}, d.ProposedState)
	if err != nil {
		return EvaluationOutcome{Execution: result}, err
	}
	if err = o.Store.CommitEvaluation(ctx, i, i.StateVersion, d, re, result, evaluatedAt); err != nil {
		return EvaluationOutcome{Execution: result}, err
	}
	return EvaluationOutcome{Execution: result, RiskDecision: re.Decision, RiskReasonCodes: re.ReasonCodes, RiskChecks: re.Checks, ApprovalRequired: re.ApprovalRequired, LiveExecutionAvailable: false}, nil
}
