package activity

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	activityhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/activity/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

func RegisterRoutes(
	mux *http.ServeMux,
	h *activityhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}

	mux.Handle("GET /v1/platform/activity/meta", middleware.Chain(
		http.HandlerFunc(h.Meta), authn, require(rbac.PermPlatformActivityRead),
	))
	mux.Handle("GET /v1/platform/activity", middleware.Chain(
		http.HandlerFunc(h.List), authn, require(rbac.PermPlatformActivityRead),
	))
}
