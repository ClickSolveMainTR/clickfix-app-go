package usecase

import (
	"context"
	"os"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/email"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOutboundEmailUsesConversationBrandTemplateAndSignature(t *testing.T) {
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
	var org int64
	if err := pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Brand mail') RETURNING id", "brandmail-"+uuid.NewString()).Scan(&org); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id=$1", org) }()
	domain := "brandmail-" + uuid.NewString() + ".test"
	support := "support@" + domain
	var id uuid.UUID
	if err := pool.QueryRow(ctx, "INSERT INTO brands(organization_id,name,support_email,sender_name,reply_to,signature) VALUES($1,'Acme',$2,'Acme Support','reply@acme.test','Acme team') RETURNING uuid", org, support).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO brand_email_templates(organization_id,brand_id,code,subject,body) VALUES($1,$2,'reply','{{brand_name}}: {{subject}}','Hello! {{content}}')", org, id); err != nil {
		t.Fatal(err)
	}
	svc := New(repository.New(pool, db.New(pool)), nil, nil)
	original := channels.Credentials{email.FieldFromAddress: "other@other.test"}
	settings := map[string]any{"other": "kept"}
	inst := channels.Instance{Type: channels.TypeEmail, Credentials: original, Settings: settings}
	out := channels.OutboundMessage{Text: "Your answer", Subject: "Question"}
	if err := svc.applyEmailBrand(ctx, db.Conversation{OrganizationID: org, BrandID: pgtype.UUID{Bytes: id, Valid: true}}, &inst, &out); err == nil {
		t.Fatal("brand bypassed organization sender-domain ownership")
	}
	if _, err := pool.Exec(ctx, "INSERT INTO channel_email_domains(organization_id,domain,resend_domain_id) VALUES($1,$2,$3)", org, domain, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if err := svc.applyEmailBrand(ctx, db.Conversation{OrganizationID: org, BrandID: pgtype.UUID{Bytes: id, Valid: true}}, &inst, &out); err != nil {
		t.Fatal(err)
	}
	if inst.Credentials[email.FieldFromAddress] != support || inst.Settings["brand_reply_to"] != "reply@acme.test" || out.Text != "Hello! Your answer\n\nAcme team" || out.Subject != "Acme: Question" {
		t.Fatalf("brand mail=%+v %+v", inst, out)
	}
	if original[email.FieldFromAddress] != "other@other.test" || settings["brand_reply_to"] != nil {
		t.Fatal("mutated shared channel state")
	}
	if err := svc.applyEmailBrand(ctx, db.Conversation{OrganizationID: org + 1, BrandID: pgtype.UUID{Bytes: id, Valid: true}}, &inst, &out); err == nil {
		t.Fatal("cross-org brand email")
	}
}
