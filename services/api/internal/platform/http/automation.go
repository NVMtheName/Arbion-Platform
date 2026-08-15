package http

import (
	"errors"
	stdhttp "net/http"
	"strconv"

	"github.com/arbion/platform/services/api/internal/automation"
)

func registerAutomationRoutes(m *stdhttp.ServeMux, h *authHandler) {
	if h.automation == nil {
		return
	}
	m.Handle("GET /api/capital-buckets", h.require(stdhttp.HandlerFunc(h.listBuckets)))
	m.Handle("POST /api/capital-buckets", h.require(stdhttp.HandlerFunc(h.createBucket)))
	m.Handle("GET /api/capital-buckets/{id}", h.require(stdhttp.HandlerFunc(h.getBucket)))
	m.Handle("PATCH /api/capital-buckets/{id}", h.require(stdhttp.HandlerFunc(h.updateBucket)))
	m.Handle("DELETE /api/capital-buckets/{id}", h.require(stdhttp.HandlerFunc(h.deleteBucket)))
	m.Handle("GET /api/automations", h.require(stdhttp.HandlerFunc(h.listAutomations)))
	m.Handle("POST /api/automations", h.require(stdhttp.HandlerFunc(h.createAutomation)))
	m.Handle("GET /api/automations/{id}", h.require(stdhttp.HandlerFunc(h.getAutomation)))
	m.Handle("PATCH /api/automations/{id}", h.require(stdhttp.HandlerFunc(h.updateAutomation)))
	for _, x := range []string{"ready", "pause", "disable", "archive"} {
		m.Handle("POST /api/automations/{id}/"+x, h.require(stdhttp.HandlerFunc(h.transitionAutomation)))
	}
	m.Handle("GET /api/automations/{id}/versions", h.require(stdhttp.HandlerFunc(h.versions)))
	m.Handle("GET /api/automations/{id}/versions/{version}", h.require(stdhttp.HandlerFunc(h.version)))
	m.Handle("GET /api/automation-strategies", h.require(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		writeJSON(w, 200, map[string]any{"strategies": automation.Strategies, "execution_capable": false})
	})))
}
func (h *authHandler) automationError(w stdhttp.ResponseWriter, e error) {
	switch {
	case errors.Is(e, automation.ErrForbidden):
		writeError(w, 403, "PERMISSION_DENIED", "Automation entitlement is required.")
	case errors.Is(e, automation.ErrNotFound):
		writeError(w, 404, "NOT_FOUND", "The requested resource was not found.")
	case errors.Is(e, automation.ErrConflict):
		writeError(w, 409, "VERSION_CONFLICT", "The resource changed or has durable dependencies.")
	case errors.Is(e, automation.ErrInvalid):
		writeError(w, 422, "INVALID_AUTOMATION", "The automation configuration is invalid.")
	default:
		writeError(w, 500, "INTERNAL_ERROR", "The request could not be completed.")
	}
}
func (h *authHandler) autoMutation(w stdhttp.ResponseWriter, r *stdhttp.Request, fn func() error) {
	if !h.csrf(r) {
		writeError(w, 403, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	if e := fn(); e != nil {
		h.automationError(w, e)
	}
}
func (h *authHandler) createBucket(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	h.autoMutation(w, r, func() error {
		var x automation.CreateBucketCommand
		if !decode(w, r, &x) {
			return nil
		}
		b, e := h.automation.CreateBucket(r.Context(), principal(r), x)
		if e == nil {
			writeJSON(w, 201, map[string]any{"capital_bucket": b})
		}
		return e
	})
}
func (h *authHandler) listBuckets(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.automation.ListBuckets(r.Context(), principal(r))
	if e != nil {
		h.automationError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"capital_buckets": nonNil(v)})
}
func (h *authHandler) getBucket(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.automation.GetBucket(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.automationError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"capital_bucket": v})
}
func (h *authHandler) updateBucket(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	h.autoMutation(w, r, func() error {
		var x automation.CreateBucketCommand
		if !decode(w, r, &x) {
			return nil
		}
		b, e := h.automation.UpdateBucket(r.Context(), principal(r), r.PathValue("id"), x)
		if e == nil {
			writeJSON(w, 200, map[string]any{"capital_bucket": b})
		}
		return e
	})
}
func (h *authHandler) deleteBucket(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	h.autoMutation(w, r, func() error {
		e := h.automation.DeleteBucket(r.Context(), principal(r), r.PathValue("id"))
		if e == nil {
			w.WriteHeader(204)
		}
		return e
	})
}

type mandateInput struct {
	automation.MandateCommand
	ExpectedVersion int `json:"expected_version"`
}

func (h *authHandler) createAutomation(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	h.autoMutation(w, r, func() error {
		var x automation.MandateCommand
		if !decode(w, r, &x) {
			return nil
		}
		v, e := h.automation.Create(r.Context(), principal(r), x)
		if e == nil {
			writeJSON(w, 201, map[string]any{"automation": v})
		}
		return e
	})
}
func (h *authHandler) listAutomations(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.automation.List(r.Context(), principal(r))
	if e != nil {
		h.automationError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"automations": nonNil(v), "execution_enabled": false})
}
func (h *authHandler) getAutomation(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.automation.Get(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.automationError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"automation": v, "execution_enabled": false})
}
func (h *authHandler) updateAutomation(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	h.autoMutation(w, r, func() error {
		var x mandateInput
		if !decode(w, r, &x) {
			return nil
		}
		v, e := h.automation.Update(r.Context(), principal(r), r.PathValue("id"), x.ExpectedVersion, x.MandateCommand)
		if e == nil {
			writeJSON(w, 200, map[string]any{"automation": v})
		}
		return e
	})
}
func (h *authHandler) transitionAutomation(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	h.autoMutation(w, r, func() error {
		var x struct {
			ExpectedVersion int `json:"expected_version"`
		}
		if !decode(w, r, &x) {
			return nil
		}
		status := map[string]string{"ready": "READY", "pause": "PAUSED", "disable": "DISABLED", "archive": "ARCHIVED"}[r.PathValue("action")]
		if status == "" {
			for k, v := range map[string]string{"/ready": "READY", "/pause": "PAUSED", "/disable": "DISABLED", "/archive": "ARCHIVED"} {
				if len(r.URL.Path) >= len(k) && r.URL.Path[len(r.URL.Path)-len(k):] == k {
					status = v
				}
			}
		}
		v, e := h.automation.Transition(r.Context(), principal(r), r.PathValue("id"), x.ExpectedVersion, status)
		if e == nil {
			writeJSON(w, 200, map[string]any{"automation": v, "execution_enabled": false})
		}
		return e
	})
}
func (h *authHandler) versions(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, e := h.automation.Versions(r.Context(), principal(r), r.PathValue("id"))
	if e != nil {
		h.automationError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"versions": nonNil(v)})
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
func (h *authHandler) version(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	n, e := strconv.Atoi(r.PathValue("version"))
	if e != nil {
		h.automationError(w, automation.ErrInvalid)
		return
	}
	v, e := h.automation.Version(r.Context(), principal(r), r.PathValue("id"), n)
	if e != nil {
		h.automationError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"version": v})
}
