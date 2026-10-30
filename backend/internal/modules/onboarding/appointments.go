package onboarding

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	appointmentsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/appointments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
)

// AppointmentsAdapter plugs the appointments module (CF-138) into the
// applier: appointment types of a plan are created through its usecase
// (validation, slug, position), hosted round-robin by the owner who ran the
// set-up. Available only while the plan feature and the organization
// switch are on.
type AppointmentsAdapter struct {
	Svc *appointmentsusecase.Service
	Q   *db.Queries
}

// Enabled reports whether the organization may use appointments.
func (a AppointmentsAdapter) Enabled(ctx context.Context, org int64) bool {
	st, err := a.Svc.GetSettings(ctx, org)
	return err == nil && st.FeatureEnabled && st.Enabled
}

// Find looks an active appointment type up by name.
func (a AppointmentsAdapter) Find(ctx context.Context, org int64, name string) (uuid.UUID, bool, error) {
	types, err := a.Svc.ListTypes(ctx, org, false)
	if err != nil {
		return uuid.Nil, false, err
	}
	for _, t := range types {
		if strings.EqualFold(t.Name, name) {
			return t.UUID, true, nil
		}
	}
	return uuid.Nil, false, nil
}

// Create adds an appointment type with one duration, hosted by the actor.
func (a AppointmentsAdapter) Create(ctx context.Context, org, actor int64, in PlanAppointment) (uuid.UUID, error) {
	user, err := a.Q.GetUserByID(ctx, actor)
	if err != nil {
		return uuid.Nil, err
	}
	d := safeconv.ToInt32(in.DurationMinutes)
	t, err := a.Svc.CreateType(ctx, org, appointmentsusecase.TypeInput{
		Name: in.Name, Description: in.Description, DurationOptions: []int32{d}, DefaultDuration: d,
		LocationKind: "video", HostMode: "round_robin", HostUUIDs: []uuid.UUID{user.Uuid},
		BookingWindowDays: 60, MinNoticeMinutes: 240, AllowCancel: true, AllowReschedule: true,
		IsPublic: true, IsActive: true,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return t.UUID, nil
}

// Delete removes an appointment type created by the set-up.
func (a AppointmentsAdapter) Delete(ctx context.Context, org int64, id uuid.UUID) error {
	return a.Svc.DeleteType(ctx, org, id)
}
