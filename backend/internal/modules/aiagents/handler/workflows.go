package handler

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/usecase"
	kbhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func scope(r *http.Request) (int64, int64) {
	return orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal
}
func writeResult(w http.ResponseWriter, r *http.Request, value any, err error) {
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, value)
}
func writePage(w http.ResponseWriter, r *http.Request, items any, total int64, q apiquery.Query, err error) {
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"items": items, "total": total, "limit": q.Limit, "offset": q.Offset})
}

func (h *Handler) BusinessContext(w http.ResponseWriter, r *http.Request) {
	org, actor := scope(r)
	if r.Method == http.MethodGet {
		q, ok := listQuery(w, r)
		if !ok {
			return
		}
		rows, err := h.Service.Repo.Q.BusinessContextList(r.Context(), db.BusinessContextListParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			fail(w, r, err)
			return
		}
		total, err := h.Service.Repo.Q.BusinessContextCount(r.Context(), org)
		writePage(w, r, rows, total, q, err)
		return
	}
	var id uuid.UUID
	if r.Method == http.MethodPut {
		var ok bool
		id, ok = conversationID(w, r)
		if !ok {
			return
		}
	}
	var in usecase.BusinessContextInput
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	row, err := h.Service.SaveBusinessContext(r.Context(), org, actor, id, in)
	if err == nil {
		h.audit(r, "business_context.saved")
	}
	writeResult(w, r, row, err)
}
func (h *Handler) BusinessContextVersions(w http.ResponseWriter, r *http.Request) {
	org, _ := scope(r)
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	q, ok := listQuery(w, r)
	if !ok {
		return
	}
	rows, err := h.Service.Repo.Q.BusinessContextVersions(r.Context(), db.BusinessContextVersionsParams{OrganizationID: org, Uuid: id, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		fail(w, r, err)
		return
	}
	total, err := h.Service.Repo.Q.BusinessContextVersionCount(r.Context(), db.BusinessContextVersionCountParams{OrganizationID: org, Uuid: id})
	writePage(w, r, rows, total, q, err)
}
func (h *Handler) Simulation(w http.ResponseWriter, r *http.Request) {
	org, actor := scope(r)
	var in usecase.SimulationInput
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	out, err := h.Service.Simulate(r.Context(), org, actor, in)
	if err == nil {
		h.audit(r, "simulation.completed")
	}
	writeResult(w, r, out, err)
}
func (h *Handler) EvaluationCases(w http.ResponseWriter, r *http.Request) {
	org, actor := scope(r)
	if r.Method == http.MethodGet {
		q, ok := listQuery(w, r)
		if !ok {
			return
		}
		dept, ok := h.departmentFilter(w, r, org)
		if !ok {
			return
		}
		rows, err := h.Service.Repo.Q.AIEvaluationCases(r.Context(), db.AIEvaluationCasesParams{OrganizationID: org, DepartmentID: dept, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			fail(w, r, err)
			return
		}
		views, err := h.caseViews(r.Context(), org, rows)
		if err != nil {
			fail(w, r, err)
			return
		}
		total, err := h.Service.Repo.Q.AIEvaluationCaseCount(r.Context(), db.AIEvaluationCaseCountParams{OrganizationID: org, DepartmentID: dept})
		writePage(w, r, views, total, q, err)
		return
	}
	var in usecase.EvaluationCaseInput
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	out, err := h.Service.SaveEvaluationCase(r.Context(), org, actor, in)
	if err == nil {
		h.audit(r, "evaluation_case.created")
	}
	h.oneCase(w, r, org, out, err)
}
func (h *Handler) Evaluations(w http.ResponseWriter, r *http.Request) {
	org, actor := scope(r)
	if r.Method == http.MethodGet {
		q, ok := listQuery(w, r)
		if !ok {
			return
		}
		rows, err := h.Service.Repo.Q.AIEvaluationList(r.Context(), db.AIEvaluationListParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			fail(w, r, err)
			return
		}
		views, err := h.evaluationViews(r.Context(), org, rows)
		if err != nil {
			fail(w, r, err)
			return
		}
		total, err := h.Service.Repo.Q.AIEvaluationCount(r.Context(), org)
		writePage(w, r, views, total, q, err)
		return
	}
	var in usecase.EvaluationInput
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	out, err := h.Service.Evaluate(r.Context(), org, actor, in)
	if err == nil {
		h.audit(r, "evaluation.completed")
	}
	h.oneEvaluation(w, r, org, out, err)
}
func (h *Handler) Evaluation(w http.ResponseWriter, r *http.Request) {
	org, _ := scope(r)
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	out, err := h.Service.Repo.Q.AIEvaluationGet(r.Context(), db.AIEvaluationGetParams{OrganizationID: org, Uuid: id})
	h.oneEvaluation(w, r, org, out, err)
}
func (h *Handler) EvaluationResults(w http.ResponseWriter, r *http.Request) {
	org, _ := scope(r)
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	q, ok := listQuery(w, r)
	if !ok {
		return
	}
	rows, err := h.Service.Repo.Q.AIEvaluationResults(r.Context(), db.AIEvaluationResultsParams{OrganizationID: org, EvaluationUuid: id, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		fail(w, r, err)
		return
	}
	views, err := h.resultViews(r.Context(), org, rows)
	if err != nil {
		fail(w, r, err)
		return
	}
	total, err := h.Service.Repo.Q.AIEvaluationResultCount(r.Context(), db.AIEvaluationResultCountParams{OrganizationID: org, EvaluationUuid: id})
	writePage(w, r, views, total, q, err)
}
func (h *Handler) KnowledgeGaps(w http.ResponseWriter, r *http.Request) {
	org, _ := scope(r)
	q, ok := listQuery(w, r)
	if !ok {
		return
	}
	since := h.Service.Now().AddDate(0, 0, -30)
	if raw := r.URL.Query().Get("since"); raw != "" {
		var err error
		since, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			fail(w, r, model.ErrInvalid)
			return
		}
	}
	if _, _, err := usecase.TimeRange(h.Service.Now(), since, h.Service.Now()); err != nil {
		fail(w, r, err)
		return
	}
	from := pgtype.Timestamptz{Time: since, Valid: true}
	rows, err := h.Service.Repo.Q.AIKnowledgeGapTopics(r.Context(), db.AIKnowledgeGapTopicsParams{OrganizationID: org, Since: from, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		fail(w, r, err)
		return
	}
	total, err := h.Service.Repo.Q.AIKnowledgeGapTopicCount(r.Context(), db.AIKnowledgeGapTopicCountParams{OrganizationID: org, Since: from})
	writePage(w, r, rows, total, q, err)
}
func (h *Handler) KnowledgeSuggestions(w http.ResponseWriter, r *http.Request) {
	org, _ := scope(r)
	q, ok := listQuery(w, r)
	if !ok {
		return
	}
	rows, err := h.Service.Repo.Q.AIKnowledgeSuggestions(r.Context(), db.AIKnowledgeSuggestionsParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		fail(w, r, err)
		return
	}
	total, err := h.Service.Repo.Q.AIKnowledgeSuggestionCount(r.Context(), org)
	writePage(w, r, rows, total, q, err)
}
func (h *Handler) SuggestKnowledge(w http.ResponseWriter, r *http.Request) {
	org, actor := scope(r)
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	var in struct {
		Locale string `json:"locale"`
	}
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	out, err := h.Service.SuggestKnowledge(r.Context(), org, actor, id, in.Locale)
	if err == nil {
		h.audit(r, "knowledge.suggested")
	}
	writeResult(w, r, out, err)
}
func (h *Handler) ReviewSuggestion(w http.ResponseWriter, r *http.Request) {
	org, actor := scope(r)
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	var in struct {
		Approve bool   `json:"approve"`
		Locale  string `json:"locale"`
	}
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	out, err := h.Service.ReviewSuggestion(r.Context(), org, actor, id, in.Approve, in.Locale)
	if err == nil {
		h.audit(r, "suggestion.reviewed")
	}
	writeResult(w, r, out, err)
}
func (h *Handler) TranslateArticle(w http.ResponseWriter, r *http.Request) {
	org, actor := scope(r)
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	var in struct {
		Locale string `json:"locale"`
	}
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	out, err := h.Service.TranslateArticle(r.Context(), org, actor, id, in.Locale)
	if err == nil {
		h.audit(r, "article.translation_drafted")
	}
	writeResult(w, r, out, err)
}
func (h *Handler) KnowledgeReviews(w http.ResponseWriter, r *http.Request) {
	org, _ := scope(r)
	q, ok := listQuery(w, r)
	if !ok {
		return
	}
	rows, err := h.Service.Repo.Q.KnowledgeReviews(r.Context(), db.KnowledgeReviewsParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		fail(w, r, err)
		return
	}
	total, err := h.Service.Repo.Q.KnowledgeReviewCount(r.Context(), org)
	writePage(w, r, rows, total, q, err)
}
func (h *Handler) ReviewDone(w http.ResponseWriter, r *http.Request) {
	org, _ := scope(r)
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	out, err := h.Service.Repo.Q.KnowledgeReviewDone(r.Context(), db.KnowledgeReviewDoneParams{OrganizationID: org, Uuid: id})
	if err == nil {
		h.audit(r, "article.review_completed")
	}
	writeResult(w, r, out, err)
}
func (h *Handler) SweepReviews(w http.ResponseWriter, r *http.Request) {
	org, _ := scope(r)
	var in struct {
		Months int `json:"months"`
	}
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	err := h.Service.SweepKnowledgeReviews(r.Context(), org, in.Months)
	if err == nil {
		h.audit(r, "article.staleness_checked")
	}
	writeResult(w, r, map[string]bool{"checked": err == nil}, err)
}
func (h *Handler) FlagArticle(w http.ResponseWriter, r *http.Request) {
	org, _ := scope(r)
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
		Detail string `json:"detail"`
	}
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	out, err := h.Service.FlagArticle(r.Context(), org, id, in.Reason, in.Detail)
	if err == nil {
		h.audit(r, "article.flagged")
	}
	writeResult(w, r, out, err)
}

func (h *Handler) Sources(w http.ResponseWriter, r *http.Request) {
	org, actor := scope(r)
	if r.Method == http.MethodGet {
		q, ok := listQuery(w, r)
		if !ok {
			return
		}
		rows, err := h.Service.Repo.Q.AIKnowledgeSources(r.Context(), db.AIKnowledgeSourcesParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			fail(w, r, err)
			return
		}
		total, err := h.Service.Repo.Q.AIKnowledgeSourceCount(r.Context(), org)
		writePage(w, r, rows, total, q, err)
		return
	}
	var in usecase.SourceInput
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	row, err := h.Service.Sources.CreateURL(r.Context(), org, actor, in)
	if err == nil {
		h.audit(r, "source.created")
	}
	writeResult(w, r, map[string]any{"uuid": row.Uuid, "status": row.Status}, err)
}
func (h *Handler) SourceUpload(w http.ResponseWriter, r *http.Request) {
	org, actor := scope(r)
	r.Body = http.MaxBytesReader(w, r.Body, ioengine.MaxKnowledgeFileBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		fail(w, r, model.ErrInvalid)
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	file, header, err := r.FormFile("file")
	if err != nil {
		fail(w, r, model.ErrInvalid)
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, ioengine.MaxKnowledgeFileBytes+1))
	if err != nil {
		fail(w, r, model.ErrInvalid)
		return
	}
	row, err := h.Service.Sources.Upload(r.Context(), org, actor, usecase.SourceInput{Title: r.FormValue("title"), Locale: r.FormValue("locale"), IntervalHours: 24}, header.Filename, data)
	if err == nil {
		h.audit(r, "source.uploaded")
	}
	writeResult(w, r, map[string]any{"uuid": row.Uuid, "status": row.Status}, err)
}
func (h *Handler) SourceSync(w http.ResponseWriter, r *http.Request) {
	org, _ := scope(r)
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	err := h.Service.Sources.Sync(r.Context(), org, id)
	if err == nil {
		h.audit(r, "source.synced")
	}
	writeResult(w, r, map[string]bool{"synced": err == nil}, err)
}
func (h *Handler) SourceToggle(w http.ResponseWriter, r *http.Request) {
	org, _ := scope(r)
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	out, err := h.Service.Repo.Q.AIKnowledgeSourceToggle(r.Context(), db.AIKnowledgeSourceToggleParams{OrganizationID: org, Uuid: id, Enabled: in.Enabled})
	if err == nil {
		h.audit(r, "source.toggled")
	}
	writeResult(w, r, map[string]any{"uuid": out}, err)
}
func (h *Handler) Performance(w http.ResponseWriter, r *http.Request) {
	org, _ := scope(r)
	from, to := time.Time{}, time.Time{}
	var err error
	if raw := r.URL.Query().Get("since"); raw != "" {
		from, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			fail(w, r, model.ErrInvalid)
			return
		}
	}
	if raw := r.URL.Query().Get("until"); raw != "" {
		to, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			fail(w, r, model.ErrInvalid)
			return
		}
	}
	minutes := float64(10)
	if raw := r.URL.Query().Get("minutes_per_resolution"); raw != "" {
		minutes, err = strconv.ParseFloat(raw, 64)
		if err != nil {
			fail(w, r, model.ErrInvalid)
			return
		}
	}
	out, err := h.Service.Performance(r.Context(), org, from, to, minutes)
	writeResult(w, r, out, err)
}

