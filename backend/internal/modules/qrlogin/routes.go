package qrlogin

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
)

// RegisterRoutes mounts the QR sign-in endpoints.
func RegisterRoutes(mux *http.ServeMux, h *Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)

	// Web tab (anonymous; the browser secret proves tab ownership).
	mux.HandleFunc("POST /v1/auth/qr/sessions", h.Create)
	mux.HandleFunc("POST /v1/auth/qr/sessions/{id}/state", h.State)
	mux.HandleFunc("POST /v1/auth/qr/exchange", h.Exchange)

	// Signed-in phone: any account approves its own sign-in (workspace
	// members hold no platform permission since cms_user was removed).
	mux.Handle("GET /v1/auth/qr/sessions/{id}", middleware.Chain(http.HandlerFunc(h.Get), authn))
	mux.Handle("POST /v1/auth/qr/sessions/{id}/approve", middleware.Chain(http.HandlerFunc(h.Approve), authn))
	mux.Handle("POST /v1/auth/qr/sessions/{id}/reject", middleware.Chain(http.HandlerFunc(h.Reject), authn))
}
