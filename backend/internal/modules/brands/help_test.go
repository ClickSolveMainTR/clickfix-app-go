package brands

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPublicHelpOnlyServesPublishedBrandArticles(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var org, author int64
	slug := "brandhelp-" + uuid.NewString()
	if err := tx.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Help') RETURNING id", slug).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname) VALUES($1,'x','Test','Agent') RETURNING id", uuid.NewString()+"@example.test").Scan(&author); err != nil {
		t.Fatal(err)
	}
	var a, b uuid.UUID
	host := "help-" + uuid.NewString() + ".test"
	if err := tx.QueryRow(ctx, "INSERT INTO brands(organization_id,name,help_subdomain,help_path) VALUES($1,'A',$2,'alpha') RETURNING uuid", org, host).Scan(&a); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, "INSERT INTO brands(organization_id,name) VALUES($1,'B') RETURNING uuid", org).Scan(&b); err != nil {
		t.Fatal(err)
	}
	for _, article := range []struct {
		slug       string
		brand      uuid.UUID
		visibility string
		published  bool
	}{{"public-a", a, "public", true}, {"public-b", b, "public", true}, {"draft-a", a, "public", false}, {"internal-a", a, "internal", true}} {
		var id int64
		if err := tx.QueryRow(ctx, "INSERT INTO knowledge_articles(organization_id,slug,locale,title,body,visibility,author_id,brand_id) VALUES($1,$2,'tr',$2,'Private draft body',$3,$4,$5) RETURNING id", org, article.slug, article.visibility, author, article.brand).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if article.published {
			if _, err := tx.Exec(ctx, "INSERT INTO knowledge_article_versions(organization_id,article_id,version,title,body,visibility,author_id) VALUES($1,$2,1,$3,'Published body',$4,$5)", org, id, article.slug, article.visibility, author); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, "UPDATE knowledge_articles SET published_version=1 WHERE id=$1", id); err != nil {
				t.Fatal(err)
			}
		}
	}
	svc := &Service{Repo: &repository.Repository{Q: db.New(tx)}}
	mux := http.NewServeMux()
	RegisterRoutes(supportops.Router{Mux: mux}, svc)
	for _, path := range []string{"/v1/public/brand-help/" + strings.ToUpper(host), "/v1/public/help/" + slug + "/brand-paths/alpha", "/v1/public/help/" + slug + "/brands/" + a.String()} {
		r := httptest.NewRecorder()
		mux.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
		body := r.Body.String()
		if r.Code != 200 || !strings.Contains(body, "public-a") || strings.Contains(body, "public-b") || strings.Contains(body, "draft-a") || strings.Contains(body, "internal-a") || strings.Contains(body, "Private draft body") {
			t.Fatalf("help %s = %d %s", path, r.Code, body)
		}
	}
	for _, slug := range []string{"public-a", "public-b", "draft-a", "internal-a"} {
		r := httptest.NewRecorder()
		mux.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/public/brand-help/"+host+"/articles/"+slug, nil))
		want := 404
		if slug == "public-a" {
			want = 200
		}
		if r.Code != want {
			t.Fatalf("article %s = %d %s", slug, r.Code, r.Body.String())
		}
	}
}
