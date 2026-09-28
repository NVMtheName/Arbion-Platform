package execution

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Database interface {
	Begin(context.Context) (pgx.Tx, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type PostgresStore struct{ db Database }

func NewPostgresStore(db Database) *PostgresStore { return &PostgresStore{db: db} }

// Prepare saves immutable terms, not a live approval or a capital reservation.
func (s *PostgresStore) Prepare(ctx context.Context, r Request) (Order, error) {
	digest, err := requestDigest(r)
	if err != nil {
		return Order{}, err
	}
	body, _ := json.Marshal(r)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Order{}, err
	}
	defer rollback(tx)
	_, err = tx.Exec(ctx, `INSERT INTO execution_orders
		(owner_id,financial_account_id,provider_connection_id,capital_bucket_id,client_order_id,request_digest,request)
		VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(client_order_id) DO NOTHING`,
		r.OwnerID, r.AccountID, r.ConnectionID, r.CapitalBucketID, r.ClientOrderID, digest, body)
	if err != nil {
		return Order{}, mapError(err)
	}
	order, err := readOrder(ctx, tx, r.OwnerID, "client_order_id", r.ClientOrderID)
	if errors.Is(err, ErrNotFound) {
		return Order{}, ErrConflict
	}
	if err != nil {
		return Order{}, err
	}
	if order.RequestDigest != digest {
		return Order{}, ErrConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return Order{}, ErrCommitUnknown
	}
	return order, nil
}

// Claim commits at most one attempt per order and one unresolved account hold.
// Only the winning call gets an Attempt with nil error. On ANY error the caller
// must not send. A commit error may mean the attempt exists: recover by ReadAttempt.
// Only exact terminal reconciliation releases the account slot. An order's
// attempt itself is permanent and cannot be claimed again after settlement.
func (s *PostgresStore) Claim(ctx context.Context, ownerID, orderID string, authority Authority) (Attempt, error) {
	if authority == nil {
		return Attempt{}, ErrNotAuthorized
	}
	if !validUUID(ownerID) || !validUUID(orderID) {
		return Attempt{}, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Attempt{}, err
	}
	defer rollback(tx)
	order, err := readOrder(ctx, tx, ownerID, "id", orderID)
	if err != nil {
		return Attempt{}, err
	}
	// Serialize competing orders on the same real account. The authority must
	// acquire/recheck its remaining controls under this transaction too.
	var account string
	err = tx.QueryRow(ctx, `SELECT id::text FROM financial_accounts WHERE id=$1 AND user_id=$2 FOR UPDATE`, order.Request.AccountID, ownerID).Scan(&account)
	if err != nil {
		return Attempt{}, mapError(err)
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_dispatch_attempts WHERE order_id=$1)`, orderID).Scan(&exists); err != nil {
		return Attempt{}, err
	}
	if exists {
		return Attempt{}, ErrAlreadyAttempted
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_reconciliation_blocks WHERE financial_account_id=$1)`, account).Scan(&exists); err != nil {
		return Attempt{}, err
	}
	if exists {
		return Attempt{}, ErrReconciliationBlocked
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_account_holds WHERE financial_account_id=$1)`, account).Scan(&exists); err != nil {
		return Attempt{}, err
	}
	if exists {
		return Attempt{}, ErrAccountHeld
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return Attempt{}, err
	}
	approval, err := authority.AuthorizeDispatch(ctx, tx, order, now)
	if err != nil {
		return Attempt{}, err
	}
	// Use wall time after authority/lock waits, never transaction start time.
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return Attempt{}, err
	}
	if !validUUID(approval.ID) || approval.RequestDigest != order.RequestDigest || approval.CredentialGeneration <= 0 ||
		!approval.ExpiresAt.After(now) || approval.ExpiresAt.After(now.Add(time.Minute)) {
		return Attempt{}, ErrNotAuthorized
	}
	a := Attempt{OrderID: orderID, ClientOrderID: order.Request.ClientOrderID, RequestDigest: order.RequestDigest,
		AuthorizationID: approval.ID, CredentialGeneration: approval.CredentialGeneration, ClaimedAt: now, ExpiresAt: approval.ExpiresAt}
	_, err = tx.Exec(ctx, `INSERT INTO execution_dispatch_attempts(order_id,owner_id,financial_account_id,authorization_id,credential_generation,claimed_at,expires_at)
		VALUES($1,$2,$3,$4,$5,$6,$7)`, orderID, ownerID, account, approval.ID, approval.CredentialGeneration, now, approval.ExpiresAt)
	if err != nil {
		return Attempt{}, mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Attempt{}, ErrCommitUnknown
	}
	return a, nil
}

// RecordAcknowledgement stores provider correlation, NOT a fill or settlement.
// Exact replay succeeds; a changed broker order identity is a conflict.
func (s *PostgresStore) RecordAcknowledgement(ctx context.Context, ownerID, orderID, providerOrderID string) error {
	if !validUUID(ownerID) || !validUUID(orderID) || !validUUID(providerOrderID) {
		return ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	var id string
	err = tx.QueryRow(ctx, `SELECT order_id::text FROM execution_dispatch_attempts WHERE order_id=$1 AND owner_id=$2 FOR UPDATE`, orderID, ownerID).Scan(&id)
	if err != nil {
		return mapError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO execution_broker_acknowledgements(order_id,owner_id,provider_order_id)
		VALUES($1,$2,$3) ON CONFLICT(order_id) DO NOTHING`, orderID, ownerID, providerOrderID)
	if err != nil {
		return mapError(err)
	}
	var saved string
	err = tx.QueryRow(ctx, `SELECT provider_order_id::text FROM execution_broker_acknowledgements WHERE order_id=$1 AND owner_id=$2`, orderID, ownerID).Scan(&saved)
	if err != nil {
		return mapError(err)
	}
	if saved != providerOrderID {
		return ErrConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return ErrCommitUnknown
	}
	return nil
}

