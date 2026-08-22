package orderintent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ db *pgxpool.Pool }

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore { return &PostgresStore{db: db} }

const intentColumns = `i.id::text,i.financial_account_id::text,i.capital_bucket_id::text,i.source,i.provider_name,i.product_id,i.base_asset,i.quote_currency,i.side,i.order_type,i.requested_size::text,i.requested_size_currency,i.status,i.version,i.created_at,i.updated_at,p.provider_name,p.feed,p.preview_state,p.base_size::text,p.quote_size::text,p.order_total::text,p.commission_total::text,p.best_bid::text,p.best_ask::text,p.estimated_average_filled_price::text,p.slippage::text,p.provider_trading_authorized,p.block_reasons,p.warnings,p.previewed_at,p.expires_at,p.evidence_hash,p.product_rules_feed,p.product_type,p.product_status,p.base_increment::text,p.quote_increment::text,p.base_min_size::text,p.base_max_size::text,p.quote_min_size::text,p.quote_max_size::text,p.product_market_ioc_enabled,p.product_block_reasons,p.product_rules_observed_at,l.policy_version,l.risk_evaluation_id::text,l.capital_bucket_id::text,l.capital_bucket_name,l.allocation_type,l.allocation_value::text,l.protected_amount::text,l.allocation_limit::text,l.account_available_cash::text,l.account_reserved_cash::text,l.bucket_reserved_cash::text,l.target_available_quantity::text,l.target_reserved_quantity::text,l.proposed_notional::text,l.observed_at,r.decision,r.approval_required,r.platform_execution_available,r.reason_codes,r.warnings,r.checks,c.resource_type,c.resource_asset,c.quantity::text,c.reserved_at,c.expires_at`

