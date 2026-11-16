package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/actionlink"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/content"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/providers"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/templates"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/observability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/mail"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/queue"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrForbidden      = errors.New("forbidden")
	ErrInvalidRequest = errors.New("invalid request")
)

// Querier is the persistence surface used by the notification service.
type Querier interface {
	CreateNotification(ctx context.Context, arg db.CreateNotificationParams) (db.Notification, error)
	GetNotificationByUUID(ctx context.Context, arg uuid.UUID) (db.Notification, error)
	GetNotificationByID(ctx context.Context, id int64) (db.Notification, error)
	ListNotificationsForUser(ctx context.Context, arg db.ListNotificationsForUserParams) ([]db.Notification, error)
	CountNotificationsForUser(ctx context.Context, arg db.CountNotificationsForUserParams) (int64, error)
	CountUnreadInappForUser(ctx context.Context, userID pgtype.Int8) (int64, error)
	ListPlatformNotifications(ctx context.Context, arg db.ListPlatformNotificationsParams) ([]db.Notification, error)
	CountPlatformNotifications(ctx context.Context, arg db.CountPlatformNotificationsParams) (int64, error)
	MarkNotificationProcessing(ctx context.Context, id int64) (db.Notification, error)
	ResumeNotificationProcessing(ctx context.Context, id int64) (db.Notification, error)
	MarkNotificationDead(ctx context.Context, arg db.MarkNotificationDeadParams) (db.Notification, error)
	MarkNotificationSkipped(ctx context.Context, arg db.MarkNotificationSkippedParams) (db.Notification, error)
	ListStuckProcessingNotificationIDs(ctx context.Context, staleMinutes int32) ([]int64, error)
	MarkNotificationSent(ctx context.Context, arg db.MarkNotificationSentParams) (db.Notification, error)
	MarkNotificationFailed(ctx context.Context, arg db.MarkNotificationFailedParams) (db.Notification, error)
	MarkNotificationRead(ctx context.Context, arg db.MarkNotificationReadParams) (db.Notification, error)
	MarkAllNotificationsReadForUser(ctx context.Context, userID pgtype.Int8) (int64, error)
	InsertNotificationHistory(ctx context.Context, arg db.InsertNotificationHistoryParams) (db.NotificationHistory, error)
	GetTemplateByCodeChannelLang(ctx context.Context, arg db.GetTemplateByCodeChannelLangParams) (db.NotificationTemplate, error)
	GetNotificationPreferences(ctx context.Context, userID int64) (db.NotificationPreference, error)
	UpsertNotificationPreferences(ctx context.Context, arg db.UpsertNotificationPreferencesParams) (db.NotificationPreference, error)
	GetUserByID(ctx context.Context, id int64) (db.User, error)
	GetUserByUUID(ctx context.Context, id uuid.UUID) (db.User, error)
}

// Enqueuer schedules background delivery tasks.
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Service coordinates enqueue + delivery.
type Service struct {
	q            Querier
	queue        Enqueuer
	providers    map[string]providers.Provider
	log          *slog.Logger
	syncMode     bool // when queue is nil, deliver inline
	vapid        *VAPIDConfig
	expo         *expoClient
	pushQueue    Enqueuer
	actionSecret []byte
	actionTTL    time.Duration
	appURL       string
	now          func() time.Time
}

// New creates a notification service.
func New(q Querier, enq Enqueuer, provs []providers.Provider, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	m := make(map[string]providers.Provider, len(provs))
	for _, p := range provs {
		m[p.Channel()] = p
	}
	return &Service{
		q: q, queue: enq, providers: m, log: log, syncMode: enq == nil,
		actionTTL: actionlink.DefaultTTL, now: time.Now,
	}
}

// WithAppURL sets the public web origin: e-mail links to app pages are made
// absolute with it.
func (s *Service) WithAppURL(u string) *Service {
	s.appURL = strings.TrimRight(strings.TrimSpace(u), "/")
	return s
}

