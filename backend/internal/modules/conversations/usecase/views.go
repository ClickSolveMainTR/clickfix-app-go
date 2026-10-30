package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrForbidden: the caller may not change the record.
var ErrForbidden = errors.New("conversations: not allowed")

// View scopes.
const (
	ViewPersonal     = "personal"
	ViewOrganization = "organization"
)

// ViewFilters is the stored filter set of a saved view (CF-47). It mirrors
// the list API query.
type ViewFilters struct {
	Statuses            []string    `json:"statuses,omitempty"`
	Priorities          []string    `json:"priorities,omitempty"`
	ChannelTypes        []string    `json:"channel_types,omitempty"`
	Inboxes             []uuid.UUID `json:"inboxes,omitempty"`
	Assignee            string      `json:"assignee,omitempty"`
	Tags                []uuid.UUID `json:"tags,omitempty"`
	Unread              bool        `json:"unread,omitempty"`
	Mentioned           bool        `json:"mentioned,omitempty"`
	SLABreached         *bool       `json:"sla_breached,omitempty"`
	SLADueWithinMinutes int         `json:"sla_due_within_minutes,omitempty"`
	Q                   string      `json:"q,omitempty"`
	Sort                string      `json:"sort,omitempty"`
}

// ListFilter converts stored filters into a list query.
func (f ViewFilters) ListFilter() ListFilter {
	out := ListFilter{
		Statuses: f.Statuses, Priorities: f.Priorities, ChannelTypes: f.ChannelTypes, Inboxes: f.Inboxes,
		Assignee: f.Assignee, Tags: f.Tags, Unread: f.Unread, Mentioned: f.Mentioned, SLABreached: f.SLABreached,
		SLADueWithin: time.Duration(f.SLADueWithinMinutes) * time.Minute, Q: f.Q, Sort: "last_message_at", Desc: true,
	}
	if f.Sort != "" {
		out.Desc = strings.HasPrefix(f.Sort, "-")
		out.Sort = strings.TrimPrefix(f.Sort, "-")
	}
	return out
}

func (f ViewFilters) validate() error {
	for _, st := range f.Statuses {
		if !model.Valid(st, model.Statuses) {
			return fmt.Errorf("%w: unknown status %q", ErrInvalid, st)
		}
	}
	for _, p := range f.Priorities {
		if !model.Valid(p, model.Priorities) {
			return fmt.Errorf("%w: unknown priority %q", ErrInvalid, p)
		}
	}
	if f.Assignee != "" && f.Assignee != "me" && f.Assignee != "unassigned" {
		if _, err := uuid.Parse(f.Assignee); err != nil {
			return fmt.Errorf("%w: assignee must be me, unassigned or a user uuid", ErrInvalid)
		}
	}
	if f.SLADueWithinMinutes < 0 || f.SLADueWithinMinutes > 7*24*60 {
		return fmt.Errorf("%w: sla_due_within_minutes must be 0-10080", ErrInvalid)
	}
	switch strings.TrimPrefix(f.Sort, "-") {
	case "", "last_message_at", "created_at", "waiting_since", "priority":
	default:
		return fmt.Errorf("%w: unknown sort", ErrInvalid)
	}
	return nil
}

// View is a saved inbox view.
type View struct {
	UUID     uuid.UUID   `json:"uuid"`
	Scope    string      `json:"scope"`
	Name     string      `json:"name"`
	Icon     string      `json:"icon"`
	Filters  ViewFilters `json:"filters"`
	Position int32       `json:"position"`
	Mine     bool        `json:"mine"`
}

// ViewInput creates or edits a view.
type ViewInput struct {
	Scope    string
	Name     string
	Icon     string
	Filters  ViewFilters
	Position int32
}

// Views lists the viewer's views (personal and organization).
func (s *Service) Views(ctx context.Context, orgID, viewerID int64) ([]View, error) {
	rows, err := s.store.Q().ListConversationViews(ctx, db.ListConversationViewsParams{OrganizationID: orgID, ViewerID: viewerID})
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(rows))
	for _, r := range rows {
		out = append(out, viewOf(r, viewerID))
	}
	return out, nil
}

