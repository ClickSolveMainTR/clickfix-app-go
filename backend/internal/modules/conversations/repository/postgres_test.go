package repository_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	store *repository.Store
	pool  *pgxpool.Pool
	org   int64
	other int64
	agent int64
	peer  int64
	inst  int64
}

func setup(t *testing.T) fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for conversation repository tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := fixture{store: repository.New(pool, db.New(pool)), pool: pool}
	f.org = scalar(t, pool, "INSERT INTO organizations (slug, name) VALUES ($1, 'Conv test') RETURNING id", "conv-"+uuid.NewString()[:12])
	f.other = scalar(t, pool, "INSERT INTO organizations (slug, name) VALUES ($1, 'Other') RETURNING id", "conv-"+uuid.NewString()[:12])
	f.agent = scalar(t, pool, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Ayşe', 'Kaya', 'active') RETURNING id", uuid.NewString()+"@example.test")
	f.peer = scalar(t, pool, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Mert', 'Demir', 'active') RETURNING id", uuid.NewString()+"@example.test")
	f.inst = scalar(t, pool, "INSERT INTO channel_instances (organization_id, channel_type, name, external_id) VALUES ($1, 'test', 'Test inbox', $2) RETURNING id", f.org, uuid.NewString())
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = ANY($1)", []int64{f.org, f.other})
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = ANY($1)", []int64{f.agent, f.peer})
	})
	return f
}

