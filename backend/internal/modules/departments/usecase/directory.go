package usecase

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/google/uuid"
)

// Directory maps an organization's department UUIDs to internal ids and
// back for the modules that keep a department_id (CF-225): the measurement
// set (aiagents DepartmentDirectory) and the category tree (categories
// Departments). Every lookup is organization scoped; another
// organization's department is pgx.ErrNoRows.
type Directory struct{ Q *db.Queries }

// NewDirectory returns the department directory over q.
func NewDirectory(q *db.Queries) *Directory { return &Directory{Q: q} }

// DepartmentID returns the internal id of the organization's department.
func (d *Directory) DepartmentID(ctx context.Context, org int64, id uuid.UUID) (int64, error) {
	row, err := d.Q.GetDepartmentByUUID(ctx, db.GetDepartmentByUUIDParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return 0, err
	}
	return row.ID, nil
}

// DepartmentUUIDs maps internal ids to public UUIDs; ids of other
// organizations are left out.
func (d *Directory) DepartmentUUIDs(ctx context.Context, org int64, ids []int64) (map[int64]uuid.UUID, error) {
	refs, err := Refs(ctx, d.Q, org, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]uuid.UUID, len(refs))
	for id, r := range refs {
		out[id] = r.UUID
	}
	return out, nil
}

// ID implements the categories Departments interface.
func (d *Directory) ID(ctx context.Context, org int64, id uuid.UUID) (int64, error) {
	return d.DepartmentID(ctx, org, id)
}

// UUID implements the categories Departments interface.
func (d *Directory) UUID(ctx context.Context, org int64, id int64) (uuid.UUID, error) {
	row, err := d.Q.GetDepartmentByID(ctx, db.GetDepartmentByIDParams{OrganizationID: org, ID: id})
	if err != nil {
		return uuid.Nil, err
	}
	return row.Uuid, nil
}
