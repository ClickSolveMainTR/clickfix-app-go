package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	conv "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	notif "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type Sender interface {
	Post(context.Context, conv.PostInput) (conv.Posted, error)
}
type Notifier interface {
	Enqueue(context.Context, notif.EnqueueInput) ([]notif.Notification, error)
}
type Service struct {
	repo            *repository.Repository
	sender          Sender
	notifier        Notifier
	secret, baseURL string
}

func New(r *repository.Repository, s Sender, n Notifier, secret, baseURL string) *Service {
	return &Service{r, s, n, secret, strings.TrimRight(baseURL, "/")}
}
func (s *Service) WithSender(sender Sender) *Service { s.sender = sender; return s }
func valid(s string, min, max int) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(s))
	return n >= min && n <= max
}

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,62}$`)
var domainRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?\.[a-z]{2,63}$`)

func (s *Service) SavePage(ctx context.Context, org int64, in model.PageInput) (db.StatusPage, error) {
	if !slugRE.MatchString(in.Slug) || !valid(in.Title, 1, 120) || len(in.CustomDomain) > 253 || (in.CustomDomain != "" && !domainRE.MatchString(in.CustomDomain)) {
		return db.StatusPage{}, model.ErrInvalid
	}
	return s.repo.Q.StatusSavePage(ctx, db.StatusSavePageParams{OrganizationID: org, Slug: in.Slug, Title: in.Title, CustomDomain: in.CustomDomain, Published: in.Published})
}
func (s *Service) Page(ctx context.Context, org int64) (db.StatusPage, error) {
	return s.repo.Q.StatusPage(ctx, org)
}
func (s *Service) SaveComponent(ctx context.Context, org int64, id uuid.UUID, in model.ComponentInput) (db.StatusComponent, error) {
	if !valid(in.Name, 1, 120) || !valid(in.Group, 0, 120) || !valid(in.Description, 0, 2000) {
		return db.StatusComponent{}, model.ErrInvalid
	}
	if id != uuid.Nil {
		return s.repo.Q.StatusUpdateComponent(ctx, db.StatusUpdateComponentParams{OrganizationID: org, Uuid: id, Name: in.Name, GroupName: in.Group, Description: in.Description, SortOrder: in.SortOrder})
	}
	return s.repo.Q.StatusCreateComponent(ctx, db.StatusCreateComponentParams{OrganizationID: org, Name: in.Name, GroupName: in.Group, Description: in.Description, SortOrder: in.SortOrder})
}
func (s *Service) Components(ctx context.Context, org int64) (json.RawMessage, error) {
	v, err := s.repo.Q.StatusComponents(ctx, org)
	return json.RawMessage(v), err
}
func (s *Service) CreateIncident(ctx context.Context, org int64, in model.IncidentInput) (db.StatusIncident, error) {
	var out db.StatusIncident
	if in.Kind == "" {
		in.Kind = "incident"
	}
	if in.StartedAt.IsZero() {
		in.StartedAt = time.Now().UTC()
	}
	if !valid(in.Title, 1, 200) || !valid(in.Body, 1, 10000) || !valid(in.NotificationBody, 0, 5000) || !valid(in.Postmortem, 0, 20000) || len(in.Components) == 0 || len(in.Components) > 100 {
		return out, model.ErrInvalid
	}
	if in.Kind != "incident" && in.Kind != "maintenance" {
		return out, model.ErrInvalid
	}
	if in.Severity != "degraded" && in.Severity != "partial_outage" && in.Severity != "major_outage" && in.Severity != "maintenance" {
		return out, model.ErrInvalid
	}
	if (in.EndsAt != nil && !in.EndsAt.After(in.StartedAt)) || (in.Kind == "maintenance" && (in.EndsAt == nil || in.Severity != "maintenance")) || (in.Kind == "incident" && (in.Severity == "maintenance" || in.StartedAt.After(time.Now()))) {
		return out, model.ErrInvalid
	}
	var template []byte
	if in.NotificationTemplate != nil {
		if in.NotificationTemplate.Name == "" || in.NotificationTemplate.Language == "" {
			return out, model.ErrInvalid
		}
		template, _ = json.Marshal(in.NotificationTemplate)
	}
	status := "investigating"
	if in.Kind == "maintenance" {
		status = "scheduled"
	}
	ends := pgtype.Timestamptz{}
	if in.EndsAt != nil {
		ends = pgtype.Timestamptz{Time: *in.EndsAt, Valid: true}
	}
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		var err error
		out, err = q.StatusCreateIncident(ctx, db.StatusCreateIncidentParams{OrganizationID: org, Title: in.Title, Kind: in.Kind, Severity: in.Severity, Status: status, Public: in.Public, StartedAt: pgtype.Timestamptz{Time: in.StartedAt, Valid: true}, EndsAt: ends, NotificationBody: in.NotificationBody, NotificationTemplate: template, Postmortem: in.Postmortem})
		if err != nil {
			return err
		}
		for _, id := range in.Components {
			n, err := q.StatusLinkComponent(ctx, db.StatusLinkComponentParams{OrganizationID: org, IncidentID: out.ID, ComponentUuid: id})
			if err != nil {
				return err
			}
			if n != 1 {
				return model.ErrInvalid
			}
		}
		uid, err := q.StatusAddUpdate(ctx, db.StatusAddUpdateParams{OrganizationID: org, IncidentID: out.ID, Status: status, Body: in.Body})
		if err != nil {
			return err
		}
		return emit(ctx, q, "status.changed", out, uid, in.Body)
	})
	return out, err
}
func emit(ctx context.Context, q *db.Queries, name string, i db.StatusIncident, update uuid.UUID, body string) error {
	return outbox.EnqueueQueries(ctx, q, events.New(name).WithOrganization(i.OrganizationID).WithEntity("incident", &i.ID, &i.Uuid).WithPayload(map[string]any{"update_uuid": update.String(), "body": body, "public": i.Public, "status": i.Status}).WithIdempotencyKey(update.String()))
}
func (s *Service) Update(ctx context.Context, org int64, id uuid.UUID, in model.UpdateInput) (db.StatusIncident, error) {
	var out db.StatusIncident
	if !valid(in.Body, 1, 10000) || !valid(in.Postmortem, 0, 20000) {
		return out, model.ErrInvalid
	}
	if in.Status != "scheduled" && in.Status != "investigating" && in.Status != "identified" && in.Status != "monitoring" && in.Status != "resolved" {
		return out, model.ErrInvalid
	}
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		old, err := q.StatusIncident(ctx, db.StatusIncidentParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		if old.Status == "resolved" {
			return model.ErrConflict
		}
		if old.StartedAt.Time.After(time.Now()) && in.Status == "resolved" {
			return model.ErrInvalid
		}
		out, err = q.StatusSetIncidentState(ctx, db.StatusSetIncidentStateParams{OrganizationID: org, Uuid: id, Status: in.Status, Postmortem: in.Postmortem})
		if err != nil {
			return err
		}
		uid, err := q.StatusAddUpdate(ctx, db.StatusAddUpdateParams{OrganizationID: org, IncidentID: out.ID, Status: in.Status, Body: in.Body})
		if err != nil {
			return err
		}
		if err = emit(ctx, q, "status.changed", out, uid, in.Body); err != nil {
			return err
		}
		if in.Status == "resolved" && out.NotificationBody != "" {
			if err = q.StatusPrepareDeliveries(ctx, db.StatusPrepareDeliveriesParams{OrganizationID: org, IncidentID: out.ID}); err != nil {
				return err
			}
			return emit(ctx, q, "status.notify_contacts", out, out.Uuid, out.NotificationBody)
		}
		return nil
	})
	return out, err
}
func (s *Service) Incidents(ctx context.Context, org int64, public bool, limit, offset int32) (json.RawMessage, error) {
	v, err := s.repo.Q.StatusIncidents(ctx, db.StatusIncidentsParams{OrganizationID: org, PublicOnly: public, RowLimit: limit, RowOffset: offset})
	return json.RawMessage(v), err
}
func (s *Service) Public(ctx context.Context, slug string) (map[string]any, error) {
	p, err := s.repo.Q.StatusPublicPage(ctx, slug)
	if err != nil {
		return nil, err
	}
	c, err := s.Components(ctx, p.OrganizationID)
	if err != nil {
		return nil, err
	}
	i, err := s.Incidents(ctx, p.OrganizationID, true, 100, 0)
	if err != nil {
		return nil, err
	}
	return map[string]any{"title": p.Title, "slug": p.Slug, "components": c, "incidents": i}, nil
}
func (s *Service) History(ctx context.Context, slug string, days int) (json.RawMessage, error) {
	if days < 1 || days > 90 {
		return nil, model.ErrInvalid
	}
	p, err := s.repo.Q.StatusPublicPage(ctx, slug)
	if err != nil {
		return nil, err
	}
	to := time.Now().UTC()
	from := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -days+1)
	v, err := s.repo.Q.StatusUptime(ctx, db.StatusUptimeParams{OrganizationID: p.OrganizationID, FromAt: pgtype.Timestamptz{Time: from, Valid: true}, ToAt: pgtype.Timestamptz{Time: to, Valid: true}})
	return json.RawMessage(v), err
}
func (s *Service) Link(ctx context.Context, org int64, id, conversation uuid.UUID) error {
	return s.repo.InTx(ctx, func(q *db.Queries) error {
		i, err := q.StatusIncident(ctx, db.StatusIncidentParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		if i.Status == "resolved" {
			return model.ErrConflict
		}
		n, err := q.StatusLinkConversation(ctx, db.StatusLinkConversationParams{OrganizationID: org, IncidentID: i.ID, ConversationUuid: conversation})
		if err != nil {
			return err
		}
		if n == 0 {
			return model.ErrInvalid
		}
		return nil
	})
}
func (s *Service) Preview(ctx context.Context, org int64, id uuid.UUID) (map[string]any, error) {
	var out map[string]any
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		i, err := q.StatusIncident(ctx, db.StatusIncidentParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		raw, err := q.StatusNotificationPreview(ctx, db.StatusNotificationPreviewParams{OrganizationID: org, IncidentID: i.ID})
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &out); err != nil {
			return err
		}
		out["body"] = i.NotificationBody
		if len(i.NotificationTemplate) > 0 {
			out["template"] = json.RawMessage(i.NotificationTemplate)
		} else {
			out["template"] = nil
		}
		return nil
	})
	return out, err
}
func (s *Service) NotifyContacts(ctx context.Context, org int64, id uuid.UUID) error {
	var i db.StatusIncident
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		var err error
		i, err = q.StatusIncident(ctx, db.StatusIncidentParams{OrganizationID: org, Uuid: id})
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // incident deleted before the event ran
	}
	if err != nil {
		return err
	}
	if i.Status != "resolved" || i.NotificationBody == "" {
		return nil
	}
	if s.sender == nil {
		return model.ErrInvalid
	}

	var template *channels.Template
	if len(i.NotificationTemplate) > 0 {
		if err = json.Unmarshal(i.NotificationTemplate, &template); err != nil {
			return err
		}
	}
	var errs []error
	var after int64
	for {
		recipients, err := s.repo.Q.StatusNotificationRecipients(ctx, db.StatusNotificationRecipientsParams{OrganizationID: org, IncidentID: i.ID, AfterContact: after})
		if err != nil {
			return err
		}
		if len(recipients) == 0 {
			break
		}
		for _, r := range recipients {
			_, err := s.sender.Post(ctx, conv.PostInput{OrganizationID: org, ConversationUUID: r.Uuid, Content: i.NotificationBody, Template: template, ClientID: uuid.NewSHA1(i.Uuid, []byte("incident-contact:"+fmtInt(r.ContactID)))})
			if err != nil {
				errs = append(errs, err)
			}
			after = r.ContactID
		}
	}
	return errors.Join(errs...)
}
func (s *Service) Subscribe(ctx context.Context, slug, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 320 {
		return model.ErrInvalid
	}
	p, err := s.repo.Q.StatusPublicPage(ctx, slug)
	if err != nil {
		return err
	}
	return s.repo.InTx(ctx, func(q *db.Queries) error {
		row, err := q.StatusSubscribe(ctx, db.StatusSubscribeParams{OrganizationID: p.OrganizationID, Email: email})
		if err != nil {
			return err
		}
		return outbox.EnqueueQueries(ctx, q, events.New("status.subscription_requested").WithOrganization(p.OrganizationID).WithEntity("status_subscription", &row.ID, &row.Uuid).WithPayload(map[string]any{"subscription_uuid": row.Uuid.String()}).WithIdempotencyKey(row.Uuid.String()+":"+row.ExpiresAt.Time.UTC().Format(time.RFC3339Nano)))
	})
}

func (s *Service) IncidentCount(ctx context.Context, org int64) (int64, error) {
	return s.repo.Q.StatusIncidentCount(ctx, org)
}

func (s *Service) PublicURL(slug string) string { return s.baseURL + "/status/" + slug }
