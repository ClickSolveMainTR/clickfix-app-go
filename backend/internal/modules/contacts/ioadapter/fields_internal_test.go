package ioadapter

import (
	"context"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
)

type stubFields []customfields.Definition

func (s stubFields) Definitions(context.Context, int64, string, *int64) ([]customfields.Definition, error) {
	return s, nil
}

// CF-196: exports carry reportable fields only; a sensitive field or one
// hidden from reports never becomes a column or a cell.
func TestExportSkipsSensitiveFields(t *testing.T) {
	src := stubFields{
		{Key: "paket", Label: "Paket", Type: customfields.TypeSelect, Visibility: customfields.DefaultVisibility,
			Options: []customfields.Option{{Value: "pro", Label: "Pro Paket"}}},
		{Key: "tc_kimlik", Label: "TC kimlik", Type: customfields.TypeText, Visibility: customfields.DefaultVisibility, Sensitive: true},
		{Key: "ic_not", Label: "İç not", Type: customfields.TypeText, Visibility: customfields.Visibility{Agent: true, AI: true}},
	}
	defs, err := reportFields(context.Background(), src, 1, customfields.EntityContact)
	if err != nil {
		t.Fatal(err)
	}
	cols := fieldColumns(defs)
	if len(cols) != 1 || cols[0].Key != "cf.paket" || cols[0].LabelKey != "Paket" {
		t.Fatalf("columns = %+v", cols)
	}
	row := map[string]any{}
	fieldCells(row, defs, map[string]any{"paket": "pro", "tc_kimlik": "12345678901", "ic_not": "x"})
	if row["cf.paket"] != "Pro Paket" || len(row) != 1 {
		t.Fatalf("row = %v", row)
	}
	if defs, _ := reportFields(context.Background(), nil, 1, customfields.EntityContact); defs != nil {
		t.Fatal("no field source should add no columns")
	}
}
