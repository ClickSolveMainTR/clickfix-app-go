package categories_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/categories"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/categories/classify"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/categories/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/categories/usecase"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
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

// departments stands in for CF-215: two departments of organization A.
type departments map[uuid.UUID]int64

func (d departments) ID(_ context.Context, _ int64, id uuid.UUID) (int64, error) {
	if v, ok := d[id]; ok {
		return v, nil
	}
	return 0, usecase.ErrNotFound
}

func (d departments) UUID(_ context.Context, _ int64, id int64) (uuid.UUID, error) {
	for k, v := range d {
		if v == id {
			return k, nil
		}
	}
	return uuid.Nil, usecase.ErrNotFound
}

// guess always proposes the category with the given slug.
type guess struct {
	q    *db.Queries
	slug string
	conf float64
	hits int
}

func (g *guess) Classify(ctx context.Context, in classify.Input, cands []classify.Candidate) (classify.Result, error) {
	g.hits++
	for _, c := range cands {
		if c.Slug == g.slug {
			return classify.Result{Prediction: classify.Prediction{CategoryID: c.ID, Confidence: g.conf, Source: classify.SourceAI, Classifier: "test"}, NeedsReview: g.conf < 0.6}, nil
		}
	}
	return classify.Result{NeedsReview: true}, nil
}

type embedder struct{}

func (embedder) Model() string { return "test-embed" }
func (embedder) Embed(_ context.Context, in []string) (ai.Embeddings, error) {
	v := make([][]float32, len(in))
	for i := range v {
		v[i] = []float32{0.5, 0.25, 0.125}
	}
	return ai.Embeddings{Vectors: v, InputTokens: 1}, nil
}

