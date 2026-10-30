package handler

import (
	"net/http"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/appointments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// Public booking routes: no auth, rate limited per client IP (reads and
// writes separately) and abuse limited per e-mail in the use case.

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, action string, limit int, window time.Duration) bool {
	w.Header().Set("Cache-Control", "no-store")
	if h.limit != nil && !h.limit(r, action, limit, window) {
		response.TooManyRequests(w, r, "Too many requests")
		return false
	}
	return true
}

func (h *Handler) readable(w http.ResponseWriter, r *http.Request) bool {
	return h.allow(w, r, "appointments_public_read", 120, time.Minute)
}

func (h *Handler) writable(w http.ResponseWriter, r *http.Request) bool {
	return h.allow(w, r, "appointments_public_write", 10, 10*time.Minute)
}

// PublicIndex lists an organization's bookable types.
func (h *Handler) PublicIndex(w http.ResponseWriter, r *http.Request) {
	if !h.readable(w, r) {
		return
	}
	v, err := h.svc.PublicIndex(r.Context(), r.PathValue("org"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// PublicType returns a type's booking page.
func (h *Handler) PublicType(w http.ResponseWriter, r *http.Request) {
	if !h.readable(w, r) {
		return
	}
	v, err := h.svc.PublicGetType(r.Context(), r.PathValue("org"), r.PathValue("type"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// PublicSlots returns free slots of a public type.
func (h *Handler) PublicSlots(w http.ResponseWriter, r *http.Request) {
	if !h.readable(w, r) {
		return
	}
	dur, ok := durationParam(r)
	if !ok {
		badQuery(w, r, "duration")
		return
	}
	q := r.URL.Query()
	v, err := h.svc.PublicSlots(r.Context(), r.PathValue("org"), r.PathValue("type"), q.Get("from"), q.Get("to"), dur)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": v})
}

// PublicBook books a public type.
func (h *Handler) PublicBook(w http.ResponseWriter, r *http.Request) {
	if !h.writable(w, r) {
		return
	}
	var in usecase.PublicBookInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.PublicBook(r.Context(), r.PathValue("org"), r.PathValue("type"), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

// PublicManage returns the booking of a manage token.
func (h *Handler) PublicManage(w http.ResponseWriter, r *http.Request) {
	if !h.readable(w, r) {
		return
	}
	v, err := h.svc.PublicManageGet(r.Context(), r.PathValue("token"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// PublicManageSlots returns free slots to reschedule a booking into.
func (h *Handler) PublicManageSlots(w http.ResponseWriter, r *http.Request) {
	if !h.readable(w, r) {
		return
	}
	q := r.URL.Query()
	v, err := h.svc.PublicRescheduleSlots(r.Context(), r.PathValue("token"), q.Get("from"), q.Get("to"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": v})
}

// PublicICS downloads the booking as an iCalendar file.
func (h *Handler) PublicICS(w http.ResponseWriter, r *http.Request) {
	if !h.readable(w, r) {
		return
	}
	body, err := h.svc.PublicICS(r.Context(), r.PathValue("token"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeICS(w, body)
}

// PublicCancel cancels a booking by manage token.
func (h *Handler) PublicCancel(w http.ResponseWriter, r *http.Request) {
	if !h.writable(w, r) {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.PublicCancel(r.Context(), r.PathValue("token"), in.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// PublicReschedule moves a booking by manage token.
func (h *Handler) PublicReschedule(w http.ResponseWriter, r *http.Request) {
	if !h.writable(w, r) {
		return
	}
	var in usecase.PublicRescheduleInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.PublicReschedule(r.Context(), r.PathValue("token"), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}
