package execution

import (
	"context"
	"math/big"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

// AccountSettlementEvidence is private provider evidence, never a command from
// a browser/model. Complete zero-hold account reads and empty unresolved-order
// scans must agree around a verified final order. This is observational evidence,
// not a claim of an atomic broker snapshot or permission for the next order.
type AccountSettlementEvidence struct {
	PortfolioID                                         string
	Observation                                         BrokerObservation
	CashUSD, AvailableCashUSD, TotalBase, AvailableBase string
	Complete, NoOpenOrders                              bool
	StartedAt, CompletedAt                              time.Time
}

type AccountSettlementProvider interface {
	CollectAccountSettlement(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (AccountSettlementEvidence, error)
}

// AccountSettlement is an immutable per-order accounting receipt. Opening
// amounts are the original pinned pre-send funding; closing amounts are verified
// against broker reads and saved fills. It never writes broker holdings or
// substitutes for fresh risk/reconciliation/owner authority on a subsequent order.
type AccountSettlement struct {
	BrokerIdentity
	PortfolioID          string
	CredentialGeneration int64
	TerminalStatus       string
	Totals
	OpeningCashUSD, OpeningBase, ClosingCashUSD, ClosingBase string
	StartedAt, ObservedAt, RecordedAt                        time.Time
}

func ValidateAccountSettlementEvidence(s ConfirmedSubmission, a Attempt, e AccountSettlementEvidence, now time.Time) error {
	if e.PortfolioID != s.PortfolioID || !e.Complete || !e.NoOpenOrders || !terminalStatus(e.Observation.Status) ||
		ValidateBrokerObservation(s, a, e.Observation, now) != nil || e.StartedAt.Before(a.ClaimedAt) ||
		e.Observation.StartedAt.Before(e.StartedAt) || e.CompletedAt.Before(e.Observation.ObservedAt) ||
		e.CompletedAt.After(now) || e.CompletedAt.Sub(e.StartedAt) > 15*time.Second || now.Sub(e.CompletedAt) > 30*time.Second {
		return ErrUnreconciled
	}
	cash, cok := decimal(e.CashUSD, false)
	base, bok := decimal(e.TotalBase, false)
	availableCash, acok := decimal(e.AvailableCashUSD, false)
	availableBase, abok := decimal(e.AvailableBase, false)
	if !cok || !bok || !acok || !abok || cash.Cmp(availableCash) != 0 || base.Cmp(availableBase) != 0 {
		return ErrUnreconciled
	}
	openingCash, _ := decimal(s.Preflight.CashUSD, false)
	openingBase, _ := decimal(s.Preflight.TotalBase, false)
	qty, _ := amount(e.Observation.BaseQuantity)
	gross, _ := amount(e.Observation.GrossUSD)
	fee, _ := amount(e.Observation.FeeUSD)
	expectedCash, expectedBase := new(big.Rat).Set(openingCash), new(big.Rat).Set(openingBase)
	if s.Order.Request.Side == "BUY" {
		expectedCash.Sub(expectedCash, gross)
		expectedBase.Add(expectedBase, qty)
	} else {
		expectedCash.Add(expectedCash, gross)
		expectedBase.Sub(expectedBase, qty)
	}
	expectedCash.Sub(expectedCash, fee)
	if expectedCash.Sign() < 0 || expectedBase.Sign() < 0 || cash.Cmp(expectedCash) != 0 || base.Cmp(expectedBase) != 0 {
		return ErrUnreconciled
	}
	return nil
}
