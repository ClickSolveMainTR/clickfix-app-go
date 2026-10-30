package usecase

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/testchannel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mailCapture is an e-mail adapter that records what it sends.
type mailCapture struct {
	*testchannel.Adapter
	mu   sync.Mutex
	sent []channels.OutboundMessage
	fail error
}

func (m *mailCapture) Type() string { return channels.TypeEmail }

func (m *mailCapture) Send(_ context.Context, _ channels.Instance, msg channels.OutboundMessage) (channels.SendResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return channels.SendResult{}, m.fail
	}
	m.sent = append(m.sent, msg)
	return channels.SendResult{ExternalID: "<cf." + msg.MessageUUID.String() + "@mail.test>"}, nil
}

type linkInstances struct {
	email channels.Instance
	mail  *mailCapture
}

func (f linkInstances) Instance(_ context.Context, _, id int64) (channels.Instance, channels.Adapter, error) {
	if id == f.email.ID {
		return f.email, f.mail, nil
	}
	return channels.Instance{}, nil, errors.New("unknown instance")
}

type linkFixture struct {
	svc   *Service
	pool  *pgxpool.Pool
	org   int64
	agent Actor
	test  channels.Instance
	email channels.Instance
	mail  *mailCapture
}

func newLinkFixture(t *testing.T) linkFixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var f linkFixture
	f.pool = pool
	q := func(sql string, args ...any) int64 {
		var id int64
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id
	}
	f.org = q("INSERT INTO organizations (slug, name) VALUES ($1, 'Links') RETURNING id", "links-"+uuid.NewString()[:12])
	var agentUUID uuid.UUID
	agentID := q("INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Ayşe', 'Kaya', 'active') RETURNING id", uuid.NewString()+"@example.test")
	if err := pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id = $1", agentID).Scan(&agentUUID); err != nil {
		t.Fatal(err)
	}
	f.agent = Actor{ID: agentID, UUID: agentUUID, Name: "Ayşe Kaya"}
	testID := q("INSERT INTO channel_instances (organization_id, channel_type, name, external_id) VALUES ($1, 'test', 'Test', $2) RETURNING id", f.org, uuid.NewString())
	emailID := q("INSERT INTO channel_instances (organization_id, channel_type, name, external_id) VALUES ($1, 'email', 'Destek', $2) RETURNING id", f.org, uuid.NewString())
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", agentID)
	})
	f.test = channels.Instance{ID: testID, UUID: uuid.New(), OrganizationID: f.org, Type: channels.TypeTest, Name: "Test", Status: "active"}
	f.email = channels.Instance{ID: emailID, UUID: uuid.New(), OrganizationID: f.org, Type: channels.TypeEmail, Name: "Destek", Status: "active"}
	f.mail = &mailCapture{Adapter: testchannel.New()}
	f.svc = New(repository.New(pool, db.New(pool)), linkInstances{email: f.email, mail: f.mail}, nil)
	return f
}

