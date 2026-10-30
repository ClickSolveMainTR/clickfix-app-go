package ioadapter_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/ioadapter"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	importusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/imports/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/i18n"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type env struct {
	pool      *pgxpool.Pool
	svc       *usecase.Service
	imports   *importusecase.Service
	contacts  *ioadapter.Contacts
	companies *ioadapter.Companies
	org       int64
	actor     int64
	ownerUUID uuid.UUID
}

func ptr[T any](v T) *T { return &v }

// memDriver is the in-memory store behind the storage.Driver the import
// service needs (uploads and downloads only).
type memDriver struct {
	storage.Driver
	m *storage.Memory
}

func (d memDriver) Upload(ctx context.Context, f storage.File, path string) error {
	return d.m.Upload(ctx, f, path)
}

func (d memDriver) Download(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	return d.m.Download(ctx, path)
}

func setup(t *testing.T) env {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var e env
	e.pool = pool
	suffix := uuid.NewString()[:12]
	if err := pool.QueryRow(ctx, "INSERT INTO organizations (slug, name) VALUES ($1, 'IO') RETURNING id", "io-"+suffix).Scan(&e.org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname, status)
		VALUES ($1, 'x', 'Cem', 'Kaya', 'active') RETURNING id, uuid`, "csm-"+suffix+"@example.com").Scan(&e.actor, &e.ownerUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_members (organization_id, user_id, role_id)
		VALUES ($1, $2, (SELECT id FROM roles WHERE organization_id IS NULL AND scope = 'tenant' ORDER BY id LIMIT 1))`, e.org, e.actor); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM import_jobs WHERE organization_id = $1", e.org)
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", e.org)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", e.actor)
	})
	q := db.New(pool)
	e.svc = usecase.New(repository.New(pool, q))
	e.contacts = ioadapter.NewContacts(e.svc, q)
	e.companies = ioadapter.NewCompanies(e.svc, q)
	e.imports = importusecase.New(q, memDriver{m: storage.NewMemory()}, ioengine.NewRegistry(e.contacts, e.companies), nil, nil, nil, nil)
	return e
}

// run uploads, previews and applies a CSV like the UI does.
func (e env) run(t *testing.T, ctx context.Context, resource, csv string) (importusecase.ImportJobView, importusecase.ImportJobView) {
	t.Helper()
	job, err := e.imports.UploadTenant(ctx, e.org, e.actor, resource, ioengine.ImportCSV, "tr", "x.csv", strings.NewReader(csv))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if job.Status != "mapped" || len(job.Mapping) == 0 {
		t.Fatalf("headers should be auto-mapped: %+v", job)
	}
	preview, err := e.imports.Preview(ctx, job.UUID, e.actor)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	applied, err := e.imports.Confirm(ctx, job.UUID, e.actor)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	return preview, applied
}

func TestCompanyImportDedupesAndRollsBack(t *testing.T) {
	e := setup(t)
	ctx := orgctx.WithOrganizationID(context.Background(), e.org)
	existing, err := e.svc.CreateCompany(ctx, e.org, usecase.CompanyInput{Name: ptr("Acme"), Domain: ptr("acme.com"), Tier: ptr("smb")})
	if err != nil {
		t.Fatal(err)
	}
	var ownerEmail string
	_ = e.pool.QueryRow(ctx, "SELECT email FROM users WHERE id = $1", e.actor).Scan(&ownerEmail)

	csv := "name,domain,tier,account_owner_email\n" +
		"Acme Corp,https://www.acme.com/,enterprise," + ownerEmail + "\n" + // update by domain
		"Globex,globex.io,smb,\n" + // create
		",nameless.io,,\n" + // invalid: no name
		"Initech,initech.com,,nobody@example.com\n" // invalid: owner not a member
	preview, applied := e.run(t, ctx, ioadapter.ResourceCompanies, csv)
	ps := preview.PreviewSummary
	if ps.Total != 4 || ps.Valid != 2 || ps.Invalid != 2 || ps.Creates != 1 || ps.Updates != 1 {
		t.Fatalf("preview summary = %+v", ps)
	}
	if applied.Status != "applied" || applied.PreviewSummary.Valid != 2 || applied.PreviewSummary.Invalid != 2 {
		t.Fatalf("applied = %+v", applied)
	}
	got, _ := e.svc.GetCompany(ctx, e.org, existing.UUID)
	if got.Name != "Acme Corp" || got.Tier != "enterprise" || got.AccountOwner == nil || got.AccountOwner.UUID != e.ownerUUID {
		t.Fatalf("dedupe update = %+v", got)
	}
	list, total, _ := e.svc.ListCompanies(ctx, e.org, "", 50, 0)
	if total != 2 {
		t.Fatalf("companies after import = %d %+v", total, list)
	}

	// Export carries the imported rows.
	ds, err := e.companies.Export(ctx, ioengine.ExportQuery{}, i18n.LocaleTR)
	if err != nil || len(ds.Rows) != 2 {
		t.Fatalf("export = %d %v", len(ds.Rows), err)
	}
	if data, err := ioengine.EncodeExport(ioengine.ExportCSV, ds, "tr", nil, "companies"); err != nil || !bytes.Contains(data, []byte("Globex")) {
		t.Fatalf("csv = %s %v", data, err)
	}

	rolled, err := e.imports.Rollback(ctx, applied.UUID, e.actor)
	if err != nil || rolled.Status != "rolled_back" {
		t.Fatalf("rollback = %+v %v", rolled, err)
	}
	got, _ = e.svc.GetCompany(ctx, e.org, existing.UUID)
	if got.Name != "Acme" || got.Tier != "smb" || got.AccountOwner != nil {
		t.Fatalf("rollback should restore the updated company: %+v", got)
	}
	if _, total, _ := e.svc.ListCompanies(ctx, e.org, "", 50, 0); total != 1 {
		t.Fatalf("rollback should remove the created company, total = %d", total)
	}
}

