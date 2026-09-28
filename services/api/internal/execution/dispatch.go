// Package execution owns the durable submission boundary. It is not wired to
// HTTP, the scheduler, AI, or a broker. Preparing an order grants no authority.
package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalid          = errors.New("invalid execution request")
	ErrNotFound         = errors.New("execution order not found")
	ErrConflict         = errors.New("execution identity conflict")
	ErrNotAuthorized    = errors.New("current execution authorization unavailable")
	ErrAlreadyAttempted = errors.New("submission already claimed; reconcile, never resend")
	ErrAccountHeld      = errors.New("account has an unresolved submission")
	ErrCapitalHeld      = errors.New("account capital remains reserved pending reconciliation")
	ErrCommitUnknown    = errors.New("execution commit outcome unknown; recover, never dispatch")
	uuidPattern         = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	productPattern      = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,15}-USD$`)
	decimalPattern      = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$`)
)

// Request is a new execution request, never an existing non-executable preview
// review. Only LIMIT_IOC spot orders are representable. Amounts are exact, and
// MaximumDebitUSD includes the fee allowance, not only the order notional.
// Product increments, available capital, and live approval are checked by the
// future control-plane Authority, not inferred from these syntactic checks.
type Request struct {
	OwnerID         string
	AccountID       string
	ConnectionID    string
	CapitalBucketID string
	ClientOrderID   string
	ProductID       string
	Side            string
	BaseSize        string
	LimitPrice      string
	FeeAllowanceUSD string
	MaximumDebitUSD string
}

type Order struct {
	ID            string
	Request       Request
	RequestDigest string
	CreatedAt     time.Time
}

// Authorization must originate in the control plane, not a model, preview
// review, or caller-supplied boolean. This package has no production Authority.
type Authorization struct {
	ID                   string
	RequestDigest        string
	CredentialGeneration int64
	ExpiresAt            time.Time
}

// Claim holds current owner, entitlement, account, connection, bucket and scoped
// breaker controls before invoking Authority. Authority must additionally lock
// and verify exact live approval, available cash/inventory, credential scope,
// risk, product/quote evidence and any mandate-specific controls. The persistent
// reservation created by Claim is a hold, not proof that capital was available.
// Denial aborts the claim. Implementations must not perform any broker write.
// A later sender must also serialize revocation through the send boundary;
// a committed attempt is NOT sufficient authority to send indefinitely.
type Authority interface {
	AuthorizeDispatch(context.Context, pgx.Tx, Order, time.Time) (Authorization, error)
}

// Attempt is committed before any future network send. An attempt without an
// acknowledgement is unknown, including a crash before sending. Recovery never
// yields a second dispatch claim. These records are private server data.
type Attempt struct {
	OrderID              string
	ClientOrderID        string
	RequestDigest        string
	AuthorizationID      string
	CredentialGeneration int64
	ClaimedAt            time.Time
	ExpiresAt            time.Time
	ProviderOrderID      string
}

func validUUID(s string) bool {
	return uuidPattern.MatchString(s) && s != "00000000-0000-0000-0000-000000000000"
}

func decimal(s string, positive bool) (*big.Rat, bool) {
	if !decimalPattern.MatchString(s) {
		return nil, false
	}
	n, ok := new(big.Rat).SetString(s)
	return n, ok && (!positive || n.Sign() > 0)
}

func requestDigest(r Request) (string, error) {
	for _, id := range []string{r.OwnerID, r.AccountID, r.ConnectionID, r.CapitalBucketID, r.ClientOrderID} {
		if !validUUID(id) {
			return "", ErrInvalid
		}
	}
	if !productPattern.MatchString(r.ProductID) || r.ProductID == "USD-USD" || (r.Side != "BUY" && r.Side != "SELL") {
		return "", ErrInvalid
	}
	quantity, qOK := decimal(r.BaseSize, true)
	price, pOK := decimal(r.LimitPrice, true)
	fee, fOK := decimal(r.FeeAllowanceUSD, false)
	debit, dOK := decimal(r.MaximumDebitUSD, r.Side == "BUY")
	if !qOK || !pOK || !fOK || !dOK {
		return "", ErrInvalid
	}
	if r.Side == "BUY" {
		bound := new(big.Rat).Add(new(big.Rat).Mul(quantity, price), fee)
		if bound.Cmp(debit) > 0 {
			return "", ErrInvalid
		}
	} else if debit.Sign() != 0 {
		return "", ErrInvalid
	}
	// Bind the exact canonical request bytes, including all account identities.
	// Different decimal spellings are conflicting requests, never rewritten.
	b, err := json.Marshal(r)
	if err != nil {
		return "", ErrInvalid
	}
	h := sha256.Sum256(append([]byte("arbion-coinbase-limit-ioc-v1\x00"), b...))
	return hex.EncodeToString(h[:]), nil
}
