package teams_test

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
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teams"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teams/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teams/usecase"
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

// TestTeamRoutes covers team CRUD, membership, skills, permissions and
// cross-organization isolation over HTTP with the real resolver (CF-5).
func TestTeamRoutes(t *testing.T) {
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
	for _, name := range []string{"owner", "agent", "outsider"} {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: "team-" + name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "T", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		users[name] = u
		ids[u.Uuid] = u.ID
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), nil)
	orgA, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Team A " + suffix, OwnerUserID: users["owner"].ID})
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Team B " + suffix, OwnerUserID: users["outsider"].ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.ID, orgB.ID})
		for _, u := range users {
			_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", u.ID)
		}
	})
	if _, err := orgSvc.AddMember(ctx, orgA.UUID, users["agent"].Uuid, "agent"); err != nil {
		t.Fatal(err)
	}

	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	teams.RegisterRoutes(mux, handler.New(usecase.New(pool, q), nil), tokens, ids, orgSvc)
	call := func(as, org, method, path string, body any) (int, map[string]any, []any) {
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
			Data json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		var obj map[string]any
		_ = json.Unmarshal(env.Data, &obj)
		var items []any
		if obj != nil {
			if v, ok := obj["items"].([]any); ok {
				items = v
			}
		}
		return rr.Code, obj, items
	}

	// Owner creates a team with the agent as member; the agent (teams.read
	// only) cannot create.
	hours := map[string]any{"days": []map[string]any{
		{"day": "mon", "open": true, "start": "08:00", "end": "17:00"},
		{"day": "tue", "open": true, "start": "08:00", "end": "17:00"},
		{"day": "wed", "open": true, "start": "08:00", "end": "17:00"},
		{"day": "thu", "open": true, "start": "08:00", "end": "17:00"},
		{"day": "fri", "open": true, "start": "08:00", "end": "17:00"},
		{"day": "sat", "open": false, "start": "09:00", "end": "13:00"},
		{"day": "sun", "open": false, "start": "09:00", "end": "13:00"},
	}}
	code, team, _ := call("owner", orgA.Slug, http.MethodPost, "/v1/tenant/teams", map[string]any{
		"name": "L1 Destek", "description": "İlk hat", "color": "#0EA5E9", "timezone": "Europe/Istanbul",
		"working_hours": hours, "skills": []string{"TR", "en", "tr"},
		"members": []map[string]any{{"user_uuid": users["owner"].Uuid, "role": "lead"}, {"user_uuid": users["agent"].Uuid}},
	})
	if code != http.StatusCreated {
		t.Fatalf("create = %d %v", code, team)
	}
	teamUUID := team["uuid"].(string)
	if team["member_count"].(float64) != 2 || len(team["leads"].([]any)) != 1 || len(team["skills"].([]any)) != 2 {
		t.Fatalf("created team = %v", team)
	}
	if code, _, _ := call("agent", orgA.Slug, http.MethodPost, "/v1/tenant/teams", map[string]any{"name": "X"}); code != http.StatusForbidden {
		t.Fatalf("agent create = %d", code)
	}
	if code, _, items := call("agent", orgA.Slug, http.MethodGet, "/v1/tenant/teams?member="+users["agent"].Uuid.String(), nil); code != http.StatusOK || len(items) != 1 {
		t.Fatalf("agent list = %d %v", code, items)
	}
	// Duplicate name, bad hours.
	if code, _, _ := call("owner", orgA.Slug, http.MethodPost, "/v1/tenant/teams", map[string]any{"name": "l1 destek"}); code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Fatalf("duplicate name = %d", code)
	}
	bad := map[string]any{"days": []map[string]any{{"day": "mon", "open": true, "start": "10:00", "end": "09:00"}}}
	if code, _, _ := call("owner", orgA.Slug, http.MethodPatch, "/v1/tenant/teams/"+teamUUID, map[string]any{"working_hours": bad}); code == http.StatusOK {
		t.Fatal("bad working hours accepted")
	}
	// Inherit organization hours.
	code, team, _ = call("owner", orgA.Slug, http.MethodPatch, "/v1/tenant/teams/"+teamUUID, map[string]any{"inherit_hours": true})
	if code != http.StatusOK || team["working_hours"] != nil || team["timezone"] != nil {
		t.Fatalf("inherit = %d %v", code, team)
	}
	// Member skills and the skill catalog.
	code, _, _ = call("owner", orgA.Slug, http.MethodPut, "/v1/tenant/members/"+users["agent"].Uuid.String()+"/skills", map[string]any{"skills": []string{"Billing", "de"}})
	if code != http.StatusOK {
		t.Fatalf("skills = %d", code)
	}
	if code, _, items := call("agent", orgA.Slug, http.MethodGet, "/v1/tenant/skills", nil); code != http.StatusOK || len(items) != 4 {
		t.Fatalf("skill catalog = %d %v", code, items)
	}
	// Team role change and removal.
	code, team, _ = call("owner", orgA.Slug, http.MethodPut, "/v1/tenant/teams/"+teamUUID+"/members/"+users["agent"].Uuid.String(), map[string]any{"role": "lead"})
	if code != http.StatusOK || len(team["leads"].([]any)) != 2 {
		t.Fatalf("promote = %d %v", code, team)
	}

	// Isolation: the outsider (owner of B) neither sees nor changes A's team,
	// and cannot add A's members to B's team.
	if code, _, _ := call("outsider", orgA.Slug, http.MethodGet, "/v1/tenant/teams", nil); code != http.StatusForbidden {
		t.Fatalf("outsider on A = %d", code)
	}
	if code, _, _ := call("outsider", orgB.Slug, http.MethodGet, "/v1/tenant/teams/"+teamUUID, nil); code != http.StatusNotFound {
		t.Fatalf("A's team through B = %d", code)
	}
	if code, _, _ := call("outsider", orgB.Slug, http.MethodDelete, "/v1/tenant/teams/"+teamUUID, nil); code != http.StatusNotFound {
		t.Fatalf("delete A's team through B = %d", code)
	}
	code, teamB, _ := call("outsider", orgB.Slug, http.MethodPost, "/v1/tenant/teams", map[string]any{"name": "L1 Destek"})
	if code != http.StatusCreated {
		t.Fatalf("same name in B = %d", code)
	}
	if code, _, _ := call("outsider", orgB.Slug, http.MethodPut, "/v1/tenant/teams/"+teamB["uuid"].(string)+"/members/"+users["agent"].Uuid.String(), map[string]any{"role": "member"}); code == http.StatusOK {
		t.Fatal("B added A's member to its team")
	}
	if code, _, items := call("outsider", orgB.Slug, http.MethodGet, "/v1/tenant/skills", nil); code != http.StatusOK || len(items) != 0 {
		t.Fatalf("B skill catalog = %v", items)
	}

	// Leaving the organization removes the member from its teams.
	if err := orgSvc.RemoveMember(ctx, orgA.UUID, users["agent"].Uuid); err != nil {
		t.Fatal(err)
	}
	if code, team, _ = call("owner", orgA.Slug, http.MethodGet, "/v1/tenant/teams/"+teamUUID, nil); code != http.StatusOK || team["member_count"].(float64) != 1 {
		t.Fatalf("after removal = %v", team)
	}
	if code, _, _ := call("owner", orgA.Slug, http.MethodDelete, "/v1/tenant/teams/"+teamUUID, nil); code != http.StatusOK {
		t.Fatalf("delete = %d", code)
	}
}
