// Package platformhealth is the platform staff health overview (CF-14):
// background queue depth (Asynq), outbox backlog, channel error rates, AI
// provider status and the e-mail transport (SMTP reachability and
// notification delivery). GET /v1/platform/health (platform.organizations.read).
package platformhealth

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/observability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/mail"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Window is the period channel errors are counted over.
const Window = 24 * time.Hour

// Thresholds of the overall status.
const (
	QueueBacklogWarn     = 1000
	OutboxAgeWarnSeconds = 300
	ChannelFailWarnRatio = 0.10
)

// Queue is one Asynq queue.
type Queue struct {
	Name      string `json:"name"`
	Size      int    `json:"size"`
	Pending   int    `json:"pending"`
	Active    int    `json:"active"`
	Scheduled int    `json:"scheduled"`
	Retry     int    `json:"retry"`
	Archived  int    `json:"archived"`
	Failed    int    `json:"failed_today"`
	Processed int    `json:"processed_today"`
	Paused    bool   `json:"paused"`
}

// Outbox is the event outbox backlog.
type Outbox struct {
	Pending              int64 `json:"pending"`
	Failed               int64 `json:"failed"`
	OldestPendingSeconds int64 `json:"oldest_pending_seconds"`
}

// Channel is the health of one channel type over Window.
type Channel struct {
	Type                string  `json:"type"`
	Instances           int64   `json:"instances"`
	InstancesWithErrors int64   `json:"instances_with_errors"`
	OutboundTotal       int64   `json:"outbound_total"`
	OutboundFailed      int64   `json:"outbound_failed"`
	ErrorRate           float64 `json:"error_rate"`
}

// AIProvider is the reachability of an AI provider.
type AIProvider struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok, degraded, down, not_configured
	Detail string `json:"detail"`
}