// WithActionSigner enables HMAC-signed action URLs (marks read on redeem).
func (s *Service) WithActionSigner(secret string, ttl time.Duration) *Service {
	if secret != "" {
		s.actionSecret = []byte(secret)
	}
	if ttl > 0 {
		s.actionTTL = ttl
	}
	return s
}

// Enqueue creates queued notification rows and schedules delivery.
func (s *Service) Enqueue(ctx context.Context, in model.EnqueueInput) ([]model.Notification, error) {
	if len(in.Channels) == 0 {
		return nil, fmt.Errorf("%w: channels required", ErrInvalidRequest)
	}
	lang := in.Language
	if lang == "" && in.UserID != nil {
		if u, err := s.q.GetUserByID(ctx, *in.UserID); err == nil && u.Locale != "" {
			lang = u.Locale
		}
	}
	if lang == "" {
		lang = "tr"
	}
	priority := in.Priority
	if priority == "" {
		priority = model.PriorityNormal
	}

	prefs := defaultPreferences()
	if in.UserID != nil {
		if p, err := s.loadPreferences(ctx, *in.UserID); err == nil {
			prefs = p
		}
	}
	now := s.now().UTC()
	lang = content.Lang(lang)
	// Enriched text of the event in the recipient's language (CF-157).
	var msg content.Message
	rich := false
	if in.ContentVars != nil {
		msg, rich = content.Build(in.SourceEvent, lang, in.ContentVars, now)
	}
	actionURL := in.ActionURL
	if rich && actionURL == nil {
		if a, ok := msg.Primary(); ok {
			u := a.URL
			actionURL = &u
		}
	}

	var out []model.Notification
	for _, ch := range in.Channels {
		ch = strings.TrimSpace(ch)
		if ch == "" {
			continue
		}
		if !channelAllowed(ch, prefs, in.SecurityEmail) {
			s.log.Debug("notification_channel_skipped_by_preference", "channel", ch, "template", in.TemplateCode)
			continue
		}
		// Catalog events (CF-9): per-event matrix, quiet hours, digest. Other
		// sources still honor quiet hours (CF-157).
		plan := channelPlan{}
		if in.UserID != nil && !in.SecurityEmail {
			plan = planChannel(prefs, in.SourceEvent, ch, priority, now)
		}
		if plan.skip {
			s.log.Debug("notification_channel_skipped_by_event_preference", "channel", ch, "event", in.SourceEvent)
			continue
		}
		if plan.asPush {
			ch = model.ChannelPush
		}
		title, body := in.Title, in.Body
		if rich {
			title, body = msg.Title, msg.Summary()
			if ch == model.ChannelEmail {
				email := msg
				if in.UserID != nil && s.hasDesktopSession(ctx, *in.UserID) {
					email = msg.WithDesktopAction()
				}
				body = email.EmailText(s.appURL)
				// The layout shows the primary action as its button.
				if a, ok := msg.Primary(); ok && actionURL != nil && *actionURL == a.URL {
					body = email.EmailBody(s.appURL)
				}
			}
		} else if in.TemplateCode != "" {
			tpl, err := s.q.GetTemplateByCodeChannelLang(ctx, db.GetTemplateByCodeChannelLangParams{
				Code: in.TemplateCode, Channel: ch, Language: lang,
			})
			if errors.Is(err, pgx.ErrNoRows) && lang != "en" {
				tpl, err = s.q.GetTemplateByCodeChannelLang(ctx, db.GetTemplateByCodeChannelLangParams{
					Code: in.TemplateCode, Channel: ch, Language: "en",
				})
			}
			if err == nil {
				title = templates.Render(tpl.Subject, in.TemplateVars)
				body = templates.Render(tpl.Body, in.TemplateVars)
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			} else if title == "" {
				title = in.TemplateCode
			}
		}
		if plan.digest {
			if rich {
				body = msg.Summary()
			}
			if err := s.holdForDigest(ctx, *in.UserID, in.TenantID, in.SourceEvent, title, body, actionURL); err != nil {
				return nil, err
			}
			continue
		}
		rowPayload := withPushVars(ch, in.Payload, in.TemplateVars)
		if plan.noPush {
			rowPayload["suppress_push"] = true
		}
		if rich {
			rowPayload["content"] = msg.PayloadValue()
			rowPayload["push"] = map[string]string{"title": msg.Push.Title, "body": msg.Push.Body}
			rowPayload["category"] = msg.Category
		}
		if in.DedupeKey != "" {
			if _, set := rowPayload["source_event_id"]; !set {
				rowPayload["source_event_id"] = in.DedupeKey
			}
		}
		if ch == model.ChannelEmail {
			rowPayload["locale"] = lang // picks the branded layout's footer copy
			if a, ok := msg.Primary(); rich && ok {
				rowPayload["action_label"] = a.Label // the layout's button text
			}
		}
		payload, err := json.Marshal(rowPayload)
		if err != nil {
			payload = []byte("{}")
		}
		recipient := textPtr(in.Recipient)
		if ch == model.ChannelEmail && !recipient.Valid && in.UserID != nil {
			if u, err := s.q.GetUserByID(ctx, *in.UserID); err == nil {
				recipient = pgtype.Text{String: u.Email, Valid: true}
			}
		}
		row, err := s.q.CreateNotification(ctx, db.CreateNotificationParams{
			UserID:       int8Ptr(in.UserID),
			Channel:      ch,
			Status:       model.StatusQueued,
			Priority:     priority,
			Title:        title,
			Body:         body,
			Payload:      payload,
			ActionUrl:    textPtr(actionURL),
			Recipient:    recipient,
			TemplateCode: pgtype.Text{String: in.TemplateCode, Valid: in.TemplateCode != ""},
			SourceEvent:  pgtype.Text{String: in.SourceEvent, Valid: in.SourceEvent != ""},
			ScheduledAt:  pgTime(plan.scheduledAt),
			MaxAttempts:  5,
			// The organization the notification is about (mobile push opens it there).
			OrganizationID: int8Ptr(in.TenantID),
		})
		if err != nil {
			return nil, fmt.Errorf("create notification: %w", err)
		}
		if row.Status != "" && row.Status != model.StatusQueued {
			// Same source_event_id already handled (redelivered event): the
			// existing row is the result, nothing is sent again.
			out = append(out, s.project(row))
			continue
		}
		_, _ = s.q.InsertNotificationHistory(ctx, db.InsertNotificationHistoryParams{
			NotificationID: row.ID,
			Event:          "queued",
			Metadata:       []byte("{}"),
		})
		if err := s.scheduleDeliverAt(ctx, row.ID, plan.scheduledAt); err != nil {
			return nil, err
		}
		out = append(out, s.project(row))
	}
	return out, nil
}

