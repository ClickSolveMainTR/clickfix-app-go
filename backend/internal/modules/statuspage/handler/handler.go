package handler

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"io"
	"net/http"
	"strconv"
)

type Handler struct {
	svc   *usecase.Service
	rec   *activity.Recorder
	limit func(*http.Request) bool
}

func New(s *usecase.Service, rec *activity.Recorder, limit func(*http.Request) bool) *Handler {
	return &Handler{s, rec, limit}
}
func fail(w http.ResponseWriter, r *http.Request, err error) {
	var pe *pgconn.PgError
	switch {
	case errors.Is(err, model.ErrInvalid):
		response.Error(w, r, 400, "VALIDATION_ERROR", "Invalid status input")
	case errors.Is(err, model.ErrConflict):
		response.Conflict(w, r, "STATUS_CONFLICT", "Incident already resolved")
	case errors.As(err, &pe) && pe.Code == "23505":
		response.Conflict(w, r, "STATUS_CONFLICT", "Already exists")
	case errors.Is(err, pgx.ErrNoRows):
		response.NotFound(w, r, "Status resource not found")
	default:
		response.Internal(w, r, "Status unavailable")
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		fail(w, r, model.ErrInvalid)
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		response.Error(w, r, 400, "VALIDATION_ERROR", "Expected one JSON value")
		return false
	}
	return true
}
func id(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	v, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		fail(w, r, model.ErrInvalid)
		return v, false
	}
	return v, true
}
func (h *Handler) record(r *http.Request, id uuid.UUID, op string) {
	p, ok := authctx.PrincipalFrom(r.Context())
	var actor *int64
	if ok {
		actor = &p.UserInternal
	}
	h.rec.Record(r.Context(), actor, "settings.updated", "settings", &id, map[string]any{"module": "status", "operation": op}, r)
}
func (h *Handler) Page(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	if r.Method == http.MethodGet {
		v, err := h.svc.Page(r.Context(), org)
		if err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, v)
		return
	}
	var in model.PageInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.SavePage(r.Context(), org, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.record(r, v.Uuid, "save_page")
	response.JSON(w, r, 200, v)
}
func (h *Handler) Components(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	if r.Method == http.MethodGet {
		v, err := h.svc.Components(r.Context(), org)
		if err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, map[string]any{"items": v})
		return
	}
	var in model.ComponentInput
	if !decode(w, r, &in) {
		return
	}
	var uid uuid.UUID
	if r.PathValue("uuid") != "" {
		var ok bool
		uid, ok = id(w, r)
		if !ok {
			return
		}
	}
	v, err := h.svc.SaveComponent(r.Context(), org, uid, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.record(r, v.Uuid, "save_component")
	response.JSON(w, r, 200, v)
}
func (h *Handler) Incidents(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	if r.Method == http.MethodGet {
		if len(apiquery.ForbiddenParams(r.URL.Query())) > 0 || r.URL.Query().Get("sort") != "" {
			fail(w, r, model.ErrInvalid)
			return
		}
		q := apiquery.Parse(r.URL.Query())
		v, err := h.svc.Incidents(r.Context(), org, false, int32(q.Limit), int32(q.Offset))
		if err != nil {
			fail(w, r, err)
			return
		}
		total, err := h.svc.IncidentCount(r.Context(), org)
		if err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, map[string]any{"items": v, "total": total, "limit": q.Limit, "offset": q.Offset})
		return
	}
	var in model.IncidentInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.CreateIncident(r.Context(), org, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.record(r, v.Uuid, "create_incident")
	response.JSON(w, r, 201, v)
}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	uid, ok := id(w, r)
	if !ok {
		return
	}
	var in model.UpdateInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.Update(r.Context(), orgctx.Must(r.Context()).ID, uid, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.record(r, uid, "update_incident")
	response.JSON(w, r, 200, v)
}
func (h *Handler) Link(w http.ResponseWriter, r *http.Request) {
	uid, ok := id(w, r)
	if !ok {
		return
	}
	var in struct {
		Conversation uuid.UUID `json:"conversation_uuid"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.Link(r.Context(), orgctx.Must(r.Context()).ID, uid, in.Conversation); err != nil {
		fail(w, r, err)
		return
	}
	h.record(r, uid, "link_conversation")
	response.JSON(w, r, 200, map[string]bool{"linked": true})
}
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	uid, ok := id(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Preview(r.Context(), orgctx.Must(r.Context()).ID, uid)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, v)
}
func (h *Handler) Public(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Public(r.Context(), r.PathValue("slug"))
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	response.JSON(w, r, 200, v)
}
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	days := 90
	if v := r.URL.Query().Get("days"); v != "" {
		days, _ = strconv.Atoi(v)
	}
	v, err := h.svc.History(r.Context(), r.PathValue("slug"), days)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"items": v, "timezone": "UTC"})
}
func (h *Handler) Subscribe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.limit != nil && !h.limit(r) {
		response.Error(w, r, 429, "RATE_LIMITED", "Too many requests")
		return
	}
	var in struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.Subscribe(r.Context(), r.PathValue("slug"), in.Email); err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 202, map[string]string{"status": "confirmation_required"})
}

// SubscriptionPreview says what a subscription link does; it changes
// nothing (link scanners and previews open e-mail links with GET).
func (h *Handler) SubscriptionPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	out, err := h.svc.PreviewSubscription(r.Context(), r.PathValue("slug"), r.PathValue("token"))
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}

// SubscriptionAction confirms or cancels the subscription (the button on
// the confirmation page; also RFC 8058 one-click unsubscribe).
func (h *Handler) SubscriptionAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if err := h.svc.SubscriptionAction(r.Context(), r.PathValue("slug"), r.PathValue("token")); err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]bool{"ok": true})
}

type rss struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}
type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}
type rssItem struct {
	Title       string  `xml:"title"`
	GUID        rssGUID `xml:"guid"`
	Description string  `xml:"description"`
}

func (h *Handler) RSS(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Public(r.Context(), r.PathValue("slug"))
	if err != nil {
		fail(w, r, err)
		return
	}
	var items []struct {
		UUID   string `json:"uuid"`
		Title  string `json:"title"`
		Status string `json:"status"`
	}
	raw, _ := v["incidents"].(json.RawMessage)
	if json.Unmarshal(raw, &items) != nil {
		response.Internal(w, r, "Feed unavailable")
		return
	}
	feed := rss{Version: "2.0", Channel: rssChannel{Title: v["title"].(string), Link: h.svc.PublicURL(r.PathValue("slug")), Description: "Service status updates"}}
	for _, i := range items {
		feed.Channel.Items = append(feed.Channel.Items, rssItem{Title: i.Title, GUID: rssGUID{IsPermaLink: "false", Value: i.UUID}, Description: i.Status})
	}
	body, err := xml.Marshal(feed)
	if err != nil {
		response.Internal(w, r, "Feed unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	_, _ = w.Write(append([]byte(xml.Header), body...))
}

type rssGUID struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}
