package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/engine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	pool  *pgxpool.Pool
	q     *db.Queries
	svc   *usecase.Service
	org   int64
	topic uuid.UUID
	order uuid.UUID
	plan  uuid.UUID
}

func setup(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for request form tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	f := &fixture{pool: pool, q: q, svc: usecase.New(pool, q)}
	f.org = scalarID(t, pool, "INSERT INTO organizations (slug, name) VALUES ($1, 'Form test') RETURNING id", "form-"+uuid.NewString()[:12])
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org) })
	def := func(entity, key, label, typ, options string) uuid.UUID {
		var u uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO custom_field_definitions (organization_id, entity_type, key, label, field_type, options)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb) RETURNING uuid`, f.org, entity, key, label, typ, options).Scan(&u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	f.topic = def("conversation", "topic", "Konu", "select", `[{"value":"billing","label":"Fatura"},{"value":"tech","label":"Teknik"}]`)
	f.order = def("conversation", "order_no", "Sipariş no", "text", `[]`)
	f.plan = def("contact", "plan", "Paket", "text", `[]`)
	return f
}

func scalarID(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func (f *fixture) input(name string) model.FormInput {
	return model.FormInput{Name: name, Channels: []string{"widget"}, Fields: []engine.Field{
		{Key: engine.KeyName, Source: engine.SourceSystem, Required: true},
		{Key: engine.KeyEmail, Source: engine.SourceSystem, Required: true},
		{Key: engine.KeyMessage, Source: engine.SourceSystem, Required: true},
		{Source: engine.SourceConversation, CustomFieldUUID: &f.topic, Required: true},
		{Source: engine.SourceConversation, CustomFieldUUID: &f.order, RequiredToResolve: true,
			Condition: &customfields.Condition{Field: "conversation.topic", Operator: customfields.CondEquals, Value: "billing"}},
		{Source: engine.SourceContact, CustomFieldUUID: &f.plan},
		{Key: engine.KeyPriority, Source: engine.SourceSystem},
	}}
}

func TestFormLifecycle(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.svc.Create(ctx, f.org, model.FormInput{Name: "Bozuk", Fields: []engine.Field{{Key: "nope", Source: engine.SourceSystem}}}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("bad form err=%v", err)
	}
	form, err := f.svc.Create(ctx, f.org, f.input("Destek talebi"))
	if err != nil {
		t.Fatal(err)
	}
	if form.Fields[3].Key != "conversation.topic" {
		t.Fatalf("custom key not normalized: %+v", form.Fields[3])
	}
	pub, err := f.svc.ForChannel(ctx, f.org, "widget")
	if err != nil || pub == nil || len(pub.Fields) != 7 || pub.Fields[3].Type != customfields.TypeSelect || len(pub.Fields[3].Options) != 2 {
		t.Fatalf("public form %+v err=%v", pub, err)
	}
	if none, _ := f.svc.ForChannel(ctx, f.org, "email"); none != nil {
		t.Fatal("e-mail channel has no form")
	}

	// Missing topic → field error keyed by form key.
	_, _, err = f.svc.Check(ctx, f.org, "widget", &model.Submission{FormUUID: form.UUID, Values: map[string]any{"name": "Ayşe", "email": "a@example.com"}}, "message")
	var ve *customfields.ValidationError
	if !errors.As(err, &ve) || ve.Errors[0].Field != "conversation.topic" {
		t.Fatalf("expected topic required, got %v", err)
	}
	sub, formUUID, err := f.svc.Check(ctx, f.org, "widget", &model.Submission{FormUUID: form.UUID, Values: map[string]any{
		"name": "Ayşe", "email": "AYSE@example.com", "conversation.topic": "billing", "contact.plan": "Pro", "priority": "high",
	}}, "message")
	if err != nil || formUUID != form.UUID {
		t.Fatal(err)
	}

	contact := scalarID(t, f.pool, "INSERT INTO contacts (organization_id) VALUES ($1) RETURNING id", f.org)
	var convID int64
	var convUUID uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO conversations (organization_id, number, channel_type, contact_id)
		VALUES ($1, 1, 'widget', $2) RETURNING id, uuid`, f.org, contact).Scan(&convID, &convUUID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Apply(ctx, f.org, convID, formUUID, *sub); err != nil {
		t.Fatal(err)
	}
	var priority, name, email string
	var cf, contactCF []byte
	_ = f.pool.QueryRow(ctx, "SELECT priority, custom_fields FROM conversations WHERE id = $1", convID).Scan(&priority, &cf)
	_ = f.pool.QueryRow(ctx, "SELECT name, email, custom_fields FROM contacts WHERE id = $1", contact).Scan(&name, &email, &contactCF)
	if priority != "high" || name != "Ayşe" || email != "ayse@example.com" || !strings.Contains(string(cf), `"topic": "billing"`) || !strings.Contains(string(contactCF), `"plan": "Pro"`) {
		t.Fatalf("applied: %s %s %s %s %s", priority, name, email, cf, contactCF)
	}

	// The billing topic needs an order number before resolving.
	cv, err := f.svc.ConversationForm(ctx, f.org, convUUID)
	if err != nil || cv.Form == nil || len(cv.MissingToResolve) != 1 || cv.MissingToResolve[0] != "conversation.order_no" {
		t.Fatalf("conversation form %+v err=%v", cv, err)
	}
	conv, _ := f.q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: f.org, ID: convID})
	if err := f.svc.CheckResolve(ctx, conv); err == nil || !strings.Contains(err.Error(), "Sipariş no") {
		t.Fatalf("resolve guard err=%v", err)
	}
	if _, err := f.svc.UpdateConversationValues(ctx, f.org, convUUID, map[string]any{"conversation.topic": "nope"}); !errors.As(err, &ve) {
		t.Fatalf("bad option err=%v", err)
	}
	cv, err = f.svc.UpdateConversationValues(ctx, f.org, convUUID, map[string]any{"conversation.order_no": "A-1001"})
	if err != nil || len(cv.MissingToResolve) != 0 || cv.Values["conversation.order_no"] != "A-1001" {
		t.Fatalf("after update %+v err=%v", cv, err)
	}
	conv, _ = f.q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: f.org, ID: convID})
	if err := f.svc.CheckResolve(ctx, conv); err != nil {
		t.Fatalf("resolve guard after fill: %v", err)
	}
	var stored map[string]any
	_ = json.Unmarshal(conv.CustomFields, &stored)
	if stored["topic"] != "billing" {
		t.Fatalf("other values lost: %v", stored)
	}

	// A deactivated form is no longer offered and accepts no submission.
	off := false
	in := f.input("Destek talebi")
	in.IsActive = &off
	if _, err := f.svc.Update(ctx, f.org, form.UUID, in); err != nil {
		t.Fatal(err)
	}
	if s, _, err := f.svc.Check(ctx, f.org, "widget", nil, "message"); s != nil || err != nil {
		t.Fatalf("inactive form checked: %v %v", s, err)
	}
	if err := f.svc.Delete(ctx, f.org, form.UUID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Delete(ctx, f.org, form.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("second delete err=%v", err)
	}
}
