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
type AIPaperRepository interface {
	CommitAIPaperEvaluation(context.Context, Instance, int, Decision, risk.RiskEvaluation, AIPaperFill, time.Time) error
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

// EvaluateAIPaperDecision runs an AI proposal through deterministic policy and
// a provider-independent simulator. Only a risk-allowed simulated fill reaches
// the specialized atomic portfolio writer; denials and simulation rejections
// are recorded through the ordinary immutable non-live evidence transaction.
func (o *Orchestrator) EvaluateAIPaperDecision(ctx context.Context, i Instance, d Decision, rc risk.EvaluationContext, portfolio AIPaperPortfolioSnapshot, market AIPaperMarketReference, config AIPaperSimulationConfig, evaluatedAt time.Time) (EvaluationOutcome, error) {
	if d.ProposedAction == nil || i.ExecutionMode != Paper || d.Source != "AI" || d.ProposedState != AIMonitoring {
		return EvaluationOutcome{}, ErrInvalid
	}
	evaluation := o.Risk.Evaluate(rc, *d.ProposedAction)
	fill := SimulateAIPaperSpotFill(*d.ProposedAction, evaluation, d.InstrumentType, portfolio, market, config, evaluatedAt)
	result := ExecutionResult{Status: fill.Status, Reason: fill.Reason, ExpectedState: AIMonitoring}
	if fill.Status == SimulatedFilled {
		result.Price = &fill.FillPrice
		result.Notional = &fill.GrossNotional
		store, ok := o.Store.(AIPaperRepository)
		if !ok {
			return EvaluationOutcome{Execution: result}, ErrInvalid
		}
		if err := store.CommitAIPaperEvaluation(ctx, i, i.StateVersion, d, evaluation, fill, evaluatedAt); err != nil {
			return EvaluationOutcome{Execution: result}, err
		}
	} else {
		if err := o.Store.CommitEvaluation(ctx, i, i.StateVersion, d, evaluation, result, evaluatedAt); err != nil {
			return EvaluationOutcome{Execution: result}, err
		}
	}
	return EvaluationOutcome{Execution: result, RiskDecision: evaluation.Decision, RiskReasonCodes: evaluation.ReasonCodes, RiskChecks: evaluation.Checks, ApprovalRequired: evaluation.ApprovalRequired, LiveExecutionAvailable: false}, nil
}
