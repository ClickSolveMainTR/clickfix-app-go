// Package usecase manages canned responses and macros (CF-48): reusable
// answers with variables rendered for a conversation, and one-click action
// lists (reply, note, status, priority, assign, tag) applied through the
// inbox core so every change lands on the timeline and the outbox.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors returned by the service.
var (
	ErrNotFound       = errors.New("macros: not found")
	ErrInvalid        = errors.New("macros: invalid request")
	ErrForbidden      = errors.New("macros: not allowed")
	ErrShortcutExists = errors.New("macros: shortcut already used")
)

// Scopes of canned responses and macros.
const (
	ScopePersonal     = "personal"
	ScopeTeam         = "team"
	ScopeOrganization = "organization"
)

// Macro action types.
const (
	ActionReply    = "reply"
	ActionNote     = "note"
	ActionStatus   = "status"
	ActionPriority = "priority"
	ActionAssign   = "assign"
	ActionTag      = "tag"
	ActionUntag    = "untag"
)

var actionTypes = []string{ActionReply, ActionNote, ActionStatus, ActionPriority, ActionAssign, ActionTag, ActionUntag}

// Viewer is the agent using canned responses.
type Viewer struct {
	OrganizationID int64
	UserID         int64
	UserUUID       uuid.UUID
	// CanWrite: tenant.macros.write (organization wide items).
	CanWrite bool
}

// Canned is the API shape of a canned response.
type Canned struct {
	UUID       uuid.UUID  `json:"uuid"`
	Scope      string     `json:"scope"`
	Title      string     `json:"title"`
	Shortcut   string     `json:"shortcut"`
	Category   string     `json:"category"`
	Content    string     `json:"content"`
	UsageCount int32      `json:"usage_count"`
	LastUsedAt *time.Time `json:"last_used_at"`
	Mine       bool       `json:"mine"`
}

// Action is one step of a macro.
type Action struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Macro is the API shape of a macro.
type Macro struct {
	UUID        uuid.UUID  `json:"uuid"`
	Scope       string     `json:"scope"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Actions     []Action   `json:"actions"`
	UsageCount  int32      `json:"usage_count"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	Mine        bool       `json:"mine"`
}

// Service manages canned responses and macros.
type Service struct {
	q      *db.Queries
	inbox  *convusecase.Service
	fields FieldSource
}

// FieldSource lists custom field definitions (customfields usecase).
type FieldSource interface {
	Definitions(ctx context.Context, orgID int64, entity string, objectTypeID *int64) ([]customfields.Definition, error)
}

// WithFields renders custom field variables through the catalog (CF-196):
// list fields show their option label, sensitive and agent-hidden fields
// render empty, and {{contact.<key>}} / {{company.<key>}} work besides
// {{contact.custom.<key>}}.
func (s *Service) WithFields(f FieldSource) *Service { s.fields = f; return s }

// New builds the service.
func New(q *db.Queries, inbox *convusecase.Service) *Service {
	return &Service{q: q, inbox: inbox}
}

// ---------------------------------------------------------------------------
// Canned responses

// ListCanned returns the canned responses the viewer may use, most used first.
func (s *Service) ListCanned(ctx context.Context, v Viewer, q string) ([]Canned, error) {
	var qt pgtype.Text
	if q = strings.TrimSpace(strings.TrimPrefix(q, "/")); q != "" {
		qt = pgtype.Text{String: q, Valid: true}
	}
	rows, err := s.q.ListCannedResponses(ctx, db.ListCannedResponsesParams{OrganizationID: v.OrganizationID, ViewerID: v.UserID, Q: qt})
	if err != nil {
		return nil, err
	}
	out := make([]Canned, 0, len(rows))
	for _, r := range rows {
		out = append(out, cannedOf(r, v))
	}
	return out, nil
}

// CannedInput creates or edits a canned response.
type CannedInput struct {
	Scope    string
	Title    string
	Shortcut string
	Category string
	Content  string
}

var shortcutRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

func (in *CannedInput) normalize() error {
	in.Title = strings.TrimSpace(in.Title)
	in.Shortcut = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(in.Shortcut, "/")))
	in.Category = strings.TrimSpace(in.Category)
	in.Content = strings.TrimSpace(in.Content)
	switch {
	case in.Title == "" || len([]rune(in.Title)) > 120:
		return fmt.Errorf("%w: title must be 1-120 characters", ErrInvalid)
	case !shortcutRe.MatchString(in.Shortcut):
		return fmt.Errorf("%w: shortcut must be lowercase letters, digits, - or _ (max 40)", ErrInvalid)
	case in.Content == "" || len([]rune(in.Content)) > 10000:
		return fmt.Errorf("%w: content must be 1-10000 characters", ErrInvalid)
	case len([]rune(in.Category)) > 60:
		return fmt.Errorf("%w: category is too long", ErrInvalid)
	}
	return nil
}

