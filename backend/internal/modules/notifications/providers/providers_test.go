package providers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/mail"
	"github.com/jackc/pgx/v5/pgtype"
)

type captureSender struct{ msgs []mail.Message }

func (c *captureSender) Send(_ context.Context, m mail.Message) error {
	c.msgs = append(c.msgs, m)
	return nil
}

func TestEmailProviderSendsBrandedMultipart(t *testing.T) {
	sender := &captureSender{}
	p := EmailProvider{Mail: sender, AppURL: "https://app.example.com/", DeliverReserved: true}
	_, err := p.Deliver(context.Background(), db.Notification{
		Title:        "You're invited",
		Body:         "Join Acme.",
		Recipient:    pgtype.Text{String: "a@example.com", Valid: true},
		ActionUrl:    pgtype.Text{String: "/invite/abc", Valid: true},
		TemplateCode: pgtype.Text{String: "organizations.invitation", Valid: true},
		UserID:       pgtype.Int8{Int64: 1, Valid: true},
		Payload:      []byte(`{"locale":"en"}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(sender.msgs) != 1 {
		t.Fatalf("sent %d", len(sender.msgs))
	}
	m := sender.msgs[0]
	if m.Subject != "You're invited" || m.HTML == "" || len(m.Inline) != 1 {
		t.Fatalf("message %+v", m)
	}
	for _, want := range []string{
		`href="https://app.example.com/invite/abc"`, ">Accept invitation</a>",
		"Team invitation", `href="https://app.example.com/profile"`,
	} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("html missing %q", want)
		}
	}
	if !strings.HasPrefix(m.Body, "Join Acme.") || !strings.Contains(m.Body, "https://app.example.com/invite/abc") {
		t.Fatalf("plain text %q", m.Body)
	}
}

func TestEmailProviderDefaultsToTurkish(t *testing.T) {
	sender := &captureSender{}
	_, err := EmailProvider{Mail: sender, DeliverReserved: true}.Deliver(context.Background(), db.Notification{
		Title: "Ödeme alındı", Body: "Ödemeniz alındı.",
		Recipient:   pgtype.Text{String: "a@example.com", Valid: true},
		SourceEvent: pgtype.Text{String: "billing.payment_received", Valid: true},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	html := sender.msgs[0].HTML
	if !strings.Contains(html, "Faturalama") || !strings.Contains(html, "ClickFix tarafından gönderildi") {
		t.Fatal("expected Turkish eyebrow and footer")
	}
	if strings.Contains(html, "Bildirim tercihleri") {
		t.Fatal("no preferences link without a user")
	}
}

// TestEmailProviderUsesEnrichedActionLabel: an enriched notification (CF-157)
// names its button; the detail rows become the layout's details table.
func TestEmailProviderUsesEnrichedActionLabel(t *testing.T) {
	sender := &captureSender{}
	_, err := EmailProvider{Mail: sender, AppURL: "https://app.example.com", DeliverReserved: true}.Deliver(context.Background(), db.Notification{
		Title:       "Konuşma size atandı · #42 · Deniz Kaya",
		Body:        "Ali Veli, Deniz Kaya ile olan #42 numaralı konuşmayı size atadı.\n\nMüşteri: Deniz Kaya\nKanal: WhatsApp\nKonuşma: #42",
		Recipient:   pgtype.Text{String: "a@example.com", Valid: true},
		ActionUrl:   pgtype.Text{String: "/inbox?c=1", Valid: true},
		SourceEvent: pgtype.Text{String: "conversation.assigned", Valid: true},
		UserID:      pgtype.Int8{Int64: 1, Valid: true},
		Payload:     []byte(`{"locale":"tr","action_label":"Konuşmayı aç"}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := sender.msgs[0]
	for _, want := range []string{`href="https://app.example.com/inbox?c=1"`, ">Konuşmayı aç</a>", ">Müşteri</td>", ">WhatsApp</td>", "Konuşma"} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("html missing %q", want)
		}
	}
	if !strings.Contains(m.Body, "Konuşmayı aç: https://app.example.com/inbox?c=1") {
		t.Fatalf("plain text lacks the action: %q", m.Body)
	}
}

func TestEmailProviderSkipsReservedDomain(t *testing.T) {
	sender := &captureSender{}
	_, err := EmailProvider{Mail: sender}.Deliver(context.Background(), db.Notification{
		Title: "Hi", Body: "x", Recipient: pgtype.Text{String: "demo@example.com", Valid: true},
	}, nil)
	var skip *SkipError
	if !errors.As(err, &skip) || skip.Reason != SkipReservedDomain {
		t.Fatalf("want SkipError(reserved_domain), got %v", err)
	}
	if len(sender.msgs) != 0 {
		t.Fatalf("sent %d, want 0", len(sender.msgs))
	}
}
