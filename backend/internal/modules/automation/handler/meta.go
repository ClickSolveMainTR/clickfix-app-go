package handler

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"net/http"
)

func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, 200, resourcemeta.ResourceMeta{
		Resource: "tenant.automations", DefaultFields: []string{"uuid", "name", "trigger_event", "enabled", "position"}, Capabilities: resourcemeta.Capabilities{Create: true, Read: true, Update: true, Delete: true},
		SearchableFields: []string{}, SortableFields: []string{}, FilterableFields: []string{}, Columns: []resourcemeta.Column{}, Filters: []resourcemeta.Filter{}, Includes: []string{},
	})
}
