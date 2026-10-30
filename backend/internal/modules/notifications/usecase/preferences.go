package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DefaultTimezone is used when a user has not picked one.
const DefaultTimezone = "Europe/Istanbul"

// eventCatalog is the preference matrix (CF-9): which helpdesk events notify
// a user and on which channels by default. The keys are the domain event
// names and the template codes.
var eventCatalog = []model.EventType{
	{
		Key: events.ConversationAssigned, Group: "inbox", Priority: model.PriorityHigh, Locked: []string{model.PrefChannelInapp},
		Defaults: map[string]bool{model.PrefChannelInapp: true, model.PrefChannelEmail: true, model.PrefChannelPush: true},
	},
	{
		Key: events.MessageReceived, Group: "inbox", Priority: model.PriorityNormal,
		Defaults: map[string]bool{model.PrefChannelInapp: true, model.PrefChannelEmail: false, model.PrefChannelPush: true},
	},
	{
		Key: events.ConversationMentioned, Group: "inbox", Priority: model.PriorityHigh, Locked: []string{model.PrefChannelInapp},
		Defaults: map[string]bool{model.PrefChannelInapp: true, model.PrefChannelEmail: true, model.PrefChannelPush: true},
	},
	{
		Key: events.TicketAssigned, Group: "tickets", Priority: model.PriorityHigh, Locked: []string{model.PrefChannelInapp},
		Defaults: map[string]bool{model.PrefChannelInapp: true, model.PrefChannelEmail: true, model.PrefChannelPush: true},
	},
	{
		Key: events.TicketCommented, Group: "tickets", Priority: model.PriorityNormal,
		Defaults: map[string]bool{model.PrefChannelInapp: true, model.PrefChannelEmail: false, model.PrefChannelPush: false},
	},
	{
		Key: events.SLAWarning, Group: "sla", Priority: model.PriorityHigh,
		Defaults: map[string]bool{model.PrefChannelInapp: true, model.PrefChannelEmail: false, model.PrefChannelPush: true},
	},
	{
		Key: events.SLABreached, Group: "sla", Priority: model.PriorityCritical, Locked: []string{model.PrefChannelInapp},
		Defaults: map[string]bool{model.PrefChannelInapp: true, model.PrefChannelEmail: true, model.PrefChannelPush: true},
	},
	{
		Key: events.CompanyHealthDropped, Group: "accounts", Priority: model.PriorityHigh,
		Defaults: map[string]bool{model.PrefChannelInapp: true, model.PrefChannelEmail: true, model.PrefChannelPush: false},
	},
	{
		Key: events.CSATSubmitted, Group: "quality", Priority: model.PriorityLow,
		Defaults: map[string]bool{model.PrefChannelInapp: true, model.PrefChannelEmail: false, model.PrefChannelPush: false},
	},
	// CF-157: every notification a member receives goes through the matrix.
	{
		Key: EventReminderDue, Group: "inbox", Priority: model.PriorityNormal,
		Defaults: on(true, false, true),
	},
	{
		Key: EventCallMissed, Group: "inbox", Priority: model.PriorityHigh,
		Defaults: on(true, false, true),
	},
	{
		Key: EventTeamchatMentioned, Group: "team", Priority: model.PriorityNormal,
		Defaults: on(true, false, true),
	},
	{Key: "appointment.booked", Group: "calendar", Priority: model.PriorityNormal, Defaults: on(true, true, true)},
	{Key: "appointment.rescheduled", Group: "calendar", Priority: model.PriorityNormal, Defaults: on(true, true, true)},
	{Key: "appointment.confirmed", Group: "calendar", Priority: model.PriorityLow, Defaults: on(true, false, false)},
	{Key: "appointment.cancelled", Group: "calendar", Priority: model.PriorityNormal, Defaults: on(true, true, true)},
	{Key: "appointment.reminder", Group: "calendar", Priority: model.PriorityHigh, Defaults: on(true, false, true)},
	{Key: "appointment.no_show", Group: "calendar", Priority: model.PriorityLow, Defaults: on(true, false, false)},
	{
		// The escalation policy picks the channels; the member cannot mute a page.
		Key: EventOncallPage, Group: "oncall", Priority: model.PriorityCritical,
		Locked: []string{model.PrefChannelInapp, model.PrefChannelPush}, Defaults: on(true, true, true),
	},
	{Key: "exports.ready", Group: "jobs", Priority: model.PriorityLow, Defaults: on(true, false, true)},
	{Key: "imports.applied", Group: "jobs", Priority: model.PriorityLow, Defaults: on(true, false, true)},
	{Key: "bulk.completed", Group: "jobs", Priority: model.PriorityLow, Defaults: on(true, false, true)},
	{Key: "bulk.failed", Group: "jobs", Priority: model.PriorityNormal, Defaults: on(true, false, true)},
	{Key: EventReportReady, Group: "jobs", Priority: model.PriorityLow, Defaults: on(true, true, false)},
	{Key: EventVideoBalanceLow, Group: "billing", Priority: model.PriorityHigh, Defaults: on(true, true, false)},
}

