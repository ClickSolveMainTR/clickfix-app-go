// Package model defines the versioned billing catalog and lifecycle contracts.
package model

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalid  = errors.New("billing: invalid input")
	ErrConflict = errors.New("billing: conflict")
	ErrNotFound = errors.New("billing: not found")
	// A version draft and the feature catalog disagree: the catalog has a
	// feature the draft lacks, or the draft names one the catalog lacks.
	// Both wrap ErrInvalid.
	ErrMissingFeature = fmt.Errorf("%w: missing feature", ErrInvalid)
	ErrUnknownFeature = fmt.Errorf("%w: unknown/duplicate feature", ErrInvalid)
)

type PlanInput struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
	SortOrder   int32  `json:"sort_order"`
}
type Plan struct {
	UUID uuid.UUID `json:"uuid"`
	PlanInput
	ArchivedAt *time.Time `json:"archived_at"`
	Versions   []Version  `json:"versions"`
}
type Tier struct {
	Feature    string `json:"feature"`
	Lower      int64  `json:"lower_bound"`
	Upper      *int64 `json:"upper_bound"`
	UnitAmount int64  `json:"unit_amount_minor"`
}
type Price struct {
	UUID         uuid.UUID `json:"uuid"`
	Interval     string    `json:"interval"`
	Currency     string    `json:"currency"`
	Amount       int64     `json:"amount_minor"`
	SeatAmount   int64     `json:"seat_amount_minor"`
	TaxInclusive bool      `json:"tax_inclusive"`
	VAT          int32     `json:"vat_basis_points"`
	Tiers        []Tier    `json:"tiers"`
}
type Grant struct {
	Feature     string `json:"feature"`
	Enabled     *bool  `json:"enabled"`
	Limit       *int64 `json:"limit"`
	Unlimited   bool   `json:"unlimited"`
	Enforcement string `json:"enforcement"`
	WarnPercent int32  `json:"warn_percent"`
}
type VersionInput struct {
	TrialDays    int32   `json:"trial_days"`
	Prices       []Price `json:"prices"`
	Entitlements []Grant `json:"entitlements"`
}
type Version struct {
	UUID        uuid.UUID  `json:"uuid"`
	Number      int32      `json:"version"`
	Status      string     `json:"status"`
	PublishedAt *time.Time `json:"published_at"`
	VersionInput
}
type Subscription struct {
	TrialEndsAt        *time.Time `json:"trial_ends_at"`
	TrialDaysRemaining int        `json:"trial_days_remaining"`
	UUID               uuid.UUID  `json:"uuid"`
	VersionUUID        uuid.UUID  `json:"version_uuid"`
	PriceUUID          uuid.UUID  `json:"price_uuid"`
	Status             string     `json:"status"`
	PeriodStart        time.Time  `json:"period_start"`
	PeriodEnd          time.Time  `json:"period_end"`
	GraceEndsAt        *time.Time `json:"grace_ends_at"`
	CanceledAt         *time.Time `json:"canceled_at"`
	CancelAtPeriodEnd  bool       `json:"cancel_at_period_end"`
	PendingVersionUUID *uuid.UUID `json:"pending_version_uuid"`
}
type CreateSubscription struct {
	OrganizationUUID uuid.UUID  `json:"organization_uuid"`
	VersionUUID      uuid.UUID  `json:"version_uuid"`
	PriceUUID        uuid.UUID  `json:"price_uuid"`
	Status           string     `json:"status"`
	PeriodStart      time.Time  `json:"period_start"`
	PeriodEnd        time.Time  `json:"period_end"`
	TrialEndsAt      *time.Time `json:"trial_ends_at"`
}
type Transition struct {
	To          string     `json:"to"`
	Reason      string     `json:"reason"`
	GraceEndsAt *time.Time `json:"grace_ends_at"`
	PeriodStart *time.Time `json:"period_start"`
	PeriodEnd   *time.Time `json:"period_end"`
}
type Override struct {
	Grant
	Reason    string     `json:"reason"`
	ExpiresAt *time.Time `json:"expires_at"`
}
type Migration struct {
	VersionUUID       uuid.UUID   `json:"version_uuid"`
	SubscriptionUUIDs []uuid.UUID `json:"subscription_uuids"`
	OrganizationUUIDs []uuid.UUID `json:"organization_uuids"`
	Reason            string      `json:"reason"`
}

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,79}$`)

func (p *PlanInput) Validate(create bool) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Visibility == "" {
		p.Visibility = "public"
	}
	if p.Name == "" || len(p.Name) > 200 || len(p.Description) > 10000 || (create && !codePattern.MatchString(p.Code)) {
		return fmt.Errorf("%w: name or code", ErrInvalid)
	}
	if p.Visibility != "public" && p.Visibility != "hidden" && p.Visibility != "custom" {
		return fmt.Errorf("%w: visibility", ErrInvalid)
	}
	return nil
}

// ValidateGrant ports comcode's typed limit validation to BIGINT and explicit unlimited values.
func ValidateGrant(g Grant, kind string) error {
	if kind == "boolean" && g.Enabled != nil && g.Limit == nil && !g.Unlimited {
		return nil
	}
	if kind == "limit" && g.Enabled == nil && ((g.Limit != nil) != g.Unlimited) && (g.Limit == nil || *g.Limit >= 0) {
		return nil
	}
	return fmt.Errorf("%w: feature %s type/value mismatch", ErrInvalid, g.Feature)
}
func (v *VersionInput) Validate(features map[string]string) error {
	if v.TrialDays < 0 || v.TrialDays > 365 || len(v.Prices) == 0 || len(v.Prices) > 6 {
		return fmt.Errorf("%w: trial_days or prices", ErrInvalid)
	}
	seen := map[string]bool{}
	for i := range v.Entitlements {
		g := &v.Entitlements[i]
		kind, ok := features[g.Feature]
		if !ok || seen[g.Feature] {
			return fmt.Errorf("%w %s", ErrUnknownFeature, g.Feature)
		}
		seen[g.Feature] = true
		if err := ValidateGrant(*g, kind); err != nil {
			return err
		}
		if g.Enforcement == "" {
			g.Enforcement = "hard"
		}
		if g.WarnPercent == 0 {
			g.WarnPercent = 80
		}
		if (g.Enforcement != "hard" && g.Enforcement != "soft") || g.WarnPercent < 1 || g.WarnPercent > 100 {
			return fmt.Errorf("%w: enforcement or warn_percent", ErrInvalid)
		}
	}
	for k := range features {
		if !seen[k] {
			return fmt.Errorf("%w %s", ErrMissingFeature, k)
		}
	}
	seen = map[string]bool{}
	for _, p := range v.Prices {
		k := p.Interval + ":" + p.Currency
		if seen[k] || (p.Interval != "monthly" && p.Interval != "yearly") || (p.Currency != "TRY" && p.Currency != "USD" && p.Currency != "EUR") || p.Amount < 0 || p.SeatAmount < 0 || p.VAT < 0 || p.VAT > 10000 {
			return fmt.Errorf("%w: price", ErrInvalid)
		}
		seen[k] = true
		// Per feature tiers must cover [0,infinity) contiguously; prevents ambiguous billing.
		ends := map[string]*int64{}
		for _, t := range p.Tiers {
			kind, ok := features[t.Feature]
			prev, exists := ends[t.Feature]
			if !ok || kind != "limit" || t.Lower < 0 || t.UnitAmount < 0 || (t.Upper != nil && *t.Upper <= t.Lower) || (!exists && t.Lower != 0) || (exists && (prev == nil || *prev != t.Lower)) {
				return fmt.Errorf("%w: tier range", ErrInvalid)
			}
			ends[t.Feature] = t.Upper
		}
		for _, end := range ends {
			if end != nil {
				return fmt.Errorf("%w: final tier must be unbounded", ErrInvalid)
			}
		}
	}
	return nil
}

type Feature struct {
	UUID        uuid.UUID `json:"uuid"`
	Key         string    `json:"key"`
	Kind        string    `json:"kind"`
	LabelTR     string    `json:"label_tr"`
	LabelEN     string    `json:"label_en"`
	Unit        string    `json:"unit"`
	Aggregation string    `json:"aggregation"`
	Period      string    `json:"period"`
	// Default grant for plan versions published before the feature existed.
	DefaultEnabled   *bool  `json:"default_enabled"`
	DefaultLimit     *int64 `json:"default_limit"`
	DefaultUnlimited bool   `json:"default_unlimited"`
}
type HistoryEntry struct {
	UUID      uuid.UUID `json:"uuid"`
	From      string    `json:"from_status"`
	To        string    `json:"to_status"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}
