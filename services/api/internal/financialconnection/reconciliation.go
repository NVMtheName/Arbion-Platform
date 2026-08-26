package financialconnection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/financial"
)

var (
	ErrReconciliationUnavailable = errors.New("portfolio reconciliation unavailable")
	ErrReconciliationNotFound    = errors.New("portfolio reconciliation not found")
)

const maxReconciliationPositions = 1000

type ReconciliationChange struct {
	Symbol           string            `json:"symbol"`
	InstrumentType   string            `json:"instrument_type"`
	Direction        string            `json:"direction"`
	ChangeType       string            `json:"change_type"`
	PreviousQuantity financial.Decimal `json:"previous_quantity,omitempty"`
	CurrentQuantity  financial.Decimal `json:"current_quantity,omitempty"`
}

type ReconciliationPosition struct {
	Symbol              string             `json:"symbol"`
	InstrumentType      string             `json:"instrument_type"`
	Direction           string             `json:"direction"`
	Quantity            financial.Decimal  `json:"quantity"`
	AvailableQuantity   *financial.Decimal `json:"available_quantity,omitempty"`
	UnavailableQuantity *financial.Decimal `json:"unavailable_to_trade_quantity,omitempty"`
	MarketValue         *financial.Money   `json:"market_value,omitempty"`
	AveragePrice        *financial.Money   `json:"average_price,omitempty"`
	CurrentPrice        *financial.Money   `json:"current_price,omitempty"`
	DayProfitLoss       *financial.Money   `json:"day_profit_loss,omitempty"`
	OpenProfitLoss      *financial.Money   `json:"open_profit_loss,omitempty"`
	PerformanceStatus   string             `json:"performance_status"`
	PriceBasis          string             `json:"price_basis,omitempty"`
}

type PortfolioReconciliation struct {
	ID                        string                   `json:"id"`
	FinancialAccountID        string                   `json:"financial_account_id"`
	Provider                  string                   `json:"provider"`
	ComparisonStatus          string                   `json:"comparison_status"`
	BalancesStatus            string                   `json:"balances_status"`
	PositionsStatus           string                   `json:"positions_status"`
	PerformanceStatus         string                   `json:"performance_status"`
	RealizedPerformanceStatus string                   `json:"realized_performance_status"`
	AutonomySignal            string                   `json:"autonomy_signal"`
	AutonomyEnforcementActive bool                     `json:"autonomy_enforcement_active"`
	BlocksNewActions          bool                     `json:"blocks_new_actions"`
	ObservedPositionCount     int                      `json:"observed_position_count"`
	PerformancePositionCount  int                      `json:"performance_position_count"`
	ChangeCount               int                      `json:"change_count"`
	Balances                  financial.Balances       `json:"balances"`
	PreviousReconciliationID  *string                  `json:"previous_reconciliation_id,omitempty"`
	Changes                   []ReconciliationChange   `json:"changes"`
	Positions                 []ReconciliationPosition `json:"positions"`
	EvidenceHash              string                   `json:"evidence_hash"`
	ObservedAt                time.Time                `json:"observed_at"`
	CreatedAt                 time.Time                `json:"created_at"`
}

type ReconciliationStore interface {
	LatestReconciliation(context.Context, string, string) (PortfolioReconciliation, error)
	LatestReliableReconciliation(context.Context, string, string) (PortfolioReconciliation, error)
	CreateReconciliation(context.Context, string, PortfolioReconciliation, []byte) (PortfolioReconciliation, error)
}

type reconciliationEvidence struct {
	FinancialAccountID        string                   `json:"financial_account_id"`
	Provider                  string                   `json:"provider"`
	ComparisonStatus          string                   `json:"comparison_status"`
	BalancesStatus            string                   `json:"balances_status"`
	PositionsStatus           string                   `json:"positions_status"`
	PerformanceStatus         string                   `json:"performance_status"`
	RealizedPerformanceStatus string                   `json:"realized_performance_status"`
	AutonomySignal            string                   `json:"autonomy_signal"`
	AutonomyEnforcementActive bool                     `json:"autonomy_enforcement_active"`
	BlocksNewActions          bool                     `json:"blocks_new_actions"`
	Balances                  financial.Balances       `json:"balances"`
	PreviousReconciliationID  *string                  `json:"previous_reconciliation_id,omitempty"`
	Changes                   []ReconciliationChange   `json:"changes"`
	Positions                 []ReconciliationPosition `json:"positions"`
	ObservedAt                time.Time                `json:"observed_at"`
}

type reconciliationPositionKey struct {
	Symbol, InstrumentType, Direction string
}

func reconciliationKey(position ReconciliationPosition) reconciliationPositionKey {
	return reconciliationPositionKey{position.Symbol, position.InstrumentType, position.Direction}
}

