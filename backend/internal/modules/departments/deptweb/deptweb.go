// Package deptweb gives other modules' handlers (knowledge, AI workflows)
// the department side of a request (CF-215): the member's knowledge scope,
// the department of a knowledge write, its errors and the department
// reference added to list rows.
package deptweb

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/deptscope"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/jackc/pgx/v5/pgtype"
)

// Scope is the knowledge scope of the request's member.
func Scope(r *http.Request, q *db.Queries) (deptscope.Scope, error) {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		return deptscope.Only(), nil
	}
	return usecase.ScopeOf(r.Context(), q, orgctx.Must(r.Context()), p.UserInternal)
}

// Resolve turns the department_uuid of a knowledge write into its id.
func Resolve(r *http.Request, q *db.Queries, scope deptscope.Scope, opt deptscope.Optional) (pgtype.Int8, error) {
	return usecase.ResolveFor(r.Context(), q, orgctx.Must(r.Context()).ID, scope, opt)
}

// Fail writes a department error and reports whether err was one.
func Fail(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, model.ErrForbidden):
		response.Forbidden(w, r, "This department's knowledge is not yours to change")
	case errors.Is(err, model.ErrNotFound):
		response.ValidationError(w, r, []response.Detail{{Field: "department_uuid", Code: "not_found", Message: "department not found"}})
	default:
		return false
	}
	return true
}

// Row is a list row with its department reference added.
type Row[T any] struct {
	Item       T
	Department *model.Ref
}

// MarshalJSON writes the row's own fields plus "department".
func (r Row[T]) MarshalJSON() ([]byte, error) {
	raw, err := json.Marshal(r.Item)
	if err != nil {
		return nil, err
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	dept, err := json.Marshal(r.Department)
	if err != nil {
		return nil, err
	}
	fields["department"] = dept
	delete(fields, "department_id")
	return json.Marshal(fields)
}

// Annotate adds the department reference of every row.
func Annotate[T any](ctx context.Context, q *db.Queries, org int64, rows []T, department func(T) pgtype.Int8) ([]Row[T], error) {
	ids := []int64{}
	for _, row := range rows {
		if d := department(row); d.Valid {
			ids = append(ids, d.Int64)
		}
	}
	refs, err := usecase.Refs(ctx, q, org, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Row[T], 0, len(rows))
	for _, row := range rows {
		item := Row[T]{Item: row}
		if d := department(row); d.Valid {
			if ref, ok := refs[d.Int64]; ok {
				item.Department = &ref
			}
		}
		out = append(out, item)
	}
	return out, nil
}