func scanIntent(row pgx.Row) (Intent, []byte, error) {
	var intent Intent
	var requestedAmount string
	var capitalBucketID *string
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
	var productFeed, productType, productStatus *string
	var baseIncrement, quoteIncrement, baseMin, baseMax, quoteMin, quoteMax *string
	var productMarketIOCEnabled *bool
	var productBlockReasonsJSON []byte
	var productObservedAt *time.Time
	var policyVersion, riskEvaluationID, riskCapitalBucketID, riskBucketName, riskAllocationType *string
	var riskAllocationValue, riskProtectedAmount, riskAllocationLimit, riskAvailableCash, riskAccountReservedCash, riskBucketReservedCash, riskTargetQuantity, riskTargetReservedQuantity, riskProposedNotional *string
	var riskObservedAt *time.Time
	var riskDecision *string
	var riskApprovalRequired, riskPlatformExecution *bool
	var riskReasonsJSON, riskWarningsJSON, riskChecksJSON []byte
	var reservationResourceType, reservationAsset, reservationQuantity *string
	var reservationReservedAt, reservationExpiresAt *time.Time
	err := row.Scan(
		&intent.ID, &intent.FinancialAccountID, &capitalBucketID, &intent.Source, &intent.Provider, &intent.ProductID, &intent.BaseAsset, &intent.QuoteCurrency,
		&intent.Side, &intent.OrderType, &requestedAmount, &requestedCurrency, &intent.Status, &intent.Version, &intent.CreatedAt, &intent.UpdatedAt,
		&intent.Preview.Provider, &intent.Preview.Feed, &intent.Preview.PreviewState, &intent.Preview.BaseSize, &intent.Preview.QuoteSize,
		&orderTotal, &commissionTotal, &bestBid, &bestAsk, &estimatedPrice, &slippage, &intent.Preview.ProviderTradingAuthorized,
		&blockReasonsJSON, &warningsJSON, &intent.Preview.PreviewedAt, &intent.Preview.ExpiresAt, &evidenceHash,
		&productFeed, &productType, &productStatus, &baseIncrement, &quoteIncrement, &baseMin, &baseMax, &quoteMin, &quoteMax,
		&productMarketIOCEnabled, &productBlockReasonsJSON, &productObservedAt,
		&policyVersion, &riskEvaluationID, &riskCapitalBucketID, &riskBucketName, &riskAllocationType,
		&riskAllocationValue, &riskProtectedAmount, &riskAllocationLimit, &riskAvailableCash, &riskAccountReservedCash, &riskBucketReservedCash, &riskTargetQuantity, &riskTargetReservedQuantity, &riskProposedNotional, &riskObservedAt,
		&riskDecision, &riskApprovalRequired, &riskPlatformExecution, &riskReasonsJSON, &riskWarningsJSON, &riskChecksJSON,
		&reservationResourceType, &reservationAsset, &reservationQuantity, &reservationReservedAt, &reservationExpiresAt,
	)
	if err != nil {
		return Intent{}, nil, err
	}
	intent.RequestedSize = financial.Money{Amount: financial.Decimal(canonicalStoredDecimal(requestedAmount)), Currency: requestedCurrency}
	if capitalBucketID != nil {
		intent.CapitalBucketID = *capitalBucketID
	}
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
	if productFeed != nil {
		if productType == nil || productStatus == nil || baseIncrement == nil || quoteIncrement == nil || baseMin == nil || baseMax == nil || quoteMin == nil || quoteMax == nil || productMarketIOCEnabled == nil || productObservedAt == nil {
			return Intent{}, nil, errors.New("incomplete stored product rules")
		}
		productRules := &financial.SpotProductRules{
			Provider: "coinbase", Feed: *productFeed, ProductID: intent.ProductID, ProductType: *productType, BaseAsset: intent.BaseAsset, QuoteCurrency: intent.QuoteCurrency,
			BaseIncrement: financial.Decimal(canonicalStoredDecimal(*baseIncrement)), QuoteIncrement: financial.Decimal(canonicalStoredDecimal(*quoteIncrement)),
			BaseMinSize: financial.Decimal(canonicalStoredDecimal(*baseMin)), BaseMaxSize: financial.Decimal(canonicalStoredDecimal(*baseMax)),
			QuoteMinSize: financial.Decimal(canonicalStoredDecimal(*quoteMin)), QuoteMaxSize: financial.Decimal(canonicalStoredDecimal(*quoteMax)),
			Status: *productStatus, MarketIOCEnabled: *productMarketIOCEnabled, ObservedAt: *productObservedAt,
		}
		if err = json.Unmarshal(productBlockReasonsJSON, &productRules.BlockReasons); err != nil {
			return Intent{}, nil, err
		}
		intent.Preview.ProductRules = productRules
	}
	if policyVersion != nil {
		if riskEvaluationID == nil || riskCapitalBucketID == nil || riskBucketName == nil || riskAllocationType == nil || riskAllocationValue == nil || riskProtectedAmount == nil || riskAvailableCash == nil || riskAccountReservedCash == nil || riskBucketReservedCash == nil || riskTargetQuantity == nil || riskTargetReservedQuantity == nil || riskProposedNotional == nil || riskObservedAt == nil || riskDecision == nil || riskApprovalRequired == nil || riskPlatformExecution == nil {
			return Intent{}, nil, errors.New("incomplete stored manual risk evidence")
		}
		riskEvidence := &ManualRiskEvidence{
			PolicyVersion: *policyVersion, EvaluationID: *riskEvaluationID, CapitalBucketID: *riskCapitalBucketID, CapitalBucketName: *riskBucketName,
			AllocationType: *riskAllocationType, AllocationValue: financial.Decimal(canonicalStoredDecimal(*riskAllocationValue)), ProtectedAmount: financial.Decimal(canonicalStoredDecimal(*riskProtectedAmount)),
			AccountAvailableCash: financial.Money{Amount: financial.Decimal(canonicalStoredDecimal(*riskAvailableCash)), Currency: "USD"},
			AccountReservedCash:  financial.Money{Amount: financial.Decimal(canonicalStoredDecimal(*riskAccountReservedCash)), Currency: "USD"}, BucketReservedCash: financial.Money{Amount: financial.Decimal(canonicalStoredDecimal(*riskBucketReservedCash)), Currency: "USD"},
			TargetAvailableQuantity: financial.Decimal(canonicalStoredDecimal(*riskTargetQuantity)), TargetReservedQuantity: financial.Decimal(canonicalStoredDecimal(*riskTargetReservedQuantity)),
			ProposedNotional: financial.Money{Amount: financial.Decimal(canonicalStoredDecimal(*riskProposedNotional)), Currency: "USD"}, Decision: risk.Decision(*riskDecision),
			ApprovalRequired: *riskApprovalRequired, PlatformExecution: *riskPlatformExecution, ObservedAt: *riskObservedAt,
		}
		if riskAllocationLimit != nil {
			value := financial.Decimal(canonicalStoredDecimal(*riskAllocationLimit))
			riskEvidence.AllocationLimit = &value
		}
		if err = json.Unmarshal(riskReasonsJSON, &riskEvidence.ReasonCodes); err != nil {
			return Intent{}, nil, err
		}
		if err = json.Unmarshal(riskWarningsJSON, &riskEvidence.Warnings); err != nil {
			return Intent{}, nil, err
		}
		if err = json.Unmarshal(riskChecksJSON, &riskEvidence.Checks); err != nil {
			return Intent{}, nil, err
		}
		intent.Risk = riskEvidence
	}
	if reservationResourceType != nil {
		if reservationAsset == nil || reservationQuantity == nil || reservationReservedAt == nil || reservationExpiresAt == nil {
			return Intent{}, nil, errors.New("incomplete stored capital reservation")
		}
		intent.CapitalReservation = &CapitalReservation{ResourceType: *reservationResourceType, Asset: *reservationAsset, Quantity: financial.Decimal(canonicalStoredDecimal(*reservationQuantity)), ReservedAt: *reservationReservedAt, ExpiresAt: *reservationExpiresAt}
	}
	intent.ReviewScope = ProposalReviewOnly
	intent.SubmissionAvailable = false
	intent.RiskApprovalAvailable = false
	intent.AIExecutionAuthority = false
	intent.LiveExecutionAvailable = false
	return intent, bytes.Clone(evidenceHash), nil
}

