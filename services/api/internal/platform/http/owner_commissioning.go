package http

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/platform/config"
)

const ownerCommissioningRoot = "/api/personal-execution/commissioning"

// Only tests can substitute this controller. Production construction requires
// the fixed owner domain workflow, never request-selected financial authority.
type ownerCommissioningController interface {
	Review(context.Context, authorization.Principal) (execution.OwnerCommissioningReview, error)
	Read(context.Context, authorization.Principal) (execution.OwnerCommissioningReceipt, error)
	Prepare(context.Context, authorization.Principal, string) (execution.OwnerCommissioningReceipt, error)
	Approve(context.Context, authorization.Principal, string, string, string) (execution.MandateConsent, error)
	Consent(context.Context, authorization.Principal) (execution.OwnerCommissioningConsent, error)
	RevokeInitialConsent(context.Context, authorization.Principal) (execution.OwnerCommissioningConsent, error)
}

// This projection deliberately excludes account/owner/bucket/connection IDs,
// credentials, credential generations and MFA details. Digests bind the exact
// server-selected terms without letting the browser choose authority fields.
type ownerCommissioningView struct {
	TermsDigest           string    `json:"terms_digest"`
	ProductID             string    `json:"product_id"`
	AccountLabel          string    `json:"account_label"`
	AllocationUSD         string    `json:"allocation_usd"`
	MaximumOrderUSD       string    `json:"maximum_order_usd"`
	EffectiveFrom         time.Time `json:"effective_from"`
	ExpiresAt             time.Time `json:"expires_at"`
	ModelID               string    `json:"model_id"`
	Objective             string    `json:"objective"`
	IntervalMinutes       int       `json:"interval_minutes"`
	MaxTradesPerDay       int       `json:"max_trades_per_day"`
	MaxCapitalDeployedUSD string    `json:"max_capital_deployed_usd"`
	MaxSinglePositionUSD  string    `json:"max_single_position_usd"`
	MinimumCashReserveUSD string    `json:"minimum_cash_reserve_usd"`
}

func commissioningView(r execution.OwnerCommissioningReview) ownerCommissioningView {
	t := r.Terms
	return ownerCommissioningView{TermsDigest: r.TermsDigest, ProductID: t.Pilot.ProductID, AccountLabel: "Connected Coinbase portfolio",
		AllocationUSD: t.Pilot.InitialCashUSD, MaximumOrderUSD: t.Pilot.Limits.MaximumOrderUSD,
		EffectiveFrom: t.EffectiveFrom, ExpiresAt: t.Pilot.Limits.ExpiresAt, ModelID: t.AIModelID,
		Objective: t.Objective, IntervalMinutes: t.IntervalMinutes, MaxTradesPerDay: t.MaxTradesPerDay,
		MaxCapitalDeployedUSD: t.MaxCapitalDeployedUSD, MaxSinglePositionUSD: t.MaxSinglePositionUSD,
		MinimumCashReserveUSD: t.MinimumCashReserveUSD}
}

func NewOwnerCommissioningHandler(cfg config.Auth, service *auth.Service, workflow *execution.OwnerCommissioning) stdhttp.Handler {
	var controller ownerCommissioningController
	if workflow != nil {
		controller = workflow
	}
	return newOwnerCommissioningHandler(cfg, service, controller)
}

// Mount outside WithOwnerExecution, which owns the parent namespace. No
// workflow is constructed here and nil is the production default.
func WithOwnerCommissioning(base stdhttp.Handler, cfg config.Config, service *auth.Service, workflow *execution.OwnerCommissioning) stdhttp.Handler {
	owner := NewOwnerCommissioningHandler(cfg.Auth, service, workflow)
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path == ownerCommissioningRoot || strings.HasPrefix(r.URL.Path, ownerCommissioningRoot+"/") {
			owner.ServeHTTP(w, r)
			return
		}
		base.ServeHTTP(w, r)
	})
}

func ownerCommissioningBinding(session, termsDigest string) string {
	digest := sha256.Sum256([]byte("arbion-owner-commissioning-session-v1\x00" + session + "\x00" + termsDigest))
	return fmt.Sprintf("%x", digest)
}