// inbound stores a customer message and returns its conversation.
func (f linkFixture) inbound(t *testing.T, from, text, thread string) db.Conversation {
	t.Helper()
	ctx := context.Background()
	if _, err := f.svc.Receive(ctx, f.test, channels.WebhookResult{Messages: []channels.InboundMessage{{
		ExternalID: uuid.NewString(), From: channels.Sender{ExternalID: from, Name: from}, Kind: "text", Text: text, ThreadID: thread,
	}}}); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := f.pool.QueryRow(ctx, `SELECT m.conversation_id FROM messages m WHERE m.organization_id = $1 AND m.content = $2 ORDER BY m.id DESC LIMIT 1`, f.org, text).Scan(&id); err != nil {
		t.Fatal(err)
	}
	c, err := f.svc.store.Q().GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: f.org, ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f linkFixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMergeMovesMessagesAndClosesSource(t *testing.T) {
	f := newLinkFixture(t)
	ctx := context.Background()
	target := f.inbound(t, "ali", "ilk talep", "")
	// A second conversation of another contact (merge across contacts is allowed).
	source := f.inbound(t, "veli", "aynı konu", "")
	f.inbound(t, "veli", "ek bilgi", "")
	if _, err := f.svc.TagConversation(ctx, f.org, source.Uuid, f.agent, uuid.Nil, "fatura", true); err != nil {
		t.Fatal(err)
	}

	res, err := f.svc.Merge(ctx, f.org, f.agent, target.Uuid, source.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	if res.Messages < 3 { // two texts + the tag event
		t.Fatalf("moved %d messages", res.Messages)
	}
	if res.Source.Status != model.StatusClosed || !res.Source.MergedIntoID.Valid || res.Source.MergedIntoID.Int64 != target.ID {
		t.Fatalf("source not closed and pointed: %+v", res.Source)
	}
	if got := f.count(t, "SELECT COUNT(*) FROM messages WHERE organization_id = $1 AND conversation_id = $2 AND kind <> 'event'", f.org, target.ID); got != 3 {
		t.Fatalf("target holds %d customer messages, want 3", got)
	}
	if res.Target.MessageCount != int32(f.count(t, "SELECT COUNT(*) FROM messages WHERE organization_id = $1 AND conversation_id = $2", f.org, target.ID)) {
		t.Fatalf("target counter %d out of sync", res.Target.MessageCount)
	}
	if f.count(t, "SELECT COUNT(*) FROM conversation_tags WHERE organization_id = $1 AND conversation_id = $2", f.org, target.ID) != 1 {
		t.Fatal("tag not carried over")
	}
	if f.count(t, `SELECT COUNT(*) FROM messages WHERE organization_id = $1 AND conversation_id = $2 AND content_attributes->>'event' = 'merged_into'`, f.org, source.ID) != 1 {
		t.Fatal("source has no merged_into event")
	}
	links, err := f.svc.Links(ctx, f.org, target.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(links.Items) != 1 || links.Items[0].Relation != RelationMerged || links.Items[0].UUID != source.Uuid {
		t.Fatalf("links of target: %+v", links.Items)
	}
	srcLinks, err := f.svc.Links(ctx, f.org, source.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	if srcLinks.MergedInto == nil || srcLinks.MergedInto.UUID != target.Uuid {
		t.Fatalf("source merged_into: %+v", srcLinks.MergedInto)
	}

	// Merging again, or into itself, is refused.
	if _, err := f.svc.Merge(ctx, f.org, f.agent, target.Uuid, source.Uuid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("second merge: %v", err)
	}
	if _, err := f.svc.Merge(ctx, f.org, f.agent, target.Uuid, target.Uuid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("self merge: %v", err)
	}
	// Another organization's conversation is not found.
	if _, err := f.svc.Merge(ctx, f.org+1_000_000, f.agent, target.Uuid, source.Uuid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign org: %v", err)
	}
}

func TestMergedThreadFollowsTarget(t *testing.T) {
	f := newLinkFixture(t)
	ctx := context.Background()
	target := f.inbound(t, "ali", "a1", "<thread-a@x>")
	source := f.inbound(t, "ali", "b1", "<thread-b@x>")
	if target.ID == source.ID {
		t.Fatal("expected two conversations")
	}
	if _, err := f.svc.Merge(ctx, f.org, f.agent, target.Uuid, source.Uuid); err != nil {
		t.Fatal(err)
	}
	again := f.inbound(t, "ali", "b2 reply", "<thread-b@x>")
	if again.ID != target.ID {
		t.Fatalf("reply on merged thread went to %d, want target %d", again.ID, target.ID)
	}
}

func TestSplitCreatesChildWithFollowingMessages(t *testing.T) {
	f := newLinkFixture(t)
	ctx := context.Background()
	conv := f.inbound(t, "ali", "s1", "")
	f.inbound(t, "ali", "s2 yeni konu", "")
	f.inbound(t, "ali", "s3", "")
	var msgUUID uuid.UUID
	if err := f.pool.QueryRow(ctx, "SELECT uuid FROM messages WHERE organization_id = $1 AND content = 's2 yeni konu'", f.org).Scan(&msgUUID); err != nil {
		t.Fatal(err)
	}
	child, err := f.svc.Split(ctx, SplitInput{OrganizationID: f.org, Actor: f.agent, ConversationUUID: conv.Uuid, MessageUUID: msgUUID, Following: true, Subject: "Yeni konu"})
	if err != nil {
		t.Fatal(err)
	}
	if child.Subject != "Yeni konu" || child.ContactID != conv.ContactID {
		t.Fatalf("child: %+v", child)
	}
	if got := f.count(t, "SELECT COUNT(*) FROM messages WHERE organization_id = $1 AND conversation_id = $2 AND kind <> 'event'", f.org, child.ID); got != 2 {
		t.Fatalf("child holds %d messages, want 2", got)
	}
	if got := f.count(t, "SELECT COUNT(*) FROM messages WHERE organization_id = $1 AND conversation_id = $2 AND kind <> 'event'", f.org, conv.ID); got != 1 {
		t.Fatalf("parent keeps %d messages, want 1", got)
	}
	links, err := f.svc.Links(ctx, f.org, child.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(links.Items) != 1 || links.Items[0].Relation != RelationParent || links.Items[0].UUID != conv.Uuid {
		t.Fatalf("child links: %+v", links.Items)
	}
	// A system event cannot be split off.
	var ev uuid.UUID
	if err := f.pool.QueryRow(ctx, "SELECT uuid FROM messages WHERE organization_id = $1 AND conversation_id = $2 AND kind = 'event' LIMIT 1", f.org, conv.ID).Scan(&ev); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Split(ctx, SplitInput{OrganizationID: f.org, Actor: f.agent, ConversationUUID: conv.Uuid, MessageUUID: ev}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("split event: %v", err)
	}
}

func TestLinksParentChildRelatedAndCycles(t *testing.T) {
	f := newLinkFixture(t)
	ctx := context.Background()
	a := f.inbound(t, "a", "a", "")
	b := f.inbound(t, "b", "b", "")
	c := f.inbound(t, "c", "c", "")
	link := func(conv, other db.Conversation, rel string) error {
		return f.svc.Link(ctx, LinkInput{OrganizationID: f.org, Actor: f.agent, ConversationUUID: conv.Uuid, OtherUUID: other.Uuid, Relation: rel})
	}
	if err := link(a, b, RelationChild); err != nil { // b is a's child
		t.Fatal(err)
	}
	if err := link(b, c, RelationChild); err != nil { // c is b's child
		t.Fatal(err)
	}
	if err := link(c, a, RelationChild); !errors.Is(err, ErrLinkConflict) { // a under c: cycle
		t.Fatalf("cycle: %v", err)
	}
	if err := link(c, a, RelationParent); !errors.Is(err, ErrLinkConflict) { // c already has a parent
		t.Fatalf("second parent: %v", err)
	}
	if err := link(c, a, RelationRelated); err != nil {
		t.Fatal(err)
	}
	if err := link(a, a, RelationRelated); !errors.Is(err, ErrInvalid) {
		t.Fatalf("self link: %v", err)
	}
	got, err := f.svc.Links(ctx, f.org, a.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	rels := map[uuid.UUID]string{}
	for _, it := range got.Items {
		rels[it.UUID] = it.Relation
	}
	if rels[b.Uuid] != RelationChild || rels[c.Uuid] != RelationRelated {
		t.Fatalf("links of a: %+v", rels)
	}
	if err := f.svc.Unlink(ctx, f.org, f.agent, b.Uuid, a.Uuid); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Unlink(ctx, f.org, f.agent, b.Uuid, a.Uuid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second unlink: %v", err)
	}
}

func TestSideConversationSendAndReply(t *testing.T) {
	f := newLinkFixture(t)
	ctx := context.Background()
	conv := f.inbound(t, "ali", "kargo nerede", "")
	sc, err := f.svc.StartSide(ctx, StartSideInput{
		OrganizationID: f.org, Actor: f.agent, ConversationUUID: conv.Uuid,
		To: []string{"Depo <depo@supplier.test>, lojistik@supplier.test"}, Subject: "Sipariş 123 kargo durumu", Body: "Merhaba, durum nedir?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.mail.sent) != 1 {
		t.Fatalf("sent %d mails", len(f.mail.sent))
	}
	out := f.mail.sent[0]
	if out.To.Email != "depo@supplier.test" || !strings.HasPrefix(out.ThreadID, sideRootPrefix) || out.Attributes[attrNewThread] != true {
		t.Fatalf("outbound: %+v", out)
	}
	if cc, _ := out.Attributes["cc"].([]string); len(cc) != 1 || cc[0] != "lojistik@supplier.test" {
		t.Fatalf("cc: %+v", out.Attributes["cc"])
	}
	if len(sc.Messages) != 1 || sc.Messages[0].Status != model.MessageSent || sc.Inbox == nil || sc.Inbox.Name != "Destek" {
		t.Fatalf("side: %+v", sc)
	}

	// The supplier answers on the thread through the e-mail inbox.
	reply := channels.WebhookResult{Messages: []channels.InboundMessage{{
		ExternalID: "<answer-1@supplier.test>", From: channels.Sender{ExternalID: "depo@supplier.test", Email: "depo@supplier.test", Name: "Depo"},
		Kind: "email", Text: "Yarın teslim", ThreadID: out.ThreadID, At: time.Now(),
	}}}
	if _, err := f.svc.Receive(ctx, f.email, reply); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Receive(ctx, f.email, reply); err != nil { // provider retry
		t.Fatal(err)
	}
	if n := f.count(t, "SELECT COUNT(*) FROM conversations WHERE organization_id = $1", f.org); n != 1 {
		t.Fatalf("a side reply opened a customer conversation (%d conversations)", n)
	}
	got, err := f.svc.GetSide(ctx, f.org, sc.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 2 || got.Messages[1].Direction != model.DirectionIn || got.Messages[1].Content != "Yarın teslim" {
		t.Fatalf("side messages: %+v", got.Messages)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM messages WHERE organization_id = $1 AND conversation_id = $2 AND content_attributes->>'event' LIKE 'side_conversation_%'`, f.org, conv.ID); n != 2 {
		t.Fatalf("parent timeline has %d side events, want 2", n)
	}

	// A reply threads on the supplier's Message-ID.
	if _, err := f.svc.ReplySide(ctx, f.org, f.agent, sc.UUID, "Teşekkürler"); err != nil {
		t.Fatal(err)
	}
	if last := f.mail.sent[len(f.mail.sent)-1]; last.ReplyToExternalID != "<answer-1@supplier.test>" || len(last.Attributes) != 1 {
		t.Fatalf("reply: %+v", last)
	}

	// A failed send is stored on the message.
	f.mail.fail = errors.New("smtp down")
	failed, err := f.svc.ReplySide(ctx, f.org, f.agent, sc.UUID, "tekrar")
	if err != nil {
		t.Fatal(err)
	}
	if m := failed.Messages[len(failed.Messages)-1]; m.Status != model.MessageFailed || m.Error == "" {
		t.Fatalf("failed message: %+v", m)
	}

	if _, err := f.svc.StartSide(ctx, StartSideInput{OrganizationID: f.org, Actor: f.agent, ConversationUUID: conv.Uuid, To: []string{"bad"}, Subject: "x", Body: "y"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad address: %v", err)
	}
	closed, err := f.svc.SetSideStatus(ctx, f.org, sc.UUID, "closed")
	if err != nil || closed.Status != "closed" {
		t.Fatalf("close: %v %+v", err, closed.Status)
	}
	list, err := f.svc.ListSide(ctx, f.org, conv.Uuid)
	if err != nil || len(list.Items) != 1 || len(list.Inboxes) != 1 {
		t.Fatalf("list: %v %+v", err, list)
	}
}
