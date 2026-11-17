package vectorstore

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Scope narrows a vector query. The organization is mandatory (tenant
// boundary).
//
// Brand: with a brand, rows of that brand and organization-wide rows
// (brand_id NULL) match and the brand's own rows rank first; without a brand
// only organization-wide rows match, so one brand's knowledge never answers
// a conversation of another (or of no) brand.
//
// Department (CF-215, CF-222): department_id is the internal BIGINT id of
// departments. Organization-wide rows (department_id NULL) always match as
// the fallback; rows of a department match when it is one of Departments (or
// when the source's DepartmentVisible rule says so, e.g. a share). Rows of
// DepartmentID, the conversation's department, rank first. Without any
// department only organization-wide rows match (closed by default).
type Scope struct {
	OrganizationID int64
	BrandID        pgtype.UUID
	// DepartmentID is the department whose rows rank first.
	DepartmentID pgtype.Int8
	// Departments are the departments whose rows are visible (the viewer's
	// departments, deptscope). Conversation puts DepartmentID here.
	Departments []int64
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

// Conversation returns the scope of a customer conversation: its brand and
// its department, each with organization-wide rows as the fallback.
func Conversation(org int64, brand pgtype.UUID, department pgtype.Int8) Scope {
	s := Scope{OrganizationID: org, BrandID: brand, DepartmentID: department, Departments: []int64{}}
	if department.Valid {
		s.Departments = []int64{department.Int64}
	}
	return s
}

// WithDepartments replaces the department visibility with a viewer's
// (deptscope.Scope.Args): restrict false sees every department, otherwise the
// given departments plus organization-wide rows. DepartmentID keeps ranking.
func (s Scope) WithDepartments(restrict bool, departments []int64) Scope {
	s.AllDepartments = !restrict
	s.Departments = append([]int64{}, departments...)
	return s
}

// defaultDepartmentVisible is the department rule of sources without shares.
const defaultDepartmentVisible = "(%[1]s.department_id IS NULL OR %[1]s.department_id=ANY(@scope_departments::bigint[]))"

// predicate renders the scope filter for table alias a and adds its args.
// visible is the source's department rule (Source.DepartmentVisible, "" for
// the default). The second value is the SQL expression that is true for
// brand/department specific rows (they rank above organization-wide
// fallbacks); it is never NULL.
func (s Scope) predicate(a, visible string, args pgx.NamedArgs) (string, string) {
	where := fmt.Sprintf("%s.organization_id=@scope_org", a)
	args["scope_org"] = s.OrganizationID
	specific := []string{}
	switch {
	case s.AllBrands:
	case s.BrandID.Valid:
		where += fmt.Sprintf(" AND (%s.brand_id=@scope_brand OR %s.brand_id IS NULL)", a, a)
		args["scope_brand"] = s.BrandID
		specific = append(specific, fmt.Sprintf("%s.brand_id IS NOT NULL", a))
	default:
		where += fmt.Sprintf(" AND %s.brand_id IS NULL", a)
	}
	if !s.AllDepartments {
		if visible == "" {
			visible = fmt.Sprintf(defaultDepartmentVisible, a)
		}
		departments := s.Departments
		if departments == nil {
			departments = []int64{}
		}
		args["scope_departments"] = departments
		where += " AND " + visible
	}
	if s.DepartmentID.Valid {
		args["scope_department"] = s.DepartmentID.Int64
		specific = append(specific, fmt.Sprintf("(%s.department_id IS NOT NULL AND %s.department_id=@scope_department)", a, a))
	}
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
