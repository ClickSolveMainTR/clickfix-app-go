// Package usecase implements the category taxonomy, record assignments and
// the label store (CF-218). Every method takes the organization id of the
// caller's orgctx.Scope.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/categories/classify"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/categories/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is an unknown category or record (or one of another
	// organization).
	ErrNotFound = errors.New("category not found")
	// ErrSlugTaken is a duplicate slug in the organization.
	ErrSlugTaken = errors.New("category slug is already used")
	// ErrInUse: the node still has children, records or labels; archive it.
	ErrInUse = errors.New("category is in use; archive it instead")
	// ErrDepartmentsUnavailable: department scoping needs CF-215.
	ErrDepartmentsUnavailable = errors.New("departments are not available yet")
	// ErrUnknownSubject is an unsupported record kind.
	ErrUnknownSubject = errors.New("unknown record kind")
	// ErrNoCategories: nothing to classify into.
	ErrNoCategories = errors.New("the organization has no categories")
	// ErrNoText: the record has no text to classify.
	ErrNoText = errors.New("the record has no text")
)

// FieldError is one field-level validation problem.
type FieldError struct{ Field, Code, Message string }

// ValidationError carries field details (422 VALIDATION_ERROR).
type ValidationError struct{ Details []FieldError }

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, FieldError{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) orNil() error {
	if len(e.Details) == 0 {
		return nil
	}
	return e
}

func invalid(field, code, msg string) error {
	return &ValidationError{Details: []FieldError{{Field: field, Code: code, Message: msg}}}
}

// Departments maps department uuids to internal ids. CF-215 provides the
// real implementation; until then NoDepartments keeps every node
// organization-wide.
type Departments interface {
	ID(ctx context.Context, org int64, id uuid.UUID) (int64, error)
	UUID(ctx context.Context, org int64, id int64) (uuid.UUID, error)
}

// NoDepartments rejects department scoping (CF-215 not merged yet).
type NoDepartments struct{}

// ID implements Departments.
func (NoDepartments) ID(context.Context, int64, uuid.UUID) (int64, error) {
	return 0, ErrDepartmentsUnavailable
}

// UUID implements Departments.
func (NoDepartments) UUID(context.Context, int64, int64) (uuid.UUID, error) {
	return uuid.Nil, ErrDepartmentsUnavailable
}

// Service is the categories use case.
type Service struct {
	pool        *pgxpool.Pool
	q           *db.Queries
	departments Departments
	subjects    map[string]SubjectSource
	classifier  classify.Classifier
	embedder    ai.Embedder
}

// Options wires the optional parts of the service.
type Options struct {
	Departments Departments
	// Classifier defaults to the phase-1 chain built from Embedder and AI.
	Classifier classify.Classifier
	Embedder   ai.Embedder
	AI         classify.Runner
}

// New builds the service with the built-in record kinds (conversation,
// suggestion). CF-216 / CF-217 add theirs with RegisterSubject.
func New(pool *pgxpool.Pool, q *db.Queries, opt Options) *Service {
	s := &Service{pool: pool, q: q, departments: opt.Departments, embedder: opt.Embedder, subjects: map[string]SubjectSource{}}
	if s.departments == nil {
		s.departments = NoDepartments{}
	}
	s.classifier = opt.Classifier
	if s.classifier == nil {
		s.classifier = PhaseOne(q, opt.Embedder, opt.AI)
	}
	s.RegisterSubject(classify.SubjectConversation, conversationSource{q: q})
	s.RegisterSubject(classify.SubjectSuggestion, suggestionSource{q: q})
	return s
}

// PhaseOne is embedding similarity, then the AI, then a human.
func PhaseOne(q *db.Queries, embedder ai.Embedder, runner classify.Runner) *classify.Chain {
	chain := &classify.Chain{Thresholds: classify.DefaultThresholds}
	if embedder != nil {
		chain.Stages = append(chain.Stages, &classify.EmbeddingStage{Embedder: embedder, Store: &vectorStore{q: q}})
	}
	if runner != nil {
		chain.Stages = append(chain.Stages, &classify.AIStage{AI: runner})
	}
	return chain
}

// Classifier exposes the configured classifier (CF-217 reuses it).
func (s *Service) Classifier() classify.Classifier { return s.classifier }

