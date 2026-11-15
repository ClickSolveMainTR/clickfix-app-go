package departments_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/usecase"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	kbrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/repository"
	kbusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/usecase"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/deptscope"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type embedder struct{}

func (embedder) Model() string { return "fake-dept-v1" }
func (embedder) Embed(_ context.Context, in []string) (ai.Embeddings, error) {
	out := make([][]float32, 0, len(in))
	for range in {
		out = append(out, testVec(1, 0))
	}
	return ai.Embeddings{Vectors: out}, nil
}

// testVec pads the given leading values to a full-size embedding.
func testVec(xs ...float32) []float32 {
	v := make([]float32, ai.EmbeddingDimensions)
	copy(v, xs)
	return v
}

type fixture struct {
	ctx      context.Context
	pool     *pgxpool.Pool
	q        *db.Queries
	org      orgmodel.Organization
	orgSvc   *orgusecase.Service
	users    map[string]db.User
	svc      *usecase.Service
	sales    model.Department
	support  model.Department
	teamSale db.Team
	teamSup  db.Team
	contact  int64
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	f := &fixture{ctx: ctx, pool: pool, q: q, users: map[string]db.User{}}
	suffix := uuid.NewString()[:8]
	for _, name := range []string{"owner", "agent", "lead"} {
		u, err := q.CreateUser(ctx, db.CreateUserParams{Email: "dept-" + name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "D", Status: "active"})
		if err != nil {
			t.Fatal(err)
		}
		f.users[name] = u
	}
	f.orgSvc = orgusecase.New(orgrepository.NewPostgres(pool, q), nil)
	f.org, err = f.orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Dept " + suffix, OwnerUserID: f.users["owner"].ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		for _, stmt := range []string{
			"DELETE FROM knowledge_chunks WHERE organization_id = $1",
			"DELETE FROM knowledge_index_states WHERE organization_id = $1",
			"DELETE FROM knowledge_article_versions WHERE organization_id = $1",
			"DELETE FROM knowledge_articles WHERE organization_id = $1",
			"DELETE FROM business_context_sections WHERE organization_id = $1",
			"DELETE FROM outbox_events WHERE organization_id = $1",
			"DELETE FROM organizations WHERE id = $1",
		} {
			_, _ = pool.Exec(bg, stmt, f.org.ID)
		}
		for _, u := range f.users {
			_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", u.ID)
		}
	})
	for _, name := range []string{"agent", "lead"} {
		if _, err := f.orgSvc.AddMember(ctx, f.org.UUID, f.users[name].Uuid, "agent"); err != nil {
			t.Fatal(err)
		}
	}
	team := func(name string, member db.User, role string) db.Team {
		tm, err := q.CreateTeam(ctx, db.CreateTeamParams{OrganizationID: f.org.ID, Name: name, Skills: []string{}})
		if err != nil {
			t.Fatal(err)
		}
		if err := q.UpsertTeamMember(ctx, db.UpsertTeamMemberParams{OrganizationID: f.org.ID, TeamID: tm.ID, UserID: member.ID, Role: role}); err != nil {
			t.Fatal(err)
		}
		return tm
	}
	f.teamSale = team("Satış ekibi", f.users["agent"], "member")
	f.teamSup = team("Destek L1", f.users["lead"], "lead")
	f.svc = usecase.New(pool, q)
	str := func(s string) *string { return &s }
	f.sales, err = f.svc.Create(ctx, f.org.ID, model.Input{Name: str("Satış"), Description: str("Fiyat ve teklif"), TeamUUIDs: &[]uuid.UUID{f.teamSale.Uuid},
		AI: &model.AIInput{RoutingHint: str("fiyat, teklif, satın alma")}})
	if err != nil {
		t.Fatal(err)
	}
	f.support, err = f.svc.Create(ctx, f.org.ID, model.Input{Name: str("Teknik destek"), TeamUUIDs: &[]uuid.UUID{f.teamSup.Uuid}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Create(ctx, f.org.ID, model.Input{Name: str("satış")}); !errors.Is(err, model.ErrNameTaken) {
		t.Fatalf("duplicate name = %v", err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO contacts (organization_id) VALUES ($1) RETURNING id", f.org.ID).Scan(&f.contact); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) scope(t *testing.T, user string, perms ...string) deptscope.Scope {
	t.Helper()
	s, err := usecase.ScopeOf(f.ctx, f.q, orgctx.Scope{ID: f.org.ID, Permissions: perms}, f.users[user].ID)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *fixture) viewer(t *testing.T, user string, perms ...string) usecase.Viewer {
	t.Helper()
	v, err := f.svc.ViewerFor(f.ctx, orgctx.Scope{ID: f.org.ID, Permissions: perms}, f.users[user].ID)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func titles(cs []kbmodel.Citation) []string {
	out := []string{}
	for _, c := range cs {
		if !slices.Contains(out, c.Title) {
			out = append(out, c.Title)
		}
	}
	sort.Strings(out)
	return out
}

// TestKnowledgeVisibility covers the four visibility rules of CF-215:
// closed by default, managers see everything, a lead's share opens one item
// to another department, organization-wide knowledge is every department's
// fallback.
func TestKnowledgeVisibility(t *testing.T) {
	f := setup(t)
	ctx := f.ctx
	kb := kbusecase.New(kbrepo.New(f.pool))
	rag := &kbusecase.RAG{KB: kb, Embedder: embedder{}}
	article := func(slug, title string, dept int64) db.KnowledgeArticle {
		in := kbmodel.ArticleInput{Slug: slug, Locale: "tr", Title: title, Body: "Kargo takibi ve iade süreci hakkında bilgi.", Visibility: "public"}
		if dept > 0 {
			id := uuid.New()
			in.Department = deptscope.Optional{Set: true, Value: &id}
			in.DepartmentID = pgtype.Int8{Int64: dept, Valid: true}
		}
		a, err := kb.SaveArticle(ctx, f.org.ID, f.users["owner"].ID, uuid.New(), in, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := kb.Publish(ctx, f.org.ID, a.Uuid, a.Version, true); err != nil {
			t.Fatal(err)
		}
		if err := rag.Index(ctx, f.org.ID, a.Uuid); err != nil {
			t.Fatal(err)
		}
		return a
	}
	article("kargo-genel", "Genel", 0)
	salesArticle := article("kargo-satis", "Satış", f.sales.ID)
	supportArticle := article("kargo-destek", "Destek", f.support.ID)

	search := func(s *deptscope.Scope) []string {
		t.Helper()
		c := ctx
		if s != nil {
			c = deptscope.With(ctx, *s)
		}
		hits, err := rag.Search(c, f.org.ID, "kargo", "tr", true)
		if err != nil {
			t.Fatal(err)
		}
		return titles(hits)
	}
	agent := f.scope(t, "agent", rbac.PermTenantDepartmentsRead)
	admin := f.scope(t, "owner", rbac.PermTenantDepartmentsRead, rbac.PermTenantDepartmentsReadAll)

	// Closed by default: the sales agent sees sales + organization-wide.
	if got := search(&agent); !slices.Equal(got, []string{"Genel", "Satış"}) {
		t.Fatalf("agent sees %v", got)
	}
	// Managers (read_all) see every department.
	if got := search(&admin); !slices.Equal(got, []string{"Destek", "Genel", "Satış"}) {
		t.Fatalf("admin sees %v", got)
	}
	// Organization-wide fallback: a context without a scope, and a
	// conversation without a department, see only organization-wide; a
	// department's conversation adds its own.
	if got := search(nil); !slices.Equal(got, []string{"Genel"}) {
		t.Fatalf("no scope sees %v", got)
	}
	none := deptscope.Of(pgtype.Int8{})
	if got := search(&none); !slices.Equal(got, []string{"Genel"}) {
		t.Fatalf("org-wide conversation sees %v", got)
	}
	supportConv := deptscope.Of(pgtype.Int8{Int64: f.support.ID, Valid: true})
	if got := search(&supportConv); !slices.Equal(got, []string{"Destek", "Genel"}) {
		t.Fatalf("support conversation sees %v", got)
	}
	// The admin list follows the same rule, and filters by department.
	rows, total, err := kb.Articles(ctx, f.org.ID, kbusecase.ArticleFilter{Scope: agent}, 20, 0)
	if err != nil || total != 2 || len(rows) != 2 {
		t.Fatalf("agent list = %d %d %v", len(rows), total, err)
	}
	rows, _, err = kb.Articles(ctx, f.org.ID, kbusecase.ArticleFilter{Scope: admin, DepartmentMode: "id", DepartmentID: pgtype.Int8{Int64: f.support.ID, Valid: true}}, 20, 0)
	if err != nil || len(rows) != 1 || rows[0].Uuid != supportArticle.Uuid || rows[0].DepartmentName.String != "Teknik destek" {
		t.Fatalf("support filter = %+v %v", rows, err)
	}
	rows, _, err = kb.Articles(ctx, f.org.ID, kbusecase.ArticleFilter{Scope: admin, DepartmentMode: "none"}, 20, 0)
	if err != nil || len(rows) != 1 || rows[0].DepartmentUuid.Valid {
		t.Fatalf("org-wide filter = %+v %v", rows, err)
	}
	// Single reads use the same rule.
	seen, err := usecase.Sees(ctx, f.q, f.org.ID, agent, model.KindArticle, supportArticle.ID, supportArticle.DepartmentID)
	if err != nil || seen {
		t.Fatalf("agent sees support article: %v %v", seen, err)
	}
	// Writes: a member files knowledge only under their own departments.
	if _, err := usecase.ResolveFor(ctx, f.q, f.org.ID, agent, deptscope.Optional{Set: true, Value: &f.support.UUID}); !errors.Is(err, model.ErrForbidden) {
		t.Fatalf("agent files under support: %v", err)
	}
	if d, err := usecase.ResolveFor(ctx, f.q, f.org.ID, agent, deptscope.Optional{Set: true, Value: &f.sales.UUID}); err != nil || d.Int64 != f.sales.ID {
		t.Fatalf("agent files under sales: %v %v", d, err)
	}

	// Sharing: only a lead of the owning department shares an item.
	agentV := f.viewer(t, "agent", rbac.PermTenantDepartmentsRead)
	leadV := f.viewer(t, "lead", rbac.PermTenantDepartmentsRead)
	if _, err := f.svc.Share(ctx, f.org.ID, f.users["agent"].ID, agentV, f.support.UUID, model.ShareInput{ResourceKind: model.KindArticle, ResourceUUID: salesArticle.Uuid}); !errors.Is(err, model.ErrForbidden) {
		t.Fatalf("member shares = %v", err)
	}
	share, err := f.svc.Share(ctx, f.org.ID, f.users["lead"].ID, leadV, f.sales.UUID, model.ShareInput{ResourceKind: model.KindArticle, ResourceUUID: supportArticle.Uuid})
	if err != nil || share.Owner == nil || share.Owner.UUID != f.support.UUID || share.Department.UUID != f.sales.UUID || share.ResourceTitle != "Destek" {
		t.Fatalf("lead share = %+v %v", share, err)
	}
	if got := search(&agent); !slices.Equal(got, []string{"Destek", "Genel", "Satış"}) {
		t.Fatalf("agent after share sees %v", got)
	}
	if list, err := f.svc.Shares(ctx, f.org.ID, f.support.UUID); err != nil || len(list) != 1 {
		t.Fatalf("support shares = %+v %v", list, err)
	}
	if err := f.svc.Unshare(ctx, f.org.ID, agentV, f.sales.UUID, share.UUID); !errors.Is(err, model.ErrForbidden) {
		t.Fatalf("member unshares = %v", err)
	}
	if err := f.svc.Unshare(ctx, f.org.ID, leadV, f.sales.UUID, share.UUID); err != nil {
		t.Fatal(err)
	}
	if got := search(&agent); !slices.Equal(got, []string{"Genel", "Satış"}) {
		t.Fatalf("agent after unshare sees %v", got)
	}

	// Business context sections follow the rule too.
	for _, d := range []pgtype.Int8{{}, {Int64: f.support.ID, Valid: true}} {
		sec, err := f.q.BusinessContextCreate(ctx, db.BusinessContextCreateParams{OrganizationID: f.org.ID, Kind: "other", Title: "Bölüm", Body: "x", Active: true, UpdatedBy: f.users["owner"].ID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.q.SetBusinessContextDepartment(ctx, db.SetBusinessContextDepartmentParams{OrganizationID: f.org.ID, Uuid: sec.Uuid, DepartmentID: d}); err != nil {
			t.Fatal(err)
		}
	}
	restrict, ids := agent.Args()
	sections, err := f.q.BusinessContextList(ctx, db.BusinessContextListParams{OrganizationID: f.org.ID, Limit: 20, DeptRestrict: restrict, ViewerDepartments: ids})
	if err != nil || len(sections) != 1 || sections[0].DepartmentID.Valid {
		t.Fatalf("agent sections = %+v %v", sections, err)
	}

	// A department that owns knowledge is not deleted unless it is moved.
	var inUse *model.InUseError
	if err := f.svc.Delete(ctx, f.org.ID, f.support.UUID, deptscope.Optional{}); !errors.As(err, &inUse) || inUse.Usage.Articles != 1 || inUse.Usage.ContextSections != 1 {
		t.Fatalf("delete in use = %v", err)
	}
	if err := f.svc.Delete(ctx, f.org.ID, f.support.UUID, deptscope.Optional{Set: true, Value: &f.sales.UUID}); err != nil {
		t.Fatal(err)
	}
	if got := search(&agent); !slices.Equal(got, []string{"Destek", "Genel", "Satış"}) {
		t.Fatalf("moved knowledge = %v", got)
	}
	var teamDept pgtype.Int8
	if err := f.pool.QueryRow(ctx, "SELECT department_id FROM teams WHERE id = $1", f.teamSup.ID).Scan(&teamDept); err != nil || teamDept.Valid {
		t.Fatalf("team kept deleted department: %v %v", teamDept, err)
	}
}

type classifier struct {
	calls int
	out   model.Prediction
}

func (c *classifier) ClassifyDepartment(_ context.Context, _, _ int64, transcript, _ string, candidates []model.Candidate) (model.Prediction, error) {
	c.calls++
	if transcript == "" || len(candidates) == 0 {
		return model.Prediction{}, errors.New("no input")
	}
	return c.out, nil
}

type inbox struct {
	updates []convusecase.UpdateInput
	events  []map[string]any
}

func (i *inbox) Update(_ context.Context, in convusecase.UpdateInput) (db.Conversation, error) {
	i.updates = append(i.updates, in)
	return db.Conversation{}, nil
}

func (i *inbox) SystemEvent(_ context.Context, _, _ int64, _ convusecase.Actor, attrs map[string]any) error {
	i.events = append(i.events, attrs)
	return nil
}

func (f *fixture) conversation(t *testing.T, n int) (int64, uuid.UUID) {
	t.Helper()
	var id int64
	var u uuid.UUID
	if err := f.pool.QueryRow(f.ctx, "INSERT INTO conversations (organization_id, number, channel_type, contact_id, waiting_since) VALUES ($1, $2, 'web', $3, NOW()) RETURNING id, uuid",
		f.org.ID, n, f.contact).Scan(&id, &u); err != nil {
		t.Fatal(err)
	}
	return id, u
}

// TestConversationDepartment covers the widget pick, the AI's one-time
// prediction with its confidence threshold and a transfer with context.
func TestConversationDepartment(t *testing.T) {
	f := setup(t)
	ctx := f.ctx
	cls := &classifier{}
	box := &inbox{}
	f.svc.Classifier, f.svc.Conversations = cls, box

	// Widget: only visible departments are offered and accepted.
	hidden := false
	if _, err := f.svc.Update(ctx, f.org.ID, f.support.UUID, model.Input{WidgetVisible: &hidden}); err != nil {
		t.Fatal(err)
	}
	offered, err := f.svc.Widget(ctx, f.org.ID, pgtype.UUID{})
	if err != nil || len(offered) != 1 || offered[0].UUID != f.sales.UUID {
		t.Fatalf("widget offers %+v %v", offered, err)
	}
	c1, c1u := f.conversation(t, 1)
	if err := f.svc.PickFromWidget(ctx, f.org.ID, c1, f.support.UUID); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("hidden pick = %v", err)
	}
	if err := f.svc.PickFromWidget(ctx, f.org.ID, c1, f.sales.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Route(ctx, f.org.ID, c1, "fiyat", "tr"); err != nil || cls.calls != 0 {
		t.Fatalf("routed a picked conversation: %d %v", cls.calls, err)
	}
	got, err := f.svc.Conversation(ctx, f.org.ID, c1u)
	if err != nil || got.Department == nil || got.Department.UUID != f.sales.UUID || *got.Source != model.SourceCustomer || len(got.Transfers) != 1 {
		t.Fatalf("picked = %+v %v", got, err)
	}

	// AI prediction above the threshold routes once.
	c2, c2u := f.conversation(t, 2)
	cls.out = model.Prediction{DepartmentUUID: &f.support.UUID, Confidence: 0.9, Reason: "teknik arıza"}
	if p, err := f.svc.Route(ctx, f.org.ID, c2, "modem çalışmıyor", "tr"); err != nil || p == nil || cls.calls != 1 {
		t.Fatalf("route = %+v %v", p, err)
	}
	if _, err := f.svc.Route(ctx, f.org.ID, c2, "modem çalışmıyor", "tr"); err != nil || cls.calls != 1 {
		t.Fatalf("routed twice: %d %v", cls.calls, err)
	}
	got, _ = f.svc.Conversation(ctx, f.org.ID, c2u)
	if got.Department == nil || got.Department.UUID != f.support.UUID || *got.Source != model.SourceAI || got.Confidence == nil || *got.Confidence < 0.89 {
		t.Fatalf("predicted = %+v", got)
	}
	// Below the threshold (or an unknown uuid) the conversation stays
	// organization-wide and is not predicted again.
	c3, c3u := f.conversation(t, 3)
	cls.out = model.Prediction{DepartmentUUID: &f.sales.UUID, Confidence: 0.3}
	if _, err := f.svc.Route(ctx, f.org.ID, c3, "merhaba", "tr"); err != nil {
		t.Fatal(err)
	}
	got, _ = f.svc.Conversation(ctx, f.org.ID, c3u)
	if got.Department != nil || got.RoutedAt == nil || len(got.Transfers) != 0 {
		t.Fatalf("low confidence = %+v", got)
	}
	c4, c4u := f.conversation(t, 4)
	ghost := uuid.New()
	cls.out = model.Prediction{DepartmentUUID: &ghost, Confidence: 0.99}
	if _, err := f.svc.Route(ctx, f.org.ID, c4, "selam", "tr"); err != nil {
		t.Fatal(err)
	}
	if got, _ = f.svc.Conversation(ctx, f.org.ID, c4u); got.Department != nil {
		t.Fatalf("unknown department routed: %+v", got)
	}

	// Transfer with context: the support team is released, the timeline
	// and the history carry the reason and summary.
	if _, err := f.pool.Exec(ctx, "UPDATE conversations SET assignee_team_id = $1 WHERE id = $2", f.teamSup.ID, c2); err != nil {
		t.Fatal(err)
	}
	out, err := f.svc.Transfer(ctx, f.org.ID, c2u, usecase.Actor{Kind: model.SourceAgent, UserID: f.users["lead"].ID},
		model.TransferInput{DepartmentUUID: &f.sales.UUID, Reason: "Fiyat teklifi istiyor", ContextSummary: "Modem sorunu çözüldü, yeni paket soruyor."})
	if err != nil {
		t.Fatal(err)
	}
	if out.Department == nil || out.Department.UUID != f.sales.UUID || *out.Source != model.SourceAgent || len(out.Transfers) != 2 {
		t.Fatalf("transfer = %+v", out)
	}
	last := out.Transfers[0]
	if last.Reason != "Fiyat teklifi istiyor" || last.ContextSummary == "" || last.From == nil || last.From.UUID != f.support.UUID || last.ActorUUID == nil || *last.ActorUUID != f.users["lead"].Uuid {
		t.Fatalf("transfer history = %+v", last)
	}
	if len(box.updates) != 1 || box.updates[0].Team == nil || *box.updates[0].Team != nil {
		t.Fatalf("team not released: %+v", box.updates)
	}
	ev := box.events[len(box.events)-1]
	if ev["event"] != "department_transferred" || ev["to"] != "Satış" || ev["from"] != "Teknik destek" || ev["context_summary"] == "" {
		t.Fatalf("timeline event = %v", ev)
	}
	// The AI may transfer too; a transfer to the same department is a no-op.
	if _, err := f.svc.Transfer(ctx, f.org.ID, c3u, usecase.Actor{Kind: model.SourceAI}, model.TransferInput{DepartmentUUID: &f.support.UUID, Reason: "teknik"}); err != nil {
		t.Fatal(err)
	}
	before := len(box.events)
	if _, err := f.svc.Transfer(ctx, f.org.ID, c3u, usecase.Actor{Kind: model.SourceAI}, model.TransferInput{DepartmentUUID: &f.support.UUID}); err != nil || len(box.events) != before {
		t.Fatalf("same-department transfer wrote %d events: %v", len(box.events)-before, err)
	}
	var unknown *usecase.ValidationError
	if _, err := f.svc.Transfer(ctx, f.org.ID, c3u, usecase.Actor{Kind: model.SourceAgent}, model.TransferInput{DepartmentUUID: &ghost}); !errors.As(err, &unknown) {
		t.Fatalf("unknown target = %v", err)
	}
}

type loader map[uuid.UUID]int64

func (l loader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: l[id]}, nil
}

// TestDepartmentRoutes covers the HTTP permissions (CF-215): members read
// departments and see a conversation's department; only managers create.
func TestDepartmentRoutes(t *testing.T) {
	f := setup(t)
	ids := loader{}
	for _, u := range f.users {
		ids[u.Uuid] = u.ID
	}
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	departments.RegisterRoutes(mux, handler.New(f.svc, f.q, nil), tokens, ids, f.orgSvc)
	call := func(as, method, path string, body any) (int, map[string]any) {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		access, _, _ := tokens.IssueAccess(jwt.AccessInput{UserID: f.users[as].Uuid})
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, f.org.Slug)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return rr.Code, env.Data
	}
	if code, _ := call("agent", http.MethodPost, "/v1/tenant/departments", map[string]any{"name": "Faturalama"}); code != http.StatusForbidden {
		t.Fatalf("agent create = %d", code)
	}
	code, created := call("owner", http.MethodPost, "/v1/tenant/departments", map[string]any{
		"name": "Faturalama", "color": "#16A34A", "ai": map[string]any{"enabled": false, "instructions": "Fatura numarasını sor."},
		"working_hours": map[string]any{"days": []map[string]any{{"day": "mon", "open": true, "start": "09:00", "end": "17:00"}}}, "timezone": "Europe/Istanbul",
	})
	if code != http.StatusCreated || created["ai"].(map[string]any)["enabled"] != false || created["working_hours"] == nil {
		t.Fatalf("owner create = %d %v", code, created)
	}
	if code, page := call("agent", http.MethodGet, "/v1/tenant/departments?mine=true", nil); code != http.StatusOK || page["total"].(float64) != 1 {
		t.Fatalf("agent mine = %d %v", code, page)
	}
	if code, body := call("owner", http.MethodPatch, "/v1/tenant/departments/"+created["uuid"].(string), map[string]any{"working_hours": map[string]any{"days": []map[string]any{{"day": "xxx"}}}}); code != http.StatusBadRequest {
		t.Fatalf("bad hours = %d %v %s", code, body, created["uuid"])
	}
	_, conv := f.conversation(t, 9)
	if code, body := call("agent", http.MethodGet, "/v1/tenant/conversations/"+conv.String()+"/department", nil); code != http.StatusOK || body["department"] != nil {
		t.Fatalf("conversation department = %d %v", code, body)
	}
	if code, _ := call("owner", http.MethodDelete, "/v1/tenant/departments/"+created["uuid"].(string), nil); code != http.StatusOK {
		t.Fatalf("delete = %d", code)
	}
}
