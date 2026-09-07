package financialconnection

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

var ErrAuthorizationReceiptUnavailable = errors.New("financial authorization receipt history is unavailable")

const authorizationReceiptLimit = 20

var authorizationReceiptProviderPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

type AuthorizationReceipt struct {
	ID                                   string     `json:"id"`
	Provider                             string     `json:"provider"`
	Status                               string     `json:"status"`
	ConnectionID                         string     `json:"connection_id,omitempty"`
	AuthorizationExpiresAt               *time.Time `json:"authorization_expires_at,omitempty"`
	CurrentLastVerifiedAt                *time.Time `json:"current_last_verified_at,omitempty"`
	AuthorizationExpiryMatchesConnection *bool      `json:"authorization_expiry_matches_current_connection,omitempty"`
	OccurredAt                           time.Time  `json:"occurred_at"`
}

type AuthorizationReceiptStore interface {
	ListAuthorizationReceipts(context.Context, string, int) ([]AuthorizationReceipt, error)
}

func (s *Service) AuthorizationReceipts(ctx context.Context, principal authorization.Principal) ([]AuthorizationReceipt, error) {
	if !allowed(principal) {
		return nil, ErrForbidden
	}
	if s.authorizationReceipts == nil {
		return nil, ErrAuthorizationReceiptUnavailable
	}
	receipts, err := s.authorizationReceipts.ListAuthorizationReceipts(ctx, principal.UserID, authorizationReceiptLimit)
	if err != nil {
		return nil, ErrAuthorizationReceiptUnavailable
	}
	if len(receipts) > authorizationReceiptLimit {
		return nil, ErrAuthorizationReceiptUnavailable
	}
	seenIDs := make(map[string]struct{}, len(receipts))
	for _, receipt := range receipts {
		if strings.TrimSpace(receipt.ID) == "" ||
			!authorizationReceiptProviderPattern.MatchString(receipt.Provider) ||
			(receipt.Status != "STARTED" && receipt.Status != "COMPLETED" && receipt.Status != "FAILED") ||
			receipt.OccurredAt.IsZero() ||
			(receipt.Status == "COMPLETED" && strings.TrimSpace(receipt.ConnectionID) == "") ||
			(receipt.Status == "COMPLETED" && (receipt.CurrentLastVerifiedAt == nil || receipt.CurrentLastVerifiedAt.IsZero() || receipt.AuthorizationExpiryMatchesConnection == nil)) ||
			(receipt.Status != "COMPLETED" && (receipt.CurrentLastVerifiedAt != nil || receipt.AuthorizationExpiryMatchesConnection != nil)) ||
			(receipt.AuthorizationExpiresAt != nil && !receipt.AuthorizationExpiresAt.After(receipt.OccurredAt)) {
			return nil, ErrAuthorizationReceiptUnavailable
		}
		if _, exists := seenIDs[receipt.ID]; exists {
			return nil, ErrAuthorizationReceiptUnavailable
		}
		seenIDs[receipt.ID] = struct{}{}
	}
	return receipts, nil
}