// CreateCanned creates a canned response. Organization scope needs write.
func (s *Service) CreateCanned(ctx context.Context, v Viewer, in CannedInput) (Canned, error) {
	if err := in.normalize(); err != nil {
		return Canned{}, err
	}
	scope, owner, err := scopeOf(in.Scope, v)
	if err != nil {
		return Canned{}, err
	}
	row, err := s.q.InsertCannedResponse(ctx, db.InsertCannedResponseParams{
		OrganizationID: v.OrganizationID, Scope: scope, OwnerUserID: owner, Title: in.Title, Shortcut: in.Shortcut,
		Category: in.Category, Content: in.Content, CreatedBy: pgtype.Int8{Int64: v.UserID, Valid: true},
	})
	if err != nil {
		return Canned{}, unique(err)
	}
	return cannedOf(row, v), nil
}

// UpdateCanned edits a canned response the viewer may change.
func (s *Service) UpdateCanned(ctx context.Context, v Viewer, id uuid.UUID, in CannedInput) (Canned, error) {
	if err := in.normalize(); err != nil {
		return Canned{}, err
	}
	cur, err := s.editableCanned(ctx, v, id)
	if err != nil {
		return Canned{}, err
	}
	row, err := s.q.UpdateCannedResponse(ctx, db.UpdateCannedResponseParams{
		Title: in.Title, Shortcut: in.Shortcut, Category: in.Category, Content: in.Content,
		OrganizationID: v.OrganizationID, ID: cur.ID,
	})
	if err != nil {
		return Canned{}, unique(err)
	}
	return cannedOf(row, v), nil
}

// DeleteCanned deletes a canned response the viewer may change.
func (s *Service) DeleteCanned(ctx context.Context, v Viewer, id uuid.UUID) error {
	cur, err := s.editableCanned(ctx, v, id)
	if err != nil {
		return err
	}
	_, err = s.q.DeleteCannedResponse(ctx, db.DeleteCannedResponseParams{OrganizationID: v.OrganizationID, ID: cur.ID})
	return err
}

func (s *Service) editableCanned(ctx context.Context, v Viewer, id uuid.UUID) (db.CannedResponse, error) {
	row, err := s.q.GetCannedResponse(ctx, db.GetCannedResponseParams{OrganizationID: v.OrganizationID, Uuid: id, ViewerID: v.UserID})
	if err != nil {
		return db.CannedResponse{}, notFound(err)
	}
	if row.Scope != ScopePersonal && !v.CanWrite {
		return db.CannedResponse{}, ErrForbidden
	}
	return row, nil
}

// Rendered is a canned response rendered for a conversation.
type Rendered struct {
	Content string `json:"content"`
}

// Render fills the variables of a canned response for a conversation and
// counts the use.
func (s *Service) Render(ctx context.Context, v Viewer, id, conversation uuid.UUID) (Rendered, error) {
	row, err := s.q.GetCannedResponse(ctx, db.GetCannedResponseParams{OrganizationID: v.OrganizationID, Uuid: id, ViewerID: v.UserID})
	if err != nil {
		return Rendered{}, notFound(err)
	}
	vars, err := s.variables(ctx, v, conversation)
	if err != nil {
		return Rendered{}, err
	}
	_ = s.q.TouchCannedResponse(ctx, db.TouchCannedResponseParams{OrganizationID: v.OrganizationID, ID: row.ID})
	return Rendered{Content: Render(row.Content, vars)}, nil
}

