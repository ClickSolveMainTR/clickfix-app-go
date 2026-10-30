// Package helpcenter mounts the public, unauthenticated help center API
// (CF-74): published public knowledge articles per organization.
package helpcenter

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/helpcenter/handler"
)

// RegisterRoutes mounts /v1/public/help/{organization_slug}/...
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler) {
	mux.HandleFunc("GET /v1/public/help/{organization_slug}", h.Home)
	mux.HandleFunc("GET /v1/public/help/{organization_slug}/logo", h.Logo)
	mux.HandleFunc("GET /v1/public/help/{organization_slug}/sitemap", h.Sitemap)
	mux.HandleFunc("GET /v1/public/help/{organization_slug}/search", h.Search)
	mux.HandleFunc("GET /v1/public/help/{organization_slug}/collections/{collection_slug}", h.Collection)
	mux.HandleFunc("GET /v1/public/help/{organization_slug}/articles/{article_slug}", h.Article)
	mux.HandleFunc("POST /v1/public/help/{organization_slug}/articles/{article_slug}/feedback", h.Feedback)
}
