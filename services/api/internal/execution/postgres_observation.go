package execution

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
)

// ReconcileBrokerOrder is unwired and GET-only. Only an already recovered
// broker identity can be observed. Source collection is outside locks; exact
// credentials/current access are checked again before recording any facts.
// Complete validated fills and an optional terminal are saved atomically on
// success. A proven conflict commits quarantine; neither path frees capital.
func (s *PostgresStore) ReconcileBrokerOrder(ctx context.Context, ownerID, orderID string, vault FinancialCredentialReader, provider BrokerObservationProvider) (Reconciliation, error) {
	if !validUUID(ownerID) || !validUUID(orderID) || vault == nil || provider == nil {
		return Reconciliation{}, ErrNotAuthorized
	}
	sub, a, generation, err := s.loadRecoveryContext(ctx, ownerID, orderID)
	if err != nil {
		return Reconciliation{}, err
	}
	if a.ProviderOrderID == "" {
		return Reconciliation{}, ErrSubmissionUnknown
	}
	raw, materialGeneration, err := vault.RetrieveFinancialVersion(ctx, credential.Locator{ConnectionID: sub.Order.Request.ConnectionID, UserID: ownerID, Class: credential.Financial})
	defer clear(raw)
	if err != nil || materialGeneration != generation || len(raw) > 16384 {
		return Reconciliation{}, ErrNotAuthorized
	}
	var cr financial.Credentials
	if json.Unmarshal(raw, &cr) != nil || cr.PortfolioID != sub.PortfolioID || cr.APIKeyName == "" || cr.APIPrivateKey == "" {
		return Reconciliation{}, ErrNotAuthorized
	}
	defer func() { cr = financial.Credentials{} }()
	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	b, err := provider.CollectExecutionObservation(readCtx, &cr, sub, a)
	if err != nil || readCtx.Err() != nil {
		return Reconciliation{}, ErrUnreconciled
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Reconciliation{}, err
	}
	defer rollback(tx)
	current, err := lockEvidenceAccess(ctx, tx, sub.Order, sub.PortfolioID, true)
	if err != nil || current != materialGeneration {
		return Reconciliation{}, ErrNotAuthorized
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return Reconciliation{}, err
	}
	if err = ValidateBrokerObservation(sub, a, b, now); err != nil {
		return Reconciliation{}, err
	}
	// Validate the entire collection before any write. Sort only our private
	// copy so out-of-order delivery never changes the provider event timestamps.
	fills := append([]Fill(nil), b.Fills...)
	sort.Slice(fills, func(i, j int) bool {
		if fills[i].TradedAt.Equal(fills[j].TradedAt) {
			return fills[i].TradeID < fills[j].TradeID
		}
		return fills[i].TradedAt.Before(fills[j].TradedAt)
	})
	var conflict error
	for _, f := range fills {
		if err = s.recordFill(ctx, tx, f); err != nil {
			if errors.Is(err, ErrReconciliationBlocked) {
				conflict = err
				break
			}
			return Reconciliation{}, err
		}
	}
	if conflict != nil {
		// Quarantine is staged, never committed past the current-access guard.
	} else if terminalStatus(b.Status) {
		if err = s.reconcileTerminal(ctx, tx, b.terminal()); err != nil {
			if !errors.Is(err, ErrReconciliationBlocked) {
				return Reconciliation{}, err
			}
			conflict = err
		}
	} else {
		// A scan older/smaller than saved fills cannot prove a current state.
		totals, _, e := readTotals(ctx, tx, orderID)
		if e != nil {
			return Reconciliation{}, e
		}
		if totals.FillCount != b.FillCount || !sameExactAmount(totals.BaseQuantity, b.BaseQuantity) || !sameExactAmount(totals.GrossUSD, b.GrossUSD) || !sameExactAmount(totals.FeeUSD, b.FeeUSD) {
			return Reconciliation{}, ErrUnreconciled
		}
		var terminal bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_order_terminals WHERE order_id=$1)`, orderID).Scan(&terminal); err != nil {
			return Reconciliation{}, err
		}
		if terminal {
			return Reconciliation{}, ErrUnreconciled
		}
	}
	current, err = lockEvidenceAccess(ctx, tx, sub.Order, sub.PortfolioID, true)
	if err != nil || current != materialGeneration {
		return Reconciliation{}, ErrNotAuthorized
	}
	if err = tx.Commit(ctx); err != nil {
		return Reconciliation{}, ErrCommitUnknown
	}
	if conflict != nil {
		return Reconciliation{}, conflict
	}
	return s.ReadReconciliation(ctx, ownerID, orderID)
}
