package execution

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
)

// SettleBrokerAccount remains runtime-unwired. It never submits/cancels orders,
// changes real holdings or clears existing reconciliation/risk controls. Only a
// complete current account proof matching the pinned opening funding and exact
// immutable final fills can release this order's internal capital fence.
func (s *PostgresStore) SettleBrokerAccount(ctx context.Context, ownerID, orderID string, vault FinancialCredentialReader, provider AccountSettlementProvider) (AccountSettlement, error) {
	if !validUUID(ownerID) || !validUUID(orderID) || vault == nil || provider == nil {
		return AccountSettlement{}, ErrNotAuthorized
	}
	sub, a, generation, err := s.loadRecoveryContext(ctx, ownerID, orderID)
	if err != nil {
		return AccountSettlement{}, err
	}
	// A retry returns the immutable historical receipt, not another observation
	// of balances which may already belong to a later order. Never apply twice.
	if saved, err := s.ReadAccountSettlement(ctx, ownerID, orderID); err == nil {
		return saved, nil
	} else if !errors.Is(err, ErrNotFound) {
		return AccountSettlement{}, err
	}
	if a.ProviderOrderID == "" {
		return AccountSettlement{}, ErrSubmissionUnknown
	}
	var terminalBody []byte
	if err = s.db.QueryRow(ctx, `SELECT payload FROM execution_order_terminals WHERE order_id=$1 AND owner_id=$2`, orderID, ownerID).Scan(&terminalBody); err != nil {
		return AccountSettlement{}, ErrUnreconciled
	}
	var terminal TerminalReport
	if json.Unmarshal(terminalBody, &terminal) != nil {
		return AccountSettlement{}, ErrUnreconciled
	}
	raw, materialGeneration, err := vault.RetrieveFinancialVersion(ctx, credential.Locator{ConnectionID: sub.Order.Request.ConnectionID, UserID: ownerID, Class: credential.Financial})
	defer clear(raw)
	if err != nil || materialGeneration != generation || len(raw) > 16384 {
		return AccountSettlement{}, ErrNotAuthorized
	}
	var cr financial.Credentials
	if json.Unmarshal(raw, &cr) != nil || cr.PortfolioID != sub.PortfolioID || cr.APIKeyName == "" || cr.APIPrivateKey == "" {
		return AccountSettlement{}, ErrNotAuthorized
	}
	defer func() { cr = financial.Credentials{} }()
	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	e, err := provider.CollectAccountSettlement(readCtx, &cr, sub, a)
	if err != nil || readCtx.Err() != nil {
		return AccountSettlement{}, ErrUnreconciled
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return AccountSettlement{}, err
	}
	defer rollback(tx)
	var current int64
	if err = tx.QueryRow(ctx, `SELECT lock_execution_settlement_controls($1,$2)`, orderID, sub.PortfolioID).Scan(&current); err != nil || current != materialGeneration {
		return AccountSettlement{}, ErrNotAuthorized
	}
	if saved, err := readAccountSettlement(ctx, tx, ownerID, orderID); err == nil {
		return saved, nil
	} else if !errors.Is(err, ErrNotFound) {
		return AccountSettlement{}, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return AccountSettlement{}, err
	}
	if ValidateAccountSettlementEvidence(sub, a, e, now) != nil || e.StartedAt.Before(terminal.ObservedAt) {
		return AccountSettlement{}, ErrUnreconciled
	}
	// Read the exact immutable facts again under the account lock. Aggregate
	// equality alone cannot substitute one trade identity for another.
	if err = tx.QueryRow(ctx, `SELECT payload FROM execution_order_terminals WHERE order_id=$1 AND owner_id=$2`, orderID, ownerID).Scan(&terminalBody); err != nil || json.Unmarshal(terminalBody, &terminal) != nil || !sameTerminal(terminal, e.Observation.terminal()) {
		return AccountSettlement{}, ErrUnreconciled
	}
	if err = matchSettlementFills(ctx, tx, sub.Order, a, e.Observation, now); err != nil {
		return AccountSettlement{}, err
	}
	body, err := json.Marshal(e)
	if err != nil || len(body) > 2*1024*1024 {
		return AccountSettlement{}, ErrUnreconciled
	}
	r := sub.Order.Request
	_, err = tx.Exec(ctx, `INSERT INTO execution_account_settlements(order_id,owner_id,financial_account_id,provider_connection_id,capital_bucket_id,provider_order_id,credential_generation,portfolio_id,terminal_status,fill_count,base_quantity,gross_usd,fee_usd,opening_cash_usd,opening_base,closing_cash_usd,closing_base,started_at,observed_at,evidence)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`, orderID, ownerID, r.AccountID, r.ConnectionID, r.CapitalBucketID, a.ProviderOrderID, materialGeneration, sub.PortfolioID, terminal.Status, terminal.FillCount, terminal.BaseQuantity, terminal.GrossUSD, terminal.FeeUSD, sub.Preflight.CashUSD, sub.Preflight.TotalBase, e.CashUSD, e.TotalBase, e.StartedAt.Round(time.Microsecond), e.CompletedAt.Round(time.Microsecond), body)
	if err != nil {
		return AccountSettlement{}, mapError(err)
	}
	// Release happens in the receipt trigger, within this same transaction.
	// Wall-clock expiry after insert still rolls receipt and release back.
	if err = tx.QueryRow(ctx, `SELECT lock_execution_settlement_controls($1,$2)`, orderID, sub.PortfolioID).Scan(&current); err != nil || current != materialGeneration {
		return AccountSettlement{}, ErrNotAuthorized
	}
	if err = tx.Commit(ctx); err != nil {
		return AccountSettlement{}, ErrCommitUnknown
	}
	return s.ReadAccountSettlement(ctx, ownerID, orderID)
}

