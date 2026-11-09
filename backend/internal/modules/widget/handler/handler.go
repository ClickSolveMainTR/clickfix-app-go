// Package handler serves the public web widget API (/v1/widget/{public_id}/*,
// CF-55). The routes are called from customers' sites, so they answer CORS
// (no credentials: the visitor token travels in the Authorization header)
// and check the page origin against the widget's allowlist.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	formshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/handler"
	formsmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/model"
	helpusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/helpcenter/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the widget API.
const (
	CodeWidgetNotFound        = "WIDGET_NOT_FOUND"
	CodeOriginNotAllowed      = "WIDGET_ORIGIN_NOT_ALLOWED"
	CodeVisitorToken          = "WIDGET_VISITOR_TOKEN_INVALID"
	CodeIdentityDisabled      = usecase.RejectionIdentityDisabled
	CodeIdentityInvalid       = usecase.RejectionIdentityInvalid
	CodeIdentityRequired      = "WIDGET_IDENTITY_REQUIRED"
	CodeConversationClosed    = "WIDGET_CONVERSATION_CLOSED"
	CodeRatingNotAllowed      = "WIDGET_RATING_NOT_ALLOWED"
	CodeRatingDisabled        = "WIDGET_RATING_DISABLED"
	CodeUploadDisabled        = "WIDGET_UPLOAD_DISABLED"
	CodeFileTooLarge          = "WIDGET_FILE_TOO_LARGE"
	CodeConversationNotFound  = "WIDGET_CONVERSATION_NOT_FOUND"
	CodeFileType              = "WIDGET_FILE_TYPE"
	CodeConsentRequired       = "WIDGET_CONSENT_REQUIRED"
	maxJSONBytes              = 64 << 10
	multipartOverheadBytes    = 1 << 20
	defaultRequestsPerMinute  = 300
	typingPerMinute           = 60
	bootstrapsPerMinutePerIP  = 60
	identifyPerMinutePerIP    = 20
	uploadContentTypeFallback = "application/octet-stream"
)

// Handler serves the widget routes.
type Handler struct {
	svc        *usecase.Service
	limiter    RateLimiter
	log        *slog.Logger
	help       HelpSearcher
	helpOrigin string
	booking    BookingLinker
	country    func(*http.Request) string
}

// WithCountry resolves the visitor's country (ISO alpha-2) for country
// targeting; without it country rules never hide the widget.
func (h *Handler) WithCountry(fn func(*http.Request) string) *Handler {
	h.country = fn
	return h
}

// RateLimiter throttles by action and subject.
type RateLimiter func(r *http.Request, action, subject string, limit int) (bool, time.Duration)

// New builds the handler. limit may be nil (no throttling).
func New(svc *usecase.Service, limit RateLimiter, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{svc: svc, limiter: limit, log: log}
}

