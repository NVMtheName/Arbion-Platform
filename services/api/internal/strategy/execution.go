package strategy

import (
	"math/big"

	"github.com/arbion/platform/services/api/internal/risk"
)

type ExecutionAdapter interface {
	Execute(risk.ProposedAction, risk.RiskEvaluation, MarketSnapshot, State) (ExecutionResult, error)
}
type PaperExecutionAdapter struct{}

func (PaperExecutionAdapter) Execute(a risk.ProposedAction, r risk.RiskEvaluation, market MarketSnapshot, next State) (ExecutionResult, error) {
	if r.Decision != risk.Allow {
		return ExecutionResult{Status: RiskDenied, Reason: "risk_not_allowed"}, nil
	}
	if a.EstimatedPrice == nil {
		return ExecutionResult{Status: SimulatedRejected, Reason: "required_price_unavailable"}, nil
	}
	price, ok := new(big.Rat).SetString(*a.EstimatedPrice)
	if !ok || price.Sign() < 0 {
		return ExecutionResult{Status: SimulatedRejected, Reason: "invalid_price"}, nil
	}
	q, qok := new(big.Rat).SetString(a.Quantity)
	if !qok {
		return ExecutionResult{Status: SimulatedRejected, Reason: "invalid_quantity"}, nil
	}
	notional := new(big.Rat).Mul(price, new(big.Rat).Mul(q, big.NewRat(100, 1)))
	p := price.FloatString(10)
	n := notional.FloatString(10)
	return ExecutionResult{Status: SimulatedFilled, Price: &p, Notional: &n, ExpectedState: next}, nil
}

type ShadowExecutionAdapter struct{}

func (ShadowExecutionAdapter) Execute(a risk.ProposedAction, r risk.RiskEvaluation, _ MarketSnapshot, next State) (ExecutionResult, error) {
	if r.Decision != risk.Allow {
		return ExecutionResult{Status: RiskDenied, Reason: "risk_not_allowed"}, nil
	}
	return ExecutionResult{Status: WouldHaveSubmitted, Price: a.EstimatedPrice, Notional: &a.Notional, ExpectedState: next, Reason: "no_order_was_sent"}, nil
}