func normalizePosition(position financial.Position) (ReconciliationPosition, error) {
	normalized := ReconciliationPosition{
		Symbol:              strings.ToUpper(strings.TrimSpace(position.Symbol)),
		InstrumentType:      strings.ToUpper(strings.TrimSpace(position.InstrumentType)),
		Direction:           strings.ToLower(strings.TrimSpace(position.Direction)),
		Quantity:            financial.Decimal(strings.TrimSpace(string(position.Quantity))),
		AvailableQuantity:   normalizedDecimal(position.AvailableQuantity),
		UnavailableQuantity: normalizedDecimal(position.UnavailableToTradeQuantity),
		MarketValue:         normalizedMoney(position.MarketValue),
		AveragePrice:        normalizedMoney(position.CostBasis),
		CurrentPrice:        normalizedMoney(position.CurrentPrice),
		DayProfitLoss:       normalizedMoney(position.DayProfitLoss),
		OpenProfitLoss:      normalizedMoney(position.OpenProfitLoss),
		PriceBasis:          strings.TrimSpace(position.PriceBasis),
		PerformanceStatus:   "UNAVAILABLE",
	}
	if normalized.Symbol == "" || len(normalized.Symbol) > 64 || strings.IndexFunc(normalized.Symbol, unicode.IsControl) >= 0 || normalized.InstrumentType == "" || len(normalized.InstrumentType) > 40 || len(normalized.PriceBasis) > 80 || (normalized.Direction != "long" && normalized.Direction != "short") || !validDecimal(normalized.Quantity) || decimalSign(normalized.Quantity) < 0 {
		return ReconciliationPosition{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	for _, value := range []*financial.Decimal{normalized.AvailableQuantity, normalized.UnavailableQuantity} {
		if value != nil && (!validDecimal(*value) || decimalSign(*value) < 0) {
			return ReconciliationPosition{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
		}
	}
	for _, value := range []*financial.Money{normalized.MarketValue, normalized.AveragePrice, normalized.CurrentPrice, normalized.DayProfitLoss, normalized.OpenProfitLoss} {
		if value != nil && (!validDecimal(value.Amount) || len(strings.TrimSpace(value.Currency)) != 3) {
			return ReconciliationPosition{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
		}
	}
	if normalized.AveragePrice != nil && normalized.CurrentPrice != nil && normalized.OpenProfitLoss != nil {
		normalized.PerformanceStatus = "AVAILABLE"
	}
	return normalized, nil
}

func decimalSign(value financial.Decimal) int {
	parsed, ok := new(big.Rat).SetString(string(value))
	if !ok {
		return -1
	}
	return parsed.Sign()
}

func validDecimal(value financial.Decimal) bool {
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" || len(trimmed) > 80 || strings.ContainsAny(trimmed, "/eE") {
		return false
	}
	unsigned := strings.TrimPrefix(strings.TrimPrefix(trimmed, "+"), "-")
	parts := strings.Split(unsigned, ".")
	if len(parts) > 2 || parts[0] == "" || (len(parts) == 2 && (parts[1] == "" || len(parts[1]) > 18)) {
		return false
	}
	integerDigits := len(strings.TrimLeft(parts[0], "0"))
	for _, part := range parts {
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return integerDigits <= 20
}

func normalizedDecimal(value *financial.Decimal) *financial.Decimal {
	if value == nil {
		return nil
	}
	normalized := financial.Decimal(strings.TrimSpace(string(*value)))
	return &normalized
}

func normalizedMoney(value *financial.Money) *financial.Money {
	if value == nil {
		return nil
	}
	return &financial.Money{Amount: financial.Decimal(strings.TrimSpace(string(value.Amount))), Currency: strings.ToUpper(strings.TrimSpace(value.Currency))}
}

func compareReconciliationPositions(previous, current []ReconciliationPosition) []ReconciliationChange {
	previousByKey := make(map[reconciliationPositionKey]ReconciliationPosition, len(previous))
	currentByKey := make(map[reconciliationPositionKey]ReconciliationPosition, len(current))
	keys := map[reconciliationPositionKey]struct{}{}
	for _, position := range previous {
		key := reconciliationKey(position)
		previousByKey[key] = position
		keys[key] = struct{}{}
	}
	for _, position := range current {
		key := reconciliationKey(position)
		currentByKey[key] = position
		keys[key] = struct{}{}
	}
	ordered := make([]reconciliationPositionKey, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Symbol != ordered[j].Symbol {
			return ordered[i].Symbol < ordered[j].Symbol
		}
		if ordered[i].InstrumentType != ordered[j].InstrumentType {
			return ordered[i].InstrumentType < ordered[j].InstrumentType
		}
		return ordered[i].Direction < ordered[j].Direction
	})
	changes := []ReconciliationChange{}
	for _, key := range ordered {
		before, hadBefore := previousByKey[key]
		after, hasAfter := currentByKey[key]
		if hadBefore && hasAfter && decimalsEqual(before.Quantity, after.Quantity) {
			continue
		}
		change := ReconciliationChange{Symbol: key.Symbol, InstrumentType: key.InstrumentType, Direction: key.Direction, ChangeType: "QUANTITY_CHANGED"}
		if hadBefore {
			change.PreviousQuantity = before.Quantity
		}
		if hasAfter {
			change.CurrentQuantity = after.Quantity
		}
		if !hadBefore {
			change.ChangeType = "POSITION_APPEARED"
		} else if !hasAfter {
			change.ChangeType = "POSITION_DISAPPEARED"
		}
		changes = append(changes, change)
	}
	return changes
}

func decimalsEqual(left, right financial.Decimal) bool {
	l, leftOK := new(big.Rat).SetString(string(left))
	r, rightOK := new(big.Rat).SetString(string(right))
	return leftOK && rightOK && l.Cmp(r) == 0
}

func reconciliationPerformanceStatus(observed, available int) string {
	if observed > 0 && observed == available {
		return "AVAILABLE"
	}
	if available > 0 {
		return "PARTIAL"
	}
	return "UNAVAILABLE"
}

func normalizeReconciliationBalances(balances financial.Balances) (financial.Balances, error) {
	normalized := financial.Balances{
		Cash: normalizedMoney(balances.Cash), AvailableCash: normalizedMoney(balances.AvailableCash),
		BuyingPower: normalizedMoney(balances.BuyingPower), AccountValue: normalizedMoney(balances.AccountValue),
	}
	for _, value := range []*financial.Money{normalized.Cash, normalized.AvailableCash, normalized.BuyingPower, normalized.AccountValue} {
		if value != nil && (!validDecimal(value.Amount) || len(strings.TrimSpace(value.Currency)) != 3) {
			return financial.Balances{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
		}
	}
	return normalized, nil
}

func terminalReconciliationAuthorizationError(err error) bool {
	var providerError *financial.ProviderError
	return errors.As(err, &providerError) && (providerError.Code == financial.AuthorizationFailed || providerError.Code == financial.AuthorizationExpired || providerError.Code == financial.PermissionDenied)
}

func (s *Service) RunReconciliation(ctx context.Context, principal authorization.Principal, accountID string) (PortfolioReconciliation, error) {
	if !allowed(principal) {
		return PortfolioReconciliation{}, ErrForbidden
	}
	if s.reconciliations == nil {
		return PortfolioReconciliation{}, ErrReconciliationUnavailable
	}
	account, err := s.GetAccount(ctx, principal, accountID)
	if err != nil {
		return PortfolioReconciliation{}, err
	}
	if account.Status != "active" {
		return PortfolioReconciliation{}, ErrDisabled
	}
	if account.Provider != "coinbase" && account.Provider != "schwab" {
		return PortfolioReconciliation{}, ErrReconciliationUnavailable
	}
	connection, credentials, err := s.credentials(ctx, principal.UserID, account.ProviderConnectionID)
	if err != nil {
		return PortfolioReconciliation{}, err
	}
	provider, err := s.provider(connection.Provider)
	if err != nil {
		return PortfolioReconciliation{}, err
	}
	observedAt := time.Now().UTC()
	balancesStatus := "READY"
	positionsStatus := "READY"
	balances, balanceErr := provider.GetBalances(ctx, &credentials, account.ProviderAccountID)
	if balanceErr != nil {
		balancesStatus = "UNAVAILABLE"
		balances = financial.Balances{}
		s.observeProviderFailure(ctx, principal.UserID, connection, balanceErr)
	} else {
		balances, err = normalizeReconciliationBalances(balances)
		if err != nil {
			return PortfolioReconciliation{}, err
		}
	}
	providerPositions := []financial.Position(nil)
	var positionsErr error
	if terminalReconciliationAuthorizationError(balanceErr) {
		positionsStatus = "UNAVAILABLE"
	} else {
		providerPositions, positionsErr = provider.GetPositions(ctx, &credentials, account.ProviderAccountID)
	}
	if positionsErr != nil {
		positionsStatus = "UNAVAILABLE"
		providerPositions = nil
		s.observeProviderFailure(ctx, principal.UserID, connection, positionsErr)
	}
	if len(providerPositions) > maxReconciliationPositions {
		return PortfolioReconciliation{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	positions := make([]ReconciliationPosition, 0, len(providerPositions))
	performanceCount := 0
	seen := map[reconciliationPositionKey]struct{}{}
	if positionsStatus == "READY" {
		for _, providerPosition := range providerPositions {
			position, normalizeErr := normalizePosition(providerPosition)
			if normalizeErr != nil {
				return PortfolioReconciliation{}, normalizeErr
			}
			key := reconciliationKey(position)
			if _, exists := seen[key]; exists {
				return PortfolioReconciliation{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
			}
			seen[key] = struct{}{}
			if position.PerformanceStatus == "AVAILABLE" {
				performanceCount++
			}
			positions = append(positions, position)
		}
		sort.Slice(positions, func(i, j int) bool {
			left, right := reconciliationKey(positions[i]), reconciliationKey(positions[j])
			if left.Symbol != right.Symbol {
				return left.Symbol < right.Symbol
			}
			if left.InstrumentType != right.InstrumentType {
				return left.InstrumentType < right.InstrumentType
			}
			return left.Direction < right.Direction
		})
	}
	report := PortfolioReconciliation{
		FinancialAccountID: account.ID, Provider: account.Provider,
		BalancesStatus: balancesStatus, PositionsStatus: positionsStatus,
		PerformanceStatus:         reconciliationPerformanceStatus(len(positions), performanceCount),
		RealizedPerformanceStatus: "UNAVAILABLE", AutonomyEnforcementActive: true,
		ObservedPositionCount: len(positions), PerformancePositionCount: performanceCount,
		Balances: balances, Changes: []ReconciliationChange{}, Positions: positions, ObservedAt: observedAt,
	}
	previous, previousErr := s.reconciliations.LatestReliableReconciliation(ctx, principal.UserID, account.ID)
	if previousErr != nil && !errors.Is(previousErr, ErrReconciliationNotFound) {
		return PortfolioReconciliation{}, previousErr
	}
	if balancesStatus == "UNAVAILABLE" || positionsStatus == "UNAVAILABLE" {
		report.ComparisonStatus = "INCOMPLETE"
		report.AutonomySignal = "INSUFFICIENT_EVIDENCE"
	} else if errors.Is(previousErr, ErrReconciliationNotFound) {
		report.ComparisonStatus = "BASELINE"
		report.AutonomySignal = "INSUFFICIENT_EVIDENCE"
	} else {
		report.PreviousReconciliationID = &previous.ID
		report.Changes = compareReconciliationPositions(previous.Positions, positions)
		report.ChangeCount = len(report.Changes)
		if report.ChangeCount > 0 {
			report.ComparisonStatus = "DRIFT_DETECTED"
			report.AutonomySignal = "REVIEW_RECOMMENDED"
		} else {
			report.ComparisonStatus = "MATCHED"
			report.AutonomySignal = "CLEAR"
		}
	}
	report.BlocksNewActions = report.ComparisonStatus != "MATCHED"
	evidence, err := json.Marshal(reconciliationEvidence{
		FinancialAccountID: report.FinancialAccountID, Provider: report.Provider,
		ComparisonStatus: report.ComparisonStatus, BalancesStatus: report.BalancesStatus,
		PositionsStatus: report.PositionsStatus, PerformanceStatus: report.PerformanceStatus,
		RealizedPerformanceStatus: report.RealizedPerformanceStatus, AutonomySignal: report.AutonomySignal,
		AutonomyEnforcementActive: report.AutonomyEnforcementActive,
		BlocksNewActions:          report.BlocksNewActions, Balances: report.Balances, PreviousReconciliationID: report.PreviousReconciliationID,
		Changes: report.Changes, Positions: report.Positions, ObservedAt: report.ObservedAt,
	})
	if err != nil {
		return PortfolioReconciliation{}, err
	}
	digest := sha256.Sum256(evidence)
	report.EvidenceHash = hex.EncodeToString(digest[:])
	report, err = s.reconciliations.CreateReconciliation(ctx, principal.UserID, report, digest[:])
	if err == nil {
		s.record(ctx, principal.UserID, "financial.portfolio_reconciled", map[string]any{
			"account_id": account.ID, "provider": account.Provider, "comparison_status": report.ComparisonStatus,
			"position_count": report.ObservedPositionCount, "change_count": report.ChangeCount,
		})
	}
	return report, err
}

func (s *Service) LatestReconciliation(ctx context.Context, principal authorization.Principal, accountID string) (PortfolioReconciliation, error) {
	if !allowed(principal) {
		return PortfolioReconciliation{}, ErrForbidden
	}
	if s.reconciliations == nil {
		return PortfolioReconciliation{}, ErrReconciliationUnavailable
	}
	account, err := s.GetAccount(ctx, principal, accountID)
	if err != nil {
		return PortfolioReconciliation{}, err
	}
	return s.reconciliations.LatestReconciliation(ctx, principal.UserID, account.ID)
}
