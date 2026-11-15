// Package handler serves the CF-216 knowledge layer: the agent assistant's
// memory search, case memory review, knowledge units, cache hit rate and
// unanswered questions.
package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aimemory/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aimemory/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	kbhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Handler struct {
	Service  *usecase.Service
	Activity *activity.Recorder
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, model.ErrInvalid):
		response.BadRequest(w, r, "VALIDATION_ERROR", "Invalid AI memory input")
	case errors.Is(err, model.ErrNotFound):
		response.NotFound(w, r, "")
	case errors.Is(err, model.ErrConflict):
		response.Conflict(w, r, "CONFLICT", "The case summary is not anonymized and cannot be approved")
	default:
		response.InternalErr(w, r, err, "")
	}
}

func page(w http.ResponseWriter, r *http.Request) (apiquery.Query, bool) {
	values := r.URL.Query()
	q := apiquery.Parse(values)
	for key := range values {
		if key != "limit" && key != "offset" && key != "since" && key != "status" {
			response.ValidationError(w, r, []response.Detail{{Field: key, Code: "unsupported", Message: "Unsupported query parameter"}})
			return q, false
		}
	}
	return q, true
}

func since(w http.ResponseWriter, r *http.Request, now time.Time) (time.Time, bool) {
	from := now.AddDate(0, 0, -30)
	if raw := r.URL.Query().Get("since"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil || t.After(now) || now.Sub(t) > 366*24*time.Hour {
			fail(w, r, model.ErrInvalid)
			return from, false
		}
		from = t
	}
	return from, true
}

func items(w http.ResponseWriter, r *http.Request, list any, total int64, q apiquery.Query, err error) {
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"items": list, "total": total, "limit": q.Limit, "offset": q.Offset})
}

type searchInput struct {
	Question         string     `json:"question"`
	Locale           string     `json:"locale"`
	ConversationUUID *uuid.UUID `json:"conversation_uuid"`
}

// Search is the agent assistant's view of the knowledge layer. Case links are
// returned only for conversations the agent may open.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	principal := authctx.MustPrincipal(r.Context())
	var in searchInput
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	in.Question = strings.TrimSpace(in.Question)
	if in.Locale == "" {
		in.Locale = "tr"
	}
	if in.Question == "" || len(in.Question) > 4000 {
		fail(w, r, model.ErrInvalid)
		return
	}
	viewer, err := access.For(r.Context(), h.Service.Q, scope, principal.UserInternal)
	if err != nil {
		fail(w, r, err)
		return
	}
	q := model.Query{Organization: scope.ID, Audience: model.AudienceAgent, Locale: in.Locale, Question: in.Question}
	if in.ConversationUUID != nil {
		conv, err := h.Service.Q.CaseMemoryConversation(r.Context(), db.CaseMemoryConversationParams{OrganizationID: scope.ID, Uuid: *in.ConversationUUID})
		if err != nil {
			fail(w, r, model.ErrNotFound)
			return
		}
		if ok, err := viewer.CanSee(r.Context(), h.Service.Q, scope.ID, conv.ID); err != nil || !ok {
			fail(w, r, model.ErrNotFound)
			return
		}
		q.ExcludeConversationID = conv.ID
		if conv.BrandID.Valid {
			brand := uuid.UUID(conv.BrandID.Bytes)
			q.Scope.BrandID = &brand
		}
	}
	out, err := h.Service.Lookup(r.Context(), q)
	if err != nil {
		fail(w, r, err)
		return
	}
	canRead := access.CanRead(scope)
	for i := range out.AgentCases {
		ok := false
		if canRead {
			ok, _ = viewer.CanSee(r.Context(), h.Service.Q, scope.ID, out.AgentCases[i].SourceConversationID)
		}
		if !ok {
			out.AgentCases[i].ConversationUUID = nil
		}
	}
	if out.AgentCases == nil {
		out.AgentCases = []model.AgentCase{}
	}
	response.JSON(w, r, 200, out)
}

func (h *Handler) Cases(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	q, ok := page(w, r)
	if !ok {
		return
	}
	status := pgtype.Text{}
	if raw := r.URL.Query().Get("status"); raw != "" {
		if raw != "pending" && raw != "approved" && raw != "rejected" {
			fail(w, r, model.ErrInvalid)
			return
		}
		status = pgtype.Text{String: raw, Valid: true}
	}
	rows, err := h.Service.Q.CaseMemoryList(r.Context(), db.CaseMemoryListParams{OrganizationID: org, Status: status, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		fail(w, r, err)
		return
	}
	total, err := h.Service.Q.CaseMemoryCount(r.Context(), db.CaseMemoryCountParams{OrganizationID: org, Status: status})
	items(w, r, rows, total, q, err)
}

type caseView struct {
	UUID       uuid.UUID `json:"uuid"`
	Status     string    `json:"status"`
	Anonymized bool      `json:"anonymized"`
	Problem    string    `json:"problem"`
	Resolution string    `json:"resolution"`
	Category   string    `json:"category"`
}

func (h *Handler) ReviewCase(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		fail(w, r, model.ErrInvalid)
		return
	}
	var in struct {
		Approve bool `json:"approve"`
	}
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	row, err := h.Service.Review(r.Context(), org, actor, id, in.Approve)
	if err != nil {
		fail(w, r, err)
		return
	}
	if h.Activity != nil {
		h.Activity.Record(r.Context(), &actor, "organizations.updated", "organizations", nil, map[string]any{"area": "ai_memory", "operation": "case_memory." + row.Status, "case_uuid": row.Uuid.String()}, r)
	}
	response.JSON(w, r, 200, caseView{UUID: row.Uuid, Status: row.Status, Anonymized: row.Anonymized, Problem: row.Problem, Resolution: row.Resolution, Category: row.Category})
}

func (h *Handler) Units(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	q, ok := page(w, r)
	if !ok {
		return
	}
	rows, err := h.Service.Q.KnowledgeUnitList(r.Context(), db.KnowledgeUnitListParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		fail(w, r, err)
		return
	}
	total, err := h.Service.Q.KnowledgeUnitCount(r.Context(), org)
	items(w, r, rows, total, q, err)
}

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	from, ok := since(w, r, h.Service.Now())
	if !ok {
		return
	}
	out, err := h.Service.Stats(r.Context(), org, from)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}

func (h *Handler) GapQuestions(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	q, ok := page(w, r)
	if !ok {
		return
	}
	from, ok := since(w, r, h.Service.Now())
	if !ok {
		return
	}
	at := pgtype.Timestamptz{Time: from, Valid: true}
	rows, err := h.Service.Q.KnowledgeGapQuestions(r.Context(), db.KnowledgeGapQuestionsParams{OrganizationID: org, Since: at, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		fail(w, r, err)
		return
	}
	total, err := h.Service.Q.KnowledgeGapQuestionCount(r.Context(), db.KnowledgeGapQuestionCountParams{OrganizationID: org, Since: at})
	items(w, r, rows, total, q, err)
}
