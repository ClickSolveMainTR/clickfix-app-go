package customfields

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
)

// Operator is a filter operator.
type Operator string

const (
	OpEq       Operator = "eq"
	OpNeq      Operator = "neq"
	OpIn       Operator = "in"
	OpContains Operator = "contains"
	OpGt       Operator = "gt"
	OpGte      Operator = "gte"
	OpLt       Operator = "lt"
	OpLte      Operator = "lte"
	OpSet      Operator = "set"
)

// QueryPrefix marks custom field filters and sorts in list queries:
// `cf.<key>=v`, `cf.<key>.<op>=v`, `sort=-cf.<key>`.
const QueryPrefix = "cf."

// Operators returns the operators a type supports.
func Operators(t FieldType) []Operator {
	switch t {
	case TypeNumber, TypeMoney, TypeDate:
		return []Operator{OpEq, OpNeq, OpIn, OpGt, OpGte, OpLt, OpLte, OpSet}
	case TypeText, TypeTextarea, TypeURL, TypeEmail, TypePhone:
		return []Operator{OpEq, OpNeq, OpIn, OpContains, OpSet}
	case TypeMultiselect:
		return []Operator{OpEq, OpIn, OpSet}
	case TypeBoolean:
		return []Operator{OpEq, OpSet}
	default: // select, user
		return []Operator{OpEq, OpNeq, OpIn, OpSet}
	}
}

// Filter is one parsed custom field condition.
type Filter struct {
	Def    Definition
	Op     Operator
	Values []any // normalized values; one bool for OpSet
}

// ParseFilters reads `cf.*` parameters. Several values (repeated params or
// comma separated) are only allowed with in; for multiselect eq means "has
// all of", in means "has any of".
func ParseFilters(q url.Values, defs []Definition) ([]Filter, error) {
	byKey := make(map[string]Definition, len(defs))
	for _, d := range defs {
		byKey[d.Key] = d
	}
	params := make([]string, 0)
	for name := range q {
		if strings.HasPrefix(name, QueryPrefix) {
			params = append(params, name)
		}
	}
	sort.Strings(params)
	var out []Filter
	var errs []FieldError
	for _, name := range params {
		rest := strings.TrimPrefix(name, QueryPrefix)
		key, op := rest, OpEq
		if i := strings.LastIndex(rest, "."); i > 0 {
			key, op = rest[:i], Operator(rest[i+1:])
		}
		d, ok := byKey[key]
		if !ok {
			errs = append(errs, FieldError{Field: name, Code: CodeUnknown, Message: "unknown custom field"})
			continue
		}
		if !slices.Contains(Operators(d.Type), op) {
			errs = append(errs, FieldError{Field: name, Code: CodeInvalid, Message: fmt.Sprintf("operator %s is not supported for %s fields", op, d.Type)})
			continue
		}
		var raw []string
		for _, v := range q[name] {
			if op == OpIn || (d.Type == TypeMultiselect && op == OpEq) {
				for _, part := range strings.Split(v, ",") {
					if part = strings.TrimSpace(part); part != "" {
						raw = append(raw, part)
					}
				}
				continue
			}
			raw = append(raw, v)
		}
		if len(raw) == 0 || (len(raw) > 1 && op != OpIn && d.Type != TypeMultiselect) {
			errs = append(errs, FieldError{Field: name, Code: CodeInvalid, Message: "expects exactly one value (use .in for lists)"})
			continue
		}
		f := Filter{Def: d, Op: op}
		bad := false
		for _, v := range raw {
			norm, err := filterValue(d, op, v)
			if err != nil {
				errs = append(errs, FieldError{Field: name, Code: CodeInvalid, Message: err.Error()})
				bad = true
				break
			}
			f.Values = append(f.Values, norm)
		}
		if !bad {
			out = append(out, f)
		}
	}
	if len(errs) > 0 {
		return nil, &ValidationError{Errors: errs}
	}
	return out, nil
}

func filterValue(d Definition, op Operator, v string) (any, error) {
	switch op {
	case OpSet:
		return Normalize(Definition{Type: TypeBoolean}, v)
	case OpContains:
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("contains needs text")
		}
		return strings.TrimSpace(v), nil
	}
	switch d.Type {
	case TypeMultiselect:
		return Normalize(Definition{Type: TypeSelect, Options: d.Options}, v)
	case TypeMoney:
		// Filters compare amounts in major units of any currency.
		f, err := toFloat(v)
		if err != nil {
			return nil, err
		}
		return int64(f*100 + 0.5*sign(f)), nil
	case TypeText, TypeTextarea:
		return strings.TrimSpace(v), nil
	}
	// Range filters must not be bounded by the field's own min/max.
	d.Config = Config{Currency: d.Config.Currency}
	return Normalize(d, v)
}

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

