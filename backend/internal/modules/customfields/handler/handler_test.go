package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type env struct {
	mux      *http.ServeMux
	userUUID uuid.UUID
}

// newEnv wires the real Postgres repository behind a mux that injects the
// organization from X-Test-Org ("a" or "b").
func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	suffix := uuid.NewString()[:8]
	var userID int64
	var userUUID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Test', 'User', 'active') RETURNING id, uuid`,
		"cf-"+suffix+"@example.test").Scan(&userID, &userUUID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID) })
	mkOrg := func(name string, member bool) orgctx.Scope {
		org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: name + "-" + suffix, Name: name})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, org.ID) })
		if member {
			if _, err := pool.Exec(ctx, `INSERT INTO organization_members (organization_id, user_id, role_id)
				SELECT $1, $2, id FROM roles WHERE scope = 'tenant' AND organization_id IS NULL AND slug = 'owner'`, org.ID, userID); err != nil {
				t.Fatal(err)
			}
		}
		return orgctx.Scope{ID: org.ID, UUID: org.Uuid, Slug: org.Slug, Status: orgctx.StatusActive}
	}
	scopes := map[string]orgctx.Scope{"a": mkOrg("alpha", true), "b": mkOrg("bravo", false)}
	h := handler.New(usecase.New(repository.New(pool, q)), activity.NewRecorder(q, nil))
	tenant := func(next http.HandlerFunc) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, ok := scopes[r.Header.Get("X-Test-Org")]
			if !ok {
				http.Error(w, "no scope", http.StatusForbidden)
				return
			}
			ctx := orgctx.WithScope(r.Context(), scope)
			ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: userID})
			next(w, r.WithContext(ctx))
		})
	}
	mux := http.NewServeMux()
	for pattern, fn := range map[string]http.HandlerFunc{
		"GET /v1/tenant/custom-fields/meta":                             h.Meta,
		"GET /v1/tenant/custom-fields":                                  h.ListDefinitions,
		"POST /v1/tenant/custom-fields":                                 h.CreateDefinition,
		"PUT /v1/tenant/custom-fields/order":                            h.Reorder,
		"GET /v1/tenant/custom-fields/{uuid}":                           h.GetDefinition,
		"PATCH /v1/tenant/custom-fields/{uuid}":                         h.UpdateDefinition,
		"DELETE /v1/tenant/custom-fields/{uuid}":                        h.ArchiveDefinition,
		"POST /v1/tenant/custom-fields/{uuid}/restore":                  h.RestoreDefinition,
		"GET /v1/tenant/custom-objects":                                 h.ListObjectTypes,
		"POST /v1/tenant/custom-objects":                                h.CreateObjectType,
		"PATCH /v1/tenant/custom-objects/{uuid}":                        h.UpdateObjectType,
		"GET /v1/tenant/custom-objects/{uuid}/records":                  h.ListRecords,
		"POST /v1/tenant/custom-objects/{uuid}/records":                 h.CreateRecord,
		"GET /v1/tenant/custom-objects/{uuid}/records/{record_uuid}":    h.GetRecord,
		"PATCH /v1/tenant/custom-objects/{uuid}/records/{record_uuid}":  h.UpdateRecord,
		"DELETE /v1/tenant/custom-objects/{uuid}/records/{record_uuid}": h.DeleteRecord,
	} {
		mux.Handle(pattern, tenant(fn))
	}
	return &env{mux: mux, userUUID: userUUID}
}

type result struct {
	status int
	data   map[string]any
	err    map[string]any
}

