// Package engine is the pure logic of dynamic request forms (CF-52): which
// fields a form shows, how a submission is validated and split into the
// contact, the conversation and their custom fields, and which conversation
// fields still block resolving it.
//
// Field values are validated with the CF-7 custom field engine
// (platform/customfields): system fields get built-in definitions, custom
// fields reuse their organization definition with the form's own required
// flag and condition. Form keys are the system key ("subject") or
// "<source>.<definition key>" ("conversation.order_no").
package engine

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/google/uuid"
)

// Field sources.
const (
	SourceSystem       = "system"
	SourceConversation = "conversation"
	SourceContact      = "contact"
)

// System field keys.
const (
	KeyName     = "name"
	KeyEmail    = "email"
	KeyPhone    = "phone"
	KeySubject  = "subject"
	KeyMessage  = "message"
	KeyPriority = "priority"
)

// SystemKeys in display order.
var SystemKeys = []string{KeyName, KeyEmail, KeyPhone, KeySubject, KeyMessage, KeyPriority}

// MaxFields bounds a form.
const MaxFields = 40

// ErrInvalid marks an invalid form definition.
var ErrInvalid = errors.New("forms: invalid form")

// Field is one stored form field.
type Field struct {
	Key               string                  `json:"key"`
	Source            string                  `json:"source"`
	CustomFieldUUID   *uuid.UUID              `json:"custom_field_uuid,omitempty"`
	Label             string                  `json:"label"`
	Help              string                  `json:"help"`
	Required          bool                    `json:"required"`
	RequiredToResolve bool                    `json:"required_to_resolve"`
	Condition         *customfields.Condition `json:"condition,omitempty"`
}

// Custom is an organization custom field a form may reference.
type Custom struct {
	UUID   uuid.UUID
	Entity string // conversation or contact
	Def    customfields.Definition
}

// Resolved is a form field with its engine definition.
type Resolved struct {
	Field
	// Def is keyed by the form key, with the form's label, required flag
	// and condition.
	Def customfields.Definition
	// StoredKey is the key in the owner's custom_fields (custom fields only).
	StoredKey string
}

// SystemDefinition is the built-in definition of a system field.
func SystemDefinition(key string) (customfields.Definition, bool) {
	switch key {
	case KeyName:
		return customfields.Definition{Key: key, Label: "Name", Type: customfields.TypeText, Config: customfields.Config{MaxLength: 120}}, true
	case KeyEmail:
		return customfields.Definition{Key: key, Label: "E-mail", Type: customfields.TypeEmail}, true
	case KeyPhone:
		return customfields.Definition{Key: key, Label: "Phone", Type: customfields.TypePhone}, true
	case KeySubject:
		return customfields.Definition{Key: key, Label: "Subject", Type: customfields.TypeText, Config: customfields.Config{MaxLength: 200}}, true
	case KeyMessage:
		return customfields.Definition{Key: key, Label: "Message", Type: customfields.TypeTextarea, Config: customfields.Config{MaxLength: 5000}}, true
	case KeyPriority:
		return customfields.Definition{Key: key, Label: "Priority", Type: customfields.TypeSelect, Options: []customfields.Option{
			{Value: "low", Label: "Low"}, {Value: "normal", Label: "Normal"},
			{Value: "high", Label: "High"}, {Value: "urgent", Label: "Urgent"},
		}}, true
	}
	return customfields.Definition{}, false
}

// FormKey is the form key of a custom field.
func FormKey(source, defKey string) string { return source + "." + defKey }

