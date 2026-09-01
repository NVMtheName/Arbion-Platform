package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5"
)

// CommitAIPaperEvaluation persists one risk-allowed AI paper fill atomically.
// The transaction contains only simulation state and evidence; there is no
// financial-provider client or broker execution boundary in this method.
func (s *PostgresStore) CommitAIPaperEvaluation(ctx context.Context, instance Instance, expectedVersion int, decision Decision, evaluation risk.RiskEvaluation, fill AIPaperFill, evaluatedAt time.Time) error {
	if !validAIPaperCommit(instance, expectedVersion, decision, evaluation, fill, evaluatedAt) {
		return ErrInvalid
	}
	action := *decision.ProposedAction
	reasonCodes, err := json.Marshal(evaluation.ReasonCodes)
	if err != nil {
		return err
	}
	checks, err := json.Marshal(evaluation.Checks)
	if err != nil {
		return err
	}
	executionMetadata, err := json.Marshal(map[string]any{
		"candidate_count":          decision.CandidateCount,
		"expected_state":           AIMonitoring,
		"live_execution_available": false,
		"simulation_only":          true,
		"requested_notional":       fill.RequestedNotional,
		"reference_price":          fill.ReferencePrice,
		"fee":                      fill.Fee,
		"cash_delta":               fill.CashDelta,
		"position_delta":           fill.PositionDelta,
		"pricing_basis":            fill.PricingBasis,
		"market_provider":          fill.MarketProvider,
		"market_feed":              fill.MarketFeed,
		"market_quality":           fill.MarketQuality,
		"market_observed_at":       fill.MarketObservedAt,
		"quote_reference":          decision.QuoteReference,
		"reason":                   fill.Reason,
	})
	if err != nil {
		return err
	}
	positionMetadata, err := json.Marshal(map[string]any{
		"last_proposed_action_id": action.ID,
		"last_risk_evaluation_id": evaluation.ID,
		"simulation_only":         true,
	})
	if err != nil {
		return err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	claimed, err := tx.Exec(ctx, `INSERT INTO strategy_evaluation_events(strategy_instance_id,event_id,status,created_at,completed_at) VALUES($1,$2,'COMMITTED',$3,$3) ON CONFLICT DO NOTHING`, instance.ID, action.CorrelationID, evaluatedAt)
	if err != nil {
		return err
	}
	if claimed.RowsAffected() != 1 {
		return ErrDuplicate
	}

	var portfolioID, storedCash string
	if err = tx.QueryRow(ctx, `SELECT id::text,cash::text FROM paper_portfolios WHERE strategy_instance_id=$1 AND user_id=$2 FOR UPDATE`, instance.ID, instance.UserID).Scan(&portfolioID, &storedCash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalid
		}
		return err
	}
	if !sameAIPaperDecimal(storedCash, fill.PreviousCash) {
		return ErrConflict
	}

	storedPosition := "0.0000000000"
	positionErr := tx.QueryRow(ctx, `SELECT quantity::text FROM paper_positions WHERE paper_portfolio_id=$1 AND symbol=$2 AND instrument=$3 AND option_type IS NULL AND strike IS NULL AND expiration IS NULL FOR UPDATE`, portfolioID, fill.Symbol, fill.Instrument).Scan(&storedPosition)
	if positionErr != nil && !errors.Is(positionErr, pgx.ErrNoRows) {
		return positionErr
	}
	if !sameAIPaperDecimal(storedPosition, fill.PreviousPositionQuantity) {
		return ErrConflict
	}

	if _, err = tx.Exec(ctx, `INSERT INTO risk_evaluations(id,user_id,financial_account_id,proposed_action_id,correlation_id,mandate_id,mandate_version,decision,approval_required,execution_mode,platform_execution_available,reason_codes,checks,evaluated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'PAPER',false,$10,$11,$12)`, evaluation.ID, evaluation.UserID, evaluation.AccountID, action.ID, action.CorrelationID, action.MandateID, action.MandateVersion, evaluation.Decision, evaluation.ApprovalRequired, reasonCodes, checks, evaluatedAt); err != nil {
		return err
	}

	var executionID string
	if err = tx.QueryRow(ctx, `INSERT INTO nonlive_execution_records(idempotency_key,user_id,strategy_instance_id,mandate_id,mandate_version,proposed_action_id,risk_evaluation_id,mode,status,symbol,instrument,side,quantity,price,notional,metadata,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,'PAPER','SIMULATED_FILLED',$8,$9,$10,$11,$12,$13,$14,$15,$15) RETURNING id::text`, action.ID, instance.UserID, instance.ID, instance.AutomationMandateID, instance.MandateVersion, action.ID, evaluation.ID, fill.Symbol, fill.Instrument, fill.Side, fill.Quantity, fill.FillPrice, fill.GrossNotional, executionMetadata, evaluatedAt).Scan(&executionID); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `INSERT INTO decision_journal_entries(user_id,financial_account_id,mandate_id,mandate_version,strategy_instance_id,strategy_state,source,decision_type,structured_rationale,proposed_action_id,risk_evaluation_id,execution_record_id,resulting_state,created_at) VALUES($1,$2,$3,$4,$5,'AI_MONITORING','AI','ALLOW_SIMULATED_FILLED',$6,$7,$8,$9,'AI_MONITORING',$10)`, instance.UserID, instance.FinancialAccountID, instance.AutomationMandateID, instance.MandateVersion, instance.ID, decision.Rationale, action.ID, evaluation.ID, executionID, evaluatedAt); err != nil {
		return err
	}

	updatedPortfolio, err := tx.Exec(ctx, `UPDATE paper_portfolios SET cash=$3,version=version+1,updated_at=$4 WHERE id=$1 AND user_id=$2 AND cash=$5`, portfolioID, instance.UserID, fill.ResultingCash, evaluatedAt, fill.PreviousCash)
	if err != nil {
		return err
	}
	if updatedPortfolio.RowsAffected() != 1 {
		return ErrConflict
	}

	if fill.Side == "BUY" {
		_, err = tx.Exec(ctx, `INSERT INTO paper_positions(paper_portfolio_id,symbol,instrument,quantity,average_price,metadata,updated_at) VALUES($1,$2,$3,$4,round(($5::numeric+$6::numeric)/$4::numeric,10),$7,$8)
			ON CONFLICT (paper_portfolio_id,symbol,instrument) WHERE instrument IN ('EQUITY','CRYPTO') AND option_type IS NULL AND strike IS NULL AND expiration IS NULL
			DO UPDATE SET quantity=EXCLUDED.quantity,average_price=round(((paper_positions.quantity*paper_positions.average_price)+$5::numeric+$6::numeric)/EXCLUDED.quantity,10),metadata=paper_positions.metadata||EXCLUDED.metadata,updated_at=EXCLUDED.updated_at`, portfolioID, fill.Symbol, fill.Instrument, fill.ResultingPositionQuantity, fill.GrossNotional, fill.Fee, positionMetadata, evaluatedAt)
	} else {
		if errors.Is(positionErr, pgx.ErrNoRows) {
			return ErrConflict
		}
		var updatedPosition string
		err = tx.QueryRow(ctx, `UPDATE paper_positions SET quantity=$4,metadata=metadata||$5::jsonb,updated_at=$6 WHERE paper_portfolio_id=$1 AND symbol=$2 AND instrument=$3 AND quantity=$7 AND option_type IS NULL AND strike IS NULL AND expiration IS NULL RETURNING quantity::text`, portfolioID, fill.Symbol, fill.Instrument, fill.ResultingPositionQuantity, positionMetadata, evaluatedAt, fill.PreviousPositionQuantity).Scan(&updatedPosition)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConflict
		}
	}
	if err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `INSERT INTO ai_paper_spot_fills(user_id,strategy_instance_id,paper_portfolio_id,execution_record_id,proposed_action_id,risk_evaluation_id,symbol,instrument,side,quantity,requested_notional,reference_price,fill_price,gross_notional,fee,cash_delta,position_delta,previous_cash,previous_position_quantity,resulting_cash,resulting_position_quantity,pricing_basis,market_provider,market_feed,market_quality,market_observed_at,simulated_at,simulation_only) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,true)`, instance.UserID, instance.ID, portfolioID, executionID, action.ID, evaluation.ID, fill.Symbol, fill.Instrument, fill.Side, fill.Quantity, fill.RequestedNotional, fill.ReferencePrice, fill.FillPrice, fill.GrossNotional, fill.Fee, fill.CashDelta, fill.PositionDelta, fill.PreviousCash, fill.PreviousPositionQuantity, fill.ResultingCash, fill.ResultingPositionQuantity, fill.PricingBasis, fill.MarketProvider, fill.MarketFeed, fill.MarketQuality, fill.MarketObservedAt, evaluatedAt); err != nil {
		return err
	}

	var updatedInstanceID string
	if err = tx.QueryRow(ctx, `UPDATE strategy_instances SET last_evaluated_at=$4,updated_at=$4 WHERE id=$1 AND user_id=$2 AND state_version=$3 AND current_state='AI_MONITORING' AND status='ACTIVE' AND execution_mode='PAPER' RETURNING id::text`, instance.ID, instance.UserID, expectedVersion, evaluatedAt).Scan(&updatedInstanceID); errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validAIPaperCommit(instance Instance, expectedVersion int, decision Decision, evaluation risk.RiskEvaluation, fill AIPaperFill, evaluatedAt time.Time) bool {
	if instance.StrategyIdentifier != "ai_shadow" || instance.ExecutionMode != Paper || instance.CurrentState != AIMonitoring || instance.Status != "ACTIVE" || expectedVersion < 1 || expectedVersion != instance.StateVersion || evaluatedAt.IsZero() {
		return false
	}
	if decision.ProposedAction == nil || decision.Source != "AI" || (decision.InstrumentType != "EQUITY" && decision.InstrumentType != "CRYPTO") || decision.ProposedState != AIMonitoring || !json.Valid(decision.Rationale) || len(decision.Rationale) == 0 || decision.Rationale[0] != '{' {
		return false
	}
	if !validAIProposalQuoteReferenceEvidence(decision, evaluatedAt) {
		return false
	}
	action := decision.ProposedAction
	if action.ID == "" || action.CorrelationID == "" || action.Source != risk.SourceAI || action.FinancialAccountID != instance.FinancialAccountID || action.MandateID == nil || *action.MandateID != instance.AutomationMandateID || action.MandateVersion == nil || *action.MandateVersion != instance.MandateVersion || action.Option != nil || action.RequiresMargin {
		return false
	}
	if evaluation.ID == "" || evaluation.UserID != instance.UserID || evaluation.AccountID != instance.FinancialAccountID || evaluation.MandateID == nil || *evaluation.MandateID != instance.AutomationMandateID || evaluation.MandateVersion == nil || *evaluation.MandateVersion != instance.MandateVersion || evaluation.Decision != risk.Allow || evaluation.ApprovalRequired || evaluation.Mode != "PAPER" || evaluation.PlatformExecutionAvailable {
		return false
	}
	if fill.Status != SimulatedFilled || !fill.SimulationOnly || fill.Reason != "paper_simulation_only_no_broker_order" || fill.Instrument != decision.InstrumentType || fill.Symbol != action.Instrument || fill.Side != action.Side || !sameAIPaperDecimal(fill.Quantity, action.Quantity) || !sameAIPaperDecimal(fill.RequestedNotional, action.Notional) || action.EstimatedPrice == nil || !sameAIPaperDecimal(fill.ReferencePrice, *action.EstimatedPrice) || !fill.SimulatedAt.Equal(evaluatedAt) {
		return false
	}
	if fill.MarketObservedAt.IsZero() || fill.MarketProvider == "" || fill.MarketFeed == "" || fill.MarketQuality == "" || fill.PricingBasis == "" {
		return false
	}
	if decision.QuoteReference.Symbol != fill.Symbol || decision.QuoteReference.Side != fill.Side || !sameAIPaperDecimal(decision.QuoteReference.Price, fill.ReferencePrice) || decision.QuoteReference.Basis != fill.PricingBasis || decision.QuoteReference.Provider != fill.MarketProvider || decision.QuoteReference.Feed != fill.MarketFeed || decision.QuoteReference.Quality != fill.MarketQuality || !decision.QuoteReference.ObservedAt.Equal(fill.MarketObservedAt) {
		return false
	}
	for _, value := range []string{fill.Quantity, fill.RequestedNotional, fill.ReferencePrice, fill.FillPrice, fill.GrossNotional, fill.Fee, fill.PreviousCash, fill.PreviousPositionQuantity, fill.ResultingCash, fill.ResultingPositionQuantity} {
		if !validAIPaperDecimal(value) {
			return false
		}
	}
	return true
}

func sameAIPaperDecimal(left, right string) bool {
	leftValue, leftOK := new(big.Rat).SetString(left)
	rightValue, rightOK := new(big.Rat).SetString(right)
	return leftOK && rightOK && leftValue.Cmp(rightValue) == 0
}
