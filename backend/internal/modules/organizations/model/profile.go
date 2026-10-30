package model

// Organization profile (CF-3): regional defaults, company identity, business
// hours and branding. The regional defaults are organizations columns; the
// rest lives in organizations.settings under the keys below.

// Weekdays of the business hours, Monday first.
var Weekdays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

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

// BusinessDay is the opening time of one weekday ("HH:MM", end after start).
type BusinessDay struct {
	Day   string `json:"day"`
	Open  bool   `json:"open"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// BusinessHours are the default working hours of the organization (teams and
// SLA policies may override them later).
type BusinessHours struct {
	Enabled bool          `json:"enabled"`
	Days    []BusinessDay `json:"days"`
}

// Branding is the organization's look in the widget, e-mails and portal.
type Branding struct {
	PrimaryColor string `json:"primary_color"`
}

// ProfileSettings is the part of organizations.settings the profile owns.
type ProfileSettings struct {
	Company       Company       `json:"company"`
	BusinessHours BusinessHours `json:"business_hours"`
	Branding      Branding      `json:"branding"`
}

// DefaultBusinessHours is Monday-Friday 09:00-18:00, weekend closed.
func DefaultBusinessHours() BusinessHours {
	days := make([]BusinessDay, 0, len(Weekdays))
	for i, d := range Weekdays {
		days = append(days, BusinessDay{Day: d, Open: i < 5, Start: "09:00", End: "18:00"})
	}
	return BusinessHours{Enabled: false, Days: days}
}

// OrganizationProfile is GET/PATCH /v1/tenant/organization.
type OrganizationProfile struct {
	UUID           string        `json:"uuid"`
	Slug           string        `json:"slug"`
	Name           string        `json:"name"`
	LogoURL        *string       `json:"logo_url"`
	Status         string        `json:"status"`
	AccessStartsAt string        `json:"access_starts_at"`
	AccessEndsAt   *string       `json:"access_ends_at"`
	UpdatedAt      string        `json:"updated_at"`
	Timezone       string        `json:"timezone"`
	Locale         string        `json:"locale"`
	Currency       string        `json:"currency"`
	CountryCode    string        `json:"country_code"`
	Company        Company       `json:"company"`
	BusinessHours  BusinessHours `json:"business_hours"`
	Branding       Branding      `json:"branding"`
}

// PatchProfileInput is the body of PATCH /v1/tenant/organization. Nil
// fields keep their value; objects are replaced whole.
type PatchProfileInput struct {
	Name          *string        `json:"name"`
	Timezone      *string        `json:"timezone"`
	Locale        *string        `json:"locale"`
	Currency      *string        `json:"currency"`
	CountryCode   *string        `json:"country_code"`
	Company       *Company       `json:"company"`
	BusinessHours *BusinessHours `json:"business_hours"`
	Branding      *Branding      `json:"branding"`
}
