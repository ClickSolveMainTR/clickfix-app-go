package customfields

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Audience is a surface that reads custom field values (CF-196). Every
// reader asks Project for its audience instead of reading custom_fields
// directly, so the visibility switches and the sensitive flag are enforced
// in one place.
type Audience string

const (
	// AudienceAgent: the agent screens (contact/company card, conversation
	// panel). Sensitive values are masked, not dropped.
	AudienceAgent Audience = "agent"
	// AudienceAI: AI context and AI-written text (greeting). Sensitive
	// values never pass.
	AudienceAI Audience = "ai"
	// AudienceWorkflow: workflow and automation conditions.
	AudienceWorkflow Audience = "workflow"
	// AudienceReports: exports and reports. Sensitive values never pass.
	AudienceReports Audience = "reports"
)

// Mask replaces a sensitive value on agent screens.
const Mask = "••••"

// Allows reports whether the audience may read d at all.
func (d Definition) Allows(a Audience) bool {
	switch a {
	case AudienceAgent:
		return d.Visibility.Agent
	case AudienceAI:
		return d.Visibility.AI && !d.Sensitive && !d.Archived
	case AudienceWorkflow:
		return d.Visibility.Workflow && !d.Archived
	case AudienceReports:
		return d.Visibility.Reports && !d.Sensitive && !d.Archived
	}
	return false
}

// FieldValue is one stored value as an audience sees it.
type FieldValue struct {
	Key     string    `json:"key"`
	Label   string    `json:"label"`
	Type    FieldType `json:"type"`
	Value   any       `json:"value"`
	Display string    `json:"display"`
	Masked  bool      `json:"masked"`
}

// Project returns the values of defs that audience a may read, in
// definition order. Unset fields are skipped. On AudienceAgent a sensitive
// value comes back with Value nil, Display Mask and Masked true.
func Project(defs []Definition, values map[string]any, a Audience) []FieldValue {
	out := []FieldValue{}
	for _, d := range defs {
		v, set := values[d.Key]
		if !set || isEmpty(v) || !d.Allows(a) {
			continue
		}
		fv := FieldValue{Key: d.Key, Label: d.Label, Type: d.Type, Value: v, Display: Display(d, v)}
		if a == AudienceAgent && d.Sensitive {
			fv.Value, fv.Display, fv.Masked = nil, Mask, true
		}
		out = append(out, fv)
	}
	return out
}

// FilterFor returns the stored map with only the keys audience a may read
// (sensitive values dropped, also for AudienceAgent). Use it where a raw map
// leaves the module, e.g. a workflow data map.
func FilterFor(defs []Definition, values map[string]any, a Audience) map[string]any {
	out := map[string]any{}
	for _, d := range defs {
		if v, ok := values[d.Key]; ok && d.Allows(a) && (a != AudienceAgent || !d.Sensitive) {
			out[d.Key] = v
		}
	}
	return out
}

// Display renders a stored value as text: option labels for list fields,
// "Evet"/"Hayır" for booleans, "12.50 TRY" for money.
func Display(d Definition, v any) string {
	label := func(id string) string {
		for _, o := range d.Options {
			if o.Value == id {
				return o.Label
			}
		}
		return id
	}
	switch d.Type {
	case TypeSelect:
		if s, ok := v.(string); ok {
			return label(s)
		}
	case TypeMultiselect:
		var parts []string
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				parts = append(parts, label(fmt.Sprint(item)))
			}
		case []string:
			for _, item := range x {
				parts = append(parts, label(item))
			}
		}
		return strings.Join(parts, ", ")
	case TypeBoolean:
		if b, ok := v.(bool); ok {
			if b {
				return "Evet"
			}
			return "Hayır"
		}
	case TypeMoney:
		if m, ok := v.(map[string]any); ok {
			f, err := toFloat(m["amount_minor"])
			if err == nil {
				return strconv.FormatFloat(f/100, 'f', 2, 64) + " " + fmt.Sprint(m["currency"])
			}
		}
	case TypeNumber:
		if f, ok := v.(float64); ok && f == math.Trunc(f) && math.Abs(f) < 1e15 {
			return strconv.FormatInt(int64(f), 10)
		}
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// TemplateVars returns template variables "<prefix>.<key>" → display text
// for the fields audience a may read (sensitive and hidden fields are left
// out, so {{contact.secret}} renders empty).
func TemplateVars(prefix string, defs []Definition, values map[string]any, a Audience) map[string]string {
	out := map[string]string{}
	for _, fv := range Project(defs, values, a) {
		if fv.Masked {
			continue
		}
		out[prefix+"."+fv.Key] = fv.Display
	}
	return out
}

// FieldMatch is a catalog condition: the entity's list field `key` holds one
// of `options` (option ids, never labels). SLA policies, workflow-free
// consumers and article audiences store this shape.
type FieldMatch struct {
	Entity  string   `json:"entity"`
	Key     string   `json:"key"`
	Options []string `json:"options"`
}

