package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Revenue sources (CF-86).
const (
	RevenueSourceManual = "manual"
	RevenueSourceAPI    = "api"
	RevenueSourceStripe = "stripe"
	RevenueSourceIyzico = "iyzico"
)

// Billing statuses of a customer.
var billingStatuses = []string{"none", "trialing", "active", "past_due", "canceled"}

var revenueSources = []string{RevenueSourceManual, RevenueSourceAPI, RevenueSourceStripe, RevenueSourceIyzico}

// Revenue is what a company (the organization's customer) pays the
// organization. Amounts are minor units of Currency.
type Revenue struct {
	MRRMinor           int64      `json:"mrr_minor"`
	Currency           string     `json:"currency"`
	Plan               string     `json:"plan"`
	BillingStatus      string     `json:"billing_status"`
	RenewalAt          *time.Time `json:"renewal_at"`
	LifetimeValueMinor int64      `json:"lifetime_value_minor"`
	Source             string     `json:"source"`
	ExternalRef        string     `json:"external_ref"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// RevenueChange is one entry of a company's revenue history.
type RevenueChange struct {
	MRRMinor           int64      `json:"mrr_minor"`
	Currency           string     `json:"currency"`
	Plan               string     `json:"plan"`
	BillingStatus      string     `json:"billing_status"`
	RenewalAt          *time.Time `json:"renewal_at"`
	LifetimeValueMinor int64      `json:"lifetime_value_minor"`
	Source             string     `json:"source"`
	ChangedBy          *Ref       `json:"changed_by"`
	CreatedAt          time.Time  `json:"created_at"`
}

// RevenueInput sets revenue fields; nil fields keep the stored value (or
// the default on the first write).
type RevenueInput struct {
	MRRMinor           *int64
	Currency           *string
	Plan               *string
	BillingStatus      *string
	RenewalAt          **time.Time
	LifetimeValueMinor *int64
	ExternalRef        *string
	Source             string
	ActorID            *int64
}

// CompanyKey names a company in an integration upsert: by uuid, by the
// external id of the organization's own system, or by domain (first set wins).
type CompanyKey struct {
	UUID       *uuid.UUID
	ExternalID string
	Domain     string
}

// GetRevenue returns a company's revenue (nil when never set) and its
// latest history entries.
func (s *Service) GetRevenue(ctx context.Context, orgID int64, companyUUID uuid.UUID, historyLimit int32) (*Revenue, []RevenueChange, error) {
	co, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: companyUUID})
	if err != nil {
		return nil, nil, notFound(err)
	}
	var rev *Revenue
	row, err := s.q.GetCompanyRevenue(ctx, db.GetCompanyRevenueParams{OrganizationID: orgID, CompanyID: co.ID})
	switch {
	case err == nil:
		r := revenueOf(row)
		rev = &r
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, nil, err
	}
	hist, err := s.q.ListCompanyRevenueHistory(ctx, db.ListCompanyRevenueHistoryParams{OrganizationID: orgID, CompanyID: co.ID, RowLimit: historyLimit})
	if err != nil {
		return nil, nil, err
	}
	out := make([]RevenueChange, 0, len(hist))
	for _, h := range hist {
		ch := RevenueChange{
			MRRMinor: h.MrrMinor, Currency: h.Currency, Plan: h.Plan, BillingStatus: h.BillingStatus,
			RenewalAt: timePtr(h.RenewalAt), LifetimeValueMinor: h.LifetimeValueMinor, Source: h.Source,
			CreatedAt: h.CreatedAt.Time.UTC(),
		}
		if h.ChangedByUuid.Valid {
			ch.ChangedBy = &Ref{UUID: uuid.UUID(h.ChangedByUuid.Bytes), Name: strings.TrimSpace(h.ChangedByName.String + " " + h.ChangedBySurname.String)}
		}
		out = append(out, ch)
	}
	return rev, out, nil
}

// SetRevenue writes a company's revenue and appends a history entry when a
// tracked value changed. It returns the company uuid and the stored revenue.
func (s *Service) SetRevenue(ctx context.Context, orgID int64, key CompanyKey, in RevenueInput) (uuid.UUID, Revenue, error) {
	co, err := s.companyByKey(ctx, orgID, key)
	if err != nil {
		return uuid.Nil, Revenue{}, err
	}
	var out Revenue
	err = s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		// Lock the company row so concurrent writes serialize their history.
		if _, err := tx.Raw.Exec(ctx, "SELECT 1 FROM companies WHERE organization_id = $1 AND id = $2 FOR UPDATE", orgID, co.ID); err != nil {
			return err
		}
		cur := db.CompanyRevenue{Currency: "TRY", BillingStatus: "none"}
		existed := true
		prev, err := tx.Q.GetCompanyRevenue(ctx, db.GetCompanyRevenueParams{OrganizationID: orgID, CompanyID: co.ID})
		switch {
		case err == nil:
			cur = prev
		case errors.Is(err, pgx.ErrNoRows):
			existed = false
		default:
			return err
		}
		next, err := applyRevenue(cur, in)
		if err != nil {
			return err
		}
		var actor pgtype.Int8
		if in.ActorID != nil {
			actor = pgtype.Int8{Int64: *in.ActorID, Valid: true}
		}
		row, err := tx.Q.UpsertCompanyRevenue(ctx, db.UpsertCompanyRevenueParams{
			OrganizationID: orgID, CompanyID: co.ID, MrrMinor: next.MrrMinor, Currency: next.Currency,
			Plan: next.Plan, BillingStatus: next.BillingStatus, RenewalAt: next.RenewalAt,
			LifetimeValueMinor: next.LifetimeValueMinor, Source: in.Source, ExternalRef: next.ExternalRef,
			UpdatedByUserID: actor,
		})
		if err != nil {
			return err
		}
		if !existed || revenueChanged(prev, row) {
			if err := tx.Q.InsertCompanyRevenueHistory(ctx, db.InsertCompanyRevenueHistoryParams{
				OrganizationID: orgID, CompanyID: co.ID, MrrMinor: row.MrrMinor, Currency: row.Currency,
				Plan: row.Plan, BillingStatus: row.BillingStatus, RenewalAt: row.RenewalAt,
				LifetimeValueMinor: row.LifetimeValueMinor, Source: row.Source, ChangedByUserID: actor,
			}); err != nil {
				return err
			}
		}
		out = revenueOf(row)
		return nil
	})
	return co.Uuid, out, err
}

// ClearRevenue removes a company's current revenue (history stays).
func (s *Service) ClearRevenue(ctx context.Context, orgID int64, companyUUID uuid.UUID) error {
	co, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: companyUUID})
	if err != nil {
		return notFound(err)
	}
	n, err := s.q.DeleteCompanyRevenue(ctx, db.DeleteCompanyRevenueParams{OrganizationID: orgID, CompanyID: co.ID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) companyByKey(ctx context.Context, orgID int64, key CompanyKey) (db.Company, error) {
	var (
		co  db.Company
		err error
	)
	switch {
	case key.UUID != nil:
		co, err = s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: *key.UUID})
	case strings.TrimSpace(key.ExternalID) != "":
		co, err = s.q.GetCompanyByExternalID(ctx, db.GetCompanyByExternalIDParams{OrganizationID: orgID, ExternalID: strings.TrimSpace(key.ExternalID)})
	case strings.TrimSpace(key.Domain) != "":
		co, err = s.q.GetCompanyByDomain(ctx, db.GetCompanyByDomainParams{OrganizationID: orgID, Domain: normalizeDomain(key.Domain)})
	default:
		return db.Company{}, fmt.Errorf("%w: company_uuid, external_id or domain is required", ErrInvalid)
	}
	return co, notFound(err)
}

// applyRevenue validates in on top of cur.
func applyRevenue(cur db.CompanyRevenue, in RevenueInput) (db.CompanyRevenue, error) {
	if !contains(revenueSources, in.Source) {
		return cur, fmt.Errorf("%w: unknown source", ErrInvalid)
	}
	if in.MRRMinor != nil {
		if *in.MRRMinor < 0 {
			return cur, fmt.Errorf("%w: mrr_minor must not be negative", ErrInvalid)
		}
		cur.MrrMinor = *in.MRRMinor
	}
	if in.LifetimeValueMinor != nil {
		if *in.LifetimeValueMinor < 0 {
			return cur, fmt.Errorf("%w: lifetime_value_minor must not be negative", ErrInvalid)
		}
		cur.LifetimeValueMinor = *in.LifetimeValueMinor
	}
	if in.Currency != nil {
		c := strings.ToUpper(strings.TrimSpace(*in.Currency))
		if len(c) != 3 || strings.Trim(c, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
			return cur, fmt.Errorf("%w: currency must be an ISO 4217 code", ErrInvalid)
		}
		cur.Currency = c
	}
	if in.Plan != nil {
		p := strings.TrimSpace(*in.Plan)
		if len([]rune(p)) > 120 {
			return cur, fmt.Errorf("%w: plan is too long", ErrInvalid)
		}
		cur.Plan = p
	}
	if in.BillingStatus != nil {
		st := strings.TrimSpace(*in.BillingStatus)
		if !contains(billingStatuses, st) {
			return cur, fmt.Errorf("%w: unknown billing_status", ErrInvalid)
		}
		cur.BillingStatus = st
	}
	if in.RenewalAt != nil {
		if *in.RenewalAt == nil {
			cur.RenewalAt = pgtype.Timestamptz{}
		} else {
			cur.RenewalAt = pgtype.Timestamptz{Time: (**in.RenewalAt).UTC(), Valid: true}
		}
	}
	if in.ExternalRef != nil {
		ref := strings.TrimSpace(*in.ExternalRef)
		if len(ref) > 200 {
			return cur, fmt.Errorf("%w: external_ref is too long", ErrInvalid)
		}
		cur.ExternalRef = ref
	}
	return cur, nil
}

func revenueChanged(a, b db.CompanyRevenue) bool {
	return a.MrrMinor != b.MrrMinor || a.Currency != b.Currency || a.Plan != b.Plan ||
		a.BillingStatus != b.BillingStatus || a.LifetimeValueMinor != b.LifetimeValueMinor ||
		a.RenewalAt.Valid != b.RenewalAt.Valid || (a.RenewalAt.Valid && !a.RenewalAt.Time.Equal(b.RenewalAt.Time))
}

func revenueOf(r db.CompanyRevenue) Revenue {
	return Revenue{
		MRRMinor: r.MrrMinor, Currency: r.Currency, Plan: r.Plan, BillingStatus: r.BillingStatus,
		RenewalAt: timePtr(r.RenewalAt), LifetimeValueMinor: r.LifetimeValueMinor, Source: r.Source,
		ExternalRef: r.ExternalRef, UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
}

// attachRevenue fills Revenue on a page of companies with one query.
func (s *Service) attachRevenue(ctx context.Context, orgID int64, ids []int64, out []Company) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.q.ListCompanyRevenueByCompanyIDs(ctx, db.ListCompanyRevenueByCompanyIDsParams{OrganizationID: orgID, CompanyIds: ids})
	if err != nil {
		return err
	}
	byID := make(map[int64]Revenue, len(rows))
	for _, r := range rows {
		byID[r.CompanyID] = revenueOf(r)
	}
	for i, id := range ids {
		if r, ok := byID[id]; ok {
			out[i].Revenue = &r
		}
	}
	return nil
}

func normalizeDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
	if i := strings.IndexAny(d, "/?#"); i >= 0 {
		d = d[:i]
	}
	// Same form applyCompany stores.
	return strings.TrimPrefix(d, "www.")
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
