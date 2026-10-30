package helpcenter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/helpcenter"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/helpcenter/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/helpcenter/usecase"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	kbrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/repository"
	kbusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ratelimit"
)

type fixture struct {
	pool               *pgxpool.Pool
	kb                 *kbusecase.Service
	help               *usecase.Service
	org, other, actor  int64
	orgSlug, otherSlug string
}

func setup(t *testing.T) fixture {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := fixture{pool: pool, orgSlug: "hc-" + uuid.NewString()[:8], otherSlug: "hc-" + uuid.NewString()[:8]}
	if err = pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Help org') RETURNING id", f.orgSlug).Scan(&f.org); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Other org') RETURNING id", f.otherSlug).Scan(&f.other); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname) VALUES($1,'fake','Help','Author') RETURNING id", uuid.NewString()+"@example.test").Scan(&f.actor); err != nil {
		t.Fatal(err)
	}
	f.kb = kbusecase.New(kbrepo.New(pool))
	f.help = usecase.New(db.New(pool), usecase.LexicalOnly(&kbusecase.RAG{KB: f.kb}), nil, "test-secret")
	return f
}

func (f fixture) article(t *testing.T, org int64, collection *uuid.UUID, slug, locale, title, body, visibility string, publish bool) db.KnowledgeArticle {
	t.Helper()
	ctx := context.Background()
	a, err := f.kb.SaveArticle(ctx, org, f.actor, uuid.New(), kbmodel.ArticleInput{Collection: collection, Slug: slug, Locale: locale, Title: title, Body: body, Visibility: visibility}, true)
	if err != nil {
		t.Fatal(err)
	}
	if publish {
		if err = f.kb.Publish(ctx, org, a.Uuid, a.Version, true); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

func TestHelpCenterNeverLeaksDraftsInternalOrOtherOrganizations(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// No published public article yet: the help center does not exist.
	if _, err := f.help.Home(ctx, f.orgSlug, "tr"); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("empty help center: %v", err)
	}

	col, err := f.kb.SaveCollection(ctx, f.org, uuid.New(), kbmodel.CollectionInput{Name: "Faturalar", Slug: "faturalar", Kind: "collection"})
	if err != nil {
		t.Fatal(err)
	}
	cat, err := f.kb.SaveCollection(ctx, f.org, uuid.New(), kbmodel.CollectionInput{Name: "İade", Slug: "iade", Kind: "category", Parent: &col.Uuid})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := f.kb.SaveCollection(ctx, f.org, uuid.New(), kbmodel.CollectionInput{Name: "Boş", Slug: "bos", Kind: "collection"})
	if err != nil {
		t.Fatal(err)
	}

	public := f.article(t, f.org, &cat.Uuid, "iade-nasil-yapilir", "tr", "İade nasıl yapılır", "## Adımlar\n\nFaturanızı **iade** etmek için [panel](https://example.com) açın.", "public", true)
	f.article(t, f.org, &col.Uuid, "taslak", "tr", "Taslak makale gizli", "taslak gövde iade", "public", false)
	f.article(t, f.org, &col.Uuid, "ic-makale", "tr", "İç makale gizli", "iç gövde iade", "internal", true)
	f.article(t, f.org, nil, "english-refund", "en", "Refund guide", "How to refund an invoice.", "public", true)
	f.article(t, f.other, nil, "iade-nasil-yapilir", "tr", "Başka org iade", "başka org gövde iade", "public", true)

	// A newer draft of a published article must not replace the public snapshot.
	if _, err = f.kb.SaveArticle(ctx, f.org, f.actor, public.Uuid, kbmodel.ArticleInput{Collection: &cat.Uuid, Slug: public.Slug, Locale: "tr", Title: "Yeni taslak başlık", Body: "GIZLI TASLAK iade", Visibility: "public", Version: public.Version}, false); err != nil {
		t.Fatal(err)
	}

	home, err := f.help.Home(ctx, f.orgSlug, "tr")
	if err != nil {
		t.Fatal(err)
	}
	if home.Organization.Name != "Help org" || len(home.Recent) != 1 || home.Recent[0].Title != "İade nasıl yapılır" {
		t.Fatalf("home %+v", home)
	}
	if strings.Contains(home.Recent[0].Excerpt, "**") || strings.Contains(home.Recent[0].Excerpt, "](") {
		t.Fatalf("excerpt not plain text: %q", home.Recent[0].Excerpt)
	}
	slugs := map[string]int64{}
	for _, c := range home.Collections {
		slugs[c.Slug] = c.ArticleCount
	}
	if slugs["faturalar"] != 1 || slugs["iade"] != 1 || slugs[empty.Slug] != 0 || len(home.Collections) != 2 {
		t.Fatalf("collections %+v", home.Collections)
	}

	page, err := f.help.Collection(ctx, f.orgSlug, "faturalar", "tr", 0, 0)
	if err != nil || page.Total != 1 || len(page.Children) != 1 || page.Articles[0].Slug != "iade-nasil-yapilir" {
		t.Fatalf("collection %+v %v", page, err)
	}
	if _, err = f.help.Collection(ctx, f.orgSlug, "bos", "tr", 0, 0); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("empty collection %v", err)
	}

	art, err := f.help.Article(ctx, f.orgSlug, "iade-nasil-yapilir", "tr")
	if err != nil {
		t.Fatal(err)
	}
	if art.Article.Title != "İade nasıl yapılır" || strings.Contains(art.Article.Body, "GIZLI") || art.Article.Collection == nil || art.Article.Parent == nil || art.Article.Parent.Slug != "faturalar" {
		t.Fatalf("article leaked draft or lost breadcrumb %+v", art.Article)
	}
	for _, slug := range []string{"taslak", "ic-makale", "english-refund"} {
		if _, err = f.help.Article(ctx, f.orgSlug, slug, "tr"); !errors.Is(err, usecase.ErrNotFound) {
			t.Fatalf("%s readable: %v", slug, err)
		}
	}
	if _, err = f.help.Article(ctx, f.orgSlug, "english-refund", "en"); err != nil {
		t.Fatalf("en article %v", err)
	}
	if _, err = f.help.Article(ctx, f.orgSlug, "x", "de"); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("locale %v", err)
	}

	_, hits, err := f.help.Search(ctx, f.orgSlug, "iade", "tr")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Slug != "iade-nasil-yapilir" {
		t.Fatalf("search leaked draft/internal/other org %+v", hits)
	}
	if _, hits, _ = f.help.Search(ctx, f.orgSlug, "GIZLI", "tr"); len(hits) != 0 {
		t.Fatalf("search matched unpublished draft %+v", hits)
	}
	if _, hits, _ = f.help.Search(ctx, f.orgSlug, "100%_", "tr"); len(hits) != 0 {
		t.Fatalf("like wildcards not escaped %+v", hits)
	}

	other, err := f.help.Article(ctx, f.otherSlug, "iade-nasil-yapilir", "tr")
	if err != nil || other.Article.Title != "Başka org iade" {
		t.Fatalf("other org %+v %v", other, err)
	}

	sm, err := f.help.Sitemap(ctx, f.orgSlug)
	if err != nil || len(sm.Articles) != 2 || len(sm.Collections) != 2 {
		t.Fatalf("sitemap %+v %v", sm, err)
	}

	// Unpublishing removes the article; the help center disappears with its last article.
	if err = f.kb.Publish(ctx, f.other, mustArticle(t, f, f.other, "iade-nasil-yapilir"), 1, false); err != nil {
		t.Fatal(err)
	}
	if _, err = f.help.Home(ctx, f.otherSlug, "tr"); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("unpublished org still public %v", err)
	}

	// Suspended organizations have no help center.
	if _, err = f.pool.Exec(ctx, "UPDATE organizations SET status='suspended' WHERE id=$1", f.org); err != nil {
		t.Fatal(err)
	}
	if _, err = f.help.Home(ctx, f.orgSlug, "tr"); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("suspended org %v", err)
	}
}

