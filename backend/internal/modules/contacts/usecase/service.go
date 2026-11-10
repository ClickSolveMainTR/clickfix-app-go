// Package usecase manages contacts, their channel identities and companies
// (CF-84). Contacts are created by the inbound pipeline as well; here agents
// edit them, link them to companies and merge duplicates.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors returned by the service.
var (
	ErrNotFound     = errors.New("contacts: not found")
	ErrInvalid      = errors.New("contacts: invalid request")
	ErrDomainExists = errors.New("contacts: another company has this domain")
	// ErrExternalIDExists: another contact (or company) of the organization
	// already uses this external id (CF-195).
	ErrExternalIDExists = errors.New("contacts: external id is already used")
)

// MaxExternalIDLen bounds the customer's own ids (the widget accepts the
// same length for user_id and company.id).
const MaxExternalIDLen = 200

// Ref names a related record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Identity is how a contact is known on one channel.
type Identity struct {
	ID          int64      `json:"id"`
	ChannelType string     `json:"channel_type"`
	Inbox       *uuid.UUID `json:"inbox_uuid"`
	ExternalID  string     `json:"external_id"`
	DisplayName string     `json:"display_name"`
	Username    string     `json:"username"`
	LastSeenAt  *time.Time `json:"last_seen_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Contact is the API shape of a contact.
type Contact struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Email *string   `json:"email"`
	Phone *string   `json:"phone"`
	// ExternalID is the customer's own user id (CF-195): unique in the
	// organization, the same value as widget identify user_id.
	ExternalID *string `json:"external_id"`
	// IdentityState: verified (user_hash proved the id), anonymous or
	// self_named; VerifiedAt is the last successful identify.
	IdentityState     string     `json:"identity_state"`
	VerifiedAt        *time.Time `json:"verified_at"`
	JobTitle          string     `json:"job_title"`
	Company           *Ref       `json:"company"`
	Locale            *string    `json:"locale"`
	Timezone          *string    `json:"timezone"`
	Notes             string     `json:"notes"`
	CustomFields      any        `json:"custom_fields"`
	Blocked           bool       `json:"blocked"`
	ChannelTypes      []string   `json:"channel_types"`
	ConversationCount int64      `json:"conversation_count"`
	Identities        []Identity `json:"identities,omitempty"`
	LastSeenAt        *time.Time `json:"last_seen_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// Company is the API shape of a company.
type Company struct {
	UUID       uuid.UUID `json:"uuid"`
	Name       string    `json:"name"`
	Domain     *string   `json:"domain"`
	ExternalID *string   `json:"external_id"`
	// VerifiedAt is the last time a user proved ExternalID with a
	// company-signed user_hash (CF-206); nil when never proved.
	VerifiedAt        *time.Time `json:"verified_at"`
	Tier              string     `json:"tier"`
	AccountOwner      *Ref       `json:"account_owner"`
	Notes             string     `json:"notes"`
	CustomFields      any        `json:"custom_fields"`
	ContactCount      int64      `json:"contact_count"`
	OpenConversations int64      `json:"open_conversations"`
	// Revenue is the customer's revenue data (CF-86); nil when never set.
	Revenue *Revenue `json:"revenue"`
	// Health is the customer health score (CF-87); nil until computed.
	Health    *HealthSummary `json:"health"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// Service manages contacts and companies.
type Service struct {
	q     *db.Queries
	store *repository.Store
}

// New builds the service.
func New(store *repository.Store) *Service {
	return &Service{q: store.Q(), store: store}
}

// ---------------------------------------------------------------------------
// Contacts

// ContactFilter narrows the contact list.
type ContactFilter struct {
	Q string
	// ExternalID matches the customer's own user id exactly (CF-195).
	ExternalID string
	// IdentityState: verified, anonymous or self_named; "" for any.
	IdentityState string
	Company       *uuid.UUID
	Sort          string
	Desc          bool
	Limit         int32
	Offset        int32
	Detailed      bool
}

// ListContacts returns one page of contacts.
func (s *Service) ListContacts(ctx context.Context, orgID int64, f ContactFilter) ([]Contact, int64, error) {
	p := db.ListContactsParams{OrganizationID: orgID, SortField: f.Sort, SortDesc: f.Desc, RowLimit: f.Limit, RowOffset: f.Offset}
	switch p.SortField {
	case "", "created_at":
		p.SortField = "created_at"
	case "name", "last_seen_at":
	default:
		return nil, 0, fmt.Errorf("%w: unknown sort", ErrInvalid)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: q, Valid: true}
	}
	p.ExternalID = text(f.ExternalID)
	if st := strings.TrimSpace(f.IdentityState); st != "" {
		if !model.ValidIdentityState(st) {
			return nil, 0, fmt.Errorf("%w: unknown identity state", ErrInvalid)
		}
		p.IdentityState = pgtype.Text{String: st, Valid: true}
	}
	if f.Company != nil {
		co, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: *f.Company})
		if err != nil {
			return []Contact{}, 0, nil
		}
		p.CompanyID = pgtype.Int8{Int64: co.ID, Valid: true}
	}
	rows, err := s.q.ListContacts(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountContacts(ctx, db.CountContactsParams{OrganizationID: orgID, CompanyID: p.CompanyID, Q: p.Q, ExternalID: p.ExternalID, IdentityState: p.IdentityState})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Contact, 0, len(rows))
	for _, r := range rows {
		c := contactOf(db.Contact{
			ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, CompanyID: r.CompanyID, Name: r.Name, Email: r.Email,
			Phone: r.Phone, Locale: r.Locale, Timezone: r.Timezone, Notes: r.Notes, CustomFields: r.CustomFields,
			Blocked: r.Blocked, LastSeenAt: r.LastSeenAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, JobTitle: r.JobTitle,
			ExternalID: r.ExternalID, VerifiedAt: r.VerifiedAt,
		})
		if r.CompanyUuid.Valid {
			c.Company = &Ref{UUID: uuid.UUID(r.CompanyUuid.Bytes), Name: r.CompanyName.String}
		}
		c.ConversationCount = r.ConversationCount
		c.ChannelTypes = nonNil(r.ChannelTypes)
		out = append(out, c)
	}
	return out, total, nil
}

// GetContact returns one contact with its identities.
func (s *Service) GetContact(ctx context.Context, orgID int64, id uuid.UUID) (Contact, error) {
	row, err := s.q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return Contact{}, notFound(err)
	}
	return s.detail(ctx, row)
}

func (s *Service) detail(ctx context.Context, row db.Contact) (Contact, error) {
	c := contactOf(row)
	if row.CompanyID.Valid {
		if co, err := s.companyByID(ctx, row.OrganizationID, row.CompanyID.Int64); err == nil {
			c.Company = &Ref{UUID: co.Uuid, Name: co.Name}
		}
	}
	ids, err := s.q.ListContactIdentities(ctx, db.ListContactIdentitiesParams{OrganizationID: row.OrganizationID, ContactID: row.ID})
	if err != nil {
		return Contact{}, err
	}
	c.Identities = []Identity{}
	seen := map[string]bool{}
	for _, i := range ids {
		it := Identity{
			ID: i.ID, ChannelType: i.ChannelType, ExternalID: i.ExternalID, DisplayName: i.DisplayName,
			Username: i.Username, LastSeenAt: timePtr(i.LastSeenAt), CreatedAt: i.CreatedAt.Time,
		}
		if i.ChannelInstanceUuid.Valid {
			u := uuid.UUID(i.ChannelInstanceUuid.Bytes)
			it.Inbox = &u
		}
		c.Identities = append(c.Identities, it)
		if !seen[i.ChannelType] {
			seen[i.ChannelType] = true
			c.ChannelTypes = append(c.ChannelTypes, i.ChannelType)
		}
	}
	return c, nil
}

func (s *Service) companyByID(ctx context.Context, orgID, id int64) (db.Company, error) {
	var co db.Company
	err := s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		return tx.Raw.QueryRow(ctx, "SELECT uuid, name FROM companies WHERE organization_id = $1 AND id = $2", orgID, id).Scan(&co.Uuid, &co.Name)
	})
	return co, err
}

// ContactInput creates or edits a contact. Nil pointers keep the stored
// value on update.
type ContactInput struct {
	Name         *string
	Email        *string
	Phone        *string
	JobTitle     *string
	Company      **uuid.UUID
	Locale       *string
	Timezone     *string
	Notes        *string
	CustomFields map[string]any
	Blocked      *bool
	// ExternalID sets the customer's own user id; "" clears it.
	ExternalID *string
}

// CreateContact creates a contact; without a company the e-mail domain
// picks one.
func (s *Service) CreateContact(ctx context.Context, orgID int64, in ContactInput) (Contact, error) {
	row := db.Contact{OrganizationID: orgID, CustomFields: []byte("{}")}
	if err := s.apply(ctx, orgID, &row, in); err != nil {
		return Contact{}, err
	}
	if row.Name == "" && !row.Email.Valid && !row.Phone.Valid {
		return Contact{}, fmt.Errorf("%w: a name, e-mail or phone is required", ErrInvalid)
	}
	if in.Company == nil && row.Email.Valid {
		if d := repository.CompanyDomain(row.Email.String); d != "" {
			if co, err := s.q.GetCompanyByDomain(ctx, db.GetCompanyByDomainParams{OrganizationID: orgID, Domain: d}); err == nil {
				row.CompanyID = pgtype.Int8{Int64: co.ID, Valid: true}
			}
		}
	}
	var created db.Contact
	err := s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		var err error
		created, err = tx.Q.InsertContact(ctx, db.InsertContactParams{OrganizationID: orgID, CompanyID: row.CompanyID, Name: row.Name, Email: row.Email, Phone: row.Phone, Locale: row.Locale, Timezone: row.Timezone, Notes: row.Notes, CustomFields: row.CustomFields, JobTitle: row.JobTitle, ExternalID: row.ExternalID})
		if err != nil {
			return err
		}
		return outbox.EnqueueQueries(ctx, tx.Q, events.New(events.ContactCreated).WithOrganization(orgID).WithEntity("contact", &created.ID, &created.Uuid).WithPayload(map[string]any{
			"contact_uuid": created.Uuid, "contact_external_id": textPtr(created.ExternalID),
		}).WithIdempotencyKey(created.Uuid.String()))
	})
	if err != nil {
		return Contact{}, uniqueContact(err)
	}
	return s.detail(ctx, created)
}

// UpdateContact edits a contact.
func (s *Service) UpdateContact(ctx context.Context, orgID int64, id uuid.UUID, in ContactInput) (Contact, error) {
	row, err := s.q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return Contact{}, notFound(err)
	}
	if err := s.apply(ctx, orgID, &row, in); err != nil {
		return Contact{}, err
	}
	updated, err := s.q.UpdateContact(ctx, db.UpdateContactParams{
		CompanyID: row.CompanyID, Name: row.Name, Email: row.Email, Phone: row.Phone, Locale: row.Locale,
		Timezone: row.Timezone, Notes: row.Notes, CustomFields: row.CustomFields, Blocked: row.Blocked,
		JobTitle: row.JobTitle, ExternalID: row.ExternalID, OrganizationID: orgID, ID: row.ID,
	})
	if err != nil {
		return Contact{}, notFound(uniqueContact(err))
	}
	return s.detail(ctx, updated)
}

// UpsertContact creates the contact with in.ExternalID or updates the one
// that already has it (CF-195): calling it twice with the same external id
// edits one contact. It never matches by e-mail or phone, so an anonymous
// contact that typed the same address is not taken over.
func (s *Service) UpsertContact(ctx context.Context, orgID int64, in ContactInput) (Contact, bool, error) {
	if in.ExternalID == nil || strings.TrimSpace(*in.ExternalID) == "" {
		return Contact{}, false, fmt.Errorf("%w: external_id is required", ErrInvalid)
	}
	ext := strings.TrimSpace(*in.ExternalID)
	in.ExternalID = &ext
	for attempt := 0; attempt < 2; attempt++ {
		row, err := s.q.GetContactByExternalID(ctx, db.GetContactByExternalIDParams{OrganizationID: orgID, ExternalID: ext})
		switch {
		case err == nil:
			c, err := s.UpdateContact(ctx, orgID, row.Uuid, in)
			return c, false, err
		case !errors.Is(err, pgx.ErrNoRows):
			return Contact{}, false, err
		}
		c, err := s.CreateContact(ctx, orgID, in)
		if errors.Is(err, ErrExternalIDExists) && attempt == 0 {
			// A concurrent call created it first: update that one.
			continue
		}
		return c, err == nil, err
	}
	return Contact{}, false, ErrExternalIDExists
}

func (s *Service) apply(ctx context.Context, orgID int64, row *db.Contact, in ContactInput) error {
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if len([]rune(n)) > 255 {
			return fmt.Errorf("%w: name is too long", ErrInvalid)
		}
		row.Name = n
	}
	if in.Email != nil {
		e := strings.TrimSpace(*in.Email)
		if e != "" {
			addr, err := mail.ParseAddress(e)
			if err != nil || addr.Address != e {
				return fmt.Errorf("%w: email is invalid", ErrInvalid)
			}
		}
		row.Email = text(strings.ToLower(e))
	}
	if in.Phone != nil {
		row.Phone = text(*in.Phone)
	}
	if in.JobTitle != nil {
		t := strings.TrimSpace(*in.JobTitle)
		if len([]rune(t)) > 120 {
			return fmt.Errorf("%w: job title is too long", ErrInvalid)
		}
		row.JobTitle = t
	}
	if in.Locale != nil {
		row.Locale = text(*in.Locale)
	}
	if in.Timezone != nil {
		if tz := strings.TrimSpace(*in.Timezone); tz != "" {
			if _, err := time.LoadLocation(tz); err != nil {
				return fmt.Errorf("%w: timezone is invalid", ErrInvalid)
			}
		}
		row.Timezone = text(*in.Timezone)
	}
	if in.Notes != nil {
		row.Notes = strings.TrimSpace(*in.Notes)
	}
	if in.Blocked != nil {
		row.Blocked = *in.Blocked
	}
	if in.ExternalID != nil {
		ext, err := cleanExternalID(*in.ExternalID)
		if err != nil {
			return err
		}
		row.ExternalID = text(ext)
	}
	if in.CustomFields != nil {
		raw, err := json.Marshal(in.CustomFields)
		if err != nil {
			return fmt.Errorf("%w: custom_fields", ErrInvalid)
		}
		row.CustomFields = raw
	}
	if in.Company != nil {
		if *in.Company == nil {
			row.CompanyID = pgtype.Int8{}
		} else {
			co, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: **in.Company})
			if err != nil {
				return fmt.Errorf("%w: company not found", ErrInvalid)
			}
			row.CompanyID = pgtype.Int8{Int64: co.ID, Valid: true}
		}
	}
	return nil
}

// DeleteContact soft deletes a contact (its conversations stay).
func (s *Service) DeleteContact(ctx context.Context, orgID int64, id uuid.UUID) error {
	row, err := s.q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return notFound(err)
	}
	_, err = s.q.SoftDeleteContact(ctx, db.SoftDeleteContactParams{OrganizationID: orgID, ID: row.ID})
	return notFound(err)
}

// MergeResult is what a merge moved.
type MergeResult struct {
	Contact       Contact `json:"contact"`
	Identities    int64   `json:"identities"`
	Conversations int64   `json:"conversations"`
	Messages      int64   `json:"messages"`
}

// Merge folds `from` into `into`: identities, conversations and messages
// move, empty fields are filled, `from` is soft deleted and forwards future
// messages of its identities to `into`.
func (s *Service) Merge(ctx context.Context, orgID int64, into, from uuid.UUID) (MergeResult, error) {
	if into == from {
		return MergeResult{}, fmt.Errorf("%w: a contact cannot be merged into itself", ErrInvalid)
	}
	a, err := s.q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: orgID, Uuid: into})
	if err != nil {
		return MergeResult{}, notFound(err)
	}
	b, err := s.q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: orgID, Uuid: from})
	if err != nil {
		return MergeResult{}, notFound(err)
	}
	var res repository.MergeResult
	err = s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		var err error
		res, err = tx.MergeContacts(ctx, orgID, b.ID, a.ID)
		return err
	})
	if errors.Is(err, repository.ErrInvalidMerge) {
		return MergeResult{}, fmt.Errorf("%w: these contacts cannot be merged", ErrInvalid)
	}
	if err != nil {
		return MergeResult{}, err
	}
	c, err := s.GetContact(ctx, orgID, into)
	if err != nil {
		return MergeResult{}, err
	}
	return MergeResult{Contact: c, Identities: res.Identities, Conversations: res.Conversations, Messages: res.Messages}, nil
}

// ---------------------------------------------------------------------------
// Companies

// ListCompanies returns one page of companies.
func (s *Service) ListCompanies(ctx context.Context, orgID int64, q string, limit, offset int32) ([]Company, int64, error) {
	return s.ListCompaniesByExternalID(ctx, orgID, q, "", limit, offset)
}

// ListCompaniesByExternalID is ListCompanies that also matches the
// company's external id exactly when ext is set (CF-195).
func (s *Service) ListCompaniesByExternalID(ctx context.Context, orgID int64, q, ext string, limit, offset int32) ([]Company, int64, error) {
	var qt pgtype.Text
	if q = strings.TrimSpace(q); q != "" {
		qt = pgtype.Text{String: q, Valid: true}
	}
	et := text(ext)
	rows, err := s.q.ListCompanies(ctx, db.ListCompaniesParams{OrganizationID: orgID, Q: qt, ExternalID: et, RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountCompanies(ctx, db.CountCompaniesParams{OrganizationID: orgID, Q: qt, ExternalID: et})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Company, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
		c := companyOf(db.Company{
			ID: r.ID, Uuid: r.Uuid, Name: r.Name, Domain: r.Domain, ExternalID: r.ExternalID, VerifiedAt: r.VerifiedAt, Notes: r.Notes,
			CustomFields: r.CustomFields, Tier: r.Tier, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
		if r.OwnerUuid.Valid {
			c.AccountOwner = &Ref{UUID: uuid.UUID(r.OwnerUuid.Bytes), Name: strings.TrimSpace(r.OwnerName.String + " " + r.OwnerSurname.String)}
		}
		c.ContactCount, c.OpenConversations = r.ContactCount, r.OpenConversations
		out = append(out, c)
	}
	if err := s.attachRevenue(ctx, orgID, ids, out); err != nil {
		return nil, 0, err
	}
	if err := s.attachHealth(ctx, orgID, ids, out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// GetCompany returns one company.
func (s *Service) GetCompany(ctx context.Context, orgID int64, id uuid.UUID) (Company, error) {
	row, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return Company{}, notFound(err)
	}
	return s.companyDetail(ctx, row)
}

func (s *Service) companyDetail(ctx context.Context, row db.Company) (Company, error) {
	c := companyOf(row)
	if row.AccountOwnerID.Valid {
		if u, err := s.q.GetUserRefByID(ctx, row.AccountOwnerID.Int64); err == nil {
			c.AccountOwner = &Ref{UUID: u.Uuid, Name: strings.TrimSpace(u.Name + " " + u.Surname)}
		}
	}
	one := []Company{c}
	if err := s.attachRevenue(ctx, row.OrganizationID, []int64{row.ID}, one); err != nil {
		return Company{}, err
	}
	if err := s.attachHealth(ctx, row.OrganizationID, []int64{row.ID}, one); err != nil {
		return Company{}, err
	}
	return one[0], nil
}

// CompanyInput creates or edits a company; nil fields stay on update.
type CompanyInput struct {
	Name         *string
	Domain       *string
	ExternalID   *string
	Tier         *string
	AccountOwner **uuid.UUID
	Notes        *string
	CustomFields map[string]any
}

// CreateCompany creates a company and adopts contacts on its domain.
func (s *Service) CreateCompany(ctx context.Context, orgID int64, in CompanyInput) (Company, error) {
	row := db.Company{OrganizationID: orgID, CustomFields: []byte("{}")}
	if err := s.applyCompany(ctx, orgID, &row, in); err != nil {
		return Company{}, err
	}
	if row.Name == "" {
		return Company{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	created, err := s.q.InsertCompany(ctx, db.InsertCompanyParams{
		OrganizationID: orgID, Name: row.Name, Domain: row.Domain, ExternalID: row.ExternalID, Notes: row.Notes,
		CustomFields: row.CustomFields, Tier: row.Tier, AccountOwnerID: row.AccountOwnerID,
	})
	if err != nil {
		return Company{}, uniqueDomain(err)
	}
	if created.Domain.Valid {
		if _, err := s.q.LinkContactsToCompanyByDomain(ctx, db.LinkContactsToCompanyByDomainParams{
			CompanyID: pgtype.Int8{Int64: created.ID, Valid: true}, OrganizationID: orgID, Domain: created.Domain.String,
		}); err != nil {
			return Company{}, err
		}
	}
	return s.GetCompany(ctx, orgID, created.Uuid)
}

// UpdateCompany edits a company.
func (s *Service) UpdateCompany(ctx context.Context, orgID int64, id uuid.UUID, in CompanyInput) (Company, error) {
	row, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return Company{}, notFound(err)
	}
	if err := s.applyCompany(ctx, orgID, &row, in); err != nil {
		return Company{}, err
	}
	if row.Name == "" {
		return Company{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	updated, err := s.q.UpdateCompany(ctx, db.UpdateCompanyParams{
		Name: row.Name, Domain: row.Domain, ExternalID: row.ExternalID, Notes: row.Notes, CustomFields: row.CustomFields,
		Tier: row.Tier, AccountOwnerID: row.AccountOwnerID, SlaPolicyID: row.SlaPolicyID, OrganizationID: orgID, ID: row.ID,
	})
	if err != nil {
		return Company{}, uniqueDomain(err)
	}
	return s.companyDetail(ctx, updated)
}

func (s *Service) applyCompany(ctx context.Context, orgID int64, row *db.Company, in CompanyInput) error {
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if len([]rune(n)) > 255 {
			return fmt.Errorf("%w: name is too long", ErrInvalid)
		}
		row.Name = n
	}
	if in.Domain != nil {
		d := strings.ToLower(strings.TrimSpace(*in.Domain))
		d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
		d = strings.TrimPrefix(strings.TrimSuffix(d, "/"), "www.")
		if d != "" && (!strings.Contains(d, ".") || strings.ContainsAny(d, " /@")) {
			return fmt.Errorf("%w: domain is invalid", ErrInvalid)
		}
		row.Domain = text(d)
	}
	if in.ExternalID != nil {
		ext, err := cleanExternalID(*in.ExternalID)
		if err != nil {
			return err
		}
		row.ExternalID = text(ext)
	}
	if in.Tier != nil {
		t := strings.TrimSpace(*in.Tier)
		if len([]rune(t)) > 64 {
			return fmt.Errorf("%w: tier is too long", ErrInvalid)
		}
		row.Tier = t
	}
	if in.Notes != nil {
		row.Notes = strings.TrimSpace(*in.Notes)
	}
	if in.CustomFields != nil {
		raw, err := json.Marshal(in.CustomFields)
		if err != nil {
			return fmt.Errorf("%w: custom_fields", ErrInvalid)
		}
		row.CustomFields = raw
	}
	if in.AccountOwner != nil {
		if *in.AccountOwner == nil {
			row.AccountOwnerID = pgtype.Int8{}
		} else {
			a, err := s.q.GetOrganizationAgentByUUID(ctx, db.GetOrganizationAgentByUUIDParams{OrganizationID: orgID, Uuid: **in.AccountOwner})
			if err != nil {
				return fmt.Errorf("%w: account owner is not a member", ErrInvalid)
			}
			row.AccountOwnerID = pgtype.Int8{Int64: a.ID, Valid: true}
		}
	}
	return nil
}

// DeleteCompany soft deletes a company; its contacts stay without it.
func (s *Service) DeleteCompany(ctx context.Context, orgID int64, id uuid.UUID) error {
	row, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return notFound(err)
	}
	if _, err := s.q.SoftDeleteCompany(ctx, db.SoftDeleteCompanyParams{OrganizationID: orgID, ID: row.ID}); err != nil {
		return notFound(err)
	}
	return s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		_, err := tx.Raw.Exec(ctx, "UPDATE contacts SET company_id = NULL WHERE organization_id = $1 AND company_id = $2", orgID, row.ID)
		return err
	})
}

// ---------------------------------------------------------------------------

func contactOf(r db.Contact) Contact {
	c := Contact{
		UUID: r.Uuid, Name: r.Name, Email: textPtr(r.Email), Phone: textPtr(r.Phone), JobTitle: r.JobTitle,
		ExternalID: textPtr(r.ExternalID), VerifiedAt: timePtr(r.VerifiedAt),
		Locale: textPtr(r.Locale), Timezone: textPtr(r.Timezone), Notes: r.Notes, CustomFields: decode(r.CustomFields),
		Blocked: r.Blocked, ChannelTypes: []string{}, LastSeenAt: timePtr(r.LastSeenAt),
		CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
	c.IdentityState = model.IdentityState(c.VerifiedAt, c.Name, c.Email, c.Phone)
	return c
}

// cleanExternalID trims an external id; "" clears it.
func cleanExternalID(v string) (string, error) {
	v = strings.TrimSpace(v)
	if len(v) > MaxExternalIDLen {
		return "", fmt.Errorf("%w: external_id must be at most %d characters", ErrInvalid, MaxExternalIDLen)
	}
	if strings.ContainsAny(v, "\x00\r\n") {
		return "", fmt.Errorf("%w: external_id is invalid", ErrInvalid)
	}
	return v, nil
}

func companyOf(r db.Company) Company {
	return Company{
		UUID: r.Uuid, Name: r.Name, Domain: textPtr(r.Domain), ExternalID: textPtr(r.ExternalID), VerifiedAt: timePtr(r.VerifiedAt), Tier: r.Tier,
		Notes: r.Notes, CustomFields: decode(r.CustomFields), CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func uniqueDomain(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if pgErr.ConstraintName == "uq_companies_org_external_id" {
			return ErrExternalIDExists
		}
		return ErrDomainExists
	}
	return err
}

func uniqueContact(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_contacts_org_external_id" {
		return ErrExternalIDExists
	}
	return err
}

func text(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func decode(raw []byte) any {
	var v any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil || v == nil {
		return map[string]any{}
	}
	return v
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// ValidateContactInput checks a contact input without writing (CF-84 import
// preview).
func (s *Service) ValidateContactInput(ctx context.Context, orgID int64, in ContactInput) error {
	row := db.Contact{OrganizationID: orgID, CustomFields: []byte("{}")}
	return s.apply(ctx, orgID, &row, in)
}

// ValidateCompanyInput checks a company input without writing (CF-84 import
// preview).
func (s *Service) ValidateCompanyInput(ctx context.Context, orgID int64, in CompanyInput) error {
	row := db.Company{OrganizationID: orgID, CustomFields: []byte("{}")}
	return s.applyCompany(ctx, orgID, &row, in)
}
