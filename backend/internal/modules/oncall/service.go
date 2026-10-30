package oncall

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	notif "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/oncall/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	webhook "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Notifier interface {
	Enqueue(context.Context, notif.EnqueueInput) ([]notif.Notification, error)
}
type Service struct {
	Repo     *repository.Repository
	Client   *http.Client
	Notifier Notifier
}

func New(r *repository.Repository, n Notifier) *Service {
	return &Service{Repo: r, Client: webhook.PublicHTTPClient(), Notifier: n}
}
func nameValid(s string) bool { return strings.TrimSpace(s) != "" && len(s) <= 120 }
func (s *Service) member(ctx context.Context, org int64, id uuid.UUID) error {
	_, err := s.Repo.Q.ReportsMember(ctx, db.ReportsMemberParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return supportops.ErrInvalid
	}
	return err
}

func (s *Service) Rota(ctx context.Context, org int64, id uuid.UUID, in RotaInput) (db.OncallRota, error) {
	if !nameValid(in.Name) || len(in.Layers) == 0 || len(in.Layers) > 20 || len(in.Overrides) > 100 {
		return db.OncallRota{}, supportops.ErrInvalid
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil || in.Timezone == "" {
		return db.OncallRota{}, supportops.ErrInvalid
	}
	for _, l := range in.Layers {
		if l.StartsAt.IsZero() || l.RotationMinutes < 1 || l.RotationMinutes > 525600 || len(l.Agents) == 0 || len(l.Agents) > 100 || (l.EndsAt != nil && !l.EndsAt.After(l.StartsAt)) {
			return db.OncallRota{}, supportops.ErrInvalid
		}
		for _, a := range l.Agents {
			if err := s.member(ctx, org, a); err != nil {
				return db.OncallRota{}, err
			}
		}
	}
	for i, o := range in.Overrides {
		if o.StartsAt.IsZero() || !o.EndsAt.After(o.StartsAt) {
			return db.OncallRota{}, supportops.ErrInvalid
		}
		for _, p := range in.Overrides[:i] {
			if o.StartsAt.Before(p.EndsAt) && p.StartsAt.Before(o.EndsAt) {
				return db.OncallRota{}, supportops.ErrInvalid
			}
		}
		if err := s.member(ctx, org, o.Agent); err != nil {
			return db.OncallRota{}, err
		}
	}
	if in.Overrides == nil {
		in.Overrides = []Override{}
	}
	layers, err := json.Marshal(in.Layers)
	if err != nil {
		return db.OncallRota{}, err
	}
	overrides, err := json.Marshal(in.Overrides)
	if err != nil {
		return db.OncallRota{}, err
	}
	if id == uuid.Nil {
		return s.Repo.Q.OncallCreateRota(ctx, db.OncallCreateRotaParams{OrganizationID: org, Name: in.Name, Timezone: in.Timezone, Layers: layers, Overrides: overrides})
	}
	return s.Repo.Q.OncallUpdateRota(ctx, db.OncallUpdateRotaParams{OrganizationID: org, Uuid: id, Name: in.Name, Timezone: in.Timezone, Layers: layers, Overrides: overrides})
}

func (s *Service) current(ctx context.Context, org int64, id uuid.UUID, at time.Time) ([]uuid.UUID, error) {
	r, err := s.Repo.Q.OncallRota(ctx, db.OncallRotaParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return nil, err
	}
	var ls []Layer
	var os []Override
	if err = json.Unmarshal(r.Layers, &ls); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(r.Overrides, &os); err != nil {
		return nil, err
	}
	return Agents(ls, os, at), nil
}

func (s *Service) targets(ctx context.Context, org int64, step Step) ([]uuid.UUID, error) {
	if step.Agent != nil {
		if err := s.member(ctx, org, *step.Agent); err != nil {
			return nil, err
		}
		return []uuid.UUID{*step.Agent}, nil
	}
	if step.Rota != nil {
		return s.current(ctx, org, *step.Rota, time.Now())
	}
	if step.Team != nil {
		ids, err := s.Repo.Q.OncallTeamMembers(ctx, db.OncallTeamMembersParams{OrganizationID: org, Uuid: *step.Team})
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 || len(ids) > 500 {
			return nil, supportops.ErrInvalid
		}
		return ids, nil
	}
	return nil, supportops.ErrInvalid
}

func (s *Service) Policy(ctx context.Context, org int64, id uuid.UUID, in PolicyInput) (db.EscalationPolicy, error) {
	if !nameValid(in.Name) || len(in.Steps) == 0 || len(in.Steps) > 20 {
		return db.EscalationPolicy{}, supportops.ErrInvalid
	}
	previous := -1
	for _, st := range in.Steps {
		n := 0
		for _, p := range []*uuid.UUID{st.Agent, st.Rota, st.Team} {
			if p != nil {
				n++
			}
		}
		if n != 1 || st.AfterMinutes < 0 || st.AfterMinutes > 10080 || st.AfterMinutes < previous || len(st.Channels) == 0 || len(st.Channels) > 4 {
			return db.EscalationPolicy{}, supportops.ErrInvalid
		}
		previous = st.AfterMinutes
		for _, ch := range st.Channels {
			if !slices.Contains([]string{notif.ChannelInapp, notif.ChannelPush, notif.ChannelEmail, notif.ChannelSMS}, ch) {
				return db.EscalationPolicy{}, supportops.ErrInvalid
			}
		}
		if _, err := s.targets(ctx, org, st); err != nil {
			return db.EscalationPolicy{}, err
		}
	}
	raw, err := json.Marshal(in.Steps)
	if err != nil {
		return db.EscalationPolicy{}, err
	}
	if id == uuid.Nil {
		return s.Repo.Q.OncallCreatePolicy(ctx, db.OncallCreatePolicyParams{OrganizationID: org, Name: in.Name, Steps: raw, UrgentConversations: in.UrgentConversations, Incidents: in.Incidents, Enabled: in.Enabled})
	}
	return s.Repo.Q.OncallUpdatePolicy(ctx, db.OncallUpdatePolicyParams{OrganizationID: org, Uuid: id, Name: in.Name, Steps: raw, UrgentConversations: in.UrgentConversations, Incidents: in.Incidents, Enabled: in.Enabled})
}

func (s *Service) Monitor(ctx context.Context, org int64, id uuid.UUID, in MonitorInput) (db.UptimeMonitor, error) {
	if !nameValid(in.Name) || in.IntervalSeconds < 60 || in.IntervalSeconds > 86400 || in.ExpectedStatus < 100 || in.ExpectedStatus > 599 || len(in.Keyword) > 1000 || webhook.ValidateMonitorURL(in.URL) != nil {
		return db.UptimeMonitor{}, supportops.ErrInvalid
	}
	var policy pgtype.UUID
	if in.Policy != nil {
		if _, err := s.Repo.Q.OncallPolicy(ctx, db.OncallPolicyParams{OrganizationID: org, Uuid: *in.Policy}); err != nil {
			return db.UptimeMonitor{}, supportops.ErrInvalid
		}
		policy = pgtype.UUID{Bytes: *in.Policy, Valid: true}
	}
	if id == uuid.Nil {
		return s.Repo.Q.OncallCreateMonitor(ctx, db.OncallCreateMonitorParams{OrganizationID: org, Name: in.Name, Url: in.URL, IntervalSeconds: in.IntervalSeconds, ExpectedStatus: in.ExpectedStatus, Keyword: in.Keyword, ComponentUuid: in.Component, PolicyUuid: policy, Enabled: in.Enabled})
	}
	return s.Repo.Q.OncallUpdateMonitor(ctx, db.OncallUpdateMonitorParams{ComponentUuid: in.Component, PolicyUuid: policy, OrganizationID: org, Uuid: id, Name: in.Name, Url: in.URL, IntervalSeconds: in.IntervalSeconds, ExpectedStatus: in.ExpectedStatus, Keyword: in.Keyword, Enabled: in.Enabled})
}

func (s *Service) Probe(ctx context.Context, m db.UptimeMonitor) (bool, int32, int64) {
	start := time.Now()
	if webhook.ValidateMonitorURL(m.Url) != nil {
		return false, 0, 0
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, m.Url, nil)
	if err != nil {
		return false, 0, 0
	}
	response, err := s.Client.Do(request)
	if err != nil {
		return false, 0, time.Since(start).Milliseconds()
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
	status := safeconv.ToInt32(response.StatusCode)
	return err == nil && len(data) <= 1024*1024 && status == m.ExpectedStatus && (m.Keyword == "" || strings.Contains(string(data), m.Keyword)), status, time.Since(start).Milliseconds()
}

func (s *Service) SweepMonitors(ctx context.Context) error {
	if err := s.Repo.Q.OncallPurgeResults(ctx); err != nil {
		return err
	}
	rows, err := s.Repo.Q.OncallMonitorDue(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, m := range rows {
		if err = s.check(ctx, m.OrganizationID, m.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) check(ctx context.Context, org, id int64) error {
	return s.Repo.Within(ctx, func(r *repository.Repository) error {
		m, err := r.Q.OncallLockMonitor(ctx, db.OncallLockMonitorParams{OrganizationID: org, ID: id})
		if err != nil {
			return err
		}
		probeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		up, status, latency := s.Probe(probeCtx, m)
		if err = r.Q.OncallInsertResult(ctx, db.OncallInsertResultParams{OrganizationID: org, MonitorID: id, Up: up, StatusCode: status, LatencyMs: latency}); err != nil {
			return err
		}
		incident := m.IncidentID
		if !up && !incident.Valid {
			i, err := r.Q.StatusCreateIncident(ctx, db.StatusCreateIncidentParams{OrganizationID: org, Title: "Uptime: " + m.Name, Kind: "incident", Severity: "major_outage", Status: "investigating", Public: true, StartedAt: ts(time.Now()), NotificationBody: "", Postmortem: ""})
			if err != nil {
				return err
			}
			incident = pgtype.Int8{Int64: i.ID, Valid: true}
			if err = r.Q.OncallLinkComponent(ctx, db.OncallLinkComponentParams{OrganizationID: org, IncidentID: i.ID, ComponentID: m.ComponentID}); err != nil {
				return err
			}
			u, err := r.Q.StatusAddUpdate(ctx, db.StatusAddUpdateParams{OrganizationID: org, IncidentID: i.ID, Status: "investigating", Body: "HTTP uptime check failed"})
			if err != nil {
				return err
			}
			if err = r.Emit(ctx, events.New("status.changed").WithOrganization(org).WithEntity("incident", &i.ID, &i.Uuid).WithPayload(map[string]any{"update_uuid": u, "body": "HTTP uptime check failed", "public": true, "status": "investigating"}).WithIdempotencyKey(u.String())); err != nil {
				return err
			}
			if m.PolicyID.Valid {
				if err = r.Q.OncallOpenIncidentAlert(ctx, db.OncallOpenIncidentAlertParams{OrganizationID: org, ID: m.PolicyID.Int64, ID_2: i.ID}); err != nil {
					return err
				}
			}
		}
		if up && incident.Valid {
			i, err := r.Q.OncallIncidentByID(ctx, db.OncallIncidentByIDParams{OrganizationID: org, ID: incident.Int64})
			if err != nil {
				return err
			}
			if i.Status != "resolved" {
				i, err = r.Q.StatusSetIncidentState(ctx, db.StatusSetIncidentStateParams{OrganizationID: org, Uuid: i.Uuid, Status: "resolved", Postmortem: i.Postmortem})
				if err != nil {
					return err
				}
				u, err := r.Q.StatusAddUpdate(ctx, db.StatusAddUpdateParams{OrganizationID: org, IncidentID: i.ID, Status: "resolved", Body: "HTTP uptime check recovered"})
				if err != nil {
					return err
				}
				if err = r.Emit(ctx, events.New("status.changed").WithOrganization(org).WithEntity("incident", &i.ID, &i.Uuid).WithPayload(map[string]any{"update_uuid": u, "body": "HTTP uptime check recovered", "public": true, "status": "resolved"}).WithIdempotencyKey(u.String())); err != nil {
					return err
				}
			}
			incident = pgtype.Int8{}
		}
		if err = r.Q.OncallCloseSourceAlerts(ctx, org); err != nil {
			return err
		}
		return r.Q.OncallFinishMonitor(ctx, db.OncallFinishMonitorParams{OrganizationID: org, ID: id, LastUp: pgtype.Bool{Bool: up, Valid: true}, LastStatus: status, IncidentID: incident})
	})
}

func (s *Service) SweepAlerts(ctx context.Context) error {
	rows, err := s.Repo.Q.OncallAlertsDue(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, a := range rows {
		if err = s.escalate(ctx, a.OrganizationID, a.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) escalate(ctx context.Context, org, id int64) error {
	return s.Repo.Within(ctx, func(r *repository.Repository) error {
		if err := r.Q.OncallCloseSourceAlerts(ctx, org); err != nil {
			return err
		}
		a, err := r.Q.OncallLockAlert(ctx, db.OncallLockAlertParams{OrganizationID: org, ID: id})
		if err != nil {
			return err
		}
		var steps []Step
		if err = json.Unmarshal(a.Steps, &steps); err != nil {
			return err
		}
		n := int(a.NextStep)
		// Exhausted policies remain unacknowledged, visible in the alert list; no repeating pages.
		next := time.Now().AddDate(100, 0, 0)
		if a.PolicyEnabled && n < len(steps) {
			due := a.CreatedAt.Time.Add(time.Duration(steps[n].AfterMinutes) * time.Minute)
			if time.Now().Before(due) {
				return r.Q.OncallSetNextAlert(ctx, db.OncallSetNextAlertParams{OrganizationID: org, ID: id, NextNotifyAt: ts(due)})
			}
			ids, err := s.targets(ctx, org, steps[n])
			// Removed agents, teams or rotas must not prevent later fallback steps.
			if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, supportops.ErrInvalid) {
				return err
			}
			if len(ids) > 0 {
				if err = r.Emit(ctx, events.New("oncall.page").WithOrganization(org).WithEntity("oncall_alert", &id, &a.Uuid).WithPayload(map[string]any{"recipients": ids, "channels": steps[n].Channels}).WithIdempotencyKey(a.Uuid.String()+":"+strconv.Itoa(n))); err != nil {
					return err
				}
			}
			if n+1 < len(steps) {
				next = a.CreatedAt.Time.Add(time.Duration(steps[n+1].AfterMinutes) * time.Minute)
			}
		}
		return r.Q.OncallAdvanceAlert(ctx, db.OncallAdvanceAlertParams{OrganizationID: org, ID: id, NextNotifyAt: ts(next)})
	})
}

func (s *Service) Consumer() eventdispatch.Consumer {
	return eventdispatch.Consumer{Name: "oncall", Events: []string{"oncall.page", "status.changed", "conversation.*", "conversations.*"}, MaxRetry: 8, Timeout: time.Minute, Handle: s.Consume}
}

func (s *Service) Consume(ctx context.Context, e events.Event) error {
	if e.TenantID == nil {
		return nil
	}
	org := *e.TenantID
	if e.Name == "oncall.page" {
		if e.EntityUUID == nil {
			return nil
		}
		if _, err := s.Repo.Q.OncallAlertActive(ctx, db.OncallAlertActiveParams{OrganizationID: org, Uuid: *e.EntityUUID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		var ids []uuid.UUID
		var channels []string
		b, err := json.Marshal(e.Payload["recipients"])
		if err != nil {
			return err
		}
		if err = json.Unmarshal(b, &ids); err != nil {
			return err
		}
		b, err = json.Marshal(e.Payload["channels"])
		if err != nil {
			return err
		}
		if err = json.Unmarshal(b, &channels); err != nil {
			return err
		}
		if s.Notifier == nil {
			return errors.New("oncall notifier unavailable")
		}
		for _, id := range ids {
			m, err := s.Repo.Q.ReportsMember(ctx, db.ReportsMemberParams{OrganizationID: org, Uuid: id})
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			title, _ := e.Payload["title"].(string)
			if _, err = s.Notifier.Enqueue(ctx, notif.EnqueueInput{TenantID: &org, UserID: &m.ID, Channels: channels, Title: "ClickFix nöbet uyarısı", Body: "Açık bir nöbet uyarısı var. Üstlenmek için ClickFix'i açın.", Priority: notif.PriorityCritical, SourceEvent: "oncall.page", Payload: map[string]any{"alert_uuid": e.EntityUUID.String()},
				ContentVars: map[string]string{"title": title}, DedupeKey: "oncall.page:" + e.EventID.String()}); err != nil {
				return err
			}
		}
		return nil
	}
	if e.EntityID == nil {
		return nil
	}
	var incident, conversation pgtype.Int8
	switch e.EntityType {
	case "incident":
		incident = pgtype.Int8{Int64: *e.EntityID, Valid: true}
	case "conversation":
		conversation = pgtype.Int8{Int64: *e.EntityID, Valid: true}
	default:
		return nil
	}
	if err := s.Repo.Q.OncallCloseSourceAlerts(ctx, org); err != nil {
		return err
	}
	return s.Repo.Q.OncallOpenSourceAlerts(ctx, db.OncallOpenSourceAlertsParams{OrganizationID: org, IncidentID: incident, ConversationID: conversation})
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: !t.IsZero()} }
