// Package marketing holds the public marketing site settings that the
// platform admin manages without a redeploy (CF-144): tag / pixel ids,
// verification tokens, SEO defaults, sanitized custom snippets and the default
// share image. It also records where signed-up users and organizations came
// from (UTM, click ids, referrer) for the "Kaynaklar" report.
package marketing

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// Config is the stored tracking configuration. Every id is optional; an
// empty one means the tag is not loaded.
type Config struct {
	GTMID                string `json:"gtm_id"`
	GA4ID                string `json:"ga4_id"`
	MetaPixelID          string `json:"meta_pixel_id"`
	LinkedInPartnerID    string `json:"linkedin_partner_id"`
	TikTokPixelID        string `json:"tiktok_pixel_id"`
	GoogleAdsID          string `json:"google_ads_id"`
	GoogleAdsSignupLabel string `json:"google_ads_signup_label"`
	ClarityID            string `json:"clarity_id"`
	HotjarID             string `json:"hotjar_id"`
	CustomHead           string `json:"custom_head"`
	CustomBody           string `json:"custom_body"`
	SiteTitle            string `json:"site_title"`
	SiteDescription      string `json:"site_description"`
	GoogleVerification   string `json:"google_verification"`
	BingVerification     string `json:"bing_verification"`
	YandexVerification   string `json:"yandex_verification"`
}

// Limits of the free-text fields.
const (
	MaxSnippetBytes     = 20 << 10
	MaxTitleRunes       = 70
	MaxDescriptionRunes = 200
)

var (
	reGTM          = regexp.MustCompile(`^GTM-[A-Z0-9]{4,12}$`)
	reGA4          = regexp.MustCompile(`^G-[A-Z0-9]{4,16}$`)
	reDigits       = regexp.MustCompile(`^[0-9]{3,20}$`)
	reTikTok       = regexp.MustCompile(`^[A-Z0-9]{10,32}$`)
	reGoogleAds    = regexp.MustCompile(`^AW-[0-9]{6,15}$`)
	reAdsLabel     = regexp.MustCompile(`^[A-Za-z0-9_-]{4,64}$`)
	reClarity      = regexp.MustCompile(`^[a-z0-9]{6,20}$`)
	reVerification = regexp.MustCompile(`^[A-Za-z0-9_=-]{4,128}$`)
)

// Normalize trims every field, upper-cases the ids that are always upper case
// and sanitizes the custom snippets.
func (c Config) Normalize() Config {
	trim := strings.TrimSpace
	c.GTMID = strings.ToUpper(trim(c.GTMID))
	c.GA4ID = strings.ToUpper(trim(c.GA4ID))
	c.MetaPixelID = trim(c.MetaPixelID)
	c.LinkedInPartnerID = trim(c.LinkedInPartnerID)
	c.TikTokPixelID = strings.ToUpper(trim(c.TikTokPixelID))
	c.GoogleAdsID = strings.ToUpper(trim(c.GoogleAdsID))
	c.GoogleAdsSignupLabel = trim(c.GoogleAdsSignupLabel)
	c.ClarityID = strings.ToLower(trim(c.ClarityID))
	c.HotjarID = trim(c.HotjarID)
	c.SiteTitle = trim(c.SiteTitle)
	c.SiteDescription = trim(c.SiteDescription)
	c.GoogleVerification = trim(c.GoogleVerification)
	c.BingVerification = trim(c.BingVerification)
	c.YandexVerification = trim(c.YandexVerification)
	c.CustomHead = SanitizeSnippet(c.CustomHead)
	c.CustomBody = SanitizeSnippet(c.CustomBody)
	return c
}

// Validate checks a normalized config; snippets are measured before
// sanitizing by the caller (Patch).
func (c Config) Validate() []response.Detail {
	var out []response.Detail
	check := func(field, value string, re *regexp.Regexp, hint string) {
		if value != "" && !re.MatchString(value) {
			out = append(out, response.Detail{Field: field, Message: hint})
		}
	}
	check("gtm_id", c.GTMID, reGTM, "expected GTM-XXXXXXX")
	check("ga4_id", c.GA4ID, reGA4, "expected G-XXXXXXXXXX")
	check("meta_pixel_id", c.MetaPixelID, reDigits, "expected a numeric pixel id")
	check("linkedin_partner_id", c.LinkedInPartnerID, reDigits, "expected a numeric partner id")
	check("tiktok_pixel_id", c.TikTokPixelID, reTikTok, "expected the TikTok pixel code")
	check("google_ads_id", c.GoogleAdsID, reGoogleAds, "expected AW-XXXXXXXXX")
	check("google_ads_signup_label", c.GoogleAdsSignupLabel, reAdsLabel, "expected the conversion label")
	check("clarity_id", c.ClarityID, reClarity, "expected the Clarity project id")
	check("hotjar_id", c.HotjarID, reDigits, "expected a numeric site id")
	check("google_verification", c.GoogleVerification, reVerification, "expected the verification token only")
	check("bing_verification", c.BingVerification, reVerification, "expected the verification token only")
	check("yandex_verification", c.YandexVerification, reVerification, "expected the verification token only")
	if c.GoogleAdsSignupLabel != "" && c.GoogleAdsID == "" {
		out = append(out, response.Detail{Field: "google_ads_id", Message: "required with a conversion label"})
	}
	if utf8.RuneCountInString(c.SiteTitle) > MaxTitleRunes {
		out = append(out, response.Detail{Field: "site_title", Message: "at most 70 characters"})
	}
	if utf8.RuneCountInString(c.SiteDescription) > MaxDescriptionRunes {
		out = append(out, response.Detail{Field: "site_description", Message: "at most 200 characters"})
	}
	if len(c.CustomHead) > MaxSnippetBytes {
		out = append(out, response.Detail{Field: "custom_head", Message: "at most 20 KB"})
	}
	if len(c.CustomBody) > MaxSnippetBytes {
		out = append(out, response.Detail{Field: "custom_body", Message: "at most 20 KB"})
	}
	return out
}