func (s *Service) withTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// List returns the tree as a flat, parent-before-child list. department
// uuid.Nil lists everything; a department lists the organization-wide nodes
// plus that department's branches.
func (s *Service) List(ctx context.Context, org int64, department uuid.UUID, includeArchived bool) ([]model.Category, error) {
	dept := pgtype.Int8{}
	if department != uuid.Nil {
		id, err := s.departments.ID(ctx, org, department)
		if err != nil {
			return nil, departmentErr(err)
		}
		dept = pgtype.Int8{Int64: id, Valid: true}
	}
	rows, err := s.q.ListCategories(ctx, db.ListCategoriesParams{OrganizationID: org, IncludeArchived: includeArchived, DepartmentID: dept})
	if err != nil {
		return nil, err
	}
	out := make([]model.Category, 0, len(rows))
	deptUUIDs := map[int64]uuid.UUID{}
	for _, r := range rows {
		c := model.Category{
			UUID: r.Uuid, Name: r.Name, Slug: r.Slug, Description: r.Description, Examples: r.Examples,
			Position: r.Position, Depth: r.Depth, Archived: r.ArchivedAt.Valid, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		}
		if r.ParentUuid.Valid {
			id := uuid.UUID(r.ParentUuid.Bytes)
			c.ParentUUID = &id
		}
		c.DepartmentUUID = s.deptUUID(ctx, org, r.DepartmentID, deptUUIDs)
		out = append(out, c)
	}
	return out, nil
}

func (s *Service) deptUUID(ctx context.Context, org int64, id pgtype.Int8, cache map[int64]uuid.UUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	u, ok := cache[id.Int64]
	if !ok {
		var err error
		if u, err = s.departments.UUID(ctx, org, id.Int64); err != nil {
			return nil
		}
		cache[id.Int64] = u
	}
	return &u
}

// Get returns one node.
func (s *Service) Get(ctx context.Context, org int64, id uuid.UUID) (model.Category, error) {
	row, err := s.q.GetCategoryByUUID(ctx, db.GetCategoryByUUIDParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Category{}, ErrNotFound
	}
	if err != nil {
		return model.Category{}, err
	}
	c := model.Category{
		UUID: row.Uuid, Name: row.Name, Slug: row.Slug, Description: row.Description, Examples: row.Examples,
		Position: row.Position, Depth: row.Depth, Archived: row.ArchivedAt.Valid, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.ParentUuid.Valid {
		p := uuid.UUID(row.ParentUuid.Bytes)
		c.ParentUUID = &p
	}
	c.DepartmentUUID = s.deptUUID(ctx, org, row.DepartmentID, map[int64]uuid.UUID{})
	return c, nil
}

var slugRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

var trSlug = strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "İ", "i", "ö", "o", "ş", "s", "ü", "u", "Ç", "c", "Ğ", "g", "Ö", "o", "Ş", "s", "Ü", "u")

// Slugify turns a name into a slug ("Fatura & İade" -> "fatura-iade").
func Slugify(name string) string {
	s := strings.ToLower(trSlug.Replace(strings.TrimSpace(name)))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if len(out) > 80 {
		out = strings.TrimSuffix(out[:80], "-")
	}
	return out
}

// node is the validated shape written to the database.
type node struct {
	department, parent pgtype.Int8
	name, slug, desc   string
	examples           []string
	position           int32
	depth              int16
	archived           bool
}

func cleanExamples(in []string) ([]string, bool) {
	out := []string{}
	seen := map[string]bool{}
	for _, e := range in {
		e = strings.TrimSpace(e)
		if e == "" || seen[e] {
			continue
		}
		if utf8.RuneCountInString(e) > 500 {
			return nil, false
		}
		seen[e] = true
		out = append(out, e)
	}
	return out, len(out) <= 50
}