// CORS answers preflights and adds the CORS headers to every widget
// response. Any origin may call (the per-widget allowlist is enforced by
// the handlers); credentials are never allowed.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Add("Vary", "Origin")
		if origin := r.Header.Get("Origin"); origin != "" {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Widget-Page")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Preflight answers OPTIONS for every widget path.
func (h *Handler) Preflight(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// OriginHeader carries the page origin when the request is relayed by the
// app's BFF (which never forwards the browser Origin header to the API).
const OriginHeader = "X-Widget-Origin"

// pageOrigin is the origin of the page the widget runs on: the relayed
// origin, the Origin header, else the Referer's origin (same-origin GETs
// omit Origin). Non-browser callers can claim any origin; the allowlist
// keeps other sites' pages from embedding the widget, not scripts.
func pageOrigin(r *http.Request) string {
	if o := r.Header.Get(OriginHeader); o != "" {
		return o
	}
	if o := r.Header.Get("Origin"); o != "" {
		return o
	}
	ref := r.Header.Get("Referer")
	if ref == "" {
		return ""
	}
	if i := strings.Index(ref, "://"); i > 0 {
		rest := ref[i+3:]
		if j := strings.IndexAny(rest, "/?#"); j >= 0 {
			rest = rest[:j]
		}
		return ref[:i+3] + rest
	}
	return ""
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" {
			return ip
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, action, subject string, limit int) bool {
	if h.limiter == nil {
		return true
	}
	ok, wait := h.limiter(r, "widget_"+action, subject, limit)
	if !ok {
		if wait > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		}
		response.TooManyRequests(w, r, "Too many requests, please slow down")
		return false
	}
	return true
}

// open resolves the widget of the path and checks the page origin.
func (h *Handler) open(w http.ResponseWriter, r *http.Request) (usecase.Widget, bool) {
	id, err := uuid.Parse(r.PathValue("public_id"))
	if err != nil {
		response.Error(w, r, http.StatusNotFound, CodeWidgetNotFound, "Widget not found")
		return usecase.Widget{}, false
	}
	if !h.allow(w, r, "request", id.String()+":"+clientIP(r), defaultRequestsPerMinute) {
		return usecase.Widget{}, false
	}
	wd, err := h.svc.Open(r.Context(), id, pageOrigin(r))
	if err != nil {
		h.fail(w, r, err)
		return usecase.Widget{}, false
	}
	return wd, true
}

// visitor resolves the widget and the visitor token.
func (h *Handler) visitor(w http.ResponseWriter, r *http.Request) (usecase.Widget, widget.Claims, bool) {
	wd, ok := h.open(w, r)
	if !ok {
		return usecase.Widget{}, widget.Claims{}, false
	}
	claims, err := h.svc.Authenticate(wd, bearer(r))
	if err != nil {
		h.fail(w, r, err)
		return usecase.Widget{}, widget.Claims{}, false
	}
	return wd, claims, true
}

func bearer(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
		return strings.TrimSpace(v[7:])
	}
	return ""
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.ContentLength == 0 {
		return true
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

func conversationID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("conversation"))
	if err != nil {
		response.Error(w, r, http.StatusNotFound, CodeConversationNotFound, "Conversation not found")
		return uuid.Nil, false
	}
	return id, true
}

