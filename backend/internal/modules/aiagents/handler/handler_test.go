package handler

import (
	"encoding/json"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

// JSONB columns must reach clients as objects, not base64 strings.
func TestViewsKeepJSONBObjects(t *testing.T) {
	raw, err := json.Marshal(viewProcedure(db.AiProcedure{Intent: "refund", Draft: []byte(`{"steps":["Sipariş no sor"]}`), Revision: 1}))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Draft     map[string]any `json:"draft"`
		Published *struct{}      `json:"published"`
		Intent    string         `json:"intent"`
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("procedure view is not an object: %v (%s)", err, raw)
	}
	if out.Intent != "refund" || out.Draft["steps"] == nil || out.Published != nil {
		t.Fatalf("unexpected procedure view %s", raw)
	}
	raw, err = json.Marshal(toolView{AIToolsRow: db.AIToolsRow{Name: "order_lookup"}, Schema: rawJSON([]byte(`{"type":"object"}`))})
	if err != nil {
		t.Fatal(err)
	}
	var tool struct {
		Schema map[string]any `json:"schema"`
		Name   string         `json:"name"`
	}
	if err = json.Unmarshal(raw, &tool); err != nil || tool.Schema["type"] != "object" || tool.Name != "order_lookup" {
		t.Fatalf("tool view lost schema object: %v (%s)", err, raw)
	}
}