func TestContactImportDedupesByEmailAndPhone(t *testing.T) {
	e := setup(t)
	ctx := orgctx.WithOrganizationID(context.Background(), e.org)
	acme, _ := e.svc.CreateCompany(ctx, e.org, usecase.CompanyInput{Name: ptr("Acme"), Domain: ptr("acme.com")})
	byEmail, _ := e.svc.CreateContact(ctx, e.org, usecase.ContactInput{Name: ptr("Old Name"), Email: ptr("deniz@acme.com")})
	byPhone, _ := e.svc.CreateContact(ctx, e.org, usecase.ContactInput{Name: ptr("Phone Only"), Phone: ptr("+905551112233")})

	csv := "Ad Soyad,E-posta,Telefon,Unvan,Şirket\n" +
		"Deniz Ak,DENIZ@acme.com,,CTO,\n" + // update by e-mail (case-insensitive)
		"Ali Veli,,+90 (555) 111 22 33,,\n" + // update by normalized phone
		"Selin,selin@acme.com,,,Acme\n" + // create, company by name
		"Bad,not-an-email,,,\n" + // invalid e-mail
		"Ghost,ghost@x.com,,,Nope Inc\n" // unknown company
	preview, applied := e.run(t, ctx, ioadapter.ResourceContacts, csv)
	ps := preview.PreviewSummary
	if ps.Valid != 3 || ps.Invalid != 2 || ps.Creates != 1 || ps.Updates != 2 {
		t.Fatalf("preview = %+v", ps)
	}
	if applied.Status != "applied" {
		t.Fatalf("applied = %+v", applied)
	}
	if c, _ := e.svc.GetContact(ctx, e.org, byEmail.UUID); c.Name != "Deniz Ak" || c.JobTitle != "CTO" {
		t.Fatalf("email dedupe = %+v", c)
	}
	if c, _ := e.svc.GetContact(ctx, e.org, byPhone.UUID); c.Name != "Ali Veli" {
		t.Fatalf("phone dedupe = %+v", c)
	}
	list, total, _ := e.svc.ListContacts(ctx, e.org, usecase.ContactFilter{Company: &acme.UUID, Limit: 50})
	if total != 2 {
		t.Fatalf("acme contacts = %d %+v", total, list)
	}

	// Another organization cannot see or roll back the job.
	var other int64
	_ = e.pool.QueryRow(ctx, "INSERT INTO organizations (slug, name) VALUES ($1, 'Other') RETURNING id", "io-o-"+uuid.NewString()[:10]).Scan(&other)
	t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", other) })
	if _, err := e.imports.Rollback(orgctx.WithOrganizationID(context.Background(), other), applied.UUID, e.actor); err == nil {
		t.Fatal("rollback from another organization must fail")
	}
	if _, err := e.imports.Rollback(context.Background(), applied.UUID, e.actor); err == nil {
		t.Fatal("platform scope must not touch tenant jobs")
	}
	if _, err := e.imports.GetTenantJob(ctx, other, applied.UUID); err == nil {
		t.Fatal("job of another organization")
	}

	if _, err := e.imports.Rollback(ctx, applied.UUID, e.actor); err != nil {
		t.Fatal(err)
	}
	if c, _ := e.svc.GetContact(ctx, e.org, byEmail.UUID); c.Name != "Old Name" || c.JobTitle != "" {
		t.Fatalf("rollback restore = %+v", c)
	}
	if _, total, _ := e.svc.ListContacts(ctx, e.org, usecase.ContactFilter{Limit: 50}); total != 2 {
		t.Fatalf("rollback should delete the created contact, total = %d", total)
	}
	jobs, n, err := e.imports.ListTenantJobs(ctx, e.org, ioadapter.ResourceContacts, 10, 0)
	if err != nil || n != 1 || len(jobs) != 1 {
		t.Fatalf("jobs = %d %v", n, err)
	}
}
