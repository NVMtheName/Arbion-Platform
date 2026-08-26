package financialconnection

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
)

const reconciliationColumns = `id::text,financial_account_id::text,provider_name,comparison_status,balances_status,positions_status,performance_status,realized_performance_status,autonomy_signal,blocks_new_actions,observed_position_count,performance_position_count,change_count,cash_amount::text,cash_currency,available_cash_amount::text,available_cash_currency,buying_power_amount::text,buying_power_currency,account_value_amount::text,account_value_currency,previous_reconciliation_id::text,changes,evidence_hash,observed_at,created_at`

func scanReconciliation(row pgx.Row) (PortfolioReconciliation, error) {
	var report PortfolioReconciliation
	var previousID *string
	var cashAmount, cashCurrency, availableCashAmount, availableCashCurrency, buyingPowerAmount, buyingPowerCurrency, accountValueAmount, accountValueCurrency *string
	var rawChanges, evidenceHash []byte
	err := row.Scan(&report.ID, &report.FinancialAccountID, &report.Provider, &report.ComparisonStatus, &report.BalancesStatus, &report.PositionsStatus, &report.PerformanceStatus, &report.RealizedPerformanceStatus, &report.AutonomySignal, &report.BlocksNewActions, &report.ObservedPositionCount, &report.PerformancePositionCount, &report.ChangeCount, &cashAmount, &cashCurrency, &availableCashAmount, &availableCashCurrency, &buyingPowerAmount, &buyingPowerCurrency, &accountValueAmount, &accountValueCurrency, &previousID, &rawChanges, &evidenceHash, &report.ObservedAt, &report.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PortfolioReconciliation{}, ErrReconciliationNotFound
	}
	if err != nil {
		return PortfolioReconciliation{}, err
	}
	report.PreviousReconciliationID = previousID
	report.Balances.Cash = moneyPointer(cashAmount, cashCurrency)
	report.Balances.AvailableCash = moneyPointer(availableCashAmount, availableCashCurrency)
	report.Balances.BuyingPower = moneyPointer(buyingPowerAmount, buyingPowerCurrency)
	report.Balances.AccountValue = moneyPointer(accountValueAmount, accountValueCurrency)
	report.EvidenceHash = hex.EncodeToString(evidenceHash)
	report.Changes = []ReconciliationChange{}
	if err = json.Unmarshal(rawChanges, &report.Changes); err != nil {
		return PortfolioReconciliation{}, err
	}
	report.Positions = []ReconciliationPosition{}
	return report, nil
}

func (s *PostgresStore) LatestReconciliation(ctx context.Context, userID, accountID string) (PortfolioReconciliation, error) {
	report, err := scanReconciliation(s.db.QueryRow(ctx, `SELECT `+reconciliationColumns+` FROM portfolio_reconciliations WHERE user_id=$1 AND financial_account_id=$2 ORDER BY observed_at DESC,id DESC LIMIT 1`, userID, accountID))
	return s.loadReconciliationPositions(ctx, userID, accountID, report, err)
}

func (s *PostgresStore) LatestReliableReconciliation(ctx context.Context, userID, accountID string) (PortfolioReconciliation, error) {
	report, err := scanReconciliation(s.db.QueryRow(ctx, `SELECT `+reconciliationColumns+` FROM portfolio_reconciliations WHERE user_id=$1 AND financial_account_id=$2 AND balances_status='READY' AND positions_status='READY' ORDER BY observed_at DESC,id DESC LIMIT 1`, userID, accountID))
	return s.loadReconciliationPositions(ctx, userID, accountID, report, err)
}

