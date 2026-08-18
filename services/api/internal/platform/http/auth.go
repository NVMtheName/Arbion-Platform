package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	stdhttp "net/http"
	"net/url"
	"strings"

	"github.com/arbion/platform/services/api/internal/aiconnection"
	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/financialconnection"
	"github.com/arbion/platform/services/api/internal/neural"
	"github.com/arbion/platform/services/api/internal/platform/config"
	"github.com/arbion/platform/services/api/internal/strategy"
)

type identityKey struct{}
type authHandler struct {
	service            *auth.Service
	admin              *authorization.Service
	ai                 *aiconnection.Service
	financialProviders financial.Registry
	schwabConfigured   bool
	financial          *financialconnection.Service
	automation         *automation.Service
	strategies         *strategy.InstanceService
	evaluations        *strategy.EvaluationService
	cfg                config.Auth
}
type credentials struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}
type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewApplicationHandler(database ReadinessChecker, timeout config.Config, service *auth.Service, admins ...*authorization.Service) stdhttp.Handler {
	mux := stdhttp.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /readyz", readiness(database, timeout.Database.ReadinessTimeout))
	h := &authHandler{service: service, cfg: timeout.Auth}
	if len(admins) > 0 {
		h.admin = admins[0]
	}
	mux.HandleFunc("POST /api/auth/register", h.register)
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("POST /api/auth/verification/request", h.requestVerification)
	mux.HandleFunc("POST /api/auth/verification/confirm", h.confirmVerification)
	mux.HandleFunc("POST /api/auth/password-reset/request", h.requestPasswordReset)
	mux.HandleFunc("POST /api/auth/password-reset/confirm", h.confirmPasswordReset)
	mux.Handle("POST /api/auth/logout", h.require(stdhttp.HandlerFunc(h.logout)))
	mux.Handle("POST /api/auth/logout-all", h.require(stdhttp.HandlerFunc(h.logoutAll)))
	mux.Handle("PUT /api/auth/password", h.require(stdhttp.HandlerFunc(h.changePassword)))
	mux.Handle("GET /api/auth/me", h.require(stdhttp.HandlerFunc(h.me)))
	mux.Handle("GET /api/auth/protected-test", h.require(stdhttp.HandlerFunc(h.me)))
	if h.admin != nil {
		mux.Handle("GET /api/admin/me", h.require(h.requireAdmin(stdhttp.HandlerFunc(h.adminMe))))
		mux.Handle("GET /api/admin/users", h.require(h.requireAdmin(stdhttp.HandlerFunc(h.adminUsers))))
		mux.Handle("GET /api/admin/users/{id}", h.require(h.requireAdmin(stdhttp.HandlerFunc(h.adminUser))))
		mux.Handle("PUT /api/admin/users/{id}/role", h.require(h.requireAdmin(stdhttp.HandlerFunc(h.updateRole))))
		mux.Handle("PUT /api/admin/users/{id}/entitlement", h.require(h.requireAdmin(stdhttp.HandlerFunc(h.updateEntitlement))))
	}
	return securityHeaders(mux)
}

