package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"

	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/platform/config"
)

// This interface is private to the transport. Production construction accepts
// only the owner workflow, never an injectable authorizer, sender or provider.
type ownerExecutionController interface {
	Prepare(context.Context, authorization.Principal, execution.OwnerPrepareCommand) (execution.OwnerOrder, error)
	Get(context.Context, authorization.Principal, string) (execution.OwnerOrder, error)
	Approve(context.Context, authorization.Principal, string, execution.OwnerApproveCommand) (execution.OwnerOrder, error)
	Revoke(context.Context, authorization.Principal, string) (execution.OwnerOrder, error)
	Capture(context.Context, authorization.Principal, string) (execution.OwnerPreflight, error)
	Send(context.Context, authorization.Principal, string, execution.OwnerSendCommand) (execution.OwnerOrder, error)
	Recover(context.Context, authorization.Principal, string) (execution.OwnerOrder, error)
	Reconcile(context.Context, authorization.Principal, string) (execution.OwnerOrder, error)
	Cancel(context.Context, authorization.Principal, string) (execution.OwnerOrder, error)
	Settle(context.Context, authorization.Principal, string) (execution.OwnerOrder, error)
}

// NewOwnerExecutionHandler is deliberately unmounted. Calling this constructor
// does not register routes on the application or activate a broker workflow.
func NewOwnerExecutionHandler(cfg config.Auth, service *auth.Service, workflow *execution.OwnerWorkflow) stdhttp.Handler {
	var controller ownerExecutionController
	if workflow != nil {
		controller = workflow
	}
	return newOwnerExecutionHandler(cfg, service, controller)
}

func newOwnerExecutionHandler(cfg config.Auth, service *auth.Service, workflow ownerExecutionController) stdhttp.Handler {
	h := &authHandler{service: service, cfg: cfg}
	mux := stdhttp.NewServeMux()
	const orders = "/api/personal-execution/orders"
	respond := func(w stdhttp.ResponseWriter, order execution.OwnerOrder, err error) {
		if err != nil {
			ownerExecutionError(w, err)
			return
		}
		writeJSON(w, stdhttp.StatusOK, map[string]execution.OwnerOrder{"order": order})
	}
	post := func(next stdhttp.HandlerFunc) stdhttp.HandlerFunc {
		return func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			if !h.csrf(r) {
				writeError(w, stdhttp.StatusForbidden, "csrf_rejected", "Request origin is not allowed.")
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("POST "+orders, post(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		var command execution.OwnerPrepareCommand
		if !decodeOwnerExecution(w, r, &command, "request_key", "side", "base_size", "limit_price", "fee_allowance_usd", "maximum_debit_usd") {
			return
		}
		order, err := workflow.Prepare(r.Context(), principal(r), command)
		respond(w, order, err)
	}))
	mux.HandleFunc("GET "+orders+"/{id}", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		// ServeMux otherwise treats HEAD as GET. This owner command surface has
		// only the explicitly documented methods, even for read operations.
		if r.Method != stdhttp.MethodGet {
			w.Header().Set("Allow", stdhttp.MethodGet)
			writeError(w, stdhttp.StatusMethodNotAllowed, "method_not_allowed", "Request method is not allowed.")
			return
		}
		order, err := workflow.Get(r.Context(), principal(r), r.PathValue("id"))
		respond(w, order, err)
	})
	mux.HandleFunc("POST "+orders+"/{id}/approve", post(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		var command execution.OwnerApproveCommand
		if !decodeOwnerExecution(w, r, &command, "expected_digest", "mfa_code") {
			return
		}
		order, err := workflow.Approve(r.Context(), principal(r), r.PathValue("id"), command)
		command.MFACode = ""
		respond(w, order, err)
	}))
	mux.HandleFunc("POST "+orders+"/{id}/send", post(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		var command execution.OwnerSendCommand
		if !decodeOwnerExecution(w, r, &command, "evidence_id") {
			return
		}
		order, err := workflow.Send(r.Context(), principal(r), r.PathValue("id"), command)
		respond(w, order, err)
	}))
	mux.HandleFunc("POST "+orders+"/{id}/preflight", post(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if !decodeOwnerExecution(w, r, &struct{}{}) {
			return
		}
		proof, err := workflow.Capture(r.Context(), principal(r), r.PathValue("id"))
		if err != nil {
			ownerExecutionError(w, err)
			return
		}
		writeJSON(w, stdhttp.StatusOK, proof)
	}))
	for name, action := range map[string]func(context.Context, authorization.Principal, string) (execution.OwnerOrder, error){
		"revoke": workflowAction(workflow, "revoke"), "recover": workflowAction(workflow, "recover"),
		"reconcile": workflowAction(workflow, "reconcile"), "cancel": workflowAction(workflow, "cancel"), "settle": workflowAction(workflow, "settle"),
	} {
		mux.HandleFunc("POST "+orders+"/{id}/"+name, post(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			if !decodeOwnerExecution(w, r, &struct{}{}) {
				return
			}
			order, err := action(r.Context(), principal(r), r.PathValue("id"))
			respond(w, order, err)
		}))
	}
	protected := h.require(mux)
	return securityHeaders(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		// Set this outside authentication so cookies, denials and malformed routes
		// cannot accidentally become cacheable private execution responses.
		w.Header().Set("Cache-Control", "no-store")
		if service == nil || workflow == nil {
			ownerExecutionError(w, nil)
			return
		}
		protected.ServeHTTP(w, r)
	}))
}