func (s *PostgresStore) loadReconciliationPositions(ctx context.Context, userID, accountID string, report PortfolioReconciliation, err error) (PortfolioReconciliation, error) {
	if err != nil {
		return PortfolioReconciliation{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT symbol,instrument_type,direction,quantity::text,available_quantity::text,unavailable_to_trade_quantity::text,market_value_amount::text,market_value_currency,average_price_amount::text,average_price_currency,current_price_amount::text,current_price_currency,day_profit_loss_amount::text,day_profit_loss_currency,open_profit_loss_amount::text,open_profit_loss_currency,performance_status,price_basis FROM portfolio_reconciliation_positions WHERE reconciliation_id=$1 AND user_id=$2 AND financial_account_id=$3 ORDER BY symbol,instrument_type,direction`, report.ID, userID, accountID)
	if err != nil {
		return PortfolioReconciliation{}, err
	}
	defer rows.Close()
	for rows.Next() {
		position, scanErr := scanReconciliationPosition(rows)
		if scanErr != nil {
			return PortfolioReconciliation{}, scanErr
		}
		report.Positions = append(report.Positions, position)
	}
	return report, rows.Err()
}

func scanReconciliationPosition(row pgx.Row) (ReconciliationPosition, error) {
	var position ReconciliationPosition
	var quantity string
	var available, unavailable, marketAmount, marketCurrency, averageAmount, averageCurrency, currentAmount, currentCurrency, dayAmount, dayCurrency, openAmount, openCurrency *string
	err := row.Scan(&position.Symbol, &position.InstrumentType, &position.Direction, &quantity, &available, &unavailable, &marketAmount, &marketCurrency, &averageAmount, &averageCurrency, &currentAmount, &currentCurrency, &dayAmount, &dayCurrency, &openAmount, &openCurrency, &position.PerformanceStatus, &position.PriceBasis)
	if err != nil {
		return ReconciliationPosition{}, err
	}
	position.Quantity = financial.Decimal(quantity)
	position.AvailableQuantity = decimalPointer(available)
	position.UnavailableQuantity = decimalPointer(unavailable)
	position.MarketValue = moneyPointer(marketAmount, marketCurrency)
	position.AveragePrice = moneyPointer(averageAmount, averageCurrency)
	position.CurrentPrice = moneyPointer(currentAmount, currentCurrency)
	position.DayProfitLoss = moneyPointer(dayAmount, dayCurrency)
	position.OpenProfitLoss = moneyPointer(openAmount, openCurrency)
	return position, nil
}

func decimalPointer(value *string) *financial.Decimal {
	if value == nil {
		return nil
	}
	decimal := financial.Decimal(*value)
	return &decimal
}

func moneyPointer(amount, currency *string) *financial.Money {
	if amount == nil || currency == nil {
		return nil
	}
	return &financial.Money{Amount: financial.Decimal(*amount), Currency: *currency}
}

func (s *PostgresStore) CreateReconciliation(ctx context.Context, userID string, report PortfolioReconciliation, evidenceHash []byte) (PortfolioReconciliation, error) {
	positions := append([]ReconciliationPosition(nil), report.Positions...)
	changes, err := json.Marshal(report.Changes)
	if err != nil {
		return PortfolioReconciliation{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PortfolioReconciliation{}, err
	}
	defer tx.Rollback(ctx)
	report, err = scanReconciliation(tx.QueryRow(ctx, `INSERT INTO portfolio_reconciliations(user_id,financial_account_id,provider_name,comparison_status,balances_status,positions_status,performance_status,realized_performance_status,autonomy_signal,blocks_new_actions,observed_position_count,performance_position_count,change_count,cash_amount,cash_currency,available_cash_amount,available_cash_currency,buying_power_amount,buying_power_currency,account_value_amount,account_value_currency,previous_reconciliation_id,changes,evidence_hash,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25) RETURNING `+reconciliationColumns, userID, report.FinancialAccountID, report.Provider, report.ComparisonStatus, report.BalancesStatus, report.PositionsStatus, report.PerformanceStatus, report.RealizedPerformanceStatus, report.AutonomySignal, report.BlocksNewActions, report.ObservedPositionCount, report.PerformancePositionCount, report.ChangeCount, moneyAmount(report.Balances.Cash), moneyCurrency(report.Balances.Cash), moneyAmount(report.Balances.AvailableCash), moneyCurrency(report.Balances.AvailableCash), moneyAmount(report.Balances.BuyingPower), moneyCurrency(report.Balances.BuyingPower), moneyAmount(report.Balances.AccountValue), moneyCurrency(report.Balances.AccountValue), report.PreviousReconciliationID, changes, evidenceHash, report.ObservedAt))
	if err != nil {
		return PortfolioReconciliation{}, err
	}
	for _, position := range positions {
		_, err = tx.Exec(ctx, `INSERT INTO portfolio_reconciliation_positions(reconciliation_id,user_id,financial_account_id,symbol,instrument_type,direction,quantity,available_quantity,unavailable_to_trade_quantity,market_value_amount,market_value_currency,average_price_amount,average_price_currency,current_price_amount,current_price_currency,day_profit_loss_amount,day_profit_loss_currency,open_profit_loss_amount,open_profit_loss_currency,performance_status,price_basis) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`, report.ID, userID, report.FinancialAccountID, position.Symbol, position.InstrumentType, position.Direction, position.Quantity, storedDecimal(position.AvailableQuantity), storedDecimal(position.UnavailableQuantity), moneyAmount(position.MarketValue), moneyCurrency(position.MarketValue), moneyAmount(position.AveragePrice), moneyCurrency(position.AveragePrice), moneyAmount(position.CurrentPrice), moneyCurrency(position.CurrentPrice), moneyAmount(position.DayProfitLoss), moneyCurrency(position.DayProfitLoss), moneyAmount(position.OpenProfitLoss), moneyCurrency(position.OpenProfitLoss), position.PerformanceStatus, position.PriceBasis)
		if err != nil {
			return PortfolioReconciliation{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return PortfolioReconciliation{}, err
	}
	report.Positions = positions
	report.EvidenceHash = hex.EncodeToString(evidenceHash)
	return report, nil
}

func moneyAmount(value *financial.Money) any {
	if value == nil {
		return nil
	}
	return value.Amount
}

func moneyCurrency(value *financial.Money) any {
	if value == nil {
		return nil
	}
	return value.Currency
}

func storedDecimal(value *financial.Decimal) any {
	if value == nil {
		return nil
	}
	return string(*value)
}
