package capability

import (
	"context"
	"regexp"
)

var uuidRxp = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Schema helpers shared by module capabilities (CF-211). Descriptions are
// kept short on purpose: every word is sent to the model on every turn the
// tool is loaded.

// Object is an input schema with additionalProperties false.
func Object(required []string, props map[string]any) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": props}
}

// UUID is a UUID string field.
func UUID(desc string) map[string]any {
	f := map[string]any{"type": "string", "pattern": "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"}
	if desc != "" {
		f["description"] = desc
	}
	return f
}

// Text is a string field with a maximum length.
func Text(max int, desc string) map[string]any {
	f := map[string]any{"type": "string", "maxLength": max}
	if desc != "" {
		f["description"] = desc
	}
	return f
}

// Enum is a string field with fixed values.
func Enum(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

// Bool is a boolean field.
func Bool(desc string) map[string]any {
	f := map[string]any{"type": "boolean"}
	if desc != "" {
		f["description"] = desc
	}
	return f
}

// Int is an integer field in [min, max].
func Int(min, max int) map[string]any {
	return map[string]any{"type": "integer", "minimum": min, "maximum": max}
}

// Limit and Offset page a list (the API caps limit at 100).
var (
	Limit  = Int(1, 100)
	Offset = map[string]any{"type": "integer", "minimum": 0}
)

// Paged adds limit/offset to list properties.
func Paged(props map[string]any) map[string]any {
	props["limit"] = Limit
	props["offset"] = Offset
	return props
}

// Titled sets the card form label of a field (stripped from the model's schema).
func Titled(title string, f map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range f {
		out[k] = v
	}
	out["title"] = title
	return out
}

// CheckUUIDs validates UUID params that are set.
func CheckUUIDs(p Params, keys ...string) error {
	for _, k := range keys {
		if v, ok := p[k].(string); ok && v != "" && !uuidRxp.MatchString(v) {
			return Invalid("%s must be a UUID", k)
		}
	}
	return nil
}

// T is a tr/en card title.
func T(tr, en string) map[string]string { return map[string]string{"tr": tr, "en": en} }

// Get is a read capability's Run for one GET route.
func Get(path string, query ...string) func(ctx context.Context, e Exec, p Params) (Result, error) {
	return Endpoint{Method: "GET", Path: path, Query: query}.Call
}
