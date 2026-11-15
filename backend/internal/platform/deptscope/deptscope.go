// Package deptscope carries the department visibility of a request (CF-215).
//
// Knowledge (articles, sources, business context, suggestions) belongs to the
// organization (department NULL) or to one department. A viewer sees
// organization-wide knowledge, the knowledge of its own departments and items
// another department shared with one of them; holders of
// tenant.departments.read_all see everything. The SQL side of the rule is
// department_visible() (migration 000225); queries take its two arguments
// from Scope.Args.
//
// The scope travels in the context so retrieval (RAG search, business
// context) needs no new parameters: the AI agent sets the conversation's
// department, request handlers set the member's departments. A context
// without a scope is closed: only organization-wide knowledge.
package deptscope

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Scope is the set of departments a viewer sees knowledge of.
type Scope struct {
	// Restrict false sees every department (tenant.departments.read_all).
	Restrict bool
	// Departments are the viewer's departments (internal ids).
	Departments []int64
}

// All sees every department.
func All() Scope { return Scope{} }

// Only sees organization-wide knowledge and the given departments.
func Only(ids ...int64) Scope {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id > 0 && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return Scope{Restrict: true, Departments: out}
}

// Of is the scope of one conversation: its department, or organization-wide
// knowledge only when it has none.
func Of(department pgtype.Int8) Scope {
	if department.Valid {
		return Only(department.Int64)
	}
	return Only()
}

// Args are the dept_restrict / viewer_departments query arguments.
func (s Scope) Args() (bool, []int64) {
	if s.Departments == nil {
		return s.Restrict, []int64{}
	}
	return s.Restrict, s.Departments
}

// Sees reports whether a row of the department is visible; shared are the
// departments the row was shared with.
func (s Scope) Sees(department pgtype.Int8, shared []int64) bool {
	if !s.Restrict || !department.Valid {
		return true
	}
	if slices.Contains(s.Departments, department.Int64) {
		return true
	}
	for _, id := range shared {
		if slices.Contains(s.Departments, id) {
			return true
		}
	}
	return false
}

type key struct{}

// With returns ctx carrying the scope.
func With(ctx context.Context, s Scope) context.Context { return context.WithValue(ctx, key{}, s) }

// From returns the scope of ctx, if any.
func From(ctx context.Context) (Scope, bool) {
	s, ok := ctx.Value(key{}).(Scope)
	return s, ok
}

// Effective is the scope of ctx; without one only organization-wide
// knowledge is visible (closed by default).
func Effective(ctx context.Context) Scope {
	if s, ok := From(ctx); ok {
		return s
	}
	return Only()
}

// Optional is a nullable department uuid of a request body that tells an
// omitted field (keep) from null (organization-wide).
type Optional struct {
	Set   bool
	Value *uuid.UUID
}

// UnmarshalJSON marks the field as present.
func (o *Optional) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var id uuid.UUID
	if err := json.Unmarshal(b, &id); err != nil {
		return err
	}
	o.Value = &id
	return nil
}

// MarshalJSON writes the uuid or null.
func (o Optional) MarshalJSON() ([]byte, error) {
	if o.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(o.Value.String())
}
