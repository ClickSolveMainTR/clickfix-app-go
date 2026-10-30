package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	notif "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/i18n"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// EventChanged is published on org:{uuid} after an appointment or type
// changes; open calendars refetch.
const EventChanged = "appointment.changed"

// live publishes a change (nil-safe, fail-soft).
func (s *Service) live(ctx context.Context, org int64, kind string, id uuid.UUID) {
	if s.pub == nil {
		return
	}
	st, err := s.q.AppointmentSettings(ctx, org)
	if err != nil {
		return
	}
	msg := realtime.Message{Type: EventChanged, Data: map[string]any{"kind": kind, "uuid": id.String()}}
	if err := s.pub.Publish(ctx, realtime.OrgChannel(st.OrganizationUuid), msg); err != nil {
		s.warn("appointments_publish_failed", "error", err)
	}
}

// change describes what happened to a booking.
type change struct {
	event         string // booked, rescheduled, confirmed, completed, no_show, cancelled
	actorID       int64
	by            string // host / invitee
	previous      *time.Time
	notifyInvitee bool
}

// afterChange runs the side effects of a committed change: realtime,
// conversation and company timelines, notifications. All fail-soft: the
// booking is already stored.
func (s *Service) afterChange(ctx context.Context, org, id int64, c change) {
	a, err := s.q.AppointmentByID(ctx, db.AppointmentByIDParams{OrganizationID: org, ID: id})
	if err != nil {
		s.warn("appointments_after_change_load_failed", "error", err)
		return
	}
	if c.event == "booked" {
		s.recordVideoFallback(ctx, a)
	}
	s.live(ctx, org, "appointment", a.Uuid)
	view, err := s.Get(ctx, org, a.Uuid)
	if err != nil {
		return
	}
	st, err := s.settings(ctx, org)
	if err != nil {
		return
	}
	attrs := map[string]any{
		"event": "appointment_" + c.event, "appointment_uuid": a.Uuid.String(), "title": a.Title,
		"type": view.Type.Name, "starts_at": a.StartsAt.Time.UTC().Format(time.RFC3339),
		"to": formatWhen(a.StartsAt.Time, st.loc, st.OrganizationLocale), "status": a.Status,
	}
	if view.Host != nil {
		attrs["host"] = view.Host.Name
	}
	if c.previous != nil {
		attrs["from"] = formatWhen(*c.previous, st.loc, st.OrganizationLocale)
	}
	if a.ConversationID.Valid && s.timeline != nil {
		if err := s.timeline.AppointmentEvent(ctx, org, a.ConversationID.Int64, convusecase.Actor{ID: c.actorID}, attrs); err != nil {
			s.warn("appointments_timeline_failed", "error", err)
		}
	}
	if a.CompanyID.Valid {
		props, _ := json.Marshal(map[string]any{
			"type": view.Type.Name, "starts_at": attrs["starts_at"], "status": a.Status, "host": attrs["host"],
			"appointment_uuid": a.Uuid.String(),
		})
		if _, err := s.q.CreateCompanyEvent(ctx, db.CreateCompanyEventParams{
			OrganizationID: org, CompanyID: a.CompanyID.Int64, ContactID: a.ContactID, Name: "appointment." + c.event,
			Properties: props, Source: "appointments", OccurredAt: ts(s.now()),
			IdempotencyKey: pgtype.Text{String: fmt.Sprintf("appointment:%s:%s:%d", a.Uuid, c.event, a.Sequence), Valid: true},
		}); err != nil {
			s.warn("appointments_company_event_failed", "error", err)
		}
	}
	s.notifyChange(ctx, org, st, a, view, c)
}