// Bootstrap starts or resumes the visitor session.
func (h *Handler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	wd, ok := h.open(w, r)
	if !ok {
		return
	}
	if !h.allow(w, r, "bootstrap", clientIP(r), bootstrapsPerMinutePerIP) {
		return
	}
	var in usecase.BootstrapInput
	if !decode(w, r, &in) {
		return
	}
	in.UserAgent, in.Origin = r.UserAgent(), pageOrigin(r)
	if h.country != nil {
		in.Country = h.country(r)
	}
	out, err := h.svc.Bootstrap(r.Context(), wd, bearer(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.PersonalizeGreeting(r.Context(), wd, &out); err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Identify verifies the user_hash and switches the visitor to the user.
func (h *Handler) Identify(w http.ResponseWriter, r *http.Request) {
	wd, claims, ok := h.visitor(w, r)
	if !ok || !h.allow(w, r, "identify", clientIP(r), identifyPerMinutePerIP) {
		return
	}
	var in usecase.IdentifyInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Identify(r.Context(), wd, claims, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// UpdateProfile updates the visitor's own contact.
func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	wd, claims, ok := h.visitor(w, r)
	if !ok {
		return
	}
	var in usecase.ProfileInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.UpdateProfile(r.Context(), wd, claims, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListConversations returns the visitor's conversations.
func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	wd, claims, ok := h.visitor(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListConversations(r.Context(), wd, claims)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type sendBody struct {
	Text     string                 `json:"text"`
	ClientID *uuid.UUID             `json:"client_id"`
	Name     string                 `json:"name"`
	Email    string                 `json:"email"`
	PageURL  string                 `json:"page_url"`
	Events   []usecase.TrackedEvent `json:"events"`
	Profile  *usecase.ProfileInput  `json:"profile"`
	// Form is the request form (CF-52) submitted with a first message.
	Form *formsmodel.Submission `json:"form"`
	// CF-150: custom pre-chat answers, consent, intent and SDK attributes.
	PreChat    map[string]any `json:"prechat"`
	Consent    *bool          `json:"consent"`
	Intent     string         `json:"intent"`
	Attributes map[string]any `json:"attributes"`
	Phone      string         `json:"phone"`
}

// readSend reads a JSON or multipart (text + one "file") message.
func (h *Handler) readSend(w http.ResponseWriter, r *http.Request) (usecase.SendInput, bool) {
	var in usecase.SendInput
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxUploadBytes+multipartOverheadBytes)
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			response.Error(w, r, http.StatusRequestEntityTooLarge, CodeFileTooLarge, "The file is too large")
			return in, false
		}
		in.Text = r.FormValue("text")
		in.PageURL = r.FormValue("page_url")
		in.Profile = usecase.ProfileInput{Name: r.FormValue("name"), Email: r.FormValue("email"), Phone: r.FormValue("phone")}
		in.Intent = r.FormValue("intent")
		if v := r.FormValue("consent"); v != "" {
			given := v == "true"
			in.Consent = &given
		}
		for key, dst := range map[string]*map[string]any{"prechat": &in.PreChat, "attributes": &in.Attributes} {
			if v := r.FormValue(key); v != "" {
				if err := json.Unmarshal([]byte(v), dst); err != nil {
					response.BadRequest(w, r, response.CodeValidationError, key+" must be a JSON object")
					return in, false
				}
			}
		}
		if v := r.FormValue("form"); v != "" {
			var sub formsmodel.Submission
			if err := json.Unmarshal([]byte(v), &sub); err != nil {
				response.BadRequest(w, r, response.CodeValidationError, "form must be a JSON object")
				return in, false
			}
			in.Form = &sub
		}
		if v := r.FormValue("client_id"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				response.BadRequest(w, r, response.CodeValidationError, "client_id must be a uuid")
				return in, false
			}
			in.ClientID = id
		}
		if f, hdr, err := r.FormFile("file"); err == nil {
			defer func() { _ = f.Close() }()
			data, err := io.ReadAll(io.LimitReader(f, usecase.MaxUploadBytes+1))
			if err != nil {
				response.BadRequest(w, r, response.CodeValidationError, "Could not read the file")
				return in, false
			}
			if len(data) > usecase.MaxUploadBytes {
				response.Error(w, r, http.StatusRequestEntityTooLarge, CodeFileTooLarge, "The file is too large")
				return in, false
			}
			ct := hdr.Header.Get("Content-Type")
			if ct == "" {
				ct = uploadContentTypeFallback
			}
			in.File = &usecase.Upload{Filename: hdr.Filename, ContentType: ct, Data: data}
		}
		return in, true
	}
	var body sendBody
	if !decode(w, r, &body) {
		return in, false
	}
	in.Text, in.PageURL, in.Events = body.Text, body.PageURL, body.Events
	in.Profile = usecase.ProfileInput{Name: body.Name, Email: body.Email, Phone: body.Phone}
	in.PreChat, in.Consent, in.Intent, in.Attributes = body.PreChat, body.Consent, body.Intent, body.Attributes
	if body.Profile != nil {
		in.Profile = *body.Profile
	}
	if body.ClientID != nil {
		in.ClientID = *body.ClientID
	}
	in.Form = body.Form
	return in, true
}

// CreateConversation starts a conversation with its first message.
func (h *Handler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	h.send(w, r, false)
}

// SendMessage adds a visitor message to a conversation.
func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	h.send(w, r, true)
}

func (h *Handler) send(w http.ResponseWriter, r *http.Request, existing bool) {
	wd, claims, ok := h.visitor(w, r)
	if !ok || !h.allow(w, r, "message", wd.Instance.UUID.String()+":"+claims.Visitor, wd.Settings.RateLimit.MessagesPerMinute) {
		return
	}
	in, ok := h.readSend(w, r)
	if !ok {
		return
	}
	if existing {
		id, ok := conversationID(w, r)
		if !ok {
			return
		}
		in.ConversationUUID = &id
	}
	out, err := h.svc.Send(r.Context(), wd, claims, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if !existing {
		status = http.StatusCreated
	}
	response.JSON(w, r, status, out)
}

// ListMessages returns a page of a conversation's messages.
func (h *Handler) ListMessages(w http.ResponseWriter, r *http.Request) {
	wd, claims, ok := h.visitor(w, r)
	if !ok {
		return
	}
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	cursor := func(key string) (uuid.UUID, bool) {
		v := r.URL.Query().Get(key)
		if v == "" {
			return uuid.Nil, true
		}
		u, err := uuid.Parse(v)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, key+" must be a message uuid")
			return uuid.Nil, false
		}
		return u, true
	}
	after, ok := cursor("after")
	if !ok {
		return
	}
	before, ok := cursor("before")
	if !ok {
		return
	}
	page, err := h.svc.ListMessages(r.Context(), wd, claims, id, after, before)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

// MarkRead marks the agent replies of a conversation read.
func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	wd, claims, ok := h.visitor(w, r)
	if !ok {
		return
	}
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	n, err := h.svc.MarkRead(r.Context(), wd, claims, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"updated": n})
}

