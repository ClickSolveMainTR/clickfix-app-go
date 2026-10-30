// Package ioadapter plugs contacts and companies into the import/export
// engine (CF-84). Both adapters are organization-scoped: they read the
// organization from orgctx, which the import service sets from the job (also
// in the worker), and the export handlers from the request.
//
// Imports dedupe instead of failing on known records: a contact row matching
// an existing contact by e-mail (then phone) updates it, a company row
// matching by external id, then domain, then exact name updates it. Updates
// only touch the columns the row fills, and store the previous values so a
// rollback can restore them; created rows are soft deleted on rollback.
package ioadapter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/i18n"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Resource slugs.
const (
	ResourceContacts  = "tenant.contacts"
	ResourceCompanies = "tenant.companies"

	entityContact = "contact"
	entityCompany = "company"
	opCreate      = "create"
	opUpdate      = "update"

	exportPage  = 500
	exportLimit = 50000
)

var errNoOrganization = errors.New("ioadapter: organization missing from context")

func orgOf(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// ---------------------------------------------------------------------------
// Contacts

// Contacts is the tenant.contacts adapter.
type Contacts struct {
	svc *usecase.Service
	q   *db.Queries
}

// NewContacts builds the contacts adapter.
func NewContacts(svc *usecase.Service, q *db.Queries) *Contacts { return &Contacts{svc: svc, q: q} }

// Resource implements ioengine.ResourceAdapter.
func (a *Contacts) Resource() string { return ResourceContacts }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *Contacts) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "uuid", LabelKey: "contacts.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "name", LabelKey: "contacts.name", Type: ioengine.ColumnTypeString},
		{Key: "email", LabelKey: "contacts.email", Type: ioengine.ColumnTypeString},
		{Key: "phone", LabelKey: "contacts.phone", Type: ioengine.ColumnTypeString},
		{Key: "job_title", LabelKey: "contacts.job_title", Type: ioengine.ColumnTypeString},
		{Key: "company", LabelKey: "contacts.company", Type: ioengine.ColumnTypeString},
		{Key: "locale", LabelKey: "contacts.locale", Type: ioengine.ColumnTypeString},
		{Key: "timezone", LabelKey: "contacts.timezone", Type: ioengine.ColumnTypeString},
		{Key: "channels", LabelKey: "contacts.channels", Type: ioengine.ColumnTypeString},
		{Key: "conversation_count", LabelKey: "contacts.conversation_count", Type: ioengine.ColumnTypeString},
		{Key: "blocked", LabelKey: "contacts.blocked", Type: ioengine.ColumnTypeBoolean},
		{Key: "notes", LabelKey: "contacts.notes", Type: ioengine.ColumnTypeString},
		{Key: "created_at", LabelKey: "contacts.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

// Export implements ioengine.ResourceAdapter. Query keys: q, company (uuid).
func (a *Contacts) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := orgOf(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := usecase.ContactFilter{Q: query["q"], Sort: "created_at", Limit: exportPage}
	if raw := strings.TrimSpace(query["company"]); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return ioengine.Dataset{}, fmt.Errorf("%w: company must be a uuid", usecase.ErrInvalid)
		}
		f.Company = &id
	}
	rows := make([]map[string]any, 0, exportPage)
	for f.Offset = 0; f.Offset < exportLimit; f.Offset += exportPage {
		items, _, err := a.svc.ListContacts(ctx, orgID, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, c := range items {
			company := ""
			if c.Company != nil {
				company = c.Company.Name
			}
			rows = append(rows, map[string]any{
				"uuid": c.UUID.String(), "name": c.Name, "email": deref(c.Email), "phone": deref(c.Phone),
				"job_title": c.JobTitle, "company": company, "locale": deref(c.Locale), "timezone": deref(c.Timezone),
				"channels": strings.Join(c.ChannelTypes, ","), "conversation_count": fmt.Sprint(c.ConversationCount),
				"blocked": c.Blocked, "notes": c.Notes, "created_at": c.CreatedAt,
			})
		}
		if len(items) < exportPage {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourceContacts, Columns: a.ExportColumns(), Rows: rows}, nil
}

// ImportSchema implements ioengine.ResourceAdapter.
func (a *Contacts) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "name", LabelKey: "contacts.name", Type: ioengine.ColumnTypeString},
		{Key: "email", LabelKey: "contacts.email", Type: ioengine.ColumnTypeString},
		{Key: "phone", LabelKey: "contacts.phone", Type: ioengine.ColumnTypeString},
		{Key: "job_title", LabelKey: "contacts.job_title", Type: ioengine.ColumnTypeString},
		{Key: "company", LabelKey: "contacts.company", Type: ioengine.ColumnTypeString},
		{Key: "company_domain", LabelKey: "contacts.company_domain", Type: ioengine.ColumnTypeString},
		{Key: "locale", LabelKey: "contacts.locale", Type: ioengine.ColumnTypeString},
		{Key: "timezone", LabelKey: "contacts.timezone", Type: ioengine.ColumnTypeString},
		{Key: "notes", LabelKey: "contacts.notes", Type: ioengine.ColumnTypeString},
	}
}

