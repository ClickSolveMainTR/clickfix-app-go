package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	"github.com/jackc/pgx/v5/pgtype"
)

// Organization profile (/v1/tenant/organization, CF-3).

var (
	hhmmRE     = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	currencyRE = regexp.MustCompile(`^[A-Z]{3}$`)
	countryRE  = regexp.MustCompile(`^[A-Z]{2}$`)
	colorRE    = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	taxNoRE    = regexp.MustCompile(`^[0-9]{10,11}$`)
)

// GetProfile returns the profile of an organization.
func (m *Management) GetProfile(ctx context.Context, orgID int64) (orgmodel.OrganizationProfile, error) {
	row, err := m.orgs.repo.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return orgmodel.OrganizationProfile{}, mapRepoNotFound(err, ErrNotFound)
	}
	return ProfileFromRow(row), nil
}

// PatchProfile validates and saves a profile change. Validation problems
// come back as *ValidationError with dotted field paths.
func (m *Management) PatchProfile(ctx context.Context, orgID int64, in orgmodel.PatchProfileInput) (orgmodel.OrganizationProfile, []string, error) {
	row, err := m.orgs.repo.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return orgmodel.OrganizationProfile{}, nil, mapRepoNotFound(err, ErrNotFound)
	}
	ve := &ValidationError{}
	params := db.UpdateOrganizationProfileParams{ID: orgID}
	var changed []string
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		if v == "" || utf8.RuneCountInString(v) > 255 {
			ve.add("name", "required", "name is required (max 255 characters)")
		}
		params.Name = pgtype.Text{String: v, Valid: true}
		changed = append(changed, "name")
	}
	if in.Timezone != nil {
		v := strings.TrimSpace(*in.Timezone)
		if _, err := time.LoadLocation(v); v == "" || err != nil {
			ve.add("timezone", "invalid", "timezone must be an IANA time zone")
		}
		params.Timezone = pgtype.Text{String: v, Valid: true}
		changed = append(changed, "timezone")
	}
	if in.Locale != nil {
		v := strings.TrimSpace(*in.Locale)
		if !slices.Contains(orgmodel.Locales, v) {
			ve.add("locale", "invalid", "locale must be tr or en")
		}
		params.Locale = pgtype.Text{String: v, Valid: true}
		changed = append(changed, "locale")
	}
	if in.Currency != nil {
		v := strings.ToUpper(strings.TrimSpace(*in.Currency))
		if !currencyRE.MatchString(v) {
			ve.add("currency", "invalid", "currency must be an ISO 4217 code")
		}
		params.Currency = pgtype.Text{String: v, Valid: true}
		changed = append(changed, "currency")
	}
	if in.CountryCode != nil {
		v := strings.ToUpper(strings.TrimSpace(*in.CountryCode))
		if !countryRE.MatchString(v) {
			ve.add("country_code", "invalid", "country_code must be an ISO 3166-1 alpha-2 code")
		}
		params.CountryCode = pgtype.Text{String: v, Valid: true}
		changed = append(changed, "country_code")
	}
	if in.Company != nil || in.Branding != nil {
		doc := map[string]json.RawMessage{}
		if len(row.Settings) > 0 {
			_ = json.Unmarshal(row.Settings, &doc)
		}
		if in.Company != nil {
			c := normalizeCompany(*in.Company)
			validateCompany(ve, c)
			doc["company"], _ = json.Marshal(c)
			changed = append(changed, "company")
		}
		if in.Branding != nil {
			b := orgmodel.Branding{PrimaryColor: strings.TrimSpace(in.Branding.PrimaryColor)}
			if b.PrimaryColor != "" && !colorRE.MatchString(b.PrimaryColor) {
				ve.add("branding.primary_color", "invalid", "primary_color must be #RRGGBB")
			}
			doc["branding"], _ = json.Marshal(b)
			changed = append(changed, "branding")
		}
		raw, err := json.Marshal(doc)
		if err != nil {
			return orgmodel.OrganizationProfile{}, nil, err
		}
		params.Settings = raw
	}
	if err := ve.orNil(); err != nil {
		return orgmodel.OrganizationProfile{}, nil, err
	}
	if len(changed) == 0 {
		return ProfileFromRow(row), nil, nil
	}
	updated, err := m.repo.UpdateOrganizationProfile(ctx, params)
	if err != nil {
		return orgmodel.OrganizationProfile{}, nil, mapRepoNotFound(err, ErrNotFound)
	}
	return ProfileFromRow(updated), changed, nil
}