func (s *Service) notifyChange(ctx context.Context, org int64, st orgSettings, a db.Appointment, view Appointment, c change) {
	if s.notifier == nil {
		return
	}
	t, err := s.q.AppointmentTypeByID(ctx, db.AppointmentTypeByIDParams{OrganizationID: org, ID: a.TypeID})
	if err != nil {
		return
	}
	lang := st.OrganizationLocale
	vars := templateVars(view, st, t)
	// Invitee e-mail.
	if c.notifyInvitee && a.InviteeEmail != "" {
		subject, body := inviteeMessage(lang, c.event, t, vars)
		recipient := a.InviteeEmail
		if _, err := s.notifier.Enqueue(ctx, notif.EnqueueInput{
			TenantID: &org, Channels: []string{notif.ChannelEmail}, Recipient: &recipient, Title: subject, Body: body,
			SourceEvent: "appointment." + c.event, Language: lang,
			Payload: map[string]any{"appointment_uuid": a.Uuid.String()},
		}); err != nil {
			s.warn("appointments_invitee_notify_failed", "error", err)
		}
	}
	// Host in-app (push mirror) and e-mail per the host's preferences,
	// unless the host did it.
	if a.HostUserID.Valid && a.HostUserID.Int64 != c.actorID {
		s.notifyHost(ctx, org, st, a, view, c.event, c.previous, fmt.Sprintf("%s:%d", c.event, a.Sequence))
	}
}

// notifyHost tells the host about a booking (CF-157: enriched text in the
// host's language and the organization's time zone, catalog preferences).
// key makes a repeated change a no-op.
func (s *Service) notifyHost(ctx context.Context, org int64, st orgSettings, a db.Appointment, view Appointment, event string, previous *time.Time, key string) {
	host := a.HostUserID.Int64
	lang := st.OrganizationLocale
	action := "/calendar?appointment=" + a.Uuid.String()
	vars := map[string]string{
		"customer": view.Invitee.Name, "type": view.Type.Name, "timezone": st.loc.String(), "url": action,
		"start": view.StartsAt.UTC().Format(time.RFC3339), "end": view.EndsAt.UTC().Format(time.RFC3339),
		"location": view.LocationValue,
	}
	if vars["location"] == "" {
		vars["location"] = locationLabel(lang, view.LocationKind)
	}
	if view.LocationKind == LocationVideo {
		// Members open the room in the app; the guest link is the invitee's.
		vars["location"] = locationLabel(lang, LocationVideo)
		if view.VideoRoomUUID != nil {
			vars["video_url"] = "/video/" + view.VideoRoomUUID.String()
		}
	}
	if previous != nil {
		vars["previous"] = previous.UTC().Format(time.RFC3339)
	}
	if _, err := s.notifier.Enqueue(ctx, notif.EnqueueInput{
		TenantID: &org, UserID: &host, Channels: []string{notif.ChannelInapp, notif.ChannelEmail},
		Title: hostTitle(lang, event, view), Body: formatWhen(view.StartsAt, st.loc, lang) + " · " + view.Type.Name, ActionURL: &action,
		// No Language: the host reads it in their own language.
		SourceEvent: "appointment." + event, ContentVars: vars,
		DedupeKey: "appointment:" + a.Uuid.String() + ":" + key,
		Payload:   map[string]any{"appointment_uuid": a.Uuid.String()},
	}); err != nil {
		s.warn("appointments_host_notify_failed", "error", err)
	}
}

// Placeholders a template may use: {{name}}, {{type}}, {{start}}, {{end}},
// {{timezone}}, {{location}}, {{host}}, {{organization}}, {{manage_url}},
// {{ics_url}}, {{google_url}}.
var Placeholders = []string{"name", "type", "start", "end", "timezone", "location", "host", "organization", "manage_url", "ics_url", "google_url"}

func templateVars(a Appointment, st orgSettings, t db.AppointmentType) map[string]string {
	loc := st.loc
	if l, err := time.LoadLocation(a.Timezone); err == nil {
		loc = l
	}
	lang := st.OrganizationLocale
	location := a.LocationValue
	if location == "" {
		location = locationLabel(lang, a.LocationKind)
	}
	return map[string]string{
		"name": a.Invitee.Name, "type": a.Type.Name, "start": formatWhen(a.StartsAt, loc, lang),
		"end": formatClock(a.EndsAt.In(loc), lang), "timezone": loc.String(), "location": location,
		"host": hostName(a), "organization": st.OrganizationName, "manage_url": a.Links.Manage,
		"video": videoText(a), "ics_url": a.Links.ICS, "google_url": a.Links.Google,
	}
}

// Render fills {{placeholders}}; unknown ones stay as written.
func Render(tpl string, vars map[string]string) string {
	out := tpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
		out = strings.ReplaceAll(out, "{{ "+k+" }}", v)
	}
	return out
}

