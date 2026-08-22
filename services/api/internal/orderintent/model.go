// Package orderintent owns durable, non-executing trade proposals and their
// review evidence. It has no provider submission, cancellation, replacement,
// transfer, or reconciliation interface.
package orderintent

import (
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

const (
	SourceUI     = "UI"
	SourceAI     = "AI"
	SourceHybrid = "HYBRID"

	ReviewRequired             = "REVIEW_REQUIRED"
	Blocked                    = "BLOCKED"
	UserApprovedNonExecutable  = "USER_APPROVED_NONEXECUTABLE"
	ProposalReviewOnly         = "PROPOSAL_REVIEW_ONLY"
	previewEvidenceLifetime    = time.Minute
	maxOrderIntentsPerResponse = 50
)

type CreateCommand struct {
	Symbol         string            `json:"symbol"`
	Side           string            `json:"side"`
	Size           financial.Decimal `json:"size"`
	IdempotencyKey string            `json:"idempotency_key"`
}

type ReviewCommand struct {
	ExpectedVersion int64  `json:"expected_version"`
	MFACode         string `json:"mfa_code"`
}

type PreviewEvidence struct {
	Provider                    string             `json:"provider"`
	Feed                        string             `json:"feed"`
	PreviewState                string             `json:"preview_state"`
	BaseSize                    financial.Decimal  `json:"base_size"`
	QuoteSize                   financial.Decimal  `json:"quote_size"`
	OrderTotal                  financial.Money    `json:"order_total"`
	CommissionTotal             financial.Money    `json:"commission_total"`
	BestBid                     *financial.Money   `json:"best_bid,omitempty"`
	BestAsk                     *financial.Money   `json:"best_ask,omitempty"`
	EstimatedAverageFilledPrice *financial.Money   `json:"estimated_average_filled_price,omitempty"`
	Slippage                    *financial.Decimal `json:"slippage,omitempty"`
	ProviderTradingAuthorized   bool               `json:"provider_trading_authorized"`
	BlockReasons                []string           `json:"block_reasons"`
	Warnings                    []string           `json:"warnings"`
	PreviewedAt                 time.Time          `json:"previewed_at"`
	ExpiresAt                   time.Time          `json:"expires_at"`
}

type Intent struct {
	ID                     string          `json:"id"`
	FinancialAccountID     string          `json:"financial_account_id"`
	Source                 string          `json:"source"`
	Provider               string          `json:"provider"`
	ProductID              string          `json:"product_id"`
	BaseAsset              string          `json:"base_asset"`
	QuoteCurrency          string          `json:"quote_currency"`
	Side                   string          `json:"side"`
	OrderType              string          `json:"order_type"`
	RequestedSize          financial.Money `json:"requested_size"`
	Status                 string          `json:"status"`
	Version                int64           `json:"version"`
	Preview                PreviewEvidence `json:"preview"`
	ReviewScope            string          `json:"review_scope"`
	SubmissionAvailable    bool            `json:"submission_available"`
	RiskApprovalAvailable  bool            `json:"risk_approval_available"`
	AIExecutionAuthority   bool            `json:"ai_execution_authority"`
	LiveExecutionAvailable bool            `json:"live_execution_available"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

type draft struct {
	Intent
	UserID         string
	IdempotencyKey string
	RequestHash    []byte
	EvidenceHash   []byte
}

type reviewEvidence struct {
	IntentID        string
	UserID          string
	ExpectedVersion int64
	EvidenceHash    []byte
	MFAMethod       string
	ReviewedAt      time.Time
}