// Catalog keys of events that have no constant in platform/events.
const (
	EventCallMissed        = "calls.missed"
	EventReminderDue       = "conversation.reminder_due"
	EventTeamchatMentioned = "teamchat.mentioned"
	EventOncallPage        = "oncall.page"
	EventReportReady       = "reports.ready"
	EventVideoBalanceLow   = "video.balance_low"
)

// on is a defaults row: in-app, e-mail, push.
func on(inapp, email, push bool) map[string]bool {
	return map[string]bool{model.PrefChannelInapp: inapp, model.PrefChannelEmail: email, model.PrefChannelPush: push}
}

// EventTypes returns the preference matrix rows.
func EventTypes() []model.EventType {
	out := slices.Clone(eventCatalog)
	for i := range out {
		if out[i].Locked == nil {
			out[i].Locked = []string{}
		}
	}
	return out
}

func eventType(key string) (model.EventType, bool) {
	for _, e := range eventCatalog {
		if e.Key == key {
			return e, true
		}
	}
	return model.EventType{}, false
}

func defaultPreferences() model.Preferences {
	return model.Preferences{
		EmailEnabled: true, InappEnabled: true, RealtimeEnabled: true, PushEnabled: false,
		EventChannels: map[string]map[string]bool{},
		QuietHours:    model.QuietHours{Start: "22:00", End: "08:00", Timezone: DefaultTimezone},
		EmailDigest:   model.DigestOff,
		DigestHour:    9,
	}
}

func preferencesFromRow(row db.NotificationPreference) model.Preferences {
	p := model.Preferences{
		EmailEnabled: row.EmailEnabled, InappEnabled: row.InappEnabled, RealtimeEnabled: row.RealtimeEnabled,
		PushEnabled:   row.PushEnabled,
		EventChannels: map[string]map[string]bool{},
		QuietHours: model.QuietHours{
			Enabled: row.QuietHoursEnabled, Start: row.QuietHoursStart, End: row.QuietHoursEnd, Timezone: row.Timezone,
		},
		EmailDigest: row.EmailDigest,
		DigestHour:  int(row.DigestHour),
	}
	_ = json.Unmarshal(row.EventChannels, &p.EventChannels)
	if p.EventChannels == nil {
		p.EventChannels = map[string]map[string]bool{}
	}
	return p
}

// loadPreferences returns stored preferences or the defaults.
func (s *Service) loadPreferences(ctx context.Context, userID int64) (model.Preferences, error) {
	row, err := s.q.GetNotificationPreferences(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultPreferences(), nil
	}
	if err != nil {
		return model.Preferences{}, err
	}
	return preferencesFromRow(row), nil
}

