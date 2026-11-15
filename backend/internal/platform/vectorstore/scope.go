package vectorstore

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Scope narrows a vector query. The organization is mandatory (tenant
// boundary). With a brand, rows of that brand and organization-wide rows
// (brand_id NULL) match and the brand's own rows rank first; without a brand
// only organization-wide rows match, so one brand's knowledge never answers
// a conversation of another (or of no) brand. Departments follow the same rule
// (CF-215 fills department_id; until then every row is organization wide).
type Scope struct {
	OrganizationID int64
	BrandID        pgtype.UUID
	DepartmentID   pgtype.UUID
	// AllBrands / AllDepartments drop that filter. Only internal, agent-side
	// searches that are not tied to a conversation (admin search, team chat)
	// set them.
	AllBrands      bool
	AllDepartments bool
}

// Organization returns an unfiltered (all brands, all departments) scope for
// internal searches.
func Organization(org int64) Scope {
	return Scope{OrganizationID: org, AllBrands: true, AllDepartments: true}
}

// Conversation returns the scope of a customer conversation.
func Conversation(org int64, brand, department pgtype.UUID) Scope {
	return Scope{OrganizationID: org, BrandID: brand, DepartmentID: department}
}

// predicate renders the scope filter for table alias a and adds its args. The
// second value is the SQL expression that is true for brand/department
// specific rows (they rank above organization-wide fallbacks).
func (s Scope) predicate(a string, args pgx.NamedArgs) (string, string) {
	where := fmt.Sprintf("%s.organization_id=@scope_org", a)
	args["scope_org"] = s.OrganizationID
	specific := []string{}
	add := func(column, key string, all bool, value pgtype.UUID) {
		switch {
		case all:
		case value.Valid:
			where += fmt.Sprintf(" AND (%s.%s=@%s OR %s.%s IS NULL)", a, column, key, a, column)
			args[key] = value
			specific = append(specific, fmt.Sprintf("%s.%s IS NOT NULL", a, column))
		default:
			where += fmt.Sprintf(" AND %s.%s IS NULL", a, column)
		}
	}
	add("brand_id", "scope_brand", s.AllBrands, s.BrandID)
	add("department_id", "scope_department", s.AllDepartments, s.DepartmentID)
	expr := "false"
	for i, e := range specific {
		if i == 0 {
			expr = "(" + e
		} else {
			expr += " OR " + e
		}
		if i == len(specific)-1 {
			expr += ")"
		}
	}
	return where, expr
}
