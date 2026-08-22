package orderintent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ db *pgxpool.Pool }

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore { return &PostgresStore{db: db} }

const intentColumns = `i.id::text,i.financial_account_id::text,i.source,i.provider_name,i.product_id,i.base_asset,i.quote_currency,i.side,i.order_type,i.requested_size::text,i.requested_size_currency,i.status,i.version,i.created_at,i.updated_at,p.provider_name,p.feed,p.preview_state,p.base_size::text,p.quote_size::text,p.order_total::text,p.commission_total::text,p.best_bid::text,p.best_ask::text,p.estimated_average_filled_price::text,p.slippage::text,p.provider_trading_authorized,p.block_reasons,p.warnings,p.previewed_at,p.expires_at,p.evidence_hash`

func scanIntent(row pgx.Row) (Intent, []byte, error) {
	var intent Intent
	var requestedAmount string
	var requestedCurrency string
	var orderTotal string
	var commissionTotal string
	var bestBid *string
	var bestAsk *string
	var estimatedPrice *string
	var slippage *string
	var blockReasonsJSON []byte
	var warningsJSON []byte
	var evidenceHash []byte
	err := row.Scan(
		&intent.ID, &intent.FinancialAccountID, &intent.Source, &intent.Provider, &intent.ProductID, &intent.BaseAsset, &intent.QuoteCurrency,
		&intent.Side, &intent.OrderType, &requestedAmount, &requestedCurrency, &intent.Status, &intent.Version, &intent.CreatedAt, &intent.UpdatedAt,
		&intent.Preview.Provider, &intent.Preview.Feed, &intent.Preview.PreviewState, &intent.Preview.BaseSize, &intent.Preview.QuoteSize,
		&orderTotal, &commissionTotal, &bestBid, &bestAsk, &estimatedPrice, &slippage, &intent.Preview.ProviderTradingAuthorized,
		&blockReasonsJSON, &warningsJSON, &intent.Preview.PreviewedAt, &intent.Preview.ExpiresAt, &evidenceHash,
	)
	if err != nil {
		return Intent{}, nil, err
	}
	intent.RequestedSize = financial.Money{Amount: financial.Decimal(canonicalStoredDecimal(requestedAmount)), Currency: requestedCurrency}
	intent.Preview.BaseSize = financial.Decimal(canonicalStoredDecimal(string(intent.Preview.BaseSize)))
	intent.Preview.QuoteSize = financial.Decimal(canonicalStoredDecimal(string(intent.Preview.QuoteSize)))
	intent.Preview.OrderTotal = financial.Money{Amount: financial.Decimal(canonicalStoredDecimal(orderTotal)), Currency: "USD"}
	intent.Preview.CommissionTotal = financial.Money{Amount: financial.Decimal(canonicalStoredDecimal(commissionTotal)), Currency: "USD"}
	if bestBid != nil {
		intent.Preview.BestBid = &financial.Money{Amount: financial.Decimal(canonicalStoredDecimal(*bestBid)), Currency: "USD"}
	}
	if bestAsk != nil {
		intent.Preview.BestAsk = &financial.Money{Amount: financial.Decimal(canonicalStoredDecimal(*bestAsk)), Currency: "USD"}
	}
	if estimatedPrice != nil {
		intent.Preview.EstimatedAverageFilledPrice = &financial.Money{Amount: financial.Decimal(canonicalStoredDecimal(*estimatedPrice)), Currency: "USD"}
	}
	if slippage != nil {
		value := financial.Decimal(canonicalStoredDecimal(*slippage))
		intent.Preview.Slippage = &value
	}
	if err = json.Unmarshal(blockReasonsJSON, &intent.Preview.BlockReasons); err != nil {
		return Intent{}, nil, err
	}
	if err = json.Unmarshal(warningsJSON, &intent.Preview.Warnings); err != nil {
		return Intent{}, nil, err
	}
	intent.ReviewScope = ProposalReviewOnly
	intent.SubmissionAvailable = false
	intent.RiskApprovalAvailable = false
	intent.AIExecutionAuthority = false
	intent.LiveExecutionAvailable = false
	return intent, bytes.Clone(evidenceHash), nil
}