// UpdatePreferences merges an update into the stored preferences.
func (s *Service) UpdatePreferences(ctx context.Context, userID int64, in model.PreferencesUpdate) (model.Preferences, error) {
	p, err := s.loadPreferences(ctx, userID)
	if err != nil {
		return model.Preferences{}, err
	}
	setBool := func(dst *bool, v *bool) {
		if v != nil {
			*dst = *v
		}
	}
	setBool(&p.EmailEnabled, in.EmailEnabled)
	setBool(&p.InappEnabled, in.InappEnabled)
	setBool(&p.RealtimeEnabled, in.RealtimeEnabled)
	setBool(&p.PushEnabled, in.PushEnabled)
	for key, channels := range in.EventChannels {
		et, ok := eventType(key)
		if !ok {
			return model.Preferences{}, fmt.Errorf("%w: unknown event type %q", ErrInvalidRequest, key)
		}
		merged := p.EventChannels[key]
		if merged == nil {
			merged = map[string]bool{}
		}
		for ch, on := range channels {
			if ch != model.PrefChannelInapp && ch != model.PrefChannelEmail && ch != model.PrefChannelPush {
				return model.Preferences{}, fmt.Errorf("%w: unknown channel %q", ErrInvalidRequest, ch)
			}
			if !on && slices.Contains(et.Locked, ch) {
				return model.Preferences{}, fmt.Errorf("%w: %s cannot be turned off for %s", ErrInvalidRequest, ch, key)
			}
			merged[ch] = on
		}
		p.EventChannels[key] = merged
	}
	if in.QuietHours != nil {
		q := *in.QuietHours
		if q.Timezone == "" {
			q.Timezone = DefaultTimezone
		}
		if _, err := time.LoadLocation(q.Timezone); err != nil {
			return model.Preferences{}, fmt.Errorf("%w: unknown timezone %q", ErrInvalidRequest, q.Timezone)
		}
		if _, err := clockMinutes(q.Start); err != nil {
			return model.Preferences{}, fmt.Errorf("%w: quiet_hours.start must be HH:MM", ErrInvalidRequest)
		}
		if _, err := clockMinutes(q.End); err != nil {
			return model.Preferences{}, fmt.Errorf("%w: quiet_hours.end must be HH:MM", ErrInvalidRequest)
		}
		if q.Start == q.End {
			return model.Preferences{}, fmt.Errorf("%w: quiet hours need different start and end", ErrInvalidRequest)
		}
		p.QuietHours = q
	}
	if in.EmailDigest != nil {
		switch *in.EmailDigest {
		case model.DigestOff, model.DigestHourly, model.DigestDaily:
			p.EmailDigest = *in.EmailDigest
		default:
			return model.Preferences{}, fmt.Errorf("%w: email_digest must be off, hourly or daily", ErrInvalidRequest)
		}
	}
	if in.DigestHour != nil {
		if *in.DigestHour < 0 || *in.DigestHour > 23 {
			return model.Preferences{}, fmt.Errorf("%w: digest_hour must be 0-23", ErrInvalidRequest)
		}
		p.DigestHour = *in.DigestHour
	}
	matrix, err := json.Marshal(p.EventChannels)
	if err != nil {
		return model.Preferences{}, err
	}
	row, err := s.q.UpsertNotificationPreferences(ctx, db.UpsertNotificationPreferencesParams{
		UserID: userID, EmailEnabled: p.EmailEnabled, InappEnabled: p.InappEnabled,
		RealtimeEnabled: p.RealtimeEnabled, PushEnabled: p.PushEnabled,
		EventChannels: matrix, QuietHoursEnabled: p.QuietHours.Enabled,
		QuietHoursStart: p.QuietHours.Start, QuietHoursEnd: p.QuietHours.End, Timezone: p.QuietHours.Timezone,
		EmailDigest: p.EmailDigest, DigestHour: safeconv.ToInt16(p.DigestHour),
	})
	if err != nil {
		return model.Preferences{}, err
	}
	return preferencesFromRow(row), nil
}

// EventChannelOn resolves one cell of the matrix (stored choice, else
// default; locked channels are always on).
func EventChannelOn(p model.Preferences, eventKey, channel string) bool {
	et, ok := eventType(eventKey)
	if !ok {
		return true
	}
	if slices.Contains(et.Locked, channel) {
		return true
	}
	if v, ok := p.EventChannels[eventKey][channel]; ok {
		return v
	}
	return et.Defaults[channel]
}

func clockMinutes(hhmm string) (int, error) {
	h, m, ok := strings.Cut(hhmm, ":")
	if !ok || len(h) != 2 || len(m) != 2 {
		return 0, errors.New("bad clock")
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, errors.New("bad clock")
	}
	return hh*60 + mm, nil
}

// QuietUntil reports whether now falls in the user's quiet hours and, if so,
// when they end.
func QuietUntil(q model.QuietHours, now time.Time) (time.Time, bool) {
	if !q.Enabled {
		return time.Time{}, false
	}
	loc, err := time.LoadLocation(q.Timezone)
	if err != nil {
		loc, _ = time.LoadLocation(DefaultTimezone)
	}
	start, err1 := clockMinutes(q.Start)
	end, err2 := clockMinutes(q.End)
	if err1 != nil || err2 != nil || start == end {
		return time.Time{}, false
	}
	local := now.In(loc)
	cur := local.Hour()*60 + local.Minute()
	inside := false
	if start < end {
		inside = cur >= start && cur < end
	} else {
		inside = cur >= start || cur < end
	}
	if !inside {
		return time.Time{}, false
	}
	endToday := time.Date(local.Year(), local.Month(), local.Day(), end/60, end%60, 0, 0, loc)
	if !endToday.After(local) {
		endToday = endToday.AddDate(0, 0, 1)
	}
	return endToday.UTC(), true
}

