package forum_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	aiuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	billmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	billrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forum"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forum/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forum/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forum/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	widgetuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	widgettoken "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type instanceSource struct{ instance channels.Instance }

func (i instanceSource) ForWebhook(context.Context, string, uuid.UUID) (channels.Instance, channels.Adapter, error) {
	return i.instance, nil, nil
}

type deniedAI struct{ calls int }

func (a *deniedAI) RunAgent(context.Context, int64, int64, aimodel.RunInput, aiuc.AgentOptions) (aiuc.RunResult, error) {
	a.calls++
	return aiuc.RunResult{}, aimodel.ErrCredits
}
func TestForumIsolationModerationNotificationsAndPublicProjection(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	suffix := uuid.NewString()
	var org, other, actor, author, subscriber int64
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('Forum',$1) RETURNING id", suffix).Scan(&org))
	must(pool.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('Other',$1) RETURNING id", suffix+"-other").Scan(&other))
	must(pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname,status) VALUES($1,'test','Forum','Agent','active') RETURNING id", suffix+"@example.test").Scan(&actor))
	must(pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Author') RETURNING id", org).Scan(&author))
	must(pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Subscriber') RETURNING id", org).Scan(&subscriber))
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM forum_notifications WHERE organization_id=$1", org)
		_, _ = pool.Exec(ctx, "DELETE FROM forum_votes WHERE organization_id=$1", org)
		_, _ = pool.Exec(ctx, "DELETE FROM forum_subscriptions WHERE organization_id=$1", org)
		_, _ = pool.Exec(ctx, "UPDATE forum_posts SET accepted_reply_id=NULL WHERE organization_id=$1", org)
		_, _ = pool.Exec(ctx, "DELETE FROM forum_posts WHERE organization_id=$1", org)
		_, _ = pool.Exec(ctx, "DELETE FROM forum_categories WHERE organization_id=$1", org)
		_, _ = pool.Exec(ctx, "DELETE FROM contacts WHERE organization_id=$1", org)
		_, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id=ANY($1::bigint[])", []int64{org, other})
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id=$1", actor)
	}()
	_, err = q.ForumCreateCategory(ctx, db.ForumCreateCategoryParams{OrganizationID: org, Slug: "general", Name: "General"})
	must(err)
	ai := &deniedAI{}
	s := &usecase.Service{Repo: billrepo.New(pool, q), AI: ai}
	topic, err := s.Create(ctx, org, author, nil, model.PostInput{Category: "general", Title: "Question", Body: "How does this work?"})
	must(err)
	n, err := q.ForumTopicCount(ctx, db.ForumTopicCountParams{OrganizationID: org})
	must(err)
	if n != 0 {
		t.Fatal("pending topic visible")
	}
	_, err = s.Moderate(ctx, other, actor, topic.Uuid, "published")
	if err == nil {
		t.Fatal("cross-org moderation allowed")
	}
	topic, err = s.Moderate(ctx, org, actor, topic.Uuid, "published")
	must(err)
	must(q.ForumSubscribe(ctx, db.ForumSubscribeParams{OrganizationID: org, TopicID: topic.ID, ContactID: subscriber}))
	reply, err := s.Create(ctx, org, author, &topic, model.PostInput{Body: "Here is the answer"})
	must(err)
	if _, err = s.Assist(ctx, org, actor, reply.Uuid); !errors.Is(err, aimodel.ErrCredits) {
		t.Fatal("AI credits not enforced", err)
	}
	if ai.calls != 1 {
		t.Fatal("AI not called through credit-controlled interface")
	}
	_, err = s.Moderate(ctx, org, actor, reply.Uuid, "published")
	must(err)
	if _, err = s.Moderate(ctx, org, actor, reply.Uuid, "published"); !errors.Is(err, supportops.ErrConflict) {
		t.Fatal("decided post moderated again", err)
	}
	notifications, err := q.ForumNotifications(ctx, db.ForumNotificationsParams{OrganizationID: org, ContactID: subscriber, Limit: 20})
	must(err)
	if len(notifications) != 1 {
		t.Fatalf("expected one durable notification, got %d", len(notifications))
	}
	others, err := q.ForumNotifications(ctx, db.ForumNotificationsParams{OrganizationID: other, ContactID: subscriber, Limit: 20})
	must(err)
	if len(others) != 0 {
		t.Fatal("cross-org notification leak")
	}
	must(s.Accept(ctx, org, actor, topic.Uuid, reply.Uuid))
	must(q.ForumVote(ctx, db.ForumVoteParams{OrganizationID: org, PostID: reply.ID, ContactID: subscriber}))
	must(q.ForumVote(ctx, db.ForumVoteParams{OrganizationID: org, PostID: reply.ID, ContactID: subscriber}))
	rows, err := q.ForumReplies(ctx, db.ForumRepliesParams{OrganizationID: org, TopicID: pgtype.Int8{Int64: topic.ID, Valid: true}, Limit: 20})
	must(err)
	if len(rows) != 1 || rows[0].Votes != 1 {
		t.Fatal("votes are not unique", rows)
	}
	otherTopic, err := s.Create(ctx, org, author, nil, model.PostInput{Category: "general", Title: "Other", Body: "Other question"})
	must(err)
	_, err = s.Moderate(ctx, org, actor, otherTopic.Uuid, "published")
	must(err)
	if err = s.Accept(ctx, org, actor, otherTopic.Uuid, reply.Uuid); err == nil {
		t.Fatal("answer from different topic accepted")
	}
	// Conversion is atomic and idempotent; scope checks still guard linked inbox data.
	var inboxID int64
	var inboxUUID uuid.UUID
	must(pool.QueryRow(ctx, "INSERT INTO channel_instances(organization_id,channel_type,name) VALUES($1,'widget','Forum inbox') RETURNING id,uuid", org).Scan(&inboxID, &inboxUUID))
	scoped := orgctx.WithScope(ctx, orgctx.Scope{ID: org, Permissions: []string{"tenant.conversations.read_all"}})
	conv, err := s.ToConversation(scoped, org, actor, topic.Uuid, inboxUUID)
	must(err)
	again, err := s.ToConversation(scoped, org, actor, topic.Uuid, inboxUUID)
	must(err)
	if conv.ID != again.ID {
		t.Fatal("conversion duplicated conversation")
	}
	var count int64
	must(pool.QueryRow(ctx, "SELECT count(*) FROM messages WHERE organization_id=$1 AND conversation_id=$2", org, conv.ID).Scan(&count))
	if count != 1 {
		t.Fatal("forum body was not copied exactly once")
	}
	var external uuid.UUID
	must(pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,channel_type,contact_id,subject) VALUES($1,1000000,'widget',$2,'Private support') RETURNING uuid", org, author).Scan(&external))
	input := model.ConversionInput{Category: "general", Title: "Redacted public topic", Body: "Safe excerpt", Conversation: external}
	if _, err = s.FromConversation(orgctx.WithScope(ctx, orgctx.Scope{ID: org}), org, actor, input); !errors.Is(err, supportops.ErrDenied) {
		t.Fatal("private conversation read permission bypassed", err)
	}
	copied, err := s.FromConversation(scoped, org, actor, input)
	must(err)
	if copied.Status != "pending" || copied.Body != "Safe excerpt" {
		t.Fatal("conversion auto-published or copied private content")
	}
	// The widget token must be verified and bound to the forum organization.
	mustExec := func(query string, args ...any) { t.Helper(); _, err := pool.Exec(ctx, query, args...); must(err) }
	mustExec("INSERT INTO contact_identities(organization_id,contact_id,channel_type,channel_instance_id,external_id) VALUES($1,$2,'widget',$3,'user:customer-1')", org, subscriber, inboxID)
	signer := widgettoken.NewSigner(strings.Repeat("x", 32))
	widget := widgetuc.New(widgetuc.Deps{Queries: q, Signer: signer, Instances: instanceSource{instance: channels.Instance{ID: inboxID, UUID: inboxUUID, OrganizationID: org, Status: "active", Settings: map[string]any{}}}})
	valid, _ := signer.Issue(widgettoken.Claims{Instance: inboxUUID, Visitor: "visitor-1", User: "customer-1"})
	anonymous, _ := signer.Issue(widgettoken.Claims{Instance: inboxUUID, Visitor: "visitor-1"})
	publicMux := http.NewServeMux()
	forum.RegisterRoutes(supportops.Router{Mux: publicMux}, &handler.Handler{Service: s, Widget: widget})
	for _, tc := range []struct {
		slug, token string
		status      int
	}{{suffix, valid, 200}, {suffix, anonymous, 403}, {suffix + "-other", valid, 403}, {suffix, "invalid", 403}} {
		req := httptest.NewRequest("GET", "/v1/public/help/"+tc.slug+"/forum/notifications", nil)
		req.Header.Set("X-Widget-ID", inboxUUID.String())
		req.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		publicMux.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("visitor isolation: got %d want %d: %s", w.Code, tc.status, w.Body.String())
		}
	}
	mux := http.NewServeMux()
	forum.RegisterRoutes(supportops.Router{Mux: mux}, &handler.Handler{Service: s})
	read := func(path string) (int, string) {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w.Code, w.Body.String()
	}
	status, body := read("/v1/public/help/" + suffix + "/forum/topics/" + topic.Slug)
	if status != 200 || strings.Contains(body, "contact_id") || strings.Contains(body, "flag_reason") || !strings.Contains(body, "Here is the answer") {
		t.Fatal("public projection invalid", status, body)
	}
	// The moderation endpoint decides pending posts only; hide the reply directly.
	if _, err = s.Moderate(ctx, org, actor, reply.Uuid, "rejected"); !errors.Is(err, supportops.ErrConflict) {
		t.Fatal("published post rejected through the moderation queue", err)
	}
	mustExec("UPDATE forum_posts SET status='rejected' WHERE organization_id=$1 AND id=$2", org, reply.ID)
	_, body = read("/v1/public/help/" + suffix + "/forum/topics/" + topic.Slug)
	if strings.Contains(body, "Here is the answer") {
		t.Fatal("hidden accepted reply leaked", body)
	}
	status, body = read("/v1/public/help/" + suffix + "-other/forum/topics/" + topic.Slug)
	if status != 404 {
		t.Fatal("cross-org slug leaked", status, body)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/v1/public/help/"+suffix+"/forum/topics", strings.NewReader(`{"body":"spoof"}`)))
	if w.Code != 403 {
		t.Fatal("unverified visitor accepted", w.Code)
	}
	var result map[string]any
	must(json.Unmarshal(w.Body.Bytes(), &result))
}

// Two moderators deciding the same pending post: the first decision stands,
// the other gets a conflict, and a rejected post cannot be published later
// from a stale queue (CF-180).
func TestForumModerationDecidesAPendingPostOnce(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	suffix := uuid.NewString()
	var org, actor, author int64
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('Forum race',$1) RETURNING id", suffix).Scan(&org))
	must(pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname,status) VALUES($1,'test','Forum','Agent','active') RETURNING id", suffix+"@example.test").Scan(&actor))
	must(pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Author') RETURNING id", org).Scan(&author))
	// Leaves nothing behind, the outbox rows of the emitted events included;
	// a failed delete fails the test instead of leaking rows silently.
	defer func() {
		for _, stmt := range []string{
			"DELETE FROM outbox_events WHERE organization_id=$1",
			"DELETE FROM forum_subscriptions WHERE organization_id=$1",
			"DELETE FROM forum_posts WHERE organization_id=$1",
			"DELETE FROM forum_categories WHERE organization_id=$1",
			"DELETE FROM contacts WHERE organization_id=$1",
			"DELETE FROM organizations WHERE id=$1",
		} {
			if _, err := pool.Exec(ctx, stmt, org); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
		if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id=$1", actor); err != nil {
			t.Errorf("cleanup user: %v", err)
		}
	}()
	_, err = q.ForumCreateCategory(ctx, db.ForumCreateCategoryParams{OrganizationID: org, Slug: "general", Name: "General"})
	must(err)
	s := &usecase.Service{Repo: billrepo.New(pool, q)}
	status := func(id uuid.UUID) string {
		t.Helper()
		var v string
		must(pool.QueryRow(ctx, "SELECT status FROM forum_posts WHERE organization_id=$1 AND uuid=$2", org, id).Scan(&v))
		return v
	}

	decisions := []string{"published", "rejected"}
	for round := range 10 {
		post, err := s.Create(ctx, org, author, nil, model.PostInput{Category: "general", Title: "Race", Body: "Two moderators, one post"})
		must(err)
		start := make(chan struct{})
		errs := make([]error, len(decisions))
		var wg sync.WaitGroup
		for i, decision := range decisions {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, errs[i] = s.Moderate(ctx, org, actor, post.Uuid, decision)
			}()
		}
		close(start)
		wg.Wait()
		winner := -1
		for i, err := range errs {
			switch {
			case err == nil && winner == -1:
				winner = i
			case err == nil:
				t.Fatalf("round %d: both decisions were applied", round)
			case !errors.Is(err, supportops.ErrConflict):
				t.Fatalf("round %d: loser got %v, want a conflict", round, err)
			}
		}
		if winner == -1 {
			t.Fatalf("round %d: no decision was applied: %v", round, errs)
		}
		if got := status(post.Uuid); got != decisions[winner] {
			t.Fatalf("round %d: stored %q, the applied decision was %q", round, got, decisions[winner])
		}
	}

	spam, err := s.Create(ctx, org, author, nil, model.PostInput{Category: "general", Title: "Spam", Body: "Rejected by the first moderator"})
	must(err)
	_, err = s.Moderate(ctx, org, actor, spam.Uuid, "rejected")
	must(err)
	if _, err = s.Moderate(ctx, org, actor, spam.Uuid, "published"); !errors.Is(err, supportops.ErrConflict) {
		t.Fatal("rejected post published from a stale queue", err)
	}
	if got := status(spam.Uuid); got != "rejected" {
		t.Fatal("rejected post changed status", got)
	}

	// Over HTTP the loser sees 409 CONFLICT and a missing post 404.
	mux := http.NewServeMux()
	mux.Handle("PATCH /posts/{uuid}", supportops.Router{}.Handler(func(r *http.Request) (any, error) {
		return s.Moderate(r.Context(), org, actor, supportops.ID(r), "published")
	}, false))
	for _, tc := range []struct {
		id   uuid.UUID
		code int
	}{{spam.Uuid, http.StatusConflict}, {uuid.New(), http.StatusNotFound}} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("PATCH", "/posts/"+tc.id.String(), nil))
		if w.Code != tc.code {
			t.Fatalf("moderation over HTTP: got %d want %d: %s", w.Code, tc.code, w.Body.String())
		}
	}
}

