package audit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/audit"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type loader map[uuid.UUID]int64

func (l loader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: l[id]}, nil
}

// TestOrganizationAuditLog: the recorder stamps the organization from the
// scope and scrubs personal data; the audit API only shows the caller's
// organization, filters, exports CSV and needs tenant.audit.* (CF-12).
func TestOrganizationAuditLog(t *testing.T) {
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
	ids := loader{}
	for _, name := range []string{"owner", "agent", "other"} {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: "audit-" + name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "T", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		users[name] = u
		ids[u.Uuid] = u.ID
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), nil)
	orgA, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Audit A " + suffix, OwnerUserID: users["owner"].ID})
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Audit B " + suffix, OwnerUserID: users["other"].ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = ANY($1)", []int64{orgA.ID, orgB.ID})
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.ID, orgB.ID})
		for _, u := range users {
			_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", u.ID)
		}
	})
	if _, err := orgSvc.AddMember(ctx, orgA.UUID, users["agent"].Uuid, "agent"); err != nil {
		t.Fatal(err)
	}

	rec := activity.NewRecorder(q, nil)
	inA := orgctx.WithScope(ctx, orgctx.Scope{ID: orgA.ID, UUID: orgA.UUID, Slug: orgA.Slug})
	inB := orgctx.WithScope(ctx, orgctx.Scope{ID: orgB.ID, UUID: orgB.UUID, Slug: orgB.Slug})
	ownerID, otherID := users["owner"].ID, users["other"].ID
	target := uuid.New()
	rec.Record(inA, &ownerID, "invitations.created", "tenant.invitations", &target, map[string]any{
		"email": "guest@example.com", "phone": "+905551112233", "roles": []string{"agent"},
	}, nil)
	rec.Record(inA, &ownerID, "teams.created", "tenant.teams", &target, map[string]any{"name": "L1"}, nil)
	rec.Record(inB, &otherID, "teams.created", "tenant.teams", &target, map[string]any{"name": "Gizli"}, nil)
	rec.Record(ctx, &ownerID, "platform.only", "platform.users", nil, map[string]any{}, nil)

	tokens, _ := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	mux := http.NewServeMux()
	audit.RegisterRoutes(mux, audit.NewHandler(audit.NewService(q)), tokens, ids, orgSvc)
	call := func(as, org, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		access, _, _ := tokens.IssueAccess(jwt.AccessInput{UserID: users[as].Uuid})
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, org)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}
	type page struct {
		Data struct {
			Items []audit.Event `json:"items"`
			Total int64         `json:"total"`
		} `json:"data"`
	}
	list := func(as, org, path string) page {
		t.Helper()
		rr := call(as, org, path)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s %s = %d %s", as, path, rr.Code, rr.Body.String())
		}
		var p page
		_ = json.Unmarshal(rr.Body.Bytes(), &p)
		return p
	}

	p := list("owner", orgA.Slug, "/v1/tenant/audit")
	if p.Data.Total != 2 {
		t.Fatalf("A events = %d %+v", p.Data.Total, p.Data.Items)
	}
	for _, e := range p.Data.Items {
		if e.Action == "platform.only" || e.Payload["name"] == "Gizli" {
			t.Fatalf("foreign event leaked: %+v", e)
		}
		if e.Actor == nil || e.Actor.UUID != users["owner"].Uuid {
			t.Fatalf("actor = %+v", e.Actor)
		}
		if e.Action == "invitations.created" {
			if e.Payload["email"] != "g***@example.com" || e.Payload["phone"] != nil {
				t.Fatalf("payload not scrubbed: %v", e.Payload)
			}
		}
	}
	if p := list("owner", orgA.Slug, "/v1/tenant/audit?resource=tenant.teams"); p.Data.Total != 1 {
		t.Fatalf("resource filter = %d", p.Data.Total)
	}
	if p := list("owner", orgA.Slug, "/v1/tenant/audit?actor="+users["agent"].Uuid.String()); p.Data.Total != 0 {
		t.Fatalf("actor filter = %d", p.Data.Total)
	}
	tomorrow := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")
	if p := list("owner", orgA.Slug, "/v1/tenant/audit?from="+tomorrow); p.Data.Total != 0 {
		t.Fatalf("from filter = %d", p.Data.Total)
	}
	if rr := call("owner", orgA.Slug, "/v1/tenant/audit?from=yesterday"); rr.Code != http.StatusUnprocessableEntity && rr.Code != http.StatusBadRequest {
		t.Fatalf("bad from = %d", rr.Code)
	}
	if p := list("other", orgB.Slug, "/v1/tenant/audit"); p.Data.Total != 1 {
		t.Fatalf("B events = %d", p.Data.Total)
	}
	// Agents have no audit permission; non-members are refused.
	if rr := call("agent", orgA.Slug, "/v1/tenant/audit"); rr.Code != http.StatusForbidden {
		t.Fatalf("agent = %d", rr.Code)
	}
	if rr := call("other", orgA.Slug, "/v1/tenant/audit"); rr.Code != http.StatusForbidden {
		t.Fatalf("outsider = %d", rr.Code)
	}
	// Facets and CSV export.
	rr := call("owner", orgA.Slug, "/v1/tenant/audit/facets")
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "tenant.invitations") || strings.Contains(rr.Body.String(), "platform.users") {
		t.Fatalf("facets = %d %s", rr.Code, rr.Body.String())
	}
	rr = call("owner", orgA.Slug, "/v1/tenant/audit/export?resource=tenant.invitations")
	body := rr.Body.String()
	if rr.Code != http.StatusOK || !strings.HasPrefix(rr.Header().Get("Content-Type"), "text/csv") ||
		strings.Count(body, "\n") != 2 || !strings.Contains(body, "invitations.created") || strings.Contains(body, "guest@example.com") {
		t.Fatalf("export = %d %q", rr.Code, body)
	}
}
