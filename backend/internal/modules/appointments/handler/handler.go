// Package handler is the HTTP layer of the appointments module (CF-138).
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/appointments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Limiter allows an action for a subject (public routes).
type Limiter func(r *http.Request, action string, limit int, window time.Duration) bool

// Handler serves the tenant and public appointment routes.
type Handler struct {
	svc   *usecase.Service
	rec   *activity.Recorder
	q     *db.Queries
	limit Limiter
}

// New builds the handler. q is used for conversation visibility checks.
func New(svc *usecase.Service, rec *activity.Recorder, q *db.Queries, limit Limiter) *Handler {
	return &Handler{svc: svc, rec: rec, q: q, limit: limit}
}

// Service exposes the use case (route gating).
func (h *Handler) Service() *usecase.Service { return h.svc }

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var pe *pgconn.PgError
	switch {
	case entitlement.WriteError(w, r, err):
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Appointment resource not found")
	case errors.Is(err, usecase.ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Message: strings.TrimPrefix(err.Error(), usecase.ErrInvalid.Error()+": ")}})
	case errors.Is(err, usecase.ErrForbidden), errors.Is(err, supportops.ErrDenied):
		response.Forbidden(w, r, "Access denied")
	case errors.Is(err, usecase.ErrDisabled):
		response.Error(w, r, http.StatusForbidden, "APPOINTMENTS_DISABLED", "Appointments are turned off for this organization")
	case errors.Is(err, usecase.ErrSlotTaken):
		response.Conflict(w, r, "SLOT_UNAVAILABLE", "This time is no longer available")
	case errors.Is(err, usecase.ErrPolicy):
		response.Error(w, r, http.StatusForbidden, "APPOINTMENT_POLICY", "The booking policy does not allow this change")
	case errors.Is(err, usecase.ErrRateLimited):
		response.TooManyRequests(w, r, "Too many bookings")
	case errors.As(err, &pe) && (pe.Code == "23514" || pe.Code == "22001"):
		response.ValidationError(w, r, []response.Detail{{Message: "Invalid input"}})
	case errors.As(err, &pe) && pe.Code == "23505":
		response.Conflict(w, r, response.CodeConflict, "Already exists")
	default:
		response.InternalErr(w, r, err, "Appointments unavailable")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		response.ValidationError(w, r, []response.Detail{{Message: "Invalid JSON body"}})
		return false
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		response.ValidationError(w, r, []response.Detail{{Message: "Expected one JSON value"}})
		return false
	}
	return true
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "uuid", Message: "invalid uuid"}})
		return uuid.Nil, false
	}
	return id, true
}