func matchSettlementFills(ctx context.Context, tx pgx.Tx, o Order, a Attempt, b BrokerObservation, now time.Time) error {
	want := make(map[string]Fill, len(b.Fills))
	for _, f := range b.Fills {
		n, err := normalizeFill(o, a, f, now)
		if err != nil {
			return ErrUnreconciled
		}
		want[n.TradeID] = n
	}
	rows, err := tx.Query(ctx, `SELECT payload FROM execution_fills WHERE order_id=$1 AND owner_id=$2`, o.ID, o.Request.OwnerID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var body []byte
		var f Fill
		if rows.Scan(&body) != nil || json.Unmarshal(body, &f) != nil || !sameFill(f, want[f.TradeID]) {
			return ErrUnreconciled
		}
		delete(want, f.TradeID)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(want) != 0 {
		return ErrUnreconciled
	}
	return nil
}

func (s *PostgresStore) ReadAccountSettlement(ctx context.Context, ownerID, orderID string) (AccountSettlement, error) {
	if !validUUID(ownerID) || !validUUID(orderID) {
		return AccountSettlement{}, ErrInvalid
	}
	return readAccountSettlement(ctx, s.db, ownerID, orderID)
}

func readAccountSettlement(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, ownerID, orderID string) (AccountSettlement, error) {
	var r AccountSettlement
	err := q.QueryRow(ctx, `SELECT s.order_id::text,s.owner_id::text,s.financial_account_id::text,s.provider_connection_id::text,o.client_order_id::text,s.provider_order_id::text,o.request->>'ProductID',o.request->>'Side',s.portfolio_id::text,s.credential_generation,s.terminal_status,s.fill_count,s.base_quantity::text,s.gross_usd::text,s.fee_usd::text,s.opening_cash_usd::text,s.opening_base::text,s.closing_cash_usd::text,s.closing_base::text,s.started_at,s.observed_at,s.recorded_at
	 FROM execution_account_settlements s JOIN execution_orders o ON o.id=s.order_id WHERE s.order_id=$1 AND s.owner_id=$2`, orderID, ownerID).Scan(&r.OrderID, &r.OwnerID, &r.AccountID, &r.ConnectionID, &r.ClientOrderID, &r.ProviderOrderID, &r.ProductID, &r.Side, &r.PortfolioID, &r.CredentialGeneration, &r.TerminalStatus, &r.FillCount, &r.BaseQuantity, &r.GrossUSD, &r.FeeUSD, &r.OpeningCashUSD, &r.OpeningBase, &r.ClosingCashUSD, &r.ClosingBase, &r.StartedAt, &r.ObservedAt, &r.RecordedAt)
	if err != nil {
		return AccountSettlement{}, mapError(err)
	}
	for _, v := range []*string{&r.BaseQuantity, &r.GrossUSD, &r.FeeUSD, &r.OpeningCashUSD, &r.OpeningBase, &r.ClosingCashUSD, &r.ClosingBase} {
		n, ok := amount(*v)
		if !ok {
			return AccountSettlement{}, ErrInvalid
		}
		*v = canonical(n)
	}
	return r, nil
}
