// Package usecase implements teams and team membership (CF-5). Every method
// takes the organization id of the caller's orgctx.Scope.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teams/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/deptscope"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is an unknown team (or a team of another organization).
	ErrNotFound = errors.New("team not found")
	// ErrMemberNotFound is a user that is not a member of the organization
	// or the team.
	ErrMemberNotFound = errors.New("member not found")
	// ErrNameTaken is a duplicate team name in the organization.
	ErrNameTaken = errors.New("team name is already used")
)

// FieldError is one field-level validation problem.
type FieldError struct {
	Field, Code, Message string
}

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

var (
	hhmmRE  = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	colorRE = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	skillRE = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} ._+-]{0,39}$`)
)

// maxSkills bounds the skill tags of a team or member.
const maxSkills = 30

// SortColumns are the sortable list fields.
var SortColumns = map[string]string{"name": "name", "created_at": "created_at"}

// Service is the teams use case.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// New builds the service.
func New(pool *pgxpool.Pool, q *db.Queries) *Service { return &Service{pool: pool, q: q} }

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

// List lists the teams of the organization with their leads.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Team, int64, error) {
	if _, ok := SortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "name", false
	}
	var memberUserID pgtype.Int8
	if f.MemberUUID != uuid.Nil {
		id, err := s.q.GetOrganizationMemberUserID(ctx, db.GetOrganizationMemberUserIDParams{OrganizationID: orgID, UserUuid: f.MemberUUID})
		if errors.Is(err, pgx.ErrNoRows) {
			return []model.Team{}, 0, nil
		}
		if err != nil {
			return nil, 0, err
		}
		memberUserID = pgtype.Int8{Int64: id, Valid: true}
	}
	rows, err := s.q.ListTeams(ctx, db.ListTeamsParams{
		OrganizationID: orgID, Q: text(f.Q), Skill: text(f.Skill), MemberUserID: memberUserID,
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountTeams(ctx, db.CountTeamsParams{
		OrganizationID: orgID, Q: text(f.Q), Skill: text(f.Skill), MemberUserID: memberUserID,
	})
	if err != nil {
		return nil, 0, err
	}
	teams := make([]model.Team, 0, len(rows))
	for _, r := range rows {
		t := mapTeam(r.ID, r.Uuid, r.Name, r.Description, r.Color, r.Timezone, r.WorkingHours, r.Skills, r.MemberCount, r.CreatedAt, r.UpdatedAt)
		t.DepartmentID = r.DepartmentID
		teams = append(teams, t)
	}
	if err := s.attachMembers(ctx, orgID, teams, false); err != nil {
		return nil, 0, err
	}
	if err := s.attachDepartments(ctx, orgID, teams); err != nil {
		return nil, 0, err
	}
	return teams, total, nil
}

// Get returns one team with its members.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Team, error) {
	r, err := s.q.GetTeamByUUID(ctx, db.GetTeamByUUIDParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Team{}, ErrNotFound
	}
	if err != nil {
		return model.Team{}, err
	}
	teams := []model.Team{mapTeam(r.ID, r.Uuid, r.Name, r.Description, r.Color, r.Timezone, r.WorkingHours, r.Skills, r.MemberCount, r.CreatedAt, r.UpdatedAt)}
	teams[0].DepartmentID = r.DepartmentID
	if err := s.attachMembers(ctx, orgID, teams, true); err != nil {
		return model.Team{}, err
	}
	if err := s.attachDepartments(ctx, orgID, teams); err != nil {
		return model.Team{}, err
	}
	return teams[0], nil
}

// Create creates a team, optionally with its first members.
func (s *Service) Create(ctx context.Context, orgID int64, in model.TeamInput) (model.Team, error) {
	ve := &ValidationError{}
	name := ""
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	f := fields{name: name, description: deref(in.Description), color: deref(in.Color), skills: in.Skills}
	if !in.InheritHours {
		f.timezone, f.hours = in.Timezone, in.WorkingHours
	}
	params, err := s.validate(ctx, orgID, uuid.Nil, f, ve)
	if err != nil {
		return model.Team{}, err
	}
	members, err := s.resolveMembers(ctx, orgID, in.Members, ve)
	if err != nil {
		return model.Team{}, err
	}
	dept, err := s.department(ctx, orgID, in.Department, ve)
	if err != nil {
		return model.Team{}, err
	}
	if err := ve.orNil(); err != nil {
		return model.Team{}, err
	}
	var created db.Team
	err = s.withTx(ctx, func(q *db.Queries) error {
		row, err := q.CreateTeam(ctx, db.CreateTeamParams{
			OrganizationID: orgID, Name: params.name, Description: params.description, Color: params.color,
			Timezone: params.timezone, WorkingHours: params.hours, Skills: params.skills,
		})
		if err != nil {
			return err
		}
		created = row
		if dept.Valid {
			if _, err := q.SetTeamDepartment(ctx, db.SetTeamDepartmentParams{OrganizationID: orgID, Uuid: row.Uuid, DepartmentID: dept}); err != nil {
				return err
			}
		}
		for _, m := range members {
			if err := q.UpsertTeamMember(ctx, db.UpsertTeamMemberParams{
				OrganizationID: orgID, TeamID: row.ID, UserID: m.userID, Role: m.role,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Team{}, mapUnique(err)
	}
	return s.Get(ctx, orgID, created.Uuid)
}

// Update patches a team. Nil fields keep their value.
func (s *Service) Update(ctx context.Context, orgID int64, id uuid.UUID, in model.TeamInput) (model.Team, error) {
	current, err := s.q.GetTeamByUUID(ctx, db.GetTeamByUUIDParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Team{}, ErrNotFound
	}
	if err != nil {
		return model.Team{}, err
	}
	cur := mapTeam(current.ID, current.Uuid, current.Name, current.Description, current.Color, current.Timezone, current.WorkingHours, current.Skills, current.MemberCount, current.CreatedAt, current.UpdatedAt)
	f := fields{name: cur.Name, description: cur.Description, color: cur.Color, skills: cur.Skills, timezone: cur.Timezone, hours: cur.WorkingHours}
	if in.Name != nil {
		f.name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		f.description = *in.Description
	}
	if in.Color != nil {
		f.color = *in.Color
	}
	if in.Skills != nil {
		f.skills = in.Skills
	}
	if in.InheritHours {
		f.timezone, f.hours = nil, nil
	} else {
		if in.Timezone != nil {
			f.timezone = in.Timezone
		}
		if in.WorkingHours != nil {
			f.hours = in.WorkingHours
		}
	}
	ve := &ValidationError{}
	params, err := s.validate(ctx, orgID, id, f, ve)
	if err != nil {
		return model.Team{}, err
	}
	dept, err := s.department(ctx, orgID, in.Department, ve)
	if err != nil {
		return model.Team{}, err
	}
	if err := ve.orNil(); err != nil {
		return model.Team{}, err
	}
	if in.Department.Set {
		if _, err := s.q.SetTeamDepartment(ctx, db.SetTeamDepartmentParams{OrganizationID: orgID, Uuid: id, DepartmentID: dept}); err != nil {
			return model.Team{}, err
		}
	}
	if _, err := s.q.UpdateTeam(ctx, db.UpdateTeamParams{
		OrganizationID: orgID, Uuid: id, Name: params.name, Description: params.description, Color: params.color,
		Timezone: params.timezone, WorkingHours: params.hours, Skills: params.skills,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Team{}, ErrNotFound
		}
		return model.Team{}, mapUnique(err)
	}
	return s.Get(ctx, orgID, id)
}

// Delete removes a team (its memberships go with it).
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := s.q.DeleteTeam(ctx, db.DeleteTeamParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetMember adds a member to a team or changes its team role.
func (s *Service) SetMember(ctx context.Context, orgID int64, teamID uuid.UUID, in model.MemberInput) (model.Team, error) {
	team, err := s.q.GetTeamByUUID(ctx, db.GetTeamByUUIDParams{OrganizationID: orgID, Uuid: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Team{}, ErrNotFound
	}
	if err != nil {
		return model.Team{}, err
	}
	ve := &ValidationError{}
	members, err := s.resolveMembers(ctx, orgID, []model.MemberInput{in}, ve)
	if err != nil {
		return model.Team{}, err
	}
	if err := ve.orNil(); err != nil {
		return model.Team{}, err
	}
	if err := s.q.UpsertTeamMember(ctx, db.UpsertTeamMemberParams{
		OrganizationID: orgID, TeamID: team.ID, UserID: members[0].userID, Role: members[0].role,
	}); err != nil {
		return model.Team{}, err
	}
	return s.Get(ctx, orgID, teamID)
}

// RemoveMember takes a member out of a team.
func (s *Service) RemoveMember(ctx context.Context, orgID int64, teamID, userUUID uuid.UUID) (model.Team, error) {
	team, err := s.q.GetTeamByUUID(ctx, db.GetTeamByUUIDParams{OrganizationID: orgID, Uuid: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Team{}, ErrNotFound
	}
	if err != nil {
		return model.Team{}, err
	}
	userID, err := s.q.GetOrganizationMemberUserID(ctx, db.GetOrganizationMemberUserIDParams{OrganizationID: orgID, UserUuid: userUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Team{}, ErrMemberNotFound
	}
	if err != nil {
		return model.Team{}, err
	}
	n, err := s.q.DeleteTeamMember(ctx, db.DeleteTeamMemberParams{OrganizationID: orgID, TeamID: team.ID, UserID: userID})
	if err != nil {
		return model.Team{}, err
	}
	if n == 0 {
		return model.Team{}, ErrMemberNotFound
	}
	return s.Get(ctx, orgID, teamID)
}

// SetMemberSkills replaces the skill tags of an organization member.
func (s *Service) SetMemberSkills(ctx context.Context, orgID int64, userUUID uuid.UUID, skills []string) ([]string, error) {
	ve := &ValidationError{}
	clean := normalizeSkills(skills, "skills", ve)
	if err := ve.orNil(); err != nil {
		return nil, err
	}
	userID, err := s.q.GetOrganizationMemberUserID(ctx, db.GetOrganizationMemberUserIDParams{OrganizationID: orgID, UserUuid: userUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMemberNotFound
	}
	if err != nil {
		return nil, err
	}
	if _, err := s.q.SetMemberSkills(ctx, db.SetMemberSkillsParams{OrganizationID: orgID, UserID: userID, Skills: clean}); err != nil {
		return nil, err
	}
	return clean, nil
}

// Skills lists every skill tag used in the organization.
func (s *Service) Skills(ctx context.Context, orgID int64) ([]string, error) {
	out, err := s.q.ListOrganizationSkills(ctx, orgID)
	if out == nil {
		out = []string{}
	}
	return out, err
}

// TeamIDsForUser returns the teams of a member. Modules use it to limit
// conversations / tickets of members without `.read_all` to their teams.
func (s *Service) TeamIDsForUser(ctx context.Context, orgID, userID int64) ([]int64, error) {
	return s.q.ListTeamIDsForUser(ctx, db.ListTeamIDsForUserParams{OrganizationID: orgID, UserID: userID})
}

// --- helpers ---------------------------------------------------------------

type fields struct {
	name, description, color string
	timezone                 *string
	hours                    *model.WorkingHours
	skills                   []string
}

type validated struct {
	name, description, color string
	timezone                 pgtype.Text
	hours                    []byte
	skills                   []string
}

func (s *Service) validate(ctx context.Context, orgID int64, self uuid.UUID, f fields, ve *ValidationError) (validated, error) {
	out := validated{name: f.name, description: strings.TrimSpace(f.description), color: strings.TrimSpace(f.color)}
	if out.name == "" || utf8.RuneCountInString(out.name) > 120 {
		ve.add("name", "required", "name is required (max 120 characters)")
	} else {
		taken, err := s.q.TeamNameTaken(ctx, db.TeamNameTakenParams{OrganizationID: orgID, Name: out.name, ExceptUuid: self})
		if err != nil {
			return out, err
		}
		if taken {
			ve.add("name", "taken", "another team already uses this name")
		}
	}
	if utf8.RuneCountInString(out.description) > 1000 {
		ve.add("description", "too_long", "description is longer than 1000 characters")
	}
	if out.color != "" && !colorRE.MatchString(out.color) {
		ve.add("color", "invalid", "color must be #RRGGBB")
	}
	if f.timezone != nil && strings.TrimSpace(*f.timezone) != "" {
		tz := strings.TrimSpace(*f.timezone)
		if _, err := time.LoadLocation(tz); err != nil {
			ve.add("timezone", "invalid", "timezone must be an IANA time zone")
		}
		out.timezone = pgtype.Text{String: tz, Valid: true}
	}
	if f.hours != nil {
		validateHours(*f.hours, ve)
		raw, err := json.Marshal(f.hours)
		if err != nil {
			return out, err
		}
		out.hours = raw
	}
	out.skills = normalizeSkills(f.skills, "skills", ve)
	return out, nil
}

type resolvedMember struct {
	userID int64
	role   string
}

func (s *Service) resolveMembers(ctx context.Context, orgID int64, in []model.MemberInput, ve *ValidationError) ([]resolvedMember, error) {
	out := make([]resolvedMember, 0, len(in))
	seen := map[uuid.UUID]bool{}
	for i, m := range in {
		field := fmt.Sprintf("members.%d", i)
		role := strings.TrimSpace(m.Role)
		if role == "" {
			role = model.RoleMember
		}
		if role != model.RoleLead && role != model.RoleMember {
			ve.add(field+".role", "invalid", "role must be lead or member")
			continue
		}
		if seen[m.UserUUID] {
			continue
		}
		seen[m.UserUUID] = true
		id, err := s.q.GetOrganizationMemberUserID(ctx, db.GetOrganizationMemberUserIDParams{OrganizationID: orgID, UserUuid: m.UserUUID})
		if errors.Is(err, pgx.ErrNoRows) {
			// Same answer for unknown users and members of other organizations.
			ve.add(field+".user_uuid", "not_found", "user is not a member of the organization")
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, resolvedMember{userID: id, role: role})
	}
	return out, nil
}

func (s *Service) attachMembers(ctx context.Context, orgID int64, teams []model.Team, full bool) error {
	if len(teams) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(teams))
	idx := map[int64]int{}
	for i, t := range teams {
		ids = append(ids, t.ID)
		idx[t.ID] = i
	}
	rows, err := s.q.ListTeamMembers(ctx, db.ListTeamMembersParams{OrganizationID: orgID, TeamIds: ids})
	if err != nil {
		return err
	}
	for _, r := range rows {
		t := &teams[idx[r.TeamID]]
		user := model.MemberUser{UUID: r.UserUuid, Email: r.Email, Name: r.Name, Surname: r.Surname, Status: r.Status}
		if r.Role == model.RoleLead {
			t.Leads = append(t.Leads, user)
		}
		if full {
			skills := r.Skills
			if skills == nil {
				skills = []string{}
			}
			t.Members = append(t.Members, model.TeamMember{User: user, Role: r.Role, Skills: skills, JoinedAt: r.CreatedAt.Time.UTC()})
		}
	}
	for i := range teams {
		if teams[i].Leads == nil {
			teams[i].Leads = []model.MemberUser{}
		}
		if full && teams[i].Members == nil {
			teams[i].Members = []model.TeamMember{}
		}
	}
	return nil
}

func validateHours(h model.WorkingHours, ve *ValidationError) {
	if len(h.Days) != len(model.Weekdays) {
		ve.add("working_hours.days", "invalid", "days must list the seven weekdays, Monday first")
		return
	}
	for i, d := range h.Days {
		prefix := fmt.Sprintf("working_hours.days.%d", i)
		if d.Day != model.Weekdays[i] {
			ve.add(prefix+".day", "invalid", "day must be "+model.Weekdays[i])
		}
		if !hhmmRE.MatchString(d.Start) {
			ve.add(prefix+".start", "invalid", "start must be HH:MM")
		}
		if !hhmmRE.MatchString(d.End) {
			ve.add(prefix+".end", "invalid", "end must be HH:MM")
		}
		if d.Open && hhmmRE.MatchString(d.Start) && hhmmRE.MatchString(d.End) && d.End <= d.Start {
			ve.add(prefix+".end", "invalid", "end must be after start")
		}
	}
}

// normalizeSkills trims, lower-cases, de-duplicates and sorts skill tags.
func normalizeSkills(in []string, field string, ve *ValidationError) []string {
	out := []string{}
	for i, raw := range in {
		v := strings.ToLower(strings.TrimSpace(raw))
		if v == "" {
			continue
		}
		if !skillRE.MatchString(v) {
			ve.add(fmt.Sprintf("%s.%d", field, i), "invalid", "skill must be 1-40 letters, digits, space . _ + -")
			continue
		}
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	if len(out) > maxSkills {
		ve.add(field, "too_many", fmt.Sprintf("at most %d skills", maxSkills))
	}
	sort.Strings(out)
	return out
}

// department resolves the department of a team write (CF-215).
func (s *Service) department(ctx context.Context, orgID int64, opt deptscope.Optional, ve *ValidationError) (pgtype.Int8, error) {
	if !opt.Set || opt.Value == nil {
		return pgtype.Int8{}, nil
	}
	d, err := s.q.GetDepartmentByUUID(ctx, db.GetDepartmentByUUIDParams{OrganizationID: orgID, Uuid: *opt.Value})
	if errors.Is(err, pgx.ErrNoRows) {
		ve.add("department_uuid", "not_found", "department not found")
		return pgtype.Int8{}, nil
	}
	if err != nil {
		return pgtype.Int8{}, err
	}
	return pgtype.Int8{Int64: d.ID, Valid: true}, nil
}

// attachDepartments fills the department reference of each team.
func (s *Service) attachDepartments(ctx context.Context, orgID int64, teams []model.Team) error {
	ids := []int64{}
	for _, t := range teams {
		if t.DepartmentID.Valid {
			ids = append(ids, t.DepartmentID.Int64)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.q.ListDepartmentRefs(ctx, db.ListDepartmentRefsParams{OrganizationID: orgID, Ids: ids})
	if err != nil {
		return err
	}
	refs := map[int64]model.DepartmentRef{}
	for _, r := range rows {
		refs[r.ID] = model.DepartmentRef{UUID: r.Uuid, Name: r.Name, Color: r.Color}
	}
	for i, t := range teams {
		if ref, ok := refs[t.DepartmentID.Int64]; ok && t.DepartmentID.Valid {
			teams[i].Department = &ref
		}
	}
	return nil
}

func mapTeam(id int64, u uuid.UUID, name, desc, color string, tz pgtype.Text, hours []byte, skills []string, members int64, created, updated pgtype.Timestamptz) model.Team {
	t := model.Team{
		ID: id, UUID: u, Name: name, Description: desc, Color: color, Skills: skills, MemberCount: members,
		CreatedAt: created.Time.UTC(), UpdatedAt: updated.Time.UTC(),
	}
	if t.Skills == nil {
		t.Skills = []string{}
	}
	if tz.Valid {
		v := tz.String
		t.Timezone = &v
	}
	if len(hours) > 0 {
		var h model.WorkingHours
		if json.Unmarshal(hours, &h) == nil && len(h.Days) == len(model.Weekdays) {
			t.WorkingHours = &h
		}
	}
	return t
}

func mapUnique(err error) error {
	if err != nil && strings.Contains(err.Error(), "uq_teams_organization_name") {
		return ErrNameTaken
	}
	return err
}

func text(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