// desktopSessions is implemented by db.Queries (CF-236); test fakes may
// leave it out, then no desktop link is added.
type desktopSessions interface {
	UserHasDesktopSession(ctx context.Context, userID int64) (bool, error)
}

// hasDesktopSession reports whether the user is signed in to ClickFix
// Masaüstü, so the e-mail can offer "Masaüstünde aç". Lookup errors only
// drop the link.
func (s *Service) hasDesktopSession(ctx context.Context, userID int64) bool {
	q, ok := s.q.(desktopSessions)
	if !ok {
		return false
	}
	has, err := q.UserHasDesktopSession(ctx, userID)
	if err != nil {
		s.log.Debug("desktop_session_lookup_failed", "error", err)
		return false
	}
	return has
}

// scheduleDeliverAt delivers now or, for e-mail held by quiet hours, at a
// later time (inline mode has no scheduler and delivers immediately).
func (s *Service) scheduleDeliverAt(ctx context.Context, id int64, at *time.Time) error {
	if s.syncMode || s.queue == nil {
		return s.Deliver(ctx, id)
	}
	task, err := queue.NewNotificationDeliverTask(id)
	if err != nil {
		return err
	}
	opts := []asynq.Option{asynq.Queue(queue.QueueNotifications)}
	if at != nil {
		opts = append(opts, asynq.ProcessAt(*at))
	}
	_, err = s.queue.Enqueue(queue.WithRequestID(ctx, task), opts...)
	return err
}

