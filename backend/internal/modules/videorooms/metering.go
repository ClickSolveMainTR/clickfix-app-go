package videorooms

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	notif "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/livekit"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Billing model: participant-minutes. Every LiveKit connection (one
// participant joining once) is rounded up to whole minutes when it closes and
// debited from the video_minutes wallet (plan allowance lots first, then
// purchased packages, FIFO by expiry). Hidden egress recorders are not
// participants and are never billed. A plan whose video.minutes entitlement
// is unlimited is not metered.

// Webhook handles a verified LiveKit webhook body.
func (s *Service) Webhook(ctx context.Context, r *http.Request) error {
	body, err := s.LiveKit.VerifyWebhook(r)
	if err != nil {
		return err
	}
	var ev livekit.Event
	if err = livekit.Decode(body, &ev); err != nil {
		return ErrInvalid
	}
	if ev.ID != "" {
		n, err := s.Q.VideoWebhookSeen(ctx, ev.ID)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
	if err = s.handleEvent(ctx, ev); err != nil && ev.ID != "" {
		// Forget the id so LiveKit's retry is processed, not skipped.
		if ferr := s.Q.VideoForgetWebhookEvent(ctx, ev.ID); ferr != nil {
			return errors.Join(err, ferr)
		}
	}
	return err
}

func (s *Service) handleEvent(ctx context.Context, ev livekit.Event) error {
	at := s.now()
	if ev.CreatedAt > 0 {
		at = time.Unix(int64(ev.CreatedAt), 0).UTC()
	}
	// Remote support rooms (rs_, CF-233) share this webhook endpoint.
	if s.RemoteSupportEvent != nil && remoteSupportEvent(ev) {
		return s.RemoteSupportEvent(ctx, ev)
	}
	switch ev.Event {
	case "egress_started", "egress_updated", "egress_ended":
		if ev.EgressInfo != nil {
			return s.egressEvent(ctx, *ev.EgressInfo)
		}
		return nil
	}
	if ev.Room == nil || ev.Room.Name == "" {
		return nil
	}
	room, err := s.Q.VideoRoomByLivekitName(ctx, ev.Room.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	// room_started fires when the API creates the room (first token), so a
	// room counts as live from the first participant joining.
	switch ev.Event {
	case "room_finished":
		if err = s.closeOpenSessions(ctx, room, at); err != nil {
			return err
		}
		return s.Q.VideoIdleRoom(ctx, db.VideoIdleRoomParams{OrganizationID: room.OrganizationID, ID: room.ID})
	case "participant_joined":
		if ev.Participant == nil || !billable(ev.Participant.Identity) {
			return nil
		}
		joined := at
		if ev.Participant.JoinedAt > 0 {
			joined = time.Unix(int64(ev.Participant.JoinedAt), 0).UTC()
		}
		if err = s.Q.VideoStartRoom(ctx, db.VideoStartRoomParams{OrganizationID: room.OrganizationID, ID: room.ID}); err != nil {
			return err
		}
		return s.Q.VideoOpenSession(ctx, db.VideoOpenSessionParams{OrganizationID: room.OrganizationID, RoomID: room.ID, LivekitSid: ev.Participant.SID,
			Identity: ev.Participant.Identity, JoinedAt: pgtype.Timestamptz{Time: joined, Valid: true}})
	case "participant_left":
		if ev.Participant == nil {
			return nil
		}
		return s.closeSession(ctx, room, ev.Participant.SID, at)
	}
	return nil
}

func remoteSupportEvent(ev livekit.Event) bool {
	if ev.EgressInfo != nil && strings.HasPrefix(ev.EgressInfo.RoomName, RemoteSupportRoomPrefix) {
		return true
	}
	return ev.Room != nil && strings.HasPrefix(ev.Room.Name, RemoteSupportRoomPrefix)
}

// billable: our own participants are "u:" (members) and "g:" (guests).
func billable(identity string) bool {
	return strings.HasPrefix(identity, "u:") || strings.HasPrefix(identity, "g:")
}

func (s *Service) closeOpenSessions(ctx context.Context, room db.VideoRoom, at time.Time) error {
	open, err := s.Q.VideoOpenSessions(ctx, db.VideoOpenSessionsParams{OrganizationID: room.OrganizationID, RoomID: room.ID})
	if err != nil {
		return err
	}
	var errs []error
	for _, sess := range open {
		errs = append(errs, s.closeSession(ctx, room, sess.LivekitSid, at))
	}
	return errors.Join(errs...)
}

func (s *Service) closeSession(ctx context.Context, room db.VideoRoom, sid string, at time.Time) error {
	sess, err := s.Q.VideoCloseSession(ctx, db.VideoCloseSessionParams{OrganizationID: room.OrganizationID, LivekitSid: sid, LeftAt: pgtype.Timestamptz{Time: at, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	return s.bill(ctx, room, sess)
}

func (s *Service) unlimited(ctx context.Context, org int64) (bool, error) {
	if s.Entitlements == nil {
		return false, nil
	}
	snap, err := s.Entitlements.Resolve(ctx, org)
	if err != nil {
		return false, err
	}
	return snap.Values["video.minutes"].Unlimited, nil
}

func (s *Service) bill(ctx context.Context, room db.VideoRoom, sess db.VideoSession) error {
	if sess.Minutes <= 0 || s.Wallet == nil {
		return nil
	}
	unlimited, err := s.unlimited(ctx, room.OrganizationID)
	if err != nil {
		return err
	}
	var billed, balance int64
	if !unlimited {
		billed, balance, err = s.Wallet.MeterUsage(ctx, room.OrganizationID, UnitVideoMinutes, sess.Minutes, "video:"+sess.LivekitSid, "Video call minutes")
		if err != nil {
			return err
		}
	}
	if err = s.Q.VideoSetSessionBilled(ctx, db.VideoSetSessionBilledParams{OrganizationID: room.OrganizationID, ID: sess.ID, BilledMinutes: billed}); err != nil {
		return err
	}
	unbilled := int64(0)
	if !unlimited {
		unbilled = sess.Minutes - billed
	}
	if err = s.Q.VideoAddRoomBilling(ctx, db.VideoAddRoomBillingParams{OrganizationID: room.OrganizationID, ID: room.ID, Billed: billed, Unbilled: unbilled}); err != nil {
		return err
	}
	if unlimited {
		return nil
	}
	return s.checkLowBalance(ctx, room.OrganizationID, balance)
}

// checkLowBalance warns owners and admins once a day while the balance is
// under the organization's threshold.
func (s *Service) checkLowBalance(ctx context.Context, org, balance int64) error {
	a, err := s.limits(ctx, org)
	if err != nil {
		return err
	}
	_, tighten, err := s.orgSettings(ctx, org)
	if err != nil {
		return err
	}
	threshold := a.Tighten(tighten).LowBalanceMinutes
	if balance >= threshold {
		return s.Q.VideoClearLowBalance(ctx, org)
	}
	n, err := s.Q.VideoMarkLowBalanceNotified(ctx, org)
	if err != nil || n == 0 || s.Notifier == nil {
		return err
	}
	recipients, err := s.Q.BillingBillingRecipients(ctx, org)
	if err != nil {
		return err
	}
	link := strings.TrimRight(s.FrontendURL, "/") + "/billing"
	for _, u := range recipients {
		uid := u.ID
		title, body := "Görüntülü görüşme dakikanız azalıyor", fmt.Sprintf("Kalan görüntülü görüşme dakikası: %d. Bakiye bittiğinde yeni oda açılamaz ve süren görüşmeler sonlandırılır.", balance)
		if lang(u.Locale) == "en" {
			title, body = "Your video minutes are running low", fmt.Sprintf("Video minutes left: %d. When the balance runs out no new room can be opened and running calls are ended.", balance)
		}
		if _, err = s.Notifier.Enqueue(ctx, notif.EnqueueInput{TenantID: &org, UserID: &uid, Channels: []string{notif.ChannelInapp, notif.ChannelEmail},
			Title: title, Body: body, ActionURL: &link, Language: lang(u.Locale), SourceEvent: "video.balance_low", Payload: map[string]any{"balance": balance},
			ContentVars: map[string]string{"balance": strconv.FormatInt(int64(balance), 10)}}); err != nil {
			return err
		}
	}
	return nil
}

// Sweep runs every minute: it ends rooms past their maximum duration or link
// lifetime, ends live rooms of organizations whose minutes ran out, and stops
// recordings past the recording limit.
func (s *Service) Sweep(ctx context.Context) error {
	rooms, err := s.Q.VideoLiveRooms(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	exhausted := map[int64]bool{}
	var errs []error
	for _, r := range rooms {
		reason := ""
		switch {
		case r.AppointmentUuid.Valid && r.ScheduledEndAt.Valid && !now.Before(r.ScheduledEndAt.Time):
			reason = "appointment_window"
		case r.Status == "live" && r.StartedAt.Valid && now.Sub(r.StartedAt.Time) > time.Duration(r.MaxDurationMinutes)*time.Minute:
			reason = "max_duration"
		case r.Status != "live" && now.After(r.LinksExpireAt.Time):
			reason = "expired"
		case r.Status == "live":
			out, seen := exhausted[r.OrganizationID]
			if !seen {
				out, err = s.outOfMinutes(ctx, r.OrganizationID)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				exhausted[r.OrganizationID] = out
			}
			if out {
				reason = "no_minutes"
			}
		}
		if reason != "" {
			errs = append(errs, s.end(ctx, r, reason))
			continue
		}
		errs = append(errs, s.limitRecording(ctx, r, now))
	}
	errs = append(errs, s.Q.VideoPurgeWebhookEvents(ctx))
	return errors.Join(errs...)
}

func (s *Service) outOfMinutes(ctx context.Context, org int64) (bool, error) {
	if s.Wallet == nil {
		return false, nil
	}
	unlimited, err := s.unlimited(ctx, org)
	if err != nil || unlimited {
		return false, err
	}
	balance, err := s.Wallet.MeteredBalance(ctx, org, UnitVideoMinutes)
	if err != nil {
		return false, err
	}
	open, err := s.Q.VideoOpenMinutes(ctx, org)
	if err != nil {
		return false, err
	}
	return balance-open <= 0, nil
}

func (s *Service) limitRecording(ctx context.Context, r db.VideoRoom, now time.Time) error {
	rec, err := s.Q.VideoActiveRecording(ctx, db.VideoActiveRecordingParams{OrganizationID: r.OrganizationID, RoomID: r.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	a, err := s.limits(ctx, r.OrganizationID)
	if err != nil {
		return err
	}
	_, tighten, err := s.orgSettings(ctx, r.OrganizationID)
	if err != nil {
		return err
	}
	if now.Sub(rec.StartedAt.Time) <= time.Duration(a.Tighten(tighten).MaxRecordingMinutes)*time.Minute || rec.EgressID == "" {
		return nil
	}
	return s.LiveKit.StopRecording(ctx, rec.EgressID)
}
