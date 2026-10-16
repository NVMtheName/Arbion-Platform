package http

import (
	stdhttp "net/http"

	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/platform/config"
)

// WithOwnerRuntime mounts the two existing authenticated control namespaces
// from one fixed runtime. Commissioning must own its child namespace before
// owner execution sees the parent. Nil stays unavailable. This HTTP composition
// never starts or invokes the worker, including after successful owner consent.
func WithOwnerRuntime(base stdhttp.Handler, cfg config.Config, service *auth.Service, runtime *execution.OwnerRuntime) stdhttp.Handler {
	return WithOwnerCommissioning(WithOwnerExecution(base, cfg, service, runtime.Owner()), cfg, service, runtime.Commissioning())
}
