package marketing

import (
	"html"
	"io"
	"strings"

	nethtml "golang.org/x/net/html"
)

// snippetElements are the only elements a custom snippet may carry: what tag
// vendors ship (script / noscript pixel / iframe fallback / meta / link).
// Everything else, comments and stray text included, is dropped.
var snippetElements = map[string]bool{
	"script": true, "noscript": true, "style": true, "meta": true,
	"link": true, "img": true, "iframe": true,
}

var voidElements = map[string]bool{"meta": true, "link": true, "img": true}

// snippetAttrs are kept on the elements above; event handlers (`on*`),
// `srcdoc` and anything else are dropped.
var snippetAttrs = map[string]bool{
	"src": true, "href": true, "rel": true, "type": true, "async": true,
	"defer": true, "id": true, "name": true, "content": true, "property": true,
	"charset": true, "crossorigin": true, "integrity": true, "referrerpolicy": true,
	"width": true, "height": true, "alt": true, "style": true, "title": true,
	"loading": true, "media": true, "as": true, "allow": true, "frameborder": true,
	"nomodule": true,
}

// SanitizeSnippet reduces admin-entered HTML to the vendor tag shapes above.
// Inline script bodies are kept (that is what a tag snippet is), but URLs
// must be https and nothing can carry an inline event handler. The snippet
// is only ever rendered on the public marketing pages after marketing
// consent, and only platform admins with settings.write can save it.
func SanitizeSnippet(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return ""
	}
	z := nethtml.NewTokenizer(strings.NewReader(in))
	var b strings.Builder
	var open []string
	rawParent := ""
	for {
		tt := z.Next()
		switch tt {
		case nethtml.ErrorToken:
			if z.Err() != io.EOF {
				return ""
			}
			for i := len(open) - 1; i >= 0; i-- {
				b.WriteString("</" + open[i] + ">")
			}
			return strings.TrimSpace(b.String())
		case nethtml.TextToken:
			if rawParent == "script" || rawParent == "style" {
				b.Write(z.Raw())
			} else if len(open) > 0 {
				b.WriteString(html.EscapeString(string(z.Text())))
			}
		case nethtml.StartTagToken, nethtml.SelfClosingTagToken:
			tok := z.Token()
			if tok.Data == "noscript" && tt == nethtml.StartTagToken {
				// Parse the fallback pixel inside as tags, not raw text.
				z.NextIsNotRawText()
			}
			if !snippetElements[tok.Data] {
				continue
			}
			b.WriteString("<" + tok.Data)
			for _, attr := range tok.Attr {
				name := strings.ToLower(attr.Key)
				if attr.Namespace != "" || (!snippetAttrs[name] && !strings.HasPrefix(name, "data-")) {
					continue
				}
				if (name == "src" || name == "href") && !safeURL(attr.Val) {
					continue
				}
				b.WriteString(" " + name + `="` + html.EscapeString(attr.Val) + `"`)
			}
			b.WriteString(">")
			switch {
			case voidElements[tok.Data]:
			case tt == nethtml.SelfClosingTagToken:
				b.WriteString("</" + tok.Data + ">")
			default:
				open = append(open, tok.Data)
				if tok.Data == "script" || tok.Data == "style" {
					rawParent = tok.Data
				}
			}
		case nethtml.EndTagToken:
			tok := z.Token()
			for i := len(open) - 1; i >= 0; i-- {
				if open[i] != tok.Data {
					continue
				}
				for j := len(open) - 1; j >= i; j-- {
					b.WriteString("</" + open[j] + ">")
				}
				open = open[:i]
				rawParent = ""
				break
			}
		}
	}
}

// safeURL accepts absolute https and protocol-relative URLs only.
func safeURL(raw string) bool {
	v := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "//")
}
