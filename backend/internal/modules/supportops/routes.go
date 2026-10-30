package supportops

import (
	"encoding/json"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"io"
	"net/http"
	"strings"
	"time"
)

type Router struct {
	Mux      *http.ServeMux
	Tokens   *jwt.Manager
	Loader   middleware.IdentityLoader
	Resolver orgctx.Resolver
	Activity *activity.Recorder
}
type Endpoint func(*http.Request) (any, error)

func (r Router) Tenant(pattern, perm string, fn Endpoint) {
	if r.Resolver == nil {
		return
	}
	r.Mux.Handle(pattern, middleware.Chain(r.Handler(fn, true), middleware.Authenticate(r.Tokens, r.Loader), middleware.RequireOrganization(r.Resolver), middleware.RequireTenantPermission(perm)))
}
func (r Router) Platform(pattern, perm string, fn Endpoint) {
	r.Mux.Handle(pattern, middleware.Chain(r.Handler(fn, true), middleware.Authenticate(r.Tokens, r.Loader), middleware.RequirePermission(perm)))
}
func (r Router) Handler(fn Endpoint, audit bool) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		out, err := fn(req)
		if err != nil {
			Fail(w, req, err)
			return
		}
		if audit && req.Method != http.MethodGet {
			Record(r.Activity, req, "support_operations.updated", nil)
		}
		response.JSON(w, req, 200, out)
	}
}
func Read[T any](r *http.Request) (T, error) {
	var in T
	b, err := io.ReadAll(io.LimitReader(r.Body, 64*1024+1))
	if err != nil || len(b) > 64*1024 {
		return in, ErrInvalid
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		return in, ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return in, ErrInvalid
	}
	return in, nil
}
func List(r *http.Request) (apiquery.Query, error) {
	q := apiquery.Parse(r.URL.Query())
	for _, s := range q.Sort {
		if s.Field != "created_at" || !s.Desc {
			return q, ErrInvalid
		}
	}
	return q, nil
}
func Window(r *http.Request) (time.Time, time.Time, error) {
	a, err := time.Parse(time.RFC3339, r.URL.Query().Get("since"))
	if err != nil {
		return a, time.Time{}, ErrInvalid
	}
	b, err := time.Parse(time.RFC3339, r.URL.Query().Get("until"))
	if err != nil || !b.After(a) || b.Sub(a) > 366*24*time.Hour {
		return a, b, ErrInvalid
	}
	return a, b, nil
}
func Page(items any, total int64, q apiquery.Query) any {
	return map[string]any{"items": items, "total": total, "limit": q.Limit, "offset": q.Offset}
}

// Meta returns only capabilities implemented by this extension's list surfaces.
func Meta(resource string) any {
	return resourcemeta.ResourceMeta{Resource: resource, DefaultSort: "-created_at", DefaultFields: []string{"uuid"}, Capabilities: resourcemeta.Capabilities{Read: true, Create: true}, SearchableFields: []string{}, SortableFields: []string{}, FilterableFields: []string{}, Includes: []string{}, Filters: []resourcemeta.Filter{}, Columns: []resourcemeta.Column{{Key: "uuid", LabelKey: "users.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: true}}}
}
