package onboarding

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"

	webhook "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/usecase"
)

// Website fetching for discovery (CF-140). Only public HTTPS pages on port
// 443 are read: the URL is validated before every request (redirects
// included) and the webhooks transport refuses private addresses at dial
// time, after DNS, so a public name pointing inside the network is refused
// too. Every page is capped in size and the whole crawl in time.
const (
	maxPageBytes   = 512 << 10
	maxPages       = 4
	maxRedirects   = 3
	crawlBudget    = 15 * time.Second
	maxSummaryText = 4000
	maxPageText    = 2500
)

// ErrWebsite: the website URL is not a public HTTPS address or could not be read.
var ErrWebsite = errors.New("website must be a public HTTPS address that answers")

// SiteSummary is what discovery keeps of the public website.
type SiteSummary struct {
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	SiteName    string   `json:"site_name"`
	ThemeColor  string   `json:"theme_color"`
	Language    string   `json:"language"`
	Headings    []string `json:"headings"`
	Pages       []string `json:"pages"`
	// Text is the visible text of the pages read, trimmed for the prompt.
	Text string `json:"text"`
}

// Fetcher reads public web pages.
type Fetcher interface {
	Summarize(ctx context.Context, raw string) (SiteSummary, error)
}

// WebsiteFetcher is the SSRF-safe Fetcher.
type WebsiteFetcher struct {
	Client   *http.Client
	Validate func(string) error
}

// NewWebsiteFetcher uses the webhooks transport (dial-time private IP block).
func NewWebsiteFetcher() *WebsiteFetcher {
	return &WebsiteFetcher{Client: webhook.PublicHTTPClient(), Validate: validateSiteURL}
}

// NormalizeWebsite turns "acme.com" into "https://acme.com/" and validates it.
func NormalizeWebsite(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrWebsite
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", ErrWebsite
	}
	if u.Scheme == "http" {
		u.Scheme = "https"
	}
	if u.Path == "" {
		u.Path = "/"
	}
	u.RawQuery, u.Fragment = "", ""
	out := u.String()
	if err := validateSiteURL(out); err != nil {
		return "", err
	}
	return out, nil
}

func validateSiteURL(raw string) error {
	if err := webhook.ValidateURL(raw); err != nil {
		return ErrWebsite
	}
	u, _ := url.Parse(raw)
	if p := u.Port(); p != "" && p != "443" {
		return ErrWebsite
	}
	return nil
}

// candidatePaths are read after the home page when the site links to them.
var candidatePaths = regexp.MustCompile(`(?i)^/(hakkimizda|hakkımızda|about|about-us|kurumsal|company|hizmetler|services|urunler|products|pricing|fiyatlar|destek|support|iletisim|contact|sss|faq)/?$`)