func (s *Service) variables(ctx context.Context, v Viewer, conversation uuid.UUID) (map[string]string, error) {
	vars := map[string]string{}
	if u, err := s.q.GetUserByID(ctx, v.UserID); err == nil {
		vars["agent.name"] = strings.TrimSpace(u.Name + " " + u.Surname)
		vars["agent.first_name"] = u.Name
		vars["agent.email"] = u.Email
	}
	if conversation == uuid.Nil {
		return vars, nil
	}
	d, err := s.inbox.Get(ctx, v.OrganizationID, v.UserID, conversation)
	if err != nil {
		if errors.Is(err, convusecase.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	vars["conversation.number"] = fmt.Sprint(d.Number)
	vars["conversation.subject"] = d.Subject
	vars["contact.name"] = d.Contact.Name
	vars["contact.first_name"] = firstName(d.Contact.Name)
	if d.Contact.Email != nil {
		vars["contact.email"] = *d.Contact.Email
	}
	if d.Contact.Phone != nil {
		vars["contact.phone"] = *d.Contact.Phone
	}
	if d.Company != nil {
		vars["company.name"] = d.Company.Name
	}
	if c, err := s.q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: v.OrganizationID, Uuid: d.Contact.UUID}); err == nil {
		var custom map[string]any
		if json.Unmarshal(c.CustomFields, &custom) == nil {
			if err := s.customVars(ctx, v.OrganizationID, vars, "contact", customfields.EntityContact, custom); err != nil {
				return nil, err
			}
		}
	}
	if d.Company != nil && s.fields != nil {
		if c, err := s.q.CustomFieldsOfCompany(ctx, db.CustomFieldsOfCompanyParams{OrganizationID: v.OrganizationID, Uuid: d.Company.UUID}); err == nil {
			var custom map[string]any
			if json.Unmarshal(c.CustomFields, &custom) == nil {
				if err := s.customVars(ctx, v.OrganizationID, vars, "company", customfields.EntityCompany, custom); err != nil {
					return nil, err
				}
			}
		}
	}
	if custom, ok := d.CustomFields.(map[string]any); ok {
		for k, val := range custom {
			vars["conversation.custom."+k] = fmt.Sprint(val)
		}
	}
	return vars, nil
}

// customVars adds "<prefix>.custom.<key>" and, unless a built-in variable
// owns the name, "<prefix>.<key>" for the agent-visible, non-sensitive
// fields. Without a field source the raw stored values are used (legacy).
func (s *Service) customVars(ctx context.Context, org int64, vars map[string]string, prefix, entity string, custom map[string]any) error {
	if s.fields == nil {
		for k, val := range custom {
			vars[prefix+".custom."+k] = fmt.Sprint(val)
		}
		return nil
	}
	defs, err := s.fields.Definitions(ctx, org, entity, nil)
	if err != nil {
		return err
	}
	for k, val := range customfields.TemplateVars(prefix, defs, custom, customfields.AudienceAgent) {
		key := strings.TrimPrefix(k, prefix+".")
		vars[prefix+".custom."+key] = val
		if _, builtin := vars[k]; !builtin {
			vars[k] = val
		}
	}
	return nil
}

var varRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)

// Render replaces {{name}} variables; unknown ones become empty.
func Render(content string, vars map[string]string) string {
	return varRe.ReplaceAllStringFunc(content, func(m string) string {
		key := varRe.FindStringSubmatch(m)[1]
		return vars[key]
	})
}

func firstName(name string) string {
	if f := strings.Fields(name); len(f) > 0 {
		return f[0]
	}
	return ""
}

// ---------------------------------------------------------------------------
// Macros

// ListMacros returns the macros the viewer may run.
func (s *Service) ListMacros(ctx context.Context, v Viewer) ([]Macro, error) {
	rows, err := s.q.ListMacros(ctx, db.ListMacrosParams{OrganizationID: v.OrganizationID, ViewerID: v.UserID})
	if err != nil {
		return nil, err
	}
	out := make([]Macro, 0, len(rows))
	for _, r := range rows {
		out = append(out, macroOf(r, v))
	}
	return out, nil
}

// MacroInput creates or edits a macro.
type MacroInput struct {
	Scope       string
	Name        string
	Description string
	Actions     []Action
}

