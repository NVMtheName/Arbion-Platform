package http

import (
	"errors"
	stdhttp "net/http"

	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/orderintent"
)

func registerOrderIntentRoutes(mux *stdhttp.ServeMux, handler *authHandler) {
	if handler.orderIntents == nil {
		return
	}
	mux.Handle("GET /api/accounts/{id}/order-intents", handler.require(stdhttp.HandlerFunc(handler.listOrderIntents)))
	mux.Handle("POST /api/accounts/{id}/order-intents", handler.require(stdhttp.HandlerFunc(handler.createOrderIntent)))
	mux.Handle("POST /api/order-intents/{id}/review", handler.require(stdhttp.HandlerFunc(handler.reviewOrderIntent)))
}

func (handler *authHandler) listOrderIntents(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	intents, err := handler.orderIntents.List(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		handler.orderIntentError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"order_intents": intents, "approval_scope": orderintent.ProposalReviewOnly,
		"submission_available": false, "risk_approval_available": false, "ai_execution_authority": false, "live_execution_available": false,
	})
}

func (handler *authHandler) createOrderIntent(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if !handler.csrf(request) {
		writeError(writer, stdhttp.StatusForbidden, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var command orderintent.CreateCommand
	if !decode(writer, request, &command) {
		return
	}
	intent, err := handler.orderIntents.CreateUI(request.Context(), principal(request), request.PathValue("id"), command)
	if err != nil {
		handler.orderIntentError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusCreated, map[string]any{
		"order_intent": intent, "approval_scope": orderintent.ProposalReviewOnly,
		"provider_order_created": false, "submission_available": false, "risk_approval_available": false, "ai_execution_authority": false, "live_execution_available": false,
	})
}

func (handler *authHandler) reviewOrderIntent(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if !handler.csrf(request) {
		writeError(writer, stdhttp.StatusForbidden, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var command orderintent.ReviewCommand
	if !decode(writer, request, &command) {
		return
	}
	intent, err := handler.orderIntents.Review(request.Context(), principal(request), request.PathValue("id"), command)
	command.MFACode = ""
	if err != nil {
		handler.orderIntentError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"order_intent": intent, "approval_scope": orderintent.ProposalReviewOnly,
		"provider_order_created": false, "submission_available": false, "risk_approval_available": false, "ai_execution_authority": false, "live_execution_available": false,
	})
}

func (handler *authHandler) orderIntentError(writer stdhttp.ResponseWriter, err error) {
	switch {
	case errors.Is(err, orderintent.ErrForbidden):
		writeError(writer, stdhttp.StatusForbidden, "ORDER_INTENT_FORBIDDEN", "Your plan does not allow connected-account order proposals.")
	case errors.Is(err, orderintent.ErrInvalid):
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_ORDER_INTENT", "Use a supported asset, BUY or SELL, a positive decimal amount, an account capital policy, and a unique request key.")
	case errors.Is(err, orderintent.ErrNotFound):
		writeError(writer, stdhttp.StatusNotFound, "ORDER_INTENT_NOT_FOUND", "The order proposal was not found.")
	case errors.Is(err, orderintent.ErrIdempotencyConflict):
		writeError(writer, stdhttp.StatusConflict, "ORDER_INTENT_IDEMPOTENCY_CONFLICT", "This request key is already bound to a different proposal.")
	case errors.Is(err, orderintent.ErrConflict):
		writeError(writer, stdhttp.StatusConflict, "ORDER_INTENT_CONFLICT", "The proposal changed or is no longer reviewable.")
	case errors.Is(err, orderintent.ErrExpired):
		writeError(writer, stdhttp.StatusConflict, "ORDER_INTENT_PREVIEW_EXPIRED", "The Coinbase preview expired. Create a fresh proposal before reviewing it.")
	case errors.Is(err, orderintent.ErrBlocked):
		writeError(writer, stdhttp.StatusConflict, "ORDER_INTENT_BLOCKED", "The connected account or provider permissions block this proposal.")
	case errors.Is(err, orderintent.ErrUnsafeProviderEvidence):
		writeError(writer, stdhttp.StatusBadGateway, "UNSAFE_PROVIDER_PREVIEW", "Coinbase returned preview evidence that Arbion could not safely normalize.")
	case errors.Is(err, orderintent.ErrUnsafeRiskEvidence):
		writeError(writer, stdhttp.StatusBadGateway, "UNSAFE_RISK_EVIDENCE", "Arbion could not safely normalize the connected account's capital and risk evidence.")
	case errors.Is(err, auth.ErrMFANotEnabled):
		writeError(writer, stdhttp.StatusConflict, "MFA_REQUIRED", "Enable authenticator MFA before reviewing an order proposal.")
	case errors.Is(err, auth.ErrInvalidMFACode):
		writeError(writer, stdhttp.StatusUnauthorized, "INVALID_MFA_CODE", "The fresh authenticator code is invalid or was already used.")
	case errors.Is(err, auth.ErrRateLimited):
		writeError(writer, stdhttp.StatusTooManyRequests, "MFA_RATE_LIMITED", "Too many authenticator attempts. Try again later.")
	case errors.Is(err, auth.ErrMFAUnavailable):
		writeError(writer, stdhttp.StatusServiceUnavailable, "MFA_UNAVAILABLE", "Authenticator verification is temporarily unavailable.")
	default:
		handler.financialError(writer, err)
	}
}