func (h *Handler) WorkflowMeta(w http.ResponseWriter, r *http.Request) {
	resource := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/tenant/"), "/meta")
	capabilities := resourcemeta.Capabilities{Read: true}
	switch resource {
	case "ai/business-context", "knowledge/sources":
		capabilities.Create = true
		capabilities.Update = true
	case "ai/evaluation-cases":
		capabilities.Create = true
		capabilities.Update = true
	case "ai/evaluations":
		capabilities.Create = true
	case "ai/knowledge-suggestions", "knowledge/reviews":
		capabilities.Update = true
	}
	response.JSON(w, r, 200, resourcemeta.ResourceMeta{Resource: "tenant." + strings.ReplaceAll(resource, "/", "."), DefaultSort: "", DefaultFields: []string{"uuid", "created_at"}, Capabilities: capabilities, SearchableFields: []string{}, SortableFields: []string{}, FilterableFields: []string{}, Columns: []resourcemeta.Column{}, Filters: []resourcemeta.Filter{}, Includes: []string{}})
}

func (h *Handler) CheckContradiction(w http.ResponseWriter, r *http.Request) {
	org, actor := scope(r)
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	var in struct {
		Other uuid.UUID `json:"other_article_uuid"`
	}
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	review, err := h.Service.CheckArticleContradiction(r.Context(), org, actor, id, in.Other)
	if err == nil {
		h.audit(r, "article.contradiction_checked")
	}
	writeResult(w, r, map[string]any{"review": review}, err)
}