func mustArticle(t *testing.T, f fixture, org int64, slug string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(), "SELECT uuid FROM knowledge_articles WHERE organization_id=$1 AND slug=$2", org, slug).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestFeedbackDedupesAndIsRateLimited(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.article(t, f.org, nil, "kargo", "tr", "Kargo takibi", "Kargo durumunu görün.", "public", true)
	f.article(t, f.org, nil, "ic", "tr", "İç", "iç", "internal", true)

	// Same visitor, same day: the second vote replaces the first.
	if err := f.help.Feedback(ctx, f.orgSlug, "kargo", "tr", "10.0.0.1", true); err != nil {
		t.Fatal(err)
	}
	if err := f.help.Feedback(ctx, f.orgSlug, "kargo", "tr", "10.0.0.1", false); err != nil {
		t.Fatal(err)
	}
	if err := f.help.Feedback(ctx, f.orgSlug, "kargo", "tr", "10.0.0.2", true); err != nil {
		t.Fatal(err)
	}
	if err := f.help.Feedback(ctx, f.orgSlug, "ic", "tr", "10.0.0.1", true); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("internal article accepted feedback %v", err)
	}
	if err := f.help.Feedback(ctx, f.otherSlug, "kargo", "tr", "10.0.0.1", true); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("cross org feedback %v", err)
	}
	rows, _, err := f.kb.Articles(ctx, f.org, kbusecase.ArticleFilter{Q: "kargo"}, 10, 0)
	if err != nil || len(rows) != 1 || rows[0].HelpfulCount != 1 || rows[0].NotHelpfulCount != 1 {
		t.Fatalf("admin counters %+v %v", rows, err)
	}
	var stored int
	if err = f.pool.QueryRow(ctx, "SELECT count(*) FROM knowledge_article_feedback WHERE organization_id=$1 AND voter_hash LIKE '10.%'", f.org).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("raw IP stored %d %v", stored, err)
	}

	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL required for the rate limit check")
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { _ = rdb.Close() })
	limiter := ratelimit.New(rdb, "test-"+uuid.NewString())
	mux := http.NewServeMux()
	helpcenter.RegisterRoutes(mux, handler.New(f.help, func(r *http.Request, action, subject string, limit int, window time.Duration) (bool, time.Duration) {
		return limiter.Allow(r.Context(), action, subject, limit, window)
	}, nil))
	vote := func(ip string) int {
		body, _ := json.Marshal(map[string]any{"helpful": true, "locale": "tr"})
		req := httptest.NewRequest(http.MethodPost, "/v1/public/help/"+f.orgSlug+"/articles/kargo/feedback", bytes.NewReader(body))
		req.Header.Set("X-Forwarded-For", ip)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := 0; i < 3; i++ {
		if code := vote("192.0.2.7"); code != http.StatusOK {
			t.Fatalf("vote %d: %d", i, code)
		}
	}
	if code := vote("192.0.2.7"); code != http.StatusTooManyRequests {
		t.Fatalf("4th vote from one IP on one article: %d", code)
	}
	if code := vote("192.0.2.8"); code != http.StatusOK {
		t.Fatalf("other IP limited: %d", code)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/public/help/"+f.orgSlug+"/articles/ic?locale=tr", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), handler.CodeNotFound) {
		t.Fatalf("internal article over HTTP: %d %s", rec.Code, rec.Body.String())
	}
}