// Resolve checks stored fields against the organization's custom fields and
// returns them with their definitions. Fields of archived or deleted custom
// fields are dropped (strict=false) or rejected (strict=true, on save).
func Resolve(fields []Field, customs []Custom, strict bool) ([]Resolved, error) {
	if len(fields) > MaxFields {
		return nil, fmt.Errorf("%w: at most %d fields", ErrInvalid, MaxFields)
	}
	byUUID := make(map[uuid.UUID]Custom, len(customs))
	for _, c := range customs {
		byUUID[c.UUID] = c
	}
	out := make([]Resolved, 0, len(fields))
	seen := map[string]bool{}
	for i, f := range fields {
		var def customfields.Definition
		var stored string
		switch f.Source {
		case SourceSystem:
			d, ok := SystemDefinition(f.Key)
			if !ok {
				return nil, fmt.Errorf("%w: unknown system field %q", ErrInvalid, f.Key)
			}
			def = d
			f.CustomFieldUUID = nil
			f.RequiredToResolve = false
		case SourceConversation, SourceContact:
			if f.CustomFieldUUID == nil {
				return nil, fmt.Errorf("%w: field %d needs custom_field_uuid", ErrInvalid, i+1)
			}
			c, ok := byUUID[*f.CustomFieldUUID]
			if !ok || c.Entity != f.Source || c.Def.Archived {
				if strict {
					return nil, fmt.Errorf("%w: field %d is not an active %s custom field", ErrInvalid, i+1, f.Source)
				}
				continue
			}
			def, stored = c.Def, c.Def.Key
			f.Key = FormKey(f.Source, c.Def.Key)
			if f.Source != SourceConversation {
				f.RequiredToResolve = false
			}
		default:
			return nil, fmt.Errorf("%w: unknown source %q", ErrInvalid, f.Source)
		}
		if seen[f.Key] {
			return nil, fmt.Errorf("%w: field %s appears twice", ErrInvalid, f.Key)
		}
		f.Label = strings.TrimSpace(f.Label)
		f.Help = strings.TrimSpace(f.Help)
		if len([]rune(f.Label)) > 120 || len([]rune(f.Help)) > 300 {
			return nil, fmt.Errorf("%w: label or help of %s is too long", ErrInvalid, f.Key)
		}
		if f.Condition != nil {
			if err := checkCondition(f, out); err != nil {
				if strict {
					return nil, err
				}
				f.Condition = nil
			}
		}
		seen[f.Key] = true
		def.Key = f.Key
		if f.Label != "" {
			def.Label = f.Label
		}
		def.Required = f.Required
		def.Condition = f.Condition
		def.Default = nil
		def.Archived = false
		out = append(out, Resolved{Field: f, Def: def, StoredKey: stored})
	}
	return out, nil
}

// checkCondition: a condition refers to an earlier field (no cycles) with a
// valid operator.
func checkCondition(f Field, earlier []Resolved) error {
	c := f.Condition
	i := slices.IndexFunc(earlier, func(r Resolved) bool { return r.Key == c.Field })
	if i < 0 {
		return fmt.Errorf("%w: condition of %s must refer to an earlier field", ErrInvalid, f.Key)
	}
	switch c.Operator {
	case customfields.CondEquals, customfields.CondNotEquals, customfields.CondIn:
		if c.Value == nil {
			return fmt.Errorf("%w: condition of %s needs a value", ErrInvalid, f.Key)
		}
	case customfields.CondIsSet, customfields.CondIsNotSet:
	default:
		return fmt.Errorf("%w: unknown condition operator %q", ErrInvalid, c.Operator)
	}
	return nil
}

// Submission is a validated form submission split by owner.
type Submission struct {
	Name, Email, Phone, Subject, Message, Priority string
	// Conversation and Contact are custom field values keyed by the
	// definition key (the owner's custom_fields keys).
	Conversation map[string]any
	Contact      map[string]any
}

// Check validates values (keyed by form key) against the form. Keys in
// optional are never required (the widget's message comes from its
// composer). Returns a *customfields.ValidationError on invalid input.
func Check(fields []Resolved, values map[string]any, optional ...string) (Submission, error) {
	defs := make([]customfields.Definition, 0, len(fields))
	for _, f := range fields {
		d := f.Def
		if slices.Contains(optional, f.Key) {
			d.Required = false
		}
		defs = append(defs, d)
	}
	out, err := customfields.Validate(defs, nil, values, customfields.ModeCreate)
	if err != nil {
		return Submission{}, err
	}
	sub := Submission{Conversation: map[string]any{}, Contact: map[string]any{}}
	str := func(k string) string {
		s, _ := out[k].(string)
		return s
	}
	for _, f := range fields {
		v, ok := out[f.Key]
		if !ok {
			continue
		}
		switch f.Source {
		case SourceConversation:
			sub.Conversation[f.StoredKey] = v
		case SourceContact:
			sub.Contact[f.StoredKey] = v
		}
	}
	sub.Name, sub.Email, sub.Phone = str(KeyName), str(KeyEmail), str(KeyPhone)
	sub.Subject, sub.Message, sub.Priority = str(KeySubject), str(KeyMessage), str(KeyPriority)
	return sub, nil
}

// MissingToResolve returns the conversation fields marked required to
// resolve whose condition holds but which have no value in stored (the
// conversation's custom_fields).
func MissingToResolve(fields []Resolved, stored map[string]any) []Resolved {
	values := map[string]any{}
	defs := make([]customfields.Definition, 0, len(fields))
	for _, f := range fields {
		defs = append(defs, f.Def)
		if f.Source == SourceConversation {
			if v, ok := stored[f.StoredKey]; ok {
				values[f.Key] = v
			}
		}
	}
	var out []Resolved
	for _, f := range fields {
		if !f.RequiredToResolve || f.Source != SourceConversation {
			continue
		}
		if f.Condition != nil && !customfields.ConditionHolds(f.Def, defs, values) {
			continue
		}
		if empty(values[f.Key]) {
			out = append(out, f)
		}
	}
	return out
}

func empty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []any:
		return len(x) == 0
	case []string:
		return len(x) == 0
	}
	return false
}
