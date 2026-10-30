package marketing

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

// AttributionHeader carries the visitor's first / last touch from the BFF
// (read from the `cf_ft` / `cf_lt` cookies the marketing site sets after
// analytics consent). Value: URL-encoded JSON `{"ft":Touch,"lt":Touch}`.
const AttributionHeader = "X-ClickFix-Attribution"

const maxAttributionHeader = 4 << 10

// Touch is one visit: campaign parameters, ad click ids, the external
// referrer and the first page seen.
type Touch struct {
	Source   string `json:"source,omitempty"`
	Medium   string `json:"medium,omitempty"`
	Campaign string `json:"campaign,omitempty"`
	Term     string `json:"term,omitempty"`
	Content  string `json:"content,omitempty"`
	GCLID    string `json:"gclid,omitempty"`
	FBCLID   string `json:"fbclid,omitempty"`
	MSCLKID  string `json:"msclkid,omitempty"`
	Referrer string `json:"referrer,omitempty"`
	Landing  string `json:"landing,omitempty"`
	At       string `json:"at,omitempty"`
}

// Attribution is the pair stored per user / organization.
type Attribution struct {
	FirstTouch Touch `json:"first_touch"`
	LastTouch  Touch `json:"last_touch"`
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// normalize bounds every field, drops a non-http referrer or a landing path
// that is not a path, and fills source / medium from the click id or the
// referrer host when the link had no utm_source.
func (t Touch) normalize() Touch {
	t.Source = strings.ToLower(clip(t.Source, 100))
	t.Medium = strings.ToLower(clip(t.Medium, 100))
	t.Campaign = clip(t.Campaign, 200)
	t.Term = clip(t.Term, 200)
	t.Content = clip(t.Content, 200)
	t.GCLID = clip(t.GCLID, 200)
	t.FBCLID = clip(t.FBCLID, 300)
	t.MSCLKID = clip(t.MSCLKID, 200)
	t.Referrer = clip(t.Referrer, 500)
	t.Landing = clip(t.Landing, 300)
	t.At = clip(t.At, 40)
	refHost := ""
	if t.Referrer != "" {
		u, err := url.Parse(t.Referrer)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			t.Referrer = ""
		} else {
			refHost = strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
		}
	}
	if t.Landing != "" && !strings.HasPrefix(t.Landing, "/") {
		t.Landing = ""
	}
	if _, err := time.Parse(time.RFC3339, t.At); err != nil {
		t.At = ""
	}
	if t.Source == "" {
		switch {
		case t.GCLID != "":
			t.Source, t.Medium = "google", "cpc"
		case t.MSCLKID != "":
			t.Source, t.Medium = "bing", "cpc"
		case t.FBCLID != "":
			t.Source, t.Medium = "facebook", "social"
		case refHost != "":
			t.Source, t.Medium = refHost, "referral"
		}
	}
	return t
}

func (t Touch) empty() bool { return t == Touch{} }

// ParseAttribution decodes the header value; ok is false when it is missing,
// oversized, malformed or carries nothing.
func ParseAttribution(raw string) (Attribution, bool) {
	if raw == "" || len(raw) > maxAttributionHeader {
		return Attribution{}, false
	}
	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		return Attribution{}, false
	}
	var in struct {
		FT Touch `json:"ft"`
		LT Touch `json:"lt"`
	}
	if err := json.Unmarshal([]byte(decoded), &in); err != nil {
		return Attribution{}, false
	}
	a := Attribution{FirstTouch: in.FT.normalize(), LastTouch: in.LT.normalize()}
	if a.FirstTouch.empty() {
		a.FirstTouch = a.LastTouch
	}
	if a.LastTouch.empty() {
		a.LastTouch = a.FirstTouch
	}
	return a, !a.FirstTouch.empty()
}

// summary is the report key: first touch, last touch when the first had no
// source (e.g. a direct first visit, later an ad click).
func (a Attribution) summary() (source, medium, campaign string) {
	t := a.FirstTouch
	if t.Source == "" {
		t = a.LastTouch
	}
	return t.Source, t.Medium, t.Campaign
}

// signupRoutes are the two places an account is created: password sign-up
// (user) and self-serve organization creation. Matched on method + path.
var signupRoutes = map[string]string{
	"POST /v1/auth/register":            "user",
	"POST /v1/onboarding/organizations": "organization",
}

// captureWriter tees a small JSON response so the uuid of the created
// record can be read after the handler ran.
type captureWriter struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (c *captureWriter) WriteHeader(status int) {
	c.status = status
	c.ResponseWriter.WriteHeader(status)
}

func (c *captureWriter) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	if c.buf.Len() < 64<<10 {
		c.buf.Write(p)
	}
	return c.ResponseWriter.Write(p)
}

func (c *captureWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// CaptureSignup stores the attribution of a successful sign-up without the
// auth and onboarding handlers knowing about it: it watches the two create
// routes and, on 201, records the header (organizations fall back to their
// owner's sign-up attribution). Recording never fails the request.
func CaptureSignup(q *db.Queries, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			kind, ok := signupRoutes[r.Method+" "+r.URL.Path]
			if !ok || q == nil {
				next.ServeHTTP(w, r)
				return
			}
			attr, hasAttr := ParseAttribution(r.Header.Get(AttributionHeader))
			if kind == "user" && !hasAttr {
				next.ServeHTTP(w, r)
				return
			}
			cw := &captureWriter{ResponseWriter: w}
			next.ServeHTTP(cw, r)
			if cw.status != http.StatusCreated {
				return
			}
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
			defer cancel()
			if err := record(ctx, q, kind, cw.buf.Bytes(), attr, hasAttr); err != nil && log != nil {
				log.Warn("marketing_attribution_record_failed", "kind", kind, "error", err)
			}
		})
	}
}

func record(ctx context.Context, q *db.Queries, kind string, body []byte, attr Attribution, hasAttr bool) error {
	var env struct {
		Data struct {
			User         *struct{ UUID uuid.UUID } `json:"user"`
			Organization *struct{ UUID uuid.UUID } `json:"organization"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return err
	}
	first, _ := json.Marshal(attr.FirstTouch)
	last, _ := json.Marshal(attr.LastTouch)
	source, medium, campaign := attr.summary()
	switch {
	case kind == "user" && env.Data.User != nil:
		id, err := q.MarketingUserIDByUUID(ctx, env.Data.User.UUID)
		if err != nil {
			return err
		}
		return q.MarketingRecordUserAttribution(ctx, db.MarketingRecordUserAttributionParams{
			UserID: pgtype.Int8{Int64: id, Valid: true}, FirstTouch: first, LastTouch: last,
			Source: source, Medium: medium, Campaign: campaign,
		})
	case kind == "organization" && env.Data.Organization != nil:
		id, err := q.MarketingOrganizationIDByUUID(ctx, env.Data.Organization.UUID)
		if err != nil {
			return err
		}
		if !hasAttr {
			return q.MarketingCopyOwnerAttribution(ctx, id)
		}
		return q.MarketingRecordOrganizationAttribution(ctx, db.MarketingRecordOrganizationAttributionParams{
			OrganizationID: pgtype.Int8{Int64: id, Valid: true}, FirstTouch: first, LastTouch: last,
			Source: source, Medium: medium, Campaign: campaign,
		})
	}
	return nil
}
