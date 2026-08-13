package strategy

import (
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

func ptr(s string) *string { return &s }
func fixture(kind string) (Instance, EvaluationInput) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	id := "mandate"
	identifier := map[string]string{"PUT": "cash_secured_put", "CALL": "covered_call"}[kind]
	state := ReadyForPut
	if kind == "CALL" {
		state = ReadyForCall
	}
	i := Instance{ID: "instance", UserID: "user", AutomationMandateID: id, MandateVersion: 3, FinancialAccountID: "account", StrategyIdentifier: identifier, ExecutionMode: Paper, CurrentState: state, StateVersion: 1}
	in := EvaluationInput{EventID: "event-1", Timestamp: now, PriorState: state, Mandate: MandateSnapshot{ID: id, Version: 3, StrategyIdentifier: identifier}, Account: AccountSnapshot{Timestamp: now, AvailableCash: "20000", Positions: []Position{{Symbol: "AAPL", Instrument: "EQUITY", Quantity: "100"}}}, Parameters: Parameters{Symbols: []string{"AAPL"}, MinimumDTE: 20, MaximumDTE: 60, TargetDelta: "0.30", TargetDeltaMin: "0.20", TargetDeltaMax: "0.40", MaximumContracts: 1}, Market: MarketSnapshot{Symbol: "AAPL", Timestamp: now, Options: []OptionCandidate{{Underlying: "AAPL", OptionType: kind, Strike: "190", Expiration: "2026-01-31", Bid: ptr("1.25"), Ask: ptr("1.35"), Delta: ptr(map[string]string{"PUT": "-0.31", "CALL": "0.31"}[kind]), Timestamp: now}}}}
	return i, in
}
func TestCashSecuredPutAndCoveredCallProposals(t *testing.T) {
	for _, kind := range []string{"PUT", "CALL"} {
		t.Run(kind, func(t *testing.T) {
			i, in := fixture(kind)
			d, e := NewEngine().Evaluate(i, in)
			if e != nil {
				t.Fatal(e)
			}
			if d.ProposedAction.Option.PutCall != kind || d.ProposedAction.Side != "SELL_TO_OPEN" {
				t.Fatalf("unexpected action: %#v", d.ProposedAction)
			}
			want := PutProposed
			if kind == "CALL" {
				want = CallProposed
			}
			if d.ProposedState != want {
				t.Fatalf("state %s", d.ProposedState)
			}
		})
	}
}
func TestCoveredCallRequiresShares(t *testing.T) {
	i, in := fixture("CALL")
	in.Account.Positions = nil
	if _, e := NewEngine().Evaluate(i, in); e == nil {
		t.Fatal("expected conservative coverage rejection")
	}
}
func TestInvalidStateAndMandateVersionFailClosed(t *testing.T) {
	i, in := fixture("PUT")
	i.CurrentState = ShortPutOpen
	in.PriorState = ShortPutOpen
	if _, e := NewEngine().Evaluate(i, in); e == nil {
		t.Fatal("expected invalid transition")
	}
	i, in = fixture("PUT")
	in.Mandate.Version++
	if _, e := NewEngine().Evaluate(i, in); e == nil {
		t.Fatal("expected immutable mandate binding rejection")
	}
}
func TestCandidateSelectionDeterministic(t *testing.T) {
	_, in := fixture("PUT")
	a := in.Market.Options[0]
	b := a
	b.Strike = "185"
	b.Delta = ptr("-0.29")
	in.Market.Options = []OptionCandidate{a, b}
	got, count, e := SelectCandidate(in.Timestamp, in.Parameters, "PUT", in.Market.Options)
	if e != nil {
		t.Fatal(e)
	}
	if count != 2 || got.Strike != "185" {
		t.Fatalf("got count=%d strike=%s", count, got.Strike)
	}
}
func TestMissingCandidateDataAndInvalidParameters(t *testing.T) {
	_, in := fixture("PUT")
	in.Market.Options[0].Delta = nil
	if _, _, e := SelectCandidate(in.Timestamp, in.Parameters, "PUT", in.Market.Options); e == nil {
		t.Fatal("missing delta must fail closed")
	}
	in.Parameters.MaximumContracts = 0
	if e := ValidateParameters(in.Parameters); e == nil {
		t.Fatal("invalid parameters accepted")
	}
}
func TestWheelLifecycleComposition(t *testing.T) {
	state := ShortPutOpen
	var e error
	state, e = ApplyLifecycle("wheel", state, Assignment)
	if e != nil || state != LongShares {
		t.Fatal(state, e)
	}
	state = ShortCallOpen
	state, e = ApplyLifecycle("wheel", state, CallAway)
	if e != nil || state != Cash {
		t.Fatal(state, e)
	}
	state = ShortPutOpen
	state, _ = ApplyLifecycle("wheel", state, ExpireWorthless)
	if state != ReadyForPut {
		t.Fatal(state)
	}
	state = ShortCallOpen
	state, _ = ApplyLifecycle("wheel", state, ExpireWorthless)
	if state != ReadyForCall {
		t.Fatal(state)
	}
}
func TestPaperAndShadowSemantics(t *testing.T) {
	i, in := fixture("PUT")
	d, _ := NewEngine().Evaluate(i, in)
	allow := risk.RiskEvaluation{Decision: risk.Allow}
	paper, _ := PaperExecutionAdapter{}.Execute(*d.ProposedAction, allow, in.Market, ShortPutOpen)
	if paper.Status != SimulatedFilled || paper.Notional == nil || *paper.Notional != "125.0000000000" {
		t.Fatalf("paper: %#v", paper)
	}
	shadow, _ := ShadowExecutionAdapter{}.Execute(*d.ProposedAction, allow, in.Market, ShortPutOpen)
	if shadow.Status != WouldHaveSubmitted || shadow.Reason != "no_order_was_sent" {
		t.Fatalf("shadow: %#v", shadow)
	}
	deny := risk.RiskEvaluation{Decision: risk.Deny}
	paper, _ = PaperExecutionAdapter{}.Execute(*d.ProposedAction, deny, in.Market, ShortPutOpen)
	if paper.Status != RiskDenied {
		t.Fatal("risk denial did not prevent fill")
	}
}