// SQL builds a WHERE fragment for filters on column (a trusted identifier
// such as "c.custom_fields"). Placeholders start at $next; the returned args
// follow in order. Equality uses @> so the jsonb_path_ops GIN index applies.
func SQL(column string, filters []Filter, next int) (string, []any) {
	var parts []string
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", next+len(args)-1)
	}
	contains := func(key string, v any) string {
		b, _ := json.Marshal(map[string]any{key: v})
		return fmt.Sprintf("%s @> %s::jsonb", column, arg(string(b)))
	}
	for _, f := range filters {
		key := f.Def.Key // validated by IsValidKey; safe to inline
		path := fmt.Sprintf("%s->>'%s'", column, key)
		switch f.Op {
		case OpSet:
			expr := fmt.Sprintf("%s ? %s", column, arg(key))
			if set, _ := f.Values[0].(bool); !set {
				expr = "NOT (" + expr + ")"
			}
			parts = append(parts, expr)
		case OpContains:
			parts = append(parts, fmt.Sprintf("%s ILIKE %s", path, arg("%"+escapeLike(f.Values[0].(string))+"%")))
		case OpEq, OpNeq, OpIn:
			var expr string
			switch {
			case f.Def.Type == TypeMultiselect && f.Op == OpEq:
				expr = contains(key, f.Values)
			case f.Def.Type == TypeMultiselect:
				strs := make([]string, len(f.Values))
				for i, v := range f.Values {
					strs[i] = v.(string)
				}
				expr = fmt.Sprintf("(%s->'%s') ?| %s::text[]", column, key, arg(strs))
			case f.Def.Type == TypeMoney:
				ors := make([]string, len(f.Values))
				for i, v := range f.Values {
					ors[i] = contains(key, map[string]any{"amount_minor": v})
				}
				expr = "(" + strings.Join(ors, " OR ") + ")"
			default:
				ors := make([]string, len(f.Values))
				for i, v := range f.Values {
					ors[i] = contains(key, v)
				}
				expr = "(" + strings.Join(ors, " OR ") + ")"
			}
			if f.Op == OpNeq {
				expr = "NOT " + expr
			}
			parts = append(parts, expr)
		case OpGt, OpGte, OpLt, OpLte:
			cmp := map[Operator]string{OpGt: ">", OpGte: ">=", OpLt: "<", OpLte: "<="}[f.Op]
			left, cast := sortable(column, f.Def)
			parts = append(parts, fmt.Sprintf("%s %s %s::%s", left, cmp, arg(f.Values[0]), cast))
		}
	}
	return strings.Join(parts, " AND "), args
}

// sortable returns a typed expression for ordering and range comparisons.
func sortable(column string, d Definition) (expr, cast string) {
	key := d.Key
	switch d.Type {
	case TypeNumber:
		return fmt.Sprintf("(CASE WHEN jsonb_typeof(%s->'%s') = 'number' THEN (%s->>'%s')::numeric END)", column, key, column, key), "numeric"
	case TypeMoney:
		return fmt.Sprintf("((%s->'%s'->>'amount_minor')::bigint)", column, key), "bigint"
	case TypeDate:
		return fmt.Sprintf("((%s->>'%s')::date)", column, key), "date"
	case TypeBoolean:
		return fmt.Sprintf("((%s->>'%s')::boolean)", column, key), "boolean"
	default:
		return fmt.Sprintf("(%s->>'%s')", column, key), "text"
	}
}

// SortExpr returns the ORDER BY expression for `sort=cf.<key>`; ok is false
// when field is not a custom field sort or the key is unknown. Multiselect
// fields are not sortable.
func SortExpr(column, field string, defs []Definition) (expr string, ok bool) {
	key, isCF := strings.CutPrefix(field, QueryPrefix)
	if !isCF {
		return "", false
	}
	for _, d := range defs {
		if d.Key == key && d.Type != TypeMultiselect {
			e, _ := sortable(column, d)
			return e, true
		}
	}
	return "", false
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
