package usecase

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"golang.org/x/net/html"
)

const crawlerAgent = "ClickFixKnowledgeBot"

type PageFetcher func(context.Context, string) ([]byte, int, error)
type CrawledDocument struct {
	Location string
	Title    string
	Body     string
}

func safeKnowledgeFetcher() PageFetcher {
	return func(ctx context.Context, target string) ([]byte, int, error) {
		u, err := toolURL(target)
		if err != nil {
			return nil, 0, err
		}
		transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, ResponseHeaderTimeout: 8 * time.Second, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil || !strings.EqualFold(host, u.Hostname()) || port != "443" {
				return nil, model.ErrInvalid
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil || len(ips) == 0 {
				return nil, model.ErrInvalid
			}
			for _, ip := range ips {
				if !publicToolIP(ip) {
					return nil, model.ErrInvalid
				}
			}
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		}}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("User-Agent", crawlerAgent+"/1.0")
		req.Header.Set("Accept", "text/html, text/plain, application/xml, text/xml")
		resp, err := client.Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer func() { _ = resp.Body.Close() }()
		data, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
		if err != nil || len(data) > 2<<20 {
			return nil, resp.StatusCode, model.ErrInvalid
		}
		return data, resp.StatusCode, nil
	}
}

type robotsRule struct {
	pattern *regexp.Regexp
	length  int
	allow   bool
}
type robotsPolicy struct {
	rules    []robotsRule
	sitemaps []string
	delay    time.Duration
}

func parseRobots(raw string) (robotsPolicy, error) {
	type group struct {
		agents []string
		rules  []robotsRule
		delay  time.Duration
	}
	groups := []group{}
	current := group{}
	started := false
	sitemaps := []string{}
	flush := func() {
		if len(current.agents) > 0 {
			groups = append(groups, current)
		}
		current = group{}
		started = false
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := strings.ToLower(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1])
		switch key {
		case "user-agent":
			if started {
				flush()
			}
			current.agents = append(current.agents, strings.ToLower(value))
		case "allow", "disallow":
			started = true
			if value == "" {
				continue
			}
			if len(value) > 2000 {
				return robotsPolicy{}, model.ErrInvalid
			}
			suffix := ""
			if strings.HasSuffix(value, "$") {
				value = strings.TrimSuffix(value, "$")
				suffix = "$"
			}
			pattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(value), `\*`, ".*") + suffix
			re, err := regexp.Compile(pattern)
			if err != nil {
				return robotsPolicy{}, err
			}
			current.rules = append(current.rules, robotsRule{pattern: re, length: len(strings.ReplaceAll(value, "*", "")), allow: key == "allow"})
		case "sitemap":
			sitemaps = append(sitemaps, value)
		case "crawl-delay":
			started = true
			n, err := strconv.ParseFloat(value, 64)
			if err != nil || n < 0 || n > 30 {
				return robotsPolicy{}, model.ErrInvalid
			}
			current.delay = time.Duration(n * float64(time.Second))
		}
	}
	flush()
	policy := robotsPolicy{sitemaps: sitemaps, delay: 500 * time.Millisecond}
	best := -1
	for _, g := range groups {
		specificity := -1
		for _, agent := range g.agents {
			if agent == "*" {
				specificity = max(specificity, 0)
			} else if strings.Contains(strings.ToLower(crawlerAgent), agent) {
				specificity = max(specificity, len(agent))
			}
		}
		if specificity < 0 || specificity < best {
			continue
		}
		if specificity > best {
			policy.rules = nil
			policy.delay = 500 * time.Millisecond
			best = specificity
		}
		policy.rules = append(policy.rules, g.rules...)
		policy.delay = max(policy.delay, g.delay)
	}
	return policy, nil
}
func (p robotsPolicy) allowed(u *url.URL) bool {
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	allow, best := true, -1
	for _, r := range p.rules {
		if r.pattern.MatchString(path) && (r.length > best || (r.length == best && r.allow)) {
			allow, best = r.allow, r.length
		}
	}
	return allow
}
func sameKnowledgeOrigin(base *url.URL, raw string) (*url.URL, bool) {
	ref, err := url.Parse(raw)
	if err != nil {
		return nil, false
	}
	u := base.ResolveReference(ref)
	u.Fragment = ""
	if !strings.EqualFold(u.Host, base.Host) || u.Scheme != base.Scheme {
		return nil, false
	}
	if _, err = toolURL(u.String()); err != nil {
		return nil, false
	}
	return u, true
}