func selectIntent(suffix string) string {
	return `SELECT ` + intentColumns + ` FROM order_intents i JOIN order_intent_previews p ON p.order_intent_id=i.id AND p.intent_version=1 ` + suffix
}

func (store *PostgresStore) ByIdempotency(ctx context.Context, userID, idempotencyKey string) (*Intent, []byte, error) {
	var requestHash []byte
	intent, _, err := scanIntent(store.db.QueryRow(ctx, selectIntent(`WHERE i.user_id=$1 AND i.idempotency_key=$2`), userID, idempotencyKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if err = store.db.QueryRow(ctx, `SELECT request_hash FROM order_intents WHERE id=$1 AND user_id=$2`, intent.ID, userID).Scan(&requestHash); err != nil {
		return nil, nil, err
	}
	return &intent, bytes.Clone(requestHash), nil
}

func (store *PostgresStore) Create(ctx context.Context, input draft) (Intent, []byte, error) {
	transaction, err := store.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Intent{}, nil, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var intentID string
	err = transaction.QueryRow(ctx, `INSERT INTO order_intents(user_id,financial_account_id,source,idempotency_key,request_hash,provider_name,product_id,base_asset,quote_currency,side,order_type,requested_size,requested_size_currency,status,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'coinbase',$6,$7,'USD',$8,'MARKET_IOC',$9,$10,$11,1,$12,$12) RETURNING id::text`, input.UserID, input.FinancialAccountID, input.Source, input.IdempotencyKey, input.RequestHash, input.ProductID, input.BaseAsset, input.Side, input.RequestedSize.Amount, input.RequestedSize.Currency, input.Status, input.CreatedAt).Scan(&intentID)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			_ = transaction.Rollback(ctx)
			existing, requestHash, lookupErr := store.ByIdempotency(ctx, input.UserID, input.IdempotencyKey)
			if lookupErr != nil {
				return Intent{}, nil, lookupErr
			}
			if existing == nil || !bytes.Equal(requestHash, input.RequestHash) {
				return Intent{}, nil, ErrIdempotencyConflict
			}
			return *existing, requestHash, nil
		}
		return Intent{}, nil, err
	}
	blocks, err := json.Marshal(input.Preview.BlockReasons)
	if err != nil {
		return Intent{}, nil, err
	}
	warnings, err := json.Marshal(input.Preview.Warnings)
	if err != nil {
		return Intent{}, nil, err
	}
	_, err = transaction.Exec(ctx, `INSERT INTO order_intent_previews(order_intent_id,intent_version,provider_name,feed,preview_state,base_size,quote_size,order_total,commission_total,best_bid,best_ask,estimated_average_filled_price,slippage,provider_trading_authorized,block_reasons,warnings,evidence_hash,previewed_at,expires_at,created_at) VALUES($1,1,'coinbase','advanced_trade_order_preview',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, intentID, input.Preview.PreviewState, input.Preview.BaseSize, input.Preview.QuoteSize, input.Preview.OrderTotal.Amount, input.Preview.CommissionTotal.Amount, moneyAmount(input.Preview.BestBid), moneyAmount(input.Preview.BestAsk), moneyAmount(input.Preview.EstimatedAverageFilledPrice), decimalValue(input.Preview.Slippage), input.Preview.ProviderTradingAuthorized, blocks, warnings, input.EvidenceHash, input.Preview.PreviewedAt, input.Preview.ExpiresAt, input.CreatedAt)
	if err != nil {
		return Intent{}, nil, err
	}
	eventType := "PROPOSED"
	if input.Status == Blocked {
		eventType = "BLOCKED"
	}
	_, err = transaction.Exec(ctx, `INSERT INTO order_intent_events(order_intent_id,sequence_number,event_type,metadata,occurred_at) VALUES($1,1,$2,'{}'::jsonb,$3)`, intentID, eventType, input.CreatedAt)
	if err != nil {
		return Intent{}, nil, err
	}
	if err = transaction.Commit(ctx); err != nil {
		return Intent{}, nil, err
	}
	created, _, err := store.Get(ctx, input.UserID, intentID)
	return created, bytes.Clone(input.RequestHash), err
}

func (store *PostgresStore) List(ctx context.Context, userID, accountID string, limit int) ([]Intent, error) {
	rows, err := store.db.Query(ctx, selectIntent(`WHERE i.user_id=$1 AND i.financial_account_id=$2 ORDER BY i.created_at DESC,i.id DESC LIMIT $3`), userID, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Intent, 0)
	for rows.Next() {
		intent, _, scanErr := scanIntent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, intent)
	}
	return result, rows.Err()
}

func (store *PostgresStore) Get(ctx context.Context, userID, intentID string) (Intent, []byte, error) {
	intent, evidenceHash, err := scanIntent(store.db.QueryRow(ctx, selectIntent(`WHERE i.id=$1 AND i.user_id=$2`), intentID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Intent{}, nil, ErrNotFound
	}
	return intent, evidenceHash, err
}

func (store *PostgresStore) Review(ctx context.Context, review reviewEvidence) (Intent, error) {
	transaction, err := store.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Intent{}, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var status string
	var version int64
	var expiresAt time.Time
	var evidenceHash []byte
	err = transaction.QueryRow(ctx, `SELECT i.status,i.version,p.expires_at,p.evidence_hash FROM order_intents i JOIN order_intent_previews p ON p.order_intent_id=i.id AND p.intent_version=1 WHERE i.id=$1 AND i.user_id=$2 FOR UPDATE OF i`, review.IntentID, review.UserID).Scan(&status, &version, &expiresAt, &evidenceHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Intent{}, ErrNotFound
	}
	if err != nil {
		return Intent{}, err
	}
	if status != ReviewRequired || version != review.ExpectedVersion || !bytes.Equal(evidenceHash, review.EvidenceHash) {
		return Intent{}, ErrConflict
	}
	if !review.ReviewedAt.Before(expiresAt) {
		return Intent{}, ErrExpired
	}
	result, err := transaction.Exec(ctx, `UPDATE order_intents SET status='USER_APPROVED_NONEXECUTABLE',version=version+1,updated_at=GREATEST($3,updated_at+interval '1 microsecond') WHERE id=$1 AND user_id=$2 AND status='REVIEW_REQUIRED' AND version=$4`, review.IntentID, review.UserID, review.ReviewedAt, review.ExpectedVersion)
	if err != nil {
		return Intent{}, err
	}
	if result.RowsAffected() != 1 {
		return Intent{}, ErrConflict
	}
	_, err = transaction.Exec(ctx, `INSERT INTO order_intent_reviews(order_intent_id,intent_version,user_id,decision,approval_scope,mfa_method,evidence_hash,reviewed_at,created_at) VALUES($1,$2,$3,'APPROVE','PROPOSAL_REVIEW_ONLY',$4,$5,$6,$6)`, review.IntentID, review.ExpectedVersion, review.UserID, review.MFAMethod, review.EvidenceHash, review.ReviewedAt)
	if err != nil {
		return Intent{}, err
	}
	_, err = transaction.Exec(ctx, `INSERT INTO order_intent_events(order_intent_id,sequence_number,event_type,metadata,occurred_at) VALUES($1,2,'USER_REVIEWED_NONEXECUTABLE','{"submission_available":false,"risk_approval_available":false}'::jsonb,$2)`, review.IntentID, review.ReviewedAt)
	if err != nil {
		return Intent{}, err
	}
	if err = transaction.Commit(ctx); err != nil {
		return Intent{}, err
	}
	intent, _, err := store.Get(ctx, review.UserID, review.IntentID)
	return intent, err
}

func moneyAmount(value *financial.Money) any {
	if value == nil {
		return nil
	}
	return value.Amount
}

func decimalValue(value *financial.Decimal) any {
	if value == nil {
		return nil
	}
	return *value
}

func canonicalStoredDecimal(value string) string {
	if strings.Contains(value, ".") {
		value = strings.TrimRight(strings.TrimRight(value, "0"), ".")
	}
	if value == "-0" || value == "" {
		return "0"
	}
	return value
}
