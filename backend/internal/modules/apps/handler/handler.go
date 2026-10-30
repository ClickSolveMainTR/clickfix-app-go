package handler

import (
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/apps/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/apps/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/integrationhttp"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

type Handler struct {
	Service  *usecase.Service
	Activity *activity.Recorder
}

func (h *Handler) JWKS(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, 200, h.Service.JWKS())
}
func (h *Handler) CRUD(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context())
	actor := authctx.MustPrincipal(r.Context())
	id := uuid.Nil
	if raw := r.PathValue("uuid"); raw != "" {
		var err error
		id, err = uuid.Parse(raw)
		if err != nil {
			h.fail(w, r, model.ErrInvalid)
			return
		}
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut:
		var in model.Input
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in) != nil {
			h.fail(w, r, model.ErrInvalid)
			return
		}
		row, err := h.Service.Save(r.Context(), org.ID, actor.UserInternal, id, in)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.Activity.Record(r.Context(), &actor.UserInternal, "updated", "settings", &row.Uuid, map[string]any{"sidebar_app": true, "enabled": row.Enabled}, r)
		status := 200
		if id == uuid.Nil {
			status = 201
		}
		response.JSON(w, r, status, row)
	case http.MethodDelete:
		n, err := h.Service.Store.SidebarAppDelete(r.Context(), db.SidebarAppDeleteParams{OrganizationID: org.ID, Uuid: id})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		if n == 0 {
			response.NotFound(w, r, "")
			return
		}
		h.Activity.Record(r.Context(), &actor.UserInternal, "updated", "settings", &id, map[string]any{"sidebar_app_deleted": true}, r)
		response.JSON(w, r, 200, map[string]bool{"deleted": true})
	default:
		if id != uuid.Nil {
			row, err := h.Service.Store.SidebarAppGet(r.Context(), db.SidebarAppGetParams{OrganizationID: org.ID, Uuid: id})
			if err != nil {
				h.fail(w, r, err)
				return
			}
			response.JSON(w, r, 200, row)
			return
		}
		q, ok := integrationhttp.Query(w, r, false)
		if !ok {
			return
		}
		rows, err := h.Service.Store.SidebarAppList(r.Context(), db.SidebarAppListParams{OrganizationID: org.ID, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		n, err := h.Service.Store.SidebarAppCount(r.Context(), org.ID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, apiquery.NewPage(rows, n, q.Limit, q.Offset))
	}
}
func (h *Handler) Launch(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		h.fail(w, r, model.ErrInvalid)
		return
	}
	var in model.LaunchInput
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
		h.fail(w, r, model.ErrInvalid)
		return
	}
	out, err := h.Service.Launch(r.Context(), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, 200, out)
}
func (h *Handler) Action(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		h.fail(w, r, model.ErrInvalid)
		return
	}
	var in model.ActionInput
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 20000)).Decode(&in) != nil {
		h.fail(w, r, model.ErrInvalid)
		return
	}
	out, err := h.Service.Action(r.Context(), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, model.ErrInvalid):
		response.BadRequest(w, r, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, model.ErrConflict):
		response.Conflict(w, r, "CONFLICT", "Sidebar app revision changed")
	case errors.Is(err, model.ErrDenied):
		response.Forbidden(w, r, "")
	case usecase.IsMissing(err):
		response.NotFound(w, r, "")
	default:
		response.InternalErr(w, r, err, "")
	}
}

func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, 200, resourcemeta.ResourceMeta{Resource: "tenant.apps", DefaultFields: []string{"uuid", "name", "url", "placement", "scopes", "enabled"}, Capabilities: resourcemeta.Capabilities{Create: true, Read: true, Update: true, Delete: true}, SearchableFields: []string{}, SortableFields: []string{}, FilterableFields: []string{}, Columns: []resourcemeta.Column{}, Filters: []resourcemeta.Filter{}, Includes: []string{}})
}