func NewFullApplicationHandler(database ReadinessChecker, cfg config.Config, service *auth.Service, admin *authorization.Service, ai *aiconnection.Service, finances ...*financialconnection.Service) stdhttp.Handler {
	h := &authHandler{service: service, admin: admin, ai: ai, cfg: cfg.Auth, financialProviders: financial.DefaultRegistry(), schwabConfigured: cfg.Schwab.ClientID != "" && cfg.Schwab.ClientSecret != ""}
	if len(finances) > 0 {
		h.financial = finances[0]
	}
	mux := stdhttp.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /readyz", readiness(database, cfg.Database.ReadinessTimeout))
	mux.HandleFunc("POST /api/auth/register", h.register)
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("POST /api/auth/verification/request", h.requestVerification)
	mux.HandleFunc("POST /api/auth/verification/confirm", h.confirmVerification)
	mux.HandleFunc("POST /api/auth/password-reset/request", h.requestPasswordReset)
	mux.HandleFunc("POST /api/auth/password-reset/confirm", h.confirmPasswordReset)
	mux.Handle("POST /api/auth/logout", h.require(stdhttp.HandlerFunc(h.logout)))
	mux.Handle("POST /api/auth/logout-all", h.require(stdhttp.HandlerFunc(h.logoutAll)))
	mux.Handle("PUT /api/auth/password", h.require(stdhttp.HandlerFunc(h.changePassword)))
	mux.Handle("GET /api/auth/me", h.require(stdhttp.HandlerFunc(h.me)))
	mux.Handle("GET /api/admin/me", h.require(h.requireAdmin(stdhttp.HandlerFunc(h.adminMe))))
	mux.Handle("GET /api/admin/users", h.require(h.requireAdmin(stdhttp.HandlerFunc(h.adminUsers))))
	mux.Handle("GET /api/admin/users/{id}", h.require(h.requireAdmin(stdhttp.HandlerFunc(h.adminUser))))
	mux.Handle("PUT /api/admin/users/{id}/role", h.require(h.requireAdmin(stdhttp.HandlerFunc(h.updateRole))))
	mux.Handle("PUT /api/admin/users/{id}/entitlement", h.require(h.requireAdmin(stdhttp.HandlerFunc(h.updateEntitlement))))
	mux.Handle("GET /api/connections/ai", h.require(stdhttp.HandlerFunc(h.listAI)))
	mux.Handle("POST /api/connections/ai", h.require(stdhttp.HandlerFunc(h.createAI)))
	mux.Handle("PATCH /api/connections/ai/{id}", h.require(stdhttp.HandlerFunc(h.renameAI)))
	mux.Handle("PUT /api/connections/ai/{id}/credential", h.require(stdhttp.HandlerFunc(h.replaceAI)))
	mux.Handle("POST /api/connections/ai/{id}/enable", h.require(stdhttp.HandlerFunc(h.enableAI)))
	mux.Handle("POST /api/connections/ai/{id}/disable", h.require(stdhttp.HandlerFunc(h.disableAI)))
	mux.Handle("POST /api/connections/ai/{id}/verify", h.require(stdhttp.HandlerFunc(h.verifyAI)))
	mux.Handle("GET /api/connections/ai/{id}/models", h.require(stdhttp.HandlerFunc(h.modelsAI)))
	mux.Handle("GET /api/settings/neural-engine", h.require(stdhttp.HandlerFunc(h.getNeuralPreference)))
	mux.Handle("PUT /api/settings/neural-engine", h.require(stdhttp.HandlerFunc(h.setNeuralPreference)))
	mux.Handle("POST /api/neural/insight", h.require(stdhttp.HandlerFunc(h.neuralInsight)))
	mux.Handle("DELETE /api/connections/ai/{id}", h.require(stdhttp.HandlerFunc(h.deleteAI)))
	mux.Handle("GET /api/connections/financial/providers", h.require(stdhttp.HandlerFunc(h.listFinancialProviders)))
	if h.financial != nil {
		mux.Handle("GET /api/connections/financial", h.require(stdhttp.HandlerFunc(h.listFinancialConnections)))
		mux.Handle("POST /api/connections/financial/schwab/start", h.require(stdhttp.HandlerFunc(h.startSchwab)))
		mux.HandleFunc("GET /api/connections/financial/schwab/callback", h.callbackSchwab)
		mux.Handle("POST /api/connections/financial/{id}/sync", h.require(stdhttp.HandlerFunc(h.syncFinancial)))
		mux.Handle("POST /api/connections/financial/{id}/disable", h.require(stdhttp.HandlerFunc(h.disableFinancial)))
		mux.Handle("POST /api/connections/financial/{id}/enable", h.require(stdhttp.HandlerFunc(h.enableFinancial)))
		mux.Handle("DELETE /api/connections/financial/{id}", h.require(stdhttp.HandlerFunc(h.disconnectFinancial)))
		mux.Handle("GET /api/accounts", h.require(stdhttp.HandlerFunc(h.listAccounts)))
		mux.Handle("GET /api/accounts/{id}", h.require(stdhttp.HandlerFunc(h.getAccount)))
		mux.Handle("GET /api/accounts/{id}/balances", h.require(stdhttp.HandlerFunc(h.getBalances)))
		mux.Handle("GET /api/accounts/{id}/positions", h.require(stdhttp.HandlerFunc(h.getPositions)))
	}
	registerAutomationRoutes(mux, h)
	return securityHeaders(mux)
}

