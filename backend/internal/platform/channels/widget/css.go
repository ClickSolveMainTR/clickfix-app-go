package widget

import (
	"regexp"
	"slices"
	"strings"
)

// Custom CSS runs inside the widget's Shadow DOM, so it can only restyle
// the widget. SanitizeCSS still removes everything that could load or run
// code or leak the visitor (imports, scripts, legacy expression/behaviour
// hooks, non-https resources). The SDK applies the same rules again
// (frontend/widget-sdk/src/shared/css.ts) before it injects the CSS.
var (
	cssCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssAtRuleRe  = regexp.MustCompile(`(?i)@(import|charset|namespace)[^;{}]*;?`)
	cssExprRe    = regexp.MustCompile(`(?i)expression\s*\(`)
	cssSchemeRe  = regexp.MustCompile(`(?i)(javascript|vbscript)\s*:`)
	cssBindingRe = regexp.MustCompile(`(?i)(-moz-binding|behavior)\s*:`)
	cssURLRe     = regexp.MustCompile(`(?i)url\(\s*(['"]?)([^'")]*)(['"]?)\s*\)`)
	cssSafeURLRe = regexp.MustCompile(`(?i)^(https://|data:image/(png|jpeg|gif|webp);)`)
)

// SanitizeCSS returns the safe part of custom CSS and what it removed.
func SanitizeCSS(css string) (string, []string) {
	var problems []string
	note := func(p string) {
		if !slices.Contains(problems, p) {
			problems = append(problems, p)
		}
	}
	if len([]rune(css)) > MaxCustomCSS {
		css = clip(css, MaxCustomCSS)
		note("custom CSS is too long")
	}
	css = cssCommentRe.ReplaceAllString(css, "")
	if strings.Contains(css, `\`) {
		css = strings.ReplaceAll(css, `\`, "")
		note("escape sequences are not allowed")
	}
	if strings.Contains(css, "<") {
		css = strings.ReplaceAll(css, "<", "")
		note("HTML is not allowed")
	}
	if cssAtRuleRe.MatchString(css) {
		css = cssAtRuleRe.ReplaceAllString(css, "")
		note("@import, @charset and @namespace are not allowed")
	}
	if cssExprRe.MatchString(css) {
		css = cssExprRe.ReplaceAllString(css, "invalid(")
		note("expression() is not allowed")
	}
	if cssSchemeRe.MatchString(css) {
		css = cssSchemeRe.ReplaceAllString(css, "")
		note("script URLs are not allowed")
	}
	if cssBindingRe.MatchString(css) {
		css = cssBindingRe.ReplaceAllString(css, "x-blocked:")
		note("behavior and -moz-binding are not allowed")
	}
	css = cssURLRe.ReplaceAllStringFunc(css, func(m string) string {
		target := strings.TrimSpace(cssURLRe.FindStringSubmatch(m)[2])
		if cssSafeURLRe.MatchString(target) {
			return m
		}
		note("only https:// and data:image URLs are allowed")
		return "none"
	})
	return strings.TrimSpace(css), problems
}
