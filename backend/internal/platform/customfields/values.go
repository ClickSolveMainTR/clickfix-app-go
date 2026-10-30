package customfields

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Error codes in FieldError.Code.
const (
	CodeInvalid  = "invalid"
	CodeRequired = "required"
	CodeUnknown  = "unknown"
	CodeArchived = "archived"
)

// FieldError is one validation failure. Field is the definition key for
// values ("custom_fields.<key>" in API responses).
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationError carries every FieldError of one call.
type ValidationError struct {
	Errors []FieldError
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return "custom fields: validation failed"
	}
	return fmt.Sprintf("custom fields: %s: %s", e.Errors[0].Field, e.Errors[0].Message)
}

// Money is the stored shape of a money value: minor units plus currency.
type Money struct {
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

var (
	phoneStrip   = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", ".", "")
	phonePattern = regexp.MustCompile(`^\+?[0-9]{6,15}$`)
)

// Normalize converts one input value to its stored form or explains why it
// is invalid. nil and "" are not values (callers treat them as "unset").
func Normalize(d Definition, v any) (any, error) {
	switch d.Type {
	case TypeText, TypeTextarea:
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("must be text")
		}
		s = strings.TrimSpace(s)
		limit := d.Config.MaxLength
		if limit == 0 {
			limit = DefaultTextLength
			if d.Type == TypeTextarea {
				limit = MaxTextareaLength
			}
		}
		if len([]rune(s)) > limit {
			return nil, fmt.Errorf("must be at most %d characters", limit)
		}
		return s, nil
	case TypeNumber:
		f, err := toFloat(v)
		if err != nil {
			return nil, err
		}
		if err := checkRange(d.Config, f); err != nil {
			return nil, err
		}
		if p := d.Config.Precision; p != nil {
			scaled := f * math.Pow10(*p)
			if math.Abs(scaled-math.Round(scaled)) > 1e-9 {
				return nil, fmt.Errorf("must have at most %d decimals", *p)
			}
		}
		return f, nil
	case TypeMoney:
		return normalizeMoney(d, v)
	case TypeDate:
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("must be a date (YYYY-MM-DD)")
		}
		s = strings.TrimSpace(s)
		if len(s) > 10 {
			// Accept full timestamps from clients; keep the calendar date.
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return t.Format(time.DateOnly), nil
			}
		}
		t, err := time.Parse(time.DateOnly, s)
		if err != nil {
			return nil, errors.New("must be a date (YYYY-MM-DD)")
		}
		return t.Format(time.DateOnly), nil
	case TypeBoolean:
		switch x := v.(type) {
		case bool:
			return x, nil
		case string:
			b, err := strconv.ParseBool(strings.TrimSpace(x))
			if err != nil {
				return nil, errors.New("must be true or false")
			}
			return b, nil
		}
		return nil, errors.New("must be true or false")
	case TypeSelect:
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("must be one of the options")
		}
		s = strings.TrimSpace(s)
		if !hasOption(d, s, true) {
			return nil, fmt.Errorf("%q is not an option", s)
		}
		return s, nil
	case TypeMultiselect:
		return normalizeMulti(d, v)
	case TypeURL:
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("must be a URL")
		}
		s = strings.TrimSpace(s)
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(s) > MaxURLLength {
			return nil, errors.New("must be an http(s) URL")
		}
		return s, nil
	case TypeEmail:
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("must be an e-mail address")
		}
		s = strings.ToLower(strings.TrimSpace(s))
		addr, err := mail.ParseAddress(s)
		if err != nil || addr.Address != s || len(s) > 254 {
			return nil, errors.New("must be an e-mail address")
		}
		return s, nil
	case TypePhone:
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("must be a phone number")
		}
		s = phoneStrip.Replace(strings.TrimSpace(s))
		if strings.HasPrefix(s, "00") {
			s = "+" + s[2:]
		}
		if !phonePattern.MatchString(s) {
			return nil, errors.New("must be a phone number (6-15 digits, optional +)")
		}
		return s, nil
	case TypeUser:
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("must be a user id")
		}
		id, err := uuid.Parse(strings.TrimSpace(s))
		if err != nil {
			return nil, errors.New("must be a user id")
		}
		return id.String(), nil
	}
	return nil, fmt.Errorf("unsupported type %s", d.Type)
}

func toFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, errors.New("must be a number")
		}
		return x, nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case json.Number:
		return toFloat(string(x))
	case string:
		f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(x), ",", "."), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, errors.New("must be a number")
		}
		return f, nil
	}
	return 0, errors.New("must be a number")
}

func checkRange(c Config, f float64) error {
	if c.Min != nil && f < *c.Min {
		return fmt.Errorf("must be at least %v", *c.Min)
	}
	if c.Max != nil && f > *c.Max {
		return fmt.Errorf("must be at most %v", *c.Max)
	}
	return nil
}

func normalizeMoney(d Definition, v any) (any, error) {
	currency := d.Config.Currency
	if currency == "" {
		currency = "TRY"
	}
	var minor int64
	switch x := v.(type) {
	case map[string]any:
		if c, ok := x["currency"].(string); ok && c != "" {
			currency = strings.ToUpper(strings.TrimSpace(c))
		}
		raw, ok := x["amount_minor"]
		if !ok {
			return nil, errors.New("money needs amount_minor")
		}
		f, err := toFloat(raw)
		if err != nil || f != math.Trunc(f) || math.Abs(f) > 1e15 {
			return nil, errors.New("amount_minor must be a whole number")
		}
		minor = int64(f)
	default:
		// A bare number is an amount in major units (e.g. 12.5 TRY).
		f, err := toFloat(v)
		if err != nil {
			return nil, errors.New("must be an amount")
		}
		scaled := math.Round(f * 100)
		if math.Abs(scaled-f*100) > 1e-6 || math.Abs(scaled) > 1e15 {
			return nil, errors.New("must have at most 2 decimals")
		}
		minor = int64(scaled)
	}
	if !currencyRegex.MatchString(currency) {
		return nil, errors.New("currency must be an ISO 4217 code")
	}
	if err := checkRange(d.Config, float64(minor)/100); err != nil {
		return nil, err
	}
	return map[string]any{"amount_minor": minor, "currency": currency}, nil
}

func normalizeMulti(d Definition, v any) (any, error) {
	var raw []string
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			s, ok := item.(string)
			if !ok {
				return nil, errors.New("must be a list of options")
			}
			raw = append(raw, strings.TrimSpace(s))
		}
	case []string:
		for _, s := range x {
			raw = append(raw, strings.TrimSpace(s))
		}
	case string:
		for s := range strings.SplitSeq(x, ",") {
			if s = strings.TrimSpace(s); s != "" {
				raw = append(raw, s)
			}
		}
	default:
		return nil, errors.New("must be a list of options")
	}
	picked := map[string]bool{}
	for _, s := range raw {
		if !hasOption(d, s, true) {
			return nil, fmt.Errorf("%q is not an option", s)
		}
		picked[s] = true
	}
	// Store in option order so equal sets compare equal.
	out := []any{}
	for _, o := range d.Options {
		if picked[o.Value] {
			out = append(out, o.Value)
		}
	}
	if d.Config.MaxSelected > 0 && len(out) > d.Config.MaxSelected {
		return nil, fmt.Errorf("at most %d options", d.Config.MaxSelected)
	}
	return out, nil
}

func hasOption(d Definition, v string, allowArchived bool) bool {
	return slices.ContainsFunc(d.Options, func(o Option) bool {
		return o.Value == v && (allowArchived || !o.Archived)
	})
}

