package coinbase

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/financial"
)

var _ execution.AccountSettlementProvider = (*ExecutionAdapter)(nil)

// CollectAccountSettlement collects GET-only evidence for the already known
// order. A complete terminal fill observation precedes two identical complete
// account traversals, bracketed by empty unresolved-order reads. Coinbase does
// not expose a common atomic revision for these endpoints: this is conservative
// observed stability, not a broker-side atomic snapshot or authority to trade.
func (a *ExecutionAdapter) CollectAccountSettlement(ctx context.Context, credentials *financial.Credentials, s execution.ConfirmedSubmission, attempt execution.Attempt) (execution.AccountSettlementEvidence, error) {
	fail := func(err error) (execution.AccountSettlementEvidence, error) {
		return execution.AccountSettlementEvidence{}, err
	}
	// The nested order/fill collector inherits this deadline; its own bound
	// cannot extend the total operation or leave detached requests running.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	c, closeTransport, err := a.isolatedClient()
	if err != nil {
		return fail(err)
	}
	defer closeTransport()
	started := c.now().UTC()
	if ctx.Err() != nil || execution.ValidateRecoverySubmission(s, attempt, started) != nil || !executionUUID(attempt.ProviderOrderID) {
		return fail(invalidExecutionResponse())
	}
	key, err := executionSubmissionKey(credentials, s.PortfolioID)
	if err != nil {
		return fail(err)
	}
	// This existing collector performs fresh View/no-Transfer/exact-portfolio
	// permission checks and brackets complete fills with unchanged order reads.
	observation, err := NewExecutionAdapter(c).CollectExecutionObservation(ctx, key, s, attempt)
	if err != nil {
		return fail(err)
	}
	switch observation.Status {
	case "FILLED", "CANCELLED", "EXPIRED", "REJECTED":
	default:
		return fail(invalidExecutionResponse())
	}
	if err := c.executionSettlementNoOpenOrders(ctx, key, s.PortfolioID); err != nil {
		return fail(err)
	}
	base := strings.TrimSuffix(s.Order.Request.ProductID, "-USD")
	first := execution.ProviderPreflight{PortfolioID: s.PortfolioID}
	if err := c.executionAccounts(ctx, key, base, &first); err != nil {
		return fail(err)
	}
	second := execution.ProviderPreflight{PortfolioID: s.PortfolioID}
	if err := c.executionAccounts(ctx, key, base, &second); err != nil {
		return fail(err)
	}
	for _, amounts := range [][2]string{
		{first.CashUSD, second.CashUSD}, {first.AvailableCashUSD, second.AvailableCashUSD},
		{first.TotalBase, second.TotalBase}, {first.AvailableBase, second.AvailableBase},
	} {
		// Both sides came from the strict complete account reader; invalid,
		// missing, negative, foreign, unready or held amounts already failed.
		if compareDecimal(financial.Decimal(amounts[0]), financial.Decimal(amounts[1])) != 0 {
			return fail(invalidExecutionResponse())
		}
	}
	if err := c.executionSettlementNoOpenOrders(ctx, key, s.PortfolioID); err != nil {
		return fail(err)
	}
	evidence := execution.AccountSettlementEvidence{
		PortfolioID: s.PortfolioID, Observation: observation,
		CashUSD: second.CashUSD, AvailableCashUSD: second.AvailableCashUSD,
		TotalBase: second.TotalBase, AvailableBase: second.AvailableBase,
		Complete: true, NoOpenOrders: true, StartedAt: started, CompletedAt: c.now().UTC(),
	}
	if ctx.Err() != nil {
		return fail(invalidExecutionResponse())
	}
	if err := execution.ValidateAccountSettlementEvidence(s, attempt, evidence, evidence.CompletedAt); err != nil {
		return fail(err)
	}
	return evidence, nil
}

// A filtered response must itself be a complete empty result, not a truncated
// page or a projection that silently skips an unfamiliar unresolved order.
func (c *Client) executionSettlementNoOpenOrders(ctx context.Context, key *financial.Credentials, portfolio string) error {
	query := url.Values{"retail_portfolio_id": {portfolio}, "limit": {"100"}, "order_status": {
		"PENDING", "OPEN", "QUEUED", "CANCEL_QUEUED", "EDIT_QUEUED", "UNKNOWN_ORDER_STATUS",
	}}
	var orders executionHistoryPage
	if err := c.submissionRequest(ctx, key, http.MethodGet, executionHistoryPath+"batch?"+query.Encode(), nil, &orders); err != nil {
		return err
	}
	if !validExecutionHistoryPage(orders) || *orders.HasNext || len(*orders.Orders) != 0 {
		return invalidExecutionResponse()
	}
	return nil
}
