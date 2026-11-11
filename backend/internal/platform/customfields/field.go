// Package customfields is the organization-defined field engine (CF-7).
//
// An organization defines fields per entity (contact, company, conversation,
// ticket or one of its custom object types). Values are stored in the owning
// row's custom_fields JSONB keyed by the definition key, so a new field needs
// no migration. This package validates definitions, normalizes and validates
// values (types, required and conditional rules, defaults) and builds the
// JSONB filter and sort SQL that list endpoints use. It has no database
// access; modules/customfields loads definitions and stores them.
package customfields

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// FieldType is a supported value type.
type FieldType string

const (
	TypeText        FieldType = "text"
	TypeTextarea    FieldType = "textarea"
	TypeNumber      FieldType = "number"
	TypeMoney       FieldType = "money"
	TypeDate        FieldType = "date"
	TypeBoolean     FieldType = "boolean"
	TypeSelect      FieldType = "select"
	TypeMultiselect FieldType = "multiselect"
	TypeURL         FieldType = "url"
	TypeEmail       FieldType = "email"
	TypePhone       FieldType = "phone"
	TypeUser        FieldType = "user"
)

// Types lists every field type in display order.
var Types = []FieldType{
	TypeText, TypeTextarea, TypeNumber, TypeMoney, TypeDate, TypeBoolean,
	TypeSelect, TypeMultiselect, TypeURL, TypeEmail, TypePhone, TypeUser,
}

// Entity types that carry custom fields. EntityObject needs an object type.
const (
	EntityContact      = "contact"
	EntityCompany      = "company"
	EntityConversation = "conversation"
	EntityTicket       = "ticket"
	EntityObject       = "object"
)

// EntityTypes lists the entities in display order.
var EntityTypes = []string{EntityContact, EntityCompany, EntityConversation, EntityTicket, EntityObject}

// Limits.
const (
	MaxDefinitionsPerEntity = 100
	MaxOptions              = 200
	MaxLabelLength          = 100
	DefaultTextLength       = 500
	MaxTextLength           = 500
	MaxTextareaLength       = 10000
	MaxURLLength            = 2048
)

var (
	keyPattern    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	currencyRegex = regexp.MustCompile(`^[A-Z]{3}$`)
)

// Option is one choice of a select or multiselect field. Archived options
// keep existing values readable but cannot be picked again.
type Option struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Color    string `json:"color,omitempty"`
	Archived bool   `json:"archived,omitempty"`
}