// Typing relays the visitor's typing state to the agents.
func (h *Handler) Typing(w http.ResponseWriter, r *http.Request) {
	wd, claims, ok := h.visitor(w, r)
	if !ok || !h.allow(w, r, "typing", wd.Instance.UUID.String()+":"+claims.Visitor, typingPerMinute) {
		return
	}
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	var body struct {
		Typing bool `json:"typing"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := h.svc.Typing(r.Context(), wd, claims, id, body.Typing); err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"typing": body.Typing})
}

// Rate stores the CSAT rating of a finished conversation.
func (h *Handler) Rate(w http.ResponseWriter, r *http.Request) {
	wd, claims, ok := h.visitor(w, r)
	if !ok {
		return
	}
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	var body struct {
		Score   int    `json:"score"`
		Comment string `json:"comment"`
	}
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.Rate(r.Context(), wd, claims, id, body.Score, body.Comment)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// SetAttributes stores mapped SDK attributes on the visitor's contact.
func (h *Handler) SetAttributes(w http.ResponseWriter, r *http.Request) {
	wd, claims, ok := h.visitor(w, r)
	if !ok || !h.allow(w, r, "attributes", wd.Instance.UUID.String()+":"+claims.Visitor, typingPerMinute) {
		return
	}
	var body struct {
		Attributes map[string]any `json:"attributes"`
	}
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.SetAttributes(r.Context(), wd, claims, body.Attributes)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// RealtimeToken issues the visitor's Centrifugo tokens.
func (h *Handler) RealtimeToken(w http.ResponseWriter, r *http.Request) {
	wd, claims, ok := h.visitor(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Realtime(wd, claims)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if entitlement.WriteError(w, r, err) {
		return
	}
	response.RecordFailure(w, err)
	var ve *customfields.ValidationError
	switch {
	case errors.As(err, &ve):
		// Request form fields (CF-52), keyed by form key.
		formshandler.WriteError(w, r, err)
	case errors.Is(err, usecase.ErrDiagnosticQuota):
		response.TooManyRequests(w, r, "Tanı verisi kotası aşıldı")
	case errors.Is(err, usecase.ErrNotFound):
		response.Error(w, r, http.StatusNotFound, CodeWidgetNotFound, "Not found")
	case errors.Is(err, usecase.ErrOriginNotAllowed):
		response.Error(w, r, http.StatusForbidden, CodeOriginNotAllowed, "This site may not use the widget")
	case errors.Is(err, usecase.ErrUnauthorized):
		response.Error(w, r, http.StatusUnauthorized, CodeVisitorToken, "Visitor token is missing or invalid")
	case errors.Is(err, usecase.ErrIdentityDisabled):
		response.Error(w, r, http.StatusForbidden, CodeIdentityDisabled, "Identity verification is not configured for this widget")
	case errors.Is(err, usecase.ErrIdentityInvalid):
		response.Error(w, r, http.StatusUnauthorized, CodeIdentityInvalid, "user_hash does not match user_id")
	case errors.Is(err, usecase.ErrIdentityRequired):
		response.Error(w, r, http.StatusForbidden, CodeIdentityRequired, "Sign in to chat with us")
	case errors.Is(err, usecase.ErrConversationClosed):
		response.Conflict(w, r, CodeConversationClosed, "This conversation is closed; start a new one")
	case errors.Is(err, usecase.ErrRatingNotAllowed):
		response.Conflict(w, r, CodeRatingNotAllowed, "Only finished conversations can be rated")
	case errors.Is(err, usecase.ErrRatingDisabled):
		response.Error(w, r, http.StatusForbidden, CodeRatingDisabled, "Ratings are turned off")
	case errors.Is(err, usecase.ErrUploadDisabled):
		response.Error(w, r, http.StatusForbidden, CodeUploadDisabled, "File upload is turned off")
	case errors.Is(err, usecase.ErrFileType):
		response.Error(w, r, http.StatusUnsupportedMediaType, CodeFileType, "This file type is not accepted")
	case errors.Is(err, usecase.ErrConsentRequired):
		response.Error(w, r, http.StatusBadRequest, CodeConsentRequired, "Please accept the privacy notice")
	case errors.Is(err, usecase.ErrFileTooLarge):
		response.Error(w, r, http.StatusRequestEntityTooLarge, CodeFileTooLarge, "The file is too large")
	case errors.Is(err, usecase.ErrRealtimeDisabled):
		response.ServiceUnavailable(w, r, response.CodeRealtimeDisabled, "Realtime is disabled")
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Widget request failed")
	}
}

// HelpSearcher reads the organization's public help center (CF-74).
type HelpSearcher interface {
	HasHelpCenter(ctx context.Context, org int64) (string, bool)
	SearchOrganization(ctx context.Context, org int64, query, locale string, limit int) ([]helpusecase.ArticleSummary, error)
	RecentOrganization(ctx context.Context, org int64, locale string, limit int) ([]helpusecase.ArticleSummary, error)
}

// WithHelp enables the widget help search; appOrigin builds the help center
// links (the app's public origin).
func (h *Handler) WithHelp(help HelpSearcher, appOrigin string) *Handler {
	h.help = help
	h.helpOrigin = strings.TrimRight(strings.TrimSpace(appOrigin), "/")
	return h
}

// BookingLinker names an organization's public booking page (CF-138);
// ok is false when the organization has no bookable public types.
type BookingLinker interface {
	BookingURL(ctx context.Context, org int64) (string, bool)
}

// WithBooking enables the widget's "book a meeting" entry point.
func (h *Handler) WithBooking(b BookingLinker) *Handler {
	h.booking = b
	return h
}

// BookingLink is the widget booking entry point payload.
type BookingLink struct {
	Available bool    `json:"available"`
	URL       *string `json:"url"`
}

// Booking is GET /v1/widget/{public_id}/booking: the organization's public
// booking page when appointments and public booking are on.
func (h *Handler) Booking(w http.ResponseWriter, r *http.Request) {
	wd, ok := h.open(w, r)
	if !ok {
		return
	}
	out := BookingLink{}
	if h.booking != nil {
		if u, ok := h.booking.BookingURL(r.Context(), wd.Instance.OrganizationID); ok {
			out.Available, out.URL = true, &u
		}
	}
	response.JSON(w, r, http.StatusOK, out)
}

// HelpArticle is one help center article in the widget.
type HelpArticle struct {
	Title   string `json:"title"`
	Excerpt string `json:"excerpt"`
	URL     string `json:"url"`
}

// HelpResult is the widget help tab payload.
type HelpResult struct {
	Available     bool          `json:"available"`
	HelpCenterURL *string       `json:"help_center_url"`
	Items         []HelpArticle `json:"items"`
}

const helpSearchesPerMinute = 30

// HelpSearch is GET /v1/widget/{public_id}/help/search?q=&locale=: published
// public articles of the widget's organization (the latest ones without q).
func (h *Handler) HelpSearch(w http.ResponseWriter, r *http.Request) {
	wd, ok := h.open(w, r)
	if !ok {
		return
	}
	out := HelpResult{Items: []HelpArticle{}}
	if !h.allow(w, r, "brand_help", clientIP(r), helpSearchesPerMinute) {
		return
	}
	if wd.Instance.BrandID != nil {
		rows, slug, err := h.svc.BrandHelp(r.Context(), wd, r.URL.Query().Get("q"), r.URL.Query().Get("locale"))
		if err != nil {
			h.fail(w, r, err)
			return
		}
		base := h.helpOrigin + "/help/" + url.PathEscape(slug) + "/brands/" + wd.Instance.BrandID.String()
		out.Available = true
		out.HelpCenterURL = &base
		ids := make([]uuid.UUID, 0, len(rows))
		for _, a := range rows {
			ids = append(ids, a.Uuid)
		}
		hidden, err := h.svc.HiddenArticles(r.Context(), wd, bearer(r), ids)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		for _, a := range rows {
			if hidden[a.Uuid] {
				continue
			}
			out.Items = append(out.Items, HelpArticle{Title: a.Title, Excerpt: a.Excerpt, URL: base + "/article/" + url.PathEscape(a.Slug) + "?lang=" + a.Locale})
		}
		response.JSON(w, r, http.StatusOK, out)
		return
	}
	if h.help == nil {
		response.JSON(w, r, http.StatusOK, out)
		return
	}
	if !h.allow(w, r, "help_search", clientIP(r), helpSearchesPerMinute) {
		return
	}
	locale := r.URL.Query().Get("locale")
	if locale != "en" {
		locale = "tr"
	}
	org := wd.Instance.OrganizationID
	slug, available := h.help.HasHelpCenter(r.Context(), org)
	if !available {
		response.JSON(w, r, http.StatusOK, out)
		return
	}
	out.Available = true
	base := h.helpOrigin + "/help/" + url.PathEscape(slug)
	home := base
	if locale != "tr" {
		home += "?lang=" + locale
	}
	out.HelpCenterURL = &home
	var (
		items []helpusecase.ArticleSummary
		err   error
	)
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		items, err = h.help.SearchOrganization(r.Context(), org, q, locale, 8)
	} else {
		items, err = h.help.RecentOrganization(r.Context(), org, locale, 5)
	}
	if errors.Is(err, helpusecase.ErrInvalid) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid search")
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ids := make([]uuid.UUID, 0, len(items))
	for _, a := range items {
		ids = append(ids, a.UUID)
	}
	hidden, err := h.svc.HiddenArticles(r.Context(), wd, bearer(r), ids)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	for _, a := range items {
		if hidden[a.UUID] {
			continue
		}
		link := base + "/article/" + url.PathEscape(a.Slug)
		if a.Locale != "tr" {
			link += "?lang=" + a.Locale
		}
		out.Items = append(out.Items, HelpArticle{Title: a.Title, Excerpt: a.Excerpt, URL: link})
	}
	response.JSON(w, r, http.StatusOK, out)
}
