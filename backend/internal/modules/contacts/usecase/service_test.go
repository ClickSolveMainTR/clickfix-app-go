package usecase_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ptr[T any](v T) *T { return &v }

func setup(t *testing.T) (*usecase.Service, *repository.Store, int64) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var org int64
	if err := pool.QueryRow(context.Background(), "INSERT INTO organizations (slug, name) VALUES ($1, 'CRM') RETURNING id", "crm-"+uuid.NewString()[:12]).Scan(&org); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org) })
	store := repository.New(pool, db.New(pool))
	return usecase.New(store), store, org
}

func TestCompaniesAdoptContactsByDomain(t *testing.T) {
	svc, store, org := setup(t)
	ctx := context.Background()
	early, err := svc.CreateContact(ctx, org, usecase.ContactInput{Name: ptr("Deniz"), Email: ptr("deniz@acme.com.tr")})
	if err != nil || early.Company != nil {
		t.Fatalf("contact before company: %+v %v", early, err)
	}
	free, _ := svc.CreateContact(ctx, org, usecase.ContactInput{Name: ptr("Ali"), Email: ptr("ali@gmail.com")})
	acme, err := svc.CreateCompany(ctx, org, usecase.CompanyInput{Name: ptr("Acme"), Domain: ptr("https://www.Acme.com.tr/"), Tier: ptr("enterprise")})
	if err != nil || acme.Domain == nil || *acme.Domain != "acme.com.tr" {
		t.Fatalf("company: %+v %v", acme, err)
	}
	if got, _ := svc.GetContact(ctx, org, early.UUID); got.Company == nil || got.Company.UUID != acme.UUID {
		t.Fatalf("existing contact on the domain should join the company: %+v", got.Company)
	}
	if got, _ := svc.GetContact(ctx, org, free.UUID); got.Company != nil {
		t.Fatal("free mail contacts never join a company")
	}
	late, _ := svc.CreateContact(ctx, org, usecase.ContactInput{Email: ptr("selin@acme.com.tr")})
	if late.Company == nil || late.Company.UUID != acme.UUID {
		t.Fatal("new contact should join the company by domain")
	}
	// The inbound pipeline matches the company too.
	var resolved repository.ResolvedContact
	_ = store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		var err error
		resolved, err = tx.ResolveContact(ctx, repository.IdentityInput{OrganizationID: org, ChannelType: "email", ExternalID: "ops@acme.com.tr", Email: "ops@acme.com.tr"})
		return err
	})
	if !resolved.Contact.CompanyID.Valid {
		t.Fatal("inbound contact should join the company by domain")
	}
	if _, err := svc.CreateCompany(ctx, org, usecase.CompanyInput{Name: ptr("Acme 2"), Domain: ptr("acme.com.tr")}); !errors.Is(err, usecase.ErrDomainExists) {
		t.Fatalf("duplicate domain = %v", err)
	}
	list, total, err := svc.ListContacts(ctx, org, usecase.ContactFilter{Company: &acme.UUID, Limit: 50})
	if err != nil || total != 3 || len(list) != 3 {
		t.Fatalf("company contacts = %d (%v)", total, err)
	}
	companies, _, _ := svc.ListCompanies(ctx, org, "acm", 50, 0)
	if len(companies) != 1 || companies[0].ContactCount != 3 || companies[0].Tier != "enterprise" {
		t.Fatalf("companies = %+v", companies)
	}
	if _, err := svc.CreateContact(ctx, org, usecase.ContactInput{Email: ptr("not-an-email")}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("invalid email = %v", err)
	}
	if err := svc.DeleteCompany(ctx, org, acme.UUID); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetContact(ctx, org, early.UUID); got.Company != nil {
		t.Fatal("deleting a company detaches its contacts")
	}
}

func TestMergeContactsKeepsIdentities(t *testing.T) {
	svc, store, org := setup(t)
	ctx := context.Background()
	var wa, mail repository.ResolvedContact
	_ = store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		wa, _ = tx.ResolveContact(ctx, repository.IdentityInput{OrganizationID: org, ChannelType: "whatsapp", ExternalID: "905550000000", DisplayName: "Mert"})
		mail, _ = tx.ResolveContact(ctx, repository.IdentityInput{OrganizationID: org, ChannelType: "email", ExternalID: "mert@firma.com", Email: "mert@firma.com"})
		return nil
	})
	res, err := svc.Merge(ctx, org, wa.Contact.Uuid, mail.Contact.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contact.Identities) != 2 || res.Contact.Email == nil || *res.Contact.Email != "mert@firma.com" {
		t.Fatalf("merged contact = %+v", res.Contact)
	}
	if _, err := svc.GetContact(ctx, org, mail.Contact.Uuid); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatal("merged contact must be gone")
	}
	if _, err := svc.Merge(ctx, org, wa.Contact.Uuid, wa.Contact.Uuid); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatal("self merge must fail")
	}
}