// Summarize reads the home page and up to three linked about/services/
// support pages of the same host.
func (f *WebsiteFetcher) Summarize(ctx context.Context, raw string) (SiteSummary, error) {
	start, err := NormalizeWebsite(raw)
	if err != nil {
		return SiteSummary{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, crawlBudget)
	defer cancel()

	home, final, err := f.page(ctx, start)
	if err != nil {
		return SiteSummary{}, err
	}
	sum := SiteSummary{URL: final.String()}
	doc := parsePage(home, final)
	sum.Title, sum.Description, sum.SiteName = doc.title, doc.description, doc.siteName
	sum.ThemeColor, sum.Language = doc.themeColor, doc.lang
	sum.Headings = append([]string{}, doc.headings...)
	sum.Pages = []string{final.Path}
	texts := []string{clip(doc.text, maxPageText)}

	for _, link := range doc.links {
		if len(sum.Pages) >= maxPages || ctx.Err() != nil {
			break
		}
		body, u, err := f.page(ctx, link)
		if err != nil || u.Host != final.Host {
			continue
		}
		p := parsePage(body, u)
		sum.Pages = append(sum.Pages, u.Path)
		texts = append(texts, clip(p.text, maxPageText))
		for _, h := range p.headings {
			if len(sum.Headings) < 24 {
				sum.Headings = append(sum.Headings, h)
			}
		}
	}
	sum.Text = clip(strings.Join(texts, "\n\n"), maxSummaryText)
	return sum, nil
}

// page GETs one HTML page, following at most maxRedirects validated redirects.
func (f *WebsiteFetcher) page(ctx context.Context, raw string) ([]byte, *url.URL, error) {
	for hop := 0; hop <= maxRedirects; hop++ {
		if err := f.Validate(raw); err != nil {
			return nil, nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return nil, nil, ErrWebsite
		}
		req.Header.Set("User-Agent", "ClickFixOnboarding/1.0 (+https://clickfix.app)")
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		res, err := f.Client.Do(req)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v", ErrWebsite, err)
		}
		if res.StatusCode >= 300 && res.StatusCode < 400 {
			loc, lerr := res.Location()
			_ = res.Body.Close()
			if lerr != nil {
				return nil, nil, ErrWebsite
			}
			raw = loc.String()
			continue
		}
		body, err := readHTML(res)
		if err != nil {
			return nil, nil, err
		}
		return body, res.Request.URL, nil
	}
	return nil, nil, ErrWebsite
}

func readHTML(res *http.Response) ([]byte, error) {
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, ErrWebsite
	}
	if ct := res.Header.Get("Content-Type"); ct != "" {
		mt, _, _ := mime.ParseMediaType(ct)
		if mt != "text/html" && mt != "application/xhtml+xml" {
			return nil, ErrWebsite
		}
	}
	return io.ReadAll(io.LimitReader(res.Body, maxPageBytes))
}

type parsedPage struct {
	title, description, siteName, themeColor, lang string
	headings                                       []string
	links                                          []string
	text                                           string
}

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// parsePage extracts the title, meta tags, headings, same-host candidate
// links and the visible text (scripts, styles and navigation skipped).
func parsePage(body []byte, base *url.URL) parsedPage {
	var p parsedPage
	root, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return p
	}
	var text strings.Builder
	seen := map[string]bool{}
	var walk func(n *html.Node, skip bool)
	walk = func(n *html.Node, skip bool) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "svg", "template", "iframe":
				return
			case "nav", "footer", "header", "form":
				skip = true
			case "html":
				p.lang = strings.ToLower(attr(n, "lang"))
			case "title":
				if p.title == "" && n.FirstChild != nil {
					p.title = clean(n.FirstChild.Data)
				}
			case "meta":
				content := clean(attr(n, "content"))
				switch strings.ToLower(attr(n, "name") + attr(n, "property")) {
				case "description", "og:description":
					if p.description == "" {
						p.description = clip(content, 400)
					}
				case "og:site_name":
					p.siteName = clip(content, 120)
				case "theme-color":
					if hexColor.MatchString(content) {
						p.themeColor = strings.ToLower(content)
					}
				}
			case "h1", "h2", "h3":
				if h := clip(clean(innerText(n)), 160); h != "" && len(p.headings) < 16 {
					p.headings = append(p.headings, h)
				}
			case "a":
				if href := attr(n, "href"); href != "" {
					if u, err := base.Parse(href); err == nil && u.Host == base.Host && candidatePaths.MatchString(u.Path) {
						u.RawQuery, u.Fragment = "", ""
						if s := u.String(); !seen[s] && u.Path != base.Path {
							seen[s] = true
							p.links = append(p.links, s)
						}
					}
				}
			}
		}
		if n.Type == html.TextNode && !skip {
			if t := clean(n.Data); t != "" && text.Len() < 4*maxPageText {
				text.WriteString(t)
				text.WriteByte(' ')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, skip)
		}
	}
	walk(root, false)
	p.text = strings.TrimSpace(text.String())
	return p
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func innerText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func clean(s string) string { return strings.Join(strings.Fields(s), " ") }

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