// contactRow is a parsed import row.
type contactRow struct {
	in       usecase.ContactInput
	email    string
	phone    string
	existing *db.Contact
}

// parse validates a row and finds the contact it dedupes onto.
func (a *Contacts) parse(ctx context.Context, orgID int64, row, defaults map[string]any) (contactRow, string, error) {
	get := func(k string) string {
		if v := cell(row, k); v != "" {
			return v
		}
		return cell(defaults, k)
	}
	var r contactRow
	r.email = strings.ToLower(get("email"))
	r.phone = normalizePhone(get("phone"))
	name := get("name")
	if name == "" && r.email == "" && r.phone == "" {
		return r, "a name, e-mail or phone is required", nil
	}
	set := func(dst **string, v string) {
		if v != "" {
			*dst = &v
		}
	}
	set(&r.in.Name, name)
	set(&r.in.Email, r.email)
	set(&r.in.Phone, r.phone)
	set(&r.in.JobTitle, get("job_title"))
	set(&r.in.Locale, get("locale"))
	set(&r.in.Timezone, get("timezone"))
	set(&r.in.Notes, get("notes"))
	if co, msg, err := a.company(ctx, orgID, get("company_domain"), get("company")); err != nil || msg != "" {
		return r, msg, err
	} else if co != nil {
		id := co.Uuid
		p := &id
		r.in.Company = &p
	}
	if err := a.svc.ValidateContactInput(ctx, orgID, r.in); err != nil {
		return r, strings.TrimPrefix(err.Error(), usecase.ErrInvalid.Error()+": "), nil
	}
	if r.email != "" {
		c, err := a.q.FindContactByEmail(ctx, db.FindContactByEmailParams{OrganizationID: orgID, Email: r.email})
		if err == nil {
			r.existing = &c
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return r, "", err
		}
	}
	if r.existing == nil && r.phone != "" {
		c, err := a.q.FindContactByPhone(ctx, db.FindContactByPhoneParams{OrganizationID: orgID, Phone: r.phone})
		if err == nil {
			r.existing = &c
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return r, "", err
		}
	}
	return r, "", nil
}

// company resolves the company columns: domain first, then exact name.
// Unknown companies are not created; the contact's e-mail domain may still
// match one on create.
func (a *Contacts) company(ctx context.Context, orgID int64, domain, name string) (*db.Company, string, error) {
	if domain != "" {
		co, err := a.q.GetCompanyByDomain(ctx, db.GetCompanyByDomainParams{OrganizationID: orgID, Domain: normalizeDomain(domain)})
		if err == nil {
			return &co, "", nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, "", err
		}
	}
	if name != "" {
		co, err := a.q.FindCompanyByName(ctx, db.FindCompanyByNameParams{OrganizationID: orgID, Name: name})
		if err == nil {
			return &co, "", nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, "", err
		}
	}
	if domain != "" || name != "" {
		return nil, "company not found", nil
	}
	return nil, "", nil
}

// PreviewRow implements ioengine.RowPreviewer.
func (a *Contacts) PreviewRow(ctx context.Context, row, defaults map[string]any) (string, string, error) {
	orgID, err := orgOf(ctx)
	if err != nil {
		return "", "", err
	}
	r, msg, err := a.parse(ctx, orgID, row, defaults)
	if err != nil || msg != "" {
		return "", msg, err
	}
	if r.existing != nil {
		return opUpdate, "", nil
	}
	return opCreate, "", nil
}

