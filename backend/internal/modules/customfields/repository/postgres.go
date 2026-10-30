// Package repository holds the custom field queries sqlc cannot express:
// record lists with dynamic JSONB filters and sorts.
package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository wraps sqlc queries plus the dynamic record list.
type Repository struct {
	Pool *pgxpool.Pool
	Q    *db.Queries
}

// New builds the repository.
func New(pool *pgxpool.Pool, q *db.Queries) *Repository {
	return &Repository{Pool: pool, Q: q}
}

var recordSorts = map[string]string{
	"display_name": "r.display_name",
	"created_at":   "r.created_at",
	"updated_at":   "r.updated_at",
}

// ListRecords returns one page of records and the total match count.
func (r *Repository) ListRecords(ctx context.Context, f model.RecordList) ([]db.CustomObjectRecord, int64, error) {
	args := []any{f.OrganizationID, f.ObjectTypeID}
	where := []string{"r.organization_id = $1", "r.object_type_id = $2"}
	if q := strings.TrimSpace(f.Q); q != "" {
		args = append(args, "%"+strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)+"%")
		where = append(where, fmt.Sprintf("r.display_name ILIKE $%d", len(args)))
	}
	if len(f.Filters) > 0 {
		clause, extra := customfields.SQL("r.custom_fields", f.Filters, len(args)+1)
		args = append(args, extra...)
		where = append(where, clause)
	}
	order, ok := recordSorts[f.Sort]
	if !ok {
		if order, ok = customfields.SortExpr("r.custom_fields", f.Sort, f.Defs); !ok {
			order = "r.created_at"
		}
	}
	dir := "ASC"
	if f.SortDesc {
		dir = "DESC"
	}
	args = append(args, f.Limit, f.Offset)
	sql := fmt.Sprintf(`SELECT r.id, r.uuid, r.organization_id, r.object_type_id, r.display_name,
       r.custom_fields, r.created_by, r.created_at, r.updated_at, COUNT(*) OVER() AS total
FROM custom_object_records r
WHERE %s
ORDER BY %s %s NULLS LAST, r.id %s
LIMIT $%d OFFSET $%d`, strings.Join(where, " AND "), order, dir, dir, len(args)-1, len(args))
	rows, err := r.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("customfields: list records: %w", err)
	}
	defer rows.Close()
	var out []db.CustomObjectRecord
	var total int64
	for rows.Next() {
		var rec db.CustomObjectRecord
		if err := rows.Scan(&rec.ID, &rec.Uuid, &rec.OrganizationID, &rec.ObjectTypeID, &rec.DisplayName,
			&rec.CustomFields, &rec.CreatedBy, &rec.CreatedAt, &rec.UpdatedAt, &total); err != nil {
			return nil, 0, fmt.Errorf("customfields: scan record: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(out) == 0 && f.Offset > 0 {
		// Past the last page: COUNT(*) OVER() has no row to ride on.
		if err := r.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM custom_object_records r WHERE r.organization_id = $1 AND "+strings.Join(where[1:], " AND "), args[:len(args)-2]...).Scan(&total); err != nil && err != pgx.ErrNoRows {
			return nil, 0, err
		}
	}
	return out, total, nil
}