// MatchEntities are the entities a FieldMatch may point at.
var MatchEntities = []string{EntityContact, EntityCompany, EntityConversation}

// MaxMatches caps the matches of one rule.
const MaxMatches = 20

// NormalizeMatches checks matches against the catalog: the field must be a
// live list field of its entity and every option an existing option. Input
// options may be ids or labels (old free-text rules); the result holds ids.
// defsOf returns the definitions of one entity.
func NormalizeMatches(matches []FieldMatch, defsOf func(entity string) []Definition) ([]FieldMatch, []FieldError) {
	if len(matches) > MaxMatches {
		return nil, []FieldError{{Field: "custom_fields", Code: CodeInvalid, Message: fmt.Sprintf("at most %d field conditions", MaxMatches)}}
	}
	out := make([]FieldMatch, 0, len(matches))
	var errs []FieldError
	for i, m := range matches {
		path := fmt.Sprintf("custom_fields[%d]", i)
		if !slices.Contains(MatchEntities, m.Entity) {
			errs = append(errs, FieldError{Field: path + ".entity", Code: CodeInvalid, Message: "entity must be contact, company or conversation"})
			continue
		}
		var def *Definition
		for _, d := range defsOf(m.Entity) {
			if d.Key == m.Key {
				def = &d
				break
			}
		}
		if def == nil || def.Archived || (def.Type != TypeSelect && def.Type != TypeMultiselect) {
			errs = append(errs, FieldError{Field: path + ".key", Code: CodeUnknown, Message: "key must name a list field of the entity"})
			continue
		}
		if len(m.Options) == 0 {
			errs = append(errs, FieldError{Field: path + ".options", Code: CodeRequired, Message: "pick at least one option"})
			continue
		}
		ids := []string{}
		for _, o := range m.Options {
			id, ok := ResolveOption(*def, o)
			if !ok {
				errs = append(errs, FieldError{Field: path + ".options", Code: CodeInvalid, Message: fmt.Sprintf("%q is not an option", o)})
				continue
			}
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		out = append(out, FieldMatch{Entity: m.Entity, Key: m.Key, Options: ids})
	}
	return out, errs
}

// Matches reports whether every match holds (AND across matches, OR across
// one match's options). valuesOf returns the stored custom_fields of an
// entity (nil when the entity is missing, e.g. no company: no match).
func Matches(matches []FieldMatch, valuesOf func(entity string) map[string]any) bool {
	for _, m := range matches {
		values := valuesOf(m.Entity)
		if !holdsOption(values[m.Key], m.Options) {
			return false
		}
	}
	return true
}

func holdsOption(stored any, options []string) bool {
	switch x := stored.(type) {
	case string:
		return slices.Contains(options, x)
	case []any:
		for _, item := range x {
			if s, ok := item.(string); ok && slices.Contains(options, s) {
				return true
			}
		}
	case []string:
		for _, s := range x {
			if slices.Contains(options, s) {
				return true
			}
		}
	}
	return false
}

var keyReplacer = strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "İ", "i", "ö", "o", "ş", "s", "ü", "u", "Ç", "c", "Ğ", "g", "Ö", "o", "Ş", "s", "Ü", "u")

// SuggestKey turns a received attribute name ("Lisans Bitiş", "lisansBitis")
// into a valid field key ("lisans_bitis"); "" when nothing usable is left.
func SuggestKey(raw string) string {
	var b strings.Builder
	prevUnderscore := false
	for i, r := range keyReplacer.Replace(strings.TrimSpace(raw)) {
		switch {
		case r >= 'A' && r <= 'Z':
			if i > 0 && !prevUnderscore {
				b.WriteByte('_')
			}
			b.WriteRune(r + ('a' - 'A'))
			prevUnderscore = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if !prevUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	key := strings.Trim(b.String(), "_")
	for key != "" && (key[0] < 'a' || key[0] > 'z') {
		key = strings.TrimLeft(key[1:], "_")
	}
	if len(key) > 63 {
		key = strings.TrimRight(key[:63], "_")
	}
	return key
}

// ValueKind guesses the field type of a received value for the one-click
// definition: number, boolean, date (YYYY-MM-DD) or text.
func ValueKind(v any) string {
	switch x := v.(type) {
	case bool:
		return "boolean"
	case float64, int, int64:
		return "number"
	case string:
		if _, err := Normalize(Definition{Type: TypeDate}, x); err == nil && len(strings.TrimSpace(x)) >= 10 {
			return "date"
		}
	}
	return "text"
}

var templateVar = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)

// RenderTemplate replaces {{name}} variables with vars; unknown or hidden
// ones render empty and the doubled spaces they leave are collapsed.
func RenderTemplate(text string, vars map[string]string) string {
	if !strings.Contains(text, "{{") {
		return text
	}
	out := templateVar.ReplaceAllStringFunc(text, func(m string) string {
		return vars[templateVar.FindStringSubmatch(m)[1]]
	})
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	return strings.TrimSpace(strings.ReplaceAll(out, " ,", ","))
}
