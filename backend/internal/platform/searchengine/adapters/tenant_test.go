package adapters

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func txt(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func TestConversationDocument(t *testing.T) {
	id := uuid.New()
	doc := ConversationDocument(db.SearchConversationDocsRow{
		ID: 1, Uuid: id, OrganizationID: 9, Number: 1042, Subject: "Fatura gelmedi", Status: "open",
		Priority: "high", ChannelType: "email", LastMessagePreview: "Merhaba,\n faturam yok",
		ContactName: "Ayşe Yılmaz", ContactEmail: txt("ayse@example.test"), CompanyName: "Acme", InboxName: "Destek", TagNames: "vip fatura",
	})
	if doc.OrganizationID != 9 || doc.ID != id.String() || doc.ConversationID != id.String() || doc.Spec != SpecConversations {
		t.Fatalf("doc = %+v", doc)
	}
	if doc.Title != "#1042 · Fatura gelmedi" || doc.Subtitle != "Ayşe Yılmaz · Merhaba, faturam yok" {
		t.Fatalf("title/subtitle = %q / %q", doc.Title, doc.Subtitle)
	}
	if doc.Href != "/inbox?status=all&c="+id.String() {
		t.Fatalf("href = %q", doc.Href)
	}
	for _, k := range []string{"#1042", "1042", "ayse@example.test", "Acme", "Destek", "vip fatura"} {
		if !slices.Contains(doc.Keywords, k) {
			t.Fatalf("keywords %v miss %q", doc.Keywords, k)
		}
	}
	noSubject := ConversationDocument(db.SearchConversationDocsRow{Uuid: id, Number: 3, ContactName: "Can"})
	if noSubject.Title != "#3 · Can" {
		t.Fatalf("fallback title = %q", noSubject.Title)
	}
}

func TestMessageDocumentTruncatesAndMarksNotes(t *testing.T) {
	conv := uuid.New()
	long := strings.Repeat("ğ", MaxIndexedContent+50)
	doc := MessageDocument(db.SearchMessageDocsRow{
		Uuid: uuid.New(), OrganizationID: 5, Private: true, Content: long, Direction: "internal",
		ConversationUuid: conv, ConversationNumber: 12, ContactName: "Can", SenderUserName: "Elif Şahin",
	})
	if utf8.RuneCountInString(doc.Body) != MaxIndexedContent {
		t.Fatalf("body runes = %d", utf8.RuneCountInString(doc.Body))
	}
	if utf8.RuneCountInString(doc.Subtitle) > subtitleLength {
		t.Fatalf("subtitle too long: %d", utf8.RuneCountInString(doc.Subtitle))
	}
	if !doc.Private || doc.Icon != "note" || doc.OrganizationID != 5 || doc.ConversationID != conv.String() {
		t.Fatalf("note doc = %+v", doc)
	}
	if doc.Href != "/inbox?status=all&c="+conv.String() {
		t.Fatalf("message hit must link to its conversation: %q", doc.Href)
	}
	if doc.Title != "#12 · Can" || !slices.Contains(doc.Keywords, "Elif Şahin") {
		t.Fatalf("title/keywords = %q %v", doc.Title, doc.Keywords)
	}
}

func TestContactAndCompanyDocuments(t *testing.T) {
	cid := uuid.New()
	c := ContactDocument(db.SearchContactDocsRow{Uuid: cid, OrganizationID: 3, Email: txt("x@example.test"), Phone: txt("+90555"), CompanyName: "Acme"})
	if c.Title != "x@example.test · +90555" || c.Subtitle != "x@example.test · +90555 · Acme" || c.OrganizationID != 3 {
		t.Fatalf("contact = %+v", c)
	}
	if c.Href != "/contacts/"+cid.String() {
		t.Fatalf("contact href = %q", c.Href)
	}
	co := CompanyDocument(db.SearchCompanyDocsRow{Uuid: cid, OrganizationID: 3, Name: " Acme ", Domain: txt("acme.test")})
	if co.Title != "Acme" || co.Subtitle != "acme.test" || co.Href != "/inbox?status=all&company="+cid.String() || co.OrganizationID != 3 {
		t.Fatalf("company = %+v", co)
	}
}

func TestTenantSpecs(t *testing.T) {
	specs := []searchengine.Spec{
		NewConversations(nil).Spec(), NewMessages(nil).Spec(), NewContacts(nil).Spec(), NewCompanies(nil).Spec(),
	}
	for _, s := range specs {
		if !s.Tenant || !strings.HasPrefix(s.Permission, "tenant.") {
			t.Fatalf("spec %s must be a tenant spec with a tenant permission: %+v", s.ID, s)
		}
	}
	if specs[0].Permission != rbac.PermTenantConversationsRead || specs[2].Permission != rbac.PermTenantContactsRead {
		t.Fatalf("permissions = %s / %s", specs[0].Permission, specs[2].Permission)
	}
	if NewUsers(nil).Spec().Tenant || NewRoles(nil).Spec().Tenant {
		t.Fatal("platform specs must not be tenant specs")
	}
}

// fakeQ pages messages by id like the SQL does.
type fakeQ struct {
	TenantQuerier
	msgs  []db.SearchMessageDocsRow
	calls []int64
}

func (f *fakeQ) SearchMessageDocs(_ context.Context, arg db.SearchMessageDocsParams) ([]db.SearchMessageDocsRow, error) {
	f.calls = append(f.calls, arg.AfterID)
	out := []db.SearchMessageDocsRow{}
	for _, m := range f.msgs {
		if arg.Uuid.Valid && m.Uuid != uuid.UUID(arg.Uuid.Bytes) {
			continue
		}
		if m.ID > arg.AfterID && int32(len(out)) < arg.RowLimit {
			out = append(out, m)
		}
	}
	return out, nil
}

func TestMessagesListBatchesKeyset(t *testing.T) {
	q := &fakeQ{}
	for i := int64(1); i <= 5; i++ {
		q.msgs = append(q.msgs, db.SearchMessageDocsRow{ID: i * 10, Uuid: uuid.New(), OrganizationID: i % 2, Content: "x"})
	}
	a := NewMessages(q)
	var batches []int
	err := a.ListBatches(context.Background(), 2, func(d []searchengine.Document) error {
		batches = append(batches, len(d))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(batches, []int{2, 2, 1}) || !slices.Equal(q.calls, []int64{0, 20, 40}) {
		t.Fatalf("batches = %v calls = %v", batches, q.calls)
	}
	all, err := a.ListAll(context.Background())
	if err != nil || len(all) != 5 {
		t.Fatalf("ListAll = %d, %v", len(all), err)
	}
	doc, err := a.Document(context.Background(), q.msgs[2].Uuid.String())
	if err != nil || doc.ID != q.msgs[2].Uuid.String() || doc.OrganizationID != q.msgs[2].OrganizationID {
		t.Fatalf("Document = %+v, %v", doc, err)
	}
	if _, err := a.Document(context.Background(), uuid.NewString()); !errors.Is(err, searchengine.ErrNotFound) {
		t.Fatalf("missing message err = %v", err)
	}
	if _, err := a.Document(context.Background(), "not-a-uuid"); !errors.Is(err, searchengine.ErrNotFound) {
		t.Fatalf("invalid id err = %v", err)
	}
}
