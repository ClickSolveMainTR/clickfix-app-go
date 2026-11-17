package desktoplogin

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
)

// RegisterRoutes mounts the desktop sign-in endpoints.
func RegisterRoutes(mux *http.ServeMux, h *Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)

	// Signed-in browser (/desktop/login page, through the panel BFF).
	mux.Handle("POST /v1/auth/desktop/codes", middleware.Chain(http.HandlerFunc(h.CreateCode), authn))
	// Desktop app (native client, /api/mobile passthrough).
	mux.HandleFunc("POST /v1/auth/desktop/exchange", h.Exchange)
	// Web server only (/desktop/session route): session for the web view.
	mux.Handle("POST /v1/auth/desktop/web-session", middleware.Chain(http.HandlerFunc(h.WebSession), authn))
}