const DefaultStuckProcessingMinutes int32 = 2

// Deliver processes one queued notification.
//
// Each run is one attempt (attempt_count). A transient failure (network,
// timeout, SMTP 4xx) marks the row failed and returns an error so the queue
// retries it with backoff; a permanent failure (no SMTP configured, rejected
// recipient, bad credentials) or the last allowed attempt dead-letters the
// row (failed, no attempts left) and returns nil: retrying cannot help and
// the row itself is the record.
func (s *Service) Deliver(ctx context.Context, id int64) error {
	row, err := s.q.MarkNotificationProcessing(ctx, id)
	resumed := false
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		existing, getErr := s.q.GetNotificationByID(ctx, id)
		if getErr != nil {
			if errors.Is(getErr, pgx.ErrNoRows) {
				return nil
			}
			return getErr
		}
		if existing.Status != model.StatusProcessing {
			// sent/delivered/read/cancelled, or failed with no attempts left.
			return nil
		}
		row, err = s.q.ResumeNotificationProcessing(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			// Stuck in processing with every attempt used: stop here.
			s.deadLetter(ctx, existing, errors.New("notification: delivery did not finish within the allowed attempts"))
			return nil
		}
		if err != nil {
			return err
		}
		resumed = true
	}
	if !resumed {
		_, _ = s.q.InsertNotificationHistory(ctx, db.InsertNotificationHistoryParams{
			NotificationID: row.ID, Event: "processing", Metadata: []byte("{}"),
		})
	}
	p, ok := s.providers[row.Channel]
	if !ok {
		p = providers.NoopProvider{Name: row.Channel, Log: s.log}
	}
	var userUUID *uuid.UUID
	userLocale := ""
	if row.UserID.Valid {
		if u, err := s.q.GetUserByID(ctx, row.UserID.Int64); err == nil {
			uid := u.Uuid
			userUUID = &uid
			userLocale = u.Locale
		}
	}
	res, err := p.Deliver(ctx, row, userUUID)
	if err != nil {
		var skip *providers.SkipError
		if errors.As(err, &skip) {
			s.skipped(ctx, row, skip.Reason)
			return nil
		}
		return s.deliveryFailed(ctx, row, err)
	}
	// Status writes must land even when the task context was cancelled
	// mid-send; otherwise the row stays in processing and is resent.
	writeCtx := context.WithoutCancel(ctx)
	status := res.Status
	if status == "" {
		status = model.StatusSent
	}
	_, err = s.q.MarkNotificationSent(writeCtx, db.MarkNotificationSentParams{
		ID: id, Status: status,
		Provider:          pgtype.Text{String: res.Provider, Valid: res.Provider != ""},
		ProviderReference: pgtype.Text{String: res.ProviderReference, Valid: res.ProviderReference != ""},
	})
	if err != nil {
		return err
	}
	_, _ = s.q.InsertNotificationHistory(writeCtx, db.InsertNotificationHistoryParams{
		NotificationID: id, Event: status, Metadata: []byte("{}"),
	})

	// Fan-out realtime mirror for inapp deliveries.
	if row.Channel == model.ChannelInapp {
		if rp, ok := s.providers[model.ChannelRealtime]; ok && userUUID != nil {
			_, _ = rp.Deliver(ctx, row, userUUID)
		}
	}
	// Push fan-out: browser Web Push (VAPID) + mobile devices (Expo). In-app
	// rows are mirrored; explicit push-channel rows use the same path.
	if (row.Channel == model.ChannelInapp || row.Channel == model.ChannelPush) && !pushSuppressed(row) {
		s.scheduleMobilePush(ctx, row)
		if row.UserID.Valid {
			title, body, data := WebPushContent(row, userLocale)
			s.pushBestEffort(ctx, row.UserID.Int64, title, body, data)
		}
	}
	return nil
}

