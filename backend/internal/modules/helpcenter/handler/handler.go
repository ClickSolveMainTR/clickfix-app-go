// Package handler serves the public help center API (CF-74):
// /v1/public/help/{organization_slug}/... without a session. Next.js renders
// the pages server-side from these routes; the browser only posts feedback
// and loads the logo through the same-origin BFF.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/helpcenter/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// Error codes of the help center API.
const (
	CodeNotFound = "HELP_CENTER_NOT_FOUND"
)

// Rate limits (per client IP unless noted).
const (
	requestsPerMinute       = 240
	searchesPerMinute       = 30
	feedbackPerArticleDaily = 3
	feedbackPerHour         = 30
)

// Limit throttles by action and subject within a window. nil disables limits.
type Limit func(r *http.Request, action, subject string, limit int, window time.Duration) (bool, time.Duration)

// Handler serves the help center routes.
type Handler struct {
	svc   *usecase.Service
	limit Limit
	log   *slog.Logger
}

// New builds the handler.
func New(svc *usecase.Service, limit Limit, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{svc: svc, limit: limit, log: log}
}

// ClientIP is the left-most X-Forwarded-For hop (set by the BFF from the
// edge proxy), else the remote address.
func ClientIP(r *http.Request) string {
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

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, action, subject string, limit int, window time.Duration) bool {
	if h.limit == nil {
		return true
	}
	ok, wait := h.limit(r, "help_"+action, subject, limit, window)
	if !ok {
		if wait > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		}
		response.TooManyRequests(w, r, "Too many requests, please slow down")
		return false
	}
	return true
}

func (h *Handler) open(w http.ResponseWriter, r *http.Request) bool {
	return h.allow(w, r, "request", ClientIP(r), requestsPerMinute, time.Minute)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.Error(w, r, http.StatusNotFound, CodeNotFound, "Help center page not found")
	case errors.Is(err, usecase.ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Field: "input", Code: "invalid", Message: "Invalid help center request"}})
	default:
		h.log.ErrorContext(r.Context(), "help center", "err", err)
		response.InternalErr(w, r, err, "")
	}
}

func org(r *http.Request) string { return r.PathValue("organization_slug") }

func locale(r *http.Request) string { return r.URL.Query().Get("locale") }

// Home is GET /v1/public/help/{organization_slug}.
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	if !h.open(w, r) {
		return
	}
	out, err := h.svc.Home(r.Context(), org(r), locale(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Collection is GET /v1/public/help/{organization_slug}/collections/{collection_slug}.
func (h *Handler) Collection(w http.ResponseWriter, r *http.Request) {
	if !h.open(w, r) {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	out, err := h.svc.Collection(r.Context(), org(r), r.PathValue("collection_slug"), locale(r), q.Limit, q.Offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Article is GET /v1/public/help/{organization_slug}/articles/{article_slug}.
func (h *Handler) Article(w http.ResponseWriter, r *http.Request) {
	if !h.open(w, r) {
		return
	}
	out, err := h.svc.Article(r.Context(), org(r), r.PathValue("article_slug"), locale(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// SearchResult is the help center search payload.
type SearchResult struct {
	Organization usecase.Organization     `json:"organization"`
	Query        string                   `json:"query"`
	Items        []usecase.ArticleSummary `json:"items"`
}

// Search is GET /v1/public/help/{organization_slug}/search?q=.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	if !h.open(w, r) || !h.allow(w, r, "search", ClientIP(r), searchesPerMinute, time.Minute) {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	o, items, err := h.svc.Search(r.Context(), org(r), q, locale(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, SearchResult{Organization: o, Query: q, Items: items})
}

// Sitemap is GET /v1/public/help/{organization_slug}/sitemap.
func (h *Handler) Sitemap(w http.ResponseWriter, r *http.Request) {
	if !h.open(w, r) {
		return
	}
	out, err := h.svc.Sitemap(r.Context(), org(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// FeedbackInput is the "Bu makale işe yaradı mı?" vote.
type FeedbackInput struct {
	Helpful *bool  `json:"helpful"`
	Locale  string `json:"locale"`
}

// Feedback is POST /v1/public/help/{organization_slug}/articles/{article_slug}/feedback.
func (h *Handler) Feedback(w http.ResponseWriter, r *http.Request) {
	if !h.open(w, r) {
		return
	}
	var in FeedbackInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || in.Helpful == nil {
		response.ValidationError(w, r, []response.Detail{{Field: "helpful", Code: "required", Message: "helpful must be true or false"}})
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return
	}
	ip := ClientIP(r)
	articleSlug := r.PathValue("article_slug")
	if !h.allow(w, r, "feedback", ip, feedbackPerHour, time.Hour) ||
		!h.allow(w, r, "feedback_article", ip+"|"+usecase.ArticleKey(org(r), articleSlug, in.Locale), feedbackPerArticleDaily, 24*time.Hour) {
		return
	}
	if err := h.svc.Feedback(r.Context(), org(r), articleSlug, in.Locale, ip, *in.Helpful); err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]bool{"recorded": true})
}

// Logo is GET /v1/public/help/{organization_slug}/logo.
func (h *Handler) Logo(w http.ResponseWriter, r *http.Request) {
	if !h.open(w, r) {
		return
	}
	rc, key, err := h.svc.Logo(r.Context(), org(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", storage.MIMEFromLogoKey(key))
	// The URL carries a version derived from the object key.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, io.LimitReader(rc, 8<<20))
}
