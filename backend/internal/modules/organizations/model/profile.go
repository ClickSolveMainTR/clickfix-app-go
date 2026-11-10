package model

// Organization profile (CF-3): regional defaults, company identity and
// branding. The regional defaults are organizations columns; the rest lives
// in organizations.settings under the keys below. Working hours are not
// part of the profile: the SLA module's business_hours schedule is the one
// source (CF-198).

// Supported organization locales.
var Locales = []string{"tr", "en"}

// Company is the legal / billing identity of an organization.
type Company struct {
	LegalName string `json:"legal_name"`
	TaxOffice string `json:"tax_office"`
	TaxNumber string `json:"tax_number"`
	Address   string `json:"address"`
	City      string `json:"city"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
	Website   string `json:"website"`
}

// Branding is the organization's look in the widget, e-mails and portal.
type Branding struct {
	PrimaryColor string `json:"primary_color"`
}

// ProfileSettings is the part of organizations.settings the profile owns.
type ProfileSettings struct {
	Company  Company  `json:"company"`
	Branding Branding `json:"branding"`
}

// OrganizationProfile is GET/PATCH /v1/tenant/organization.
type OrganizationProfile struct {
	UUID           string   `json:"uuid"`
	Slug           string   `json:"slug"`
	Name           string   `json:"name"`
	LogoURL        *string  `json:"logo_url"`
	Status         string   `json:"status"`
	AccessStartsAt string   `json:"access_starts_at"`
	AccessEndsAt   *string  `json:"access_ends_at"`
	UpdatedAt      string   `json:"updated_at"`
	Timezone       string   `json:"timezone"`
	Locale         string   `json:"locale"`
	Currency       string   `json:"currency"`
	CountryCode    string   `json:"country_code"`
	Company        Company  `json:"company"`
	Branding       Branding `json:"branding"`
}

// PatchProfileInput is the body of PATCH /v1/tenant/organization. Nil
// fields keep their value; objects are replaced whole.
type PatchProfileInput struct {
	Name        *string   `json:"name"`
	Timezone    *string   `json:"timezone"`
	Locale      *string   `json:"locale"`
	Currency    *string   `json:"currency"`
	CountryCode *string   `json:"country_code"`
	Company     *Company  `json:"company"`
	Branding    *Branding `json:"branding"`
}