// Mail is the e-mail transport: SMTP reachability from this host (probed at
// most every few minutes) and e-mail notification delivery over Window.
type Mail struct {
	mail.Status
	Sent         int64      `json:"sent"`
	Retrying     int64      `json:"retrying"`
	DeadLettered int64      `json:"dead_lettered"`
	Pending      int64      `json:"pending"`
	LastSentAt   *time.Time `json:"last_sent_at,omitempty"`
	LastFailedAt *time.Time `json:"last_failed_at,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
}

// MailStatus reports SMTP reachability (mail.Monitor).
type MailStatus interface {
	Status(ctx context.Context) mail.Status
}

// Overview is GET /v1/platform/health.
type Overview struct {
	Status        string    `json:"status"` // ok, degraded
	Issues        []string  `json:"issues"`
	GeneratedAt   time.Time `json:"generated_at"`
	WindowHours   int       `json:"window_hours"`
	Organizations struct {
		Active          int64 `json:"active"`
		Suspended       int64 `json:"suspended"`
		CreatedLastWeek int64 `json:"created_last_week"`
	} `json:"organizations"`
	Queues      []Queue      `json:"queues"`
	QueueError  string       `json:"queue_error,omitempty"`
	Outbox      Outbox       `json:"outbox"`
	Channels    []Channel    `json:"channels"`
	AIProviders []AIProvider `json:"ai_providers"`
	Mail        Mail         `json:"mail"`
}

// AIStatus reports AI provider health; the AI module registers one (nil =
// none configured yet).
type AIStatus interface {
	Providers(ctx context.Context) []AIProvider
}

// Service builds the overview.
type Service struct {
	q         *db.Queries
	inspector *asynq.Inspector
	ai        AIStatus
	mail      MailStatus
	now       func() time.Time
}

// NewService builds the service; inspector may be nil (no Redis).
func NewService(q *db.Queries, inspector *asynq.Inspector) *Service {
	return &Service{q: q, inspector: inspector, now: time.Now}
}

// SetAIStatus attaches AI provider health.
func (s *Service) SetAIStatus(ai AIStatus) { s.ai = ai }

// SetMailStatus attaches the SMTP transport monitor.
func (s *Service) SetMailStatus(m MailStatus) { s.mail = m }

// Overview collects every section; a failing section degrades the status
// instead of failing the request.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	now := s.now().UTC()
	o := Overview{Status: "ok", Issues: []string{}, GeneratedAt: now, WindowHours: int(Window / time.Hour)}
	counts, err := s.q.OrganizationCounts(ctx, pgtype.Timestamptz{Time: now.Add(-7 * 24 * time.Hour), Valid: true})
	if err != nil {
		return o, err
	}
	o.Organizations.Active, o.Organizations.Suspended, o.Organizations.CreatedLastWeek = counts.Active, counts.Suspended, counts.CreatedRecently

	o.Queues = []Queue{}
	if s.inspector == nil {
		o.QueueError = "queue inspector unavailable"
		o.Issues = append(o.Issues, "queues_unavailable")
	} else if names, err := s.inspector.Queues(); err != nil {
		o.QueueError = err.Error()
		o.Issues = append(o.Issues, "queues_unavailable")
	} else {
		sort.Strings(names)
		for _, n := range names {
			info, err := s.inspector.GetQueueInfo(n)
			if err != nil {
				continue
			}
			q := Queue{
				Name: n, Size: info.Size, Pending: info.Pending, Active: info.Active, Scheduled: info.Scheduled,
				Retry: info.Retry, Archived: info.Archived, Failed: info.Failed, Processed: info.Processed, Paused: info.Paused,
			}
			o.Queues = append(o.Queues, q)
			if q.Pending+q.Retry > QueueBacklogWarn {
				o.Issues = append(o.Issues, "queue_backlog:"+n)
			}
			if q.Paused {
				o.Issues = append(o.Issues, "queue_paused:"+n)
			}
		}
	}

	ob, err := s.q.OutboxHealth(ctx)
	if err != nil {
		return o, err
	}
	o.Outbox = Outbox{Pending: ob.Pending, Failed: ob.Failed, OldestPendingSeconds: ob.OldestPendingSeconds}
	if ob.OldestPendingSeconds > OutboxAgeWarnSeconds {
		o.Issues = append(o.Issues, "outbox_lagging")
	}
	if ob.Failed > 0 {
		o.Issues = append(o.Issues, "outbox_failed")
	}

	rows, err := s.q.ChannelHealth(ctx, pgtype.Timestamptz{Time: now.Add(-Window), Valid: true})
	if err != nil {
		return o, err
	}
	o.Channels = make([]Channel, 0, len(rows))
	for _, r := range rows {
		c := Channel{
			Type: r.ChannelType, Instances: r.Instances, InstancesWithErrors: r.InstancesWithErrors,
			OutboundTotal: r.OutboundTotal, OutboundFailed: r.OutboundFailed,
		}
		if c.OutboundTotal > 0 {
			c.ErrorRate = float64(c.OutboundFailed) / float64(c.OutboundTotal)
		}
		if c.ErrorRate > ChannelFailWarnRatio {
			o.Issues = append(o.Issues, "channel_errors:"+c.Type)
		}
		o.Channels = append(o.Channels, c)
	}

	o.AIProviders = []AIProvider{}
	if s.ai != nil {
		o.AIProviders = s.ai.Providers(ctx)
		for _, p := range o.AIProviders {
			if p.Status == "down" || p.Status == "degraded" {
				o.Issues = append(o.Issues, "ai_"+p.Status+":"+p.Name)
			}
		}
	}
	mailHealth, err := s.mailHealth(ctx, now)
	if err != nil {
		return o, err
	}
	o.Mail = mailHealth
	switch o.Mail.State {
	case mail.StateNotConfigured:
		o.Issues = append(o.Issues, "mail_not_configured")
	case mail.StateDown:
		o.Issues = append(o.Issues, "mail_unreachable:"+o.Mail.Cause)
	}
	if o.Mail.DeadLettered > 0 {
		o.Issues = append(o.Issues, "mail_dead_letters")
	}
	if len(o.Issues) > 0 {
		o.Status = "degraded"
	}
	return o, nil
}

func (s *Service) mailHealth(ctx context.Context, now time.Time) (Mail, error) {
	m := Mail{Status: mail.Status{State: mail.StateUnknown}}
	if s.mail != nil {
		m.Status = s.mail.Status(ctx)
	}
	row, err := s.q.EmailDeliveryHealth(ctx, pgtype.Timestamptz{Time: now.Add(-Window), Valid: true})
	if err != nil {
		return m, err
	}
	m.Sent, m.Retrying, m.DeadLettered, m.Pending = row.Sent, row.Retrying, row.Dead, row.Pending
	if row.LastSentAt.Valid {
		t := row.LastSentAt.Time.UTC()
		m.LastSentAt = &t
	}
	if row.LastFailedAt.Valid {
		t := row.LastFailedAt.Time.UTC()
		m.LastFailedAt = &t
		last, err := s.q.LastEmailDeliveryError(ctx)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return m, err
		}
		m.LastError = observability.ScrubString(last)
	}
	return m, nil
}

// RegisterRoutes mounts GET /v1/platform/health.
func RegisterRoutes(mux *http.ServeMux, svc *Service, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	mux.Handle("GET /v1/platform/health", middleware.Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o, err := svc.Overview(r.Context())
		if err != nil {
			response.InternalErr(w, r, err, "")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		response.JSON(w, r, http.StatusOK, o)
	}), middleware.Authenticate(tokens, loader), middleware.RequirePermission(rbac.PermPlatformOrganizationsRead)))
}