func isEmpty(v any) bool {
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

// Mode tells Validate whether defaults apply (create) or not (update).
type Mode int

const (
	ModeCreate Mode = iota
	ModeUpdate
)

// Validate merges input into current (stored values, may be nil), normalizes
// every value, applies defaults on create, drops values of fields whose
// condition does not hold and enforces required fields. A nil or empty input
// value clears the field. Keys of archived fields keep their stored value but
// cannot be written. Returns the new stored map or a *ValidationError.
func Validate(defs []Definition, current, input map[string]any, mode Mode) (map[string]any, error) {
	byKey := make(map[string]Definition, len(defs))
	for _, d := range defs {
		byKey[d.Key] = d
	}
	out := make(map[string]any, len(current)+len(input))
	maps.Copy(out, current)
	var errs []FieldError
	for k, v := range input {
		d, ok := byKey[k]
		if !ok {
			errs = append(errs, FieldError{Field: k, Code: CodeUnknown, Message: "unknown field"})
			continue
		}
		if d.Archived {
			errs = append(errs, FieldError{Field: k, Code: CodeArchived, Message: "field is archived"})
			continue
		}
		if isEmpty(v) {
			delete(out, k)
			continue
		}
		norm, err := Normalize(d, v)
		if err != nil {
			errs = append(errs, FieldError{Field: k, Code: CodeInvalid, Message: err.Error()})
			continue
		}
		// A newly picked archived option is rejected; keeping a stored one is fine.
		if (d.Type == TypeSelect || d.Type == TypeMultiselect) && pickedArchived(d, current[k], norm) {
			errs = append(errs, FieldError{Field: k, Code: CodeInvalid, Message: "option is archived"})
			continue
		}
		out[k] = norm
	}
	if mode == ModeCreate {
		for _, d := range defs {
			if _, set := out[d.Key]; set || d.Archived || d.Default == nil {
				continue
			}
			if norm, err := Normalize(d, d.Default); err == nil {
				out[d.Key] = norm
			}
		}
	}
	// Conditions may chain; resolve until stable (definitions are acyclic).
	for range len(defs) + 1 {
		changed := false
		for _, d := range defs {
			if d.Condition == nil {
				continue
			}
			if _, set := out[d.Key]; set && !conditionHolds(d.Condition, byKey, out) {
				delete(out, d.Key)
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	failed := map[string]bool{}
	for _, e := range errs {
		failed[e.Field] = true
	}
	for _, d := range defs {
		if !d.Required || d.Archived || failed[d.Key] {
			continue
		}
		if d.Condition != nil && !conditionHolds(d.Condition, byKey, out) {
			continue
		}
		if _, set := out[d.Key]; !set {
			errs = append(errs, FieldError{Field: d.Key, Code: CodeRequired, Message: "field is required"})
		}
	}
	if len(errs) > 0 {
		slices.SortFunc(errs, func(a, b FieldError) int { return strings.Compare(a.Field, b.Field) })
		return nil, &ValidationError{Errors: errs}
	}
	return out, nil
}

func pickedArchived(d Definition, stored, norm any) bool {
	previous := map[string]bool{}
	switch x := stored.(type) {
	case string:
		previous[x] = true
	case []any:
		for _, s := range x {
			if str, ok := s.(string); ok {
				previous[str] = true
			}
		}
	}
	var values []string
	switch x := norm.(type) {
	case string:
		values = []string{x}
	case []any:
		for _, s := range x {
			values = append(values, s.(string))
		}
	}
	for _, v := range values {
		if !previous[v] && !hasOption(d, v, false) {
			return true
		}
	}
	return false
}

// ConditionHolds evaluates a definition's condition against stored values.
// Fields without a condition always hold.
func ConditionHolds(d Definition, defs []Definition, values map[string]any) bool {
	if d.Condition == nil {
		return true
	}
	byKey := make(map[string]Definition, len(defs))
	for _, x := range defs {
		byKey[x.Key] = x
	}
	return conditionHolds(d.Condition, byKey, values)
}

func conditionHolds(c *Condition, byKey map[string]Definition, values map[string]any) bool {
	got, set := values[c.Field]
	ref, known := byKey[c.Field]
	switch c.Operator {
	case CondIsSet:
		return set
	case CondIsNotSet:
		return !set
	}
	if !set || !known {
		return c.Operator == CondNotEquals
	}
	matches := func(want any) bool {
		norm, err := Normalize(ref, want)
		if err != nil {
			return false
		}
		if list, ok := got.([]any); ok {
			// Multiselect: the condition value must be among the picked ones.
			if s, ok := norm.(string); ok {
				return slices.Contains(list, any(s))
			}
			if wantList, ok := norm.([]any); ok {
				for _, w := range wantList {
					if !slices.Contains(list, w) {
						return false
					}
				}
				return true
			}
			return false
		}
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(norm)
		return string(a) == string(b)
	}
	switch c.Operator {
	case CondEquals:
		return matches(c.Value)
	case CondNotEquals:
		return !matches(c.Value)
	case CondIn:
		list, _ := c.Value.([]any)
		for _, v := range list {
			if matches(v) {
				return true
			}
		}
	}
	return false
}

// UserIDs returns the user UUIDs referenced by user fields in values; the
// caller checks they are members of the organization.
func UserIDs(defs []Definition, values map[string]any) []uuid.UUID {
	var out []uuid.UUID
	for _, d := range defs {
		if d.Type != TypeUser {
			continue
		}
		if s, ok := values[d.Key].(string); ok {
			if id, err := uuid.Parse(s); err == nil {
				out = append(out, id)
			}
		}
	}
	return out
}
