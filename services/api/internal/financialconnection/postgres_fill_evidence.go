package financialconnection

import (
	"context"
	"errors"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
)

// CaptureFillEvidence saves only normalized observations from an existing
// owner-authorized, bounded display read. Unique keys make observed fills
// matchable across later reads; no money or strategy state is mutated here.
func (s *PostgresStore) CaptureFillEvidence(ctx context.Context, user, account string, page financial.FillEvidencePage) error {
	if user == "" || account == "" || page.Provider != "coinbase" || page.Feed != "advanced_trade_fills" || page.Fills == nil || len(page.Fills) > 50 || page.ObservedAt.IsZero() || page.ObservedAt.After(time.Now().UTC()) {
		return financial.ErrInvalidFillEvidence
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	var provider, providerAccount string
	err = tx.QueryRow(ctx, `SELECT provider_name,provider_account_id FROM financial_accounts WHERE id=$1 AND user_id=$2 FOR SHARE`, account, user).Scan(&provider, &providerAccount)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	reference, err := financial.CorrelationReference(provider, providerAccount, "account", providerAccount)
	if err != nil || provider != page.Provider || reference != page.AccountReference {
		return financial.ErrInvalidFillEvidence
	}
	seen := map[string]string{}
	inserted, matched := 0, 0
	for _, raw := range page.Fills {
		if raw.AccountReference != page.AccountReference {
			return financial.ErrInvalidFillEvidence
		}
		e, err := financial.NormalizeFillEvidence(raw, page.ObservedAt)
		if err != nil {
			return err
		}
		digest, err := financial.FillEvidenceDigest(e, page.ObservedAt)
		if err != nil {
			return err
		}
		if previous, ok := seen[e.EntryReference]; ok {
			if previous != digest {
				return ErrFillEvidenceConflict
			}
			continue
		}
		seen[e.EntryReference] = digest
		f := e.Fill
		result, err := tx.Exec(ctx, `INSERT INTO financial_fill_observations
		(user_id,financial_account_id,provider_name,entry_reference,trade_reference,order_reference,evidence_digest,
		 product_id,base_asset,quote_currency,side,price,size,size_unit,commission,liquidity,trade_time,sequence_time,observed_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		ON CONFLICT(financial_account_id,entry_reference) DO NOTHING`, user, account, provider, e.EntryReference, e.TradeReference, e.OrderReference, digest, f.ProductID, f.BaseAsset, f.QuoteCurrency, f.Side, string(f.Price), string(f.Size), f.SizeUnit, string(f.Commission.Amount), f.Liquidity, f.TradeTime.Format(time.RFC3339Nano), e.SequenceTime.Format(time.RFC3339Nano), page.ObservedAt)
		if err != nil {
			return err
		}
		if result.RowsAffected() == 1 {
			inserted++
			continue
		}
		var saved string
		err = tx.QueryRow(ctx, `SELECT evidence_digest FROM financial_fill_observations WHERE user_id=$1 AND financial_account_id=$2 AND entry_reference=$3`, user, account, e.EntryReference).Scan(&saved)
		if err != nil {
			return err
		}
		if saved != digest {
			return ErrFillEvidenceConflict
		}
		matched++
	}
	_, err = tx.Exec(ctx, `INSERT INTO financial_fill_capture_receipts(user_id,financial_account_id,provider_name,observed_at,reported_count,unique_count,inserted_count,matched_count,has_more) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, user, account, provider, page.ObservedAt, len(page.Fills), len(seen), inserted, matched, page.HasMore)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