// resolve applies input on top of cur and checks the tree rules:
//   - a root may be organization-wide or belong to a department;
//   - a child of a department node belongs to the same department;
//   - a child of an organization-wide node may open a department branch;
//   - no cycles, depth <= model.MaxDepth.
func (s *Service) resolve(ctx context.Context, q *db.Queries, org int64, self int64, cur node, in model.CategoryInput) (node, error) {
	n := cur
	ve := &ValidationError{}
	if in.Name != nil {
		n.name = strings.TrimSpace(*in.Name)
	}
	if l := utf8.RuneCountInString(n.name); l < 1 || l > 120 {
		ve.add("name", "invalid", "name must be 1-120 characters")
	}
	if in.Slug != nil {
		n.slug = strings.TrimSpace(*in.Slug)
	}
	if n.slug == "" {
		n.slug = Slugify(n.name)
	}
	if !slugRE.MatchString(n.slug) || len(n.slug) > 80 {
		ve.add("slug", "invalid", "slug must be lowercase letters, digits and dashes")
	}
	if in.Description != nil {
		n.desc = strings.TrimSpace(*in.Description)
	}
	if utf8.RuneCountInString(n.desc) > 1000 {
		ve.add("description", "invalid", "description must be at most 1000 characters")
	}
	if in.Examples != nil {
		ex, ok := cleanExamples(*in.Examples)
		if !ok {
			ve.add("examples", "invalid", "at most 50 examples of at most 500 characters")
		}
		n.examples = ex
	}
	if n.examples == nil {
		n.examples = []string{}
	}
	if in.Position != nil {
		n.position = *in.Position
	}
	if in.Archived != nil {
		n.archived = *in.Archived
	}
	if in.DepartmentUUID != nil {
		n.department = pgtype.Int8{}
		if raw := strings.TrimSpace(*in.DepartmentUUID); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				ve.add("department_uuid", "invalid", "department_uuid must be a uuid")
			} else if dept, err := s.departments.ID(ctx, org, id); err != nil {
				ve.add("department_uuid", "invalid", departmentErr(err).Error())
			} else {
				n.department = pgtype.Int8{Int64: dept, Valid: true}
			}
		}
	}
	var parent db.Category
	if in.ParentUUID != nil {
		n.parent = pgtype.Int8{}
		if raw := strings.TrimSpace(*in.ParentUUID); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				ve.add("parent_uuid", "invalid", "parent_uuid must be a uuid")
			} else if p, err := q.GetCategoryByUUID(ctx, db.GetCategoryByUUIDParams{OrganizationID: org, Uuid: id}); err != nil {
				ve.add("parent_uuid", "not_found", "parent category not found")
			} else {
				n.parent = pgtype.Int8{Int64: p.ID, Valid: true}
			}
		}
	}
	if err := ve.orNil(); err != nil {
		return n, err
	}
	n.depth = 0
	if n.parent.Valid {
		p, err := q.GetCategoryByID(ctx, db.GetCategoryByIDParams{OrganizationID: org, ID: n.parent.Int64})
		if err != nil {
			return n, err
		}
		parent = p
		// A department node's subtree stays in that department; a child
		// inherits it unless it names one explicitly.
		if parent.DepartmentID.Valid {
			if in.DepartmentUUID == nil && !n.department.Valid {
				n.department = parent.DepartmentID
			}
			if n.department != parent.DepartmentID {
				return n, invalid("department_uuid", "mismatch", "a child of a department category belongs to the same department")
			}
		}
		if parent.ArchivedAt.Valid && !n.archived {
			return n, invalid("parent_uuid", "archived", "parent category is archived")
		}
		n.depth = parent.Depth + 1
		if self > 0 {
			ancestors, err := q.CategoryAncestorIDs(ctx, db.CategoryAncestorIDsParams{OrganizationID: org, ID: parent.ID})
			if err != nil {
				return n, err
			}
			for _, a := range ancestors {
				if a == self {
					return n, invalid("parent_uuid", "cycle", "a category cannot move under itself")
				}
			}
		}
	}
	height := int32(0)
	if self > 0 {
		h, err := q.CategorySubtreeHeight(ctx, db.CategorySubtreeHeightParams{OrganizationID: org, ID: self})
		if err != nil {
			return n, err
		}
		height = h
		if n.department.Valid {
			conflict, err := q.CategoryChildDepartmentConflict(ctx, db.CategoryChildDepartmentConflictParams{OrganizationID: org, ID: pgtype.Int8{Int64: self, Valid: true}, DepartmentID: n.department})
			if err != nil {
				return n, err
			}
			if conflict {
				return n, invalid("department_uuid", "mismatch", "every category under a department category belongs to the same department")
			}
		}
	}
	if int32(n.depth)+height > model.MaxDepth {
		return n, invalid("parent_uuid", "too_deep", fmt.Sprintf("the tree is at most %d levels deep", model.MaxDepth+1))
	}
	return n, nil
}

