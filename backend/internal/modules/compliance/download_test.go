package compliance_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/usecase"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// memStore serves the export from memory; other Driver methods are unused.
type memStore struct {
	storage.Driver
	m *storage.Memory
}

func (s memStore) Upload(ctx context.Context, f storage.File, key string) error {
	return s.m.Upload(ctx, f, key)
}
func (s memStore) Download(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	return s.m.Download(ctx, key)
}

type loader map[uuid.UUID]int64

func (l loader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: l[id]}, nil
}

// TestDataSubjectExportDownload: the export ZIP (personal data) is only
// served inside its organization, and every download leaves an audit row
// (CF-153).
func TestDataSubjectExportDownload(t *testing.T) {
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
	for _, name := range []string{"owner", "attacker"} {
		u, err := q.CreateUser(ctx, db.CreateUserParams{Email: "dsr-" + name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "T", Status: "active"})
		if err != nil {
			t.Fatal(err)
		}
		users[name] = u
		ids[u.Uuid] = u.ID
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), nil)
	orgA, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "DSR A " + suffix, OwnerUserID: users["owner"].ID})
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "DSR B " + suffix, OwnerUserID: users["attacker"].ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM data_subject_requests WHERE organization_id = $1", orgA.ID)
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.ID, orgB.ID})
		for _, u := range users {
			_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE actor_user_id = $1", u.ID)
			_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", u.ID)
		}
	})
	var contact uuid.UUID
	if err := pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name,email) VALUES($1,'Kişi','dsr-"+suffix+"@example.test') RETURNING uuid", orgA.ID).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	svc := &usecase.Service{Repo: repository.New(pool, q), Store: memStore{m: storage.NewMemory()}, Activity: activity.NewRecorder(q, nil)}
	req, err := svc.Create(ctx, orgA.ID, users["owner"].ID, contact, "export")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Process(ctx, orgA.ID, req.UUID); err != nil {
		t.Fatal(err)
	}

	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	compliance.RegisterRoutes(supportops.Router{Mux: mux, Tokens: tokens, Loader: ids, Resolver: orgSvc, Activity: svc.Activity}, svc)
	download := func(as, org string) int {
		r := httptest.NewRequest(http.MethodGet, "/v1/tenant/compliance/requests/"+req.UUID.String()+"/download", nil)
		access, _, _ := tokens.IssueAccess(jwt.AccessInput{UserID: users[as].Uuid})
		r.Header.Set("Authorization", "Bearer "+access)
		r.Header.Set(orgctx.HeaderName, org)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Code
	}
	if code := download("attacker", orgB.Slug); code != http.StatusNotFound {
		t.Fatalf("cross-org download = %d, want 404", code)
	}
	if code := download("attacker", orgA.Slug); code != http.StatusForbidden && code != http.StatusNotFound {
		t.Fatalf("non-member download = %d, want 403/404", code)
	}
	if code := download("owner", orgA.Slug); code != http.StatusOK {
		t.Fatalf("owner download = %d", code)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM activity_events WHERE action = 'compliance.downloaded' AND organization_id = $1 AND resource_uuid = $2 AND actor_user_id = $3`,
		orgA.ID, req.UUID, users["owner"].ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("download audit rows = %d, want 1", n)
	}
}