// ReadAttempt is recovery-only: returning a record never grants a send claim.
// Empty ProviderOrderID means unresolved, not permission to retry submission.
func (s *PostgresStore) ReadAttempt(ctx context.Context, ownerID, orderID string) (Attempt, error) {
	if !validUUID(ownerID) || !validUUID(orderID) {
		return Attempt{}, ErrInvalid
	}
	var a Attempt
	err := s.db.QueryRow(ctx, `SELECT a.order_id::text,o.client_order_id::text,o.request_digest,a.authorization_id::text,
		a.credential_generation,a.claimed_at,a.expires_at,COALESCE(b.provider_order_id::text,'')
		FROM execution_dispatch_attempts a JOIN execution_orders o ON o.id=a.order_id
		LEFT JOIN execution_broker_acknowledgements b ON b.order_id=a.order_id
		WHERE a.order_id=$1 AND a.owner_id=$2`, orderID, ownerID).Scan(&a.OrderID, &a.ClientOrderID, &a.RequestDigest,
		&a.AuthorizationID, &a.CredentialGeneration, &a.ClaimedAt, &a.ExpiresAt, &a.ProviderOrderID)
	return a, mapError(err)
}

func readOrder(ctx context.Context, tx pgx.Tx, ownerID, column, id string) (Order, error) {
	var o Order
	var body []byte
	// column is a closed internal choice, not external SQL input.
	if column != "id" && column != "client_order_id" {
		return o, ErrInvalid
	}
	err := tx.QueryRow(ctx, `SELECT id::text,request_digest,request,created_at FROM execution_orders WHERE owner_id=$1 AND `+column+`=$2`, ownerID, id).
		Scan(&o.ID, &o.RequestDigest, &body, &o.CreatedAt)
	if err != nil {
		return o, mapError(err)
	}
	if err = json.Unmarshal(body, &o.Request); err != nil {
		return Order{}, ErrInvalid
	}
	digest, err := requestDigest(o.Request)
	if err != nil || digest != o.RequestDigest || o.Request.OwnerID != ownerID {
		return Order{}, ErrInvalid
	}
	return o, nil
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var e *pgconn.PgError
	if errors.As(err, &e) {
		switch e.Code {
		case "23505":
			return ErrConflict
		case "23503", "23514":
			return ErrInvalid
		}
	}
	return err
}