// appLocale maps the organization language to a catalog locale (anything
// but "tr" reads English here).
func appLocale(lang string) i18n.Locale {
	if lang == "tr" {
		return i18n.LocaleTR
	}
	return i18n.LocaleEN
}

func formatWhen(t time.Time, loc *time.Location, lang string) string {
	return i18n.FormatDateTime(t, loc, appLocale(lang))
}

// formatClock is the time of day in the organization's language: 14:30 / 2:30 PM.
func formatClock(t time.Time, lang string) string {
	return i18n.FormatClock(t, t.Location(), appLocale(lang))
}

func locationLabel(lang, kind string) string {
	tr := map[string]string{LocationVideo: "Görüntülü görüşme", LocationPhone: "Telefon görüşmesi", LocationInPerson: "Yüz yüze", LocationCustomURL: "Çevrim içi"}
	en := map[string]string{LocationVideo: "Video call", LocationPhone: "Phone call", LocationInPerson: "In person", LocationCustomURL: "Online"}
	if lang == "tr" {
		return tr[kind]
	}
	return en[kind]
}

func inviteeMessage(lang, event string, t db.AppointmentType, v map[string]string) (string, string) {
	tr := lang == "tr"
	var subject, intro string
	switch event {
	case "booked":
		if tr {
			subject, intro = "Randevunuz oluşturuldu: "+v["type"], "Merhaba {{name}},\n\n{{organization}} ile {{type}} randevunuz {{start}} ({{timezone}}) için oluşturuldu."
		} else {
			subject, intro = "Your appointment is booked: "+v["type"], "Hi {{name}},\n\nYour {{type}} with {{organization}} is booked for {{start}} ({{timezone}})."
		}
		if t.ConfirmationTemplate != "" {
			intro = t.ConfirmationTemplate
		}
	case "rescheduled":
		if tr {
			subject, intro = "Randevunuz güncellendi: "+v["type"], "Merhaba {{name}},\n\n{{type}} randevunuzun yeni zamanı: {{start}} ({{timezone}})."
		} else {
			subject, intro = "Your appointment was rescheduled: "+v["type"], "Hi {{name}},\n\nYour {{type}} now starts {{start}} ({{timezone}})."
		}
	case "confirmed":
		if tr {
			subject, intro = "Randevunuz onaylandı: "+v["type"], "Merhaba {{name}},\n\n{{start}} ({{timezone}}) tarihli {{type}} randevunuz onaylandı."
		} else {
			subject, intro = "Your appointment is confirmed: "+v["type"], "Hi {{name}},\n\nYour {{type}} on {{start}} ({{timezone}}) is confirmed."
		}
	case "cancelled":
		if tr {
			subject, intro = "Randevunuz iptal edildi: "+v["type"], "Merhaba {{name}},\n\n{{start}} ({{timezone}}) tarihli {{type}} randevunuz iptal edildi."
		} else {
			subject, intro = "Your appointment was canceled: "+v["type"], "Hi {{name}},\n\nYour {{type}} on {{start}} ({{timezone}}) was canceled."
		}
		return subject, Render(intro, v)
	case "reminder":
		if tr {
			subject, intro = "Hatırlatma: "+v["type"]+" · "+v["start"], "Merhaba {{name}},\n\n{{organization}} ile {{type}} randevunuzu hatırlatırız: {{start}} ({{timezone}})."
		} else {
			subject, intro = "Reminder: "+v["type"]+" · "+v["start"], "Hi {{name}},\n\nThis is a reminder of your {{type}} with {{organization}} on {{start}} ({{timezone}})."
		}
		if t.ReminderTemplate != "" {
			intro = t.ReminderTemplate
		}
	default:
		return "", ""
	}
	var details string
	if tr {
		details = "\n\nYer: {{location}}\nRandevuyu yeniden planlamak veya iptal etmek için: {{manage_url}}\nTakvime ekle (ICS): {{ics_url}}\nGoogle Takvim: {{google_url}}"
	} else {
		details = "\n\nWhere: {{location}}\nTo reschedule or cancel: {{manage_url}}\nAdd to calendar (ICS): {{ics_url}}\nGoogle Calendar: {{google_url}}"
	}
	if v["video"] != "" {
		details += "\n\n{{video}}"
	}
	return subject, Render(intro+details, v)
}

