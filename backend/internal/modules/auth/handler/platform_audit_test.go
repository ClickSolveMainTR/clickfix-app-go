package handler

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
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPlatformRoleAndUserChangesAreAudited: platform privilege changes write
// attributable audit rows carrying the browser IP the BFF forwards (CF-153).
func TestPlatformRoleAndUserChangesAreAudited(t *testing.T) {
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
	actor, err := q.CreateUser(ctx, db.CreateUserParams{Email: "audit-actor-" + suffix + "@example.test", PasswordHash: "x", Name: "A", Surname: "T", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := q.CreateUser(ctx, db.CreateUserParams{Email: "audit-target-" + suffix + "@example.test", PasswordHash: "x", Name: "B", Surname: "T", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE actor_user_id = $1", actor.ID)
		_, _ = pool.Exec(bg, "DELETE FROM roles WHERE slug = $1", "audit-"+suffix)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{actor.ID, target.ID})
	})

	tokens, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := New(usecase.New(repository.NewPostgres(pool, q), tokens), nil, nil, nil, nil, "adapter-secret", nil, activity.NewRecorder(q, nil))
	principal := authctx.WithPrincipal(ctx, authctx.Principal{UserID: actor.Uuid, UserInternal: actor.ID, IsSuperAdmin: true})
	call := func(fn http.HandlerFunc, method, path, body string, pathValues map[string]string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(principal)
		req.RemoteAddr = "172.18.0.4:5000" // the BFF
		req.Header.Set("X-Forwarded-For", "198.51.100.23")
		for k, v := range pathValues {
			req.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		fn(rec, req)
		if rec.Code >= 300 {
			t.Fatalf("%s %s = %d %s", method, path, rec.Code, rec.Body.String())
		}
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return env.Data
	}

	role := call(h.CreatePlatformRole, http.MethodPost, "/v1/platform/roles", `{"name":"Audit `+suffix+`","slug":"audit-`+suffix+`","permission_slugs":["platform.users.read"]}`, nil)
	roleUUID, _ := role["uuid"].(string)
	call(h.PatchPlatformRole, http.MethodPatch, "/v1/platform/roles/"+roleUUID, `{"permission_slugs":["platform.users.read","platform.users.write"]}`, map[string]string{"uuid": roleUUID})
	call(h.PatchPlatformUser, http.MethodPatch, "/v1/platform/users/"+target.Uuid.String(), `{"role_uuids":["`+roleUUID+`"]}`, map[string]string{"uuid": target.Uuid.String()})
	call(h.DeletePlatformRole, http.MethodDelete, "/v1/platform/roles/"+roleUUID, ``, map[string]string{"uuid": roleUUID})

	rows, err := pool.Query(ctx, `SELECT action, resource, COALESCE(host(ip_address), '') FROM activity_events WHERE actor_user_id = $1 ORDER BY id`, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var action, resource, ip string
		if err := rows.Scan(&action, &resource, &ip); err != nil {
			t.Fatal(err)
		}
		if ip != "198.51.100.23" {
			t.Fatalf("%s recorded ip %q, want the forwarded client address", action, ip)
		}
		got = append(got, resource+":"+action)
	}
	want := []string{"platform.roles:roles.created", "platform.roles:roles.updated", "platform.users:users.updated", "platform.roles:roles.deleted"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("audit rows = %v, want %v", got, want)
	}
}