// ApplyRow implements ioengine.ResourceAdapter.
func (a *Contacts) ApplyRow(ctx context.Context, row, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := orgOf(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	r, msg, err := a.parse(ctx, orgID, row, defaults)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	if msg != "" {
		return ioengine.RowResult{OK: false, Error: msg}, nil
	}
	if r.existing == nil {
		c, err := a.svc.CreateContact(ctx, orgID, r.in)
		if err != nil {
			return rowErr(err)
		}
		return ioengine.RowResult{OK: true, EntityType: entityContact, EntityUUID: c.UUID.String(), Op: opCreate}, nil
	}
	before, err := a.svc.GetContact(ctx, orgID, r.existing.Uuid)
	if err != nil {
		return rowErr(err)
	}
	if _, err := a.svc.UpdateContact(ctx, orgID, r.existing.Uuid, r.in); err != nil {
		return rowErr(err)
	}
	company := ""
	if before.Company != nil {
		company = before.Company.UUID.String()
	}
	return ioengine.RowResult{
		OK: true, EntityType: entityContact, EntityUUID: before.UUID.String(), Op: opUpdate,
		Previous: map[string]any{
			"name": before.Name, "email": deref(before.Email), "phone": deref(before.Phone), "job_title": before.JobTitle,
			"locale": deref(before.Locale), "timezone": deref(before.Timezone), "notes": before.Notes, "company_uuid": company,
		},
	}, nil
}

// RevertRow implements ioengine.ResourceAdapter: created contacts are soft
// deleted, updated ones get their previous values back.
func (a *Contacts) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	if entityType != entityContact {
		return fmt.Errorf("unsupported entity %q", entityType)
	}
	orgID, err := orgOf(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	if previous == nil {
		if err := a.svc.DeleteContact(ctx, orgID, id); err != nil && !errors.Is(err, usecase.ErrNotFound) {
			return err
		}
		return nil
	}
	in := usecase.ContactInput{}
	for k, dst := range map[string]**string{
		"name": &in.Name, "email": &in.Email, "phone": &in.Phone, "job_title": &in.JobTitle,
		"locale": &in.Locale, "timezone": &in.Timezone, "notes": &in.Notes,
	} {
		v := cell(previous, k)
		*dst = &v
	}
	var company *uuid.UUID
	if raw := cell(previous, "company_uuid"); raw != "" {
		if cid, err := uuid.Parse(raw); err == nil {
			company = &cid
		}
	}
	in.Company = &company
	_, err = a.svc.UpdateContact(ctx, orgID, id, in)
	if errors.Is(err, usecase.ErrInvalid) && company != nil {
		// The company is gone since: restore the rest.
		var none *uuid.UUID
		in.Company = &none
		_, err = a.svc.UpdateContact(ctx, orgID, id, in)
	}
	return err
}

// SampleRows is the downloadable template content.
func (a *Contacts) SampleRows() []map[string]any {
	return []map[string]any{
		{"name": "Ayşe Yılmaz", "email": "ayse@example.com", "phone": "+905551112233", "job_title": "Satın alma müdürü", "company": "", "company_domain": "example.com", "locale": "tr", "timezone": "Europe/Istanbul", "notes": ""},
	}
}

// ---------------------------------------------------------------------------
// Companies

// Companies is the tenant.companies adapter.
type Companies struct {
	svc *usecase.Service
	q   *db.Queries
}

// NewCompanies builds the companies adapter.
func NewCompanies(svc *usecase.Service, q *db.Queries) *Companies { return &Companies{svc: svc, q: q} }

// Resource implements ioengine.ResourceAdapter.
func (a *Companies) Resource() string { return ResourceCompanies }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *Companies) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "uuid", LabelKey: "companies.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "name", LabelKey: "companies.name", Type: ioengine.ColumnTypeString},
		{Key: "domain", LabelKey: "companies.domain", Type: ioengine.ColumnTypeString},
		{Key: "external_id", LabelKey: "companies.external_id", Type: ioengine.ColumnTypeString},
		{Key: "tier", LabelKey: "companies.tier", Type: ioengine.ColumnTypeString},
		{Key: "account_owner", LabelKey: "companies.account_owner", Type: ioengine.ColumnTypeString},
		{Key: "contact_count", LabelKey: "companies.contact_count", Type: ioengine.ColumnTypeString},
		{Key: "open_conversations", LabelKey: "companies.open_conversations", Type: ioengine.ColumnTypeString},
		{Key: "plan", LabelKey: "companies.plan", Type: ioengine.ColumnTypeString},
		{Key: "mrr", LabelKey: "companies.mrr", Type: ioengine.ColumnTypeString},
		{Key: "health_score", LabelKey: "companies.health_score", Type: ioengine.ColumnTypeString},
		{Key: "notes", LabelKey: "companies.notes", Type: ioengine.ColumnTypeString},
		{Key: "created_at", LabelKey: "companies.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

// Export implements ioengine.ResourceAdapter. Query key: q.
func (a *Companies) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := orgOf(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, exportPage)
	for offset := int32(0); offset < exportLimit; offset += exportPage {
		items, _, err := a.svc.ListCompanies(ctx, orgID, query["q"], exportPage, offset)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, c := range items {
			owner, plan, mrr, health := "", "", "", ""
			if c.AccountOwner != nil {
				owner = c.AccountOwner.Name
			}
			if c.Revenue != nil {
				plan = c.Revenue.Plan
				mrr = fmt.Sprintf("%d.%02d %s", c.Revenue.MRRMinor/100, c.Revenue.MRRMinor%100, c.Revenue.Currency)
			}
			if c.Health != nil && c.Health.Score != nil {
				health = fmt.Sprint(*c.Health.Score)
			}
			rows = append(rows, map[string]any{
				"uuid": c.UUID.String(), "name": c.Name, "domain": deref(c.Domain), "external_id": deref(c.ExternalID),
				"tier": c.Tier, "account_owner": owner, "contact_count": fmt.Sprint(c.ContactCount),
				"open_conversations": fmt.Sprint(c.OpenConversations), "plan": plan, "mrr": mrr, "health_score": health,
				"notes": c.Notes, "created_at": c.CreatedAt,
			})
		}
		if len(items) < exportPage {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourceCompanies, Columns: a.ExportColumns(), Rows: rows}, nil
}

// ImportSchema implements ioengine.ResourceAdapter.
func (a *Companies) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "name", LabelKey: "companies.name", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "domain", LabelKey: "companies.domain", Type: ioengine.ColumnTypeString},
		{Key: "external_id", LabelKey: "companies.external_id", Type: ioengine.ColumnTypeString},
		{Key: "tier", LabelKey: "companies.tier", Type: ioengine.ColumnTypeString},
		{Key: "account_owner_email", LabelKey: "companies.account_owner_email", Type: ioengine.ColumnTypeString},
		{Key: "notes", LabelKey: "companies.notes", Type: ioengine.ColumnTypeString},
	}
}

