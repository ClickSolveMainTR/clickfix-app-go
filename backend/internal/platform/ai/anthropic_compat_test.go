package ai

import (
	"strings"
	"testing"
)

func TestAnthropicSchemaDropsUnsupportedKeywords(t *testing.T) {
	in := map[string]any{"type": "object", "required": []string{"confidence", "tags", "one"}, "properties": map[string]any{
		"confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
		"tags":       map[string]any{"type": "array", "maxItems": 10, "minItems": 2, "items": map[string]any{"type": "string", "maxLength": 100}},
		"one":        map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string"}},
	}}
	out := anthropicSchema(in)
	props := out["properties"].(map[string]any)
	conf := props["confidence"].(map[string]any)
	if _, ok := conf["minimum"]; ok {
		t.Fatal("minimum must be dropped")
	}
	tags := props["tags"].(map[string]any)
	if _, ok := tags["maxItems"]; ok {
		t.Fatal("maxItems must be dropped")
	}
	if _, ok := tags["minItems"]; ok {
		t.Fatal("minItems > 1 must be dropped")
	}
	if tags["items"].(map[string]any)["maxLength"] != nil {
		t.Fatal("maxLength must be validated locally")
	}
	if props["one"].(map[string]any)["minItems"] != float64(1) {
		t.Fatal("minItems 1 must stay")
	}
	if _, ok := in["properties"].(map[string]any)["confidence"].(map[string]any)["minimum"]; !ok {
		t.Fatal("input schema must not be mutated (local validation uses it)")
	}
}

func TestNoAssistantPrefill(t *testing.T) {
	if validateRequest(Request{Turns: []Turn{{Role: "assistant", Text: "prefix"}}}) == nil {
		t.Fatal("assistant prefill must be rejected locally")
	}
	req := Request{Turns: []Turn{{Role: "user", Text: "data"}}}
	_, body := NewAnthropic("fake").body(req, false)
	if len(body["messages"].([]any)) != 1 {
		t.Fatal("synthetic continuation added")
	}
}

func TestRejectionDetail(t *testing.T) {
	d := rejectionDetail(strings.NewReader(`{"type":"error","error":{"type":"invalid_request_error","message":"bad schema"}}`))
	if d != "invalid_request_error: bad schema" {
		t.Fatalf("got %q", d)
	}
	if rejectionDetail(strings.NewReader("not json")) != "" {
		t.Fatal("non-JSON body must give empty detail")
	}
}

func TestStrictSchemaNestedOptionalAndReferences(t *testing.T) {
	schema := map[string]any{
		"type": "object", "required": []string{"minimum", "nested"},
		"properties": map[string]any{
			"minimum":  map[string]any{"type": "number", "minimum": 0},
			"optional": map[string]any{"type": "string", "minLength": 2, "pattern": "^ok"},
			"nested":   map[string]any{"type": "array", "minItems": int64(2), "items": map[string]any{"$ref": "#/$defs/child"}},
		},
		"$defs": map[string]any{"child": map[string]any{"type": "object", "properties": map[string]any{"label": map[string]any{"type": "string"}}}},
	}
	wire, err := structuredSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	props := wire["properties"].(map[string]any)
	if wire["additionalProperties"] != false || len(wire["required"].([]string)) != 3 {
		t.Fatal("object must be strict")
	}
	if _, ok := props["optional"].(map[string]any)["anyOf"]; !ok {
		t.Fatal("optional must be nullable and required")
	}
	nested := props["nested"].(map[string]any)
	if nested["minItems"] != nil || nested["items"].(map[string]any)["additionalProperties"] != false {
		t.Fatal("nested schema not transformed")
	}
	if props["minimum"].(map[string]any)["minimum"] != nil {
		t.Fatal("numeric constraint not stripped")
	}
	if ValidateJSON(schema, []byte(`{"minimum":-1,"nested":[]}`)) == nil {
		t.Fatal("original bounds lost")
	}
	if ValidateJSON(map[string]any{"type": "object", "properties": map[string]any{"label": map[string]any{"type": "string", "minLength": 2}}}, []byte(`{"label":null}`)) != nil {
		t.Fatal("optional null must mean absent")
	}
	recursive := map[string]any{"type": "object", "properties": map[string]any{"self": map[string]any{"$ref": "#/$defs/node"}}, "$defs": map[string]any{"node": map[string]any{"$ref": "#/$defs/node"}}}
	if _, err = structuredSchema(recursive); err == nil {
		t.Fatal("recursive schema accepted")
	}
}
