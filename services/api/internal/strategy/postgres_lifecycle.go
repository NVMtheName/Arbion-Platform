package strategy

import (
	"context"
	"encoding/json"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
)

const lifecycleColumns = `id::text,event_id,strategy_instance_id::text,event_type,previous_state,new_state,state_version,metadata,occurred_at`

func scanLifecycle(row pgx.Row) (result LifecycleResult, err error) {
	err = row.Scan(
		&result.ID,
		&result.EventID,
		&result.StrategyInstanceID,
		&result.EventType,
		&result.PreviousState,
		&result.NewState,
		&result.StateVersion,
		&result.Metadata,
		&result.OccurredAt,
	)
	return result, err
}

type paperPosition struct {
	ID, PortfolioID, Symbol, OptionType, Strike, Expiration, Quantity string
}

type equityPosition struct {
	ID, Quantity, AveragePrice string
}

func (s *PostgresStore) RecordLifecycle(ctx context.Context, userID, instanceID string, command LifecycleCommand, occurredAt time.Time) (LifecycleResult, error) {
	if userID == "" || instanceID == "" || !evaluationEventID.MatchString(command.EventID) || command.ExpectedStateVersion < 1 || !command.ConfirmPaperSimulation || occurredAt.IsZero() {
		return LifecycleResult{}, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return LifecycleResult{}, err
	}
	defer tx.Rollback(ctx)

	instance, err := scanInstance(tx.QueryRow(ctx, `SELECT `+instanceColumns+` FROM strategy_instances WHERE id=$1 AND user_id=$2 FOR UPDATE`, instanceID, userID))
	if err != nil {
		return LifecycleResult{}, err
	}
	existing, err := scanLifecycle(tx.QueryRow(ctx, `SELECT `+lifecycleColumns+` FROM strategy_lifecycle_events WHERE strategy_instance_id=$1 AND user_id=$2 AND event_id=$3`, instanceID, userID, command.EventID))
	if err == nil {
		if existing.EventType != command.EventType {
			return LifecycleResult{}, ErrConflict
		}
		existing.Duplicate = true
		return existing, nil
	}
	if err != pgx.ErrNoRows {
		return LifecycleResult{}, err
	}
	if (instance.Status != "ACTIVE" && instance.Status != "PAUSED") || instance.ExecutionMode != Paper || instance.StrategyIdentifier != "wheel" {
		return LifecycleResult{}, ErrInvalid
	}
	if instance.StateVersion != command.ExpectedStateVersion {
		return LifecycleResult{}, ErrConflict
	}
	nextState, err := ApplyLifecycle(instance.StrategyIdentifier, instance.CurrentState, command.EventType)
	if err != nil {
		return LifecycleResult{}, ErrInvalid
	}

	optionType := "PUT"
	if instance.CurrentState == ShortCallOpen {
		optionType = "CALL"
	}
	position, err := loadSingleOpenOption(ctx, tx, instanceID, userID, optionType)
	if err != nil {
		return LifecycleResult{}, err
	}
	contracts, ok := new(big.Rat).SetString(position.Quantity)
	if !ok || contracts.Sign() >= 0 {
		return LifecycleResult{}, ErrInvalid
	}
	contracts.Neg(contracts)
	if contracts.Denom().Cmp(big.NewInt(1)) != 0 {
		return LifecycleResult{}, ErrInvalid
	}
	strike, ok := new(big.Rat).SetString(position.Strike)
	if !ok || strike.Sign() <= 0 {
		return LifecycleResult{}, ErrInvalid
	}
	if command.EventType == ExpireWorthless && !optionCanExpire(position.Expiration, occurredAt) {
		return LifecycleResult{}, ErrInvalid
	}

	contractShares := new(big.Rat).Mul(contracts, big.NewRat(100, 1))
	assignmentValue := new(big.Rat).Mul(strike, contractShares)
	cashDelta := new(big.Rat)
	shareDelta := new(big.Rat)
	switch command.EventType {
	case Assignment:
		cashDelta.Neg(assignmentValue)
		shareDelta.Set(contractShares)
	case CallAway:
		cashDelta.Set(assignmentValue)
		shareDelta.Neg(contractShares)
	}

	metadata, err := json.Marshal(map[string]any{
		"cash_delta":               cashDelta.FloatString(10),
		"contracts":                contracts.FloatString(10),
		"event_id":                 command.EventID,
		"event_type":               command.EventType,
		"expiration":               position.Expiration,
		"live_execution_available": false,
		"option_type":              position.OptionType,
		"share_delta":              shareDelta.FloatString(10),
		"simulation":               true,
		"strike":                   position.Strike,
		"symbol":                   position.Symbol,
	})
	if err != nil {
		return LifecycleResult{}, err
	}
	updated, err := tx.Exec(ctx, `UPDATE paper_positions SET quantity=0,metadata=metadata||$2::jsonb,updated_at=$3 WHERE id=$1 AND quantity=$4`, position.ID, metadata, occurredAt, position.Quantity)
	if err != nil {
		return LifecycleResult{}, err
	}
	if updated.RowsAffected() != 1 {
		return LifecycleResult{}, ErrConflict
	}

	if command.EventType == Assignment {
		if err = addPaperShares(ctx, tx, position.PortfolioID, position.Symbol, contractShares, strike, metadata, occurredAt); err != nil {
			return LifecycleResult{}, err
		}
	}
	if command.EventType == CallAway {
		if err = removePaperShares(ctx, tx, position.PortfolioID, position.Symbol, contractShares, metadata, occurredAt); err != nil {
			return LifecycleResult{}, err
		}
	}
	var resultingCash string
	err = tx.QueryRow(ctx, `UPDATE paper_portfolios SET cash=cash+$3,version=version+1,updated_at=$4 WHERE id=$1 AND user_id=$2 AND cash+$3>=0 RETURNING cash::text`, position.PortfolioID, userID, cashDelta.FloatString(10), occurredAt).Scan(&resultingCash)
	if err == pgx.ErrNoRows {
		return LifecycleResult{}, ErrInvalid
	}
	if err != nil {
		return LifecycleResult{}, err
	}
	eventMetadata, err := json.Marshal(map[string]any{
		"cash_delta":               cashDelta.FloatString(10),
		"contracts":                contracts.FloatString(10),
		"event_id":                 command.EventID,
		"event_type":               command.EventType,
		"expiration":               position.Expiration,
		"live_execution_available": false,
		"option_type":              position.OptionType,
		"resulting_cash":           resultingCash,
		"share_delta":              shareDelta.FloatString(10),
		"simulation":               true,
		"strike":                   position.Strike,
		"symbol":                   position.Symbol,
	})
	if err != nil {
		return LifecycleResult{}, err
	}

	var nextVersion int
	err = tx.QueryRow(ctx, `UPDATE strategy_instances SET current_state=$5,state_version=state_version+1,updated_at=$6 WHERE id=$1 AND user_id=$2 AND state_version=$3 AND current_state=$4 AND status IN ('ACTIVE','PAUSED') AND execution_mode='PAPER' AND strategy_identifier='wheel' RETURNING state_version`, instance.ID, userID, command.ExpectedStateVersion, instance.CurrentState, nextState, occurredAt).Scan(&nextVersion)
	if err == pgx.ErrNoRows {
		return LifecycleResult{}, ErrConflict
	}
	if err != nil {
		return LifecycleResult{}, err
	}

	result, err := scanLifecycle(tx.QueryRow(ctx, `INSERT INTO strategy_lifecycle_events(strategy_instance_id,user_id,event_id,event_type,previous_state,new_state,state_version,metadata,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+lifecycleColumns, instance.ID, userID, command.EventID, command.EventType, instance.CurrentState, nextState, nextVersion, eventMetadata, occurredAt))
	if err != nil {
		return LifecycleResult{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO strategy_state_transitions(strategy_instance_id,previous_state,new_state,state_version,trigger,metadata,occurred_at) VALUES($1,$2,$3,$4,'PAPER_LIFECYCLE_EVENT',$5,$6)`, instance.ID, instance.CurrentState, nextState, nextVersion, eventMetadata, occurredAt)
	if err != nil {
		return LifecycleResult{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO decision_journal_entries(user_id,financial_account_id,mandate_id,mandate_version,strategy_instance_id,strategy_state,source,decision_type,structured_rationale,resulting_state,created_at) VALUES($1,$2,$3,$4,$5,$6,'LIFECYCLE',$7,$8,$9,$10)`, userID, instance.FinancialAccountID, instance.AutomationMandateID, instance.MandateVersion, instance.ID, instance.CurrentState, command.EventType, eventMetadata, nextState, occurredAt)
	if err != nil {
		return LifecycleResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return LifecycleResult{}, err
	}
	return result, nil
}

func loadSingleOpenOption(ctx context.Context, tx pgx.Tx, instanceID, userID, optionType string) (paperPosition, error) {
	rows, err := tx.Query(ctx, `SELECT p.id::text,p.paper_portfolio_id::text,p.symbol,p.option_type,p.strike::text,p.expiration::text,p.quantity::text FROM paper_positions p JOIN paper_portfolios f ON f.id=p.paper_portfolio_id WHERE f.strategy_instance_id=$1 AND f.user_id=$2 AND p.instrument='OPTION' AND p.option_type=$3 AND p.quantity<0 ORDER BY p.id FOR UPDATE OF p`, instanceID, userID, optionType)
	if err != nil {
		return paperPosition{}, err
	}
	defer rows.Close()
	positions := []paperPosition{}
	for rows.Next() {
		var position paperPosition
		if err = rows.Scan(&position.ID, &position.PortfolioID, &position.Symbol, &position.OptionType, &position.Strike, &position.Expiration, &position.Quantity); err != nil {
			return paperPosition{}, err
		}
		positions = append(positions, position)
	}
	if err = rows.Err(); err != nil {
		return paperPosition{}, err
	}
	if len(positions) != 1 {
		return paperPosition{}, ErrInvalid
	}
	return positions[0], nil
}

func loadEquityPositions(ctx context.Context, tx pgx.Tx, portfolioID, symbol string) ([]equityPosition, error) {
	rows, err := tx.Query(ctx, `SELECT id::text,quantity::text,average_price::text FROM paper_positions WHERE paper_portfolio_id=$1 AND symbol=$2 AND instrument='EQUITY' AND quantity>0 ORDER BY id FOR UPDATE`, portfolioID, symbol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	positions := []equityPosition{}
	for rows.Next() {
		var position equityPosition
		if err = rows.Scan(&position.ID, &position.Quantity, &position.AveragePrice); err != nil {
			return nil, err
		}
		positions = append(positions, position)
	}
	return positions, rows.Err()
}

func addPaperShares(ctx context.Context, tx pgx.Tx, portfolioID, symbol string, shares, strike *big.Rat, metadata []byte, occurredAt time.Time) error {
	positions, err := loadEquityPositions(ctx, tx, portfolioID, symbol)
	if err != nil {
		return err
	}
	if len(positions) > 1 {
		return ErrInvalid
	}
	if len(positions) == 0 {
		_, err = tx.Exec(ctx, `INSERT INTO paper_positions(paper_portfolio_id,symbol,instrument,quantity,average_price,metadata,updated_at) VALUES($1,$2,'EQUITY',$3,$4,$5,$6)`, portfolioID, symbol, shares.FloatString(10), strike.FloatString(10), metadata, occurredAt)
		return err
	}
	position := positions[0]
	quantity, quantityOK := new(big.Rat).SetString(position.Quantity)
	average, averageOK := new(big.Rat).SetString(position.AveragePrice)
	if !quantityOK || !averageOK || quantity.Sign() <= 0 || average.Sign() < 0 {
		return ErrInvalid
	}
	newQuantity := new(big.Rat).Add(quantity, shares)
	weightedCost := new(big.Rat).Add(new(big.Rat).Mul(quantity, average), new(big.Rat).Mul(shares, strike))
	newAverage := new(big.Rat).Quo(weightedCost, newQuantity)
	command, err := tx.Exec(ctx, `UPDATE paper_positions SET quantity=$2,average_price=$3,metadata=metadata||$4::jsonb,updated_at=$5 WHERE id=$1 AND quantity=$6`, position.ID, newQuantity.FloatString(10), newAverage.FloatString(10), metadata, occurredAt, position.Quantity)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func removePaperShares(ctx context.Context, tx pgx.Tx, portfolioID, symbol string, shares *big.Rat, metadata []byte, occurredAt time.Time) error {
	positions, err := loadEquityPositions(ctx, tx, portfolioID, symbol)
	if err != nil {
		return err
	}
	if len(positions) != 1 {
		return ErrInvalid
	}
	position := positions[0]
	quantity, ok := new(big.Rat).SetString(position.Quantity)
	if !ok || quantity.Cmp(shares) < 0 {
		return ErrInvalid
	}
	newQuantity := new(big.Rat).Sub(quantity, shares)
	command, err := tx.Exec(ctx, `UPDATE paper_positions SET quantity=$2,metadata=metadata||$3::jsonb,updated_at=$4 WHERE id=$1 AND quantity=$5`, position.ID, newQuantity.FloatString(10), metadata, occurredAt, position.Quantity)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func optionCanExpire(expirationText string, occurredAt time.Time) bool {
	expiration, err := time.Parse("2006-01-02", expirationText)
	if err != nil {
		return false
	}
	local := occurredAt.In(easternLocation)
	expirationLocal := time.Date(expiration.Year(), expiration.Month(), expiration.Day(), 16, 0, 0, 0, easternLocation)
	return !local.Before(expirationLocal)
}