// A published topic or reply can be taken down and published again
// (CF-182): the post moves to `removed`, leaves every public read (topic
// page, list, sitemap, accepted answer, notification inbox), is recorded
// with who, when and why, and only a removed post can be republished.
func TestForumUnpublishAndRepublish(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	suffix := uuid.NewString()
	var org, other, actor, author, subscriber int64
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('Forum takedown',$1) RETURNING id", suffix).Scan(&org))
	must(pool.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('Other',$1) RETURNING id", suffix+"-other").Scan(&other))
	must(pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname,status) VALUES($1,'test','Forum','Agent','active') RETURNING id", suffix+"@example.test").Scan(&actor))
	must(pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Author') RETURNING id", org).Scan(&author))
	must(pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Subscriber') RETURNING id", org).Scan(&subscriber))
	defer func() {
		for _, stmt := range []string{
			"DELETE FROM outbox_events WHERE organization_id=$1",
			"DELETE FROM forum_notifications WHERE organization_id=$1",
			"DELETE FROM forum_subscriptions WHERE organization_id=$1",
			"UPDATE forum_posts SET accepted_reply_id=NULL WHERE organization_id=$1",
			"DELETE FROM forum_posts WHERE organization_id=$1",
			"DELETE FROM forum_categories WHERE organization_id=$1",
			"DELETE FROM contacts WHERE organization_id=$1",
		} {
			if _, err := pool.Exec(ctx, stmt, org); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
		if _, err := pool.Exec(ctx, "DELETE FROM organizations WHERE id=ANY($1::bigint[])", []int64{org, other}); err != nil {
			t.Errorf("cleanup organizations: %v", err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id=$1", actor); err != nil {
			t.Errorf("cleanup user: %v", err)
		}
	}()
	_, err = q.ForumCreateCategory(ctx, db.ForumCreateCategoryParams{OrganizationID: org, Slug: "general", Name: "General"})
	must(err)
	s := &usecase.Service{Repo: billrepo.New(pool, q)}
	topic, err := s.Create(ctx, org, author, nil, model.PostInput{Category: "general", Title: "Takedown", Body: "Topic body"})
	must(err)
	topic, err = s.Moderate(ctx, org, actor, topic.Uuid, "published")
	must(err)
	must(q.ForumSubscribe(ctx, db.ForumSubscribeParams{OrganizationID: org, TopicID: topic.ID, ContactID: subscriber}))
	reply, err := s.Create(ctx, org, author, &topic, model.PostInput{Body: "The accepted answer"})
	must(err)
	_, err = s.Moderate(ctx, org, actor, reply.Uuid, "published")
	must(err)
	must(s.Accept(ctx, org, actor, topic.Uuid, reply.Uuid))
	rejected, err := s.Create(ctx, org, author, nil, model.PostInput{Category: "general", Title: "Rejected", Body: "Never published"})
	must(err)
	_, err = s.Moderate(ctx, org, actor, rejected.Uuid, "rejected")
	must(err)
	pending, err := s.Create(ctx, org, author, nil, model.PostInput{Category: "general", Title: "Pending", Body: "Still waiting"})
	must(err)

	mux := http.NewServeMux()
	forum.RegisterRoutes(supportops.Router{Mux: mux}, &handler.Handler{Service: s})
	read := func(path string) (int, string) {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w.Code, w.Body.String()
	}
	base := "/v1/public/help/" + suffix + "/forum"
	public := func(visible bool, answered bool) {
		t.Helper()
		status, body := read(base + "/topics/" + topic.Slug)
		if (status == 200) != visible {
			t.Fatalf("topic page: got %d, visible=%v: %s", status, visible, body)
		}
		if visible && strings.Contains(body, "The accepted answer") != answered {
			t.Fatalf("topic page shows the reply=%v, want %v: %s", !answered, answered, body)
		}
		if visible && strings.Contains(body, `"accepted_answer":null`) == answered {
			t.Fatalf("topic page accepted_answer, want answered=%v: %s", answered, body)
		}
		_, list := read(base + "/topics")
		if strings.Contains(list, topic.Slug) != visible {
			t.Fatalf("topic list lists the topic=%v, want %v: %s", !visible, visible, list)
		}
		if visible && strings.Contains(list, `"accepted_reply_uuid":null`) == answered {
			t.Fatalf("topic list accepted_reply_uuid, want answered=%v: %s", answered, list)
		}
		_, sitemap := read(base + "/sitemap")
		if strings.Contains(sitemap, topic.Slug) != visible {
			t.Fatalf("sitemap lists the topic=%v, want %v: %s", !visible, visible, sitemap)
		}
		inbox, err := q.ForumNotifications(ctx, db.ForumNotificationsParams{OrganizationID: org, ContactID: subscriber, Limit: 20})
		must(err)
		if (len(inbox) == 1) != (visible && answered) {
			t.Fatalf("inbox has %d notifications, topic visible=%v reply visible=%v", len(inbox), visible, answered)
		}
	}
	public(true, true)

	// Cross-organization: the post does not exist there (404), nothing changes.
	if _, err = s.Unpublish(ctx, other, actor, reply.Uuid, ""); !errors.Is(err, billmodel.ErrNotFound) {
		t.Fatal("cross-org unpublish", err)
	}
	public(true, true)
	// Only a published post can be taken down.
	for _, id := range []uuid.UUID{rejected.Uuid, pending.Uuid} {
		if _, err = s.Unpublish(ctx, org, actor, id, ""); !errors.Is(err, supportops.ErrConflict) {
			t.Fatal("unpublished a post that is not published", err)
		}
	}
	if _, err = s.Unpublish(ctx, org, actor, reply.Uuid, strings.Repeat("x", 501)); !errors.Is(err, supportops.ErrInvalid) {
		t.Fatal("reason longer than 500 accepted", err)
	}
	// Only a removed post can be republished; a rejected post stays rejected.
	for _, id := range []uuid.UUID{rejected.Uuid, pending.Uuid, reply.Uuid} {
		if _, err = s.Republish(ctx, org, actor, id); !errors.Is(err, supportops.ErrConflict) {
			t.Fatal("republished a post that is not removed", err)
		}
	}

	// The accepted reply is taken down: the topic stays, without an answer.
	removed, err := s.Unpublish(ctx, org, actor, reply.Uuid, "  Wrong answer  ")
	must(err)
	if removed.Status != "removed" || !removed.RemovedAt.Valid || removed.RemovedByUserID.Int64 != actor || removed.RemovedReason != "Wrong answer" {
		t.Fatalf("takedown not recorded on the row: %+v", removed)
	}
	public(true, false)
	if _, err = s.Unpublish(ctx, org, actor, reply.Uuid, ""); !errors.Is(err, supportops.ErrConflict) {
		t.Fatal("removed post taken down twice", err)
	}
	var count int64
	must(pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE organization_id=$1 AND event_name='forum.moderated' AND payload->'data'->>'status'='removed' AND payload->'data'->>'reason'='Wrong answer' AND (payload->>'actor_user_id')::bigint=$2 AND payload->>'entity_uuid'=$3", org, actor, reply.Uuid.String()).Scan(&count))
	if count != 1 {
		t.Fatalf("takedown event: got %d rows, want 1", count)
	}
	// The topic itself is taken down: page, list, sitemap and inbox drop it.
	_, err = s.Unpublish(ctx, org, actor, topic.Uuid, "")
	must(err)
	public(false, false)
	rows, err := q.ForumRemoved(ctx, db.ForumRemovedParams{OrganizationID: org, Limit: 20})
	must(err)
	n, err := q.ForumRemovedCount(ctx, org)
	must(err)
	if n != 2 || len(rows) != 2 || rows[0].Uuid != topic.Uuid || rows[1].Uuid != reply.Uuid || rows[1].TopicTitle.String != "Takedown" || rows[1].RemovedByName.String != "Forum" || rows[0].RemovedReason != "" || rows[1].RemovedReason != "Wrong answer" {
		t.Fatalf("removed list: n=%d rows=%+v", n, rows)
	}
	for _, r := range rows {
		if !r.RemovedByUuid.Valid || !r.RemovedAt.Valid {
			t.Fatalf("removed row misses who/when: %+v", r)
		}
	}
	// A removed reply cannot come back while its topic is down (like the queue).
	if _, err = s.Republish(ctx, org, actor, reply.Uuid); !errors.Is(err, supportops.ErrConflict) {
		t.Fatal("reply republished under a removed topic", err)
	}
	if _, err = s.Republish(ctx, other, actor, topic.Uuid); !errors.Is(err, billmodel.ErrNotFound) {
		t.Fatal("cross-org republish", err)
	}
	back, err := s.Republish(ctx, org, actor, topic.Uuid)
	must(err)
	if back.Status != "published" || back.RemovedAt.Valid || back.RemovedByUserID.Valid || back.RemovedReason != "" {
		t.Fatalf("republish left takedown fields: %+v", back)
	}
	public(true, false)
	_, err = s.Republish(ctx, org, actor, reply.Uuid)
	must(err)
	// The accepted answer and the subscriber's notification come back as they were.
	public(true, true)
	must(pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE organization_id=$1 AND event_name='forum.moderated' AND payload->'data'->>'previous_status'='removed'", org).Scan(&count))
	if count != 2 {
		t.Fatalf("republish events: got %d rows, want 2", count)
	}

	// Over HTTP: the permission gate answers 403, another organization 404,
	// a post that is not published 409. The audit recorder is nil here; the
	// tenant router records every non-GET call as support_operations.updated.
	api := http.NewServeMux()
	api.Handle("POST /posts/{uuid}/unpublish", middleware.RequireTenantPermission("tenant.feedback.manage")(supportops.Router{}.Handler(func(r *http.Request) (any, error) {
		return s.Unpublish(r.Context(), orgctx.Must(r.Context()).ID, actor, supportops.ID(r), "")
	}, false)))
	api.Handle("POST /posts/{uuid}/republish", middleware.RequireTenantPermission("tenant.feedback.manage")(supportops.Router{}.Handler(func(r *http.Request) (any, error) {
		return s.Republish(r.Context(), orgctx.Must(r.Context()).ID, actor, supportops.ID(r))
	}, false)))
	manage := []string{"tenant.feedback.manage"}
	for _, tc := range []struct {
		name  string
		scope orgctx.Scope
		path  string
		code  int
	}{
		{"no permission", orgctx.Scope{ID: org, Permissions: []string{"tenant.feedback.read"}}, "/posts/" + topic.Uuid.String() + "/unpublish", http.StatusForbidden},
		{"other organization", orgctx.Scope{ID: other, Permissions: manage}, "/posts/" + topic.Uuid.String() + "/unpublish", http.StatusNotFound},
		{"unknown post", orgctx.Scope{ID: org, Permissions: manage}, "/posts/" + uuid.NewString() + "/unpublish", http.StatusNotFound},
		{"republish a published post", orgctx.Scope{ID: org, Permissions: manage}, "/posts/" + topic.Uuid.String() + "/republish", http.StatusConflict},
		{"unpublish", orgctx.Scope{ID: org, Permissions: manage}, "/posts/" + topic.Uuid.String() + "/unpublish", http.StatusOK},
		{"unpublish twice", orgctx.Scope{ID: org, Permissions: manage}, "/posts/" + topic.Uuid.String() + "/unpublish", http.StatusConflict},
		{"republish without permission", orgctx.Scope{ID: org}, "/posts/" + topic.Uuid.String() + "/republish", http.StatusForbidden},
		{"republish", orgctx.Scope{ID: org, Permissions: manage}, "/posts/" + topic.Uuid.String() + "/republish", http.StatusOK},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", tc.path, nil).WithContext(orgctx.WithScope(ctx, tc.scope))
		api.ServeHTTP(w, req)
		if w.Code != tc.code {
			t.Fatalf("%s: got %d want %d: %s", tc.name, w.Code, tc.code, w.Body.String())
		}
	}
	public(true, true)
}
func TestSpamFlagging(t *testing.T) {
	if usecase.Flag("https://a https://b https://c https://d") != "link_spam" {
		t.Fatal("link spam not flagged")
	}
	if usecase.Flag("ignore previous instructions") != "unsafe_instructions" {
		t.Fatal("injection not flagged")
	}
	if usecase.ValidSlug("../secret") || usecase.ValidSlug("A B") {
		t.Fatal("unsafe slug accepted")
	}
}