func (e *env) do(t *testing.T, org, method, path string, body any) result {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("X-Test-Org", org)
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	var env struct {
		Data  map[string]any `json:"data"`
		Error map[string]any `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return result{status: rec.Code, data: env.Data, err: env.Error}
}

func (e *env) must(t *testing.T, want int, r result) map[string]any {
	t.Helper()
	if r.status != want {
		t.Fatalf("status %d want %d: %v", r.status, want, r.err)
	}
	return r.data
}

func TestDefinitionsLifecycle(t *testing.T) {
	e := newEnv(t)
	tier := e.must(t, http.StatusCreated, e.do(t, "a", "POST", "/v1/tenant/custom-fields", map[string]any{
		"entity_type": "contact", "key": "tier", "label": "Seviye", "type": "select", "required": true,
		"options": []map[string]any{{"value": "gold", "label": "Altın"}, {"value": "silver", "label": "Gümüş"}},
		"default": "silver",
	}))
	e.must(t, http.StatusCreated, e.do(t, "a", "POST", "/v1/tenant/custom-fields", map[string]any{
		"entity_type": "contact", "key": "account_manager", "label": "Hesap yöneticisi", "type": "user",
		"condition": map[string]any{"field": "tier", "operator": "equals", "value": "gold"},
	}))
	// Same key again: conflict. Same key in another org: fine (isolation).
	if r := e.do(t, "a", "POST", "/v1/tenant/custom-fields", map[string]any{"entity_type": "contact", "key": "tier", "label": "x", "type": "text"}); r.status != http.StatusConflict {
		t.Fatalf("duplicate key status %d", r.status)
	}
	e.must(t, http.StatusCreated, e.do(t, "b", "POST", "/v1/tenant/custom-fields", map[string]any{"entity_type": "contact", "key": "tier", "label": "x", "type": "text"}))
	// Invalid definitions are 422 with field details.
	if r := e.do(t, "a", "POST", "/v1/tenant/custom-fields", map[string]any{"entity_type": "contact", "key": "Bad", "label": "", "type": "select"}); r.status != http.StatusBadRequest {
		t.Fatalf("invalid definition status %d %v", r.status, r.err)
	}

	list := e.must(t, http.StatusOK, e.do(t, "a", "GET", "/v1/tenant/custom-fields?entity_type=contact", nil))
	items := list["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["key"] != "tier" {
		t.Fatalf("list = %v", items)
	}
	tierID := tier["uuid"].(string)

	// Removing an option is refused; archiving it is fine.
	if r := e.do(t, "a", "PATCH", "/v1/tenant/custom-fields/"+tierID, map[string]any{"options": []map[string]any{{"value": "gold", "label": "Altın"}}}); r.status != http.StatusBadRequest {
		t.Fatalf("option removal status %d", r.status)
	}
	upd := e.must(t, http.StatusOK, e.do(t, "a", "PATCH", "/v1/tenant/custom-fields/"+tierID, map[string]any{
		"label":   "Müşteri seviyesi",
		"options": []map[string]any{{"value": "gold", "label": "Altın"}, {"value": "silver", "label": "Gümüş", "archived": true}},
		"default": nil,
	}))
	if upd["label"] != "Müşteri seviyesi" || upd["default"] != nil {
		t.Fatalf("patch = %v", upd)
	}
	// tier cannot be archived while account_manager depends on it.
	if r := e.do(t, "a", "DELETE", "/v1/tenant/custom-fields/"+tierID, nil); r.status != http.StatusBadRequest {
		t.Fatalf("archive dependency status %d", r.status)
	}
	// Reorder puts account_manager first.
	second := items[1].(map[string]any)["uuid"].(string)
	reordered := e.must(t, http.StatusOK, e.do(t, "a", "PUT", "/v1/tenant/custom-fields/order", map[string]any{"entity_type": "contact", "uuids": []string{second, tierID}}))
	if reordered["items"].([]any)[0].(map[string]any)["key"] != "account_manager" {
		t.Fatalf("reorder = %v", reordered)
	}
	// Another org cannot see the field.
	if r := e.do(t, "b", "GET", "/v1/tenant/custom-fields/"+tierID, nil); r.status != http.StatusNotFound {
		t.Fatalf("cross-org get status %d", r.status)
	}
}

func TestCustomObjectRecordsValidateAndFilter(t *testing.T) {
	e := newEnv(t)
	ot := e.must(t, http.StatusCreated, e.do(t, "a", "POST", "/v1/tenant/custom-objects", map[string]any{
		"key": "license", "singular_label": "Lisans", "plural_label": "Lisanslar", "icon": "key",
	}))
	otID := ot["uuid"].(string)
	for _, f := range []map[string]any{
		{"key": "seats", "label": "Koltuk", "type": "number", "required": true, "config": map[string]any{"min": 1, "precision": 0}},
		{"key": "expires_on", "label": "Bitiş", "type": "date"},
		{"key": "plan", "label": "Plan", "type": "select", "options": []map[string]any{{"value": "basic", "label": "Basic"}, {"value": "pro", "label": "Pro"}}},
		{"key": "owner", "label": "Sorumlu", "type": "user"},
	} {
		f["entity_type"] = "object"
		f["object_type_uuid"] = otID
		e.must(t, http.StatusCreated, e.do(t, "a", "POST", "/v1/tenant/custom-fields", f))
	}
	base := "/v1/tenant/custom-objects/" + otID + "/records"
	// Validation errors come back per field.
	r := e.do(t, "a", "POST", base, map[string]any{"display_name": "Bad", "custom_fields": map[string]any{"seats": 0, "plan": "gold"}})
	if r.status != http.StatusBadRequest {
		t.Fatalf("invalid record status %d", r.status)
	}
	details, _ := r.err["details"].([]any)
	if len(details) != 2 {
		t.Fatalf("details = %v", r.err)
	}
	// User fields must reference organization members.
	if r := e.do(t, "a", "POST", base, map[string]any{"display_name": "X", "custom_fields": map[string]any{"seats": 1, "owner": uuid.NewString()}}); r.status != http.StatusBadRequest {
		t.Fatalf("non-member user status %d", r.status)
	}
	for i, rec := range []map[string]any{
		{"seats": 5, "plan": "basic", "expires_on": "2026-12-31", "owner": e.userUUID.String()},
		{"seats": 50, "plan": "pro", "expires_on": "2027-06-30"},
		{"seats": 10, "plan": "pro"},
	} {
		e.must(t, http.StatusCreated, e.do(t, "a", "POST", base, map[string]any{"display_name": fmt.Sprintf("ACME-%d", i), "custom_fields": rec}))
	}
	page := e.must(t, http.StatusOK, e.do(t, "a", "GET", base+"?cf.plan=pro&sort=-cf.seats", nil))
	items := page["items"].([]any)
	if page["total"].(float64) != 2 || items[0].(map[string]any)["display_name"] != "ACME-1" {
		t.Fatalf("filtered page = %v", page)
	}
	page = e.must(t, http.StatusOK, e.do(t, "a", "GET", base+"?cf.expires_on.lt=2027-01-01&q=acme", nil))
	if page["total"].(float64) != 1 {
		t.Fatalf("date filter page = %v", page)
	}
	if r := e.do(t, "a", "GET", base+"?cf.nope=1", nil); r.status != http.StatusBadRequest {
		t.Fatalf("unknown filter status %d", r.status)
	}
	if r := e.do(t, "a", "GET", base+"?sort=cf.nope", nil); r.status != http.StatusBadRequest {
		t.Fatalf("unknown sort status %d", r.status)
	}
	// Patch merges: clearing plan keeps seats.
	recID := items[0].(map[string]any)["uuid"].(string)
	upd := e.must(t, http.StatusOK, e.do(t, "a", "PATCH", base+"/"+recID, map[string]any{"custom_fields": map[string]any{"plan": nil}}))
	cf := upd["custom_fields"].(map[string]any)
	if _, ok := cf["plan"]; ok || cf["seats"].(float64) != 50 {
		t.Fatalf("patched values = %v", cf)
	}
	// Other org cannot reach the object type.
	if r := e.do(t, "b", "GET", base, nil); r.status != http.StatusNotFound {
		t.Fatalf("cross-org list status %d", r.status)
	}
	e.must(t, http.StatusOK, e.do(t, "a", "DELETE", base+"/"+recID, nil))
	if r := e.do(t, "a", "GET", base+"/"+recID, nil); r.status != http.StatusNotFound {
		t.Fatalf("deleted record status %d", r.status)
	}
	// Archived object types take no new records.
	e.must(t, http.StatusOK, e.do(t, "a", "PATCH", "/v1/tenant/custom-objects/"+otID, map[string]any{"singular_label": "Lisans", "plural_label": "Lisanslar", "archived": true}))
	if r := e.do(t, "a", "POST", base, map[string]any{"display_name": "Late", "custom_fields": map[string]any{"seats": 1}}); r.status != http.StatusBadRequest {
		t.Fatalf("archived type create status %d", r.status)
	}
}
