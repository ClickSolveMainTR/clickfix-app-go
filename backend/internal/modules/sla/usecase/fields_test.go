package usecase_test

import (
	"context"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	cfmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/model"
	cfrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/repository"
	cfusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/google/uuid"
)

// CF-196: an SLA policy picks list field options from the catalog by id; a
// label typed as free text resolves to the id and renaming the option keeps
// the policy bound.
func TestCustomFieldConditionsUseOptionIDs(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	fields := cfusecase.New(cfrepository.New(f.pool, db.New(f.pool)))
	f.svc.WithFields(fields)
	def, err := fields.CreateDefinition(ctx, f.org, cfmodel.CreateDefinitionInput{
		EntityType: customfields.EntityContact, Key: "paket", Label: "Paket", Type: customfields.TypeSelect,
		Options: []customfields.Option{{Value: "pro", Label: "Pro Paket"}, {Value: "basic", Label: "Temel"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pro, err := f.svc.CreatePolicy(ctx, f.org, standardPolicy("Pro", model.Conditions{
		CustomFields: []customfields.FieldMatch{{Entity: customfields.EntityContact, Key: "paket", Options: []string{"Pro Paket"}}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := pro.Conditions.CustomFields; len(got) != 1 || len(got[0].Options) != 1 || got[0].Options[0] != "pro" {
		t.Fatalf("stored condition = %+v (want the option id)", got)
	}
	fallback, err := f.svc.CreatePolicy(ctx, f.org, standardPolicy("Varsayılan", model.Conditions{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreatePolicy(ctx, f.org, standardPolicy("Bozuk", model.Conditions{
		CustomFields: []customfields.FieldMatch{{Entity: customfields.EntityContact, Key: "paket", Options: []string{"gold"}}},
	})); err == nil {
		t.Fatal("unknown option accepted")
	}

	id, u := f.conversation(t, "widget", "normal", f.now)
	f.exec(t, `UPDATE contacts SET custom_fields = '{"paket":"pro"}' WHERE id = (SELECT contact_id FROM conversations WHERE id = $1)`, id)
	policyOf := func() uuid.UUID {
		t.Helper()
		if err := f.svc.Sync(ctx, f.org, id, f.now); err != nil {
			t.Fatal(err)
		}
		sla, err := f.svc.ConversationSLA(ctx, f.org, u)
		if err != nil || sla.Policy == nil {
			t.Fatalf("sla %+v err=%v", sla, err)
		}
		return sla.Policy.UUID
	}
	if got := policyOf(); got != pro.UUID {
		t.Fatalf("pro policy not applied: %s", got)
	}
	renamed := []customfields.Option{{Value: "pro", Label: "Profesyonel"}, {Value: "basic", Label: "Temel"}}
	if _, err := fields.UpdateDefinition(ctx, f.org, def.UUID, cfmodel.UpdateDefinitionInput{Options: &renamed}); err != nil {
		t.Fatal(err)
	}
	if got := policyOf(); got != pro.UUID {
		t.Fatalf("policy lost after the option was renamed: %s", got)
	}
	f.exec(t, `UPDATE contacts SET custom_fields = '{"paket":"basic"}' WHERE id = (SELECT contact_id FROM conversations WHERE id = $1)`, id)
	if got := policyOf(); got != fallback.UUID {
		t.Fatalf("basic contact got %s", got)
	}
}
