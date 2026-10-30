package handler

import (
	"net/http"
	"strconv"
	"strings"

	searchusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/search/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

type Handler struct {
	svc *searchusecase.Service
}

func New(svc *searchusecase.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Specs(w http.ResponseWriter, r *http.Request) {
	specs := h.svc.ListSpecs(r.Context())
	response.JSON(w, r, http.StatusOK, map[string]any{
		"items":   specs,
		"enabled": h.svc.Enabled(),
	})
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	spec := strings.TrimSpace(r.URL.Query().Get("spec"))
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	items := h.svc.Search(r.Context(), q, spec, limit)
	if items == nil {
		items = []searchengine.Hit{}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// TenantSpecs lists the organization search specs of the caller (CF-45).
func (h *Handler) TenantSpecs(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	specs := h.svc.TenantSpecs(scope)
	if specs == nil {
		specs = []searchengine.Spec{}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{
		"items":   specs,
		"enabled": h.svc.Enabled(),
	})
}

// TenantSearch searches the organization's conversations, messages,
// contacts and companies. Hits never cross organizations.
func (h *Handler) TenantSearch(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	spec := strings.TrimSpace(r.URL.Query().Get("spec"))
	p := authctx.MustPrincipal(r.Context())
	items := h.svc.TenantSearch(r.Context(), scope, p.UserInternal, q, spec, parseLimit(r.URL.Query().Get("limit")))
	if items == nil {
		items = []searchengine.Hit{}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func parseLimit(raw string) int {
	limit := searchengine.DefaultLimit
	if raw = strings.TrimSpace(raw); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	return limit
}
