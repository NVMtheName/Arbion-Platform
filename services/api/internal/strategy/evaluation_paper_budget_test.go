package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/marketintelligence"
	"github.com/arbion/platform/services/api/internal/neural"
	"github.com/arbion/platform/services/api/internal/risk"
)

func TestCoinbasePaperBuyFitsExactCashBudget(t *testing.T) {
	for _, tc := range []struct {
		name, price, proposal, reserve string
	}{
		{"gross and fee rounding", "1.4086", "50", "950"},
		{"fill price rounding", "0.3333333333", "100", "900"},
		{"low price fill rounding", "0.00000001", "100", "900"},
		{"exact affordable budget", "1", "1.0075125000", "998.9924875000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, store, finances, ai, principal := paperBudgetEvaluationFixture(tc.price, tc.proposal, tc.reserve)
			outcome, err := service.Evaluate(context.Background(), principal, "ai-instance", "scheduled:budget")
			if err != nil || outcome.RiskDecision != risk.Allow || outcome.Execution.Status != SimulatedFilled || outcome.LiveExecutionAvailable {
				t.Fatalf("expected an allowed isolated simulation: outcome=%#v err=%v", outcome, err)
			}
			assertPaperBudgetIsolated(t, store, finances, ai)
			fill := store.paperFill
			cashDelta, deltaOK := new(big.Rat).SetString(fill.CashDelta)
			cash, cashOK := new(big.Rat).SetString(fill.ResultingCash)
			budget, budgetOK := new(big.Rat).SetString(tc.proposal)
			reserve, reserveOK := new(big.Rat).SetString(tc.reserve)
			if !deltaOK || !cashOK || !budgetOK || !reserveOK || cashDelta.Sign() >= 0 {
				t.Fatal("invalid exact simulated cash evidence", fill)
			}
			debit := new(big.Rat).Neg(cashDelta)
			if debit.Cmp(budget) > 0 || cash.Cmp(reserve) < 0 {
				t.Fatalf("conservative simulation exceeded authorized budget or reserve: quantity=%s fill_price=%s gross=%s fee=%s debit=%s budget=%s cash=%s reserve=%s",
					fill.Quantity, fill.FillPrice, fill.GrossNotional, fill.Fee, debit.FloatString(10), tc.proposal, fill.ResultingCash, tc.reserve)
			}
			if new(big.Rat).Add(cash, debit).Cmp(big.NewRat(1000, 1)) != 0 || !sameAIPaperDecimal(fill.RequestedNotional, tc.proposal) {
				t.Fatal("sizing changed authorized proposal or exact cash accounting", fill)
			}
			if tc.name == "exact affordable budget" && (debit.Cmp(budget) != 0 || fill.Quantity != "1.0000000000") {
				t.Fatal("exactly affordable quantity was unnecessarily reduced", fill)
			}
		})
	}
}

func TestCoinbasePaperBuyRejectsUnaffordableQuantity(t *testing.T) {
	service, store, finances, ai, principal := paperBudgetEvaluationFixture("100", "0.0000000001", "999.9999999999")
	_, err := service.Evaluate(context.Background(), principal, "ai-instance", "scheduled:unaffordable")
	if !errors.Is(err, ErrInvalid) {
		t.Fatal("zero affordable quantity should fail closed", err)
	}
	if ai.calls != 1 || store.paperCommits != 0 || store.commits != 0 || store.abstains != 0 ||
		finances.balanceCalls != 0 || finances.positionCalls != 0 || finances.quoteCalls != 0 || len(store.outcomeMarks) != 0 {
		t.Fatal("unaffordable quantity wrote evidence or crossed the isolated Paper boundary")
	}
}