// RetryError is a transient delivery failure the queue should retry.
type RetryError struct {
	Cause   string
	Attempt int32
	Err     error
}

func (e *RetryError) Error() string {
	return fmt.Sprintf("notification delivery attempt %d failed (%s): %v", e.Attempt, e.Cause, e.Err)
}

func (e *RetryError) Unwrap() error { return e.Err }

// deliveryFailed records a failed attempt and decides between retry and
// dead letter.
func (s *Service) deliveryFailed(ctx context.Context, row db.Notification, err error) error {
	cause, transient := deliveryCause(err)
	if !transient || row.AttemptCount >= row.MaxAttempts || s.syncMode {
		// Inline (queue-less) delivery has no retry loop: record and stop.
		s.deadLetter(ctx, row, err)
		return nil
	}
	writeCtx := context.WithoutCancel(ctx)
	_, _ = s.q.MarkNotificationFailed(writeCtx, db.MarkNotificationFailedParams{
		ID: row.ID, LastError: pgtype.Text{String: scrubError(err), Valid: true},
	})
	_, _ = s.q.InsertNotificationHistory(writeCtx, db.InsertNotificationHistoryParams{
		NotificationID: row.ID, Event: "failed",
		Metadata: mustJSON(map[string]any{
			"error": scrubError(err), "cause": cause,
			"attempt": row.AttemptCount, "max_attempts": row.MaxAttempts, "will_retry": true,
		}),
	})
	s.log.WarnContext(ctx, "notification_delivery_retry",
		"id", row.ID, "channel", row.Channel, "cause", cause,
		"attempt", row.AttemptCount, "max_attempts", row.MaxAttempts, "error", scrubError(err))
	return &RetryError{Cause: cause, Attempt: row.AttemptCount, Err: err}
}

// deadLetter marks the row failed with no attempts left. The error tracker
// gets one event per channel and cause per window, not one per row.
func (s *Service) deadLetter(ctx context.Context, row db.Notification, err error) {
	cause, _ := deliveryCause(err)
	msg := scrubError(err)
	writeCtx := context.WithoutCancel(ctx)
	_, _ = s.q.MarkNotificationDead(writeCtx, db.MarkNotificationDeadParams{
		ID: row.ID, LastError: pgtype.Text{String: msg, Valid: true},
	})
	_, _ = s.q.InsertNotificationHistory(writeCtx, db.InsertNotificationHistoryParams{
		NotificationID: row.ID, Event: "dead_lettered",
		Metadata: mustJSON(map[string]any{
			"error": msg, "cause": cause, "attempt": row.AttemptCount, "max_attempts": row.MaxAttempts,
		}),
	})
	s.log.WarnContext(ctx, "notification_dead_lettered",
		"id", row.ID, "channel", row.Channel, "cause", cause, "attempts", row.AttemptCount, "error", msg)
	if !observability.Enabled() || !observability.Allow("notification-dead:"+row.Channel+":"+cause, deadLetterReportWindow) {
		return
	}
	opts := observability.Options{
		Level:       sentry.LevelError,
		Transaction: "notification delivery " + row.Channel,
		Fingerprint: []string{"notification-dead-letter", row.Channel, cause},
		Tags:        map[string]string{"channel": row.Channel, "cause": cause, "source": sourceOf(row)},
	}
	if recipientRejected(cause) {
		// The provider refused this recipient for good (bad or unknown
		// address). Nothing is broken on our side: a warning, not an error.
		opts.Level = sentry.LevelWarning
		opts.Fingerprint = []string{"notification-recipient-rejected", row.Channel, cause}
		observability.CaptureMessage(ctx, "notification recipient rejected ("+row.Channel+", "+cause+")", opts)
		return
	}
	observability.CaptureError(ctx, err, opts)
}

// recipientRejected reports causes where the provider permanently refused
// the recipient or message (SMTP 5xx): retrying cannot help and the
// platform is healthy. Configuration, TLS and auth failures stay errors.
func recipientRejected(cause string) bool {
	return cause == mail.CauseRejected
}

