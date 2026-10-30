package supportaccess_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportaccess"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type principal struct {
	id    int64
	perms []string
}

type loader map[uuid.UUID]principal

func (l loader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: l[id].id, Permissions: l[id].perms}, nil
}

// TestSupportAccess: staff enter an organization only while a grant is
// active, with the grant mode's permissions; actions are audited via_support;
// owners grant and revoke; staff cannot extend their own access (CF-14).
func TestSupportAccess(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	suffix := uuid.NewString()[:8]
	users := map[string]db.User{}
	for _, name := range []string{"owner", "staff", "agent", "nobody"} {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: "sa-" + name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "T", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		users[name] = u
	}
	// Staff holds super_admin (platform.support.access) in the database too.
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE slug = 'super_admin' AND scope = 'platform'`, users["staff"].ID); err != nil {
		t.Fatal(err)
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), nil)
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Support " + suffix, OwnerUserID: users["owner"].ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		for _, u := range users {
			_, _ = pool.Exec(bg, "DELETE FROM user_roles WHERE user_id = $1", u.ID)
			_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", u.ID)
		}
	})
	if _, err := orgSvc.AddMember(ctx, org.UUID, users["agent"].Uuid, "agent"); err != nil {
		t.Fatal(err)
	}

	svc := supportaccess.NewService(q)
	resolver := supportaccess.NewResolver(orgSvc, svc, orgSvc)
	rec := activity.NewRecorder(q, nil)
	tokens, _ := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	l := loader{
		users["owner"].Uuid:  {id: users["owner"].ID},
		users["staff"].Uuid:  {id: users["staff"].ID, perms: []string{rbac.PermPlatformSupportAccess}},
		users["agent"].Uuid:  {id: users["agent"].ID},
		users["nobody"].Uuid: {id: users["nobody"].ID},
	}
	mux := http.NewServeMux()
	supportaccess.RegisterRoutes(mux, supportaccess.NewHandler(svc, rec), tokens, l, resolver)
	// A probe route: what the staff member may do inside the organization.
	mux.Handle("GET /v1/tenant/probe", middleware.Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := orgctx.Must(r.Context())
		p := authctx.MustPrincipal(r.Context())
		rec.Record(r.Context(), &p.UserInternal, "probe.read", "tenant.probe", nil, map[string]any{}, r)
		response.JSON(w, r, http.StatusOK, map[string]any{"support": s.Support, "mode": s.SupportMode, "permissions": s.Permissions})
	}), middleware.Authenticate(tokens, l), middleware.RequireOrganization(resolver)))

	call := func(as, method, path string, body any) (int, map[string]any) {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		access, _, _ := tokens.IssueAccess(jwt.AccessInput{UserID: users[as].Uuid})
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, org.Slug)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return rr.Code, env.Data
	}

	// No grant: staff and strangers are refused.
	if code, _ := call("staff", http.MethodGet, "/v1/tenant/probe", nil); code != http.StatusForbidden {
		t.Fatalf("staff without grant = %d", code)
	}
	// Agents cannot grant; invalid input is a 422.
	if code, _ := call("agent", http.MethodPost, "/v1/tenant/support-access", map[string]any{"mode": "full"}); code != http.StatusForbidden {
		t.Fatalf("agent grant = %d", code)
	}
	if code, _ := call("owner", http.MethodPost, "/v1/tenant/support-access", map[string]any{"mode": "god", "duration_hours": 999}); code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid grant = %d", code)
	}
	code, grant := call("owner", http.MethodPost, "/v1/tenant/support-access", map[string]any{"mode": "read_only", "duration_hours": 2, "reason": "Kanal kurulumu"})
	if code != http.StatusCreated || grant["status"] != "active" {
		t.Fatalf("grant = %d %v", code, grant)
	}

	// Read-only: reads only, flagged as support, audited via_support.
	code, probe := call("staff", http.MethodGet, "/v1/tenant/probe", nil)
	if code != http.StatusOK || probe["support"] != true || probe["mode"] != "read_only" {
		t.Fatalf("staff probe = %d %v", code, probe)
	}
	perms := []string{}
	for _, p := range probe["permissions"].([]any) {
		perms = append(perms, p.(string))
	}
	if !slices.Contains(perms, rbac.PermTenantConversationsRead) || slices.Contains(perms, rbac.PermTenantConversationsReply) {
		t.Fatalf("read_only permissions = %v", perms)
	}
	var viaSupport bool
	if err := pool.QueryRow(ctx, `SELECT (payload->>'via_support')::boolean FROM activity_events
		WHERE organization_id = $1 AND action = 'probe.read' ORDER BY id DESC LIMIT 1`, org.ID).Scan(&viaSupport); err != nil || !viaSupport {
		t.Fatalf("support action not audited via_support: %v %v", viaSupport, err)
	}
	// A user without the platform permission still cannot enter.
	if code, _ := call("nobody", http.MethodGet, "/v1/tenant/probe", nil); code != http.StatusForbidden {
		t.Fatalf("nobody = %d", code)
	}
	// Staff cannot extend their own access.
	if code, _ := call("staff", http.MethodPost, "/v1/tenant/support-access", map[string]any{"mode": "full"}); code != http.StatusForbidden {
		t.Fatalf("staff self-grant = %d", code)
	}
	// Full grant replaces the read-only one; still no member / role management.
	code, full := call("owner", http.MethodPost, "/v1/tenant/support-access", map[string]any{"mode": "full", "duration_hours": 1})
	if code != http.StatusCreated {
		t.Fatalf("full grant = %d", code)
	}
	_, probe = call("staff", http.MethodGet, "/v1/tenant/probe", nil)
	perms = perms[:0]
	for _, p := range probe["permissions"].([]any) {
		perms = append(perms, p.(string))
	}
	if !slices.Contains(perms, rbac.PermTenantConversationsReply) || slices.Contains(perms, rbac.PermTenantMembersWrite) || slices.Contains(perms, rbac.PermTenantRolesWrite) {
		t.Fatalf("full permissions = %v", perms)
	}
	code, list := call("owner", http.MethodGet, "/v1/tenant/support-access", nil)
	if code != http.StatusOK || list["total"].(float64) != 2 || list["active"].(map[string]any)["mode"] != "full" {
		t.Fatalf("history = %d %v", code, list)
	}
	// /me lists the organization for staff, flagged support.
	orgs, err := supportaccess.MeLister{Next: orgSvc, Svc: svc}.ListForUser(ctx, users["staff"].ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range orgs {
		if o.UUID == org.UUID && o.Support && o.Role.Key == orgctx.RoleSupport {
			found = true
		}
	}
	if !found {
		t.Fatalf("me organizations = %+v", orgs)
	}
	// Revoke: access ends immediately.
	if code, _ := call("owner", http.MethodPost, "/v1/tenant/support-access/"+full["uuid"].(string)+"/revoke", nil); code != http.StatusOK {
		t.Fatalf("revoke = %d", code)
	}
	if code, _ := call("staff", http.MethodGet, "/v1/tenant/probe", nil); code != http.StatusForbidden {
		t.Fatalf("staff after revoke = %d", code)
	}
	if code, _ := call("owner", http.MethodPost, "/v1/tenant/support-access/"+full["uuid"].(string)+"/revoke", nil); code != http.StatusNotFound {
		t.Fatalf("second revoke = %d", code)
	}
	// Expired grants do not open the door either.
	if _, err := pool.Exec(ctx, `INSERT INTO support_access_grants (organization_id, mode, starts_at, expires_at)
		VALUES ($1, 'full', NOW() - interval '2 hours', NOW() - interval '1 hour')`, org.ID); err != nil {
		t.Fatal(err)
	}
	if code, _ := call("staff", http.MethodGet, "/v1/tenant/probe", nil); code != http.StatusForbidden {
		t.Fatalf("staff with expired grant = %d", code)
	}
}