func NewFullApplicationHandlerWithAutomation(database ReadinessChecker, cfg config.Config, service *auth.Service, admin *authorization.Service, ai *aiconnection.Service, finances *financialconnection.Service, automations *automation.Service, strategies ...*strategy.InstanceService) stdhttp.Handler {
	return newFullApplicationHandlerWithAutomation(database, cfg, service, admin, ai, finances, automations, firstStrategy(strategies), nil)
}

func NewFullApplicationHandlerWithEvaluation(database ReadinessChecker, cfg config.Config, service *auth.Service, admin *authorization.Service, ai *aiconnection.Service, finances *financialconnection.Service, automations *automation.Service, strategies *strategy.InstanceService, evaluations *strategy.EvaluationService) stdhttp.Handler {
	return newFullApplicationHandlerWithAutomation(database, cfg, service, admin, ai, finances, automations, strategies, evaluations)
}

func firstStrategy(strategies []*strategy.InstanceService) *strategy.InstanceService {
	if len(strategies) == 0 {
		return nil
	}
	return strategies[0]
}

func newFullApplicationHandlerWithAutomation(database ReadinessChecker, cfg config.Config, service *auth.Service, admin *authorization.Service, ai *aiconnection.Service, finances *financialconnection.Service, automations *automation.Service, strategies *strategy.InstanceService, evaluations *strategy.EvaluationService) stdhttp.Handler {
	h := &authHandler{service: service, admin: admin, ai: ai, cfg: cfg.Auth, financialProviders: financial.DefaultRegistry(), schwabConfigured: cfg.Schwab.ClientID != "" && cfg.Schwab.ClientSecret != "", financial: finances, automation: automations}
	h.strategies = strategies
	h.evaluations = evaluations
	mux := stdhttp.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /readyz", readiness(database, cfg.Database.ReadinessTimeout))
	// Reuse the complete router and attach automation to a small outer router. Existing
	// routes keep their established middleware and behavior.
	base := NewFullApplicationHandler(database, cfg, service, admin, ai, finances)
	registerAutomationRoutes(mux, h)
	registerStrategyRoutes(mux, h)
	mux.Handle("/", base)
	return securityHeaders(mux)
}

