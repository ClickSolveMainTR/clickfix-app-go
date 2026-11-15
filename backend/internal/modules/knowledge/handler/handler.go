// Package handler exposes knowledge authoring APIs behind org settings permissions.
// Raw rich-text bodies are data; a future public help center must sanitize HTML on render.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/apierror"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/deptweb"
	deptmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/model"
	deptusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/deptscope"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Handler struct {
	Service  *usecase.Service
	RAG      *usecase.RAG
	Activity *activity.Recorder
}

func Fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, model.ErrNotFound):
		response.NotFound(w, r, "Article or collection not found")
	case errors.Is(err, model.ErrConflict):
		response.Conflict(w, r, "KNOWLEDGE_CONFLICT", "Knowledge revision or relationship conflict")
	case errors.Is(err, model.ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Field: "input", Code: "invalid", Message: "Invalid knowledge input"}})
	case apierror.Write(w, r, err):
	default:
		response.InternalErr(w, r, err, "")
	}
}

func Decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 512<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		Fail(w, r, model.ErrInvalid)
		return false
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		Fail(w, r, model.ErrInvalid)
		return false
	}
	return true
}

func id(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	out, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		Fail(w, r, model.ErrInvalid)
		return out, false
	}
	return out, true
}

func (h *Handler) audit(r *http.Request, operation string, id uuid.UUID) {
	p := authctx.MustPrincipal(r.Context())
	scope := orgctx.Must(r.Context())
	if h.Activity != nil {
		h.Activity.Record(r.Context(), &p.UserInternal, "organizations.updated", "organizations", &scope.UUID, map[string]any{"area": "knowledge", "operation": operation, "entity_uuid": id.String()}, r)
	}
}

func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, 200, resourcemeta.ResourceMeta{Resource: "tenant.knowledge.articles", DefaultSort: "-updated_at", DefaultFields: []string{"uuid", "title", "locale", "version", "published_version"}, Capabilities: resourcemeta.Capabilities{Create: true, Read: true, Update: true, Delete: true, Search: true}, SearchableFields: []string{"title"}, Columns: []resourcemeta.Column{{Key: "title", LabelKey: "roles.name", Type: resourcemeta.ColumnTypeString, DefaultVisible: true}}})
}

func (h *Handler) Collections(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	if r.Method == http.MethodGet {
		q := apiquery.Parse(r.URL.Query())
		if len(apiquery.ForbiddenParams(r.URL.Query())) > 0 || len(q.Sort) > 0 || q.Q != "" {
			Fail(w, r, model.ErrInvalid)
			return
		}
		rows, total, err := h.Service.Collections(r.Context(), org, q.Limit, q.Offset)
		if err != nil {
			Fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, apiquery.NewPage(rows, total, q.Limit, q.Offset))
		return
	}
	key := uuid.New()
	if r.PathValue("uuid") != "" {
		var ok bool
		key, ok = id(w, r)
		if !ok {
			return
		}
	}
	if r.Method == http.MethodDelete {
		if err := h.Service.DeleteCollection(r.Context(), org, key); err != nil {
			Fail(w, r, err)
			return
		}
		h.audit(r, "collection_deleted", key)
		response.JSON(w, r, 200, map[string]bool{"deleted": true})
		return
	}
	if r.Method == http.MethodPut {
		if _, err := h.Service.Collection(r.Context(), org, key); err != nil {
			Fail(w, r, err)
			return
		}
	}
	var in model.CollectionInput
	if !Decode(w, r, &in) {
		return
	}
	row, err := h.Service.SaveCollection(r.Context(), org, key, in)
	if err != nil {
		Fail(w, r, err)
		return
	}
	h.audit(r, "collection_saved", key)
	response.JSON(w, r, 200, row)
}