func TestPaperBudgetSizingLeavesSellAndShadowQuantityUnchanged(t *testing.T) {
	t.Run("Paper SELL retains reference quantity", func(t *testing.T) {
		service, store, finances, ai, principal := paperBudgetEvaluationFixture("100", "50", "950")
		ai.decision.Side = "SELL"
		store.facts.Paper.Positions = []Position{{Symbol: "XRP", Instrument: "CRYPTO", Quantity: "2", AveragePrice: "100"}}
		outcome, err := service.Evaluate(context.Background(), principal, "ai-instance", "scheduled:unchanged-sale")
		if err != nil || outcome.RiskDecision != risk.Allow || outcome.Execution.Status != SimulatedFilled {
			t.Fatalf("valid Paper sale changed: %#v err=%v", outcome, err)
		}
		assertPaperBudgetIsolated(t, store, finances, ai)
		if store.paperFill.Quantity != "0.5000000000" || store.paperFill.FillPrice != "99.7500000000" || store.paperFill.CashDelta != "49.6256250000" || store.paperFill.ResultingCash != "1049.6256250000" {
			t.Fatal("BUY budget fitting changed sale sizing or conservative sale costs", store.paperFill)
		}
	})
	t.Run("Shadow BUY retains reference quantity", func(t *testing.T) {
		service, store, finances, ai, principal := paperBudgetEvaluationFixture("100", "50", "950")
		store.instance.ExecutionMode = Shadow
		service.automation.(*evaluationAutomationFake).mandate.ExecutionMode = "SHADOW"
		cash := financial.Money{Amount: "1000", Currency: "USD"}
		finances.balances = financial.Balances{Cash: &cash, AvailableCash: &cash, BuyingPower: &cash}
		outcome, err := service.Evaluate(context.Background(), principal, "ai-instance", "scheduled:unchanged-shadow")
		if err != nil || outcome.RiskDecision != risk.Allow || outcome.Execution.Status != WouldHaveSubmitted || outcome.LiveExecutionAvailable {
			t.Fatalf("valid Shadow action changed: %#v err=%v", outcome, err)
		}
		if ai.calls != 1 || store.commits != 1 || store.paperCommits != 0 || store.abstains != 0 || store.decision.ProposedAction == nil || store.decision.ProposedAction.Quantity != "0.5000000000" {
			t.Fatal("Paper BUY fitting changed Shadow delivery or quantity", store.decision)
		}
	})
}

// Uses the normal service, risk engine, and simulator. Every provider/model and
// storage interface is a package-local fake; this test cannot make an order,
// contact an external API, or change an actual account or production ledger.
func paperBudgetEvaluationFixture(price, proposal, reserve string) (*EvaluationService, *evaluationStoreFake, *evaluationFinancialFake, *evaluationAIFake, authorization.Principal) {
	decision := neural.ShadowDecision{Decision: "PROPOSE", Symbol: "XRP", Side: "BUY", ProposedNotional: proposal, Confidence: "MEDIUM", Thesis: "Bounded synthetic budget fixture", RiskFlags: []string{}, Limitations: []string{"Simulation only"}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep"}}
	service, store, finances, ai, principal := aiEvaluationFixture("coinbase", decision)
	store.instance.ExecutionMode = Paper
	store.facts.Paper = &PaperEvaluationFacts{Cash: "1000.0000000000", CurrentExposure: "0", Positions: []Position{}, RiskPositions: []risk.Position{}}
	automations := service.automation.(*evaluationAutomationFake)
	automations.mandate.ExecutionMode = "PAPER"
	automations.mandate.AllowedUniverse = automation.Universe{Symbols: []string{"XRP"}}
	automations.mandate.StrategyParameters = json.RawMessage(`{"objective":"Preserve simulated capital.","max_proposal_notional":"100"}`)
	automations.mandate.Risk.MinimumCashReserve = &reserve
	automations.bucket.AllocationValue = "1000"
	markets := service.markets.(*evaluationMarketsFake)
	observation := markets.batch.Markets[0]
	observation.Symbol, observation.CurrentPrice = "XRP", marketintelligence.Decimal(price)
	observation.Bid, observation.Ask = marketDecimalPointer(price), marketDecimalPointer(price)
	markets.batch.Markets = []marketintelligence.CryptoMarketObservation{observation}
	markets.stats, markets.history, markets.liquidity = nil, nil, nil
	return service, store, finances, ai, principal
}

func assertPaperBudgetIsolated(t *testing.T, store *evaluationStoreFake, finances *evaluationFinancialFake, ai *evaluationAIFake) {
	t.Helper()
	if ai.calls != 1 || store.paperCommits != 1 || store.commits != 0 || store.abstains != 0 ||
		finances.balanceCalls != 0 || finances.positionCalls != 0 || finances.quoteCalls != 0 || len(store.outcomeMarks) != 0 ||
		!store.paperFill.SimulationOnly || store.paperFill.MarketProvider != "coinbase" {
		t.Fatal("Paper budget evaluation crossed isolated simulation boundaries")
	}
}