func org(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

func actor(r *http.Request) int64 { return authctx.MustPrincipal(r.Context()).UserInternal }

func (h *Handler) record(r *http.Request, action string, id uuid.UUID) {
	if h.rec == nil {
		return
	}
	a := actor(r)
	switch action {
	case "booked":
		h.rec.Record(r.Context(), &a, "appointments.booked", "appointment", &id, nil, r)
	case "rescheduled":
		h.rec.Record(r.Context(), &a, "appointments.rescheduled", "appointment", &id, nil, r)
	case "status":
		h.rec.Record(r.Context(), &a, "appointments.status_changed", "appointment", &id, nil, r)
	case "updated":
		h.rec.Record(r.Context(), &a, "appointments.updated", "appointment", &id, nil, r)
	case "type_saved":
		h.rec.Record(r.Context(), &a, "appointments.type_saved", "appointment_type", &id, nil, r)
	case "type_deleted":
		h.rec.Record(r.Context(), &a, "appointments.type_deleted", "appointment_type", &id, nil, r)
	case "settings":
		h.rec.Record(r.Context(), &a, "appointments.settings_updated", "appointment_settings", nil, nil, r)
	}
}

// ---------------------------------------------------------------------------
// Settings

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.GetSettings(r.Context(), org(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

func (h *Handler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	var in usecase.SettingsInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.SaveSettings(r.Context(), org(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "settings", uuid.Nil)
	response.JSON(w, r, http.StatusOK, v)
}

// ---------------------------------------------------------------------------
// Types

func (h *Handler) ListTypes(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.ListTypes(r.Context(), org(r), r.URL.Query().Get("active") == "true")
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": v})
}

func (h *Handler) GetType(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.GetType(r.Context(), org(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

func (h *Handler) CreateType(w http.ResponseWriter, r *http.Request) {
	var in usecase.TypeInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.CreateType(r.Context(), org(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "type_saved", v.UUID)
	response.JSON(w, r, http.StatusCreated, v)
}

func (h *Handler) UpdateType(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in usecase.TypeInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.UpdateType(r.Context(), org(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "type_saved", v.UUID)
	response.JSON(w, r, http.StatusOK, v)
}

func (h *Handler) DeleteType(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteType(r.Context(), org(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "type_deleted", id)
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true})
}

func durationParam(r *http.Request) (int32, bool) {
	raw := r.URL.Query().Get("duration")
	if raw == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < 0 {
		return 0, false
	}
	return safeconv.ToInt32(int(n)), true
}

func uuidParam(r *http.Request, key string) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, false
	}
	return &id, true
}

func uuidList(r *http.Request, key string) ([]uuid.UUID, bool) {
	var out []uuid.UUID
	for _, v := range r.URL.Query()[key] {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := uuid.Parse(part)
			if err != nil {
				return nil, false
			}
			out = append(out, id)
		}
	}
	return out, true
}

func list(r *http.Request, key string) []string {
	var out []string
	for _, v := range r.URL.Query()[key] {
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func badQuery(w http.ResponseWriter, r *http.Request, field string) {
	response.ValidationError(w, r, []response.Detail{{Field: field, Message: "invalid " + field}})
}

func (h *Handler) TypeSlots(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	dur, ok := durationParam(r)
	if !ok {
		badQuery(w, r, "duration")
		return
	}
	hostUUID, ok := uuidParam(r, "host_uuid")
	if !ok {
		badQuery(w, r, "host_uuid")
		return
	}
	q := r.URL.Query()
	v, err := h.svc.Slots(r.Context(), org(r), id, q.Get("from"), q.Get("to"), dur, hostUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": v})
}

// ---------------------------------------------------------------------------
// Bookings

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := usecase.ListFilter{Statuses: list(r, "status"), NewestFirst: q.Get("order") == "desc"}
	for _, k := range []string{"from", "to"} {
		if raw := q.Get(k); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				badQuery(w, r, k)
				return
			}
			if k == "from" {
				f.From = &t
			} else {
				f.To = &t
			}
		}
	}
	var ok bool
	if f.HostUUIDs, ok = uuidList(r, "host_uuid"); !ok {
		badQuery(w, r, "host_uuid")
		return
	}
	if f.TypeUUIDs, ok = uuidList(r, "type_uuid"); !ok {
		badQuery(w, r, "type_uuid")
		return
	}
	for key, dst := range map[string]**uuid.UUID{
		"team_uuid": &f.TeamUUID, "contact_uuid": &f.ContactUUID, "company_uuid": &f.CompanyUUID, "conversation_uuid": &f.ConversationUUID,
	} {
		v, ok := uuidParam(r, key)
		if !ok {
			badQuery(w, r, key)
			return
		}
		*dst = v
	}
	if f.ConversationUUID != nil {
		if _, err := supportops.Conversation(r.Context(), h.q, org(r), actor(r), *f.ConversationUUID); err != nil {
			writeError(w, r, mapConv(err))
			return
		}
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || n < 1 || n > 1000 {
			badQuery(w, r, "limit")
			return
		}
		f.Limit = safeconv.ToInt32(int(n))
	}
	v, err := h.svc.List(r.Context(), org(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": v})
}

func (h *Handler) Upcoming(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := int64(10)
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || n < 1 || n > 50 {
			badQuery(w, r, "limit")
			return
		}
		limit = n
	}
	v, err := h.svc.Upcoming(r.Context(), org(r), actor(r), q.Get("scope") != "all", safeconv.ToInt32(int(limit)))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": v})
}

func mapConv(err error) error {
	switch {
	case errors.Is(err, supportops.ErrDenied):
		return usecase.ErrForbidden
	case errors.Is(err, pgx.ErrNoRows):
		return usecase.ErrNotFound
	}
	return err
}

func (h *Handler) canManage(r *http.Request) bool {
	return orgctx.Must(r.Context()).Has(rbac.PermTenantAppointmentsManage)
}

func (h *Handler) Book(w http.ResponseWriter, r *http.Request) {
	var in usecase.BookInput
	if !decode(w, r, &in) {
		return
	}
	a := actor(r)
	canSee := func(ctx context.Context, o int64, id uuid.UUID) (db.Conversation, error) {
		conv, err := supportops.Conversation(ctx, h.q, o, a, id)
		if err != nil {
			return conv, mapConv(err)
		}
		return conv, nil
	}
	v, err := h.svc.Book(r.Context(), org(r), a, h.canManage(r), in, canSee)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "booked", v.UUID)
	response.JSON(w, r, http.StatusCreated, v)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), org(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

func (h *Handler) UpdateDetails(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in usecase.DetailsInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.UpdateDetails(r.Context(), org(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "updated", v.UUID)
	response.JSON(w, r, http.StatusOK, v)
}

func (h *Handler) Reschedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in usecase.RescheduleInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.Reschedule(r.Context(), org(r), actor(r), h.canManage(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "rescheduled", v.UUID)
	response.JSON(w, r, http.StatusOK, v)
}

func (h *Handler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in usecase.StatusInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.SetStatus(r.Context(), org(r), actor(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "status", v.UUID)
	response.JSON(w, r, http.StatusOK, v)
}

func (h *Handler) ICS(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	body, err := h.svc.ICSFor(r.Context(), org(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeICS(w, body)
}

func writeICS(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="appointment.ics"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// ---------------------------------------------------------------------------
// Calendar (ported from otopoly-go calendar handler)

// CalendarMeta returns layers the caller may see/move, colours and time zone.
func (h *Handler) CalendarMeta(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.GetMeta(r.Context(), org(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// CalendarFeed returns layers for ?from=YYYY-MM-DD&to=YYYY-MM-DD (to
// exclusive) with optional layers, host_uuid, team_uuid, type_uuid, status.
func (h *Handler) CalendarFeed(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	layers, err := usecase.ParseLayers(q.Get("layers"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	f := usecase.FeedFilter{Layers: layers, Statuses: list(r, "status")}
	var ok bool
	if f.HostUUIDs, ok = uuidList(r, "host_uuid"); !ok {
		badQuery(w, r, "host_uuid")
		return
	}
	if f.TypeUUIDs, ok = uuidList(r, "type_uuid"); !ok {
		badQuery(w, r, "type_uuid")
		return
	}
	if f.TeamUUID, ok = uuidParam(r, "team_uuid"); !ok {
		badQuery(w, r, "team_uuid")
		return
	}
	v, err := h.svc.GetFeed(r.Context(), org(r), q.Get("from"), q.Get("to"), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// CalendarMove reschedules an appointment by drag and drop or resize.
func (h *Handler) CalendarMove(w http.ResponseWriter, r *http.Request) {
	var in usecase.MoveInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.Move(r.Context(), org(r), actor(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "rescheduled", in.RefUUID)
	response.JSON(w, r, http.StatusOK, v)
}