// CreateView saves a view; organization views need canShare.
func (s *Service) CreateView(ctx context.Context, orgID, viewerID int64, canShare bool, in ViewInput) (View, error) {
	raw, err := in.normalize()
	if err != nil {
		return View{}, err
	}
	var owner pgtype.Int8
	switch in.Scope {
	case "", ViewPersonal:
		in.Scope, owner = ViewPersonal, pgInt8(viewerID)
	case ViewOrganization:
		if !canShare {
			return View{}, ErrForbidden
		}
	default:
		return View{}, fmt.Errorf("%w: scope must be personal or organization", ErrInvalid)
	}
	row, err := s.store.Q().InsertConversationView(ctx, db.InsertConversationViewParams{
		OrganizationID: orgID, Scope: in.Scope, OwnerUserID: owner, Name: in.Name, Icon: in.Icon,
		Filters: raw, Position: in.Position, CreatedBy: pgInt8(viewerID),
	})
	if err != nil {
		return View{}, err
	}
	return viewOf(row, viewerID), nil
}

// UpdateView edits a view the viewer may change.
func (s *Service) UpdateView(ctx context.Context, orgID, viewerID int64, canShare bool, id uuid.UUID, in ViewInput) (View, error) {
	raw, err := in.normalize()
	if err != nil {
		return View{}, err
	}
	cur, err := s.editableView(ctx, orgID, viewerID, canShare, id)
	if err != nil {
		return View{}, err
	}
	row, err := s.store.Q().UpdateConversationViewRow(ctx, db.UpdateConversationViewRowParams{
		Name: in.Name, Icon: in.Icon, Filters: raw, Position: in.Position, OrganizationID: orgID, ID: cur.ID,
	})
	if err != nil {
		return View{}, err
	}
	return viewOf(row, viewerID), nil
}

// DeleteView deletes a view the viewer may change.
func (s *Service) DeleteView(ctx context.Context, orgID, viewerID int64, canShare bool, id uuid.UUID) error {
	cur, err := s.editableView(ctx, orgID, viewerID, canShare, id)
	if err != nil {
		return err
	}
	_, err = s.store.Q().DeleteConversationView(ctx, db.DeleteConversationViewParams{OrganizationID: orgID, ID: cur.ID})
	return err
}

// ViewCounts counts the open conversations of every view of the viewer
// (sidebar / views bar badges). A view's own status filter wins; views
// without one count open and pending conversations.
func (s *Service) ViewCounts(ctx context.Context, orgID, viewerID int64) (map[string]int64, error) {
	views, err := s.Views(ctx, orgID, viewerID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(views))
	for _, v := range views {
		f := v.Filters.ListFilter()
		if len(f.Statuses) == 0 {
			f.Statuses = []string{model.StatusOpen, model.StatusPending}
		}
		f.Limit = 1
		_, total, err := s.List(ctx, orgID, viewerID, f)
		if err != nil {
			return nil, err
		}
		out[v.UUID.String()] = total
	}
	return out, nil
}

func (s *Service) editableView(ctx context.Context, orgID, viewerID int64, canShare bool, id uuid.UUID) (db.ConversationView, error) {
	row, err := s.store.Q().GetConversationView2(ctx, db.GetConversationView2Params{OrganizationID: orgID, Uuid: id, ViewerID: viewerID})
	if err != nil {
		return db.ConversationView{}, notFound(err)
	}
	if row.Scope != ViewPersonal && !canShare {
		return db.ConversationView{}, ErrForbidden
	}
	return row, nil
}

func (in *ViewInput) normalize() ([]byte, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Icon = strings.TrimSpace(in.Icon)
	if in.Name == "" || len([]rune(in.Name)) > 60 {
		return nil, fmt.Errorf("%w: name must be 1-60 characters", ErrInvalid)
	}
	if len(in.Icon) > 40 {
		return nil, fmt.Errorf("%w: icon is too long", ErrInvalid)
	}
	if err := in.Filters.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(in.Filters)
}

func viewOf(r db.ConversationView, viewerID int64) View {
	v := View{
		UUID: r.Uuid, Scope: r.Scope, Name: r.Name, Icon: r.Icon, Position: r.Position,
		Mine: r.OwnerUserID.Valid && r.OwnerUserID.Int64 == viewerID,
	}
	_ = json.Unmarshal(r.Filters, &v.Filters)
	return v
}
