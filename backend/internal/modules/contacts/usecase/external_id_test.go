package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
)

// CF-195: the same external id twice edits one contact.
func TestUpsertContactByExternalID(t *testing.T) {
	svc, _, org := setup(t)
	ctx := context.Background()
	first, created, err := svc.UpsertContact(ctx, org, usecase.ContactInput{ExternalID: ptr(" u-1001 "), Name: ptr("Ayşe"), Email: ptr("ayse@example.com")})
	if err != nil || !created {
		t.Fatalf("first upsert: created=%v err=%v", created, err)
	}
	if first.ExternalID == nil || *first.ExternalID != "u-1001" {
		t.Fatalf("external id = %v", first.ExternalID)
	}
	if first.IdentityState != model.IdentitySelfNamed || first.VerifiedAt != nil {
		t.Fatalf("an API contact is not verified: %s", first.IdentityState)
	}
	second, created, err := svc.UpsertContact(ctx, org, usecase.ContactInput{ExternalID: ptr("u-1001"), Name: ptr("Ayşe Yılmaz")})
	if err != nil || created {
		t.Fatalf("second upsert: created=%v err=%v", created, err)
	}
	if second.UUID != first.UUID || second.Name != "Ayşe Yılmaz" || second.Email == nil || *second.Email != "ayse@example.com" {
		t.Fatalf("second upsert should update the same contact: %+v", second)
	}
	list, total, err := svc.ListContacts(ctx, org, usecase.ContactFilter{ExternalID: "u-1001", Limit: 10})
	if err != nil || total != 1 || len(list) != 1 || list[0].UUID != first.UUID {
		t.Fatalf("search by external id = %d %v", total, err)
	}
	if _, total, _ := svc.ListContacts(ctx, org, usecase.ContactFilter{Q: "u-100", Limit: 10}); total != 1 {
		t.Fatalf("text search should match the external id, got %d", total)
	}
	if _, _, err := svc.UpsertContact(ctx, org, usecase.ContactInput{Name: ptr("x")}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("upsert without external id = %v", err)
	}
	other, err := svc.CreateContact(ctx, org, usecase.ContactInput{Name: ptr("Başka")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateContact(ctx, org, other.UUID, usecase.ContactInput{ExternalID: ptr("u-1001")}); !errors.Is(err, usecase.ErrExternalIDExists) {
		t.Fatalf("duplicate external id = %v", err)
	}
	if _, err := svc.CreateContact(ctx, org, usecase.ContactInput{Name: ptr("Üçüncü"), ExternalID: ptr("u-1001")}); !errors.Is(err, usecase.ErrExternalIDExists) {
		t.Fatalf("duplicate external id on create = %v", err)
	}
}

// CF-195: an anonymous contact that typed the customer's e-mail is never
// taken over by an upsert, and only identify makes a contact verified.
func TestAnonymousAndVerifiedDoNotMix(t *testing.T) {
	svc, store, org := setup(t)
	ctx := context.Background()
	var anon, typed repository.ResolvedContact
	err := store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		var err error
		if anon, err = tx.ResolveContact(ctx, repository.IdentityInput{OrganizationID: org, ChannelType: "widget", ExternalID: "visitor:a1"}); err != nil {
			return err
		}
		typed, err = tx.ResolveContact(ctx, repository.IdentityInput{OrganizationID: org, ChannelType: "widget", ExternalID: "visitor:a2", DisplayName: "Mehmet", Email: "mehmet@example.com"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	api, created, err := svc.UpsertContact(ctx, org, usecase.ContactInput{ExternalID: ptr("u-7"), Name: ptr("Mehmet"), Email: ptr("mehmet@example.com")})
	if err != nil || !created {
		t.Fatalf("upsert: %v %v", created, err)
	}
	if api.UUID == typed.Contact.Uuid {
		t.Fatal("upsert must not adopt the contact that only typed the address")
	}
	got, _ := svc.GetContact(ctx, org, typed.Contact.Uuid)
	if got.ExternalID != nil || got.IdentityState != model.IdentitySelfNamed {
		t.Fatalf("typed contact = %+v", got)
	}
	if got, _ := svc.GetContact(ctx, org, anon.Contact.Uuid); got.IdentityState != model.IdentityAnonymous {
		t.Fatalf("anonymous contact = %s", got.IdentityState)
	}
	// identify verifies the API contact (the widget runs this query).
	if err := store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		_, err := tx.Raw.Exec(ctx, "UPDATE contacts SET verified_at = now() WHERE organization_id = $1 AND uuid = $2", org, api.UUID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for state, want := range map[string]int64{model.IdentityVerified: 1, model.IdentityAnonymous: 1, model.IdentitySelfNamed: 1} {
		list, total, err := svc.ListContacts(ctx, org, usecase.ContactFilter{IdentityState: state, Limit: 10})
		if err != nil || total != want {
			t.Fatalf("%s: %d %v", state, total, err)
		}
		if list[0].IdentityState != state {
			t.Fatalf("%s filter returned %s", state, list[0].IdentityState)
		}
	}
	if _, _, err := svc.ListContacts(ctx, org, usecase.ContactFilter{IdentityState: "nope"}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("unknown state = %v", err)
	}
}

// CF-195: the company's external id is its own field, unique per
// organization and independent from its contacts' ids.
func TestCompanyExternalIDIsSeparateAndUnique(t *testing.T) {
	svc, _, org := setup(t)
	ctx := context.Background()
	co, err := svc.CreateCompany(ctx, org, usecase.CompanyInput{Name: ptr("Acme"), ExternalID: ptr("42")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.UpsertContact(ctx, org, usecase.ContactInput{ExternalID: ptr("42"), Name: ptr("Kişi 42"), Company: ptr(&co.UUID)}); err != nil {
		t.Fatalf("a contact may use the same value as a company: %v", err)
	}
	if _, err := svc.CreateCompany(ctx, org, usecase.CompanyInput{Name: ptr("Acme 2"), ExternalID: ptr("42")}); !errors.Is(err, usecase.ErrExternalIDExists) {
		t.Fatalf("duplicate company external id = %v", err)
	}
	list, total, err := svc.ListCompaniesByExternalID(ctx, org, "", "42", 10, 0)
	if err != nil || total != 1 || list[0].UUID != co.UUID {
		t.Fatalf("company by external id = %d %v", total, err)
	}
}