func (h *Handler) Articles(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	if r.Method == http.MethodGet {
		q := apiquery.Parse(r.URL.Query())
		if len(apiquery.ForbiddenParams(r.URL.Query())) > 0 || len(q.Sort) > 0 {
			Fail(w, r, model.ErrInvalid)
			return
		}
		values := r.URL.Query()
		filter := usecase.ArticleFilter{Q: q.Q, Locale: values.Get("locale"), Visibility: values.Get("visibility"), State: values.Get("state")}
		if raw := values.Get("collection_uuid"); raw != "" {
			c, err := uuid.Parse(raw)
			if err != nil {
				Fail(w, r, model.ErrInvalid)
				return
			}
			filter.Collection = &c
		}
		// CF-215: the member's departments and the department filter
		// ("none" = organization-wide, or a department uuid).
		dscope, err := deptweb.Scope(r, h.Service.Repo.Q)
		if err != nil {
			Fail(w, r, err)
			return
		}
		filter.Scope = dscope
		switch raw := values.Get("department"); raw {
		case "":
		case "none":
			filter.DepartmentMode = "none"
		default:
			d, err := uuid.Parse(raw)
			if err != nil {
				Fail(w, r, model.ErrInvalid)
				return
			}
			dept, err := deptweb.Resolve(r, h.Service.Repo.Q, deptscope.All(), deptscope.Optional{Set: true, Value: &d})
			if err != nil {
				if !deptweb.Fail(w, r, err) {
					Fail(w, r, err)
				}
				return
			}
			filter.DepartmentMode, filter.DepartmentID = "id", dept
		}
		rows, total, err := h.Service.Articles(r.Context(), scope.ID, filter, q.Limit, q.Offset)
		if err != nil {
			Fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, apiquery.NewPage(rows, total, q.Limit, q.Offset))
		return
	}
	var in model.ArticleInput
	if !Decode(w, r, &in) || !h.department(w, r, &in) {
		return
	}
	key := uuid.New()
	row, err := h.Service.SaveArticle(r.Context(), scope.ID, authctx.MustPrincipal(r.Context()).UserInternal, key, in, true)
	if err != nil {
		Fail(w, r, err)
		return
	}
	h.audit(r, "article_created", key)
	h.writeArticle(w, r, 201, row)
}

func (h *Handler) Article(w http.ResponseWriter, r *http.Request) {
	key, ok := id(w, r)
	if !ok {
		return
	}
	org := orgctx.Must(r.Context()).ID
	if !h.visible(w, r, key) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		row, err := h.Service.Article(r.Context(), org, key)
		if err != nil {
			Fail(w, r, err)
			return
		}
		h.writeArticle(w, r, 200, row)
	case http.MethodDelete:
		if err := h.Service.DeleteArticle(r.Context(), org, key); err != nil {
			Fail(w, r, err)
			return
		}
		h.audit(r, "article_deleted", key)
		response.JSON(w, r, 200, map[string]bool{"deleted": true})
	case http.MethodPut:
		var in model.ArticleInput
		if !Decode(w, r, &in) || !h.department(w, r, &in) {
			return
		}
		row, err := h.Service.SaveArticle(r.Context(), org, authctx.MustPrincipal(r.Context()).UserInternal, key, in, false)
		if err != nil {
			Fail(w, r, err)
			return
		}
		h.audit(r, "article_updated", key)
		h.writeArticle(w, r, 200, row)
	}
}

// Audience reads (GET) or replaces (PUT) who the widget offers an article
// to, as custom field catalog conditions (CF-196).
func (h *Handler) Audience(w http.ResponseWriter, r *http.Request) {
	key, ok := id(w, r)
	if !ok {
		return
	}
	if !h.visible(w, r, key) {
		return
	}
	org := orgctx.Must(r.Context()).ID
	if r.Method == http.MethodGet {
		out, err := h.Service.ArticleAudience(r.Context(), org, key)
		if err != nil {
			Fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, out)
		return
	}
	var in usecase.Audience
	if !Decode(w, r, &in) {
		return
	}
	out, err := h.Service.SetArticleAudience(r.Context(), org, key, in)
	if err != nil {
		if errors.Is(err, model.ErrInvalid) && err != model.ErrInvalid {
			response.ValidationError(w, r, []response.Detail{{Field: "conditions", Code: "invalid", Message: strings.TrimPrefix(err.Error(), model.ErrInvalid.Error()+": ")}})
			return
		}
		Fail(w, r, err)
		return
	}
	h.audit(r, "article_audience_updated", key)
	response.JSON(w, r, 200, out)
}

