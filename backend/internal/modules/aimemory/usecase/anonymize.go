package usecase

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

// Identity is what must never leave a case memory towards another customer:
// the source contact's name, e-mail and phone and the company's name and
// domain. Pattern-based PII (e-mail, phone, card, IBAN, national id) is
// removed for everyone regardless of whose it is.
type Identity struct {
	Names  []string // contact and company names
	Emails []string
	Phones []string
	Hosts  []string // company domains
}

var (
	placeholder = regexp.MustCompile(`\[PII_([A-Z]+)_[a-z]+_[0-9]+\]`)
	urlPattern  = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+`)
	digitsOnly  = regexp.MustCompile(`[^0-9]`)
	// Generic company suffixes stay out of the name tokens: removing every
	// "Ltd" in a summary would hide nothing and break sentences.
	stopTokens = map[string]bool{"ltd": true, "şti": true, "sti": true, "a.ş.": true, "aş": true, "as": true, "inc": true, "llc": true, "gmbh": true, "tic": true, "san": true, "ve": true, "and": true, "co": true, "the": true}
)

// Anonymize removes the identity and all pattern PII from text. It is
// deterministic so the same summary always reads the same way.
func Anonymize(text string, id Identity) string {
	text = placeholder.ReplaceAllString(ai.NewMasker().Mask(text), "[$1]")
	text = urlPattern.ReplaceAllString(text, "[URL]")
	for _, email := range id.Emails {
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		text = replaceFold(text, email, "[EMAIL]")
		if local, _, ok := strings.Cut(email, "@"); ok && utf8.RuneCountInString(local) >= 3 {
			text = replaceWord(text, local, "[EMAIL]")
		}
	}
	for _, phone := range id.Phones {
		digits := digitsOnly.ReplaceAllString(phone, "")
		if len(digits) < 7 {
			continue
		}
		// Match the last 7+ digits with any separators in between.
		tail := digits[len(digits)-7:]
		parts := make([]string, len(tail))
		for i, d := range tail {
			parts[i] = string(d)
		}
		re := regexp.MustCompile(strings.Join(parts, `[\s().-]*`))
		text = re.ReplaceAllString(text, "[PHONE]")
	}
	for _, host := range id.Hosts {
		host = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "www.")
		if host == "" {
			continue
		}
		text = replaceFold(text, host, "[DOMAIN]")
		if label, _, ok := strings.Cut(host, "."); ok && utf8.RuneCountInString(label) >= 3 {
			text = replaceWord(text, label, "[COMPANY]")
		}
	}
	// Full names first, then each meaningful token, longest first, so
	// "Ayşe Yılmaz" and a later lone "Yılmaz" are both removed.
	tokens := []string{}
	for _, name := range id.Names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		text = replaceWord(text, name, "[NAME]")
		for _, t := range strings.FieldsFunc(name, func(r rune) bool { return unicode.IsSpace(r) || r == ',' }) {
			t = strings.Trim(t, ".,;:()\"'")
			if utf8.RuneCountInString(t) >= 3 && !stopTokens[lowerTR(t)] {
				tokens = append(tokens, t)
			}
		}
	}
	sort.SliceStable(tokens, func(i, j int) bool { return len(tokens[i]) > len(tokens[j]) })
	for _, t := range tokens {
		text = replaceWord(text, t, "[NAME]")
	}
	return strings.TrimSpace(text)
}

// Leaks reports whether text still contains any part of the identity or
// pattern PII. A summary that leaks cannot be approved.
func Leaks(text string, id Identity) bool {
	if placeholder.MatchString(ai.NewMasker().Mask(text)) {
		return true
	}
	return Anonymize(text, id) != strings.TrimSpace(text)
}

// lowerTR lower-cases with Turkish dotted/dotless I so "IŞIK" and "ışık"
// compare equal.
func lowerTR(s string) string {
	return strings.ToLower(strings.NewReplacer("I", "ı", "İ", "i").Replace(s))
}

// foldPattern matches s case-insensitively, treating I/ı/İ/i as one letter.
func foldPattern(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'i', 'I', 'ı', 'İ':
			b.WriteString(`[iIıİ]`)
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return `(?i)` + b.String()
}

func replaceFold(text, value, with string) string {
	return regexp.MustCompile(foldPattern(value)).ReplaceAllString(text, with)
}

// replaceWord replaces value only at word boundaries (Unicode aware; Go's \b
// is ASCII only), so "Can" does not eat the middle of "Cannot".
func replaceWord(text, value, with string) string {
	re := regexp.MustCompile(`(^|[^\p{L}\p{N}])(` + foldPattern(value)[4:] + `)($|[^\p{L}\p{N}])`)
	re = regexp.MustCompile(`(?i)` + re.String())
	for {
		next := re.ReplaceAllString(text, "${1}"+with+"${3}")
		if next == text {
			return text
		}
		text = next
	}
}