func selectIntent(suffix string) string {
	return `SELECT ` + intentColumns + ` FROM order_intents i JOIN order_intent_previews p ON p.order_intent_id=i.id AND p.intent_version=1 LEFT JOIN order_intent_risk_evaluations l ON l.order_intent_id=i.id LEFT JOIN risk_evaluations r ON r.id=l.risk_evaluation_id LEFT JOIN capital_reservations c ON c.order_intent_id=i.id ` + suffix
}

const activeReservationsSQL = `SELECT
  COALESCE(sum(quantity) FILTER (WHERE resource_type='CASH' AND resource_asset='USD'),0)::text,
  COALESCE(sum(quantity) FILTER (WHERE resource_type='CASH' AND resource_asset='USD' AND capital_bucket_id=$3),0)::text,
  COALESCE(sum(quantity) FILTER (WHERE resource_type='ASSET' AND resource_asset=$4),0)::text
FROM capital_reservations
WHERE user_id=$1 AND financial_account_id=$2 AND expires_at>$5`

func scanReservationSnapshot(row pgx.Row, observedAt time.Time) (ReservationSnapshot, error) {
	var accountCash, bucketCash, targetQuantity string
	if err := row.Scan(&accountCash, &bucketCash, &targetQuantity); err != nil {
		return ReservationSnapshot{}, err
	}
	return ReservationSnapshot{
		AccountReservedCash: financial.Decimal(canonicalStoredDecimal(accountCash)), BucketReservedCash: financial.Decimal(canonicalStoredDecimal(bucketCash)),
		TargetReservedQuantity: financial.Decimal(canonicalStoredDecimal(targetQuantity)), ObservedAt: observedAt,
	}, nil
}