type companyRow struct {
	in       usecase.CompanyInput
	existing *db.Company
}

func (a *Companies) parse(ctx context.Context, orgID int64, row, defaults map[string]any) (companyRow, string, error) {
	get := func(k string) string {
		if v := cell(row, k); v != "" {
			return v
		}
		return cell(defaults, k)
	}
	var r companyRow
	name, domain, ext := get("name"), normalizeDomain(get("domain")), get("external_id")
	if name == "" {
		return r, "name is required", nil
	}
	set := func(dst **string, v string) {
		if v != "" {
			*dst = &v
		}
	}
	set(&r.in.Name, name)
	set(&r.in.Domain, domain)
	set(&r.in.ExternalID, ext)
	set(&r.in.Tier, get("tier"))
	set(&r.in.Notes, get("notes"))
	if email := get("account_owner_email"); email != "" {
		m, err := a.q.FindOrganizationMemberByEmail(ctx, db.FindOrganizationMemberByEmailParams{OrganizationID: orgID, Email: email})
		if errors.Is(err, pgx.ErrNoRows) {
			return r, "account owner is not a member", nil
		}
		if err != nil {
			return r, "", err
		}
		id := m.Uuid
		p := &id
		r.in.AccountOwner = &p
	}
	if err := a.svc.ValidateCompanyInput(ctx, orgID, r.in); err != nil {
		return r, strings.TrimPrefix(err.Error(), usecase.ErrInvalid.Error()+": "), nil
	}
	lookups := []func() (db.Company, error){}
	if ext != "" {
		lookups = append(lookups, func() (db.Company, error) {
			return a.q.GetCompanyByExternalID(ctx, db.GetCompanyByExternalIDParams{OrganizationID: orgID, ExternalID: ext})
		})
	}
	if domain != "" {
		lookups = append(lookups, func() (db.Company, error) {
			return a.q.GetCompanyByDomain(ctx, db.GetCompanyByDomainParams{OrganizationID: orgID, Domain: domain})
		})
	}
	lookups = append(lookups, func() (db.Company, error) {
		return a.q.FindCompanyByName(ctx, db.FindCompanyByNameParams{OrganizationID: orgID, Name: name})
	})
	for _, find := range lookups {
		co, err := find()
		if err == nil {
			r.existing = &co
			break
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return r, "", err
		}
	}
	if r.existing != nil && domain != "" && r.existing.Domain.Valid && !strings.EqualFold(r.existing.Domain.String, domain) {
		// Matched by external id / name but the domain belongs to another
		// company: the unique index would refuse the update.
		other, err := a.q.GetCompanyByDomain(ctx, db.GetCompanyByDomainParams{OrganizationID: orgID, Domain: domain})
		if err == nil && other.ID != r.existing.ID {
			return r, "another company has this domain", nil
		}
	}
	return r, "", nil
}

