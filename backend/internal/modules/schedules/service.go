// Package schedules maintains agent calendars and routing availability.
package schedules

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"time"
)

type Service struct{ Repo *repository.Repository }
type TemplateInput struct {
	Name        string `json:"name"`
	Timezone    string `json:"timezone"`
	StartMinute int32  `json:"start_minute"`
	Duration    int32  `json:"duration_minutes"`
}

func (s *Service) Template(ctx context.Context, org int64, in TemplateInput) (db.ShiftTemplate, error) {
	if _, err := time.LoadLocation(in.Timezone); err != nil || in.Name == "" || len(in.Name) > 200 || in.StartMinute < 0 || in.StartMinute > 1439 || in.Duration < 1 || in.Duration > 1440 {
		return db.ShiftTemplate{}, supportops.ErrInvalid
	}
	return s.Repo.Q.ScheduleCreateTemplate(ctx, db.ScheduleCreateTemplateParams{OrganizationID: org, Name: in.Name, Timezone: in.Timezone, StartMinute: in.StartMinute, DurationMinutes: in.Duration})
}

type ShiftInput struct {
	User     uuid.UUID  `json:"user_uuid"`
	Template *uuid.UUID `json:"template_uuid"`
	Date     string     `json:"date"`
	Starts   time.Time  `json:"starts_at"`
	Ends     time.Time  `json:"ends_at"`
	Timezone string     `json:"timezone"`
}

func (s *Service) Shift(ctx context.Context, org, actor int64, in ShiftInput) (db.AgentShift, error) {
	var out db.AgentShift
	var template pgtype.Int8
	if in.Template != nil {
		t, err := s.Repo.Q.ScheduleTemplate(ctx, db.ScheduleTemplateParams{OrganizationID: org, Uuid: *in.Template})
		if err != nil {
			return out, err
		}
		loc, err := time.LoadLocation(t.Timezone)
		if err != nil {
			return out, err
		}
		day, err := time.ParseInLocation("2006-01-02", in.Date, loc)
		if err != nil {
			return out, supportops.ErrInvalid
		}
		in.Starts = time.Date(day.Year(), day.Month(), day.Day(), int(t.StartMinute)/60, int(t.StartMinute)%60, 0, 0, loc)
		in.Ends = in.Starts.Add(time.Duration(t.DurationMinutes) * time.Minute)
		in.Timezone = t.Timezone
		template = repository.Int(&t.ID)
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil || in.Starts.IsZero() || !in.Ends.After(in.Starts) || in.Ends.Sub(in.Starts) > 24*time.Hour {
		return out, supportops.ErrInvalid
	}
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		user, err := r.Q.ScheduleMember(ctx, db.ScheduleMemberParams{OrganizationID: org, Uuid: in.User})
		if err != nil {
			return err
		}
		if _, err = r.Q.ScheduleLockMember(ctx, db.ScheduleLockMemberParams{OrganizationID: org, UserID: user}); err != nil {
			return err
		}
		overlap, err := r.Q.ScheduleOverlap(ctx, db.ScheduleOverlapParams{OrganizationID: org, UserID: user, EndsAt: repository.TS(in.Starts), StartsAt: repository.TS(in.Ends)})
		if err != nil {
			return err
		}
		if overlap {
			return supportops.ErrConflict
		}
		out, err = r.Q.ScheduleCreateShift(ctx, db.ScheduleCreateShiftParams{OrganizationID: org, UserID: user, TemplateID: template, StartsAt: repository.TS(in.Starts), EndsAt: repository.TS(in.Ends), Timezone: in.Timezone})
		if err != nil {
			return err
		}
		return r.Emit(ctx, events.New("schedule.changed").WithOrganization(org).WithActor(actor).WithEntity("shift", &out.ID, &out.Uuid))
	})
	return out, err
}

type LeaveInput struct {
	Starts time.Time `json:"starts_at"`
	Ends   time.Time `json:"ends_at"`
	Reason string    `json:"reason"`
}

func (s *Service) Leave(ctx context.Context, org, actor int64, in LeaveInput) (db.AgentTimeOff, error) {
	if in.Starts.IsZero() || !in.Ends.After(in.Starts) || in.Ends.Sub(in.Starts) > 366*24*time.Hour || len(in.Reason) > 1000 {
		return db.AgentTimeOff{}, supportops.ErrInvalid
	}
	if _, err := s.Repo.Q.ScheduleLockMember(ctx, db.ScheduleLockMemberParams{OrganizationID: org, UserID: actor}); err != nil {
		return db.AgentTimeOff{}, err
	}
	return s.Repo.Q.ScheduleRequestLeave(ctx, db.ScheduleRequestLeaveParams{OrganizationID: org, UserID: actor, StartsAt: repository.TS(in.Starts), EndsAt: repository.TS(in.Ends), Reason: in.Reason})
}
func (s *Service) Review(ctx context.Context, org, actor int64, id uuid.UUID, status string) (db.AgentTimeOff, error) {
	var out db.AgentTimeOff
	if status != "approved" && status != "rejected" {
		return out, supportops.ErrInvalid
	}
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		var err error
		out, err = r.Q.ScheduleReviewLeave(ctx, db.ScheduleReviewLeaveParams{OrganizationID: org, Uuid: id, Status: status, ReviewedBy: repository.Int(&actor)})
		if err != nil {
			return err
		}
		return r.Emit(ctx, events.New("schedule.leave_reviewed").WithOrganization(org).WithActor(actor).WithEntity("time_off", &out.ID, &out.Uuid))
	})
	return out, err
}