func (h *Handler) Versions(w http.ResponseWriter, r *http.Request) {
	key, ok := id(w, r)
	if !ok {
		return
	}
	if !h.visible(w, r, key) {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	if len(apiquery.ForbiddenParams(r.URL.Query())) > 0 || len(q.Sort) > 0 || q.Q != "" {
		Fail(w, r, model.ErrInvalid)
		return
	}
	rows, total, err := h.Service.Versions(r.Context(), orgctx.Must(r.Context()).ID, key, q.Limit, q.Offset)
	if err != nil {
		Fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, apiquery.NewPage(rows, total, q.Limit, q.Offset))
}

func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	key, ok := id(w, r)
	if !ok {
		return
	}
	if !h.visible(w, r, key) {
		return
	}
	var in struct {
		Version int32 `json:"version"`
		Publish bool  `json:"publish"`
	}
	if !Decode(w, r, &in) {
		return
	}
	if err := h.Service.Publish(r.Context(), orgctx.Must(r.Context()).ID, key, in.Version, in.Publish); err != nil {
		Fail(w, r, err)
		return
	}
	h.audit(r, "article_publication_changed", key)
	response.JSON(w, r, 200, map[string]bool{"published": in.Publish})
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	locale := r.URL.Query().Get("locale")
	if locale == "" {
		locale = "tr"
	}
	dscope, err := deptweb.Scope(r, h.Service.Repo.Q)
	if err != nil {
		Fail(w, r, err)
		return
	}
	rows, err := h.RAG.Search(deptscope.With(r.Context(), dscope), orgctx.Must(r.Context()).ID, r.URL.Query().Get("q"), locale, r.URL.Query().Get("public_only") == "true")
	if err != nil {
		Fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, rows)
}

// visible answers 404 for an article of a department the member does not
// see (CF-215).
func (h *Handler) visible(w http.ResponseWriter, r *http.Request, key uuid.UUID) bool {
	org := orgctx.Must(r.Context()).ID
	a, err := h.Service.Repo.Q.KnowledgeArticleDepartment(r.Context(), db.KnowledgeArticleDepartmentParams{OrganizationID: org, Uuid: key})
	if errors.Is(err, pgx.ErrNoRows) {
		Fail(w, r, model.ErrNotFound)
		return false
	}
	if err != nil {
		Fail(w, r, err)
		return false
	}
	dscope, err := deptweb.Scope(r, h.Service.Repo.Q)
	if err != nil {
		Fail(w, r, err)
		return false
	}
	seen, err := deptusecase.Sees(r.Context(), h.Service.Repo.Q, org, dscope, deptmodel.KindArticle, a.ID, a.DepartmentID)
	if err != nil {
		Fail(w, r, err)
		return false
	}
	if !seen {
		Fail(w, r, model.ErrNotFound)
	}
	return seen
}

// department resolves the department_uuid of an article write: members
// without read_all file articles under their own departments only.
func (h *Handler) department(w http.ResponseWriter, r *http.Request, in *model.ArticleInput) bool {
	dscope, err := deptweb.Scope(r, h.Service.Repo.Q)
	if err == nil {
		in.DepartmentID, err = deptweb.Resolve(r, h.Service.Repo.Q, dscope, in.Department)
	}
	if err != nil {
		if !deptweb.Fail(w, r, err) {
			Fail(w, r, err)
		}
		return false
	}
	return true
}

// writeArticle answers an article with its department reference (CF-215).
func (h *Handler) writeArticle(w http.ResponseWriter, r *http.Request, status int, row db.KnowledgeArticle) {
	rows, err := deptweb.Annotate(r.Context(), h.Service.Repo.Q, orgctx.Must(r.Context()).ID, []db.KnowledgeArticle{row}, func(a db.KnowledgeArticle) pgtype.Int8 { return a.DepartmentID })
	if err != nil {
		Fail(w, r, err)
		return
	}
	response.JSON(w, r, status, rows[0])
}