func (h *authHandler) financialError(w stdhttp.ResponseWriter, e error) {
	status := 500
	code := "INTERNAL_ERROR"
	message := "The request could not be completed."
	if errors.Is(e, financialconnection.ErrForbidden) {
		status = 403
		code = "PERMISSION_DENIED"
		message = "Your plan does not allow financial connections."
	} else if errors.Is(e, financialconnection.ErrNotFound) {
		status = 404
		code = "ACCOUNT_NOT_FOUND"
		message = "The requested resource was not found."
	} else if errors.Is(e, financialconnection.ErrDisabled) {
		status = 409
		code = "CONNECTION_DISABLED"
		message = "The connection is disabled."
	} else {
		var pe *financial.ProviderError
		if errors.As(e, &pe) {
			code = string(pe.Code)
			status = 502
			if pe.Code == financial.AccountNotFound {
				status = 404
			}
		}
	}
	writeError(w, status, code, message)
}
func (h *authHandler) mutationOK(w stdhttp.ResponseWriter, r *stdhttp.Request, fn func() error) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	if e := fn(); e != nil {
		h.financialError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (h *authHandler) listFinancialConnections(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.financial.ListConnections(r.Context(), principal(r))
	if e != nil {
		h.financialError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"connections": v})
}
func (h *authHandler) startSchwab(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.schwabConfigured {
		writeError(w, 503, "PROVIDER_UNAVAILABLE", "Charles Schwab is not configured.")
		return
	}
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	u, e := h.financial.StartAuthorization(r.Context(), principal(r))
	if e != nil {
		h.financialError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"authorization_url": u})
}
func (h *authHandler) callbackSchwab(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	_, e := h.financial.CompleteAuthorization(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"), r.URL.Query().Get("error"))
	target := strings.TrimRight(h.cfg.AllowedOrigins[0], "/") + "/settings/connections?financial="
	if e != nil {
		target += "error"
	} else {
		target += "connected"
	}
	stdhttp.Redirect(w, r, target, stdhttp.StatusSeeOther)
}
func (h *authHandler) syncFinancial(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	h.mutationOK(w, r, func() error { return h.financial.Sync(r.Context(), principal(r), r.PathValue("id")) })
}
func (h *authHandler) disableFinancial(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	h.mutationOK(w, r, func() error {
		_, e := h.financial.SetEnabled(r.Context(), principal(r), r.PathValue("id"), false)
		return e
	})
}
func (h *authHandler) enableFinancial(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	h.mutationOK(w, r, func() error {
		_, e := h.financial.SetEnabled(r.Context(), principal(r), r.PathValue("id"), true)
		return e
	})
}
func (h *authHandler) disconnectFinancial(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	h.mutationOK(w, r, func() error { return h.financial.Disconnect(r.Context(), principal(r), r.PathValue("id")) })
}
func (h *authHandler) listAccounts(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.financial.ListAccounts(r.Context(), principal(r))
	if e != nil {
		h.financialError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"accounts": v})
}
func (h *authHandler) getAccount(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.financial.GetAccount(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.financialError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"account": v})
}
func (h *authHandler) getBalances(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.financial.GetBalances(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.financialError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"balances": v})
}
func (h *authHandler) getPositions(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.financial.GetPositions(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.financialError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"positions": v})
}
func (h *authHandler) listFinancialProviders(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	type providerResponse struct {
		financial.ProviderDefinition
		Configured bool `json:"configured"`
	}
	providers := make([]providerResponse, 0, len(h.financialProviders))
	for _, provider := range h.financialProviders.List() {
		configured := provider.Availability == financial.Implemented
		if provider.ID == "schwab" {
			configured = h.schwabConfigured
		}
		providers = append(providers, providerResponse{ProviderDefinition: provider, Configured: configured})
	}
	writeJSON(w, 200, map[string]any{"providers": providers, "can_connect_financial_accounts": authorization.CanConnectFinancialAccounts(principal(r))})
}
func (h *authHandler) verifyAI(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	c, e := h.ai.Verify(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.aiError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"connection": c})
}
func (h *authHandler) modelsAI(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	models, e := h.ai.Models(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.aiError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"models": models})
}
func (h *authHandler) getNeuralPreference(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	p, e := h.ai.Preference(r.Context(), principal(r))
	if e != nil {
		h.aiError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"preference": p})
}
func (h *authHandler) setNeuralPreference(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var in struct {
		ConnectionID string `json:"connection_id"`
		ModelID      string `json:"model_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	p, e := h.ai.SetPreference(r.Context(), principal(r), in.ConnectionID, in.ModelID)
	if e != nil {
		h.aiError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"preference": p})
}

func (h *authHandler) neuralInsight(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var in struct {
		Prompt  string `json:"prompt"`
		Profile string `json:"profile"`
	}
	if !decode(w, r, &in) {
		return
	}
	result, err := h.ai.Analyze(r.Context(), principal(r), in.Prompt, in.Profile)
	if err != nil {
		h.aiError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"insight": result})
}

func (h *authHandler) listAI(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	items, e := h.ai.List(r.Context(), principal(r))
	if e != nil {
		h.aiError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"connections": items, "providers": h.ai.Providers(), "can_use_neural_engine": authorization.CanUseNeuralEngine(principal(r))})
}
func (h *authHandler) csrf(r *stdhttp.Request) bool { return h.originAllowed(r) }
func (h *authHandler) createAI(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var in struct {
		Provider    string `json:"provider"`
		DisplayName string `json:"display_name"`
		Credential  string `json:"credential"`
	}
	if !decode(w, r, &in) {
		return
	}
	secret := []byte(in.Credential)
	in.Credential = ""
	c, e := h.ai.Create(r.Context(), principal(r), in.Provider, in.DisplayName, secret)
	clear(secret)
	if e != nil {
		h.aiError(w, e)
		return
	}
	writeJSON(w, 201, map[string]any{"connection": c})
}
func (h *authHandler) renameAI(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var in struct {
		DisplayName string `json:"display_name"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, e := h.ai.Rename(r.Context(), principal(r), r.PathValue("id"), in.DisplayName)
	if e != nil {
		h.aiError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"connection": c})
}
func (h *authHandler) replaceAI(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var in struct {
		Credential string `json:"credential"`
	}
	if !decode(w, r, &in) {
		return
	}
	secret := []byte(in.Credential)
	in.Credential = ""
	c, e := h.ai.Replace(r.Context(), principal(r), r.PathValue("id"), secret)
	clear(secret)
	if e != nil {
		h.aiError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"connection": c})
}
func (h *authHandler) enableAI(w stdhttp.ResponseWriter, r *stdhttp.Request)  { h.stateAI(w, r, true) }
func (h *authHandler) disableAI(w stdhttp.ResponseWriter, r *stdhttp.Request) { h.stateAI(w, r, false) }
func (h *authHandler) stateAI(w stdhttp.ResponseWriter, r *stdhttp.Request, enabled bool) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	c, e := h.ai.SetEnabled(r.Context(), principal(r), r.PathValue("id"), enabled)
	if e != nil {
		h.aiError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"connection": c})
}
func (h *authHandler) deleteAI(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	if e := h.ai.Delete(r.Context(), principal(r), r.PathValue("id")); e != nil {
		h.aiError(w, e)
		return
	}
	w.WriteHeader(204)
}
func (h *authHandler) aiError(w stdhttp.ResponseWriter, e error) {
	switch {
	case errors.Is(e, aiconnection.ErrForbidden):
		writeError(w, 403, "neural_engine_unavailable", "Neural Engine access is unavailable for the current plan.")
	case errors.Is(e, aiconnection.ErrNotFound):
		writeError(w, 404, "connection_not_found", "Connection not found.")
	case errors.Is(e, aiconnection.ErrInvalid):
		writeError(w, 400, "invalid_request", "Request details are invalid.")
	case errors.Is(e, aiconnection.ErrConflict):
		writeError(w, 409, "connection_in_use", "Resolve dependent configuration before removing this connection.")
	case errors.Is(e, aiconnection.ErrDisabled):
		writeError(w, 409, "connection_disabled", "Enable the connection before verifying it.")
	case errors.Is(e, aiconnection.ErrInactive):
		writeError(w, 409, "connection_not_active", "Verify and select an AI connection before using it.")
	case errors.Is(e, aiconnection.ErrRateLimit):
		writeError(w, 429, "insight_rate_limited", "The hourly insight limit was reached. Please try again later.")
	case errors.Is(e, aiconnection.ErrProvider):
		writeError(w, 503, "provider_unavailable", "AI analysis is temporarily unavailable.")
	case neural.Code(e) == neural.AuthenticationFailed:
		writeError(w, 401, "provider_authentication_failed", "Authentication was rejected by the AI provider. Your saved credential was not removed.")
	case neural.Code(e) == neural.RateLimited:
		writeError(w, 429, "provider_rate_limited", "The provider rate limit was reached. Please try again later.")
	case neural.Code(e) == neural.ProviderUnavailable || neural.Code(e) == neural.Timeout:
		writeError(w, 503, "provider_unavailable", "Provider is temporarily unavailable. Your saved credential was not removed.")
	default:
		writeError(w, 500, "internal_error", "The request could not be completed.")
	}
}
func principal(r *stdhttp.Request) authorization.Principal {
	u, _ := r.Context().Value(identityKey{}).(auth.SafeUser)
	return authorization.Principal{UserID: u.ID, Role: authorization.Role(u.Role), Entitlement: authorization.Entitlement(u.Entitlement)}
}
func (h *authHandler) requireAdmin(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if authorization.RequireAdmin(principal(r)) != nil {
			writeError(w, 403, "forbidden", "Administrative access is required.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (h *authHandler) adminMe(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	u, _ := r.Context().Value(identityKey{}).(auth.SafeUser)
	writeJSON(w, 200, map[string]any{"user": u})
}
func (h *authHandler) adminUsers(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.admin.List(r.Context(), principal(r))
	if e != nil {
		h.adminError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"users": v})
}
func (h *authHandler) adminUser(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.admin.Get(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.adminError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"user": v})
}
func (h *authHandler) updateRole(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.originAllowed(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var in struct {
		Role authorization.Role `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, e := h.admin.SetRole(r.Context(), principal(r), r.PathValue("id"), in.Role)
	if e != nil {
		h.adminError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"user": v})
}
func (h *authHandler) updateEntitlement(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.originAllowed(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var in struct {
		Entitlement     authorization.Entitlement `json:"entitlement"`
		BillingRequired bool                      `json:"billing_required"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, e := h.admin.SetEntitlement(r.Context(), principal(r), r.PathValue("id"), in.Entitlement, in.BillingRequired)
	if e != nil {
		h.adminError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"user": v})
}
func (h *authHandler) adminError(w stdhttp.ResponseWriter, e error) {
	if errors.Is(e, authorization.ErrForbidden) {
		writeError(w, 403, "forbidden", "The privileged change is not permitted.")
		return
	}
	writeError(w, 500, "internal_error", "The request could not be completed.")
}
func securityHeaders(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
func (h *authHandler) originAllowed(r *stdhttp.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	u, e := url.Parse(origin)
	if e != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	for _, allowed := range h.cfg.AllowedOrigins {
		if strings.TrimSpace(allowed) == origin {
			return true
		}
	}
	return false
}
func decode(w stdhttp.ResponseWriter, r *stdhttp.Request, v any) bool {
	r.Body = stdhttp.MaxBytesReader(w, r.Body, 4<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		writeError(w, stdhttp.StatusBadRequest, "invalid_request", "Request body is invalid.")
		return false
	}
	if d.Decode(&struct{}{}) != io.EOF {
		writeError(w, stdhttp.StatusBadRequest, "invalid_request", "Request body must contain one JSON object.")
		return false
	}
	return true
}
func writeJSON(w stdhttp.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w stdhttp.ResponseWriter, status int, code, message string) {
	var e apiError
	e.Error.Code = code
	e.Error.Message = message
	writeJSON(w, status, e)
}
func rateKey(r *stdhttp.Request) string {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e == nil {
		return host
	}
	return r.RemoteAddr
}
func (h *authHandler) setCookie(w stdhttp.ResponseWriter, token string) {
	stdhttp.SetCookie(w, &stdhttp.Cookie{Name: h.cfg.SessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: h.cfg.CookieSecure, SameSite: stdhttp.SameSiteLaxMode, MaxAge: int(h.cfg.SessionTTL.Seconds())})
}
func (h *authHandler) clearCookie(w stdhttp.ResponseWriter) {
	stdhttp.SetCookie(w, &stdhttp.Cookie{Name: h.cfg.SessionCookie, Path: "/", HttpOnly: true, Secure: h.cfg.CookieSecure, SameSite: stdhttp.SameSiteLaxMode, MaxAge: -1})
}
func (h *authHandler) register(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.originAllowed(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	u, t, e := h.service.Register(r.Context(), in.Email, in.Password, in.DisplayName, rateKey(r))
	if e != nil {
		h.authError(w, e)
		return
	}
	if h.service.RequiresEmailVerification() {
		writeJSON(w, stdhttp.StatusAccepted, map[string]any{"user": u, "verification_required": true})
		return
	}
	h.setCookie(w, t)
	writeJSON(w, 201, map[string]any{"user": u, "verification_required": false})
}
func (h *authHandler) login(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.originAllowed(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	u, t, e := h.service.Login(r.Context(), in.Email, in.Password, rateKey(r))
	if e != nil {
		h.authError(w, e)
		return
	}
	h.setCookie(w, t)
	writeJSON(w, 200, map[string]any{"user": u})
}
func (h *authHandler) authError(w stdhttp.ResponseWriter, e error) {
	switch {
	case errors.Is(e, auth.ErrConflict), errors.Is(e, auth.ErrRegistrationUnavailable):
		writeError(w, 409, "registration_unavailable", "Unable to create account with those details.")
	case errors.Is(e, auth.ErrInvalidCredentials):
		writeError(w, 401, "invalid_credentials", "Email or password is incorrect.")
	case errors.Is(e, auth.ErrEmailVerificationRequired):
		writeError(w, 403, "email_verification_required", "Verify your email before signing in.")
	case errors.Is(e, auth.ErrInvalidEmailToken):
		writeError(w, 400, "invalid_email_link", "This secure link is invalid or has expired.")
	case errors.Is(e, auth.ErrInvalidCurrentPassword):
		writeError(w, 401, "invalid_current_password", "Current password is incorrect.")
	case errors.Is(e, auth.ErrPasswordUnchanged):
		writeError(w, 409, "password_unchanged", "New password must be different from the current password.")
	case errors.Is(e, auth.ErrInvalidPassword):
		writeError(w, 400, "weak_password", auth.ErrInvalidPassword.Error())
	case errors.Is(e, auth.ErrRateLimited):
		writeError(w, 429, "rate_limited", "Too many attempts. Try again later.")
	default:
		writeError(w, 500, "internal_error", "The request could not be completed.")
	}
}

func (h *authHandler) requestVerification(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.originAllowed(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := h.service.RequestEmailVerification(r.Context(), input.Email, rateKey(r)); err != nil {
		h.authError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusAccepted, map[string]any{"message": "If the account can be verified, a secure link will be sent."})
}

func (h *authHandler) requestPasswordReset(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.originAllowed(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := h.service.RequestPasswordReset(r.Context(), input.Email, rateKey(r)); err != nil {
		h.authError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusAccepted, map[string]any{"message": "If the account is eligible, a secure password reset link will be sent."})
}

func (h *authHandler) confirmVerification(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.originAllowed(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := h.service.VerifyEmail(r.Context(), input.Token, rateKey(r)); err != nil {
		h.authError(w, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (h *authHandler) confirmPasswordReset(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.originAllowed(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var input struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := h.service.ResetPassword(r.Context(), input.Token, input.NewPassword, rateKey(r)); err != nil {
		h.authError(w, err)
		return
	}
	h.clearCookie(w)
	w.WriteHeader(stdhttp.StatusNoContent)
}
func (h *authHandler) require(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		c, e := r.Cookie(h.cfg.SessionCookie)
		if e != nil {
			writeError(w, 401, "unauthenticated", "Authentication required.")
			return
		}
		u, e := h.service.Authenticate(r.Context(), c.Value)
		if e != nil {
			h.clearCookie(w)
			writeError(w, 401, "unauthenticated", "Authentication required.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, u)))
	})
}
func (h *authHandler) me(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	u, _ := r.Context().Value(identityKey{}).(auth.SafeUser)
	writeJSON(w, 200, map[string]any{"user": u})
}
func (h *authHandler) logout(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.originAllowed(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	c, _ := r.Cookie(h.cfg.SessionCookie)
	u, _ := r.Context().Value(identityKey{}).(auth.SafeUser)
	if e := h.service.Logout(r.Context(), c.Value, &u.ID); e != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	h.clearCookie(w)
	w.WriteHeader(204)
}

func (h *authHandler) logoutAll(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.originAllowed(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	u, _ := r.Context().Value(identityKey{}).(auth.SafeUser)
	if err := h.service.LogoutEverywhere(r.Context(), u.ID); err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	h.clearCookie(w)
	w.WriteHeader(204)
}

func (h *authHandler) changePassword(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.originAllowed(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decode(w, r, &input) {
		return
	}
	u, _ := r.Context().Value(identityKey{}).(auth.SafeUser)
	if err := h.service.ChangePassword(r.Context(), u.ID, input.CurrentPassword, input.NewPassword); err != nil {
		h.authError(w, err)
		return
	}
	h.clearCookie(w)
	w.WriteHeader(204)
}
