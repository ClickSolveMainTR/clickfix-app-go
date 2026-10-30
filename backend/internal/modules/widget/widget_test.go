package widget_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/inbox"
	channelsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	convrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	helpusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/helpcenter/usecase"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	kbrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/repository"
	kbusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/usecase"
	widgetmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	identitySecret = "identity-secret-0123456789abcdef"
	siteOrigin     = "https://shop.example.com"
)

type published struct {
	mu     sync.Mutex
	events map[string][]any
}

func (p *published) Publish(_ context.Context, channel string, data any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events[channel] = append(p.events[channel], data)
	return nil
}

func (p *published) on(channel string) []any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.events[channel]
}

type syncQueue struct{ ib *inbox.Inbox }

func (q syncQueue) EnqueueSend(ctx context.Context, orgID, messageID int64, _ time.Duration) error {
	return q.ib.Deliver(ctx, orgID, messageID, 0, 2)
}

type env struct {
	t     *testing.T
	pool  *pgxpool.Pool
	ib    *inbox.Inbox
	pub   *published
	mux   *http.ServeMux
	org   int64
	agent int64
	inst  channelsusecase.InstanceView
	h     *handler.Handler
	svc   *usecase.Service
	q     *db.Queries
}

func setup(t *testing.T) *env {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for widget tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	var cfg config.Config
	cfg.App.Env = "test"
	cfg.Encryption.Key = "0123456789abcdef0123456789abcdef"
	pub := &published{events: map[string][]any{}}
	ib, err := inbox.New(inbox.Deps{Config: cfg, DB: pool, Queries: q, Realtime: pub})
	if err != nil {
		t.Fatal(err)
	}
	ib.Conversations.WithQueue(syncQueue{ib: ib})
	e := &env{t: t, pool: pool, ib: ib, pub: pub}
	if err := pool.QueryRow(ctx, "INSERT INTO organizations (slug, name) VALUES ($1, 'Widget') RETURNING id", "wg-"+uuid.NewString()[:12]).Scan(&e.org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Elif', 'Şahin', 'active') RETURNING id", uuid.NewString()+"@example.test").Scan(&e.agent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", e.org)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", e.agent)
	})
	e.inst, err = ib.Channels.Create(ctx, channelsusecase.CreateInput{
		OrganizationID: e.org, ActorID: e.agent, Type: channels.TypeWidget, Name: "Site",
		Credentials: channels.Credentials{widget.IdentitySecretField: identitySecret},
		Settings:    map[string]any{"allowed_domains": []any{"shop.example.com"}, "color": "#112233"},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := usecase.New(usecase.Deps{
		Queries: q, Store: convrepository.New(pool, q), Instances: ib.Channels, Pipeline: ib.Conversations,
		Signer: widget.NewSigner(cfg.Encryption.Key), Publisher: pub, AppOrigin: "https://app.clickfix.test",
	})
	e.mux = http.NewServeMux()
	e.svc, e.q = svc, q
	e.h = handler.New(svc, nil, nil)
	widgetmodule.RegisterRoutes(e.mux, e.h)
	return e
}

type reply struct {
	Code int
	Data map[string]any
	Err  map[string]any
	Hdr  http.Header
}

func (e *env) call(method, path, token, origin string, body any) reply {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, "/v1/widget/"+e.inst.UUID.String()+path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	var env struct {
		Data  map[string]any `json:"data"`
		Error map[string]any `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return reply{Code: rec.Code, Data: env.Data, Err: env.Error, Hdr: rec.Header()}
}

func str(m map[string]any, keys ...string) string {
	var cur any = m
	for _, k := range keys {
		mm, _ := cur.(map[string]any)
		cur = mm[k]
	}
	s, _ := cur.(string)
	return s
}

func TestVisitorFlow(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	// CORS preflight answers any origin; the allowlist is enforced per widget.
	if r := e.call(http.MethodOptions, "/bootstrap", "", "https://evil.test", nil); r.Code != http.StatusNoContent || r.Hdr.Get("Access-Control-Allow-Origin") != "https://evil.test" {
		t.Fatalf("preflight = %d %v", r.Code, r.Hdr)
	}
	if r := e.call(http.MethodPost, "/bootstrap", "", "https://evil.test", nil); r.Code != http.StatusForbidden {
		t.Fatalf("foreign origin bootstrap = %d", r.Code)
	}
	if r := e.call(http.MethodPost, "/bootstrap", "", "https://app.clickfix.test", nil); r.Code != http.StatusOK {
		t.Fatalf("app origin (preview) bootstrap = %d", r.Code)
	}
	boot := e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil)
	if boot.Code != http.StatusOK || str(boot.Data, "widget", "settings", "color") != "#112233" {
		t.Fatalf("bootstrap = %d %v", boot.Code, boot.Data)
	}
	if _, leaked := boot.Data["widget"].(map[string]any)["settings"].(map[string]any)["allowed_domains"]; leaked {
		t.Fatal("bootstrap leaks the allowlist")
	}
	token := str(boot.Data, "visitor", "token")
	visitorID := str(boot.Data, "visitor", "id")
	if token == "" || visitorID == "" {
		t.Fatalf("no visitor session: %v", boot.Data)
	}
	// Resuming keeps the visitor.
	if again := e.call(http.MethodPost, "/bootstrap", token, siteOrigin, nil); str(again.Data, "visitor", "id") != visitorID {
		t.Fatal("bootstrap with a token started a new visitor")
	}
	if r := e.call(http.MethodGet, "/conversations", "", siteOrigin, nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("no token = %d", r.Code)
	}

	// The visitor starts a conversation (pre-chat name and e-mail).
	clientID := uuid.New()
	created := e.call(http.MethodPost, "/conversations", token, siteOrigin, map[string]any{
		"text": "Merhaba, siparişim nerede?", "client_id": clientID, "name": "Ada", "email": "ada@example.com",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %v", created.Code, created.Err)
	}
	convID := str(created.Data, "conversation_uuid")
	// A retried send (same client id) is stored once.
	if r := e.call(http.MethodPost, "/conversations/"+convID+"/messages", token, siteOrigin, map[string]any{"text": "Merhaba, siparişim nerede?", "client_id": clientID}); r.Code != http.StatusOK {
		t.Fatalf("retry = %d %v", r.Code, r.Err)
	}
	list := e.call(http.MethodGet, "/conversations", token, siteOrigin, nil)
	items, _ := list.Data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("conversations = %d %v %v", list.Code, list.Data, list.Err)
	}
	var name, email, channelType string
	if err := e.pool.QueryRow(ctx, `SELECT ct.name, COALESCE(ct.email, ''), c.channel_type FROM conversations c JOIN contacts ct ON ct.id = c.contact_id WHERE c.uuid = $1`, convID).Scan(&name, &email, &channelType); err != nil {
		t.Fatal(err)
	}
	if name != "Ada" || email != "ada@example.com" || channelType != channels.TypeWidget {
		t.Fatalf("contact %q %q %q", name, email, channelType)
	}

	// An agent replies; the widget adapter delivers it on the visitor channel.
	posted, err := e.ib.Conversations.Post(ctx, convusecase.PostInput{
		OrganizationID: e.org, ConversationUUID: uuid.MustParse(convID), AuthorID: e.agent, AuthorName: "Elif",
		Content: "Kargoya verildi.", ClientID: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	channel := widget.VisitorChannel(e.inst.UUID, "visitor:"+visitorID)
	evs := e.pub.on(channel)
	if len(evs) != 1 || evs[0].(widget.Event).Text != "Kargoya verildi." {
		t.Fatalf("visitor channel events = %v", evs)
	}
	msgs := e.call(http.MethodGet, "/conversations/"+convID+"/messages", token, siteOrigin, nil)
	mitems, _ := msgs.Data["items"].([]any)
	if msgs.Code != http.StatusOK || len(mitems) != 2 {
		t.Fatalf("messages = %d %v", msgs.Code, msgs.Data)
	}
	last := mitems[1].(map[string]any)
	if last["direction"] != "out" || last["sender_name"] != "Elif" || last["content"] != "Kargoya verildi." {
		t.Fatalf("reply = %v", last)
	}
	if unread := msgs.Data["conversation"].(map[string]any)["unread_count"].(float64); unread != 1 {
		t.Fatalf("unread = %v", unread)
	}
	// Private notes never reach the visitor.
	if _, err := e.ib.Conversations.Post(ctx, convusecase.PostInput{OrganizationID: e.org, ConversationUUID: uuid.MustParse(convID), AuthorID: e.agent, Content: "iç not", Private: true}); err != nil {
		t.Fatal(err)
	}
	if r := e.call(http.MethodGet, "/conversations/"+convID+"/messages?after="+str(mitems[0].(map[string]any), "uuid"), token, siteOrigin, nil); len(r.Data["items"].([]any)) != 1 {
		t.Fatalf("after cursor = %v", r.Data)
	}
	// Read receipt.
	if r := e.call(http.MethodPost, "/conversations/"+convID+"/read", token, siteOrigin, nil); r.Code != http.StatusOK || r.Data["updated"].(float64) != 1 {
		t.Fatalf("read = %d %v", r.Code, r.Data)
	}
	var status string
	_ = e.pool.QueryRow(ctx, "SELECT status FROM messages WHERE uuid = $1", posted.Message.UUID).Scan(&status)
	if status != "read" {
		t.Fatalf("reply status = %s", status)
	}
	if r := e.call(http.MethodPost, "/conversations/"+convID+"/typing", token, siteOrigin, map[string]any{"typing": true}); r.Code != http.StatusOK {
		t.Fatalf("typing = %d", r.Code)
	}

	// Another visitor sees nothing of it.
	other := str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "visitor", "token")
	if r := e.call(http.MethodGet, "/conversations/"+convID+"/messages", other, siteOrigin, nil); r.Code != http.StatusNotFound {
		t.Fatalf("foreign visitor messages = %d", r.Code)
	}

	// Rating: only finished conversations, once.
	if r := e.call(http.MethodPost, "/conversations/"+convID+"/rating", token, siteOrigin, map[string]any{"score": 5}); r.Code != http.StatusConflict {
		t.Fatalf("rating open conversation = %d", r.Code)
	}
	if _, err := e.pool.Exec(ctx, "UPDATE conversations SET status = 'resolved', resolved_at = NOW() WHERE uuid = $1", convID); err != nil {
		t.Fatal(err)
	}
	if r := e.call(http.MethodPost, "/conversations/"+convID+"/rating", token, siteOrigin, map[string]any{"score": 4, "comment": "Hızlı"}); r.Code != http.StatusOK || r.Data["existing"] != false {
		t.Fatalf("rating = %d %v", r.Code, r.Data)
	}
	if r := e.call(http.MethodPost, "/conversations/"+convID+"/rating", token, siteOrigin, map[string]any{"score": 1}); r.Data["existing"] != true || r.Data["score"].(float64) != 4 {
		t.Fatalf("second rating = %v", r.Data)
	}

	// Identify: a wrong hash is refused; the right one moves the anonymous
	// history to the user's contact.
	if r := e.call(http.MethodPost, "/identify", token, siteOrigin, map[string]any{"user_id": "u-1", "user_hash": "00"}); r.Code != http.StatusUnauthorized {
		t.Fatalf("bad hash = %d", r.Code)
	}
	ident := e.call(http.MethodPost, "/identify", token, siteOrigin, map[string]any{
		"user_id": "u-1", "user_hash": widget.UserHash(identitySecret, "u-1"), "name": "Ada Lovelace",
		"email": "ada@acme.test", "company": map[string]any{"id": "acme", "name": "Acme A.Ş.", "domain": "acme.test"},
	})
	if ident.Code != http.StatusOK || ident.Data["verified"] != true {
		t.Fatalf("identify = %d %v", ident.Code, ident.Err)
	}
	userToken := str(ident.Data, "token")
	if r := e.call(http.MethodGet, "/conversations", userToken, siteOrigin, nil); len(r.Data["items"].([]any)) != 1 {
		t.Fatalf("identified user lost the anonymous conversation: %v", r.Data)
	}
	var company string
	if err := e.pool.QueryRow(ctx, `SELECT co.name FROM conversations c JOIN contacts ct ON ct.id = c.contact_id JOIN companies co ON co.id = ct.company_id WHERE c.uuid = $1`, convID).Scan(&company); err != nil || company != "Acme A.Ş." {
		t.Fatalf("company = %q %v", company, err)
	}
	// Resolved long enough ago? No: a new message reopens the conversation.
	if r := e.call(http.MethodPost, "/conversations/"+convID+"/messages", userToken, siteOrigin, map[string]any{"text": "Bir sorum daha var"}); r.Code != http.StatusOK || str(r.Data, "conversation_uuid") != convID {
		t.Fatalf("reply after resolve = %d %v", r.Code, r.Data)
	}

	// A file arrives as multipart (stored as a media message).
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("client_id", uuid.NewString())
	fw, _ := mw.CreateFormFile("file", "fatura.pdf")
	_, _ = fw.Write([]byte("%PDF-1.4 test"))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/widget/"+e.inst.UUID.String()+"/conversations/"+convID+"/messages", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", siteOrigin)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("multipart = %d %s", rec.Code, rec.Body.String())
	}
	var kind string
	if err := e.pool.QueryRow(ctx, "SELECT kind FROM messages m JOIN conversations c ON c.id = m.conversation_id WHERE c.uuid = $1 ORDER BY m.id DESC LIMIT 1", convID).Scan(&kind); err != nil || kind != "media" {
		t.Fatalf("file message kind = %q %v", kind, err)
	}
	// Closed conversations take no more messages.
	if _, err := e.pool.Exec(ctx, "UPDATE conversations SET status = 'closed', closed_at = NOW() WHERE uuid = $1", convID); err != nil {
		t.Fatal(err)
	}
	if r := e.call(http.MethodPost, "/conversations/"+convID+"/messages", userToken, siteOrigin, map[string]any{"text": "hala orada mısınız"}); r.Code != http.StatusConflict {
		t.Fatalf("closed conversation = %d", r.Code)
	}
}

func TestRequireIdentity(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := e.ib.Channels.Update(ctx, e.org, e.inst.UUID, channelsusecase.UpdateInput{Settings: map[string]any{"require_identity": true}}); err != nil {
		t.Fatal(err)
	}
	token := str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "visitor", "token")
	if r := e.call(http.MethodPost, "/conversations", token, siteOrigin, map[string]any{"text": "selam"}); r.Code != http.StatusForbidden {
		t.Fatalf("anonymous on identity-only widget = %d", r.Code)
	}
	user := e.call(http.MethodPost, "/identify", token, siteOrigin, map[string]any{"user_id": "42", "user_hash": widget.UserHash(identitySecret, "42")})
	if r := e.call(http.MethodPost, "/conversations", str(user.Data, "token"), siteOrigin, map[string]any{"text": "selam"}); r.Code != http.StatusCreated {
		t.Fatalf("identified = %d %v", r.Code, r.Err)
	}
	// A paused widget disappears.
	paused := "paused"
	if _, err := e.ib.Channels.Update(ctx, e.org, e.inst.UUID, channelsusecase.UpdateInput{Status: &paused}); err != nil {
		t.Fatal(err)
	}
	if r := e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil); r.Code != http.StatusNotFound {
		t.Fatalf("paused widget bootstrap = %d", r.Code)
	}
}

// CF-55 / CF-74: the widget help tab lists only the published public
// articles of the widget's own organization, with help center links.
func TestHelpSearch(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	origin := "https://shop.example.com"
	if r := e.call(http.MethodGet, "/help/search?q=iade", "", origin, nil); r.Code != http.StatusOK || r.Data["available"] != false {
		t.Fatalf("help disabled: %d %+v", r.Code, r.Data)
	}
	kb := kbusecase.New(kbrepo.New(e.pool))
	e.h.WithHelp(helpusecase.New(db.New(e.pool), nil, nil, "secret"), "https://app.clickfix.test/")
	if r := e.call(http.MethodGet, "/help/search", "", origin, nil); r.Code != http.StatusOK || r.Data["available"] != false {
		t.Fatalf("no public article yet: %d %+v", r.Code, r.Data)
	}
	var other int64
	if err := e.pool.QueryRow(ctx, "INSERT INTO organizations (slug, name) VALUES ($1, 'Other') RETURNING id", "wg-"+uuid.NewString()[:12]).Scan(&other); err != nil {
		t.Fatal(err)
	}
	add := func(org int64, slug, title, visibility string, publish bool) {
		a, err := kb.SaveArticle(ctx, org, e.agent, uuid.New(), kbmodel.ArticleInput{Slug: slug, Locale: "tr", Title: title, Body: "İade adımları", Visibility: visibility}, true)
		if err != nil {
			t.Fatal(err)
		}
		if publish {
			if err = kb.Publish(ctx, org, a.Uuid, a.Version, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	add(e.org, "iade", "İade rehberi", "public", true)
	add(e.org, "taslak-iade", "Taslak iade", "public", false)
	add(e.org, "ic-iade", "İç iade", "internal", true)
	add(other, "baska-iade", "Başka iade", "public", true)

	r := e.call(http.MethodGet, "/help/search?q=iade", "", origin, nil)
	items, _ := r.Data["items"].([]any)
	if r.Code != http.StatusOK || r.Data["available"] != true || len(items) != 1 {
		t.Fatalf("search: %d %+v", r.Code, r.Data)
	}
	first, _ := items[0].(map[string]any)
	if first["title"] != "İade rehberi" || !strings.HasPrefix(str(first, "url"), "https://app.clickfix.test/help/") || !strings.HasSuffix(str(first, "url"), "/article/iade") {
		t.Fatalf("item %+v", first)
	}
	if r := e.call(http.MethodGet, "/help/search?q=iade", "", "https://evil.example", nil); r.Code != http.StatusForbidden {
		t.Fatalf("foreign origin: %d", r.Code)
	}
}
