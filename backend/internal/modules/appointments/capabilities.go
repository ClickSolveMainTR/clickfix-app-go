package appointments

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities of appointments (CF-211): see the calendar, find free
// slots, book, reschedule and change status through /v1/tenant as the
// member. The route checks the plan's appointments module.

// Capabilities are the appointment capabilities.
func Capabilities() []*capability.Capability {
	s, routes := capability.ReadSurfaces, []string{"/calendar", "/settings/appointments"}
	id := capability.UUID("appointment uuid")
	rfc := capability.Text(40, "RFC3339")
	return []*capability.Capability{
		{
			Name: "appointments.upcoming", Module: "appointments", Kind: capability.Read, Permission: rbac.PermTenantAppointmentsRead, Surfaces: s, Routes: routes,
			Title: capability.T("Yaklaşan randevular", "Upcoming appointments"), Description: "My upcoming appointments (scope all = whole organization).",
			Input: capability.Object(nil, map[string]any{"scope": capability.Enum("mine", "all"), "limit": capability.Int(1, 50)}),
			Run:   capability.Get("/v1/tenant/appointments/upcoming", "scope", "limit"),
		},
		{
			Name: "appointments.list", Module: "appointments", Kind: capability.Read, Permission: rbac.PermTenantAppointmentsRead, Surfaces: s, Routes: routes,
			Title: capability.T("Randevular", "Appointments"), Description: "Appointments between from and to (RFC3339), filtered by status, host, contact or conversation.",
			Input: capability.Object(nil, map[string]any{"from": rfc, "to": rfc, "status": capability.Text(80, "csv: scheduled,confirmed,completed,no_show,cancelled"),
				"host_uuid": capability.UUID(""), "contact_uuid": capability.UUID(""), "conversation_uuid": capability.UUID(""), "limit": capability.Int(1, 200)}),
			Run: capability.Get("/v1/tenant/appointments", "from", "to", "status", "host_uuid", "contact_uuid", "conversation_uuid", "limit"),
		},
		{
			Name: "appointments.types", Module: "appointments", Kind: capability.Read, Permission: rbac.PermTenantAppointmentsRead, Surfaces: s, Routes: routes,
			Title: capability.T("Randevu türleri", "Appointment types"), Description: "Appointment types with durations and hosts.",
			Run: capability.Get("/v1/tenant/appointment-types"),
		},
		{
			Name: "appointments.slots", Module: "appointments", Kind: capability.Read, Permission: rbac.PermTenantAppointmentsRead, Surfaces: s, Routes: routes,
			Title: capability.T("Boş saatler", "Free slots"), Description: "Free slots of a type between from and to (YYYY-MM-DD, at most 42 days).",
			Input: capability.Object([]string{"type_uuid", "from", "to"}, map[string]any{"type_uuid": capability.UUID(""), "from": capability.Text(10, "YYYY-MM-DD"),
				"to": capability.Text(10, "YYYY-MM-DD"), "duration": capability.Int(5, 480), "host_uuid": capability.UUID("")}),
			Run: capability.Get("/v1/tenant/appointment-types/{type_uuid}/slots", "from", "to", "duration", "host_uuid"),
		},
		{
			Name: "appointments.create", Module: "appointments", Kind: capability.Write, Permission: rbac.PermTenantAppointmentsWrite, Surfaces: s, Routes: routes,
			Title: capability.T("Randevu oluştur", "Book an appointment"), Description: "Book a free slot (check appointments_slots first); notify e-mails the invitee.",
			Input: capability.Object([]string{"type_uuid", "starts_at"}, map[string]any{"type_uuid": capability.UUID(""), "starts_at": rfc,
				"duration": capability.Int(5, 480), "host_uuid": capability.UUID(""), "title": capability.Text(200, ""), "notes": capability.Text(5000, ""),
				"contact_uuid": capability.UUID(""), "conversation_uuid": capability.UUID(""), "invitee_name": capability.Text(120, ""),
				"invitee_email": capability.Text(254, ""), "invitee_phone": capability.Text(40, ""), "notify": capability.Bool("")}),
			Apply: capability.Endpoint{Method: http.MethodPost, Path: "/v1/tenant/appointments", Route: "/calendar"}.Do(),
		},
		{
			Name: "appointments.reschedule", Module: "appointments", Kind: capability.Write, Permission: rbac.PermTenantAppointmentsWrite, Surfaces: s, Routes: routes,
			Title: capability.T("Randevuyu ertele", "Reschedule"), Description: "Move a scheduled or confirmed appointment; the invitee is notified.",
			Input: capability.Object([]string{"appointment_uuid", "starts_at"}, map[string]any{"appointment_uuid": id, "starts_at": rfc,
				"duration": capability.Int(5, 480), "host_uuid": capability.UUID("")}),
			Apply: capability.Endpoint{Method: http.MethodPost, Path: "/v1/tenant/appointments/{appointment_uuid}/reschedule", Route: "/calendar"}.Do(),
		},
		{
			Name: "appointments.set_status", Module: "appointments", Kind: capability.Write, Permission: rbac.PermTenantAppointmentsWrite, Surfaces: s, Routes: routes,
			Title: capability.T("Randevu durumunu değiştir", "Change appointment status"), Description: "Confirm, complete, mark no-show or cancel (with reason).",
			Input: capability.Object([]string{"appointment_uuid", "status"}, map[string]any{"appointment_uuid": id,
				"status": capability.Enum("confirmed", "completed", "no_show", "cancelled"), "reason": capability.Text(1000, "")}),
			Apply: capability.Endpoint{Method: http.MethodPost, Path: "/v1/tenant/appointments/{appointment_uuid}/status", Body: []string{"status", "reason"}, Route: "/calendar"}.Do(),
		},
	}
}