func (in *MacroInput) normalize() ([]byte, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" || len([]rune(in.Name)) > 120 {
		return nil, fmt.Errorf("%w: name must be 1-120 characters", ErrInvalid)
	}
	if len(in.Actions) == 0 || len(in.Actions) > 10 {
		return nil, fmt.Errorf("%w: a macro has 1-10 actions", ErrInvalid)
	}
	for i, a := range in.Actions {
		a.Type, a.Value = strings.TrimSpace(a.Type), strings.TrimSpace(a.Value)
		in.Actions[i] = a
		switch {
		case !convmodel.Valid(a.Type, actionTypes):
			return nil, fmt.Errorf("%w: unknown action %q", ErrInvalid, a.Type)
		case a.Type == ActionStatus && (!convmodel.Valid(a.Value, convmodel.Statuses) || a.Value == convmodel.StatusSnoozed):
			return nil, fmt.Errorf("%w: status must be open, pending, resolved or closed", ErrInvalid)
		case a.Type == ActionPriority && !convmodel.Valid(a.Value, convmodel.Priorities):
			return nil, fmt.Errorf("%w: unknown priority", ErrInvalid)
		case (a.Type == ActionReply || a.Type == ActionNote || a.Type == ActionTag || a.Type == ActionUntag) && a.Value == "":
			return nil, fmt.Errorf("%w: %s needs a value", ErrInvalid, a.Type)
		case a.Type == ActionAssign && a.Value != "" && a.Value != "me":
			if _, err := uuid.Parse(a.Value); err != nil {
				return nil, fmt.Errorf("%w: assign takes me, an agent uuid or empty (unassign)", ErrInvalid)
			}
		}
	}
	raw, err := json.Marshal(in.Actions)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// CreateMacro creates a macro.
func (s *Service) CreateMacro(ctx context.Context, v Viewer, in MacroInput) (Macro, error) {
	raw, err := in.normalize()
	if err != nil {
		return Macro{}, err
	}
	scope, owner, err := scopeOf(in.Scope, v)
	if err != nil {
		return Macro{}, err
	}
	row, err := s.q.InsertMacro(ctx, db.InsertMacroParams{
		OrganizationID: v.OrganizationID, Scope: scope, OwnerUserID: owner, Name: in.Name, Description: in.Description,
		Actions: raw, CreatedBy: pgtype.Int8{Int64: v.UserID, Valid: true},
	})
	if err != nil {
		return Macro{}, err
	}
	return macroOf(row, v), nil
}

// UpdateMacro edits a macro.
func (s *Service) UpdateMacro(ctx context.Context, v Viewer, id uuid.UUID, in MacroInput) (Macro, error) {
	raw, err := in.normalize()
	if err != nil {
		return Macro{}, err
	}
	cur, err := s.editableMacro(ctx, v, id)
	if err != nil {
		return Macro{}, err
	}
	row, err := s.q.UpdateMacro(ctx, db.UpdateMacroParams{Name: in.Name, Description: in.Description, Actions: raw, OrganizationID: v.OrganizationID, ID: cur.ID})
	if err != nil {
		return Macro{}, err
	}
	return macroOf(row, v), nil
}

// DeleteMacro deletes a macro.
func (s *Service) DeleteMacro(ctx context.Context, v Viewer, id uuid.UUID) error {
	cur, err := s.editableMacro(ctx, v, id)
	if err != nil {
		return err
	}
	_, err = s.q.DeleteMacro(ctx, db.DeleteMacroParams{OrganizationID: v.OrganizationID, ID: cur.ID})
	return err
}

func (s *Service) editableMacro(ctx context.Context, v Viewer, id uuid.UUID) (db.Macro, error) {
	row, err := s.q.GetMacro(ctx, db.GetMacroParams{OrganizationID: v.OrganizationID, Uuid: id, ViewerID: v.UserID})
	if err != nil {
		return db.Macro{}, notFound(err)
	}
	if row.Scope != ScopePersonal && !v.CanWrite {
		return db.Macro{}, ErrForbidden
	}
	return row, nil
}

// Permissions are what the agent running a macro may do; actions they may
// not perform fail the whole run before anything changes.
type Permissions struct {
	Reply, Note, Manage, Assign bool
}

// ApplyResult lists what a macro did.
type ApplyResult struct {
	Applied []string `json:"applied"`
}

