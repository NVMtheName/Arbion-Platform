package financialconnection

import (
	"context"
	"errors"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

var ErrFillEvidenceConflict = errors.New("saved fill identity has conflicting evidence")

type FillEvidenceStore interface {
	CaptureFillEvidence(context.Context, string, string, financial.FillEvidencePage) error
}

// captureFillPage enriches an already-authorized display read with private
// evidence. It adds no provider read and cannot disconnect an account, alter
// holdings/capital, clear reconciliation, or change any engine execution state.
func (s *Service) captureFillPage(ctx context.Context, user, account string, page *financial.TradeFillPage) {
	if page.Provider != "coinbase" {
		return
	}
	page.EvidenceCaptureStatus = "UNAVAILABLE"
	store, ok := s.store.(FillEvidenceStore)
	if !ok || page.Evidence == nil {
		return
	}
	captureCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	err := store.CaptureFillEvidence(captureCtx, user, account, *page.Evidence)
	if err == nil {
		page.EvidenceCaptureStatus = "SAVED"
		return
	}
	if errors.Is(err, ErrFillEvidenceConflict) {
		page.EvidenceCaptureStatus = "CONFLICT"
	}
	// Raw provider IDs, financial values and error bodies never enter audit.
	s.record(ctx, user, "financial.fill_evidence_capture_unavailable", map[string]any{"account_id": account, "code": page.EvidenceCaptureStatus})
}
