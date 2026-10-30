// Package usecase evaluates allowlisted reports and creates durable scheduled artifacts.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	notif "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/reports/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Notifier interface {
	Enqueue(context.Context, notif.EnqueueInput) ([]notif.Notification, error)
}
type Service struct {
	Repo     *repository.Repository
	Resolver orgctx.Resolver
	Notifier Notifier
	BaseURL  string
	Secret   string
}

func Validate(d model.Definition) error {
	if len(d.Dimensions) > 1 || len(d.Metrics) == 0 || len(d.Metrics) > len(model.Metrics) || !slices.Contains([]string{"table", "bar", "line", "pie"}, d.ChartType) {
		return supportops.ErrInvalid
	}
	if len(d.Dimensions) == 1 && !slices.Contains([]string{"agent", "team", "channel", "day"}, d.Dimensions[0]) {
		return supportops.ErrInvalid
	}
	seen := map[string]bool{}
	for _, m := range d.Metrics {
		if !slices.Contains(model.Metrics, m) || seen[m] {
			return supportops.ErrInvalid
		}
		seen[m] = true
	}
	if _, err := time.LoadLocation(d.Timezone); err != nil || d.Timezone == "" {
		return supportops.ErrInvalid
	}
	if len(d.Filters.Channel) > 40 {
		return supportops.ErrInvalid
	}
	return nil
}

func (s *Service) Save(ctx context.Context, org, actor int64, id uuid.UUID, in model.Input) (db.CustomReport, error) {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 120 {
		return db.CustomReport{}, supportops.ErrInvalid
	}
	if err := Validate(in.Definition); err != nil {
		return db.CustomReport{}, err
	}
	if in.Definition.Dimensions == nil {
		in.Definition.Dimensions = []string{}
	}
	b, err := json.Marshal(in.Definition)
	if err != nil {
		return db.CustomReport{}, err
	}
	if id == uuid.Nil {
		return s.Repo.Q.ReportsCreate(ctx, db.ReportsCreateParams{OrganizationID: org, CreatedBy: actor, Name: strings.TrimSpace(in.Name), Definition: b})
	}
	return s.Repo.Q.ReportsUpdate(ctx, db.ReportsUpdateParams{OrganizationID: org, Uuid: id, Name: strings.TrimSpace(in.Name), Definition: b})
}

func nullable(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}
func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
func Evaluate(ctx context.Context, q *db.Queries, org int64, d model.Definition, w model.Evaluation) (ioengine.Dataset, error) {
	ds := ioengine.Dataset{Resource: "tenant.reports", Rows: []map[string]any{}}
	if Validate(d) != nil || w.From.IsZero() || !w.To.After(w.From) || w.To.Sub(w.From) > 366*24*time.Hour {
		return ds, supportops.ErrInvalid
	}
	dim := ""
	if len(d.Dimensions) > 0 {
		dim = d.Dimensions[0]
	}
	raw, err := q.ReportsEvaluate(ctx, db.ReportsEvaluateParams{OrganizationID: org, FromAt: ts(w.From), ToAt: ts(w.To), Dimension: dim, Timezone: d.Timezone, Channel: d.Filters.Channel, Team: nullable(d.Filters.Team), Agent: nullable(d.Filters.Agent)})
	if err != nil {
		return ds, err
	}
	var rows []map[string]any
	if err = json.Unmarshal(raw, &rows); err != nil {
		return ds, err
	}
	if len(rows) > 10000 {
		return ds, supportops.ErrInvalid
	}
	keys := append([]string{"dimension"}, d.Metrics...)
	for _, k := range keys {
		ds.Columns = append(ds.Columns, ioengine.Column{Key: k, LabelKey: k, Type: ioengine.ColumnTypeString})
	}
	for _, row := range rows {
		out := map[string]any{}
		for _, k := range keys {
			out[k] = row[k]
		}
		ds.Rows = append(ds.Rows, out)
	}
	return ds, nil
}

func (s *Service) Evaluate(ctx context.Context, org int64, id uuid.UUID, w model.Evaluation) (ioengine.Dataset, error) {
	r, err := s.Repo.Q.ReportsGet(ctx, db.ReportsGetParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return ioengine.Dataset{}, err
	}
	var d model.Definition
	if err = json.Unmarshal(r.Definition, &d); err != nil {
		return ioengine.Dataset{}, err
	}
	return Evaluate(ctx, s.Repo.Q, org, d, w)
}

// Next aligns deliveries to calendar midnight, Monday or the first of the month in the chosen zone.
func Next(t time.Time, frequency string, loc *time.Location) time.Time {
	t = t.In(loc)
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	switch frequency {
	case "weekly":
		days := (8 - int(d.Weekday())) % 7
		if days == 0 {
			days = 7
		}
		return d.AddDate(0, 0, days).UTC()
	case "monthly":
		return time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc).UTC()
	default:
		return d.AddDate(0, 0, 1).UTC()
	}
}