// PreviewRow implements ioengine.RowPreviewer.
func (a *Companies) PreviewRow(ctx context.Context, row, defaults map[string]any) (string, string, error) {
	orgID, err := orgOf(ctx)
	if err != nil {
		return "", "", err
	}
	r, msg, err := a.parse(ctx, orgID, row, defaults)
	if err != nil || msg != "" {
		return "", msg, err
	}
	if r.existing != nil {
		return opUpdate, "", nil
	}
	return opCreate, "", nil
}

// ApplyRow implements ioengine.ResourceAdapter.
func (a *Companies) ApplyRow(ctx context.Context, row, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := orgOf(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	r, msg, err := a.parse(ctx, orgID, row, defaults)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	if msg != "" {
		return ioengine.RowResult{OK: false, Error: msg}, nil
	}
	if r.existing == nil {
		c, err := a.svc.CreateCompany(ctx, orgID, r.in)
		if err != nil {
			return rowErr(err)
		}
		return ioengine.RowResult{OK: true, EntityType: entityCompany, EntityUUID: c.UUID.String(), Op: opCreate}, nil
	}
	before, err := a.svc.GetCompany(ctx, orgID, r.existing.Uuid)
	if err != nil {
		return rowErr(err)
	}
	if _, err := a.svc.UpdateCompany(ctx, orgID, r.existing.Uuid, r.in); err != nil {
		return rowErr(err)
	}
	owner := ""
	if before.AccountOwner != nil {
		owner = before.AccountOwner.UUID.String()
	}
	return ioengine.RowResult{
		OK: true, EntityType: entityCompany, EntityUUID: before.UUID.String(), Op: opUpdate,
		Previous: map[string]any{
			"name": before.Name, "domain": deref(before.Domain), "external_id": deref(before.ExternalID),
			"tier": before.Tier, "notes": before.Notes, "account_owner_uuid": owner,
		},
	}, nil
}

// RevertRow implements ioengine.ResourceAdapter.
func (a *Companies) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	if entityType != entityCompany {
		return fmt.Errorf("unsupported entity %q", entityType)
	}
	orgID, err := orgOf(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	if previous == nil {
		if err := a.svc.DeleteCompany(ctx, orgID, id); err != nil && !errors.Is(err, usecase.ErrNotFound) {
			return err
		}
		return nil
	}
	in := usecase.CompanyInput{}
	for k, dst := range map[string]**string{
		"name": &in.Name, "domain": &in.Domain, "external_id": &in.ExternalID, "tier": &in.Tier, "notes": &in.Notes,
	} {
		v := cell(previous, k)
		*dst = &v
	}
	var owner *uuid.UUID
	if raw := cell(previous, "account_owner_uuid"); raw != "" {
		if oid, err := uuid.Parse(raw); err == nil {
			owner = &oid
		}
	}
	in.AccountOwner = &owner
	_, err = a.svc.UpdateCompany(ctx, orgID, id, in)
	if errors.Is(err, usecase.ErrInvalid) && owner != nil {
		// The owner left the organization since: restore the rest.
		var none *uuid.UUID
		in.AccountOwner = &none
		_, err = a.svc.UpdateCompany(ctx, orgID, id, in)
	}
	return err
}

// SampleRows is the downloadable template content.
func (a *Companies) SampleRows() []map[string]any {
	return []map[string]any{
		{"name": "Örnek A.Ş.", "domain": "example.com", "external_id": "CRM-1001", "tier": "enterprise", "account_owner_email": "", "notes": ""},
	}
}

// ---------------------------------------------------------------------------

func cell(row map[string]any, key string) string {
	if row == nil {
		return ""
	}
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func normalizeDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
	d = strings.TrimPrefix(d, "www.")
	return strings.TrimSuffix(d, "/")
}

// normalizePhone keeps a leading + and the digits, so "0 (555) 111 22 33"
// and "05551112233" dedupe onto the same contact.
func normalizePhone(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	var b strings.Builder
	for i, r := range p {
		if (r == '+' && i == 0) || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// rowErr turns validation errors into a failed row and keeps real errors.
func rowErr(err error) (ioengine.RowResult, error) {
	if errors.Is(err, usecase.ErrInvalid) || errors.Is(err, usecase.ErrDomainExists) || errors.Is(err, usecase.ErrNotFound) {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	return ioengine.RowResult{}, err
}

var (
	_ ioengine.ResourceAdapter = (*Contacts)(nil)
	_ ioengine.RowPreviewer    = (*Contacts)(nil)
	_ ioengine.ResourceAdapter = (*Companies)(nil)
	_ ioengine.RowPreviewer    = (*Companies)(nil)
)
