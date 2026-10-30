package brands

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func RegisterRoutes(r supportops.Router, s *Service) {
	// Existing channel-management permission covers brand identities; no new permission slugs.
	route := func(pattern, perm string, fn supportops.Endpoint) {
		r.Tenant(pattern, perm, func(req *http.Request) (any, error) {
			if raw := req.PathValue("uuid"); raw != "" {
				id, err := uuid.Parse(raw)
				if err != nil || id == uuid.Nil {
					return nil, supportops.ErrInvalid
				}
			}
			if req.Method != "GET" {
				if s.Entitlements == nil {
					return nil, supportops.ErrDenied
				}
				if err := s.Entitlements.RequireWritable(req.Context(), orgctx.Must(req.Context()).ID); err != nil {
					return nil, err
				}
			}
			return fn(req)
		})
	}
	route("GET /v1/tenant/brands/meta", rbac.PermTenantChannelsRead, func(*http.Request) (any, error) { return supportops.Meta("brands"), nil })
	route("GET /v1/tenant/brands", rbac.PermTenantChannelsRead, func(req *http.Request) (any, error) {
		items, err := s.List(req.Context(), orgctx.Must(req.Context()).ID)
		return map[string]any{"items": items}, err
	})
	route("GET /v1/tenant/brands/{uuid}", rbac.PermTenantChannelsRead, func(req *http.Request) (any, error) {
		return s.Get(req.Context(), orgctx.Must(req.Context()).ID, supportops.ID(req))
	})
	save := func(req *http.Request) (any, error) {
		in, err := supportops.Read[Input](req)
		if err != nil {
			return nil, err
		}
		return s.Save(req.Context(), orgctx.Must(req.Context()).ID, supportops.ID(req), in)
	}
	route("POST /v1/tenant/brands", rbac.PermTenantChannelsManage, save)
	route("PUT /v1/tenant/brands/{uuid}", rbac.PermTenantChannelsManage, save)
	route("DELETE /v1/tenant/brands/{uuid}", rbac.PermTenantChannelsManage, func(req *http.Request) (any, error) {
		deleted, err := s.Delete(req.Context(), orgctx.Must(req.Context()).ID, supportops.ID(req))
		return map[string]any{"deleted": deleted}, err
	})
	route("GET /v1/tenant/brands/{uuid}/email-templates", rbac.PermTenantChannelsRead, func(req *http.Request) (any, error) {
		rows, err := s.Templates(req.Context(), orgctx.Must(req.Context()).ID, supportops.ID(req))
		return map[string]any{"items": rows}, err
	})
	route("PUT /v1/tenant/brands/{uuid}/email-templates/{code}", rbac.PermTenantChannelsManage, func(req *http.Request) (any, error) {
		in, err := supportops.Read[EmailTemplateInput](req)
		if err != nil {
			return nil, err
		}
		return s.SaveTemplate(req.Context(), orgctx.Must(req.Context()).ID, supportops.ID(req), req.PathValue("code"), in)
	})
	route("PUT /v1/tenant/brands/{uuid}/articles/{article}", rbac.PermTenantKnowledgeWrite, func(req *http.Request) (any, error) {
		article, err := uuid.Parse(req.PathValue("article"))
		if err != nil {
			return nil, supportops.ErrInvalid
		}
		updated, err := s.AssignArticle(req.Context(), orgctx.Must(req.Context()).ID, supportops.ID(req), article)
		return map[string]any{"updated": updated}, err
	})
	r.Mux.HandleFunc("GET /v1/public/brand-help/{subdomain}", s.publicHelp)
	r.Mux.HandleFunc("GET /v1/public/brand-help/{subdomain}/articles/{article_slug}", s.publicHelp)
	r.Mux.HandleFunc("GET /v1/public/help/{organization_slug}/brand-paths/{help_path}", s.publicHelp)
	r.Mux.HandleFunc("GET /v1/public/help/{organization_slug}/brand-paths/{help_path}/articles/{article_slug}", s.publicHelp)
	r.Mux.HandleFunc("GET /v1/public/help/{organization_slug}/brands/{brand}", s.publicHelp)
	r.Mux.HandleFunc("GET /v1/public/help/{organization_slug}/brands/{brand}/articles/{article_slug}", s.publicHelp)
}
func OptionalUUID(v uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: v, Valid: v != uuid.Nil} }
func (s *Service) publicHelp(w http.ResponseWriter, r *http.Request) {
	out, err := s.PublicHelp(r.Context(), r.PathValue("organization_slug"), r.PathValue("brand"), r.PathValue("subdomain"), r.PathValue("help_path"), r.URL.Query().Get("locale"), r.PathValue("article_slug"), r.URL.Query().Get("q"))
	if err != nil {
		supportops.Fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