// digestDue reports whether a user's held e-mails should be sent now.
func digestDue(p model.Preferences, last *time.Time, now time.Time) bool {
	switch p.EmailDigest {
	case model.DigestHourly:
		return last == nil || now.Sub(*last) >= time.Hour
	case model.DigestDaily:
		loc, err := time.LoadLocation(p.QuietHours.Timezone)
		if err != nil {
			loc, _ = time.LoadLocation(DefaultTimezone)
		}
		local := now.In(loc)
		if local.Hour() < p.DigestHour {
			return false
		}
		slot := time.Date(local.Year(), local.Month(), local.Day(), p.DigestHour, 0, 0, 0, loc)
		return last == nil || last.Before(slot)
	}
	return false
}

// ProcessDigests sends one digest e-mail per user whose hourly/daily digest
// is due. Run by the scheduler every few minutes.
func (s *Service) ProcessDigests(ctx context.Context) (int, error) {
	q, ok := s.q.(digestStore)
	if !ok {
		return 0, nil
	}
	candidates, err := q.ListDigestCandidates(ctx)
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	sent := 0
	for _, row := range candidates {
		p := preferencesFromRow(row)
		var last *time.Time
		if row.LastDigestAt.Valid {
			t := row.LastDigestAt.Time
			last = &t
		}
		if !digestDue(p, last, now) {
			continue
		}
		if err := s.sendDigest(ctx, q, row.UserID); err != nil {
			s.log.Error("notification_digest_failed", "user_id", row.UserID, "error", err)
			continue
		}
		sent++
	}
	return sent, nil
}

type digestStore interface {
	ListDigestCandidates(ctx context.Context) ([]db.NotificationPreference, error)
	ListPendingDigestItems(ctx context.Context, userID int64) ([]db.NotificationDigestItem, error)
	MarkDigestItemsSent(ctx context.Context, arg db.MarkDigestItemsSentParams) error
	SetNotificationDigestSent(ctx context.Context, userID int64) error
	InsertNotificationDigestItem(ctx context.Context, arg db.InsertNotificationDigestItemParams) error
}

func (s *Service) sendDigest(ctx context.Context, q digestStore, userID int64) error {
	items, err := q.ListPendingDigestItems(ctx, userID)
	if err != nil || len(items) == 0 {
		return err
	}
	var b strings.Builder
	ids := make([]int64, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
		fmt.Fprintf(&b, "- %s", it.Title)
		if it.Body != "" {
			fmt.Fprintf(&b, ": %s", truncate(strings.ReplaceAll(it.Body, "\n", " "), 160))
		}
		if it.ActionUrl.Valid && it.ActionUrl.String != "" {
			fmt.Fprintf(&b, "\n  %s", it.ActionUrl.String)
		}
		b.WriteString("\n")
	}
	name := ""
	if u, err := s.q.GetUserByID(ctx, userID); err == nil {
		name = u.Name
	}
	uid := userID
	if _, err := s.Enqueue(ctx, model.EnqueueInput{
		UserID:       &uid,
		Channels:     []string{model.ChannelEmail},
		TemplateCode: "notifications.digest",
		TemplateVars: map[string]string{"name": name, "count": strconv.Itoa(len(items)), "items": b.String()},
		SourceEvent:  "notifications.digest",
		Title:        "ClickFix",
	}); err != nil {
		return err
	}
	if err := q.MarkDigestItemsSent(ctx, db.MarkDigestItemsSentParams{UserID: userID, Ids: ids}); err != nil {
		return err
	}
	return q.SetNotificationDigestSent(ctx, userID)
}

// channelPlan is what Enqueue does for one channel of a catalog event.
type channelPlan struct {
	skip        bool
	digest      bool
	scheduledAt *time.Time
	noPush      bool
	asPush      bool // in-app is off but push is on: send a push-only row
}