// TestCategoryRoutes covers tree management, department scope, RBAC,
// cross-organization isolation, classification, labels and the export
// over HTTP with the real resolver (CF-218).
func TestCategoryRoutes(t *testing.T) {
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
			Email: "cat-" + name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "T", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		users[name] = u
		ids[u.Uuid] = u.ID
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), nil)
	orgA, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Cat A " + suffix, OwnerUserID: users["owner"].ID})
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Cat B " + suffix, OwnerUserID: users["outsider"].ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		for _, tbl := range []string{"category_labels", "category_assignments", "category_examples", "messages", "conversations", "contacts"} {
			_, _ = pool.Exec(bg, "DELETE FROM "+tbl+" WHERE organization_id = ANY($1)", []int64{orgA.ID, orgB.ID})
		}
		_, _ = pool.Exec(bg, "UPDATE categories SET parent_id = NULL WHERE organization_id = ANY($1)", []int64{orgA.ID, orgB.ID})
		_, _ = pool.Exec(bg, "DELETE FROM categories WHERE organization_id = ANY($1)", []int64{orgA.ID, orgB.ID})
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.ID, orgB.ID})
		for _, u := range users {
			_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", u.ID)
		}
	})
	if _, err := orgSvc.AddMember(ctx, orgA.UUID, users["agent"].Uuid, "agent"); err != nil {
		t.Fatal(err)
	}
	support, sales := uuid.New(), uuid.New()
	classifier := &guess{q: q, slug: "iade", conf: 0.45}
	svc := usecase.New(pool, q, usecase.Options{Departments: departments{support: 9000001, sales: 9000002}, Classifier: classifier, Embedder: embedder{}})
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	categories.RegisterRoutes(mux, handler.New(svc, nil), tokens, ids, orgSvc)
	raw := func(as, org, method, path string, body any) *httptest.ResponseRecorder {
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
		return rr
	}
	call := func(as, org, method, path string, body any) (int, map[string]any) {
		t.Helper()
		rr := raw(as, org, method, path, body)
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return rr.Code, env.Data
	}
	create := func(body map[string]any) map[string]any {
		t.Helper()
		code, c := call("owner", orgA.Slug, http.MethodPost, "/v1/tenant/categories", body)
		if code != http.StatusCreated {
			t.Fatalf("create %v = %d %v", body, code, c)
		}
		return c
	}
	items := func(as, org, query string) []any {
		t.Helper()
		code, data := call(as, org, http.MethodGet, "/v1/tenant/categories"+query, nil)
		if code != http.StatusOK {
			t.Fatalf("list = %d", code)
		}
		list, _ := data["items"].([]any)
		return list
	}

	// Tree: an organization-wide root, a support branch under it and a
	// sales root.
	billing := create(map[string]any{"name": "Fatura", "description": "Ödeme ve fatura", "examples": []string{"faturamı göremiyorum", " ", "faturamı göremiyorum"}})
	if billing["slug"] != "fatura" || len(billing["examples"].([]any)) != 1 || billing["department_uuid"] != nil {
		t.Fatalf("root = %v", billing)
	}
	refund := create(map[string]any{"name": "İade", "parent_uuid": billing["uuid"], "department_uuid": support.String()})
	if refund["depth"].(float64) != 1 || refund["department_uuid"] != support.String() {
		t.Fatalf("branch = %v", refund)
	}
	partial := create(map[string]any{"name": "Kısmi iade", "parent_uuid": refund["uuid"]})
	if partial["department_uuid"] != support.String() || partial["depth"].(float64) != 2 {
		t.Fatalf("child inherits department = %v", partial)
	}
	create(map[string]any{"name": "Teklif", "department_uuid": sales.String()})

	// Department rules.
	if code, _ := call("owner", orgA.Slug, http.MethodPost, "/v1/tenant/categories", map[string]any{"name": "X", "parent_uuid": refund["uuid"], "department_uuid": sales.String()}); code != http.StatusBadRequest {
		t.Fatalf("cross-department child = %d", code)
	}
	if code, _ := call("owner", orgA.Slug, http.MethodPatch, "/v1/tenant/categories/"+refund["uuid"].(string), map[string]any{"department_uuid": sales.String()}); code != http.StatusBadRequest {
		t.Fatalf("moving a branch away from its children = %d", code)
	}
	if code, _ := call("owner", orgA.Slug, http.MethodPost, "/v1/tenant/categories", map[string]any{"name": "Y", "department_uuid": uuid.NewString()}); code != http.StatusBadRequest {
		t.Fatalf("unknown department = %d", code)
	}
	if got := len(items("owner", orgA.Slug, "")); got != 4 {
		t.Fatalf("whole tree = %d", got)
	}
	if got := len(items("agent", orgA.Slug, "?department="+support.String())); got != 3 {
		t.Fatalf("support view (org-wide + support) = %d", got)
	}
	if got := len(items("owner", orgA.Slug, "?department="+sales.String())); got != 2 {
		t.Fatalf("sales view (org-wide + sales) = %d", got)
	}

	// Tree rules: duplicate slug, cycle, depth.
	if code, _ := call("owner", orgA.Slug, http.MethodPost, "/v1/tenant/categories", map[string]any{"name": "fatura"}); code != http.StatusConflict {
		t.Fatalf("duplicate slug = %d", code)
	}
	if code, _ := call("owner", orgA.Slug, http.MethodPatch, "/v1/tenant/categories/"+billing["uuid"].(string), map[string]any{"parent_uuid": partial["uuid"]}); code != http.StatusBadRequest {
		t.Fatalf("cycle = %d", code)
	}
	parent := partial
	for i := range 2 {
		parent = create(map[string]any{"name": "Derin " + string(rune('a'+i)), "parent_uuid": parent["uuid"]})
	}
	if code, _ := call("owner", orgA.Slug, http.MethodPost, "/v1/tenant/categories", map[string]any{"name": "Çok derin", "parent_uuid": parent["uuid"]}); code != http.StatusBadRequest {
		t.Fatalf("depth limit = %d", code)
	}

	// RBAC: agents read and label, they do not manage or export.
	if code, _ := call("agent", orgA.Slug, http.MethodPost, "/v1/tenant/categories", map[string]any{"name": "Z"}); code != http.StatusForbidden {
		t.Fatalf("agent create = %d", code)
	}
	if rr := raw("agent", orgA.Slug, http.MethodGet, "/v1/tenant/categories/labels/export", nil); rr.Code != http.StatusForbidden {
		t.Fatalf("agent export = %d", rr.Code)
	}

	// Isolation.
	if code, _ := call("outsider", orgA.Slug, http.MethodGet, "/v1/tenant/categories", nil); code != http.StatusForbidden {
		t.Fatalf("outsider on A = %d", code)
	}
	if code, _ := call("outsider", orgB.Slug, http.MethodGet, "/v1/tenant/categories/"+billing["uuid"].(string), nil); code != http.StatusNotFound {
		t.Fatalf("A's category through B = %d", code)
	}
	if code, _ := call("outsider", orgB.Slug, http.MethodPost, "/v1/tenant/categories", map[string]any{"name": "B kök", "parent_uuid": billing["uuid"]}); code != http.StatusBadRequest {
		t.Fatalf("B child under A's node = %d", code)
	}
	if got := len(items("outsider", orgB.Slug, "")); got != 0 {
		t.Fatalf("B tree = %d", got)
	}

	// A conversation: classification proposes "iade" with low confidence,
	// so it waits in the review queue.
	var contact, conv int64
	var convUUID uuid.UUID
	if err := pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Test') RETURNING id", orgA.ID).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,channel_type,contact_id) VALUES($1,1,'email',$2) RETURNING id, uuid", orgA.ID, contact).Scan(&conv, &convUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,status,content) VALUES($1,$2,'in','contact','received','Param iade edilmedi, bana ali@example.com adresinden yazın')", orgA.ID, conv); err != nil {
		t.Fatal(err)
	}
	path := "/v1/tenant/category-assignments/conversation/" + convUUID.String()
	if code, data := call("owner", orgA.Slug, http.MethodGet, path, nil); code != http.StatusOK || data["assignment"] != nil {
		t.Fatalf("no assignment yet = %d %v", code, data)
	}
	code, data := call("owner", orgA.Slug, http.MethodPost, path+"/classify", nil)
	a, _ := data["assignment"].(map[string]any)
	if code != http.StatusOK || a == nil || a["source"] != "ai" || a["needs_review"] != true || a["category"].(map[string]any)["slug"] != "iade" {
		t.Fatalf("classify = %d %v", code, data)
	}
	code, page := call("agent", orgA.Slug, http.MethodGet, "/v1/tenant/categories/review", nil)
	if code != http.StatusOK || len(page["items"].([]any)) != 1 {
		t.Fatalf("review queue = %d %v", code, page)
	}
	// B cannot see or label A's conversation.
	if code, _ := call("outsider", orgB.Slug, http.MethodPut, path, map[string]any{"category_uuid": billing["uuid"]}); code != http.StatusNotFound {
		t.Fatalf("B labels A's conversation = %d", code)
	}

	// The owner corrects it (refund -> partial refund), then confirms it.
	code, data = call("owner", orgA.Slug, http.MethodPut, path, map[string]any{"category_uuid": partial["uuid"]})
	a, _ = data["assignment"].(map[string]any)
	if code != http.StatusOK || a["source"] != "human" || a["needs_review"] != false || a["confidence"].(float64) != 1 {
		t.Fatalf("correct = %d %v", code, data)
	}
	if code, _ := call("owner", orgA.Slug, http.MethodPut, path, map[string]any{"category_uuid": partial["uuid"]}); code != http.StatusOK {
		t.Fatalf("confirm = %d", code)
	}
	// A human decision survives a forced re-classification.
	if code, data := call("owner", orgA.Slug, http.MethodPost, path+"/classify?force=true", nil); code != http.StatusOK || data["assignment"].(map[string]any)["source"] != "human" {
		t.Fatalf("force over human = %d %v", code, data)
	}
	if code, page := call("owner", orgA.Slug, http.MethodGet, "/v1/tenant/categories/review", nil); code != http.StatusOK || len(page["items"].([]any)) != 0 {
		t.Fatalf("queue after labelling = %v", page)
	}

	// Export: two labels (correct, confirm), masked text with its vector.
	rr := raw("owner", orgA.Slug, http.MethodGet, "/v1/tenant/categories/labels/export", nil)
	if rr.Code != http.StatusOK || !strings.HasPrefix(rr.Header().Get("Content-Type"), "application/x-ndjson") {
		t.Fatalf("export = %d %s", rr.Code, rr.Body.String())
	}
	var lines []map[string]any
	sc := bufio.NewScanner(rr.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var l map[string]any
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, l)
	}
	if len(lines) != 2 || lines[0]["action"] != "correct" || lines[1]["action"] != "confirm" {
		t.Fatalf("export lines = %v", lines)
	}
	first := lines[0]
	if first["previous_category"].(map[string]any)["slug"] != "iade" || first["category"].(map[string]any)["slug"] != "kismi-iade" ||
		first["previous_source"] != "ai" || first["embedding_model"] != "test-embed" || len(first["embedding"].([]any)) != 3 ||
		first["labeled_by"] != users["owner"].Uuid.String() || first["department_uuid"] != nil {
		t.Fatalf("export line = %v", first)
	}
	if strings.Contains(rr.Body.String(), "ali@example.com") {
		t.Fatal("export leaks the e-mail address")
	}
	if rr := raw("outsider", orgB.Slug, http.MethodGet, "/v1/tenant/categories/labels/export", nil); rr.Code != http.StatusOK || rr.Body.Len() != 0 {
		t.Fatalf("B export = %d %q", rr.Code, rr.Body.String())
	}

	// Used categories are archived, not deleted; unused leaves are deleted.
	if code, _ := call("owner", orgA.Slug, http.MethodDelete, "/v1/tenant/categories/"+partial["uuid"].(string), nil); code != http.StatusConflict {
		t.Fatalf("delete used = %d", code)
	}
	if code, c := call("owner", orgA.Slug, http.MethodPatch, "/v1/tenant/categories/"+refund["uuid"].(string), map[string]any{"archived": true}); code != http.StatusOK || c["archived"] != true {
		t.Fatalf("archive = %d %v", code, c)
	}
	if got := len(items("owner", orgA.Slug, "")); got != 2 {
		t.Fatalf("archiving cascades to the branch: %d active", got)
	}
	if code, _ := call("owner", orgA.Slug, http.MethodPut, path, map[string]any{"category_uuid": partial["uuid"]}); code != http.StatusBadRequest {
		t.Fatalf("label with archived = %d", code)
	}
	if code, _ := call("outsider", orgB.Slug, http.MethodDelete, "/v1/tenant/categories/"+billing["uuid"].(string), nil); code != http.StatusNotFound {
		t.Fatalf("B deletes A's node = %d", code)
	}

	// The inbox consumer classifies a new conversation once.
	var conv2 int64
	if err := pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,channel_type,contact_id) VALUES($1,2,'email',$2) RETURNING id", orgA.ID, contact).Scan(&conv2); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,status,content) VALUES($1,$2,'in','contact','received','Faturamı göremiyorum')", orgA.ID, conv2); err != nil {
		t.Fatal(err)
	}
	classifier.slug, classifier.conf = "fatura", 0.9
	org := orgA.ID
	ev := events.Event{Name: events.MessageReceived, TenantID: &org, Payload: map[string]any{"conversation_id": conv2, "direction": "inbound", "sender_type": "contact"}}
	consumer := svc.Consumer()
	before := classifier.hits
	for range 2 {
		if err := consumer.Handle(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	var source string
	var review bool
	if err := pool.QueryRow(ctx, "SELECT source, needs_review FROM category_assignments WHERE organization_id=$1 AND subject_type='conversation' AND subject_id=$2", orgA.ID, conv2).Scan(&source, &review); err != nil || source != "ai" || review || classifier.hits != before+1 {
		t.Fatalf("consumer = %s %v %v hits=%d", source, review, err, classifier.hits-before)
	}

	// The phase-1 chain over the real vector store: example vectors are
	// built lazily and the embedding stage decides without the AI.
	phase1 := usecase.New(pool, q, usecase.Options{Departments: departments{support: 9000001, sales: 9000002}, Embedder: keywords{}})
	conv2UUID := uuid.UUID{}
	if err := pool.QueryRow(ctx, "SELECT uuid FROM conversations WHERE id=$1", conv2).Scan(&conv2UUID); err != nil {
		t.Fatal(err)
	}
	got, err := phase1.ClassifyRecord(ctx, orgA.ID, 0, classify.SubjectConversation, conv2UUID, true)
	if err != nil || got == nil || got.Source != classify.SourceEmbedding || got.Category == nil || got.Category.Slug != "fatura" || got.NeedsReview {
		t.Fatalf("phase 1 = %+v %v", got, err)
	}
	var vectors int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM category_examples WHERE organization_id=$1 AND embedding_model='keywords'", orgA.ID).Scan(&vectors); err != nil || vectors < 3 {
		t.Fatalf("example vectors = %d %v", vectors, err)
	}
}

// keywords embeds "fatura" and everything else on two axes.
type keywords struct{}

func (keywords) Model() string { return "keywords" }
func (keywords) Embed(_ context.Context, in []string) (ai.Embeddings, error) {
	v := make([][]float32, len(in))
	for i, s := range in {
		if strings.Contains(strings.ToLower(s), "fatura") {
			v[i] = []float32{1, 0}
		} else {
			v[i] = []float32{0.5, 0.5}
		}
	}
	return ai.Embeddings{Vectors: v, InputTokens: 1}, nil
}

// TestDepartmentsUnavailable: until CF-215 lands, department scope is
// refused instead of being silently dropped.
func TestDepartmentsUnavailable(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	svc := usecase.New(pool, db.New(pool), usecase.Options{})
	if _, err := svc.List(context.Background(), 1, uuid.New(), false); err != usecase.ErrDepartmentsUnavailable {
		t.Fatalf("list by department = %v", err)
	}
}