// ProfileFromRow maps an organizations row to the profile, filling defaults
// for settings that were never saved.
func ProfileFromRow(row db.Organization) orgmodel.OrganizationProfile {
	settings := orgmodel.ProfileSettings{}
	if len(row.Settings) > 0 {
		var parsed struct {
			Company  *orgmodel.Company  `json:"company"`
			Branding *orgmodel.Branding `json:"branding"`
		}
		if json.Unmarshal(row.Settings, &parsed) == nil {
			if parsed.Company != nil {
				settings.Company = *parsed.Company
			}
			if parsed.Branding != nil {
				settings.Branding = *parsed.Branding
			}
		}
	}
	const layout = "2006-01-02T15:04:05Z07:00"
	out := orgmodel.OrganizationProfile{
		UUID: row.Uuid.String(), Slug: row.Slug, Name: row.Name,
		LogoURL: LogoURL(row.Uuid, row.LogoObjectKey), Status: row.Status,
		AccessStartsAt: row.AccessStartsAt.Time.UTC().Format(layout),
		UpdatedAt:      row.UpdatedAt.Time.UTC().Format(layout),
		Timezone:       row.Timezone, Locale: row.Locale, Currency: row.Currency, CountryCode: row.CountryCode,
		Company: settings.Company, Branding: settings.Branding,
	}
	if row.AccessEndsAt.Valid {
		v := row.AccessEndsAt.Time.UTC().Format(layout)
		out.AccessEndsAt = &v
	}
	return out
}

func normalizeCompany(c orgmodel.Company) orgmodel.Company {
	return orgmodel.Company{
		LegalName: strings.TrimSpace(c.LegalName), TaxOffice: strings.TrimSpace(c.TaxOffice),
		TaxNumber: strings.TrimSpace(c.TaxNumber), Address: strings.TrimSpace(c.Address),
		City: strings.TrimSpace(c.City), Phone: strings.TrimSpace(c.Phone),
		Email: strings.ToLower(strings.TrimSpace(c.Email)), Website: strings.TrimSpace(c.Website),
	}
}

func validateCompany(ve *ValidationError, c orgmodel.Company) {
	limits := []struct {
		field, value string
		max          int
	}{
		{"legal_name", c.LegalName, 255}, {"tax_office", c.TaxOffice, 120}, {"address", c.Address, 500},
		{"city", c.City, 120}, {"phone", c.Phone, 40}, {"email", c.Email, 255}, {"website", c.Website, 255},
	}
	for _, l := range limits {
		if utf8.RuneCountInString(l.value) > l.max {
			ve.add("company."+l.field, "too_long", fmt.Sprintf("%s is longer than %d characters", l.field, l.max))
		}
	}
	// Turkish VKN (10 digits) or TCKN (11 digits).
	if c.TaxNumber != "" && !taxNoRE.MatchString(c.TaxNumber) {
		ve.add("company.tax_number", "invalid", "tax_number must be 10 or 11 digits")
	}
	if c.Email != "" {
		if a, err := mail.ParseAddress(c.Email); err != nil || a.Address != c.Email {
			ve.add("company.email", "invalid", "email is not a valid address")
		}
	}
	if c.Website != "" {
		u, err := url.Parse(c.Website)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			ve.add("company.website", "invalid", "website must be an http(s) URL")
		}
	}
}