func (s *Service) permitted(ctx context.Context, org int64, id uuid.UUID) (db.ReportsMemberRow, error) {
	m, err := s.Repo.Q.ReportsMember(ctx, db.ReportsMemberParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return m, err
	}
	if s.Resolver == nil {
		return m, supportops.ErrDenied
	}
	scope, err := s.Resolver.Resolve(ctx, m.OrganizationUuid.String(), m.ID)
	if err != nil || !scope.Has(rbac.PermTenantConversationsRead) || !scope.Has(rbac.PermTenantConversationsReadAll) {
		return m, supportops.ErrDenied
	}
	return m, nil
}

func (s *Service) Schedule(ctx context.Context, org, actor int64, in model.ScheduleInput) (db.ReportSchedule, error) {
	if !slices.Contains([]string{"daily", "weekly", "monthly"}, in.Frequency) || !slices.Contains([]string{"csv", "xlsx"}, in.Format) || len(in.Recipients) == 0 || len(in.Recipients) > 20 {
		return db.ReportSchedule{}, supportops.ErrInvalid
	}
	loc, err := time.LoadLocation(in.Timezone)
	if err != nil || in.Timezone == "" {
		return db.ReportSchedule{}, supportops.ErrInvalid
	}
	for _, id := range in.Recipients {
		if _, err = s.permitted(ctx, org, id); err != nil {
			return db.ReportSchedule{}, supportops.ErrInvalid
		}
	}
	return s.Repo.Q.ReportsSchedule(ctx, db.ReportsScheduleParams{OrganizationID: org, ReportUuid: in.ReportUUID, CreatedBy: actor, Frequency: in.Frequency, Timezone: in.Timezone, Format: in.Format, Recipients: in.Recipients, Enabled: in.Enabled, NextRunAt: ts(Next(time.Now(), in.Frequency, loc))})
}

func (s *Service) Sweep(ctx context.Context) error {
	if err := s.Repo.Q.ReportsPurge(ctx); err != nil {
		return err
	}
	due, err := s.Repo.Q.ReportsDue(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, v := range due {
		if err = s.run(ctx, v.OrganizationID, v.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) run(ctx context.Context, org, id int64) error {
	return s.Repo.Within(ctx, func(r *repository.Repository) error {
		row, err := r.Q.ReportsLockSchedule(ctx, db.ReportsLockScheduleParams{OrganizationID: org, ID: id})
		if err != nil {
			return err
		}
		loc, err := time.LoadLocation(row.Timezone)
		if err != nil {
			return err
		}
		to := row.NextRunAt.Time.In(loc)
		from := to.AddDate(0, 0, -1)
		if row.Frequency == "weekly" {
			from = to.AddDate(0, 0, -7)
		}
		if row.Frequency == "monthly" {
			from = to.AddDate(0, -1, 0)
		}
		var d model.Definition
		if err = json.Unmarshal(row.Definition, &d); err != nil {
			return err
		}
		ds, err := Evaluate(ctx, r.Q, org, d, model.Evaluation{From: from, To: to})
		if err != nil {
			return err
		}
		data, err := ioengine.EncodeExport(ioengine.ExportFormat(row.Format), ds, "tr", nil, row.Name)
		if err != nil {
			return err
		}
		uid, err := r.Q.ReportsStoreDelivery(ctx, db.ReportsStoreDeliveryParams{OrganizationID: org, ScheduleID: id, ScheduledAt: row.NextRunAt, Format: row.Format, Content: data})
		if err != nil {
			return err
		}
		if err = r.Emit(ctx, events.New("reports.ready").WithOrganization(org).WithEntity("report_delivery", nil, &uid).WithPayload(map[string]any{"recipients": row.Recipients, "title": row.Name}).WithIdempotencyKey(uid.String())); err != nil {
			return err
		}
		return r.Q.ReportsAdvance(ctx, db.ReportsAdvanceParams{OrganizationID: org, ID: id, NextRunAt: ts(Next(time.Now(), row.Frequency, loc))})
	})
}

func (s *Service) Consumer() eventdispatch.Consumer {
	return eventdispatch.Consumer{Name: "scheduled_reports", Events: []string{"reports.ready"}, MaxRetry: 5, Timeout: time.Minute, Handle: func(ctx context.Context, e events.Event) error {
		if e.TenantID == nil || e.EntityUUID == nil {
			return nil
		}
		if s.Notifier == nil {
			return fmt.Errorf("report notifier unavailable")
		}
		raw, err := json.Marshal(e.Payload["recipients"])
		if err != nil {
			return err
		}
		var ids []uuid.UUID
		if err = json.Unmarshal(raw, &ids); err != nil {
			return err
		}
		for _, id := range ids {
			m, err := s.permitted(ctx, *e.TenantID, id)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, supportops.ErrDenied) {
					continue
				}
				return err
			}
			link := s.DownloadURL(*e.TenantID, *e.EntityUUID, id, time.Now().Add(7*24*time.Hour))
			if _, err = s.Notifier.Enqueue(ctx, notif.EnqueueInput{TenantID: e.TenantID, UserID: &m.ID, Channels: []string{notif.ChannelEmail}, Title: fmt.Sprint(e.Payload["title"]), Body: link, ActionURL: &link, SourceEvent: "reports.ready",
				ContentVars: map[string]string{"title": fmt.Sprint(e.Payload["title"]), "url": link}, DedupeKey: "reports.ready:" + e.EventID.String()}); err != nil {
				return err
			}
		}
		return nil
	}}
}
