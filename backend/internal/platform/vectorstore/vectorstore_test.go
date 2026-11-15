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
	where, specific := Conversation(7, brand, pgtype.Int8{}).predicate("v", "", args)
	if !strings.Contains(where, "v.organization_id=@scope_org") || !strings.Contains(where, "(v.brand_id=@scope_brand OR v.brand_id IS NULL)") ||
		!strings.Contains(where, "(v.department_id IS NULL OR v.department_id=ANY(@scope_departments::bigint[]))") ||
		specific != "(v.brand_id IS NOT NULL)" || args["scope_org"] != int64(7) || len(args["scope_departments"].([]int64)) != 0 {
		t.Fatalf("conversation scope %q %q %v", where, specific, args)
	}
	where, specific = Conversation(7, pgtype.UUID{}, pgtype.Int8{}).predicate("v", "", pgx.NamedArgs{})
	if !strings.Contains(where, "v.brand_id IS NULL") || strings.Contains(where, "@scope_brand") || specific != "false" {
		t.Fatalf("brandless scope %q %q", where, specific)
	}
	where, specific = Organization(7).predicate("v", "", pgx.NamedArgs{})
	if where != "v.organization_id=@scope_org" || specific != "false" {
		t.Fatalf("organization scope %q %q", where, specific)
	}
}

// CF-222: a department scope sees its own rows and organization-wide rows,
// ranks its own rows first and uses the source's rule (shares) when given.
func TestScopeDepartmentPredicate(t *testing.T) {
	args := pgx.NamedArgs{}
	where, specific := Conversation(7, pgtype.UUID{}, pgtype.Int8{Int64: 42, Valid: true}).predicate("v", "", args)
	if !strings.Contains(where, "v.department_id=ANY(@scope_departments::bigint[])") ||
		specific != "((v.department_id IS NOT NULL AND v.department_id=@scope_department))" ||
		args["scope_department"] != int64(42) || args["scope_departments"].([]int64)[0] != 42 {
		t.Fatalf("department scope %q %q %v", where, specific, args)
	}
	rule := "department_visible(v.organization_id,'article',v.article_id,v.department_id,@scope_departments::bigint[])"
	where, _ = Conversation(7, pgtype.UUID{}, pgtype.Int8{Int64: 42, Valid: true}).predicate("v", rule, pgx.NamedArgs{})
	if !strings.Contains(where, " AND "+rule) || strings.Contains(where, "ANY(@scope_departments::bigint[]))") {
		t.Fatalf("source rule %q", where)
	}
	// A viewer that sees every department drops the filter but keeps ranking.
	args = pgx.NamedArgs{}
	where, specific = Conversation(7, pgtype.UUID{}, pgtype.Int8{Int64: 42, Valid: true}).WithDepartments(false, nil).predicate("v", rule, args)
	if strings.Contains(where, "department") || !strings.Contains(specific, "@scope_department") {
		t.Fatalf("all departments %q %q", where, specific)
	}
	// Organization scope narrowed to a member's departments.
	args = pgx.NamedArgs{}
	where, specific = Organization(7).WithDepartments(true, []int64{3, 4}).predicate("v", "", args)
	if !strings.Contains(where, "@scope_departments") || specific != "false" || len(args["scope_departments"].([]int64)) != 2 {
		t.Fatalf("member scope %q %q %v", where, specific, args)
	}
}

func TestSourceValidation(t *testing.T) {
	if (Source{Table: "knowledge_chunks; DROP", TextColumn: "content"}).valid() == nil {
		t.Fatal("unsafe table accepted")
	}
}
