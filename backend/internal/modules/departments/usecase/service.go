// Package usecase implements departments (CF-215): CRUD, the knowledge
// visibility of a member, sharing between departments and the department of
// a conversation (widget pick, AI prediction, transfer). Every method takes
// the organization id of the caller's orgctx.Scope.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/deptscope"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PredictThreshold is the confidence the AI needs to route a first message
// to a department on its own; below it the conversation stays
// organization-wide and an agent transfers it.
const PredictThreshold = 0.6

var (
	hhmmRE  = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	colorRE = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

var weekdays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

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

// Conversations is the inbox use case a transfer goes through, so team and
// assignee changes write the usual timeline events.
type Conversations interface {
	Update(ctx context.Context, in convusecase.UpdateInput) (db.Conversation, error)
	SystemEvent(ctx context.Context, orgID, convID int64, actor convusecase.Actor, attrs map[string]any) error
}

// Classifier predicts the department of a first message (the AI agent).
type Classifier interface {
	ClassifyDepartment(ctx context.Context, org, conversationID int64, transcript, locale string, candidates []model.Candidate) (model.Prediction, error)
}

// SLA recomputes a conversation's clocks after its department changed.
type SLA interface {
	Sync(ctx context.Context, orgID, conversationID int64, at time.Time) error
}

// Service is the departments use case.
type Service struct {
	pool          *pgxpool.Pool
	q             *db.Queries
	Conversations Conversations
	Classifier    Classifier
	SLA           SLA
	Log           *slog.Logger
	Now           func() time.Time
}

// New builds the service.
func New(pool *pgxpool.Pool, q *db.Queries) *Service {
	return &Service{pool: pool, q: q, Log: slog.Default(), Now: time.Now}
}

func (s *Service) withTx(ctx context.Context, fn func(q *db.Queries) error) error {
	if s.pool == nil {
		return fn(s.q)
	}
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

// ---------------------------------------------------------------------------
// Viewer

// Viewer is what a member may see and do across departments.
type Viewer struct {
	// Scope is the knowledge visibility of the member.
	Scope deptscope.Scope
	// Manage is tenant.departments.write: every department, every share.
	Manage bool
	// Leads are the departments the member leads (a lead of one of its teams).
	Leads []int64
}

// Leads reports whether the viewer may share the department's knowledge.
func (v Viewer) CanShare(department int64) bool {
	return v.Manage || slices.Contains(v.Leads, department)
}

// ViewerFor builds the department viewer of a member: read_all (or write)
// sees every department, everyone else their teams' departments.
func (s *Service) ViewerFor(ctx context.Context, scope orgctx.Scope, userID int64) (Viewer, error) {
	return ViewerOf(ctx, s.q, scope, userID)
}

// ViewerOf builds the department viewer with a bare query set (other
// modules' handlers).
func ViewerOf(ctx context.Context, q *db.Queries, scope orgctx.Scope, userID int64) (Viewer, error) {
	v := Viewer{Manage: scope.Has(rbac.PermTenantDepartmentsWrite)}
	rows, err := q.ListUserDepartments(ctx, db.ListUserDepartmentsParams{OrganizationID: scope.ID, UserID: userID})
	if err != nil {
		return Viewer{}, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.DepartmentID)
		if r.Lead {
			v.Leads = append(v.Leads, r.DepartmentID)
		}
	}
	if v.Manage || scope.Has(rbac.PermTenantDepartmentsReadAll) {
		v.Scope = deptscope.All()
	} else {
		v.Scope = deptscope.Only(ids...)
	}
	return v, nil
}

// ScopeOf is the knowledge scope of a member for other modules (knowledge,
// AI workflows), to put into the request context with deptscope.With.
func ScopeOf(ctx context.Context, q *db.Queries, scope orgctx.Scope, userID int64) (deptscope.Scope, error) {
	v, err := ViewerOf(ctx, q, scope, userID)
	return v.Scope, err
}

// ResolveFor turns the department of a knowledge write into its id. A member
// without read_all may only file knowledge under their own departments.
func ResolveFor(ctx context.Context, q *db.Queries, org int64, scope deptscope.Scope, opt deptscope.Optional) (pgtype.Int8, error) {
	if !opt.Set || opt.Value == nil {
		return pgtype.Int8{}, nil
	}
	d, err := q.GetDepartmentByUUID(ctx, db.GetDepartmentByUUIDParams{OrganizationID: org, Uuid: *opt.Value})
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.Int8{}, model.ErrNotFound
	}
	if err != nil {
		return pgtype.Int8{}, err
	}
	if scope.Restrict && !slices.Contains(scope.Departments, d.ID) {
		return pgtype.Int8{}, model.ErrForbidden
	}
	return pgtype.Int8{Int64: d.ID, Valid: true}, nil
}

