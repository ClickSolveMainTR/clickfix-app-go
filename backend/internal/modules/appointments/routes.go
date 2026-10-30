// Package appointments wires the appointments module and the organization
// calendar (CF-138): /v1/tenant/appointments/*, /v1/tenant/calendar/* and
// the public booking routes /v1/public/booking/* and /v1/public/appointments/*.
package appointments

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/appointments/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/appointments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// RegisterRoutes mounts the module. Every tenant route except the settings
// ones requires the module.appointments plan feature and the organization
// switch; the public routes 404 when either is off.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	mux.HandleFunc("GET /v1/public/booking/{org}", h.PublicIndex)
	mux.HandleFunc("GET /v1/public/booking/{org}/{type}", h.PublicType)
	mux.HandleFunc("GET /v1/public/booking/{org}/{type}/slots", h.PublicSlots)
	mux.HandleFunc("POST /v1/public/booking/{org}/{type}", h.PublicBook)
	mux.HandleFunc("GET /v1/public/appointments/{token}", h.PublicManage)
	mux.HandleFunc("GET /v1/public/appointments/{token}/slots", h.PublicManageSlots)
	mux.HandleFunc("GET /v1/public/appointments/{token}/ics", h.PublicICS)
	mux.HandleFunc("POST /v1/public/appointments/{token}/cancel", h.PublicCancel)
	mux.HandleFunc("POST /v1/public/appointments/{token}/reschedule", h.PublicReschedule)
	if resolver == nil {
		return
	}
	svc := h.Service()
	gate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := svc.Gate(r.Context(), orgctx.Must(r.Context()).ID); err != nil {
				switch {
				case entitlement.WriteError(w, r, err):
				case errors.Is(err, usecase.ErrDisabled):
					response.Error(w, r, http.StatusForbidden, "APPOINTMENTS_DISABLED", "Appointments are turned off for this organization")
				default:
					response.InternalErr(w, r, err, "")
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	route := func(pattern, perm string, fn http.HandlerFunc, gated bool) {
		chain := []func(http.Handler) http.Handler{
			middleware.Authenticate(tokens, loader), middleware.RequireOrganization(resolver), middleware.RequireTenantPermission(perm),
		}
		if gated {
			chain = append(chain, gate)
		}
		mux.Handle(pattern, middleware.Chain(fn, chain...))
	}
	read, write, manage := rbac.PermTenantAppointmentsRead, rbac.PermTenantAppointmentsWrite, rbac.PermTenantAppointmentsManage

	route("GET /v1/tenant/appointments/settings", read, h.GetSettings, false)
	route("PUT /v1/tenant/appointments/settings", manage, h.SaveSettings, false)

	route("GET /v1/tenant/appointment-types", read, h.ListTypes, true)
	route("POST /v1/tenant/appointment-types", manage, h.CreateType, true)
	route("GET /v1/tenant/appointment-types/{uuid}", read, h.GetType, true)
	route("PUT /v1/tenant/appointment-types/{uuid}", manage, h.UpdateType, true)
	route("DELETE /v1/tenant/appointment-types/{uuid}", manage, h.DeleteType, true)
	route("GET /v1/tenant/appointment-types/{uuid}/slots", read, h.TypeSlots, true)

	route("GET /v1/tenant/appointments", read, h.List, true)
	route("POST /v1/tenant/appointments", write, h.Book, true)
	route("GET /v1/tenant/appointments/upcoming", read, h.Upcoming, true)
	route("GET /v1/tenant/appointments/{uuid}", read, h.Get, true)
	route("PATCH /v1/tenant/appointments/{uuid}", write, h.UpdateDetails, true)
	route("POST /v1/tenant/appointments/{uuid}/reschedule", write, h.Reschedule, true)
	route("POST /v1/tenant/appointments/{uuid}/status", write, h.SetStatus, true)
	route("GET /v1/tenant/appointments/{uuid}/ics", read, h.ICS, true)

	// Calendar (ported from otopoly-go /v1/tenant/calendar). Per-layer read
	// and move permissions are checked in the use case.
	route("GET /v1/tenant/calendar/meta", read, h.CalendarMeta, true)
	route("GET /v1/tenant/calendar/feed", read, h.CalendarFeed, true)
	route("POST /v1/tenant/calendar/items/move", write, h.CalendarMove, true)
}
