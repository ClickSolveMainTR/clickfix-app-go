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
	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	aiuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
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
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM forum_subscriptions WHERE organization_id=$1", org)
		_, _ = pool.Exec(ctx, "DELETE FROM forum_posts WHERE organization_id=$1", org)
		_, _ = pool.Exec(ctx, "DELETE FROM forum_categories WHERE organization_id=$1", org)
		_, _ = pool.Exec(ctx, "DELETE FROM contacts WHERE organization_id=$1", org)
		_, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id=$1", org)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id=$1", actor)
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