// CrawlKnowledge enforces the selected robots group for every page and sitemap,
// same-origin navigation, a depth cap, bounded sitemap expansion and page count.
// Redirects are intentionally refused; every connection is DNS-pinned to public IPs.
func CrawlKnowledge(ctx context.Context, seed string, depth int, fetch PageFetcher) ([]CrawledDocument, error) {
	base, err := toolURL(seed)
	if err != nil || depth < 0 || depth > 3 {
		return nil, model.ErrInvalid
	}
	if fetch == nil {
		fetch = safeKnowledgeFetcher()
	}
	robotsURL := base.Scheme + "://" + base.Host + "/robots.txt"
	raw, status, err := fetch(ctx, robotsURL)
	if err != nil {
		return nil, err
	}
	if status != 200 && status != 404 {
		return nil, errors.New("knowledge: robots unavailable or denied")
	}
	policy := robotsPolicy{delay: 500 * time.Millisecond}
	if status == 200 {
		policy, err = parseRobots(string(raw))
		if err != nil {
			return nil, err
		}
	}
	if !policy.allowed(base) {
		return nil, errors.New("knowledge: URL disallowed by robots")
	}
	// Use one cancellable delay between requests, including sitemap requests.
	last := time.Now()
	read := func(u *url.URL) ([]byte, error) {
		if !policy.allowed(u) {
			return nil, errors.New("knowledge: robots denied page")
		}
		wait := policy.delay - time.Since(last)
		if wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		body, status, err := fetch(ctx, u.String())
		last = time.Now()
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, errors.New("knowledge: source did not return HTTP 200")
		}
		return body, nil
	}
	type entry struct {
		u     *url.URL
		depth int
	}
	pending := []entry{{base, 0}}
	maps := append([]string{}, policy.sitemaps...)
	if len(maps) == 0 {
		maps = []string{base.Scheme + "://" + base.Host + "/sitemap.xml"}
	}
	seenMaps := map[string]bool{}
	for len(maps) > 0 && len(seenMaps) < 5 && len(pending) < 100 {
		target := maps[0]
		maps = maps[1:]
		u, ok := sameKnowledgeOrigin(base, target)
		if !ok || seenMaps[u.String()] || !policy.allowed(u) {
			continue
		}
		seenMaps[u.String()] = true
		body, e := read(u)
		if e != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		var sitemap struct {
			XMLName xml.Name
			URLs    []struct {
				Loc string `xml:"loc"`
			} `xml:"url"`
			Maps []struct {
				Loc string `xml:"loc"`
			} `xml:"sitemap"`
		}
		if xml.Unmarshal(body, &sitemap) != nil {
			continue
		}
		for _, m := range sitemap.Maps {
			if len(maps) < 20 {
				maps = append(maps, m.Loc)
			}
		}
		if depth > 0 {
			for _, item := range sitemap.URLs {
				if u, ok := sameKnowledgeOrigin(base, item.Loc); ok && len(pending) < 100 {
					pending = append(pending, entry{u, 1})
				}
			}
		}
	}
	seen := map[string]bool{}
	out := []CrawledDocument{}
	for len(pending) > 0 && len(seen) < 20 {
		item := pending[0]
		pending = pending[1:]
		if seen[item.u.String()] || !policy.allowed(item.u) {
			continue
		}
		seen[item.u.String()] = true
		body, e := read(item.u)
		if e != nil {
			return nil, e
		}
		title, text, links, e := knowledgeHTML(body)
		if e != nil {
			return nil, e
		}
		if text != "" {
			out = append(out, CrawledDocument{Location: item.u.String(), Title: title, Body: text})
		}
		if item.depth < depth {
			for _, link := range links {
				if u, ok := sameKnowledgeOrigin(item.u, link); ok && len(pending) < 100 {
					pending = append(pending, entry{u, item.depth + 1})
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("knowledge: no extractable source content")
	}
	return out, nil
}
func knowledgeHTML(body []byte) (string, string, []string, error) {
	tokenizer := html.NewTokenizer(strings.NewReader(string(body)))
	var text, title strings.Builder
	links := []string{}
	skip, titleMode := 0, false
	for {
		kind := tokenizer.Next()
		if kind == html.ErrorToken {
			break
		}
		token := tokenizer.Token()
		switch kind {
		case html.StartTagToken, html.SelfClosingTagToken:
			if token.Data == "script" || token.Data == "style" || token.Data == "nav" || token.Data == "noscript" {
				skip++
			}
			if token.Data == "title" {
				titleMode = true
			}
			if token.Data == "a" && len(links) < 100 {
				for _, a := range token.Attr {
					if a.Key == "href" {
						links = append(links, a.Val)
					}
				}
			}
		case html.EndTagToken:
			if token.Data == "script" || token.Data == "style" || token.Data == "nav" || token.Data == "noscript" {
				skip = max(0, skip-1)
			}
			if token.Data == "title" {
				titleMode = false
			}
			if skip == 0 {
				text.WriteByte('\n')
			}
		case html.TextToken:
			if skip == 0 {
				if titleMode {
					title.WriteString(token.Data)
				} else {
					text.WriteString(token.Data)
					text.WriteByte(' ')
				}
			}
		}
		if text.Len() > 180000 || title.Len() > 300 {
			return "", "", nil, model.ErrInvalid
		}
	}
	return strings.TrimSpace(title.String()), strings.TrimSpace(text.String()), links, nil
}
