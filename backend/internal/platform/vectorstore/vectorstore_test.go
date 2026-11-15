package vectorstore

import (
	"math"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestVectorTextRoundTrip(t *testing.T) {
	v := Vector{1, -0.5, 0.25}
	raw, err := v.Value()
	if err != nil || raw != "[1,-0.5,0.25]" {
		t.Fatalf("value %v %v", raw, err)
	}
	var back Vector
	if err = back.Scan([]byte("[1, -0.5,0.25]")); err != nil || len(back) != 3 || back[1] != -0.5 {
		t.Fatalf("scan %v %v", back, err)
	}
	if raw, _ = Vector(nil).Value(); raw != nil {
		t.Fatal("nil vector must be NULL")
	}
	if err = back.Scan(nil); err != nil || back != nil {
		t.Fatal("NULL scan")
	}
	if back.Scan("1,2") == nil {
		t.Fatal("malformed accepted")
	}
}

func TestCheckFixedDimension(t *testing.T) {
	good := make([]float32, ai.EmbeddingDimensions)
	good[3] = 1
	if Check(good) != nil {
		t.Fatal("valid vector rejected")
	}
	if Check(good[:10]) != ErrDimension {
		t.Fatal("short vector accepted")
	}
	if Check(make([]float32, ai.EmbeddingDimensions)) == nil {
		t.Fatal("zero vector accepted")
	}
	good[0] = float32(math.NaN())
	if Check(good) == nil {
		t.Fatal("NaN accepted")
	}
}

func TestScopePredicate(t *testing.T) {
	brand := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	args := pgx.NamedArgs{}
	where, specific := Conversation(7, brand, pgtype.UUID{}).predicate("v", args)
	if !strings.Contains(where, "v.organization_id=@scope_org") || !strings.Contains(where, "(v.brand_id=@scope_brand OR v.brand_id IS NULL)") ||
		!strings.Contains(where, "v.department_id IS NULL") || specific != "(v.brand_id IS NOT NULL)" || args["scope_org"] != int64(7) {
		t.Fatalf("conversation scope %q %q %v", where, specific, args)
	}
	where, specific = Conversation(7, pgtype.UUID{}, pgtype.UUID{}).predicate("v", pgx.NamedArgs{})
	if !strings.Contains(where, "v.brand_id IS NULL") || strings.Contains(where, "@scope_brand") || specific != "false" {
		t.Fatalf("brandless scope %q %q", where, specific)
	}
	where, specific = Organization(7).predicate("v", pgx.NamedArgs{})
	if where != "v.organization_id=@scope_org" || specific != "false" {
		t.Fatalf("organization scope %q %q", where, specific)
	}
}

func TestSourceValidation(t *testing.T) {
	if (Source{Table: "knowledge_chunks; DROP", TextColumn: "content"}).valid() == nil {
		t.Fatal("unsafe table accepted")
	}
}

func TestTableValidation(t *testing.T) {
	good := Table{Name: "x", Table: "history_cases", Vectors: []Column{{Column: "embedding", Text: "v.problem"}}}
	if good.valid() != nil {
		t.Fatal("valid table rejected")
	}
	for name, bad := range map[string]Table{
		"no vectors":   {Name: "x", Table: "t"},
		"two kinds":    {Name: "x", Table: "t", Vectors: good.Vectors, Centroid: &Centroid{Column: "c", Members: "m", Key: "k", Vector: "e"}},
		"unsafe table": {Name: "x", Table: "t; DROP", Vectors: good.Vectors},
		"unsafe child": {Name: "x", Table: "t", Child: &Child{Table: "c", Key: "k;", Column: "e", Text: "c.content"}},
		"no text":      {Name: "x", Table: "t", Vectors: []Column{{Column: "embedding"}}},
	} {
		if bad.valid() == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	centroid := Table{Name: "c", Table: "history_clusters", Centroid: &Centroid{Column: "centroid", Members: "history_cases", Key: "cluster_id", Vector: "embedding"}}
	if centroid.valid() != nil || !strings.Contains(centroid.staleRow(), "<>''") {
		t.Fatal("centroid table: invalid rows must wait for members")
	}
}
