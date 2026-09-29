package execution

import (
	"context"
	"math/big"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

// BrokerObservation is a bounded complete per-order scan bracketed by stable
// provider status/totals reads. Cursor exhaustion alone is never completeness.
// These are server-only normalized facts, not browser/model commands.
type BrokerObservation struct {
	BrokerIdentity
	Status        string
	CompleteFills bool
	Totals
	Fills                 []Fill
	StartedAt, ObservedAt time.Time
}

type BrokerObservationProvider interface {
	CollectExecutionObservation(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (BrokerObservation, error)
}

func terminalStatus(status string) bool {
	return status == "FILLED" || status == "CANCELLED" || status == "EXPIRED" || status == "REJECTED"
}

func ValidateBrokerObservation(s ConfirmedSubmission, a Attempt, b BrokerObservation, now time.Time) error {
	if ValidateRecoverySubmission(s, a, now) != nil || !identityMatches(s.Order, a, b.BrokerIdentity) || !b.CompleteFills || b.Fills == nil || len(b.Fills) > 1000 ||
		b.StartedAt.Before(a.ClaimedAt) || b.ObservedAt.Before(b.StartedAt) || b.ObservedAt.After(now) || b.ObservedAt.Sub(b.StartedAt) > 15*time.Second {
		return ErrUnreconciled
	}
	if !terminalStatus(b.Status) {
		switch b.Status {
		case "PENDING", "OPEN", "QUEUED", "CANCEL_QUEUED", "EDIT_QUEUED":
		default:
			return ErrUnreconciled
		}
	}
	q, g, fee := new(big.Rat), new(big.Rat), new(big.Rat)
	trades, entries := map[string]bool{}, map[string]bool{}
	for _, f := range b.Fills {
		n, err := normalizeFill(s.Order, a, f, now)
		if err != nil || n.ProviderEvidence == nil || !f.ObservedAt.Equal(b.ObservedAt) || trades[f.TradeID] || entries[n.ProviderEvidence.EntryID] {
			return ErrUnreconciled
		}
		trades[f.TradeID], entries[n.ProviderEvidence.EntryID] = true, true
		fq, _ := amount(n.BaseQuantity)
		fg, _ := amount(n.GrossUSD)
		ff, _ := amount(n.FeeUSD)
		q.Add(q, fq)
		g.Add(g, fg)
		fee.Add(fee, ff)
	}
	if b.FillCount != int64(len(b.Fills)) || !sameExactAmount(b.BaseQuantity, canonical(q)) || !sameExactAmount(b.GrossUSD, canonical(g)) || !sameExactAmount(b.FeeUSD, canonical(fee)) || !withinBounds(s.Order.Request, b.Totals) {
		return ErrUnreconciled
	}
	if terminalStatus(b.Status) {
		_, err := normalizeTerminal(s.Order, a, b.terminal(), now)
		return err
	}
	return nil
}

func sameExactAmount(a, b string) bool {
	x, xok := amount(a)
	y, yok := amount(b)
	return xok && yok && x.Cmp(y) == 0
}

func (b BrokerObservation) terminal() TerminalReport {
	return TerminalReport{BrokerIdentity: b.BrokerIdentity, Status: b.Status, CompleteFills: b.CompleteFills, FillCount: b.FillCount, BaseQuantity: b.BaseQuantity, GrossUSD: b.GrossUSD, FeeUSD: b.FeeUSD,
		CompletedAt: b.ObservedAt, ObservedAt: b.ObservedAt, CompletionTimeBasis: "OBSERVED_TERMINAL_STATUS"}
}
