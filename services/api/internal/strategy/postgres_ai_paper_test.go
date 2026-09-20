package strategy

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

func TestValidAIPaperCommitComparesDecimalValuesNotFormatting(t *testing.T) {
	now := time.Date(2026, 8, 29, 20, 26, 55, 0, time.UTC)
	mandateID, mandateVersion := "mandate", 2
	price := "100.0000000000"
	state := string(AIMonitoring)
	instance := Instance{
		ID: "instance", UserID: "owner", AutomationMandateID: mandateID,
		MandateVersion: mandateVersion, FinancialAccountID: "account", CapitalBucketID: "bucket",
		StrategyIdentifier: "ai_shadow", ExecutionMode: Paper,
		CurrentState: AIMonitoring, StateVersion: 1, Status: "ACTIVE",
	}
	action := risk.ProposedAction{
		ID: "paper-action", CorrelationID: "scheduled:paper-action",
		FinancialAccountID: instance.FinancialAccountID, Source: risk.SourceAI,
		ActionType: risk.ActionBuy, MandateID: &mandateID,
		MandateVersion: &mandateVersion, Instrument: "BTC", Side: "BUY",
		Quantity: "1.0000000000", Notional: "100", EstimatedPrice: &price,
		StrategyInstanceID: &instance.ID, StrategyState: &state, CreatedAt: now,
	}
	evaluation := risk.RiskEvaluation{
		ID: "evaluation", UserID: instance.UserID, AccountID: instance.FinancialAccountID,
		MandateID: &mandateID, MandateVersion: &mandateVersion,
		Decision: risk.Allow, Mode: "PAPER", Timestamp: now,
	}
	decision := Decision{
		ProposedAction: &action, Source: "AI", InstrumentType: "CRYPTO",
		ProposedState: AIMonitoring,
		QuoteReference: &AIProposalQuoteReference{
			Symbol: "BTC", Side: "BUY", Price: price, Basis: "ASK",
			Provider: "coinbase", Feed: "exchange", Quality: "REALTIME", ObservedAt: now,
		},
		Rationale: json.RawMessage(`{"decision":"PROPOSE","quote_reference":{"symbol":"BTC","side":"BUY","price":"100.0000000000","basis":"ASK","provider":"coinbase","feed":"exchange","quality":"REALTIME","observed_at":"2026-08-29T20:26:55Z"}}`),
	}
	fill := SimulateAIPaperSpotFill(
		action, evaluation, "CRYPTO",
		AIPaperPortfolioSnapshot{Currency: "USD", Cash: "1000.0000000000", Positions: map[string]string{}},
		AIPaperMarketReference{Symbol: "BTC", Price: price, Basis: "ASK", Provider: "coinbase", Feed: "exchange", Quality: "REALTIME", ObservedAt: now},
		AIPaperSimulationConfig{}, now,
	)
	if fill.RequestedNotional != "100.0000000000" {
		t.Fatalf("simulator did not canonicalize the model notional: %#v", fill)
	}
	if !validAIPaperCommit(instance, instance.StateVersion, decision, evaluation, fill, now) {
		t.Fatal("semantically equal decimal representations were rejected")
	}
	for name, change := range map[string]func(*Instance, *risk.ProposedAction, *risk.RiskEvaluation){
		"missing owner":    func(i *Instance, _ *risk.ProposedAction, _ *risk.RiskEvaluation) { i.UserID = "" },
		"missing capital":  func(i *Instance, _ *risk.ProposedAction, _ *risk.RiskEvaluation) { i.CapitalBucketID = "" },
		"missing instance": func(_ *Instance, a *risk.ProposedAction, _ *risk.RiskEvaluation) { a.StrategyInstanceID = nil },
		"different instance": func(_ *Instance, a *risk.ProposedAction, _ *risk.RiskEvaluation) {
			other := "other"
			a.StrategyInstanceID = &other
		},
		"different state": func(_ *Instance, a *risk.ProposedAction, _ *risk.RiskEvaluation) {
			other := "READY_FOR_PUT"
			a.StrategyState = &other
		},
		"different strategy": func(_ *Instance, a *risk.ProposedAction, _ *risk.RiskEvaluation) {
			other := "wheel"
			a.StrategyIdentifier = &other
		},
		"different side":         func(_ *Instance, a *risk.ProposedAction, _ *risk.RiskEvaluation) { a.ActionType = risk.ActionSell },
		"different action time":  func(_ *Instance, a *risk.ProposedAction, _ *risk.RiskEvaluation) { a.CreatedAt = now.Add(-time.Second) },
		"different risk time":    func(_ *Instance, _ *risk.ProposedAction, e *risk.RiskEvaluation) { e.Timestamp = now.Add(-time.Second) },
		"different risk owner":   func(_ *Instance, _ *risk.ProposedAction, e *risk.RiskEvaluation) { e.UserID = "other" },
		"different risk account": func(_ *Instance, _ *risk.ProposedAction, e *risk.RiskEvaluation) { e.AccountID = "other" },
		"live risk":              func(_ *Instance, _ *risk.ProposedAction, e *risk.RiskEvaluation) { e.PlatformExecutionAvailable = true },
	} {
		t.Run(name, func(t *testing.T) {
			i, a, e, d := instance, action, evaluation, decision
			change(&i, &a, &e)
			d.ProposedAction = &a
			if validAIPaperCommit(i, i.StateVersion, d, e, fill, now) {
				t.Fatal("mismatched Paper authority accepted")
			}
		})
	}

	action.Notional = "101"
	if validAIPaperCommit(instance, instance.StateVersion, decision, evaluation, fill, now) {
		t.Fatal("different authorized and simulated notionals were accepted")
	}
}