// Apply runs a macro on a conversation in its action order. Reply and note
// texts are rendered with the conversation's variables.
func (s *Service) Apply(ctx context.Context, v Viewer, perms Permissions, macroID, conversation uuid.UUID) (ApplyResult, error) {
	row, err := s.q.GetMacro(ctx, db.GetMacroParams{OrganizationID: v.OrganizationID, Uuid: macroID, ViewerID: v.UserID})
	if err != nil {
		return ApplyResult{}, notFound(err)
	}
	var actions []Action
	if err := json.Unmarshal(row.Actions, &actions); err != nil {
		return ApplyResult{}, fmt.Errorf("macros: stored actions: %w", err)
	}
	for _, a := range actions {
		ok := map[string]bool{
			ActionReply: perms.Reply, ActionNote: perms.Note || perms.Reply, ActionStatus: perms.Manage,
			ActionPriority: perms.Manage, ActionTag: perms.Manage, ActionUntag: perms.Manage, ActionAssign: perms.Assign || perms.Manage,
		}[a.Type]
		if !ok {
			return ApplyResult{}, fmt.Errorf("%w: missing permission for %s", ErrForbidden, a.Type)
		}
	}
	vars, err := s.variables(ctx, v, conversation)
	if err != nil {
		return ApplyResult{}, err
	}
	actor := s.inbox.Named(ctx, convusecase.Actor{ID: v.UserID, UUID: v.UserUUID})
	res := ApplyResult{Applied: []string{}}
	for _, a := range actions {
		var err error
		switch a.Type {
		case ActionReply, ActionNote:
			_, err = s.inbox.Post(ctx, convusecase.PostInput{
				OrganizationID: v.OrganizationID, ConversationUUID: conversation, AuthorID: v.UserID,
				AuthorUUID: v.UserUUID, AuthorName: actor.Name, Content: Render(a.Value, vars), Private: a.Type == ActionNote,
				Closing: convusecase.Closing{Mode: convusecase.ClosingAuto},
			})
		case ActionStatus:
			st := a.Value
			_, err = s.inbox.Update(ctx, convusecase.UpdateInput{OrganizationID: v.OrganizationID, ConversationUUID: conversation, Actor: actor, Status: &st})
		case ActionPriority:
			p := a.Value
			_, err = s.inbox.Update(ctx, convusecase.UpdateInput{OrganizationID: v.OrganizationID, ConversationUUID: conversation, Actor: actor, Priority: &p})
		case ActionAssign:
			var target *uuid.UUID
			switch a.Value {
			case "":
			case "me":
				target = &v.UserUUID
			default:
				id := uuid.MustParse(a.Value)
				target = &id
			}
			_, err = s.inbox.Update(ctx, convusecase.UpdateInput{OrganizationID: v.OrganizationID, ConversationUUID: conversation, Actor: actor, Assignee: &target})
		case ActionTag, ActionUntag:
			if a.Type == ActionTag {
				_, err = s.inbox.TagConversation(ctx, v.OrganizationID, conversation, actor, uuid.Nil, a.Value, true)
			} else {
				err = s.untag(ctx, v, conversation, actor, a.Value)
			}
		}
		if err != nil {
			return res, fmt.Errorf("macros: %s: %w", a.Type, err)
		}
		res.Applied = append(res.Applied, a.Type)
	}
	_ = s.q.TouchMacro(ctx, db.TouchMacroParams{OrganizationID: v.OrganizationID, ID: row.ID})
	return res, nil
}

func (s *Service) untag(ctx context.Context, v Viewer, conversation uuid.UUID, actor convusecase.Actor, name string) error {
	tags, err := s.inbox.Tags(ctx, v.OrganizationID)
	if err != nil {
		return err
	}
	for _, t := range tags {
		if strings.EqualFold(t.Name, name) {
			_, err := s.inbox.TagConversation(ctx, v.OrganizationID, conversation, actor, t.UUID, "", false)
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------

func scopeOf(scope string, v Viewer) (string, pgtype.Int8, error) {
	switch scope {
	case "", ScopePersonal:
		return ScopePersonal, pgtype.Int8{Int64: v.UserID, Valid: true}, nil
	case ScopeOrganization:
		if !v.CanWrite {
			return "", pgtype.Int8{}, ErrForbidden
		}
		return ScopeOrganization, pgtype.Int8{}, nil
	default:
		// Team scope waits for teams (CF-5).
		return "", pgtype.Int8{}, fmt.Errorf("%w: scope must be personal or organization", ErrInvalid)
	}
}

func cannedOf(r db.CannedResponse, v Viewer) Canned {
	return Canned{
		UUID: r.Uuid, Scope: r.Scope, Title: r.Title, Shortcut: r.Shortcut, Category: r.Category, Content: r.Content,
		UsageCount: r.UsageCount, LastUsedAt: timePtr(r.LastUsedAt), Mine: r.OwnerUserID.Valid && r.OwnerUserID.Int64 == v.UserID,
	}
}

func macroOf(r db.Macro, v Viewer) Macro {
	m := Macro{
		UUID: r.Uuid, Scope: r.Scope, Name: r.Name, Description: r.Description, Actions: []Action{},
		UsageCount: r.UsageCount, LastUsedAt: timePtr(r.LastUsedAt), Mine: r.OwnerUserID.Valid && r.OwnerUserID.Int64 == v.UserID,
	}
	_ = json.Unmarshal(r.Actions, &m.Actions)
	return m
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func unique(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrShortcutExists
	}
	return err
}
