package http

import (
	"context"
	"errors"
	stdhttp "net/http"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/risk"
)

type breakerController interface {
	CurrentAutomation(context.Context, authorization.Principal, string) (*risk.CircuitBreaker, error)
	EngageAutomation(context.Context, authorization.Principal, string, risk.BreakerCommand) (risk.CircuitBreaker, error)
	ReleaseAutomation(context.Context, authorization.Principal, string, risk.BreakerCommand) (risk.CircuitBreaker, error)
	CurrentAccount(context.Context, authorization.Principal, string) (*risk.CircuitBreaker, error)
	EngageAccount(context.Context, authorization.Principal, string, risk.BreakerCommand) (risk.CircuitBreaker, error)
	ReleaseAccount(context.Context, authorization.Principal, string, risk.BreakerCommand) (risk.CircuitBreaker, error)
	CurrentUser(context.Context, authorization.Principal) (*risk.CircuitBreaker, error)
	EngageUser(context.Context, authorization.Principal, risk.BreakerCommand) (risk.CircuitBreaker, error)
	ReleaseUser(context.Context, authorization.Principal, risk.BreakerCommand) (risk.CircuitBreaker, error)
}

func registerBreakerRoutes(mux *stdhttp.ServeMux, handler *authHandler) {
	if handler.breakers == nil {
		return
	}
	mux.Handle("GET /api/automations/{id}/circuit-breaker", handler.require(stdhttp.HandlerFunc(handler.currentAutomationBreaker)))
	mux.Handle("POST /api/automations/{id}/circuit-breaker/engage", handler.require(stdhttp.HandlerFunc(handler.engageAutomationBreaker)))
	mux.Handle("POST /api/automations/{id}/circuit-breaker/release", handler.require(stdhttp.HandlerFunc(handler.releaseAutomationBreaker)))
	mux.Handle("GET /api/accounts/{id}/circuit-breaker", handler.require(stdhttp.HandlerFunc(handler.currentAccountBreaker)))
	mux.Handle("POST /api/accounts/{id}/circuit-breaker/engage", handler.require(stdhttp.HandlerFunc(handler.engageAccountBreaker)))
	mux.Handle("POST /api/accounts/{id}/circuit-breaker/release", handler.require(stdhttp.HandlerFunc(handler.releaseAccountBreaker)))
	mux.Handle("GET /api/risk/circuit-breaker", handler.require(stdhttp.HandlerFunc(handler.currentUserBreaker)))
	mux.Handle("POST /api/risk/circuit-breaker/engage", handler.require(stdhttp.HandlerFunc(handler.engageUserBreaker)))
	mux.Handle("POST /api/risk/circuit-breaker/release", handler.require(stdhttp.HandlerFunc(handler.releaseUserBreaker)))
}

func (handler *authHandler) breakerError(writer stdhttp.ResponseWriter, err error) {
	switch {
	case errors.Is(err, risk.ErrBreakerForbidden):
		writeError(writer, 403, "PERMISSION_DENIED", "Automation entitlement is required.")
	case errors.Is(err, risk.ErrBreakerNotFound):
		writeError(writer, 404, "NOT_FOUND", "The requested safety-control resource was not found.")
	case errors.Is(err, risk.ErrBreakerConflict):
		writeError(writer, 409, "CIRCUIT_BREAKER_CONFLICT", "The emergency-stop state changed. Refresh and review its latest state.")
	case errors.Is(err, risk.ErrBreakerInvalid):
		writeError(writer, 422, "INVALID_CIRCUIT_BREAKER", "A reason and explicit confirmation are required.")
	default:
		writeError(writer, 500, "INTERNAL_ERROR", "The emergency-stop request could not be completed.")
	}
}

func (handler *authHandler) currentAutomationBreaker(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	breaker, err := handler.breakers.CurrentAutomation(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		handler.breakerError(writer, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, 200, map[string]any{"circuit_breaker": breaker, "live_execution_available": false})
}

func (handler *authHandler) engageAutomationBreaker(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	handler.breakerMutation(writer, request, func(command risk.BreakerCommand) (risk.CircuitBreaker, error) {
		return handler.breakers.EngageAutomation(request.Context(), principal(request), request.PathValue("id"), command)
	})
}

func (handler *authHandler) releaseAutomationBreaker(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	handler.breakerMutation(writer, request, func(command risk.BreakerCommand) (risk.CircuitBreaker, error) {
		return handler.breakers.ReleaseAutomation(request.Context(), principal(request), request.PathValue("id"), command)
	})
}

func (handler *authHandler) currentAccountBreaker(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	breaker, err := handler.breakers.CurrentAccount(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		handler.breakerError(writer, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, 200, map[string]any{"circuit_breaker": breaker, "live_execution_available": false})
}

func (handler *authHandler) engageAccountBreaker(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	handler.breakerMutation(writer, request, func(command risk.BreakerCommand) (risk.CircuitBreaker, error) {
		return handler.breakers.EngageAccount(request.Context(), principal(request), request.PathValue("id"), command)
	})
}

func (handler *authHandler) releaseAccountBreaker(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	handler.breakerMutation(writer, request, func(command risk.BreakerCommand) (risk.CircuitBreaker, error) {
		return handler.breakers.ReleaseAccount(request.Context(), principal(request), request.PathValue("id"), command)
	})
}

func (handler *authHandler) currentUserBreaker(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	breaker, err := handler.breakers.CurrentUser(request.Context(), principal(request))
	if err != nil {
		handler.breakerError(writer, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, 200, map[string]any{"circuit_breaker": breaker, "live_execution_available": false})
}

func (handler *authHandler) engageUserBreaker(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	handler.breakerMutation(writer, request, func(command risk.BreakerCommand) (risk.CircuitBreaker, error) {
		return handler.breakers.EngageUser(request.Context(), principal(request), command)
	})
}

func (handler *authHandler) releaseUserBreaker(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	handler.breakerMutation(writer, request, func(command risk.BreakerCommand) (risk.CircuitBreaker, error) {
		return handler.breakers.ReleaseUser(request.Context(), principal(request), command)
	})
}

func (handler *authHandler) breakerMutation(writer stdhttp.ResponseWriter, request *stdhttp.Request, mutate func(risk.BreakerCommand) (risk.CircuitBreaker, error)) {
	if !handler.csrf(request) {
		writeError(writer, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var command risk.BreakerCommand
	if !decode(writer, request, &command) {
		return
	}
	breaker, err := mutate(command)
	if err != nil {
		handler.breakerError(writer, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, 200, map[string]any{
		"circuit_breaker":          breaker,
		"live_execution_available": false,
		"broker_action_requested":  false,
	})
}