// sourceOf is the low-cardinality origin of a row (event or template code),
// so a report shows which job sent it without any personal data.
func sourceOf(row db.Notification) string {
	if row.SourceEvent.Valid && row.SourceEvent.String != "" {
		return row.SourceEvent.String
	}
	if row.TemplateCode.Valid && row.TemplateCode.String != "" {
		return row.TemplateCode.String
	}
	return "unknown"
}

// skipped records an intentionally unsent row (status cancelled, history
// event "skipped"). It is not a failure: no retry, no error report.
func (s *Service) skipped(ctx context.Context, row db.Notification, reason string) {
	writeCtx := context.WithoutCancel(ctx)
	_, _ = s.q.MarkNotificationSkipped(writeCtx, db.MarkNotificationSkippedParams{
		ID: row.ID, LastError: pgtype.Text{String: "skipped: " + reason, Valid: true},
	})
	_, _ = s.q.InsertNotificationHistory(writeCtx, db.InsertNotificationHistoryParams{
		NotificationID: row.ID, Event: "skipped",
		Metadata: mustJSON(map[string]any{"reason": reason, "attempt": row.AttemptCount}),
	})
	s.log.InfoContext(ctx, "notification_skipped",
		"id", row.ID, "channel", row.Channel, "reason", reason, "source", sourceOf(row))
}

// deadLetterReportWindow: at most one tracker event per channel and cause.
const deadLetterReportWindow = time.Hour

// deliveryCause classifies a provider error: a stable cause label and
// whether a later retry can succeed.
func deliveryCause(err error) (string, bool) {
	if errors.Is(err, providers.ErrNoRecipient) {
		return "no_recipient", false
	}
	return mail.Cause(err), mail.IsTransient(err)
}

func scrubError(err error) string {
	if err == nil {
		return ""
	}
	return observability.ScrubString(err.Error())
}

// ReclaimStuck hands rows left in processing (worker crash, lost status
// write) back to delivery. With a queue it only enqueues them (no inline
// SMTP at startup); each resumed run counts as an attempt, so a delivery
// that keeps hanging is dead-lettered instead of reclaimed forever.
func (s *Service) ReclaimStuck(ctx context.Context, staleMinutes int32) (int, error) {
	if staleMinutes < 0 {
		staleMinutes = 0
	}
	ids, err := s.q.ListStuckProcessingNotificationIDs(ctx, staleMinutes)
	if err != nil {
		return 0, fmt.Errorf("list stuck notifications: %w", err)
	}
	var n int
	for _, id := range ids {
		if s.syncMode || s.queue == nil {
			if err := s.Deliver(ctx, id); err != nil {
				s.log.WarnContext(ctx, "notification_reclaim_failed", "id", id, "error", scrubError(err))
				continue
			}
			n++
			continue
		}
		task, err := queue.NewNotificationDeliverTask(id)
		if err != nil {
			return n, err
		}
		_, err = s.queue.Enqueue(task, asynq.Queue(queue.QueueNotifications), asynq.TaskID(fmt.Sprintf("notification-reclaim:%d", id)))
		if err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) && !errors.Is(err, asynq.ErrDuplicateTask) {
			s.log.WarnContext(ctx, "notification_reclaim_failed", "id", id, "error", scrubError(err))
			continue
		}
		n++
	}
	return n, nil
}

// ListInbox returns the caller's notifications page.
func (s *Service) ListInbox(ctx context.Context, userID int64, q apiquery.Query, status, channel string, unread *bool) (apiquery.Page[model.Notification], error) {
	if err := apiquery.ValidateSort(q.Sort, apiquery.NotificationsSort); err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	params := db.ListNotificationsForUserParams{
		UserID:      pgtype.Int8{Int64: userID, Valid: true},
		Status:      optionalText(status),
		Channel:     optionalText(channel),
		Unread:      optionalBool(unread),
		Q:           optionalText(q.Q),
		LimitCount:  q.Limit,
		OffsetCount: q.Offset,
	}
	rows, err := s.q.ListNotificationsForUser(ctx, params)
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	total, err := s.q.CountNotificationsForUser(ctx, db.CountNotificationsForUserParams{
		UserID: params.UserID, Status: params.Status, Channel: params.Channel,
		Unread: params.Unread, Q: params.Q,
	})
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	items := make([]model.Notification, 0, len(rows))
	for _, r := range rows {
		items = append(items, s.project(r))
	}
	return apiquery.NewPage(items, total, q.Limit, q.Offset), nil
}

