package videorooms

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// WithQueries binds room persistence to the caller's transaction. External
// billing/entitlement reads retain their own repositories: inside a
// transaction that inserted a row referencing the organization, use Preload.
func (s *Service) WithQueries(q *db.Queries) *Service {
	copy := *s
	copy.Q = q
	return &copy
}

// Preload reads the plan snapshot and the video-minute balance now and
// returns a copy that answers Availability from them. Appointment booking
// calls it before its transaction: the appointment insert holds a key lock on
// the organization row, and MeteredBalance takes the billing lock on that row
// from another connection, so reading it inside the transaction waits for
// the transaction itself and never returns.
func (s *Service) Preload(ctx context.Context, org int64) (*Service, error) {
	copy := *s
	if s.Entitlements != nil {
		snap, err := s.Entitlements.Resolve(ctx, org)
		if err != nil {
			return nil, err
		}
		copy.Entitlements = fixedEntitlements{snap: snap}
	}
	if s.Wallet != nil {
		balance, err := s.Wallet.MeteredBalance(ctx, org, UnitVideoMinutes)
		if err != nil {
			return nil, err
		}
		copy.Wallet = fixedBalance{Wallet: s.Wallet, balance: balance}
	}
	return &copy, nil
}

type fixedEntitlements struct{ snap entitlements.Snapshot }

func (f fixedEntitlements) Resolve(context.Context, int64) (entitlements.Snapshot, error) {
	return f.snap, nil
}

// fixedBalance answers the preloaded balance; debits still go to the wallet.
type fixedBalance struct {
	Wallet
	balance int64
}

func (f fixedBalance) MeteredBalance(context.Context, int64, string) (int64, error) {
	return f.balance, nil
}

// syncAppointmentRoom creates or moves a room without sending a second email.
// The appointment service owns confirmation and reminder delivery.
func (s *Service) syncAppointmentRoom(
	ctx context.Context, org, actor int64, appointment uuid.UUID, in AppointmentRoomInput,
) (RoomDetail, error) {
	if appointment == uuid.Nil || in.StartsAt.IsZero() || !in.EndsAt.After(in.StartsAt) {
		return RoomDetail{}, ErrInvalid
	}
	if !in.EndsAt.After(s.now()) || in.EndsAt.After(s.now().Add(366*24*time.Hour)) {
		return RoomDetail{}, ErrInvalid
	}
	start, end := in.StartsAt.Add(-s.AppointmentOpenBefore), in.EndsAt.Add(s.AppointmentGrace)
	minutes := safeconv.ToInt32(int((end.Sub(start) + time.Minute - 1) / time.Minute))
	r, err := s.Q.VideoRoomForAppointment(ctx, db.VideoRoomForAppointmentParams{
		OrganizationID: org, AppointmentUuid: pgtype.UUID{Bytes: appointment, Valid: true},
	})
	if err == nil {
		if r.Status != "open" {
			if r.ScheduledStartAt.Time.Equal(start) && r.ScheduledEndAt.Time.Equal(end) {
				return s.detail(ctx, r, 0)
			}
			return RoomDetail{}, ErrConflict
		}
		host := r.HostUserID
		if in.HostUUID != nil {
			m, err := s.Q.ReportsMember(ctx, db.ReportsMemberParams{OrganizationID: org, Uuid: *in.HostUUID})
			if err != nil {
				return RoomDetail{}, err
			}
			host = pgtype.Int8{Int64: m.ID, Valid: true}
		}
		// Resolve the current policy ceiling when the appointment duration changes.
		availability, err := s.Availability(ctx, org)
		if err != nil {
			return RoomDetail{}, err
		}
		if minutes > availability.Policy.MaxDurationMinutes {
			return RoomDetail{}, ErrDenied
		}
		r, err = s.Q.VideoMoveAppointmentRoom(ctx, db.VideoMoveAppointmentRoomParams{
			OrganizationID: org, Uuid: r.Uuid, ScheduledStartAt: pgtype.Timestamptz{Time: start, Valid: true},
			ScheduledEndAt: pgtype.Timestamptz{Time: end, Valid: true}, HostUserID: host,
			MaxDurationMinutes: minutes,
		})
		if err != nil {
			return RoomDetail{}, err
		}
		return s.detail(ctx, r, 0)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RoomDetail{}, err
	}
	a, err := s.usable(ctx, org, true)
	if err != nil {
		return RoomDetail{}, err
	}
	if !a.Policy.AllowGuestJoin {
		return RoomDetail{}, ErrDenied
	}
	// Refuse a room that would cut the appointment itself short.
	if a.Policy.MaxDurationMinutes < minutes {
		return RoomDetail{}, ErrDenied
	}
	if a.Policy.RequirePIN && in.PIN == "" {
		n, err := rand.Int(rand.Reader, big.NewInt(1000000))
		if err != nil {
			return RoomDetail{}, err
		}
		in.PIN = fmt.Sprintf("%06d", n.Int64())
	}
	d, err := s.CreateRoom(ctx, org, actor, CreateRoomInput{
		Name: truncate(in.Name, 160), PIN: in.PIN, ScheduledStartAt: &start, ScheduledEndAt: &end,
		AppointmentUUID: &appointment, HostUUID: in.HostUUID, MaxDurationMinutes: &minutes,
	})
	if err != nil {
		return d, err
	}
	// Keep the resolved host when shortening credential lifetime to the window.
	r, err = s.Q.VideoRoom(ctx, db.VideoRoomParams{OrganizationID: org, Uuid: d.UUID})
	if err != nil {
		return RoomDetail{}, err
	}
	r, err = s.Q.VideoMoveAppointmentRoom(ctx, db.VideoMoveAppointmentRoomParams{
		OrganizationID: org, Uuid: d.UUID, ScheduledStartAt: r.ScheduledStartAt, ScheduledEndAt: r.ScheduledEndAt,
		HostUserID: r.HostUserID, MaxDurationMinutes: r.MaxDurationMinutes,
	})
	if err != nil {
		return RoomDetail{}, err
	}
	return s.detail(ctx, r, 0)
}

func (s *Service) checkAppointmentWindow(r db.VideoRoom) error {
	if !r.AppointmentUuid.Valid {
		return nil
	}
	if r.ScheduledStartAt.Valid && s.now().Before(r.ScheduledStartAt.Time) {
		return ErrConflict
	}
	if r.ScheduledEndAt.Valid && !s.now().Before(r.ScheduledEndAt.Time) {
		return ErrRoomEnded
	}
	return nil
}
