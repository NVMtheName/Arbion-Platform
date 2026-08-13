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
	BeginEvaluation(context.Context, string, string) (bool, error)
	CommitEvaluation(context.Context, Instance, int, Decision, risk.RiskEvaluation, ExecutionResult, time.Time) error
}
type Orchestrator struct {
	Engine *Engine
	Risk   RiskEvaluator
	Store  Repository
	Paper  ExecutionAdapter
	Shadow ExecutionAdapter
}

func (o *Orchestrator) Evaluate(ctx context.Context, i Instance, in EvaluationInput, rc risk.EvaluationContext) (ExecutionResult, error) {
	claimed, err := o.Store.BeginEvaluation(ctx, i.ID, in.EventID)
	if err != nil {
		return ExecutionResult{}, err
	}
	if !claimed {
		return ExecutionResult{}, ErrDuplicate
	}
	d, err := o.Engine.Evaluate(i, in)
	if err != nil {
		return ExecutionResult{}, err
	}
	re := o.Risk.Evaluate(rc, *d.ProposedAction)
	var adapter ExecutionAdapter
	switch i.ExecutionMode {
	case Paper:
		adapter = o.Paper
	case Shadow:
		adapter = o.Shadow
	default:
		return ExecutionResult{}, ErrInvalid
	}
	result, err := adapter.Execute(*d.ProposedAction, re, in.Market, d.ProposedState)
	if err != nil {
		return result, err
	}
	// Repository implementations atomically persist risk evidence, execution, journal,
	// paper mutations, and the optimistic state transition.
	if err = o.Store.CommitEvaluation(ctx, i, i.StateVersion, d, re, result, in.Timestamp); err != nil {
		return result, err
	}
	return result, nil
}
