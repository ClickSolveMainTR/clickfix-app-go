package demoseed

import (
	"context"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

// seedAppointments runs inside the demo refresh transaction, after its scoped
// wipe. Refreshing replaces these fixtures rather than appending duplicates.
func (s *seeder) seedAppointments(ctx context.Context) error {
	if err := s.q.AppointmentSaveSettings(ctx, db.AppointmentSaveSettingsParams{
		OrganizationID: s.org.ID, Enabled: true, Timezone: "Europe/Istanbul",
		PublicBooking: true, ReminderMinutes: []int32{},
	}); err != nil {
		return err
	}
	definitions := []struct{ name, slug, color, location, address string }{
		{"Ürün demosu", "urun-demosu", "blue", "video", ""},
		{"Kurulum oturumu", "kurulum", "emerald", "in_person", s.brand() + " ofisi"},
		{"Müşteri eğitimi", "egitim", "violet", "phone", "Telefonla görüşme"},
	}
	types := make([]db.AppointmentType, 0, len(definitions))
	for i, definition := range definitions {
		row, err := s.q.AppointmentCreateType(ctx, db.AppointmentCreateTypeParams{
			OrganizationID: s.org.ID, Name: definition.name, Slug: definition.slug,
			Color: definition.color, LocationKind: definition.location, LocationValue: definition.address,
			DurationOptions: []int32{30, 45, 60}, DefaultDuration: 30, SlotInterval: 15,
			HostMode: "round_robin", BookingWindowDays: 90, MinNoticeMinutes: 30,
			IntakeFields: []byte("[]"), AllowCancel: true, AllowReschedule: true,
			IsPublic: true, IsActive: true, Position: safeconv.ToInt32(i),
		})
		if err != nil {
			return err
		}
		for _, user := range demoUsers {
			if err := s.q.AppointmentAddHost(ctx, db.AppointmentAddHostParams{
				OrganizationID: s.org.ID, TypeID: row.ID, UserID: s.users[user.key].ID,
			}); err != nil {
				return err
			}
		}
		types = append(types, row)
	}
	location, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return err
	}
	now := s.opts.Now.In(location)
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location)
	last := first.AddDate(0, 2, 0)
	index := 0
	for day := first; day.Before(last); day = day.AddDate(0, 0, 1) {
		if day.Weekday() == time.Sunday {
			continue
		}
		for slot := 0; slot < 2+day.Day()%2; slot++ {
			typeRow := types[index%len(types)]
			user := s.users[demoUsers[(index/3)%len(demoUsers)].key]
			// Include the owner's calendar every day, so the default "mine" view is populated.
			if slot == 0 {
				user = s.users[demoUsers[0].key]
			}
			starts := day.Add(time.Duration(9+slot*3)*time.Hour + time.Duration(index%2)*30*time.Minute)
			duration := time.Duration(30+(index%3)*15) * time.Minute
			status := "confirmed"
			if starts.Before(now) {
				status = "completed"
				if index%11 == 0 {
					status = "no_show"
				}
			} else if index%5 == 0 {
				status = "scheduled"
			}
			invitee := fmt.Sprintf("Demo Müşteri %d", index+1)
			contactID := pgtype.Int8{}
			if len(s.individuals) > 0 {
				contact := s.individuals[index%len(s.individuals)]
				invitee = contact.Name
				contactID = pgtype.Int8{Int64: contact.ID, Valid: true}
			}
			_, err := s.q.AppointmentCreate(ctx, db.AppointmentCreateParams{
				OrganizationID: s.org.ID, TypeID: typeRow.ID,
				HostUserID: pgtype.Int8{Int64: user.ID, Valid: true}, Status: status,
				StartsAt: ts(starts), EndsAt: ts(starts.Add(duration)), Timezone: "Europe/Istanbul",
				Title: typeRow.Name + " · " + invitee, Notes: "Demo randevusu — kurgusal örnek veri.",
				LocationKind: typeRow.LocationKind, LocationValue: typeRow.LocationValue,
				InviteeName: invitee, InviteeEmail: fmt.Sprintf("musteri%d@demo.example.com", index+1),
				ContactID: contactID, IntakeValues: []byte("{}"), Source: "internal",
				BookedByUserID: pgtype.Int8{Int64: user.ID, Valid: true},
			})
			if err != nil {
				return err
			}
			index++
		}
	}
	return nil
}