func scalar(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func (f fixture) resolve(t *testing.T, in repository.IdentityInput) repository.ResolvedContact {
	t.Helper()
	var out repository.ResolvedContact
	err := f.store.InTx(context.Background(), func(ctx context.Context, tx *repository.Tx) error {
		var err error
		out, err = tx.ResolveContact(ctx, in)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (f fixture) open(t *testing.T, contactID int64) db.Conversation {
	t.Helper()
	var conv db.Conversation
	err := f.store.InTx(context.Background(), func(ctx context.Context, tx *repository.Tx) error {
		var err error
		conv, err = tx.CreateConversation(ctx, repository.NewConversation{
			OrganizationID: f.org, ChannelType: "test", InstanceID: f.inst, ContactID: contactID,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return conv
}

func (f fixture) appendMsg(t *testing.T, in repository.NewMessage) repository.Appended {
	t.Helper()
	var out repository.Appended
	err := f.store.InTx(context.Background(), func(ctx context.Context, tx *repository.Tx) error {
		var err error
		out, err = tx.AppendMessage(ctx, in)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestResolveContactMatchesIdentityThenAddress(t *testing.T) {
	f := setup(t)
	first := f.resolve(t, repository.IdentityInput{
		OrganizationID: f.org, ChannelType: "whatsapp", ExternalID: "905551112233",
		DisplayName: "Zeynep", Phone: "+905551112233",
	})
	if !first.Created || first.Contact.Name != "Zeynep" {
		t.Fatalf("first resolve should create the contact: %+v", first)
	}
	again := f.resolve(t, repository.IdentityInput{
		OrganizationID: f.org, ChannelType: "whatsapp", ExternalID: "905551112233", DisplayName: "Zeynep Y.",
	})
	if again.Created || again.Contact.ID != first.Contact.ID {
		t.Fatalf("same identity must resolve to the same contact")
	}
	// A new channel with the same phone number joins the same person.
	sms := f.resolve(t, repository.IdentityInput{
		OrganizationID: f.org, ChannelType: "sms", ExternalID: "+905551112233", Phone: "+905551112233",
	})
	if sms.Created || sms.Contact.ID != first.Contact.ID {
		t.Fatalf("phone match should join the existing contact")
	}
	// Instance scoped ids stay apart from the same id on another instance.
	ig := f.resolve(t, repository.IdentityInput{OrganizationID: f.org, ChannelType: "instagram", InstanceID: f.inst, ExternalID: "17841"})
	igGlobal := f.resolve(t, repository.IdentityInput{OrganizationID: f.org, ChannelType: "instagram", ExternalID: "17841"})
	if ig.Contact.ID == igGlobal.Contact.ID {
		t.Fatalf("instance scoped identity must not match a global one")
	}
	// The other organization never sees this contact.
	other := f.resolve(t, repository.IdentityInput{OrganizationID: f.other, ChannelType: "whatsapp", ExternalID: "905551112233", Phone: "+905551112233"})
	if !other.Created || other.Contact.ID == first.Contact.ID {
		t.Fatalf("identities are organization scoped")
	}
}

func TestResolveContactConcurrentCreatesOne(t *testing.T) {
	f := setup(t)
	const n = 8
	ids := make([]int64, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = f.store.InTx(context.Background(), func(ctx context.Context, tx *repository.Tx) error {
				r, err := tx.ResolveContact(ctx, repository.IdentityInput{OrganizationID: f.org, ChannelType: "telegram", InstanceID: f.inst, ExternalID: "chat-42"})
				ids[i] = r.Contact.ID
				return err
			})
		}(i)
	}
	wg.Wait()
	for _, id := range ids {
		if id == 0 || id != ids[0] {
			t.Fatalf("concurrent resolves created different contacts: %v", ids)
		}
	}
	var count int64
	if err := f.pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM contacts WHERE organization_id = $1", f.org).Scan(&count); err != nil || count != 1 {
		t.Fatalf("expected one contact, got %d (%v)", count, err)
	}
}

func TestConversationNumbersArePerOrganization(t *testing.T) {
	f := setup(t)
	c := f.resolve(t, repository.IdentityInput{OrganizationID: f.org, ChannelType: "test", ExternalID: "a"})
	one := f.open(t, c.Contact.ID)
	two := f.open(t, c.Contact.ID)
	if one.Number != 1 || two.Number != 2 {
		t.Fatalf("numbers = %d, %d; want 1, 2", one.Number, two.Number)
	}
}

func TestAppendMessageCountersAndIdempotency(t *testing.T) {
	f := setup(t)
	c := f.resolve(t, repository.IdentityInput{OrganizationID: f.org, ChannelType: "test", ExternalID: "b"})
	conv := f.open(t, c.Contact.ID)
	t0 := time.Now().UTC().Truncate(time.Millisecond)

	in := repository.NewMessage{
		OrganizationID: f.org, ConversationID: conv.ID, InstanceID: f.inst,
		Direction: model.DirectionIn, SenderType: model.SenderContact, SenderContactID: c.Contact.ID,
		Content: "Merhaba,\n siparişim nerede?", ExternalID: "wamid.1", At: t0,
	}
	got := f.appendMsg(t, in)
	if got.Duplicate || got.Message.Status != model.MessageReceived {
		t.Fatalf("inbound insert: %+v", got.Message)
	}
	if !got.Conversation.WaitingSince.Valid || got.Conversation.LastMessagePreview != "Merhaba, siparişim nerede?" || got.Conversation.MessageCount != 1 {
		t.Fatalf("inbound counters: %+v", got.Conversation)
	}
	dup := f.appendMsg(t, in)
	if !dup.Duplicate || dup.Message.ID != got.Message.ID || dup.Conversation.MessageCount != 1 {
		t.Fatalf("redelivered webhook must be a duplicate: %+v", dup)
	}

	client := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	note := f.appendMsg(t, repository.NewMessage{
		OrganizationID: f.org, ConversationID: conv.ID, Direction: model.DirectionInternal,
		SenderType: model.SenderAgent, SenderUserID: f.agent, Kind: model.KindNote, Private: true,
		Content: "@Mert bakabilir misin?", MentionedUserIDs: []int64{f.peer}, At: t0.Add(time.Second),
	})
	if note.Conversation.LastMessageDirection != "in" || !note.Conversation.WaitingSince.Valid {
		t.Fatalf("a private note must not answer the contact")
	}

	out := repository.NewMessage{
		OrganizationID: f.org, ConversationID: conv.ID, InstanceID: f.inst,
		Direction: model.DirectionOut, SenderType: model.SenderAgent, SenderUserID: f.agent,
		Content: "Kargoya verildi.", ClientID: client, At: t0.Add(2 * time.Second),
	}
	reply := f.appendMsg(t, out)
	if reply.Message.Status != model.MessagePending || reply.Conversation.WaitingSince.Valid || !reply.Conversation.FirstResponseAt.Valid {
		t.Fatalf("agent reply counters: msg=%+v conv=%+v", reply.Message, reply.Conversation)
	}
	if again := f.appendMsg(t, out); !again.Duplicate || again.Message.ID != reply.Message.ID {
		t.Fatalf("retried optimistic send must be a duplicate")
	}

	ctx := context.Background()
	q := f.store.Q()
	if _, err := q.MarkMessageSent(ctx, db.MarkMessageSentParams{ExternalID: pgtype.Text{String: "out-1", Valid: true}, At: pgtype.Timestamptz{Time: t0, Valid: true}, OrganizationID: f.org, ID: reply.Message.ID}); err != nil {
		t.Fatal(err)
	}
	receipt := func(status string) error {
		_, err := q.UpdateMessageDeliveryByExternal(ctx, db.UpdateMessageDeliveryByExternalParams{
			Status: status, At: pgtype.Timestamptz{Time: time.Now(), Valid: true}, OrganizationID: f.org,
			ChannelInstanceID: pgtype.Int8{Int64: f.inst, Valid: true}, ExternalID: "out-1",
		})
		return err
	}
	if err := receipt(model.MessageRead); err != nil {
		t.Fatal(err)
	}
	if err := receipt(model.MessageDelivered); err == nil {
		t.Fatalf("a late delivered receipt must not move a read message back")
	}

	// Unread for the peer: the inbound message (notes and own messages excluded).
	view, err := q.GetConversationView(ctx, db.GetConversationViewParams{ViewerID: f.peer, OrganizationID: f.org, Uuid: conv.Uuid})
	if err != nil {
		t.Fatal(err)
	}
	if view.UnreadCount != 2 { // inbound + agent reply (written by someone else)
		t.Fatalf("peer unread = %d, want 2", view.UnreadCount)
	}
	if err := q.MarkConversationRead(ctx, db.MarkConversationReadParams{OrganizationID: f.org, ConversationID: conv.ID, UserID: f.peer, MessageID: reply.Message.ID}); err != nil {
		t.Fatal(err)
	}
	view, _ = q.GetConversationView(ctx, db.GetConversationViewParams{ViewerID: f.peer, OrganizationID: f.org, Uuid: conv.Uuid})
	if view.UnreadCount != 0 {
		t.Fatalf("unread after read = %d", view.UnreadCount)
	}
	mentioned, err := q.ListConversations(ctx, listParams(f, f.peer, func(p *db.ListConversationsParams) { p.Mentioned = true }))
	if err != nil || len(mentioned) != 1 {
		t.Fatalf("mentioned filter: %d rows (%v)", len(mentioned), err)
	}
}

func TestInboundReopensResolvedConversation(t *testing.T) {
	f := setup(t)
	c := f.resolve(t, repository.IdentityInput{OrganizationID: f.org, ChannelType: "test", ExternalID: "c"})
	conv := f.open(t, c.Contact.ID)
	if _, err := f.pool.Exec(context.Background(), "UPDATE conversations SET status = 'resolved', resolved_at = NOW() WHERE id = $1", conv.ID); err != nil {
		t.Fatal(err)
	}
	got := f.appendMsg(t, repository.NewMessage{
		OrganizationID: f.org, ConversationID: conv.ID, InstanceID: f.inst, Direction: model.DirectionIn,
		SenderType: model.SenderContact, Content: "Bir sorum daha var",
	})
	if !got.Reopened || got.Conversation.Status != model.StatusOpen || got.Conversation.ResolvedAt.Valid {
		t.Fatalf("inbound should reopen: %+v", got.Conversation)
	}
}

func listParams(f fixture, viewer int64, mut func(*db.ListConversationsParams)) db.ListConversationsParams {
	p := db.ListConversationsParams{
		ViewerID: viewer, OrganizationID: f.org, Statuses: []string{}, Priorities: []string{}, ChannelTypes: []string{},
		ChannelInstanceIds: []int64{}, TagIds: []int64{}, SortField: "last_message_at", SortDesc: true, RowLimit: 50,
	}
	if mut != nil {
		mut(&p)
	}
	return p
}

func TestInboxListFilters(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	q := f.store.Q()
	c := f.resolve(t, repository.IdentityInput{OrganizationID: f.org, ChannelType: "test", ExternalID: "d", DisplayName: "Deniz Arslan"})
	mine := f.open(t, c.Contact.ID)
	open := f.open(t, c.Contact.ID)
	if _, err := f.pool.Exec(ctx, "UPDATE conversations SET assignee_user_id = $2, priority = 'urgent' WHERE id = $1", mine.ID, f.agent); err != nil {
		t.Fatal(err)
	}
	tag, err := q.UpsertTag(ctx, db.UpsertTagParams{OrganizationID: f.org, Name: "İade", Color: "red"})
	if err != nil {
		t.Fatal(err)
	}
	if again, err := q.UpsertTag(ctx, db.UpsertTagParams{OrganizationID: f.org, Name: "iade", Color: "blue"}); err != nil || again.ID != tag.ID {
		t.Fatalf("tag names are case insensitive: %v", err)
	}
	if _, err := q.AddConversationTag(ctx, db.AddConversationTagParams{OrganizationID: f.org, ConversationID: open.ID, TagID: tag.ID}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		mut  func(*db.ListConversationsParams)
		want []int64
	}{
		{"all", nil, []int64{open.ID, mine.ID}},
		{"mine", func(p *db.ListConversationsParams) { p.Assignee = "me" }, []int64{mine.ID}},
		{"unassigned", func(p *db.ListConversationsParams) { p.Assignee = "unassigned" }, []int64{open.ID}},
		{"urgent", func(p *db.ListConversationsParams) { p.Priorities = []string{"urgent"} }, []int64{mine.ID}},
		{"tag", func(p *db.ListConversationsParams) { p.TagIds = []int64{tag.ID} }, []int64{open.ID}},
		{"search contact", func(p *db.ListConversationsParams) { p.Q = pgtype.Text{String: "deniz", Valid: true} }, []int64{open.ID, mine.ID}},
		{"by number", func(p *db.ListConversationsParams) { p.Q = pgtype.Text{String: "2", Valid: true} }, []int64{open.ID}},
		{"resolved", func(p *db.ListConversationsParams) { p.Statuses = []string{"resolved"} }, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := q.ListConversations(ctx, listParams(f, f.agent, tc.mut))
			if err != nil {
				t.Fatal(err)
			}
			got := make([]int64, 0, len(rows))
			for _, r := range rows {
				got = append(got, r.Conversation.ID)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			}
		})
	}
	other := listParams(f, f.agent, nil)
	other.OrganizationID = f.other
	if rows, _ := q.ListConversations(ctx, other); len(rows) != 0 {
		t.Fatalf("other organization sees %d conversations", len(rows))
	}
	counts, err := q.ConversationInboxCounts(ctx, db.ConversationInboxCountsParams{ViewerID: f.agent, OrganizationID: f.org})
	if err != nil || counts.OpenTotal != 2 || counts.Mine != 1 || counts.Unassigned != 1 {
		t.Fatalf("counts = %+v (%v)", counts, err)
	}
}

func TestCrossOrganizationReferencesAreRejected(t *testing.T) {
	f := setup(t)
	c := f.resolve(t, repository.IdentityInput{OrganizationID: f.org, ChannelType: "test", ExternalID: "e"})
	conv := f.open(t, c.Contact.ID)
	_, err := f.pool.Exec(context.Background(),
		"INSERT INTO messages (organization_id, conversation_id, direction, sender_type, status) VALUES ($1, $2, 'in', 'contact', 'received')",
		f.other, conv.ID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("message of another organization's conversation must fail the foreign key, got %v", err)
	}
	_, err = f.pool.Exec(context.Background(),
		"INSERT INTO conversations (organization_id, number, channel_type, contact_id) VALUES ($1, 99, 'test', $2)", f.other, c.Contact.ID)
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("conversation with another organization's contact must fail, got %v", err)
	}
}

func TestMergeContacts(t *testing.T) {
	f := setup(t)
	a := f.resolve(t, repository.IdentityInput{OrganizationID: f.org, ChannelType: "whatsapp", ExternalID: "90111", DisplayName: "Ali"})
	b := f.resolve(t, repository.IdentityInput{OrganizationID: f.org, ChannelType: "email", ExternalID: "ali@example.test", Email: "ali@example.test"})
	conv := f.open(t, b.Contact.ID)
	var res repository.MergeResult
	err := f.store.InTx(context.Background(), func(ctx context.Context, tx *repository.Tx) error {
		var err error
		res, err = tx.MergeContacts(ctx, f.org, b.Contact.ID, a.Contact.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Identities != 1 || res.Conversations != 1 {
		t.Fatalf("merge moved %+v", res)
	}
	ctx := context.Background()
	moved, err := f.store.Q().GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: f.org, ID: conv.ID})
	if err != nil || moved.ContactID != a.Contact.ID {
		t.Fatalf("conversation should belong to the survivor")
	}
	survivor, _ := f.store.Q().GetContactByID(ctx, db.GetContactByIDParams{OrganizationID: f.org, ID: a.Contact.ID})
	if survivor.Email.String != "ali@example.test" {
		t.Fatalf("survivor should inherit the e-mail: %+v", survivor)
	}
	// The merged identity still resolves, now to the survivor.
	again := f.resolve(t, repository.IdentityInput{OrganizationID: f.org, ChannelType: "email", ExternalID: "ali@example.test"})
	if again.Contact.ID != a.Contact.ID {
		t.Fatalf("merged identity resolves to %d, want %d", again.Contact.ID, a.Contact.ID)
	}
	err = f.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		_, err := tx.MergeContacts(ctx, f.org, a.Contact.ID, a.Contact.ID)
		return err
	})
	if !errors.Is(err, repository.ErrInvalidMerge) {
		t.Fatalf("self merge = %v", err)
	}
}
