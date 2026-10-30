package inbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/inbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments"
	attachmentshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/handler"
	attachmentsmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/model"
	attachmentsrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/repository"
	attachmentsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	convhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/handler"
	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	convrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	presencemodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/presence"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// visLoader maps token subjects to principals (realtime needs auth.session).
type visLoader map[uuid.UUID]int64

func (l visLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: l[id], Permissions: []string{rbac.PermAuthSession}}, nil
}

type denyAll struct{}

func (denyAll) CanSubscribeChannel(context.Context, uuid.UUID, bool, string) (bool, error) {
	return false, nil
}

type channelRecorder struct {
	mu       sync.Mutex
	channels []string
}

func (r *channelRecorder) Publish(_ context.Context, channel string, _ any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels = append(r.channels, channel)
	return nil
}

// TestInboxIsolationAndTeamVisibility proves over HTTP (CF-44, CF-3, CF-5):
//
//   - cross-organization isolation of list, detail, messages, badges and
//     writes (a uuid of another organization is a 404, another
//     organization's header a 403);
//   - team visibility: an agent without read_all sees its own, its teams',
//     the unrouted and the participated conversations only, on every inbox
//     endpoint, the inbox search, the realtime channels and the live fan-out;
//     an owner / supervisor (read_all) sees all.
func TestInboxIsolationAndTeamVisibility(t *testing.T) {
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
	ids := visLoader{}
	for _, name := range []string{"owner", "supervisor", "a1", "a2", "outsider"} {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: "vis-" + name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "V", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		users[name] = u
		ids[u.Uuid] = u.ID
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), nil)
	orgA, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Vis A " + suffix, OwnerUserID: users["owner"].ID})
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Vis B " + suffix, OwnerUserID: users["outsider"].ID})
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
	for name, role := range map[string]string{"supervisor": "supervisor", "a1": "agent", "a2": "agent"} {
		if _, err := orgSvc.AddMember(ctx, orgA.UUID, users[name].Uuid, role); err != nil {
			t.Fatal(err)
		}
	}
	// Teams: a1 in L1, a2 in L2.
	team := func(name string, member int64) int64 {
		var id int64
		if err := pool.QueryRow(ctx, "INSERT INTO teams (organization_id, name) VALUES ($1, $2) RETURNING id", orgA.ID, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO team_members (organization_id, team_id, user_id) VALUES ($1, $2, $3)", orgA.ID, id, member); err != nil {
			t.Fatal(err)
		}
		return id
	}
	l1, l2 := team("L1", users["a1"].ID), team("L2", users["a2"].ID)

	number := int64(0)
	conv := func(org int64, subject string, assignee, teamID int64) db.Conversation {
		t.Helper()
		number++
		ct, err := q.InsertContact(ctx, db.InsertContactParams{OrganizationID: org, Name: "Kişi " + subject, CustomFields: []byte("{}")})
		if err != nil {
			t.Fatal(err)
		}
		p := db.InsertConversationParams{
			OrganizationID: org, Number: number, ChannelType: "test", ContactID: ct.ID, Subject: subject,
			Status: "open", Priority: "normal", Source: "inbound", CustomFields: []byte("{}"),
			LastMessageAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		}
		if assignee > 0 {
			p.AssigneeUserID = pgtype.Int8{Int64: assignee, Valid: true}
		}
		if teamID > 0 {
			p.AssigneeTeamID = pgtype.Int8{Int64: teamID, Valid: true}
		}
		c, err := q.InsertConversation(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	cTeam1 := conv(orgA.ID, "team-l1", 0, l1)
	cA2 := conv(orgA.ID, "assigned-a2", users["a2"].ID, l2)
	cQueue := conv(orgA.ID, "unrouted", 0, 0)
	cWatch := conv(orgA.ID, "watched-l2", 0, l2)
	cB := conv(orgB.ID, "other-org", 0, 0)
	if _, err := q.AddConversationParticipant(ctx, db.AddConversationParticipantParams{
		OrganizationID: orgA.ID, ConversationID: cWatch.ID, UserID: users["a1"].ID, Role: convmodel.ParticipantWatcher,
	}); err != nil {
		t.Fatal(err)
	}

	// The inbox with a search index that matches every conversation of A
	// (and, wrongly, B's) with a snippet: visibility must still decide.
	finder := fakeFinder{matches: []convusecase.SearchMatch{
		{ConversationUUID: cTeam1.Uuid, Snippet: "iade l1"}, {ConversationUUID: cA2.Uuid, Snippet: "iade gizli"},
		{ConversationUUID: cQueue.Uuid, Snippet: "iade kuyruk"}, {ConversationUUID: cWatch.Uuid, Snippet: "iade izlenen"},
		{ConversationUUID: cB.Uuid, Snippet: "iade başka kurum"},
	}}
	svc := convusecase.New(convrepository.New(pool, q), nil, slog.Default()).WithSearch(nil, finder)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := realtime.NewTokenIssuer(config.CentrifugoConfig{Enabled: true, TokenHMAC: "test-centrifugo-secret-0123456789"})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	conversations.RegisterRoutes(mux, convhandler.New(svc, nil), tokens, ids, orgSvc)
	attSvc := attachmentsusecase.New(attachmentsrepository.NewPostgres(pool, q), storage.NewMemory(), "vis-test-secret", config.AttachmentConfig{}, nil)
	attachments.RegisterRoutes(mux, attachmentshandler.New(attSvc, nil).WithOwnerAccess(access.AttachmentGuard{Q: q}), tokens, ids, orgSvc)
	realtime.RegisterRoutes(mux, realtime.NewHandler(issuer, presencemodule.Authorizer{
		Next: inbox.Authorizer{Next: denyAll{}, Resolver: orgSvc, Queries: q}, Resolver: orgSvc, Queries: q,
	}), tokens, ids)

	call := func(as, org, method, path string, body any) (int, json.RawMessage) {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		tok, _, _ := tokens.IssueAccess(jwt.AccessInput{UserID: users[as].Uuid})
		req.Header.Set("Authorization", "Bearer "+tok)
		if org != "" {
			req.Header.Set(orgctx.HeaderName, org)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return rr.Code, env.Data
	}
	list := func(t *testing.T, as, org, query string) []string {
		t.Helper()
		code, data := call(as, org, http.MethodGet, "/v1/tenant/conversations"+query, nil)
		if code != http.StatusOK {
			t.Fatalf("%s list%s = %d %s", as, query, code, data)
		}
		var page struct {
			Items []struct {
				Subject     string `json:"subject"`
				SearchMatch *struct {
					Snippet string `json:"snippet"`
				} `json:"search_match"`
			} `json:"items"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, it := range page.Items {
			out = append(out, it.Subject)
			if it.SearchMatch != nil && ((as == "a1" && it.SearchMatch.Snippet == "iade gizli") || (org == orgA.Slug && it.SearchMatch.Snippet == "iade başka kurum")) {
				t.Fatalf("%s saw a hidden snippet %q", as, it.SearchMatch.Snippet)
			}
		}
		sort.Strings(out)
		if page.Total != len(out) && query == "" {
			t.Fatalf("%s total = %d, items %v", as, page.Total, out)
		}
		return out
	}
	eq := func(t *testing.T, what string, got []string, want ...string) {
		t.Helper()
		sort.Strings(want)
		if len(got) != len(want) {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s = %v, want %v", what, got, want)
			}
		}
	}
	all := []string{"team-l1", "assigned-a2", "unrouted", "watched-l2"}

	t.Run("list", func(t *testing.T) {
		eq(t, "owner", list(t, "owner", orgA.Slug, ""), all...)
		eq(t, "supervisor (read_all)", list(t, "supervisor", orgA.Slug, ""), all...)
		eq(t, "a1", list(t, "a1", orgA.Slug, ""), "team-l1", "unrouted", "watched-l2")
		eq(t, "a2", list(t, "a2", orgA.Slug, ""), "assigned-a2", "unrouted", "watched-l2")
		// Filters narrow, never widen.
		eq(t, "a1 assignee=a2", list(t, "a1", orgA.Slug, "?assignee="+users["a2"].Uuid.String()))
		eq(t, "a1 status=all", list(t, "a1", orgA.Slug, "?status=open,pending,snoozed,resolved,closed"), "team-l1", "unrouted", "watched-l2")
		// Inbox search: index matches outside the rule are dropped.
		eq(t, "a1 search", list(t, "a1", orgA.Slug, "?q=iade"), "team-l1", "unrouted", "watched-l2")
		eq(t, "owner search", list(t, "owner", orgA.Slug, "?q=iade"), all...)
		// Organization B only ever shows B.
		eq(t, "outsider", list(t, "outsider", orgB.Slug, ""), "other-org")
		eq(t, "outsider search", list(t, "outsider", orgB.Slug, "?q=iade"), "other-org")
	})

	t.Run("counts", func(t *testing.T) {
		for as, want := range map[string]float64{"owner": 4, "a1": 3, "a2": 3, "outsider": 1} {
			org := orgA.Slug
			if as == "outsider" {
				org = orgB.Slug
			}
			code, data := call(as, org, http.MethodGet, "/v1/tenant/conversations/counts", nil)
			var c map[string]any
			_ = json.Unmarshal(data, &c)
			if code != http.StatusOK || c["open"].(float64) != want || c["by_status"].(map[string]any)["open"].(float64) != want {
				t.Fatalf("%s counts = %d %v, want open %v", as, code, c, want)
			}
		}
	})

	t.Run("single conversation endpoints", func(t *testing.T) {
		paths := func(c db.Conversation) map[string][2]string {
			id := c.Uuid.String()
			return map[string][2]string{
				"get":      {http.MethodGet, "/v1/tenant/conversations/" + id},
				"messages": {http.MethodGet, "/v1/tenant/conversations/" + id + "/messages"},
				"read":     {http.MethodPost, "/v1/tenant/conversations/" + id + "/read"},
				"note":     {http.MethodPost, "/v1/tenant/conversations/" + id + "/messages"},
				"patch":    {http.MethodPatch, "/v1/tenant/conversations/" + id},
				"tag":      {http.MethodPost, "/v1/tenant/conversations/" + id + "/tags"},
			}
		}
		bodies := map[string]any{
			"note": map[string]any{"content": "iç not", "private": true}, "patch": map[string]any{"priority": "high"},
			"tag": map[string]any{"name": "vip"},
		}
		// Hidden by the team rule (a1 → a2's conversation) and by the
		// organization (owner of A → B's conversation): both 404.
		for who, c := range map[string]db.Conversation{"a1": cA2, "owner": cB} {
			for name, ep := range paths(c) {
				if code, data := call(who, orgA.Slug, ep[0], ep[1], bodies[name]); code != http.StatusNotFound {
					t.Fatalf("%s %s on hidden %s = %d %s", who, name, c.Subject, code, data)
				}
			}
		}
		// Visible ones answer for a1 (team, queue, participant).
		for _, c := range []db.Conversation{cTeam1, cQueue, cWatch} {
			for name, ep := range paths(c) {
				code, data := call("a1", orgA.Slug, ep[0], ep[1], bodies[name])
				if code != http.StatusOK && code != http.StatusCreated {
					t.Fatalf("a1 %s on %s = %d %s", name, c.Subject, code, data)
				}
			}
		}
		// Nothing a1 tried on the hidden conversation landed.
		var notes int
		_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM messages WHERE organization_id = $1 AND conversation_id = $2", orgA.ID, cA2.ID).Scan(&notes)
		var prio string
		_ = pool.QueryRow(ctx, "SELECT priority FROM conversations WHERE id = $1", cA2.ID).Scan(&prio)
		if notes != 0 || prio != "normal" {
			t.Fatalf("hidden conversation changed: %d messages, priority %s", notes, prio)
		}
		// Another organization's header: not a member.
		if code, _ := call("a1", orgB.Slug, http.MethodGet, "/v1/tenant/conversations", nil); code != http.StatusForbidden && code != http.StatusNotFound {
			t.Fatalf("a1 in org B = %d", code)
		}
	})

	t.Run("message attachments", func(t *testing.T) {
		// The owner leaves a note with a file on a2's conversation and on
		// L1's queue; a1 reads only the L1 one (docs/design/tenant-core.md §14).
		file := func(c db.Conversation) uuid.UUID {
			t.Helper()
			code, data := call("owner", orgA.Slug, http.MethodPost, "/v1/tenant/conversations/"+c.Uuid.String()+"/messages",
				map[string]any{"content": "dosya notu", "private": true})
			if code != http.StatusCreated && code != http.StatusOK {
				t.Fatalf("note on %s = %d %s", c.Subject, code, data)
			}
			var m struct {
				UUID uuid.UUID `json:"uuid"`
			}
			_ = json.Unmarshal(data, &m)
			att, err := attSvc.Upload(ctx, attachmentsmodel.UploadInput{
				OrganizationID: orgA.ID, OrganizationUUID: orgA.UUID, UploaderID: users["owner"].ID,
				Filename: "fatura.txt", DeclaredType: "text/plain", OwnerType: attachmentsmodel.OwnerMessage, OwnerID: &m.UUID,
				Body: bytes.NewReader([]byte("gizli")),
			})
			if err != nil {
				t.Fatal(err)
			}
			return att.UUID
		}
		hidden, visible := file(cA2), file(cTeam1)
		paths := func(id uuid.UUID) map[string][2]string {
			p := "/v1/tenant/attachments/" + id.String()
			return map[string][2]string{
				"get": {http.MethodGet, p}, "download": {http.MethodGet, p + "/download"},
				"signed": {http.MethodPost, p + "/signed-url"}, "delete": {http.MethodDelete, p},
			}
		}
		for name, ep := range paths(hidden) {
			if code, data := call("a1", orgA.Slug, ep[0], ep[1], nil); code != http.StatusNotFound {
				t.Fatalf("a1 %s hidden attachment = %d %s", name, code, data)
			}
		}
		for _, name := range []string{"get", "download", "signed"} {
			ep := paths(visible)[name]
			if code, data := call("a1", orgA.Slug, ep[0], ep[1], nil); code != http.StatusOK {
				t.Fatalf("a1 %s visible attachment = %d %s", name, code, data)
			}
			ep = paths(hidden)[name]
			if code, data := call("owner", orgA.Slug, ep[0], ep[1], nil); code != http.StatusOK {
				t.Fatalf("owner %s attachment = %d %s", name, code, data)
			}
		}
		listed := func(as string) map[uuid.UUID]bool {
			code, data := call(as, orgA.Slug, http.MethodGet, "/v1/tenant/attachments", nil)
			var page struct {
				Items []struct {
					UUID uuid.UUID `json:"uuid"`
				} `json:"items"`
			}
			_ = json.Unmarshal(data, &page)
			if code != http.StatusOK {
				t.Fatalf("%s list attachments = %d %s", as, code, data)
			}
			out := map[uuid.UUID]bool{}
			for _, it := range page.Items {
				out[it.UUID] = true
			}
			return out
		}
		if l := listed("a1"); l[hidden] || !l[visible] {
			t.Fatalf("a1 attachment list = %v", l)
		}
		if l := listed("owner"); !l[hidden] || !l[visible] {
			t.Fatalf("owner attachment list = %v", l)
		}
	})

	t.Run("realtime subscriptions", func(t *testing.T) {
		sub := func(as, channel string) int {
			code, _ := call(as, "", http.MethodPost, "/v1/realtime/subscription-token", map[string]any{"channel": channel})
			return code
		}
		orgWide := realtime.InboxChannel(orgA.UUID)
		cases := []struct {
			as, channel string
			want        int
		}{
			{"owner", orgWide, http.StatusOK},
			{"supervisor", orgWide, http.StatusOK},
			{"a1", orgWide, http.StatusForbidden},
			{"a1", realtime.InboxUserChannel(orgA.UUID, users["a1"].Uuid), http.StatusOK},
			{"a1", realtime.InboxUserChannel(orgA.UUID, users["a2"].Uuid), http.StatusForbidden},
			{"outsider", orgWide, http.StatusForbidden},
			{"outsider", realtime.InboxUserChannel(orgA.UUID, users["outsider"].Uuid), http.StatusForbidden},
			{"a1", realtime.ConversationChannel(cTeam1.Uuid), http.StatusOK},
			{"a1", realtime.ConversationChannel(cWatch.Uuid), http.StatusOK},
			{"a1", realtime.ConversationChannel(cA2.Uuid), http.StatusForbidden},
			{"owner", realtime.ConversationChannel(cA2.Uuid), http.StatusOK},
			{"owner", realtime.ConversationChannel(cB.Uuid), http.StatusForbidden},
			{"outsider", realtime.ConversationChannel(cB.Uuid), http.StatusOK},
		}
		for _, c := range cases {
			if got := sub(c.as, c.channel); got != c.want {
				t.Fatalf("%s subscribe %s = %d, want %d", c.as, c.channel, got, c.want)
			}
		}
	})

	t.Run("live fan-out", func(t *testing.T) {
		rec := &channelRecorder{}
		n := inbox.NewLiveNotifier(rec, q, slog.Default())
		got := func(c db.Conversation) map[string]bool {
			rec.channels = nil
			n.Notify(ctx, orgA.ID, convmodel.LiveEvent{Type: "message.created", ConversationUUID: c.Uuid})
			out := map[string]bool{}
			for _, ch := range rec.channels {
				out[ch] = true
			}
			return out
		}
		userCh := func(name string) string { return realtime.InboxUserChannel(orgA.UUID, users[name].Uuid) }
		team1 := got(cTeam1)
		if !team1[realtime.InboxChannel(orgA.UUID)] || !team1[userCh("a1")] || team1[userCh("a2")] {
			t.Fatalf("team-l1 fan-out = %v", team1)
		}
		hidden := got(cA2)
		if hidden[userCh("a1")] || !hidden[userCh("a2")] {
			t.Fatalf("assigned-a2 fan-out = %v", hidden)
		}
		queue := got(cQueue)
		if !queue[userCh("a1")] || !queue[userCh("a2")] {
			t.Fatalf("unrouted fan-out = %v", queue)
		}
		if watched := got(cWatch); !watched[userCh("a1")] || !watched[userCh("a2")] {
			t.Fatalf("watched fan-out = %v", watched)
		}
		// Another organization's conversation id never fans out in A.
		if other := got(cB); len(other) != 1 {
			t.Fatalf("cross-org fan-out = %v", other)
		}

		// Reassigning the unrouted conversation to a2 also tells a1, who saw
		// it in the queue, so a1's inbox drops the row without a reload.
		rec.channels = nil
		live := convusecase.New(convrepository.New(pool, q), nil, slog.Default()).WithNotifier(n)
		a2 := users["a2"].Uuid
		a2p := &a2
		if _, err := live.Update(ctx, convusecase.UpdateInput{OrganizationID: orgA.ID, ConversationUUID: cQueue.Uuid, Assignee: &a2p}); err != nil {
			t.Fatal(err)
		}
		reassigned := map[string]bool{}
		for _, ch := range rec.channels {
			reassigned[ch] = true
		}
		if !reassigned[userCh("a1")] || !reassigned[userCh("a2")] {
			t.Fatalf("reassignment fan-out = %v", reassigned)
		}
	})
}