// planChannel applies the matrix, quiet hours and digest to one channel.
// Sources outside the catalog have no per-event switches or digest but
// honor quiet hours the same way (CF-157): no push, e-mail held until the
// quiet hours end; critical notifications are never held.
func planChannel(p model.Preferences, eventKey, channel, priority string, now time.Time) channelPlan {
	critical := priority == model.PriorityCritical
	quietEnd, quiet := QuietUntil(p.QuietHours, now)
	if _, ok := eventType(eventKey); !ok {
		if !quiet || critical {
			return channelPlan{}
		}
		switch channel {
		case model.ChannelEmail:
			return channelPlan{scheduledAt: &quietEnd}
		case model.ChannelPush:
			return channelPlan{skip: true}
		case model.ChannelInapp:
			return channelPlan{noPush: true}
		}
		return channelPlan{}
	}
	switch channel {
	case model.ChannelEmail:
		if !EventChannelOn(p, eventKey, model.PrefChannelEmail) {
			return channelPlan{skip: true}
		}
		if !critical && p.EmailDigest != model.DigestOff {
			return channelPlan{digest: true}
		}
		if quiet && !critical {
			return channelPlan{scheduledAt: &quietEnd}
		}
	case model.ChannelInapp, model.ChannelPush:
		pushOn := EventChannelOn(p, eventKey, model.PrefChannelPush) && (!quiet || critical)
		if channel == model.ChannelPush {
			return channelPlan{skip: !pushOn}
		}
		if !EventChannelOn(p, eventKey, model.PrefChannelInapp) {
			return channelPlan{skip: !pushOn, asPush: pushOn}
		}
		return channelPlan{noPush: !pushOn}
	}
	return channelPlan{}
}

// NotifyInput fans one domain event out to users through the matrix.
type NotifyInput struct {
	Event          string
	OrganizationID *int64
	UserIDs        []int64
	Vars           map[string]string
	ActionURL      string
	Payload        map[string]any
	// Channels default to in-app (+ push mirror) and e-mail; the matrix
	// decides which of them each user gets.
	Channels []string
	// DedupeKey (usually the domain event id) makes a redelivered event a
	// no-op per user and channel.
	DedupeKey string
}

// NotifyEvent enqueues the in-app (+ push mirror) and e-mail notifications
// of a catalog event for each user, honoring each user's preferences.
func (s *Service) NotifyEvent(ctx context.Context, in NotifyInput) error {
	et, ok := eventType(in.Event)
	if !ok {
		return fmt.Errorf("%w: %s is not a notification event", ErrInvalidRequest, in.Event)
	}
	seen := map[int64]bool{}
	for _, uid := range in.UserIDs {
		if uid <= 0 || seen[uid] {
			continue
		}
		seen[uid] = true
		vars := map[string]string{"action_url": in.ActionURL, "url": in.ActionURL}
		for k, v := range in.Vars {
			vars[k] = v
		}
		if u, err := s.q.GetUserByID(ctx, uid); err == nil {
			vars["name"] = u.Name
		}
		payload := map[string]any{"kind": in.Event}
		for k, v := range in.Payload {
			payload[k] = v
		}
		var action *string
		if in.ActionURL != "" {
			a := in.ActionURL
			action = &a
		}
		id := uid
		channels := in.Channels
		if len(channels) == 0 {
			channels = []string{model.ChannelInapp, model.ChannelEmail}
		}
		dedupe := ""
		if in.DedupeKey != "" {
			dedupe = in.DedupeKey + ":" + strconv.FormatInt(uid, 10)
		}
		if _, err := s.Enqueue(ctx, model.EnqueueInput{
			TenantID: in.OrganizationID, UserID: &id, Channels: channels, Priority: et.Priority,
			TemplateCode: in.Event, TemplateVars: vars, SourceEvent: in.Event, Payload: payload, ActionURL: action,
			ContentVars: vars, DedupeKey: dedupe,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) holdForDigest(ctx context.Context, userID int64, org *int64, event, title, body string, action *string) error {
	q, ok := s.q.(digestStore)
	if !ok {
		return nil
	}
	return q.InsertNotificationDigestItem(ctx, db.InsertNotificationDigestItemParams{
		UserID: userID, OrganizationID: int8Ptr(org), EventType: event, Title: title, Body: body,
		ActionUrl: textPtr(action),
	})
}

func pgTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
