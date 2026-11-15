package contacts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	coremodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	core "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/usecase"
	convrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
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

// fakeAI stands in for the AI engine and records what it was asked.
type fakeAI struct {
	mu     sync.Mutex
	inputs []string
	err    error
}

func (f *fakeAI) RunAgent(_ context.Context, _, _ int64, in coremodel.RunInput, o core.AgentOptions) (core.RunResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs = append(f.inputs, in.Input)
	if f.err != nil {
		return core.RunResult{}, f.err
	}
	if o.Schema == nil || o.Purpose != "contact_summary" {
		return core.RunResult{}, ai.ErrInvalidOutput
	}
	out := core.RunResult{UUID: uuid.New()}
	out.Text = `{"summary":"Fatura konusunda iki kez yazdı.","open_topics":["İade bekliyor"," "],"sentiment":"negative"}`
	return out, nil
}

// TestContactProfileRoutes covers the contact page endpoints over HTTP with
// the real organization resolver (CF-221): permission denial, team
// visibility, tenant isolation, timeline paging and search, and the cached
// AI summary.
func TestContactProfileRoutes(t *testing.T) {
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
	for _, name := range []string{"owner", "agent", "light", "billing", "outsider"} {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: "cp-" + name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "T", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		users[name] = u
		ids[u.Uuid] = u.ID
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), nil)
	orgA, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Profile A " + suffix, OwnerUserID: users["owner"].ID})
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Profile B " + suffix, OwnerUserID: users["outsider"].ID})
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
	for name, role := range map[string]string{"agent": "agent", "light": "light_agent", "billing": "billing"} {
		if _, err := orgSvc.AddMember(ctx, orgA.UUID, users[name].Uuid, role); err != nil {
			t.Fatal(name, err)
		}
	}

	store := convrepository.New(pool, q)
	fake := &fakeAI{}
	svc := usecase.New(store).WithAI(fake)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	contacts.RegisterRoutes(mux, handler.New(svc, nil), tokens, ids, orgSvc)
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
			Data json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		var obj map[string]any
		_ = json.Unmarshal(env.Data, &obj)
		return rr.Code, obj
	}
	a, b := orgA.UUID.String(), orgB.UUID.String()

	// A contact of A with: an unrouted open conversation (every agent sees
	// it), one assigned to a team the agent is not in, a resolved one with a
	// CSAT rating, a consent record and a KVKK request.
	ayse, err := svc.CreateContact(ctx, orgA.ID, usecase.ContactInput{Name: ptr("Ayşe Demo"), Email: ptr("ayse@example.test")})
	if err != nil {
		t.Fatal(err)
	}
	var contactID, teamID int64
	if err := pool.QueryRow(ctx, "SELECT id FROM contacts WHERE uuid = $1", ayse.UUID).Scan(&contactID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO teams (organization_id, name) VALUES ($1, 'Billing desk') RETURNING id", orgA.ID).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-72 * time.Hour).UTC().Truncate(time.Second)
	var openConv, teamConv, doneConv int64
	if err := pool.QueryRow(ctx, `INSERT INTO conversations (organization_id, number, channel_type, contact_id, subject, status, created_at, last_message_at)
		VALUES ($1, 1, 'email', $2, 'Fatura sorunu', 'open', $3, $3) RETURNING id`, orgA.ID, contactID, base).Scan(&openConv); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO conversations (organization_id, number, channel_type, contact_id, subject, status, assignee_team_id, created_at, last_message_at, sla_breached_at)
		VALUES ($1, 2, 'whatsapp', $2, 'Gizli ekip işi', 'open', $3, $4, $4, $4) RETURNING id`, orgA.ID, contactID, teamID, base.Add(time.Hour)).Scan(&teamConv); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO conversations (organization_id, number, channel_type, contact_id, subject, status, created_at, last_message_at, resolved_at)
		VALUES ($1, 3, 'widget', $2, 'Kurulum', 'resolved', $3, $4, $4) RETURNING id`, orgA.ID, contactID, base.Add(2*time.Hour), base.Add(3*time.Hour)).Scan(&doneConv); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO conversation_ratings (organization_id, conversation_id, contact_id, score, comment, created_at)
		VALUES ($1, $2, $3, 5, 'Çok hızlı', $4)`, orgA.ID, doneConv, contactID, base.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO contact_consents (organization_id, contact_id, purpose, granted, notice_version, source, created_at)
		VALUES ($1, $2, 'marketing', true, 'v1', 'widget', $3)`, orgA.ID, contactID, base.Add(5*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO data_subject_requests (organization_id, contact_id, kind, created_at)
		VALUES ($1, $2, 'export', $3)`, orgA.ID, contactID, base.Add(6*time.Hour)); err != nil {
		t.Fatal(err)
	}
	path := "/v1/tenant/contacts/" + ayse.UUID.String()

	t.Run("permission denial", func(t *testing.T) {
		// No contacts.read: the whole page is forbidden.
		if code, _ := call("billing", a, "GET", path+"/overview", nil); code != http.StatusForbidden {
			t.Fatalf("billing overview = %d", code)
		}
		if code, _ := call("billing", a, "GET", path+"/timeline", nil); code != http.StatusForbidden {
			t.Fatalf("billing timeline = %d", code)
		}
		// No tenant.ai.use: no summary generation.
		if code, _ := call("light", a, "POST", path+"/ai-summary", nil); code != http.StatusForbidden {
			t.Fatalf("light_agent summary = %d", code)
		}
		// No contacts.write: no notes.
		if code, _ := call("billing", a, "POST", path+"/notes", map[string]any{"body": "x"}); code != http.StatusForbidden {
			t.Fatalf("billing note = %d", code)
		}
	})

	t.Run("sections follow permissions", func(t *testing.T) {
		code, owner := call("owner", a, "GET", path+"/overview", nil)
		if code != http.StatusOK {
			t.Fatalf("owner overview = %d", code)
		}
		sections := owner["sections"].(map[string]any)
		if sections["compliance"] != true || sections["conversations"] != true || sections["ai"] != true {
			t.Fatalf("owner sections = %v", sections)
		}
		if n := len(owner["conversations"].([]any)); n != 3 {
			t.Fatalf("owner sees %d conversations", n)
		}
		stats := owner["stats"].(map[string]any)
		if stats["open_conversations"].(float64) != 2 || stats["sla_breached"].(float64) != 1 || stats["csat_average"].(float64) != 5 {
			t.Fatalf("owner stats = %v", stats)
		}
		if len(owner["consents"].([]any)) != 1 || len(owner["privacy_requests"].([]any)) != 1 {
			t.Fatalf("owner compliance = %v %v", owner["consents"], owner["privacy_requests"])
		}

		// The agent has no tenant.settings.write and no read_all.
		code, agent := call("agent", a, "GET", path+"/overview", nil)
		if code != http.StatusOK {
			t.Fatalf("agent overview = %d", code)
		}
		if s := agent["sections"].(map[string]any); s["compliance"] != false || s["surveys"] != false {
			t.Fatalf("agent sections = %v", s)
		}
		if len(agent["consents"].([]any)) != 0 || len(agent["privacy_requests"].([]any)) != 0 {
			t.Fatal("agent must not see consents or KVKK requests")
		}
		convs := agent["conversations"].([]any)
		if len(convs) != 2 {
			t.Fatalf("agent sees %d conversations, want 2 (team conversation hidden)", len(convs))
		}
		for _, c := range convs {
			if c.(map[string]any)["subject"] == "Gizli ekip işi" {
				t.Fatal("agent sees another team's conversation")
			}
		}
		if st := agent["stats"].(map[string]any); st["open_conversations"].(float64) != 1 || st["sla_breached"].(float64) != 0 {
			t.Fatalf("agent stats count hidden conversations: %v", st)
		}
		for _, k := range agent["timeline_kinds"].([]any) {
			if k == "consent" || k == "privacy_request" || k == "survey" {
				t.Fatalf("agent timeline kinds include %v", k)
			}
		}

		// Asking the timeline for compliance kinds only returns nothing.
		code, page := call("agent", a, "GET", path+"/timeline?kinds=consent,privacy_request", nil)
		if code != http.StatusOK || len(page["items"].([]any)) != 0 {
			t.Fatalf("agent compliance timeline = %d %v", code, page)
		}
		code, page = call("owner", a, "GET", path+"/timeline?kinds=consent,privacy_request", nil)
		if code != http.StatusOK || len(page["items"].([]any)) != 2 {
			t.Fatalf("owner compliance timeline = %d %v", code, page)
		}
		code, page = call("agent", a, "GET", path+"/timeline?kinds=conversation_opened", nil)
		if code != http.StatusOK || len(page["items"].([]any)) != 2 {
			t.Fatalf("agent conversation timeline = %d %v", code, page)
		}
		if code, _ := call("owner", a, "GET", path+"/timeline?kinds=bogus", nil); code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
			t.Fatalf("unknown kind = %d", code)
		}
	})

	t.Run("tenant isolation", func(t *testing.T) {
		// The outsider owns B; A's contact does not exist there.
		for _, p := range []string{"/overview", "/timeline"} {
			if code, _ := call("outsider", b, "GET", path+p, nil); code != http.StatusNotFound {
				t.Fatalf("outsider %s in own org = %d", p, code)
			}
		}
		// And the outsider is not a member of A.
		if code, _ := call("outsider", a, "GET", path+"/overview", nil); code == http.StatusOK {
			t.Fatal("outsider read A's contact")
		}
		if code, _ := call("outsider", b, "POST", path+"/notes", map[string]any{"body": "sızıntı"}); code != http.StatusNotFound {
			t.Fatalf("outsider note = %d", code)
		}
	})

	t.Run("notes, paging and search", func(t *testing.T) {
		var noteIDs []string
		for _, body := range []string{"İlk not", "İkinci not", "Üçüncü not iade", "Dördüncü not", "Beşinci 100% not"} {
			code, n := call("agent", a, "POST", path+"/notes", map[string]any{"body": body})
			if code != http.StatusCreated {
				t.Fatalf("note = %d", code)
			}
			noteIDs = append(noteIDs, n["uuid"].(string))
		}
		seen := map[string]bool{}
		next := ""
		pages := 0
		for {
			url := path + "/timeline?kinds=note&limit=2"
			if next != "" {
				url += "&before=" + next
			}
			code, page := call("owner", a, "GET", url, nil)
			if code != http.StatusOK {
				t.Fatalf("page = %d", code)
			}
			pages++
			for _, it := range page["items"].([]any) {
				ref := it.(map[string]any)["ref"].(string)
				if seen[ref] {
					t.Fatalf("note %s repeated across pages", ref)
				}
				seen[ref] = true
			}
			nb, _ := page["next_before"].(string)
			if nb == "" {
				break
			}
			next = nb
		}
		if len(seen) != 5 || pages != 3 {
			t.Fatalf("paged %d notes over %d pages", len(seen), pages)
		}
		if code, _ := call("owner", a, "GET", path+"/timeline?before=not-a-cursor", nil); code == http.StatusOK {
			t.Fatal("bad cursor accepted")
		}
		// Search is literal: "%" does not match everything.
		code, page := call("owner", a, "GET", path+"/timeline?kinds=note&q=100%25", nil)
		if code != http.StatusOK || len(page["items"].([]any)) != 1 {
			t.Fatalf("search 100%% = %d %v", code, page)
		}
		code, page = call("owner", a, "GET", path+"/timeline?q=iade", nil)
		if code != http.StatusOK || len(page["items"].([]any)) != 1 {
			t.Fatalf("search iade = %d %v", code, page)
		}
		// Date range: before the contact's activity there is nothing.
		to := base.Add(-time.Hour).Format(time.RFC3339)
		code, page = call("owner", a, "GET", path+"/timeline?to="+to, nil)
		if code != http.StatusOK || len(page["items"].([]any)) != 0 {
			t.Fatalf("date range = %d %v", code, page)
		}
		if code, _ := call("owner", a, "GET", path+"/timeline?from=yesterday", nil); code == http.StatusOK {
			t.Fatal("bad from accepted")
		}
		// Only the author (or contacts.delete) removes a note.
		if code, _ := call("light", a, "DELETE", "/v1/tenant/contact-notes/"+noteIDs[0], nil); code != http.StatusForbidden {
			t.Fatalf("light_agent deletes another's note = %d", code)
		}
		if code, _ := call("outsider", b, "DELETE", "/v1/tenant/contact-notes/"+noteIDs[0], nil); code != http.StatusNotFound {
			t.Fatalf("outsider deletes A's note = %d", code)
		}
		if code, _ := call("agent", a, "DELETE", "/v1/tenant/contact-notes/"+noteIDs[0], nil); code != http.StatusOK {
			t.Fatalf("author deletes note = %d", code)
		}
	})

	t.Run("ai summary is cached per audience", func(t *testing.T) {
		code, s := call("owner", a, "POST", path+"/ai-summary?locale=tr", nil)
		if code != http.StatusOK || s["summary"] != "Fatura konusunda iki kez yazdı." || s["sentiment"] != "negative" {
			t.Fatalf("owner summary = %d %v", code, s)
		}
		if topics := s["open_topics"].([]any); len(topics) != 1 {
			t.Fatalf("blank topics kept: %v", topics)
		}
		if !strings.Contains(fake.inputs[0], "Gizli ekip işi") {
			t.Fatal("owner summary should read every conversation")
		}
		// The owner's summary comes back cached on the page...
		_, owner := call("owner", a, "GET", path+"/overview", nil)
		if cached, _ := owner["ai_summary"].(map[string]any); cached == nil || cached["stale"] != false {
			t.Fatalf("owner cached summary = %v", owner["ai_summary"])
		}
		// ...but the agent, who cannot see the team conversation, has none.
		_, agent := call("agent", a, "GET", path+"/overview", nil)
		if agent["ai_summary"] != nil {
			t.Fatal("agent sees a summary built from conversations they cannot open")
		}
		if code, _ := call("agent", a, "POST", path+"/ai-summary", nil); code != http.StatusOK {
			t.Fatalf("agent summary = %d", code)
		}
		if strings.Contains(fake.inputs[1], "Gizli ekip işi") {
			t.Fatal("agent summary read a hidden conversation")
		}
		// New activity marks the cached summary stale.
		if _, err := pool.Exec(ctx, "UPDATE conversations SET last_message_at = NOW() WHERE id = $1", openConv); err != nil {
			t.Fatal(err)
		}
		_, owner = call("owner", a, "GET", path+"/overview", nil)
		if owner["ai_summary"].(map[string]any)["stale"] != true {
			t.Fatal("summary not stale after new activity")
		}
		// AI business errors keep their API codes.
		fake.err = coremodel.ErrCredits
		if code, _ := call("owner", a, "POST", path+"/ai-summary", nil); code != http.StatusTooManyRequests {
			t.Fatalf("credits exhausted = %d", code)
		}
		fake.err = nil
	})

	t.Run("merge moves notes and drops summaries", func(t *testing.T) {
		dup, err := svc.CreateContact(ctx, orgA.ID, usecase.ContactInput{Name: ptr("Ayşe D.")})
		if err != nil {
			t.Fatal(err)
		}
		if code, _ := call("owner", a, "POST", "/v1/tenant/contacts/"+dup.UUID.String()+"/notes", map[string]any{"body": "Kopya kayıttaki not"}); code != http.StatusCreated {
			t.Fatalf("dup note = %d", code)
		}
		if _, err := svc.Merge(ctx, orgA.ID, ayse.UUID, dup.UUID); err != nil {
			t.Fatal(err)
		}
		_, page := call("owner", a, "GET", path+"/timeline?kinds=note&q=Kopya", nil)
		if len(page["items"].([]any)) != 1 {
			t.Fatalf("note not moved: %v", page)
		}
		var n int
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM contact_ai_summaries WHERE contact_id = $1", contactID).Scan(&n); err != nil || n != 0 {
			t.Fatalf("summaries after merge = %d %v", n, err)
		}
	})
}

func ptr[T any](v T) *T { return &v }
