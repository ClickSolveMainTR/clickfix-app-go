package ai

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Anonymizer irreversibly removes personal data before text is stored in the
// history archive or reaches a provider (CF-217). It extends RedactPII with
// values known from structured fields of the same record (a contact's name,
// company, address or tax number), which pattern rules alone cannot find.
// It keeps no map of originals.
type Anonymizer struct {
	known map[string]string // lower-cased value -> kind
}

func NewAnonymizer() *Anonymizer { return &Anonymizer{known: map[string]string{}} }

// Known registers a value of a kind (NAME, COMPANY, ADDRESS, TAXNO, EMAIL,
// PHONE). A multi-word NAME also registers each part of three letters or
// more, because messages often use the first name alone.
func (a *Anonymizer) Known(kind, value string) {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) < 3 {
		return
	}
	a.known[strings.ToLower(value)] = kind
	if kind == "NAME" {
		for _, part := range strings.Fields(value) {
			if utf8.RuneCountInString(part) >= 3 {
				a.known[strings.ToLower(part)] = kind
			}
		}
	}
	if kind == "PHONE" {
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, value)
		if len(digits) >= 7 {
			a.known[digits] = kind
		}
	}
}

// Redact applies the shared pattern rules first (so an e-mail address is
// replaced whole), then known values (longest first, whole words only, case
// insensitive).
func (a *Anonymizer) Redact(text string) string {
	text = RedactPII(text)
	if a != nil && len(a.known) > 0 {
		values := make([]string, 0, len(a.known))
		for v := range a.known {
			values = append(values, v)
		}
		sort.Slice(values, func(i, j int) bool {
			if len(values[i]) != len(values[j]) {
				return len(values[i]) > len(values[j])
			}
			return values[i] < values[j]
		})
		for _, v := range values {
			text = replaceWord(text, v, "[PII_"+a.known[v]+"]")
		}
	}
	return text
}

// replaceWord replaces case-insensitive occurrences of value that are not
// inside a longer word. RE2's \b is ASCII only, so boundaries are checked here.
func replaceWord(text, value, with string) string {
	pattern := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(value))
	matches := pattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for _, m := range matches {
		before, _ := utf8.DecodeLastRuneInString(text[:m[0]])
		after, _ := utf8.DecodeRuneInString(text[m[1]:])
		if m[0] > 0 && wordRune(before) || m[1] < len(text) && wordRune(after) {
			continue
		}
		out.WriteString(text[last:m[0]])
		out.WriteString(with)
		last = m[1]
	}
	out.WriteString(text[last:])
	return out.String()
}

func wordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