func hostTitle(lang, event string, a Appointment) string {
	who := a.Invitee.Name
	if lang == "tr" {
		switch event {
		case "booked":
			return "Yeni randevu: " + who
		case "rescheduled":
			return "Randevu yeniden planlandı: " + who
		case "cancelled":
			return "Randevu iptal edildi: " + who
		case "reminder":
			return "Yaklaşan randevu: " + who
		}
		return "Randevu güncellendi: " + who
	}
	switch event {
	case "booked":
		return "New appointment: " + who
	case "rescheduled":
		return "Appointment rescheduled: " + who
	case "cancelled":
		return "Appointment canceled: " + who
	case "reminder":
		return "Upcoming appointment: " + who
	}
	return "Appointment updated: " + who
}

// SweepReminders sends due reminders (every minute, platform-wide). Each
// (booking, offset, version) is claimed before sending, so a reminder goes
// out at most once; a reschedule bumps the version and re-arms them.
func (s *Service) SweepReminders(ctx context.Context) (int, error) {
	if err := s.SweepVideoOutcomes(ctx); err != nil {
		return 0, err
	}
	rows, err := s.q.AppointmentDueReminders(ctx, ts(s.now()))
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, r := range rows {
		n, err := s.q.AppointmentMarkReminder(ctx, db.AppointmentMarkReminderParams{
			OrganizationID: r.OrganizationID, AppointmentID: r.ID, OffsetMinutes: r.OffsetMinutes, TokenVersion: r.TokenVersion,
		})
		if err != nil {
			return sent, err
		}
		if n == 0 {
			continue
		}
		s.remind(ctx, r.OrganizationID, r.ID, r.OffsetMinutes)
		sent++
	}
	return sent, nil
}

func (s *Service) remind(ctx context.Context, org, id int64, offset int32) {
	if s.notifier == nil {
		return
	}
	a, err := s.q.AppointmentByID(ctx, db.AppointmentByIDParams{OrganizationID: org, ID: id})
	if err != nil {
		return
	}
	view, err := s.Get(ctx, org, a.Uuid)
	if err != nil {
		return
	}
	st, err := s.settings(ctx, org)
	if err != nil {
		return
	}
	t, err := s.q.AppointmentTypeByID(ctx, db.AppointmentTypeByIDParams{OrganizationID: org, ID: a.TypeID})
	if err != nil {
		return
	}
	lang := st.OrganizationLocale
	vars := templateVars(view, st, t)
	if a.InviteeEmail != "" {
		subject, body := inviteeMessage(lang, "reminder", t, vars)
		recipient := a.InviteeEmail
		if _, err := s.notifier.Enqueue(ctx, notif.EnqueueInput{
			TenantID: &org, Channels: []string{notif.ChannelEmail}, Recipient: &recipient, Title: subject, Body: body,
			SourceEvent: "appointment.reminder", Language: lang, Payload: map[string]any{"appointment_uuid": a.Uuid.String()},
		}); err != nil {
			s.warn("appointments_reminder_failed", "error", err)
		}
	}
	// SMS when the organization has an active SMS channel and a phone.
	if a.InviteePhone != "" {
		if on, err := s.q.AppointmentActiveSMSChannel(ctx, org); err == nil && on {
			phone := a.InviteePhone
			if _, err := s.notifier.Enqueue(ctx, notif.EnqueueInput{
				TenantID: &org, Channels: []string{notif.ChannelSMS}, Recipient: &phone,
				Title: vars["type"], Body: Render(smsText(lang), vars), SourceEvent: "appointment.reminder", Language: lang,
				Payload: map[string]any{"appointment_uuid": a.Uuid.String()},
			}); err != nil {
				s.warn("appointments_sms_reminder_failed", "error", err)
			}
		}
	}
	if a.HostUserID.Valid {
		s.notifyHost(ctx, org, st, a, view, "reminder", nil, fmt.Sprintf("reminder:%d:%d", offset, a.Sequence))
	}
}

func smsText(lang string) string {
	if lang == "tr" {
		return "{{organization}}: {{type}} randevunuz {{start}} tarihinde. Yönetmek için: {{manage_url}}"
	}
	return "{{organization}}: your {{type}} is on {{start}}. Manage: {{manage_url}}"
}