// Config holds type-specific settings; unused fields are ignored.
type Config struct {
	// MaxLength caps text/textarea (defaults per type).
	MaxLength int `json:"max_length,omitempty"`
	// Min/Max bound number values and money amounts (major units).
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
	// Precision is the number of decimals a number may carry (nil: any).
	Precision *int `json:"precision,omitempty"`
	// Currency is the default ISO 4217 code for money (TRY when empty).
	Currency string `json:"currency,omitempty"`
	// MaxSelected caps multiselect values (0: unlimited).
	MaxSelected int    `json:"max_selected,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

// Condition makes a field conditional: it is shown, required and stored only
// while the referenced field matches. Operators: equals, not_equals, in,
// is_set, is_not_set.
type Condition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
}

// Condition operators.
const (
	CondEquals    = "equals"
	CondNotEquals = "not_equals"
	CondIn        = "in"
	CondIsSet     = "is_set"
	CondIsNotSet  = "is_not_set"
)

// Definition is one field as the engine sees it.
type Definition struct {
	Key       string     `json:"key"`
	Label     string     `json:"label"`
	Type      FieldType  `json:"type"`
	Required  bool       `json:"required"`
	Options   []Option   `json:"options,omitempty"`
	Config    Config     `json:"config"`
	Default   any        `json:"default,omitempty"`
	Condition *Condition `json:"condition,omitempty"`
	Archived  bool       `json:"archived,omitempty"`
	// Visibility says which surfaces may read the value (CF-196).
	Visibility Visibility `json:"visibility"`
	// Sensitive values are masked on agent screens and never reach AI
	// context, exports, reports or template text.
	Sensitive bool `json:"sensitive,omitempty"`
	// WidgetAccess decides whether the widget SDK may write the field by its
	// key without an attribute mapping: none, verified (identified visitors)
	// or anonymous (anyone).
	WidgetAccess string `json:"widget_access,omitempty"`
}

// Visibility lists the surfaces that read a field. Each is a separate
// switch; Sensitive overrides AI and Reports.
type Visibility struct {
	Agent    bool `json:"agent"`
	AI       bool `json:"ai"`
	Workflow bool `json:"workflow"`
	Reports  bool `json:"reports"`
}

// DefaultVisibility is what a new field gets when the request omits it.
var DefaultVisibility = Visibility{Agent: true, AI: true, Workflow: true, Reports: true}

// Widget write access levels.
const (
	WidgetAccessNone      = "none"
	WidgetAccessVerified  = "verified"
	WidgetAccessAnonymous = "anonymous"
)

// WidgetAccessLevels lists the accepted WidgetAccess values.
var WidgetAccessLevels = []string{WidgetAccessNone, WidgetAccessVerified, WidgetAccessAnonymous}

// IsValidType reports whether t is a supported type.
func IsValidType(t FieldType) bool {
	return slices.Contains(Types, t)
}

// IsValidEntity reports whether e is a supported entity type.
func IsValidEntity(e string) bool {
	return slices.Contains(EntityTypes, e)
}

// IsValidKey reports whether key can name a field (also used as a JSONB key
// and inlined into SQL by the filter builder, so it must stay this strict).
func IsValidKey(key string) bool {
	return keyPattern.MatchString(key)
}

// ValidateDefinition checks one definition on its own. others are the other
// definitions of the same entity (for condition references).
func ValidateDefinition(d Definition, others []Definition) []FieldError {
	var errs []FieldError
	add := func(field, code, msg string) {
		errs = append(errs, FieldError{Field: field, Code: code, Message: msg})
	}
	if !IsValidKey(d.Key) {
		add("key", CodeInvalid, "key must start with a letter and use a-z, 0-9 or _ (max 63)")
	}
	if l := strings.TrimSpace(d.Label); l == "" || len([]rune(l)) > MaxLabelLength {
		add("label", CodeInvalid, fmt.Sprintf("label is required (max %d characters)", MaxLabelLength))
	}
	if !IsValidType(d.Type) {
		add("type", CodeInvalid, "unknown field type")
		return errs
	}
	switch d.Type {
	case TypeSelect, TypeMultiselect:
		if len(d.Options) == 0 || len(d.Options) > MaxOptions {
			add("options", CodeInvalid, fmt.Sprintf("select fields need 1-%d options", MaxOptions))
		}
		seen := map[string]bool{}
		for i, o := range d.Options {
			v := strings.TrimSpace(o.Value)
			if v == "" || len(v) > 100 || strings.TrimSpace(o.Label) == "" {
				add(fmt.Sprintf("options[%d]", i), CodeInvalid, "option value and label are required (value max 100)")
				continue
			}
			if seen[v] {
				add(fmt.Sprintf("options[%d]", i), CodeInvalid, "duplicate option value "+v)
			}
			seen[v] = true
		}
	default:
		if len(d.Options) > 0 {
			add("options", CodeInvalid, "options are only allowed on select fields")
		}
	}
	c := d.Config
	if c.Min != nil && c.Max != nil && *c.Min > *c.Max {
		add("config.min", CodeInvalid, "min must not exceed max")
	}
	if c.Precision != nil && (*c.Precision < 0 || *c.Precision > 6) {
		add("config.precision", CodeInvalid, "precision must be 0-6")
	}
	if c.Currency != "" && !currencyRegex.MatchString(c.Currency) {
		add("config.currency", CodeInvalid, "currency must be an ISO 4217 code")
	}
	if c.MaxLength < 0 || (d.Type == TypeText && c.MaxLength > MaxTextLength) || (d.Type == TypeTextarea && c.MaxLength > MaxTextareaLength) {
		add("config.max_length", CodeInvalid, "max_length is out of range")
	}
	if c.MaxSelected < 0 {
		add("config.max_selected", CodeInvalid, "max_selected must be positive")
	}
	if d.WidgetAccess != "" && !slices.Contains(WidgetAccessLevels, d.WidgetAccess) {
		add("widget_access", CodeInvalid, "widget_access must be none, verified or anonymous")
	}
	if d.Condition != nil {
		errs = append(errs, validateCondition(d, others)...)
	}
	if d.Default != nil && len(errs) == 0 {
		if d.Type == TypeUser {
			add("default", CodeInvalid, "user fields cannot have a default")
		} else if _, err := Normalize(d, d.Default); err != nil {
			add("default", CodeInvalid, "default: "+err.Error())
		}
	}
	return errs
}

func validateCondition(d Definition, others []Definition) []FieldError {
	c := d.Condition
	if c.Field == d.Key {
		return []FieldError{{Field: "condition.field", Code: CodeInvalid, Message: "a field cannot depend on itself"}}
	}
	var ref *Definition
	for i := range others {
		if others[i].Key == c.Field {
			ref = &others[i]
		}
	}
	if ref == nil {
		return []FieldError{{Field: "condition.field", Code: CodeInvalid, Message: "condition refers to an unknown field"}}
	}
	// Reject cycles: walk the condition chain from the referenced field.
	byKey := map[string]Definition{}
	for _, o := range others {
		byKey[o.Key] = o
	}
	seen := map[string]bool{d.Key: true}
	for cur := c.Field; cur != ""; {
		if seen[cur] {
			return []FieldError{{Field: "condition.field", Code: CodeInvalid, Message: "conditions form a cycle"}}
		}
		seen[cur] = true
		next, ok := byKey[cur]
		if !ok || next.Condition == nil {
			break
		}
		cur = next.Condition.Field
	}
	switch c.Operator {
	case CondIsSet, CondIsNotSet:
		return nil
	case CondEquals, CondNotEquals:
		if _, err := Normalize(*ref, c.Value); err != nil {
			return []FieldError{{Field: "condition.value", Code: CodeInvalid, Message: "condition value: " + err.Error()}}
		}
	case CondIn:
		list, ok := c.Value.([]any)
		if !ok || len(list) == 0 {
			return []FieldError{{Field: "condition.value", Code: CodeInvalid, Message: "in needs a non-empty list"}}
		}
		for _, v := range list {
			if _, err := Normalize(*ref, v); err != nil {
				return []FieldError{{Field: "condition.value", Code: CodeInvalid, Message: "condition value: " + err.Error()}}
			}
		}
	default:
		return []FieldError{{Field: "condition.operator", Code: CodeInvalid, Message: "unknown condition operator"}}
	}
	return nil
}
