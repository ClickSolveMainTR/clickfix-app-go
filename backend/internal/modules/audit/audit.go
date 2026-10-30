// Package audit is the organization audit log (CF-12): who changed what and
// when inside one organization, built on activity_events stamped by
// activity.Recorder from the request's organization scope.
//
//	GET /v1/tenant/audit/meta     table contract
//	GET /v1/tenant/audit/facets   resources / actions in use (filter options)
//	GET /v1/tenant/audit          list (actor, resource, action, from, to, q)
//	GET /v1/tenant/audit/export   CSV download of the same filters (max 10 000 rows)
package audit

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Resource is the meta slug of the audit list.
const Resource = "tenant.audit"

// MaxExportRows bounds a CSV export.
const MaxExportRows = 10000

// Actor is the person behind an event (joined at read time; never stored in
// the payload).
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Email   string    `json:"email"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
}

// Event is one audit row.
type Event struct {
	UUID         uuid.UUID      `json:"uuid"`
	Action       string         `json:"action"`
	Resource     string         `json:"resource"`
	ResourceUUID *uuid.UUID     `json:"resource_uuid"`
	Payload      map[string]any `json:"payload"`
	Actor        *Actor         `json:"actor"`
	IPAddress    *string        `json:"ip_address"`
	CreatedAt    time.Time      `json:"created_at"`
}

// Facet is a resource with the actions recorded for it.
type Facet struct {
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}

// Filter narrows the list.
type Filter struct {
	ActorUUID uuid.UUID
	Resources []string
	Actions   []string
	From, To  *time.Time
	Q         string
	Desc      bool
	Limit     int32
	Offset    int32
}

// Service reads the audit log.
type Service struct{ q *db.Queries }

// NewService builds the service.
func NewService(q *db.Queries) *Service { return &Service{q: q} }

// List returns one page of the organization's events.
func (s *Service) List(ctx context.Context, orgID int64, f Filter) ([]Event, int64, error) {
	rows, err := s.q.ListOrganizationAuditEvents(ctx, db.ListOrganizationAuditEventsParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ActorUuid: pgUUID(f.ActorUUID), Resources: nilIfEmpty(f.Resources), Actions: nilIfEmpty(f.Actions),
		FromAt: pgTime(f.From), ToAt: pgTime(f.To), Q: pgText(f.Q), SortDesc: f.Desc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountOrganizationAuditEvents(ctx, db.CountOrganizationAuditEventsParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ActorUuid: pgUUID(f.ActorUUID), Resources: nilIfEmpty(f.Resources), Actions: nilIfEmpty(f.Actions),
		FromAt: pgTime(f.From), ToAt: pgTime(f.To), Q: pgText(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		e := Event{UUID: r.Uuid, Action: r.Action, Resource: r.Resource, CreatedAt: r.CreatedAt.Time.UTC(), Payload: map[string]any{}}
		_ = json.Unmarshal(r.Payload, &e.Payload)
		if e.Payload == nil {
			e.Payload = map[string]any{}
		}
		if r.ResourceUuid.Valid {
			id := uuid.UUID(r.ResourceUuid.Bytes)
			e.ResourceUUID = &id
		}
		if r.ActorUuid.Valid {
			e.Actor = &Actor{UUID: r.ActorUuid.Bytes, Email: r.ActorEmail, Name: r.ActorName, Surname: r.ActorSurname}
		}
		if r.IpAddress != nil {
			ip := r.IpAddress.String()
			e.IPAddress = &ip
		}
		out = append(out, e)
	}
	return out, total, nil
}

// Facets lists the resources and actions recorded for the organization.
func (s *Service) Facets(ctx context.Context, orgID int64) ([]Facet, error) {
	rows, err := s.q.ListOrganizationAuditFacets(ctx, pgtype.Int8{Int64: orgID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := []Facet{}
	for _, r := range rows {
		if len(out) == 0 || out[len(out)-1].Resource != r.Resource {
			out = append(out, Facet{Resource: r.Resource, Actions: []string{}})
		}
		out[len(out)-1].Actions = append(out[len(out)-1].Actions, r.Action)
	}
	return out, nil
}

// --- HTTP ------------------------------------------------------------------

// Handler serves the audit routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the audit routes (nil resolver: nothing).
func RegisterRoutes(mux *http.ServeMux, h *Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantAuditRead)
	export := middleware.RequireTenantPermission(rbac.PermTenantAuditExport)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	route("GET /v1/tenant/audit/meta", h.Meta, read)
	route("GET /v1/tenant/audit/facets", h.Facets, read)
	route("GET /v1/tenant/audit", h.List, read)
	route("GET /v1/tenant/audit/export", h.Export, read, export)
}

// Meta is the table contract of GET /v1/tenant/audit.
func Meta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      Resource,
		DefaultSort:   "-created_at",
		DefaultFields: []string{"created_at", "actor", "action", "resource", "payload"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Search: true, Filter: true, Sort: true, Export: true,
		},
		SearchableFields: []string{"action", "resource"},
		SortableFields:   []string{"created_at"},
		FilterableFields: []string{"actor", "resource", "action", "from", "to"},
		Columns: []resourcemeta.Column{
			{Key: "created_at", LabelKey: "audit.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
			{Key: "actor", LabelKey: "audit.columns.actor", Type: resourcemeta.ColumnTypeString, Filterable: true, DefaultVisible: true},
			{Key: "action", LabelKey: "audit.columns.action", Type: resourcemeta.ColumnTypeEnum, Filterable: true, DefaultVisible: true},
			{Key: "resource", LabelKey: "audit.columns.resource", Type: resourcemeta.ColumnTypeEnum, Filterable: true, DefaultVisible: true},
			{Key: "payload", LabelKey: "audit.columns.payload", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "ip_address", LabelKey: "audit.columns.ip_address", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
		},
		Filters:     []resourcemeta.Filter{},
		Includes:    []string{"actor"},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// Meta serves GET /v1/tenant/audit/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/audit/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Facets(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// List serves GET /v1/tenant/audit.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	f, q, ok := parseFilter(w, r)
	if !ok {
		return
	}
	f.Limit, f.Offset = q.Limit, q.Offset
	items, total, err := h.svc.List(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Export serves GET /v1/tenant/audit/export: a CSV file of the filtered
// events (newest first, at most MaxExportRows).
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	f, _, ok := parseFilter(w, r)
	if !ok {
		return
	}
	scope := orgctx.Must(r.Context())
	f.Limit, f.Offset = MaxExportRows, 0
	items, _, err := h.svc.List(r.Context(), scope.ID, f)
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	name := fmt.Sprintf("audit-%s-%s.csv", scope.Slug, time.Now().UTC().Format("20060102-150405"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("\xEF\xBB\xBF")) // BOM: Excel opens UTF-8 correctly
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"created_at", "actor_uuid", "actor_email", "actor_name", "action", "resource", "resource_uuid", "ip_address", "payload"})
	for _, e := range items {
		var actorUUID, email, actorName, resUUID, ip string
		if e.Actor != nil {
			actorUUID, email = e.Actor.UUID.String(), e.Actor.Email
			actorName = strings.TrimSpace(e.Actor.Name + " " + e.Actor.Surname)
		}
		if e.ResourceUUID != nil {
			resUUID = e.ResourceUUID.String()
		}
		if e.IPAddress != nil {
			ip = *e.IPAddress
		}
		payload, _ := json.Marshal(e.Payload)
		_ = cw.Write([]string{
			e.CreatedAt.Format(time.RFC3339), actorUUID, csvSafe(email), csvSafe(actorName),
			e.Action, e.Resource, resUUID, ip, csvSafe(string(payload)),
		})
	}
	cw.Flush()
}

// csvSafe neutralises spreadsheet formula injection.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

func parseFilter(w http.ResponseWriter, r *http.Request) (Filter, apiquery.Query, bool) {
	values := r.URL.Query()
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns{"created_at": "created_at"}); err != nil {
		var ve *apiquery.ValidationError
		if errors.As(err, &ve) {
			response.ValidationError(w, r, []response.Detail{{Field: "sort", Message: err.Error(), Code: "invalid"}})
		} else {
			response.BadRequest(w, r, response.CodeValidationError, err.Error())
		}
		return Filter{}, q, false
	}
	f := Filter{Q: q.Q, Desc: true, Resources: apiquery.SplitCSV(values.Get("resource")), Actions: apiquery.SplitCSV(values.Get("action"))}
	if len(q.Sort) > 0 {
		f.Desc = q.Sort[0].Desc
	}
	if raw := strings.TrimSpace(values.Get("actor")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "actor", Message: "actor must be a user uuid", Code: "invalid"}})
			return Filter{}, q, false
		}
		f.ActorUUID = id
	}
	for _, p := range []struct {
		key string
		dst **time.Time
		end bool
	}{{"from", &f.From, false}, {"to", &f.To, true}} {
		raw := strings.TrimSpace(values.Get(p.key))
		if raw == "" {
			continue
		}
		t, err := parseTime(raw, p.end)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: p.key, Message: p.key + " must be RFC 3339 or YYYY-MM-DD", Code: "invalid"}})
			return Filter{}, q, false
		}
		*p.dst = &t
	}
	if f.From != nil && f.To != nil && !f.To.After(*f.From) {
		response.ValidationError(w, r, []response.Detail{{Field: "to", Message: "to must be after from", Code: "invalid"}})
		return Filter{}, q, false
	}
	return f, q, true
}

// parseTime accepts RFC 3339 or a date; a date `to` is exclusive end of that
// day (UTC).
func parseTime(raw string, end bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, err
	}
	if end {
		d = d.Add(24 * time.Hour)
	}
	return d, nil
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

func pgTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func pgText(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}
