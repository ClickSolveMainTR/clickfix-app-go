package guides

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

// Product feedback (web "Geri bildirim", mobile "shake to send feedback"):
// feedback about ClickFix itself, read by platform staff.

// FeedbackKinds and FeedbackStatuses mirror the CHECK constraints.
var (
	FeedbackKinds    = []string{"bug", "idea", "other"}
	FeedbackStatuses = []string{"new", "reviewed", "closed"}
)

// MaxFeedbackAttachments caps screenshots per feedback.
const MaxFeedbackAttachments = 5

// ErrAttachment: an attachment is not the caller's upload in this organization.
var ErrAttachment = errors.New("guides: unknown attachment")

// FeedbackInput is POST /v1/tenant/product-feedback.
type FeedbackInput struct {
	Message         string      `json:"message"`
	Kind            string      `json:"kind"`
	Route           string      `json:"route"`
	Surface         string      `json:"surface"`
	AppVersion      string      `json:"app_version"`
	Device          string      `json:"device"`
	AttachmentUUIDs []uuid.UUID `json:"attachment_uuids"`
}

// SubmitFeedback stores one product feedback.
func (s *Service) SubmitFeedback(ctx context.Context, org, user int64, in FeedbackInput) (uuid.UUID, error) {
	in.Message = strings.TrimSpace(in.Message)
	if in.Kind == "" {
		in.Kind = "other"
	}
	surface, err := NormalizeSurface(in.Surface)
	if err != nil || in.Message == "" || utf8.RuneCountInString(in.Message) > 5000 || !oneOf(in.Kind, FeedbackKinds) ||
		len(in.Route) > 500 || len(in.AppVersion) > 64 || len(in.Device) > 200 || len(in.AttachmentUUIDs) > MaxFeedbackAttachments {
		return uuid.Nil, ErrInvalid
	}
	ids := []uuid.UUID{}
	seen := map[uuid.UUID]bool{}
	for _, id := range in.AttachmentUUIDs {
		if id != uuid.Nil && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		n, err := s.Q.ProductFeedbackAttachmentsOwned(ctx, db.ProductFeedbackAttachmentsOwnedParams{OrganizationID: org, UserID: pgtype.Int8{Int64: user, Valid: true}, Ids: ids})
		if err != nil {
			return uuid.Nil, err
		}
		if int(n) != len(ids) {
			return uuid.Nil, ErrAttachment
		}
	}
	return s.Q.ProductFeedbackCreate(ctx, db.ProductFeedbackCreateParams{
		OrganizationID: org, UserID: pgtype.Int8{Int64: user, Valid: user > 0}, Kind: in.Kind, Message: in.Message,
		Route: NormalizeRoute(in.Route), Surface: surface, AppVersion: strings.TrimSpace(in.AppVersion),
		Device: strings.TrimSpace(in.Device), AttachmentUuids: ids,
	})
}

// FeedbackItem is one row of the platform list.
type FeedbackItem struct {
	UUID             uuid.UUID   `json:"uuid"`
	Kind             string      `json:"kind"`
	Message          string      `json:"message"`
	Route            string      `json:"route"`
	Surface          string      `json:"surface"`
	AppVersion       string      `json:"app_version"`
	Device           string      `json:"device"`
	AttachmentUUIDs  []uuid.UUID `json:"attachment_uuids"`
	Status           string      `json:"status"`
	CreatedAt        time.Time   `json:"created_at"`
	OrganizationUUID uuid.UUID   `json:"organization_uuid"`
	OrganizationName string      `json:"organization_name"`
	UserEmail        string      `json:"user_email"`
}

// FeedbackPage is a page of product feedback.
type FeedbackPage struct {
	Items  []FeedbackItem `json:"items"`
	Total  int64          `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

// ListFeedback is the platform list.
func (s *Service) ListFeedback(ctx context.Context, kind, status string, limit, offset int) (FeedbackPage, error) {
	if (kind != "" && !oneOf(kind, FeedbackKinds)) || (status != "" && !oneOf(status, FeedbackStatuses)) {
		return FeedbackPage{}, ErrInvalid
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 || offset > 1000000 {
		offset = 0
	}
	// #nosec G115 -- limit/offset are bounded above.
	rows, err := s.Q.ProductFeedbackList(ctx, db.ProductFeedbackListParams{Kind: kind, Status: status, MaxRows: int32(limit), SkipRows: int32(offset)})
	if err != nil {
		return FeedbackPage{}, err
	}
	total, err := s.Q.ProductFeedbackCount(ctx, db.ProductFeedbackCountParams{Kind: kind, Status: status})
	if err != nil {
		return FeedbackPage{}, err
	}
	out := FeedbackPage{Items: make([]FeedbackItem, 0, len(rows)), Total: total, Limit: limit, Offset: offset}
	for _, r := range rows {
		out.Items = append(out.Items, FeedbackItem{
			UUID: r.Uuid, Kind: r.Kind, Message: r.Message, Route: r.Route, Surface: r.Surface, AppVersion: r.AppVersion,
			Device: r.Device, AttachmentUUIDs: r.AttachmentUuids, Status: r.Status, CreatedAt: r.CreatedAt.Time,
			OrganizationUUID: r.OrganizationUuid, OrganizationName: r.OrganizationName, UserEmail: r.UserEmail,
		})
	}
	return out, nil
}

// SetFeedbackStatus moves a feedback through new → reviewed → closed.
func (s *Service) SetFeedbackStatus(ctx context.Context, id uuid.UUID, status string) error {
	if !oneOf(status, FeedbackStatuses) {
		return ErrInvalid
	}
	n, err := s.Q.ProductFeedbackSetStatus(ctx, db.ProductFeedbackSetStatusParams{Uuid: id, Status: status})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
