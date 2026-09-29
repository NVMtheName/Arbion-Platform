package coinbase

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/financial"
)

var _ execution.CancellationSender = (*ExecutionAdapter)(nil)

const executionCancellationPath = "/api/v3/brokerage/orders/batch_cancel"

type executionCancellationRequest struct {
	OrderIDs []string `json:"order_ids"`
}

type executionCancellationResult struct {
	OrderID       *string         `json:"order_id"`
	Success       *bool           `json:"success"`
	FailureReason json.RawMessage `json:"failure_reason"`
}

type executionCancellationResponse struct {
	Results *[]executionCancellationResult `json:"results"`
}

// CancelOnce is isolated from the existing financial Client and remains
// runtime-unwired. A durable winning cancellation claim must precede this call.
// Its only write is one singleton cancellation of the known original order;
// neither an accepted nor a rejected response proves terminal status or releases
// funds. Transport errors remain indeterminate and are never retried here.
// Wire contract: coinbase-advanced-py coinbase/rest/orders.py cancel_orders and
// coinbase/rest/types/orders_types.py CancelOrderObject (official Coinbase SDK).
func (a *ExecutionAdapter) CancelOnce(ctx context.Context, credentials *financial.Credentials, s execution.ConfirmedSubmission, attempt execution.Attempt) (execution.CancellationAcknowledgement, error) {
	fail := func(err error) (execution.CancellationAcknowledgement, error) {
		return execution.CancellationAcknowledgement{}, err
	}
	deadline, bounded := ctx.Deadline()
	remaining := time.Until(deadline)
	if !bounded || remaining <= 0 || remaining > 5*time.Second || ctx.Err() != nil {
		return fail(invalidExecutionResponse())
	}
	c, closeTransport, err := a.isolatedClient()
	if err != nil {
		return fail(err)
	}
	defer closeTransport()
	if execution.ValidateRecoverySubmission(s, attempt, c.now().UTC()) != nil || !executionUUID(attempt.ProviderOrderID) {
		return fail(invalidExecutionResponse())
	}
	key, err := executionSubmissionKey(credentials, s.PortfolioID)
	if err != nil {
		return fail(err)
	}
	if err := executionCheckPermissions(ctx, c, key, true); err != nil {
		return fail(err)
	}
	var detail executionOrderDetail
	if err := c.submissionRequest(ctx, key, http.MethodGet, executionHistoryPath+attempt.ProviderOrderID, nil, &detail); err != nil {
		return fail(err)
	}
	if _, ok := exactExecutionDetail(detail, s, attempt, attempt.ProviderOrderID, c.now().UTC()); !ok {
		return fail(invalidExecutionResponse())
	}
	o := detail.Order
	if o.Status == nil || !requiredExecutionBools(o.PendingCancel, o.Settled) || *o.PendingCancel || *o.Settled {
		return fail(invalidExecutionResponse())
	}
	switch *o.Status {
	case "PENDING", "OPEN", "QUEUED", "EDIT_QUEUED":
	default:
		// CANCEL_QUEUED already represents a pending cancellation. Terminal
		// and unknown states must be reconciled, not sent another mutation.
		return fail(invalidExecutionResponse())
	}
	if ctx.Err() != nil || execution.ValidateRecoverySubmission(s, attempt, c.now().UTC()) != nil {
		return fail(invalidExecutionResponse())
	}
	var response executionCancellationResponse
	input := executionCancellationRequest{OrderIDs: []string{attempt.ProviderOrderID}}
	if err := c.submissionRequest(ctx, key, http.MethodPost, executionCancellationPath, input, &response); err != nil {
		return fail(err)
	}
	if ctx.Err() != nil || response.Results == nil || len(*response.Results) != 1 {
		return fail(invalidExecutionResponse())
	}
	result := (*response.Results)[0]
	if result.OrderID == nil || *result.OrderID != attempt.ProviderOrderID || result.Success == nil {
		return fail(invalidExecutionResponse())
	}
	reason := ""
	if len(result.FailureReason) != 0 {
		// Omission is permitted; explicit null, numbers, arbitrary text or
		// objects are not a documented machine-code response.
		if result.FailureReason[0] != '"' || json.Unmarshal(result.FailureReason, &reason) != nil || (reason != "" && !executionRejectionCode.MatchString(reason)) {
			return fail(invalidExecutionResponse())
		}
	}
	if *result.Success && reason != "" && reason != "UNKNOWN_CANCEL_FAILURE_REASON" {
		return fail(invalidExecutionResponse())
	}
	return execution.CancellationAcknowledgement{ProviderOrderID: attempt.ProviderOrderID, Accepted: *result.Success}, nil
}