func (store *PostgresStore) ActiveReservations(ctx context.Context, userID, accountID, bucketID, symbol string, observedAt time.Time) (ReservationSnapshot, error) {
	return scanReservationSnapshot(store.db.QueryRow(ctx, activeReservationsSQL, userID, accountID, bucketID, symbol, observedAt), observedAt)
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
	if input.Risk == nil {
		return Intent{}, nil, ErrUnsafeRiskEvidence
	}
	existing, requestHash, err := store.ByIdempotency(ctx, input.UserID, input.IdempotencyKey)
	if err != nil {
		return Intent{}, nil, err
	}
	if existing != nil {
		if !bytes.Equal(requestHash, input.RequestHash) {
			return Intent{}, nil, ErrIdempotencyConflict
		}
		return *existing, requestHash, nil
	}
	transaction, err := store.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Intent{}, nil, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	if _, err = transaction.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text || ':' || $2::text || ':manual-order-reservations',0))`, input.UserID, input.FinancialAccountID); err != nil {
		return Intent{}, nil, err
	}
	currentReservations, err := scanReservationSnapshot(transaction.QueryRow(ctx, activeReservationsSQL, input.UserID, input.FinancialAccountID, input.CapitalBucketID, input.BaseAsset, input.Risk.ObservedAt), input.Risk.ObservedAt)
	if err != nil {
		return Intent{}, nil, err
	}
	if currentReservations.AccountReservedCash != input.Risk.AccountReservedCash.Amount || currentReservations.BucketReservedCash != input.Risk.BucketReservedCash.Amount || currentReservations.TargetReservedQuantity != input.Risk.TargetReservedQuantity {
		return Intent{}, nil, ErrReservationConflict
	}
	if (input.Status == ReviewRequired) != (input.CapitalReservation != nil) {
		return Intent{}, nil, ErrUnsafeRiskEvidence
	}
	var intentID string
	err = transaction.QueryRow(ctx, `INSERT INTO order_intents(user_id,financial_account_id,capital_bucket_id,source,idempotency_key,request_hash,provider_name,product_id,base_asset,quote_currency,side,order_type,requested_size,requested_size_currency,status,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,'coinbase',$7,$8,'USD',$9,'MARKET_IOC',$10,$11,$12,1,$13,$13) RETURNING id::text`, input.UserID, input.FinancialAccountID, input.CapitalBucketID, input.Source, input.IdempotencyKey, input.RequestHash, input.ProductID, input.BaseAsset, input.Side, input.RequestedSize.Amount, input.RequestedSize.Currency, input.Status, input.CreatedAt).Scan(&intentID)
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
	if input.Preview.ProductRules == nil {
		return Intent{}, nil, ErrUnsafeProviderEvidence
	}
	productBlocks, err := json.Marshal(input.Preview.ProductRules.BlockReasons)
	if err != nil {
		return Intent{}, nil, err
	}
	_, err = transaction.Exec(ctx, `INSERT INTO order_intent_previews(order_intent_id,intent_version,provider_name,feed,preview_state,base_size,quote_size,order_total,commission_total,best_bid,best_ask,estimated_average_filled_price,slippage,provider_trading_authorized,block_reasons,warnings,evidence_hash,previewed_at,expires_at,created_at,product_rules_feed,product_type,product_status,base_increment,quote_increment,base_min_size,base_max_size,quote_min_size,quote_max_size,product_market_ioc_enabled,product_block_reasons,product_rules_observed_at) VALUES($1,1,'coinbase','advanced_trade_order_preview',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,'advanced_trade_product','SPOT',$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)`, intentID, input.Preview.PreviewState, input.Preview.BaseSize, input.Preview.QuoteSize, input.Preview.OrderTotal.Amount, input.Preview.CommissionTotal.Amount, moneyAmount(input.Preview.BestBid), moneyAmount(input.Preview.BestAsk), moneyAmount(input.Preview.EstimatedAverageFilledPrice), decimalValue(input.Preview.Slippage), input.Preview.ProviderTradingAuthorized, blocks, warnings, input.EvidenceHash, input.Preview.PreviewedAt, input.Preview.ExpiresAt, input.CreatedAt, input.Preview.ProductRules.Status, input.Preview.ProductRules.BaseIncrement, input.Preview.ProductRules.QuoteIncrement, input.Preview.ProductRules.BaseMinSize, input.Preview.ProductRules.BaseMaxSize, input.Preview.ProductRules.QuoteMinSize, input.Preview.ProductRules.QuoteMaxSize, input.Preview.ProductRules.MarketIOCEnabled, productBlocks, input.Preview.ProductRules.ObservedAt)
	if err != nil {
		return Intent{}, nil, err
	}
	riskReasons, err := json.Marshal(input.Risk.ReasonCodes)
	if err != nil {
		return Intent{}, nil, err
	}
	riskWarnings, err := json.Marshal(input.Risk.Warnings)
	if err != nil {
		return Intent{}, nil, err
	}
	riskChecks, err := json.Marshal(input.Risk.Checks)
	if err != nil {
		return Intent{}, nil, err
	}
	_, err = transaction.Exec(ctx, `INSERT INTO risk_evaluations(id,user_id,financial_account_id,proposed_action_id,correlation_id,decision,approval_required,execution_mode,platform_execution_available,reason_codes,warnings,checks,evaluated_at) VALUES($1,$2,$3,$4,$5,$6,$7,'MANUAL_PROPOSAL',false,$8,$9,$10,$11)`, input.Risk.EvaluationID, input.UserID, input.FinancialAccountID, intentID, input.IdempotencyKey, input.Risk.Decision, input.Risk.ApprovalRequired, riskReasons, riskWarnings, riskChecks, input.Risk.ObservedAt)
	if err != nil {
		return Intent{}, nil, err
	}
	_, err = transaction.Exec(ctx, `INSERT INTO order_intent_risk_evaluations(order_intent_id,user_id,financial_account_id,capital_bucket_id,risk_evaluation_id,policy_version,capital_bucket_name,allocation_type,allocation_value,protected_amount,allocation_limit,account_available_cash,account_reserved_cash,bucket_reserved_cash,target_available_quantity,target_reserved_quantity,proposed_notional,observed_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, intentID, input.UserID, input.FinancialAccountID, input.CapitalBucketID, input.Risk.EvaluationID, input.Risk.PolicyVersion, input.Risk.CapitalBucketName, input.Risk.AllocationType, input.Risk.AllocationValue, input.Risk.ProtectedAmount, decimalPointer(input.Risk.AllocationLimit), input.Risk.AccountAvailableCash.Amount, input.Risk.AccountReservedCash.Amount, input.Risk.BucketReservedCash.Amount, input.Risk.TargetAvailableQuantity, input.Risk.TargetReservedQuantity, input.Risk.ProposedNotional.Amount, input.Risk.ObservedAt, input.CreatedAt)
	if err != nil {
		return Intent{}, nil, err
	}
	if input.CapitalReservation != nil {
		_, err = transaction.Exec(ctx, `INSERT INTO capital_reservations(user_id,financial_account_id,capital_bucket_id,order_intent_id,source_type,resource_type,resource_asset,quantity,reserved_at,expires_at,created_at) VALUES($1,$2,$3,$4,'ORDER_INTENT',$5,$6,$7,$8,$9,$10)`, input.UserID, input.FinancialAccountID, input.CapitalBucketID, intentID, input.CapitalReservation.ResourceType, input.CapitalReservation.Asset, input.CapitalReservation.Quantity, input.CapitalReservation.ReservedAt, input.CapitalReservation.ExpiresAt, input.CreatedAt)
		if err != nil {
			var postgresError *pgconn.PgError
			if errors.As(err, &postgresError) && strings.Contains(postgresError.Message, "capital reservation snapshot") {
				return Intent{}, nil, ErrReservationConflict
			}
			return Intent{}, nil, err
		}
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

func (store *PostgresStore) ManualPolicy(ctx context.Context, userID, accountID, bucketID string) (risk.CapitalBucket, []risk.CircuitBreaker, error) {
	var bucket risk.CapitalBucket
	var allocationLimit *string
	err := store.db.QueryRow(ctx, `SELECT id::text,user_id::text,financial_account_id::text,name,allocation_type,allocation_value::text,currency,is_reserve,protected_amount::text,allocation_limit::text,status FROM capital_buckets WHERE id=$1 AND user_id=$2 AND financial_account_id=$3`, bucketID, userID, accountID).Scan(
		&bucket.ID, &bucket.UserID, &bucket.AccountID, &bucket.Name, &bucket.AllocationType, &bucket.AllocationValue, &bucket.Currency, &bucket.IsReserve, &bucket.ProtectedAmount, &allocationLimit, &bucket.Status,
	)
	if err != nil {
		return risk.CapitalBucket{}, nil, err
	}
	bucket.AllocationLimit = allocationLimit
	rows, err := store.db.Query(ctx, `SELECT id::text,scope,scope_id::text,state,reason,source,engaged_by_user_id::text,engaged_at,released_by_user_id::text,released_at FROM risk_circuit_breakers WHERE state='OPEN' AND (scope='GLOBAL' OR (scope='USER' AND scope_id=$1) OR (scope='ACCOUNT' AND scope_id=$2)) ORDER BY engaged_at,id`, userID, accountID)
	if err != nil {
		return risk.CapitalBucket{}, nil, err
	}
	defer rows.Close()
	breakers := make([]risk.CircuitBreaker, 0, 3)
	for rows.Next() {
		var breaker risk.CircuitBreaker
		if err = rows.Scan(&breaker.ID, &breaker.Scope, &breaker.ScopeID, &breaker.State, &breaker.Reason, &breaker.Source, &breaker.EngagedByUserID, &breaker.EngagedAt, &breaker.ReleasedByUserID, &breaker.ReleasedAt); err != nil {
			return risk.CapitalBucket{}, nil, err
		}
		breakers = append(breakers, breaker)
	}
	return bucket, breakers, rows.Err()
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

func decimalPointer(value *financial.Decimal) any {
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
