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

	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/platform/config"
)

type identityKey struct{}
type authHandler struct {
	service *auth.Service
	cfg     config.Auth
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

func NewApplicationHandler(database ReadinessChecker, timeout config.Config, service *auth.Service) stdhttp.Handler {
	mux := stdhttp.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /readyz", readiness(database, timeout.Database.ReadinessTimeout))
	h := &authHandler{service: service, cfg: timeout.Auth}
	mux.HandleFunc("POST /api/auth/register", h.register)
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.Handle("POST /api/auth/logout", h.require(stdhttp.HandlerFunc(h.logout)))
	mux.Handle("GET /api/auth/me", h.require(stdhttp.HandlerFunc(h.me)))
	mux.Handle("GET /api/auth/protected-test", h.require(stdhttp.HandlerFunc(h.me)))
	return securityHeaders(mux)
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
	h.setCookie(w, t)
	writeJSON(w, 201, map[string]any{"user": u})
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
	case errors.Is(e, auth.ErrConflict):
		writeError(w, 409, "registration_unavailable", "Unable to create account with those details.")
	case errors.Is(e, auth.ErrInvalidCredentials):
		writeError(w, 401, "invalid_credentials", "Email or password is incorrect.")
	case errors.Is(e, auth.ErrInvalidPassword):
		writeError(w, 400, "weak_password", auth.ErrInvalidPassword.Error())
	case errors.Is(e, auth.ErrRateLimited):
		writeError(w, 429, "rate_limited", "Too many attempts. Try again later.")
	default:
		writeError(w, 500, "internal_error", "The request could not be completed.")
	}
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