// Delay method selection until a request so a missing workflow remains a closed,
// constructible handler rather than panicking while routes are assembled.
func workflowAction(workflow ownerExecutionController, action string) func(context.Context, authorization.Principal, string) (execution.OwnerOrder, error) {
	return func(ctx context.Context, p authorization.Principal, id string) (execution.OwnerOrder, error) {
		switch action {
		case "revoke":
			return workflow.Revoke(ctx, p, id)
		case "recover":
			return workflow.Recover(ctx, p, id)
		case "reconcile":
			return workflow.Reconcile(ctx, p, id)
		case "cancel":
			return workflow.Cancel(ctx, p, id)
		case "settle":
			return workflow.Settle(ctx, p, id)
		default:
			return execution.OwnerOrder{}, execution.ErrInvalid
		}
	}
}

// Commands contain only exact string fields. In addition to the shared 4 KiB,
// single-JSON decoder, reject null, duplicate/case-aliased members and unknown
// authority fields. Even no-payload commands require an explicit empty object.
func decodeOwnerExecution(w stdhttp.ResponseWriter, r *stdhttp.Request, out any, fields ...string) bool {
	var raw json.RawMessage
	if !decode(w, r, &raw) {
		return false
	}
	invalid := func() bool {
		writeError(w, stdhttp.StatusBadRequest, "invalid_request", "Request body is invalid.")
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	first, err := d.Token()
	if err != nil || first != json.Delim('{') {
		return invalid()
	}
	allowed, seen := map[string]bool{}, map[string]bool{}
	for _, field := range fields {
		allowed[field] = true
	}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok || !allowed[key] || seen[key] {
			return invalid()
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil || len(value) == 0 || value[0] != '"' {
			return invalid()
		}
	}
	if _, err := d.Token(); err != nil || len(seen) != len(allowed) || json.Unmarshal(raw, out) != nil {
		return invalid()
	}
	return true
}

func ownerExecutionError(w stdhttp.ResponseWriter, err error) {
	status, code, message := stdhttp.StatusServiceUnavailable, "OWNER_EXECUTION_UNAVAILABLE", "The request could not be completed. Check saved status before taking further action; do not repeat a send or cancellation."
	switch {
	case errors.Is(err, execution.ErrNoSendResolutionUnknown), errors.Is(err, execution.ErrCommitUnknown), errors.Is(err, execution.ErrSubmissionUnknown), errors.Is(err, execution.ErrCancellationUnknown):
		status, code, message = stdhttp.StatusConflict, "EXECUTION_OUTCOME_UNKNOWN", "The outcome is not confirmed. Read saved status and recover or reconcile; do not retry the send or cancellation."
	case errors.Is(err, execution.ErrSubmissionNotSent):
		status, code, message = stdhttp.StatusConflict, "EXECUTION_NOT_SENT", "The sender was not entered and the original attempt is closed. Do not resend this order."
	case errors.Is(err, execution.ErrAlreadyAttempted):
		status, code, message = stdhttp.StatusConflict, "EXECUTION_ALREADY_ATTEMPTED", "An attempt is already recorded. Recover or reconcile; do not resend."
	case errors.Is(err, execution.ErrSubmissionRejected):
		status, code, message = stdhttp.StatusConflict, "EXECUTION_REJECTED", "Submission rejection was recorded. This is not settlement; do not resend."
	case errors.Is(err, execution.ErrCancellationNotAccepted):
		status, code, message = stdhttp.StatusConflict, "EXECUTION_CANCELLATION_NOT_ACCEPTED", "Cancellation was not accepted. Reconcile the original order; do not retry cancellation."
	case errors.Is(err, execution.ErrNotFound):
		status, code, message = stdhttp.StatusNotFound, "EXECUTION_NOT_FOUND", "The order is unavailable."
	case errors.Is(err, execution.ErrInvalid):
		status, code, message = stdhttp.StatusBadRequest, "invalid_request", "Request body is invalid."
	case errors.Is(err, execution.ErrNotAuthorized), errors.Is(err, authorization.ErrForbidden), errors.Is(err, auth.ErrMFARequired), errors.Is(err, auth.ErrMFANotEnabled), errors.Is(err, auth.ErrInvalidMFACode):
		status, code, message = stdhttp.StatusForbidden, "EXECUTION_NOT_AUTHORIZED", "Current owner authorization is unavailable."
	case errors.Is(err, auth.ErrRateLimited):
		status, code, message = stdhttp.StatusTooManyRequests, "EXECUTION_RATE_LIMITED", "Too many verification attempts."
	case errors.Is(err, execution.ErrConflict), errors.Is(err, execution.ErrAccountHeld), errors.Is(err, execution.ErrCapitalHeld), errors.Is(err, execution.ErrUnreconciled), errors.Is(err, execution.ErrReconciliationBlocked):
		status, code, message = stdhttp.StatusConflict, "EXECUTION_REVIEW_REQUIRED", "The order requires review or reconciliation before further action."
	}
	writeError(w, status, code, message)
}