// Create adds a node.
func (s *Service) Create(ctx context.Context, org int64, in model.CategoryInput) (model.Category, error) {
	if in.Name == nil {
		return model.Category{}, invalid("name", "required", "name is required")
	}
	var id uuid.UUID
	err := s.withTx(ctx, func(q *db.Queries) error {
		n, err := s.resolve(ctx, q, org, 0, node{}, in)
		if err != nil {
			return err
		}
		c, err := q.CreateCategory(ctx, db.CreateCategoryParams{
			OrganizationID: org, DepartmentID: n.department, ParentID: n.parent, Name: n.name, Slug: n.slug,
			Description: n.desc, Examples: n.examples, Position: n.position, Depth: n.depth,
		})
		if err != nil {
			return slugErr(err)
		}
		id = c.Uuid
		if n.archived {
			_, err = q.UpdateCategory(ctx, updateParams(org, c.ID, n))
		}
		return err
	})
	if err != nil {
		return model.Category{}, err
	}
	return s.Get(ctx, org, id)
}

func updateParams(org, id int64, n node) db.UpdateCategoryParams {
	p := db.UpdateCategoryParams{
		OrganizationID: org, ID: id, DepartmentID: n.department, ParentID: n.parent, Name: n.name, Slug: n.slug,
		Description: n.desc, Examples: n.examples, Position: n.position, Depth: n.depth,
	}
	if n.archived {
		p.ArchivedAt = pgtype.Timestamptz{Time: timeNow(), Valid: true}
	}
	return p
}

// Update patches a node (re-parenting moves its subtree).
func (s *Service) Update(ctx context.Context, org int64, id uuid.UUID, in model.CategoryInput) (model.Category, error) {
	err := s.withTx(ctx, func(q *db.Queries) error {
		cur, err := q.GetCategoryByUUID(ctx, db.GetCategoryByUUIDParams{OrganizationID: org, Uuid: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		n, err := s.resolve(ctx, q, org, cur.ID, node{
			department: cur.DepartmentID, parent: cur.ParentID, name: cur.Name, slug: cur.Slug, desc: cur.Description,
			examples: cur.Examples, position: cur.Position, depth: cur.Depth, archived: cur.ArchivedAt.Valid,
		}, in)
		if err != nil {
			return err
		}
		p := updateParams(org, cur.ID, n)
		if n.archived && cur.ArchivedAt.Valid {
			p.ArchivedAt = cur.ArchivedAt
		}
		if _, err := q.UpdateCategory(ctx, p); err != nil {
			return slugErr(err)
		}
		if n.archived && !cur.ArchivedAt.Valid {
			if err := q.ArchiveCategoryDescendants(ctx, db.ArchiveCategoryDescendantsParams{OrganizationID: org, ID: cur.ID}); err != nil {
				return err
			}
		}
		if delta := n.depth - cur.Depth; delta != 0 {
			if err := q.ShiftCategoryDescendantDepth(ctx, db.ShiftCategoryDescendantDepthParams{OrganizationID: org, ID: pgtype.Int8{Int64: cur.ID, Valid: true}, Delta: delta}); err != nil {
				return err
			}
		}
		return q.PruneCategoryExamples(ctx, db.PruneCategoryExamplesParams{OrganizationID: org, ID: cur.ID})
	})
	if err != nil {
		return model.Category{}, err
	}
	return s.Get(ctx, org, id)
}

// Delete removes an unused leaf; used nodes are archived instead so labels
// keep their meaning.
func (s *Service) Delete(ctx context.Context, org int64, id uuid.UUID) error {
	return s.withTx(ctx, func(q *db.Queries) error {
		cur, err := q.GetCategoryByUUID(ctx, db.GetCategoryByUUIDParams{OrganizationID: org, Uuid: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		used, err := q.CategoryInUse(ctx, db.CategoryInUseParams{OrganizationID: org, ID: cur.ID})
		if err != nil {
			return err
		}
		if used {
			return ErrInUse
		}
		_, err = q.DeleteCategory(ctx, db.DeleteCategoryParams{OrganizationID: org, ID: cur.ID})
		return err
	})
}

func slugErr(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return ErrSlugTaken
	}
	return err
}

func departmentErr(err error) error {
	if errors.Is(err, ErrDepartmentsUnavailable) {
		return err
	}
	return invalid("department_uuid", "not_found", "department not found")
}