// Refs maps department ids to their public reference.
func Refs(ctx context.Context, q *db.Queries, org int64, ids []int64) (map[int64]model.Ref, error) {
	out := map[int64]model.Ref{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListDepartmentRefs(ctx, db.ListDepartmentRefsParams{OrganizationID: org, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = model.Ref{UUID: r.Uuid, Name: r.Name, Color: r.Color}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// CRUD

// List lists departments with their teams and leads.
func (s *Service) List(ctx context.Context, org int64, viewer Viewer, f model.ListFilter) ([]model.Department, int64, error) {
	brand := pgtype.UUID{}
	if f.BrandUUID != nil {
		brand = pgtype.UUID{Bytes: *f.BrandUUID, Valid: true}
	}
	onlyIDs, ids := false, []int64{}
	if f.Mine {
		onlyIDs, ids = true, viewer.Scope.Departments
		if !viewer.Scope.Restrict {
			// Managers "mine" are still their own teams' departments.
			onlyIDs = false
		}
	}
	q := pgtype.Text{String: strings.TrimSpace(f.Q), Valid: strings.TrimSpace(f.Q) != ""}
	rows, err := s.q.ListDepartments(ctx, db.ListDepartmentsParams{
		OrganizationID: org, Q: q, BrandID: brand, OnlyIds: onlyIDs, Ids: ids, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountDepartments(ctx, db.CountDepartmentsParams{OrganizationID: org, Q: q, BrandID: brand, OnlyIds: onlyIDs, Ids: ids})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Department, 0, len(rows))
	for _, r := range rows {
		d := mapDepartment(db.Department{
			ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, BrandID: r.BrandID, Name: r.Name, Description: r.Description,
			Color: r.Color, WidgetVisible: r.WidgetVisible, SortOrder: r.SortOrder, SlaPolicyID: r.SlaPolicyID, Timezone: r.Timezone,
			WorkingHours: r.WorkingHours, AiEnabled: r.AiEnabled, AiInstructions: r.AiInstructions, RoutingHint: r.RoutingHint,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
		d.TeamCount = r.TeamCount
		out = append(out, d)
	}
	if err := s.attach(ctx, org, out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Get returns one department.
func (s *Service) Get(ctx context.Context, org int64, id uuid.UUID) (model.Department, error) {
	row, err := s.q.GetDepartmentByUUID(ctx, db.GetDepartmentByUUIDParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Department{}, model.ErrNotFound
	}
	if err != nil {
		return model.Department{}, err
	}
	list := []model.Department{mapDepartment(row)}
	if err := s.attach(ctx, org, list); err != nil {
		return model.Department{}, err
	}
	list[0].TeamCount = int64(len(list[0].Teams))
	return list[0], nil
}

func (s *Service) attach(ctx context.Context, org int64, list []model.Department) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(list))
	index := map[int64]int{}
	for i, d := range list {
		ids = append(ids, d.ID)
		index[d.ID] = i
		list[i].Teams, list[i].Leads = []model.TeamRef{}, []model.UserRef{}
	}
	teams, err := s.q.ListDepartmentTeams(ctx, db.ListDepartmentTeamsParams{OrganizationID: org, DepartmentIds: ids})
	if err != nil {
		return err
	}
	for _, t := range teams {
		i := index[t.DepartmentID]
		list[i].Teams = append(list[i].Teams, model.TeamRef{UUID: t.Uuid, Name: t.Name, Color: t.Color})
	}
	leads, err := s.q.ListDepartmentLeads(ctx, db.ListDepartmentLeadsParams{OrganizationID: org, DepartmentIds: ids})
	if err != nil {
		return err
	}
	for _, l := range leads {
		i := index[l.DepartmentID]
		list[i].Leads = append(list[i].Leads, model.UserRef{UUID: l.Uuid, Email: l.Email, Name: l.Name, Surname: l.Surname})
	}
	return nil
}

// Create creates a department.
func (s *Service) Create(ctx context.Context, org int64, in model.Input) (model.Department, error) {
	cur := db.Department{WidgetVisible: true, AiEnabled: true}
	next, err := s.apply(ctx, org, uuid.Nil, cur, in)
	if err != nil {
		return model.Department{}, err
	}
	var created db.Department
	err = s.withTx(ctx, func(q *db.Queries) error {
		row, err := q.CreateDepartment(ctx, db.CreateDepartmentParams{
			OrganizationID: org, BrandID: next.BrandID, Name: next.Name, Description: next.Description, Color: next.Color,
			WidgetVisible: next.WidgetVisible, SortOrder: next.SortOrder, SlaPolicyID: next.SlaPolicyID, Timezone: next.Timezone,
			WorkingHours: next.WorkingHours, AiEnabled: next.AiEnabled, AiInstructions: next.AiInstructions, RoutingHint: next.RoutingHint,
		})
		if err != nil {
			return err
		}
		created = row
		return s.setTeams(ctx, q, org, row.ID, in.TeamUUIDs)
	})
	if err != nil {
		return model.Department{}, mapDBError(err)
	}
	return s.Get(ctx, org, created.Uuid)
}

// Update patches a department.
func (s *Service) Update(ctx context.Context, org int64, id uuid.UUID, in model.Input) (model.Department, error) {
	cur, err := s.q.GetDepartmentByUUID(ctx, db.GetDepartmentByUUIDParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Department{}, model.ErrNotFound
	}
	if err != nil {
		return model.Department{}, err
	}
	next, err := s.apply(ctx, org, id, cur, in)
	if err != nil {
		return model.Department{}, err
	}
	err = s.withTx(ctx, func(q *db.Queries) error {
		if _, err := q.UpdateDepartment(ctx, db.UpdateDepartmentParams{
			OrganizationID: org, Uuid: id, BrandID: next.BrandID, Name: next.Name, Description: next.Description, Color: next.Color,
			WidgetVisible: next.WidgetVisible, SortOrder: next.SortOrder, SlaPolicyID: next.SlaPolicyID, Timezone: next.Timezone,
			WorkingHours: next.WorkingHours, AiEnabled: next.AiEnabled, AiInstructions: next.AiInstructions, RoutingHint: next.RoutingHint,
		}); err != nil {
			return err
		}
		return s.setTeams(ctx, q, org, cur.ID, in.TeamUUIDs)
	})
	if err != nil {
		return model.Department{}, mapDBError(err)
	}
	return s.Get(ctx, org, id)
}

// setTeams makes ids the complete team list of the department (nil keeps it).
func (s *Service) setTeams(ctx context.Context, q *db.Queries, org, department int64, ids *[]uuid.UUID) error {
	if ids == nil {
		return nil
	}
	current, err := q.ListDepartmentTeams(ctx, db.ListDepartmentTeamsParams{OrganizationID: org, DepartmentIds: []int64{department}})
	if err != nil {
		return err
	}
	for _, t := range current {
		if !slices.Contains(*ids, t.Uuid) {
			if _, err := q.SetTeamDepartment(ctx, db.SetTeamDepartmentParams{OrganizationID: org, Uuid: t.Uuid}); err != nil {
				return err
			}
		}
	}
	for i, id := range *ids {
		n, err := q.SetTeamDepartment(ctx, db.SetTeamDepartmentParams{OrganizationID: org, Uuid: id, DepartmentID: pgtype.Int8{Int64: department, Valid: true}})
		if err != nil {
			return err
		}
		if n == 0 {
			return &ValidationError{Details: []FieldError{{Field: fmt.Sprintf("team_uuids[%d]", i), Code: "not_found", Message: "team not found"}}}
		}
	}
	return nil
}

// apply validates in over cur.
func (s *Service) apply(ctx context.Context, org int64, self uuid.UUID, cur db.Department, in model.Input) (db.Department, error) {
	ve := &ValidationError{}
	next := cur
	if in.Name != nil {
		next.Name = strings.TrimSpace(*in.Name)
	}
	if next.Name == "" || utf8.RuneCountInString(next.Name) > 120 {
		ve.add("name", "invalid", "name must be 1-120 characters")
	} else {
		taken, err := s.q.DepartmentNameTaken(ctx, db.DepartmentNameTakenParams{OrganizationID: org, Name: next.Name, ExceptUuid: self})
		if err != nil {
			return next, err
		}
		if taken {
			return next, model.ErrNameTaken
		}
	}
	if in.Description != nil {
		next.Description = strings.TrimSpace(*in.Description)
	}
	if utf8.RuneCountInString(next.Description) > 1000 {
		ve.add("description", "too_long", "description must be at most 1000 characters")
	}
	if in.Color != nil {
		next.Color = strings.TrimSpace(*in.Color)
	}
	if next.Color != "" && !colorRE.MatchString(next.Color) {
		ve.add("color", "invalid", "color must be #RRGGBB")
	}
	if in.WidgetVisible != nil {
		next.WidgetVisible = *in.WidgetVisible
	}
	if in.SortOrder != nil {
		next.SortOrder = *in.SortOrder
	}
	if in.BrandUUID.Set {
		next.BrandID = pgtype.UUID{}
		if in.BrandUUID.Value != nil {
			if _, err := s.q.GetBrand(ctx, db.GetBrandParams{OrganizationID: org, Uuid: *in.BrandUUID.Value}); err != nil {
				if !errors.Is(err, pgx.ErrNoRows) {
					return next, err
				}
				ve.add("brand_uuid", "not_found", "brand not found")
			}
			next.BrandID = pgtype.UUID{Bytes: *in.BrandUUID.Value, Valid: true}
		}
	}
	if in.SLAPolicyUUID.Set {
		next.SlaPolicyID = pgtype.UUID{}
		if in.SLAPolicyUUID.Value != nil {
			if _, err := s.q.GetSLAPolicyByUUID(ctx, db.GetSLAPolicyByUUIDParams{OrganizationID: org, Uuid: *in.SLAPolicyUUID.Value}); err != nil {
				if !errors.Is(err, pgx.ErrNoRows) {
					return next, err
				}
				ve.add("sla_policy_uuid", "not_found", "SLA policy not found")
			}
			next.SlaPolicyID = pgtype.UUID{Bytes: *in.SLAPolicyUUID.Value, Valid: true}
		}
	}
	if in.InheritHours {
		next.Timezone, next.WorkingHours = pgtype.Text{}, nil
	} else {
		if in.Timezone != nil {
			tz := strings.TrimSpace(*in.Timezone)
			if _, err := time.LoadLocation(tz); tz == "" || err != nil {
				ve.add("timezone", "invalid", "unknown timezone")
			}
			next.Timezone = pgtype.Text{String: tz, Valid: tz != ""}
		}
		if in.WorkingHours != nil {
			raw, ok := hours(in.WorkingHours, ve)
			if ok {
				next.WorkingHours = raw
			}
		}
	}
	if in.AI != nil {
		if in.AI.Enabled != nil {
			next.AiEnabled = *in.AI.Enabled
		}
		if in.AI.Instructions != nil {
			next.AiInstructions = strings.TrimSpace(*in.AI.Instructions)
		}
		if in.AI.RoutingHint != nil {
			next.RoutingHint = strings.TrimSpace(*in.AI.RoutingHint)
		}
	}
	if utf8.RuneCountInString(next.AiInstructions) > 4000 {
		ve.add("ai.instructions", "too_long", "instructions must be at most 4000 characters")
	}
	if utf8.RuneCountInString(next.RoutingHint) > 1000 {
		ve.add("ai.routing_hint", "too_long", "routing hint must be at most 1000 characters")
	}
	return next, ve.orNil()
}

func hours(h *model.WorkingHours, ve *ValidationError) ([]byte, bool) {
	seen := map[string]bool{}
	ok := true
	for i, d := range h.Days {
		field := fmt.Sprintf("working_hours.days[%d]", i)
		if !slices.Contains(weekdays, d.Day) || seen[d.Day] {
			ve.add(field+".day", "invalid", "day must be one of mon..sun, once")
			ok = false
			continue
		}
		seen[d.Day] = true
		if d.Open && (!hhmmRE.MatchString(d.Start) || !hhmmRE.MatchString(d.End) || d.Start >= d.End) {
			ve.add(field, "invalid", "start and end must be HH:MM with start before end")
			ok = false
		}
	}
	if !ok {
		return nil, false
	}
	raw, err := json.Marshal(h)
	if err != nil {
		return nil, false
	}
	return raw, true
}

// Delete removes a department. Knowledge it owns blocks the delete unless
// moveTo is set: Set with a nil Value makes it organization-wide, a uuid
// moves it to that department. Teams and conversations lose the department.
func (s *Service) Delete(ctx context.Context, org int64, id uuid.UUID, moveTo deptscope.Optional) error {
	cur, err := s.q.GetDepartmentByUUID(ctx, db.GetDepartmentByUUIDParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ErrNotFound
	}
	if err != nil {
		return err
	}
	target := pgtype.Int8{}
	if moveTo.Set && moveTo.Value != nil {
		if *moveTo.Value == id {
			return &ValidationError{Details: []FieldError{{Field: "move_to", Code: "invalid", Message: "cannot move knowledge to the department being deleted"}}}
		}
		t, err := s.q.GetDepartmentByUUID(ctx, db.GetDepartmentByUUIDParams{OrganizationID: org, Uuid: *moveTo.Value})
		if errors.Is(err, pgx.ErrNoRows) {
			return &ValidationError{Details: []FieldError{{Field: "move_to", Code: "not_found", Message: "department not found"}}}
		}
		if err != nil {
			return err
		}
		target = pgtype.Int8{Int64: t.ID, Valid: true}
	}
	return s.withTx(ctx, func(q *db.Queries) error {
		usage, err := q.DepartmentUsage(ctx, db.DepartmentUsageParams{OrganizationID: org, DepartmentID: pgtype.Int8{Int64: cur.ID, Valid: true}})
		if err != nil {
			return err
		}
		u := model.Usage{Articles: usage.Articles, Sources: usage.Sources, ContextSections: usage.ContextSections, Suggestions: usage.Suggestions}
		if u.Total() > 0 {
			if !moveTo.Set {
				return &model.InUseError{Usage: u}
			}
			from := pgtype.Int8{Int64: cur.ID, Valid: true}
			if err := q.MoveDepartmentArticles(ctx, db.MoveDepartmentArticlesParams{OrganizationID: org, FromDepartmentID: from, ToDepartmentID: target}); err != nil {
				return err
			}
			if err := q.MoveDepartmentSources(ctx, db.MoveDepartmentSourcesParams{OrganizationID: org, FromDepartmentID: from, ToDepartmentID: target}); err != nil {
				return err
			}
			if err := q.MoveDepartmentContext(ctx, db.MoveDepartmentContextParams{OrganizationID: org, FromDepartmentID: from, ToDepartmentID: target}); err != nil {
				return err
			}
			if err := q.MoveDepartmentSuggestions(ctx, db.MoveDepartmentSuggestionsParams{OrganizationID: org, FromDepartmentID: from, ToDepartmentID: target}); err != nil {
				return err
			}
		}
		n, err := q.DeleteDepartment(ctx, db.DeleteDepartmentParams{OrganizationID: org, ID: cur.ID})
		if err != nil {
			return mapDBError(err)
		}
		if n == 0 {
			return model.ErrNotFound
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// Sharing

// resource resolves a knowledge item of the organization.
func (s *Service) resource(ctx context.Context, org int64, kind string, id uuid.UUID) (int64, pgtype.Int8, error) {
	var (
		rid  int64
		dept pgtype.Int8
		err  error
	)
	switch kind {
	case model.KindArticle:
		var r db.KnowledgeArticleDepartmentRow
		r, err = s.q.KnowledgeArticleDepartment(ctx, db.KnowledgeArticleDepartmentParams{OrganizationID: org, Uuid: id})
		rid, dept = r.ID, r.DepartmentID
	case model.KindSource:
		var r db.KnowledgeSourceDepartmentRow
		r, err = s.q.KnowledgeSourceDepartment(ctx, db.KnowledgeSourceDepartmentParams{OrganizationID: org, Uuid: id})
		rid, dept = r.ID, r.DepartmentID
	case model.KindContext:
		var r db.BusinessContextDepartmentRow
		r, err = s.q.BusinessContextDepartment(ctx, db.BusinessContextDepartmentParams{OrganizationID: org, Uuid: id})
		rid, dept = r.ID, r.DepartmentID
	default:
		return 0, pgtype.Int8{}, &ValidationError{Details: []FieldError{{Field: "resource_kind", Code: "invalid", Message: "resource_kind must be article, source or context"}}}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, pgtype.Int8{}, model.ErrResourceNotFound
	}
	return rid, dept, err
}

// Sees reports whether the scope sees one knowledge item (single reads).
func Sees(ctx context.Context, q *db.Queries, org int64, scope deptscope.Scope, kind string, resourceID int64, department pgtype.Int8) (bool, error) {
	if !scope.Restrict || !department.Valid {
		return true, nil
	}
	_, ids := scope.Args()
	return q.DepartmentVisible(ctx, db.DepartmentVisibleParams{
		OrganizationID: org, ResourceKind: kind, ResourceID: resourceID, DepartmentID: department, ViewerDepartments: ids,
	})
}

// Share shares one item of a department with the department target. Only a
// lead of the owning department (or a manager) may share it.
func (s *Service) Share(ctx context.Context, org int64, actor int64, viewer Viewer, target uuid.UUID, in model.ShareInput) (model.Share, error) {
	to, err := s.q.GetDepartmentByUUID(ctx, db.GetDepartmentByUUIDParams{OrganizationID: org, Uuid: target})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Share{}, model.ErrNotFound
	}
	if err != nil {
		return model.Share{}, err
	}
	rid, owner, err := s.resource(ctx, org, in.ResourceKind, in.ResourceUUID)
	if err != nil {
		return model.Share{}, err
	}
	if !owner.Valid {
		return model.Share{}, &ValidationError{Details: []FieldError{{Field: "resource_uuid", Code: "organization_wide", Message: "organization-wide knowledge is already visible in every department"}}}
	}
	if owner.Int64 == to.ID {
		return model.Share{}, &ValidationError{Details: []FieldError{{Field: "resource_uuid", Code: "same_department", Message: "the item already belongs to this department"}}}
	}
	if !viewer.CanShare(owner.Int64) {
		return model.Share{}, model.ErrForbidden
	}
	row, err := s.q.CreateDepartmentShare(ctx, db.CreateDepartmentShareParams{
		OrganizationID: org, DepartmentID: to.ID, ResourceKind: in.ResourceKind, ResourceID: rid,
		SharedBy: pgtype.Int8{Int64: actor, Valid: actor > 0},
	})
	if err != nil {
		return model.Share{}, err
	}
	list, err := s.Shares(ctx, org, target)
	if err != nil {
		return model.Share{}, err
	}
	for _, sh := range list {
		if sh.UUID == row.Uuid {
			return sh, nil
		}
	}
	return model.Share{}, model.ErrNotFound
}

// Unshare removes a share of the department. A lead of the owning or the
// receiving department (or a manager) may remove it.
func (s *Service) Unshare(ctx context.Context, org int64, viewer Viewer, department, share uuid.UUID) error {
	d, err := s.q.GetDepartmentByUUID(ctx, db.GetDepartmentByUUIDParams{OrganizationID: org, Uuid: department})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ErrNotFound
	}
	if err != nil {
		return err
	}
	row, err := s.q.GetDepartmentShare(ctx, db.GetDepartmentShareParams{OrganizationID: org, Uuid: share})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.DepartmentID != d.ID) {
		return model.ErrNotFound
	}
	if err != nil {
		return err
	}
	allowed := viewer.CanShare(d.ID)
	if !allowed {
		owner, err := s.ownerOf(ctx, org, row.ResourceKind, row.ResourceID)
		if err != nil {
			return err
		}
		allowed = owner.Valid && viewer.CanShare(owner.Int64)
	}
	if !allowed {
		return model.ErrForbidden
	}
	_, err = s.q.DeleteDepartmentShare(ctx, db.DeleteDepartmentShareParams{OrganizationID: org, ID: row.ID})
	return err
}

func (s *Service) ownerOf(ctx context.Context, org int64, kind string, id int64) (pgtype.Int8, error) {
	dept, err := s.q.KnowledgeOwnerDepartment(ctx, db.KnowledgeOwnerDepartmentParams{OrganizationID: org, ResourceKind: kind, ResourceID: id})
	return pgtype.Int8{Int64: dept, Valid: dept > 0}, err
}

// Shares lists the items shared with the department and the department's
// items shared with others.
func (s *Service) Shares(ctx context.Context, org int64, department uuid.UUID) ([]model.Share, error) {
	d, err := s.q.GetDepartmentByUUID(ctx, db.GetDepartmentByUUIDParams{OrganizationID: org, Uuid: department})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListDepartmentShares(ctx, db.ListDepartmentSharesParams{OrganizationID: org, DepartmentID: d.ID})
	if err != nil {
		return nil, err
	}
	out := make([]model.Share, 0, len(rows))
	for _, r := range rows {
		sh := model.Share{
			UUID: r.Uuid, ResourceKind: r.ResourceKind, ResourceUUID: r.ResourceUuid, ResourceTitle: r.ResourceTitle,
			Department: model.Ref{UUID: r.DepartmentUuid, Name: r.DepartmentName}, CreatedAt: r.CreatedAt.Time,
		}
		if r.OwnerUuid.Valid {
			sh.Owner = &model.Ref{UUID: uuid.UUID(r.OwnerUuid.Bytes), Name: r.OwnerName.String}
		}
		out = append(out, sh)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Conversations

// Widget lists the departments the widget of a brand offers.
func (s *Service) Widget(ctx context.Context, org int64, brand pgtype.UUID) ([]model.WidgetDepartment, error) {
	rows, err := s.q.ListWidgetDepartments(ctx, db.ListWidgetDepartmentsParams{OrganizationID: org, BrandID: brand})
	if err != nil {
		return nil, err
	}
	out := make([]model.WidgetDepartment, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.WidgetDepartment{UUID: r.Uuid, Name: r.Name, Description: r.Description, Color: r.Color})
	}
	return out, nil
}

// PickFromWidget sets the department the customer picked in the widget on a
// new conversation. The department must be one the widget offers.
func (s *Service) PickFromWidget(ctx context.Context, org, conversationID int64, department uuid.UUID) error {
	conv, err := s.q.GetConversationDepartmentByID(ctx, db.GetConversationDepartmentByIDParams{OrganizationID: org, ID: conversationID})
	if err != nil {
		return err
	}
	if conv.DepartmentRoutedAt.Valid {
		return nil
	}
	offered, err := s.Widget(ctx, org, conv.BrandID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(offered, func(d model.WidgetDepartment) bool { return d.UUID == department }) {
		return model.ErrNotFound
	}
	d, err := s.q.GetDepartmentByUUID(ctx, db.GetDepartmentByUUIDParams{OrganizationID: org, Uuid: department})
	if err != nil {
		return err
	}
	return s.move(ctx, org, conv, d.ID, model.SourceCustomer, nil, 0, convusecase.Actor{}, "", "")
}

// Route predicts the department of a conversation's first message once
// (customer did not pick one). The AI must name a known department with at
// least PredictThreshold confidence; otherwise the conversation stays
// organization-wide for an agent to transfer.
func (s *Service) Route(ctx context.Context, org, conversationID int64, transcript, locale string) (*model.Prediction, error) {
	conv, err := s.q.GetConversationDepartmentByID(ctx, db.GetConversationDepartmentByIDParams{OrganizationID: org, ID: conversationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil || conv.DepartmentID.Valid || conv.DepartmentRoutedAt.Valid {
		return nil, err
	}
	rows, err := s.q.ListRoutingDepartments(ctx, org)
	if err != nil {
		return nil, err
	}
	candidates := []model.Candidate{}
	ids := map[uuid.UUID]int64{}
	for _, r := range rows {
		if r.BrandID.Valid && (!conv.BrandID.Valid || r.BrandID != conv.BrandID) {
			continue
		}
		candidates = append(candidates, model.Candidate{UUID: r.Uuid, Name: r.Name, Description: r.Description, RoutingHint: r.RoutingHint})
		ids[r.Uuid] = r.ID
	}
	if len(candidates) == 0 || s.Classifier == nil {
		// Nothing to route to: no write, a later first message may still be
		// routed once departments exist.
		return nil, nil
	}
	// Claim the one-time routing (another worker may be on it).
	n, err := s.q.MarkConversationRouted(ctx, db.MarkConversationRoutedParams{OrganizationID: org, ID: conv.ID})
	if err != nil || n == 0 {
		return nil, err
	}
	pred, err := s.Classifier.ClassifyDepartment(ctx, org, conv.ID, transcript, locale, candidates)
	if err != nil {
		s.Log.WarnContext(ctx, "department_prediction_failed", "organization_id", org, "conversation_id", conv.ID, "error", err)
		return nil, nil
	}
	if pred.Confidence < 0 || pred.Confidence > 1 {
		pred.Confidence = 0
	}
	if pred.DepartmentUUID == nil {
		return &pred, nil
	}
	id, ok := ids[*pred.DepartmentUUID]
	if !ok {
		pred.DepartmentUUID = nil
		return &pred, nil
	}
	if pred.Confidence < PredictThreshold {
		return &pred, nil
	}
	reason := clip(pred.Reason, 1000)
	if err := s.move(ctx, org, conv, id, model.SourceAI, &pred.Confidence, 0, convusecase.Actor{}, reason, ""); err != nil {
		return nil, err
	}
	return &pred, nil
}

// Actor is who transfers a conversation.
type Actor struct {
	// Kind is agent or ai.
	Kind   string
	UserID int64
}

// Transfer moves a conversation to another department (or
// organization-wide) with the context the receiving department needs: the
// reason and a summary land in the timeline and the transfer history. A team
// of another department is unassigned so the new department's routing
// picks the conversation up.
func (s *Service) Transfer(ctx context.Context, org int64, conversation uuid.UUID, actor Actor, in model.TransferInput) (model.ConversationDepartment, error) {
	in.Reason, in.ContextSummary = strings.TrimSpace(in.Reason), strings.TrimSpace(in.ContextSummary)
	ve := &ValidationError{}
	if utf8.RuneCountInString(in.Reason) > 1000 {
		ve.add("reason", "too_long", "reason must be at most 1000 characters")
	}
	if utf8.RuneCountInString(in.ContextSummary) > 4000 {
		ve.add("context_summary", "too_long", "context summary must be at most 4000 characters")
	}
	if actor.Kind != model.SourceAgent && actor.Kind != model.SourceAI {
		ve.add("actor", "invalid", "actor must be an agent or the AI")
	}
	if err := ve.orNil(); err != nil {
		return model.ConversationDepartment{}, err
	}
	conv, err := s.q.GetConversationDepartment(ctx, db.GetConversationDepartmentParams{OrganizationID: org, Uuid: conversation})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ConversationDepartment{}, model.ErrConversationNotFound
	}
	if err != nil {
		return model.ConversationDepartment{}, err
	}
	target := int64(0)
	if in.DepartmentUUID != nil {
		d, err := s.q.GetDepartmentByUUID(ctx, db.GetDepartmentByUUIDParams{OrganizationID: org, Uuid: *in.DepartmentUUID})
		if errors.Is(err, pgx.ErrNoRows) {
			return model.ConversationDepartment{}, &ValidationError{Details: []FieldError{{Field: "department_uuid", Code: "not_found", Message: "department not found"}}}
		}
		if err != nil {
			return model.ConversationDepartment{}, err
		}
		target = d.ID
	}
	if (target == 0 && !conv.DepartmentID.Valid) || (conv.DepartmentID.Valid && conv.DepartmentID.Int64 == target) {
		return s.Conversation(ctx, org, conversation)
	}
	row := db.GetConversationDepartmentByIDRow(conv)
	if err := s.move(ctx, org, row, target, actor.Kind, nil, actor.UserID, convusecase.Actor{ID: actor.UserID}, in.Reason, in.ContextSummary); err != nil {
		return model.ConversationDepartment{}, err
	}
	if err := s.handOver(ctx, org, conv.Uuid, conv.AssigneeTeamID, target, actor, in.ReleaseAssignee); err != nil {
		return model.ConversationDepartment{}, err
	}
	return s.Conversation(ctx, org, conversation)
}

// handOver releases the team (and the agent when asked) that does not belong
// to the receiving department.
func (s *Service) handOver(ctx context.Context, org int64, conv uuid.UUID, team pgtype.Int8, target int64, actor Actor, release bool) error {
	if s.Conversations == nil {
		return nil
	}
	in := convusecase.UpdateInput{OrganizationID: org, ConversationUUID: conv, Actor: convusecase.Actor{ID: actor.UserID}}
	changed := false
	if team.Valid && target > 0 {
		t, err := s.q.TeamDepartmentByID(ctx, db.TeamDepartmentByIDParams{OrganizationID: org, ID: team.Int64})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if !t.DepartmentID.Valid || t.DepartmentID.Int64 != target {
			var none *uuid.UUID
			in.Team, changed = &none, true
		}
	}
	if release {
		var none *uuid.UUID
		in.Assignee, changed = &none, true
	}
	if !changed {
		return nil
	}
	_, err := s.Conversations.Update(ctx, in)
	return err
}

// move sets the department of a conversation and records the change.
func (s *Service) move(ctx context.Context, org int64, conv db.GetConversationDepartmentByIDRow, target int64, source string, confidence *float64, actorUser int64, actor convusecase.Actor, reason, summary string) error {
	dept := pgtype.Int8{Int64: target, Valid: target > 0}
	conf := pgtype.Float4{}
	if confidence != nil {
		conf = pgtype.Float4{Float32: float32(*confidence), Valid: true}
	}
	err := s.withTx(ctx, func(q *db.Queries) error {
		if _, err := q.SetConversationDepartment(ctx, db.SetConversationDepartmentParams{
			OrganizationID: org, ID: conv.ID, DepartmentID: dept,
			DepartmentSource: pgtype.Text{String: source, Valid: dept.Valid}, DepartmentConfidence: conf,
		}); err != nil {
			return err
		}
		_, err := q.InsertDepartmentTransfer(ctx, db.InsertDepartmentTransferParams{
			OrganizationID: org, ConversationID: conv.ID, FromDepartmentID: conv.DepartmentID, ToDepartmentID: dept,
			ActorKind: source, ActorUserID: pgtype.Int8{Int64: actorUser, Valid: actorUser > 0}, Confidence: conf,
			Reason: reason, ContextSummary: summary,
		})
		return err
	})
	if err != nil {
		return err
	}
	if s.Conversations != nil {
		refs, err := Refs(ctx, s.q, org, compact(conv.DepartmentID, dept))
		if err != nil {
			return err
		}
		attrs := map[string]any{
			"event": "department_transferred", "source": source, "from": refs[conv.DepartmentID.Int64].Name, "to": refs[target].Name,
			"reason": reason, "context_summary": summary,
		}
		if confidence != nil {
			attrs["confidence"] = *confidence
		}
		if err := s.Conversations.SystemEvent(ctx, org, conv.ID, actor, attrs); err != nil {
			s.Log.WarnContext(ctx, "department_timeline_failed", "organization_id", org, "conversation_id", conv.ID, "error", err)
		}
	}
	if s.SLA != nil {
		if err := s.SLA.Sync(ctx, org, conv.ID, s.Now()); err != nil {
			s.Log.WarnContext(ctx, "department_sla_sync_failed", "organization_id", org, "conversation_id", conv.ID, "error", err)
		}
	}
	return nil
}

// Conversation returns the department state and transfer history.
func (s *Service) Conversation(ctx context.Context, org int64, conversation uuid.UUID) (model.ConversationDepartment, error) {
	conv, err := s.q.GetConversationDepartment(ctx, db.GetConversationDepartmentParams{OrganizationID: org, Uuid: conversation})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ConversationDepartment{}, model.ErrConversationNotFound
	}
	if err != nil {
		return model.ConversationDepartment{}, err
	}
	out := model.ConversationDepartment{Transfers: []model.Transfer{}}
	if conv.DepartmentID.Valid {
		refs, err := Refs(ctx, s.q, org, []int64{conv.DepartmentID.Int64})
		if err != nil {
			return out, err
		}
		if r, ok := refs[conv.DepartmentID.Int64]; ok {
			out.Department = &r
		}
	}
	if conv.DepartmentSource.Valid {
		out.Source = &conv.DepartmentSource.String
	}
	if conv.DepartmentConfidence.Valid {
		out.Confidence = &conv.DepartmentConfidence.Float32
	}
	if conv.DepartmentRoutedAt.Valid {
		out.RoutedAt = &conv.DepartmentRoutedAt.Time
	}
	rows, err := s.q.ListDepartmentTransfers(ctx, db.ListDepartmentTransfersParams{OrganizationID: org, ConversationID: conv.ID})
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		t := model.Transfer{UUID: r.Uuid, ActorKind: r.ActorKind, Reason: r.Reason, ContextSummary: r.ContextSummary, CreatedAt: r.CreatedAt.Time}
		if r.FromUuid.Valid {
			t.From = &model.Ref{UUID: uuid.UUID(r.FromUuid.Bytes), Name: r.FromName.String}
		}
		if r.ToUuid.Valid {
			t.To = &model.Ref{UUID: uuid.UUID(r.ToUuid.Bytes), Name: r.ToName.String}
		}
		if r.ActorUuid.Valid {
			id := uuid.UUID(r.ActorUuid.Bytes)
			t.ActorUUID = &id
			t.ActorName = strings.TrimSpace(r.ActorName.String + " " + r.ActorSurname.String)
		}
		if r.Confidence.Valid {
			c := r.Confidence.Float32
			t.Confidence = &c
		}
		out.Transfers = append(out.Transfers, t)
	}
	return out, nil
}

// AISettings returns the AI settings of a conversation's department (ok
// false when it has none).
func (s *Service) AISettings(ctx context.Context, org int64, department pgtype.Int8) (db.GetDepartmentAIRow, bool, error) {
	if !department.Valid {
		return db.GetDepartmentAIRow{}, false, nil
	}
	row, err := s.q.GetDepartmentAI(ctx, db.GetDepartmentAIParams{OrganizationID: org, ID: department.Int64})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false, nil
	}
	return row, err == nil, err
}

// Candidates are the departments a conversation may be transferred to (AI
// transfer tool).
func (s *Service) Candidates(ctx context.Context, org int64, brand pgtype.UUID) ([]model.Candidate, error) {
	rows, err := s.q.ListRoutingDepartments(ctx, org)
	if err != nil {
		return nil, err
	}
	out := []model.Candidate{}
	for _, r := range rows {
		if r.BrandID.Valid && (!brand.Valid || r.BrandID != brand) {
			continue
		}
		out = append(out, model.Candidate{UUID: r.Uuid, Name: r.Name, Description: r.Description, RoutingHint: r.RoutingHint})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// helpers

func mapDepartment(r db.Department) model.Department {
	d := model.Department{
		ID: r.ID, UUID: r.Uuid, Name: r.Name, Description: r.Description, Color: r.Color, WidgetVisible: r.WidgetVisible,
		SortOrder: r.SortOrder, AI: model.AISettings{Enabled: r.AiEnabled, Instructions: r.AiInstructions, RoutingHint: r.RoutingHint},
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time, Teams: []model.TeamRef{}, Leads: []model.UserRef{},
	}
	if r.BrandID.Valid {
		id := uuid.UUID(r.BrandID.Bytes)
		d.BrandUUID = &id
	}
	if r.SlaPolicyID.Valid {
		id := uuid.UUID(r.SlaPolicyID.Bytes)
		d.SLAPolicyUUID = &id
	}
	if r.Timezone.Valid {
		tz := r.Timezone.String
		d.Timezone = &tz
	}
	if len(r.WorkingHours) > 0 {
		var h model.WorkingHours
		if json.Unmarshal(r.WorkingHours, &h) == nil && len(h.Days) > 0 {
			d.WorkingHours = &h
		}
	}
	return d
}

func compact(ids ...pgtype.Int8) []int64 {
	out := []int64{}
	for _, id := range ids {
		if id.Valid {
			out = append(out, id.Int64)
		}
	}
	return out
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func mapDBError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return model.ErrNameTaken
		case "23503":
			return model.ErrInUse
		}
	}
	return err
}
