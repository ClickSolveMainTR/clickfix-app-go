package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/videorooms"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// GuestVideo contains only credentials intended for this booking's invitee.
type GuestVideo struct {
	GuestLink   string `json:"guest_link"`
	PIN         string `json:"pin"`
	HostJoined  bool   `json:"host_joined"`
	GuestJoined bool   `json:"guest_joined"`
}

// Video adds the signed member link to the invitee projection. Member links
// still require an authenticated organization member at the video endpoint.
type Video struct {
	GuestVideo
	HostLink string `json:"host_link"`
}

func guestVideo(v *Video) *GuestVideo {
	if v == nil {
		return nil
	}
	copy := v.GuestVideo
	return &copy
}

func fallbackVideo(locale string) string {
	if locale == "en" {
		return "Video room unavailable. Your appointment is booked; the host will contact you with connection details."
	}
	return "Görüntülü oda kullanılamıyor. Randevunuz oluşturuldu; temsilciniz bağlantı bilgileri için sizinle iletişime geçecek."
}

func videoUnavailable(err error) bool {
	return errors.Is(err, videorooms.ErrDisabled) || errors.Is(err, videorooms.ErrNotConfigured) ||
		errors.Is(err, videorooms.ErrNoMinutes) || errors.Is(err, videorooms.ErrDenied)
}

// preloadVideo resolves plan and balance before the booking transaction
// (see videorooms.Service.Preload). Nil when the type has no video room.
func (s *Service) preloadVideo(ctx context.Context, org int64, t db.AppointmentType) (*videorooms.Service, error) {
	if t.LocationKind != LocationVideo || s.video == nil {
		return nil, nil
	}
	return s.video.Preload(ctx, org)
}

// syncVideo runs under the appointment host locks, using the same
// transaction. video comes from preloadVideo.
func (s *Service) syncVideo(ctx context.Context, q *db.Queries, video *videorooms.Service, a db.Appointment, host uuid.UUID) error {
	if a.LocationKind != LocationVideo {
		return nil
	}
	if video == nil {
		return s.videoFallback(ctx, q, a, videorooms.ErrNotConfigured)
	}
	d, err := video.WithQueries(q).RoomForAppointment(ctx, a.OrganizationID, a.BookedByUserID.Int64, a.Uuid, videorooms.AppointmentRoomInput{
		Name: a.Title, StartsAt: a.StartsAt.Time, EndsAt: a.EndsAt.Time, HostUUID: &host,
	})
	if err != nil {
		// Existing rooms must move atomically with their appointment; an error
		// never leaves a successful reschedule pointing at the old window.
		if !a.VideoRoomUuid.Valid && videoUnavailable(err) {
			return s.videoFallback(ctx, q, a, err)
		}
		if errors.Is(err, videorooms.ErrConflict) || errors.Is(err, videorooms.ErrDenied) {
			return invalidf("video room cannot be moved under its current state or limits")
		}
		return err
	}
	location := a.LocationValue
	if !a.VideoRoomUuid.Valid {
		st, err := q.AppointmentSettings(ctx, a.OrganizationID)
		if err != nil {
			return err
		}
		if location == fallbackVideo(st.OrganizationLocale) {
			location = ""
		}
	}
	return q.AppointmentAttachVideo(ctx, db.AppointmentAttachVideoParams{
		OrganizationID: a.OrganizationID, ID: a.ID, VideoRoomUuid: pgtype.UUID{Bytes: d.UUID, Valid: true}, LocationValue: location,
	})
}

func (s *Service) videoFallback(ctx context.Context, q *db.Queries, a db.Appointment, cause error) error {
	st, err := q.AppointmentSettings(ctx, a.OrganizationID)
	if err != nil {
		return err
	}
	warning := fallbackVideo(st.OrganizationLocale)
	if err := q.AppointmentAttachVideo(ctx, db.AppointmentAttachVideoParams{
		OrganizationID: a.OrganizationID, ID: a.ID, LocationValue: warning,
	}); err != nil {
		return err
	}
	s.warn("appointment_video_unavailable", "organization_id", a.OrganizationID, "appointment_uuid", a.Uuid, "reason", cause)
	return nil
}

func (s *Service) videoView(ctx context.Context, org int64, a *Appointment) {
	if a.LocationKind != LocationVideo {
		return
	}
	if a.VideoRoomUUID == nil {
		a.VideoWarning = a.LocationValue
		return
	}
	if s.video == nil {
		return
	}
	d, err := s.video.Room(ctx, org, *a.VideoRoomUUID)
	if err != nil {
		s.warn("appointment_video_load_failed", "error", err)
		return
	}
	a.Video = &Video{GuestVideo: GuestVideo{GuestLink: d.GuestLink, PIN: d.PIN}}
	attendance, err := s.q.VideoAttendance(ctx, db.VideoAttendanceParams{OrganizationID: org, Uuid: d.UUID})
	if err == nil {
		a.Video.HostJoined = attendance.HostJoined
		a.Video.GuestJoined = attendance.GuestJoined
	}
	if active(a.Status) && d.Status != "ended" {
		link, err := s.video.Link(ctx, org, d.UUID, videorooms.LinkInput{Role: videorooms.LinkAgent})
		if err == nil {
			a.Video.HostLink = link.URL
		}
	}
}

// videoText is safe for public calendars and messages: never includes host access.
func videoText(a Appointment) string {
	if a.VideoWarning != "" {
		return a.VideoWarning
	}
	if a.Video == nil || a.Video.GuestLink == "" {
		return ""
	}
	text := a.Video.GuestLink
	if a.Video.PIN != "" {
		text += "\nPIN: " + a.Video.PIN
	}
	return strings.TrimSpace(text)
}

// SweepVideoOutcomes uses verified LiveKit session evidence, not lobby admissions.
// Cancellation is excluded and an absent host never marks the invitee as completed.
func (s *Service) SweepVideoOutcomes(ctx context.Context) error {
	rows, err := s.q.AppointmentVideoDue(ctx, ts(s.now()))
	if err != nil {
		return err
	}
	for _, a := range rows {
		attendance, err := s.q.VideoAttendance(ctx, db.VideoAttendanceParams{
			OrganizationID: a.OrganizationID, Uuid: uuid.UUID(a.VideoRoomUuid.Bytes),
		})
		if err != nil {
			return err
		}
		status := StatusNoShow
		if attendance.HostJoined && attendance.GuestJoined {
			status = StatusCompleted
		}
		n, err := s.q.AppointmentVideoFinish(ctx, db.AppointmentVideoFinishParams{OrganizationID: a.OrganizationID, ID: a.ID, Status: status, ExpectedSequence: a.Sequence})
		if err != nil {
			return err
		}
		if n > 0 {
			s.afterChange(ctx, a.OrganizationID, a.ID, change{event: status, by: "video"})
		}
	}
	return nil
}

// recordVideoFallback runs only after commit so rolled-back bookings leave no audit event.
func (s *Service) recordVideoFallback(ctx context.Context, a db.Appointment) {
	if a.LocationKind != LocationVideo || a.VideoRoomUuid.Valid {
		return
	}
	s.activity.Record(orgctx.WithOrganizationID(ctx, a.OrganizationID), nil,
		"appointments.updated", "appointment", &a.Uuid, map[string]any{"event": "appointment.video_unavailable", "reason": "video_unavailable"}, nil)
}