func newOwnerCommissioningHandler(cfg config.Auth, service *auth.Service, workflow ownerCommissioningController) stdhttp.Handler {
	h := &authHandler{service: service, cfg: cfg}
	mux := stdhttp.NewServeMux()
	receipt := func(w stdhttp.ResponseWriter, saved execution.OwnerCommissioningReceipt, err error) {
		if err != nil {
			ownerExecutionError(w, err)
			return
		}
		view := struct {
			ownerCommissioningView
			SnapshotDigest string `json:"snapshot_digest"`
		}{commissioningView(saved.OwnerCommissioningReview), saved.SnapshotDigest}
		writeJSON(w, stdhttp.StatusOK, map[string]any{"receipt": view})
	}
	consent := func(w stdhttp.ResponseWriter, saved execution.OwnerCommissioningConsent, err error) {
		if err != nil {
			ownerExecutionError(w, err)
			return
		}
		writeJSON(w, stdhttp.StatusOK, map[string]any{"consent": saved})
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
	get := func(next stdhttp.HandlerFunc) stdhttp.HandlerFunc {
		return func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			if r.Method != stdhttp.MethodGet {
				w.Header().Set("Allow", stdhttp.MethodGet)
				writeError(w, stdhttp.StatusMethodNotAllowed, "method_not_allowed", "Request method is not allowed.")
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("GET "+ownerCommissioningRoot+"/receipt", get(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		saved, err := workflow.Read(r.Context(), principal(r))
		receipt(w, saved, err)
	}))
	mux.HandleFunc("GET "+ownerCommissioningRoot+"/consent", get(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		saved, err := workflow.Consent(r.Context(), principal(r))
		consent(w, saved, err)
	}))
	mux.HandleFunc("POST "+ownerCommissioningRoot+"/prepare", post(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		var command struct {
			TermsDigest string `json:"expected_terms_digest"`
		}
		if !decodeOwnerExecution(w, r, &command, "expected_terms_digest") {
			return
		}
		saved, err := workflow.Prepare(r.Context(), principal(r), command.TermsDigest)
		receipt(w, saved, err)
	}))
	mux.HandleFunc("POST "+ownerCommissioningRoot+"/approve", post(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		var command struct {
			TermsDigest    string `json:"expected_terms_digest"`
			SnapshotDigest string `json:"expected_snapshot_digest"`
			MFACode        string `json:"mfa_code"`
		}
		if !decodeOwnerExecution(w, r, &command, "expected_terms_digest", "expected_snapshot_digest", "mfa_code") {
			return
		}
		_, err := workflow.Approve(r.Context(), principal(r), command.TermsDigest, command.SnapshotDigest, command.MFACode)
		command.MFACode = ""
		if err != nil {
			ownerExecutionError(w, err)
			return
		}
		saved, err := workflow.Consent(r.Context(), principal(r))
		consent(w, saved, err)
	}))
	mux.HandleFunc("POST "+ownerCommissioningRoot+"/revoke", post(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if !decodeOwnerExecution(w, r, &struct{}{}) {
			return
		}
		saved, err := workflow.RevokeInitialConsent(r.Context(), principal(r))
		consent(w, saved, err)
	}))
	protected := h.require(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		isContext := r.URL.Path == ownerCommissioningRoot+"/context"
		if isContext && r.Method != stdhttp.MethodGet {
			w.Header().Set("Allow", stdhttp.MethodGet)
			writeError(w, stdhttp.StatusMethodNotAllowed, "method_not_allowed", "Request method is not allowed.")
			return
		}
		if workflow == nil {
			if isContext {
				writeJSON(w, stdhttp.StatusOK, map[string]bool{"available": false})
			} else {
				ownerExecutionError(w, nil)
			}
			return
		}
		review, err := workflow.Review(r.Context(), principal(r))
		if err != nil {
			ownerExecutionError(w, err)
			return
		}
		cookie, _ := r.Cookie(cfg.SessionCookie)
		binding := ownerCommissioningBinding(cookie.Value, review.TermsDigest)
		if isContext {
			writeJSON(w, stdhttp.StatusOK, map[string]any{"available": true, "session_binding": binding, "review": commissioningView(review)})
			return
		}
		headers := r.Header.Values("X-Arbion-Execution-Session")
		if len(headers) != 1 || subtle.ConstantTimeCompare([]byte(headers[0]), []byte(binding)) != 1 {
			writeError(w, stdhttp.StatusUnauthorized, "execution_session_changed", "Reopen setup in the current session. Read saved receipts before making another request.")
			return
		}
		mux.ServeHTTP(w, r)
	}))
	return securityHeaders(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if service == nil {
			ownerExecutionError(w, nil)
			return
		}
		protected.ServeHTTP(w, r)
	}))
}