// UnreadCount returns unread in-app count.
func (s *Service) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	return s.q.CountUnreadInappForUser(ctx, pgtype.Int8{Int64: userID, Valid: true})
}

// GetOwned returns a notification owned by userID.
func (s *Service) GetOwned(ctx context.Context, userID int64, id uuid.UUID) (model.Notification, error) {
	row, err := s.q.GetNotificationByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Notification{}, ErrNotFound
		}
		return model.Notification{}, err
	}
	if !row.UserID.Valid || row.UserID.Int64 != userID {
		return model.Notification{}, ErrForbidden
	}
	return s.project(row), nil
}

// MarkRead marks one notification read.
func (s *Service) MarkRead(ctx context.Context, userID int64, id uuid.UUID) (model.Notification, error) {
	row, err := s.q.MarkNotificationRead(ctx, db.MarkNotificationReadParams{
		Uuid: id, UserID: pgtype.Int8{Int64: userID, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Notification{}, ErrNotFound
		}
		return model.Notification{}, err
	}
	return s.project(row), nil
}

// RedeemAction verifies a signed action link, marks the in-app row read, and
// returns the stored destination.
func (s *Service) RedeemAction(ctx context.Context, userID int64, id uuid.UUID, exp int64, sig string) (model.Notification, string, error) {
	row, err := s.q.GetNotificationByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Notification{}, "", ErrNotFound
		}
		return model.Notification{}, "", err
	}
	if !row.UserID.Valid || row.UserID.Int64 != userID {
		return model.Notification{}, "", ErrForbidden
	}
	if !row.ActionUrl.Valid || strings.TrimSpace(row.ActionUrl.String) == "" {
		return model.Notification{}, "", fmt.Errorf("%w: action url missing", ErrInvalidRequest)
	}
	dest := row.ActionUrl.String
	if !actionlink.Verify(s.actionSecret, id, dest, sig, exp) {
		return model.Notification{}, "", fmt.Errorf("%w: invalid or expired action link", ErrInvalidRequest)
	}
	if row.Channel == model.ChannelInapp && !row.ReadAt.Valid {
		marked, markErr := s.q.MarkNotificationRead(ctx, db.MarkNotificationReadParams{
			Uuid: id, UserID: pgtype.Int8{Int64: userID, Valid: true},
		})
		if markErr == nil {
			row = marked
		}
	}
	return s.project(row), dest, nil
}

// MarkAllRead marks all unread in-app as read.
func (s *Service) MarkAllRead(ctx context.Context, userID int64) error {
	_, err := s.q.MarkAllNotificationsReadForUser(ctx, pgtype.Int8{Int64: userID, Valid: true})
	return err
}

// GetPreferences returns preferences with defaults.
func (s *Service) GetPreferences(ctx context.Context, userID int64) (model.Preferences, error) {
	return s.loadPreferences(ctx, userID)
}

// UpsertPreferences stores the master switches (other settings unchanged).
func (s *Service) UpsertPreferences(ctx context.Context, userID int64, p model.Preferences) (model.Preferences, error) {
	return s.UpdatePreferences(ctx, userID, model.PreferencesUpdate{
		EmailEnabled: &p.EmailEnabled, InappEnabled: &p.InappEnabled,
		RealtimeEnabled: &p.RealtimeEnabled, PushEnabled: &p.PushEnabled,
	})
}

