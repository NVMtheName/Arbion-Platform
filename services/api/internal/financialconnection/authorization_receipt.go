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
var authorizationReceiptAttemptPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var authorizationReceiptUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

type AuthorizationReceipt struct {
	ID                                   string     `json:"id"`
	AttemptID                            string     `json:"attempt_id,omitempty"`
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
	attempts := make(map[string][]AuthorizationReceipt)
	observedAt := time.Now().UTC()
	for _, receipt := range receipts {
		if !authorizationReceiptUUIDPattern.MatchString(receipt.ID) ||
			(receipt.AttemptID != "" && !authorizationReceiptAttemptPattern.MatchString(receipt.AttemptID)) ||
			!authorizationReceiptProviderPattern.MatchString(receipt.Provider) ||
			(receipt.Status != "STARTED" && receipt.Status != "COMPLETED" && receipt.Status != "FAILED") ||
			receipt.OccurredAt.IsZero() || receipt.OccurredAt.After(observedAt) ||
			(receipt.ConnectionID != "" && !authorizationReceiptUUIDPattern.MatchString(receipt.ConnectionID)) ||
			(receipt.Status == "COMPLETED" && strings.TrimSpace(receipt.ConnectionID) == "") ||
			(receipt.Status == "COMPLETED" && (receipt.CurrentLastVerifiedAt == nil || receipt.CurrentLastVerifiedAt.IsZero() || receipt.AuthorizationExpiryMatchesConnection == nil)) ||
			(receipt.CurrentLastVerifiedAt != nil && receipt.CurrentLastVerifiedAt.After(observedAt)) ||
			(receipt.Status != "COMPLETED" && (receipt.AuthorizationExpiresAt != nil || receipt.CurrentLastVerifiedAt != nil || receipt.AuthorizationExpiryMatchesConnection != nil)) ||
			(receipt.AuthorizationExpiresAt != nil && !receipt.AuthorizationExpiresAt.After(receipt.OccurredAt)) {
			return nil, ErrAuthorizationReceiptUnavailable
		}
		if _, exists := seenIDs[receipt.ID]; exists {
			return nil, ErrAuthorizationReceiptUnavailable
		}
		seenIDs[receipt.ID] = struct{}{}
		if receipt.AttemptID != "" {
			attempts[receipt.AttemptID] = append(attempts[receipt.AttemptID], receipt)
		}
	}
	for _, attempt := range attempts {
		if len(attempt) > 2 {
			return nil, ErrAuthorizationReceiptUnavailable
		}
		provider := attempt[0].Provider
		connectionID := ""
		var startedAt *time.Time
		var terminalAt *time.Time
		for _, receipt := range attempt {
			if receipt.Provider != provider ||
				(connectionID != "" && receipt.ConnectionID != "" && receipt.ConnectionID != connectionID) {
				return nil, ErrAuthorizationReceiptUnavailable
			}
			if receipt.ConnectionID != "" {
				connectionID = receipt.ConnectionID
			}
			switch receipt.Status {
			case "STARTED":
				if startedAt != nil {
					return nil, ErrAuthorizationReceiptUnavailable
				}
				value := receipt.OccurredAt
				startedAt = &value
			case "COMPLETED", "FAILED":
				if terminalAt != nil {
					return nil, ErrAuthorizationReceiptUnavailable
				}
				value := receipt.OccurredAt
				terminalAt = &value
			}
		}
		if startedAt != nil && terminalAt != nil && !terminalAt.After(*startedAt) {
			return nil, ErrAuthorizationReceiptUnavailable
		}
	}
	return receipts, nil
}
