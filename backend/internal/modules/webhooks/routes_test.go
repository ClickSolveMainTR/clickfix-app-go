package webhooks_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
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

// TestWebhookRoutesTenantIsolation: an organization can neither read, change,
// delete nor replay another organization's webhooks by uuid (IDOR), a member
// without tenant.webhooks.write cannot mutate, a non-member is refused, and
// create/delete leave audit rows in the owning organization (CF-153).
func TestWebhookRoutesTenantIsolation(t *testing.T) {
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
	for _, name := range []string{"owner", "agent", "attacker"} {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: "wh-" + name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "T", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		users[name] = u
		ids[u.Uuid] = u.ID
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), nil)
	orgA, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Webhook A " + suffix, OwnerUserID: users["owner"].ID})
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Webhook B " + suffix, OwnerUserID: users["attacker"].ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.ID, orgB.ID})
		for _, u := range users {
			_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE actor_user_id = $1", u.ID)
			_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", u.ID)
		}
	})
	if _, err := orgSvc.AddMember(ctx, orgA.UUID, users["agent"].Uuid, "agent"); err != nil {
		t.Fatal(err)
	}

	box, err := crypto.NewSecretBox("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	webhooks.RegisterRoutes(mux, &handler.Handler{Service: usecase.New(q, box), Activity: activity.NewRecorder(q, nil)}, tokens, ids, orgSvc)
	call := func(as, org, method, path string, body any) (int, map[string]any) {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		access, _, _ := tokens.IssueAccess(jwt.AccessInput{UserID: users[as].Uuid})
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, org)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return rr.Code, env.Data
	}

	input := map[string]any{"name": "CRM", "url": "https://hooks.example.com/clickfix", "events": []string{events.WebhookEvents()[0]}, "enabled": true}
	code, created := call("owner", orgA.Slug, http.MethodPost, "/v1/tenant/webhooks", input)
	if code != http.StatusCreated {
		t.Fatalf("create = %d %v", code, created)
	}
	id, _ := created["uuid"].(string)
	path := "/v1/tenant/webhooks/" + id

	// Another organization's admin, scoped to their own organization, cannot
	// touch the webhook by uuid.
	if code, _ := call("attacker", orgB.Slug, http.MethodPut, path, input); code != http.StatusNotFound {
		t.Fatalf("cross-org update = %d, want 404", code)
	}
	if code, _ := call("attacker", orgB.Slug, http.MethodDelete, path, nil); code != http.StatusNotFound {
		t.Fatalf("cross-org delete = %d, want 404", code)
	}
	if code, page := call("attacker", orgB.Slug, http.MethodGet, "/v1/tenant/webhooks", nil); code != http.StatusOK || page["total"] != float64(0) {
		t.Fatalf("own list = %d %v, want 200 with no webhooks of another organization", code, page)
	}
	// Naming the victim organization does not help a non-member.
	if code, _ := call("attacker", orgA.Slug, http.MethodDelete, path, nil); code != http.StatusForbidden && code != http.StatusNotFound {
		t.Fatalf("non-member delete = %d, want 403/404", code)
	}
	// A member without tenant.webhooks.write cannot mutate.
	if code, _ := call("agent", orgA.Slug, http.MethodDelete, path, nil); code != http.StatusForbidden {
		t.Fatalf("agent delete = %d, want 403", code)
	}
	if code, _ := call("attacker", orgB.Slug, http.MethodPost, "/v1/tenant/webhook-deliveries/"+uuid.NewString()+"/replay", nil); code == http.StatusAccepted {
		t.Fatal("replay of an unknown delivery was accepted")
	}
	if code, _ := call("owner", orgA.Slug, http.MethodDelete, path, nil); code != http.StatusOK {
		t.Fatalf("owner delete = %d", code)
	}

	var actions []string
	rows, err := pool.Query(ctx, `SELECT action FROM activity_events WHERE resource = 'outbound_webhooks' AND organization_id = $1 AND resource_uuid = $2 ORDER BY id`, orgA.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var a string
		_ = rows.Scan(&a)
		actions = append(actions, a)
	}
	if len(actions) != 2 || actions[0] != "created" || actions[1] != "deleted" {
		t.Fatalf("audit actions = %v, want [created deleted]", actions)
	}
}
