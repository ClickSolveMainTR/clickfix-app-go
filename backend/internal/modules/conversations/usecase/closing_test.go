package usecase

import (
	"context"
	"os"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRenderClosing(t *testing.T) {
	got := renderClosing("Saygılarımla,\n{{ agent.name }} - {{brand.name}}{{unknown}}", map[string]string{"agent.name": "Ayşe Kaya", "brand.name": "Acme"})
	if got != "Saygılarımla,\nAyşe Kaya - Acme" {
		t.Fatalf("render=%q", got)
	}
	if closingBody(db.ClosingText{BodyTr: "tr", BodyEn: " "}, "en") != "tr" || closingBody(db.ClosingText{BodyTr: "tr", BodyEn: "en"}, "en") != "en" {
		t.Fatal("english falls back to turkish only when empty")
	}
	org := db.ClosingText{}
	brand := db.ClosingText{BrandID: pgtype.UUID{Bytes: uuid.New(), Valid: true}}
	conv := db.Conversation{BrandID: brand.BrandID}
	if !closingFits(org, db.Conversation{}) || !closingFits(brand, conv) || closingFits(brand, db.Conversation{}) {
		t.Fatal("brand texts fit only their brand's conversations")
	}
}

func TestClosingTextChoiceDefaultsAndSignature(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := func(sql string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id
	}
	org := q("INSERT INTO organizations(slug,name) VALUES($1,'Easy Org') RETURNING id", "closing-"+uuid.NewString())
	defer func() { _, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id=$1", org) }()
	agent := q("INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Ayşe', 'Kaya', 'active') RETURNING id", uuid.NewString()+"@example.test")
	defer func() { _, _ = pool.Exec(ctx, "DELETE FROM users WHERE id=$1", agent) }()
	var brand uuid.UUID
	if err := pool.QueryRow(ctx, "INSERT INTO brands(organization_id,name,signature) VALUES($1,'Acme','Acme team') RETURNING uuid", org).Scan(&brand); err != nil {
		t.Fatal(err)
	}
	contact := q("INSERT INTO contacts(organization_id,name,locale) VALUES($1,'John Smith','en-GB') RETURNING id", org)
	convID := q("INSERT INTO conversations(organization_id,number,contact_id,channel_type,source,brand_id) VALUES($1,7,$2,'api','inbound',$3) RETURNING id", org, contact, brand)
	queries := db.New(pool)
	conv, err := queries.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: org, ID: convID})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(repository.New(pool, db.New(pool)), nil, nil)

	// No closing texts: auto adds nothing.
	if got, attrs, err := svc.applyClosing(ctx, conv, agent, Closing{Mode: ClosingAuto}, "Hi", nil); err != nil || got != "Hi" || attrs != nil {
		t.Fatalf("empty list=%q %v %v", got, attrs, err)
	}
	orgDefault, err := queries.InsertClosingText(ctx, db.InsertClosingTextParams{OrganizationID: org, Name: "Genel", BodyTr: "İyi günler, {{agent.first_name}}", BodyEn: "Best, {{agent.name}}", IsDefault: true})
	if err != nil {
		t.Fatal(err)
	}
	brandText, err := queries.InsertClosingText(ctx, db.InsertClosingTextParams{OrganizationID: org, BrandID: pgtype.UUID{Bytes: brand, Valid: true}, Name: "Acme", BodyTr: "{{brand.name}} ekibi"})
	if err != nil {
		t.Fatal(err)
	}
	// The org default is used, in the contact's language (en).
	got, attrs, err := svc.applyClosing(ctx, conv, agent, Closing{Mode: ClosingAuto}, "Hi", nil)
	if err != nil || got != "Hi\n\nBest, Ayşe Kaya" || attrs[AttrClosingText] == nil {
		t.Fatalf("default=%q %v %v", got, attrs, err)
	}
	// A brand default wins over the organization default.
	if _, err := pool.Exec(ctx, "UPDATE closing_texts SET is_default=true WHERE id=$1", brandText.ID); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := svc.applyClosing(ctx, conv, agent, Closing{Mode: ClosingAuto}, "Hi", nil); got != "Hi\n\nAcme ekibi" {
		t.Fatalf("brand default=%q", got)
	}
	// An explicit pick is remembered for the agent.
	if got, _, err := svc.applyClosing(ctx, conv, agent, Closing{Mode: ClosingPick, UUID: orgDefault.Uuid}, "", nil); err != nil || got != "Best, Ayşe Kaya" {
		t.Fatalf("pick=%q %v", got, err)
	}
	opts, err := svc.ClosingOptionsFor(ctx, org, Actor{ID: agent}, conv.Uuid)
	if err != nil || len(opts.Items) != 2 || opts.Selected == nil || *opts.Selected != orgDefault.Uuid || opts.Language != "en" {
		t.Fatalf("options=%+v %v", opts, err)
	}
	// "none" is remembered too.
	if got, attrs, err := svc.applyClosing(ctx, conv, agent, Closing{Mode: ClosingNone}, "Hi", nil); err != nil || got != "Hi" || attrs != nil {
		t.Fatalf("none=%q %v", got, err)
	}
	if got, _, _ := svc.applyClosing(ctx, conv, agent, Closing{Mode: ClosingAuto}, "Hi", nil); got != "Hi" {
		t.Fatalf("remembered none=%q", got)
	}
	// System senders never get one; another brand's text is refused.
	if got, _, _ := svc.applyClosing(ctx, conv, 0, Closing{}, "Hi", nil); got != "Hi" {
		t.Fatalf("off=%q", got)
	}
	other := conv
	other.BrandID = pgtype.UUID{}
	if _, _, err := svc.applyClosing(ctx, other, agent, Closing{Mode: ClosingPick, UUID: brandText.Uuid}, "Hi", nil); err == nil {
		t.Fatal("brand text used on a conversation of another brand")
	}

	// The email signature is not added after a closing text.
	inst := channels.Instance{Type: channels.TypeEmail, Credentials: channels.Credentials{}}
	out := channels.OutboundMessage{Text: "Hi\n\nAcme ekibi", Attributes: map[string]any{AttrClosingText: map[string]any{"uuid": brandText.Uuid.String()}}}
	if err := svc.applyEmailBrand(ctx, conv, &inst, &out); err != nil || out.Text != "Hi\n\nAcme ekibi" {
		t.Fatalf("signature twice=%q %v", out.Text, err)
	}
	out = channels.OutboundMessage{Text: "Hi"}
	if err := svc.applyEmailBrand(ctx, conv, &inst, &out); err != nil || out.Text != "Hi\n\nAcme team" {
		t.Fatalf("signature=%q %v", out.Text, err)
	}

	// AI replies get the default only when the organization turned it on.
	bot := db.Message{SenderType: model.SenderBot}
	aiAttrs := map[string]any{"ai_trigger_message_uuid": uuid.NewString()}
	if text, added := svc.aiClosing(ctx, conv, bot, aiAttrs, "Answer"); added || text != "Answer" {
		t.Fatalf("ai off=%q", text)
	}
	if _, err := queries.UpsertClosingTextSettings(ctx, db.UpsertClosingTextSettingsParams{OrganizationID: org, ApplyToAi: true}); err != nil {
		t.Fatal(err)
	}
	if text, added := svc.aiClosing(ctx, conv, bot, aiAttrs, "Answer"); !added || text != "Answer\n\nAcme ekibi" {
		t.Fatalf("ai on=%q", text)
	}
	if _, added := svc.aiClosing(ctx, conv, db.Message{SenderType: model.SenderBot}, map[string]any{}, "Auto reply"); added {
		t.Fatal("non-AI automatic message got a closing text")
	}
}