// pushSuppressed reports a row whose event preferences or quiet hours turned
// the push mirror off.
func pushSuppressed(row db.Notification) bool {
	var payload map[string]any
	if json.Unmarshal(row.Payload, &payload) != nil {
		return false
	}
	v, _ := payload["suppress_push"].(bool)
	return v
}

// ListPlatform returns notifications for platform operators.
// Without platform.notifications.read_all the page is always the caller's own rows.
func (s *Service) ListPlatform(
	ctx context.Context,
	actor authctx.Principal,
	q apiquery.Query,
	status, channel, scope, userUUID string,
) (apiquery.Page[model.Notification], error) {
	if err := apiquery.ValidateSort(q.Sort, apiquery.NotificationsSort); err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	audience, err := resolvePlatformAudience(actor, scope, userUUID, func(id uuid.UUID) (int64, error) {
		return s.ResolveUserID(ctx, id)
	})
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	params := db.ListPlatformNotificationsParams{
		Status: optionalText(status), Channel: optionalText(channel), Q: optionalText(q.Q),
		UserID: audience.UserID, LimitCount: q.Limit, OffsetCount: q.Offset,
	}
	rows, err := s.q.ListPlatformNotifications(ctx, params)
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	total, err := s.q.CountPlatformNotifications(ctx, db.CountPlatformNotificationsParams{
		Status: params.Status, Channel: params.Channel, Q: params.Q, UserID: params.UserID,
	})
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	items := make([]model.Notification, 0, len(rows))
	users := map[int64]model.NotificationUser{}
	for _, r := range rows {
		n := s.project(r)
		if audience.IncludeUser && r.UserID.Valid {
			if u, ok := users[r.UserID.Int64]; ok {
				copied := u
				n.User = &copied
			} else if row, err := s.q.GetUserByID(ctx, r.UserID.Int64); err == nil {
				u := model.NotificationUser{
					UUID: row.Uuid, Name: row.Name, Surname: row.Surname, Email: row.Email,
				}
				users[r.UserID.Int64] = u
				n.User = &u
			}
		}
		items = append(items, n)
	}
	return apiquery.NewPage(items, total, q.Limit, q.Offset), nil
}

// ResolveUserID resolves public uuid to internal id.
func (s *Service) ResolveUserID(ctx context.Context, id uuid.UUID) (int64, error) {
	u, err := s.q.GetUserByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return u.ID, nil
}

func channelAllowed(ch string, prefs model.Preferences, securityEmail bool) bool {
	switch ch {
	case model.ChannelEmail:
		return prefs.EmailEnabled || securityEmail
	case model.ChannelInapp:
		return prefs.InappEnabled
	case model.ChannelRealtime:
		return prefs.RealtimeEnabled
	default:
		return true
	}
}

func (s *Service) project(row db.Notification) model.Notification {
	n := model.Notification{
		UUID: row.Uuid, Channel: row.Channel, Status: row.Status, Priority: row.Priority,
		Title: row.Title, Body: row.Body, Payload: map[string]any{},
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	_ = json.Unmarshal(row.Payload, &n.Payload)
	if row.ActionUrl.Valid {
		dest := row.ActionUrl.String
		n.ActionURL = &dest
		if signed := actionlink.URL(s.actionSecret, row.Uuid, dest, s.actionTTL); signed != "" {
			n.SignedActionURL = &signed
		}
	}
	if row.Recipient.Valid {
		s := row.Recipient.String
		n.Recipient = &s
	}
	if row.TemplateCode.Valid {
		s := row.TemplateCode.String
		n.TemplateCode = &s
	}
	if row.SourceEvent.Valid {
		s := row.SourceEvent.String
		n.SourceEvent = &s
	}
	if row.SentAt.Valid {
		t := row.SentAt.Time
		n.SentAt = &t
	}
	if row.ReadAt.Valid {
		t := row.ReadAt.Time
		n.ReadAt = &t
	}
	return n
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func textPtr(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

func optionalText(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func optionalBool(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// Ensure time import used (CreatedAt mapping).
var _ = time.Time{}
