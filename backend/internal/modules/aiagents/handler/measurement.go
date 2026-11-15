package handler

// CF-220: evaluation set, run history and automatic run views. JSONB
// columns are returned as JSON, internal department ids as public UUIDs.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/usecase"
	kbhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type evaluationCaseView struct {
	db.AiEvaluationCase
	OrganizationID       *struct{}       `json:"organization_id,omitempty"`
	DepartmentID         *struct{}       `json:"department_id,omitempty"`
	DepartmentUUID       *uuid.UUID      `json:"department_uuid"`
	Expected             json.RawMessage `json:"expected"`
	ExpectedArticleUuids []uuid.UUID     `json:"expected_article_uuids"`
}

type evaluationView struct {
	db.AiEvaluation
	OrganizationID *struct{}       `json:"organization_id,omitempty"`
	DepartmentID   *struct{}       `json:"department_id,omitempty"`
	DepartmentUUID *uuid.UUID      `json:"department_uuid"`
	Config         json.RawMessage `json:"config"`
	Report         json.RawMessage `json:"report"`
	Fingerprint    json.RawMessage `json:"fingerprint"`
}

type evaluationResultView struct {
	db.AiEvaluationResult
	OrganizationID *struct{}       `json:"organization_id,omitempty"`
	DepartmentID   *struct{}       `json:"department_id,omitempty"`
	DepartmentUUID *uuid.UUID      `json:"department_uuid"`
	CaseSnapshot   json.RawMessage `json:"case_snapshot"`
	Output         json.RawMessage `json:"output"`
}

// departments maps the internal department ids of rows to public UUIDs.
func (h *Handler) departments(ctx context.Context, org int64, ids []pgtype.Int8) (func(pgtype.Int8) *uuid.UUID, error) {
	want := []int64{}
	for _, id := range ids {
		if id.Valid {
			want = append(want, id.Int64)
		}
	}
	m, err := h.Service.DepartmentUUIDs(ctx, org, want)
	if err != nil {
		return nil, err
	}
	return func(id pgtype.Int8) *uuid.UUID {
		if u, ok := m[id.Int64]; ok && id.Valid {
			return &u
		}
		return nil
	}, nil
}

func (h *Handler) caseViews(ctx context.Context, org int64, rows []db.AiEvaluationCase) ([]evaluationCaseView, error) {
	ids := make([]pgtype.Int8, len(rows))
	for i, r := range rows {
		ids[i] = r.DepartmentID
	}
	dept, err := h.departments(ctx, org, ids)
	if err != nil {
		return nil, err
	}
	out := make([]evaluationCaseView, len(rows))
	for i, r := range rows {
		articles := r.ExpectedArticleUuids
		if articles == nil {
			articles = []uuid.UUID{}
		}
		out[i] = evaluationCaseView{AiEvaluationCase: r, DepartmentUUID: dept(r.DepartmentID), Expected: rawJSON(r.Expected), ExpectedArticleUuids: articles}
	}
	return out, nil
}

func (h *Handler) evaluationViews(ctx context.Context, org int64, rows []db.AiEvaluation) ([]evaluationView, error) {
	ids := make([]pgtype.Int8, len(rows))
	for i, r := range rows {
		ids[i] = r.DepartmentID
	}
	dept, err := h.departments(ctx, org, ids)
	if err != nil {
		return nil, err
	}
	out := make([]evaluationView, len(rows))
	for i, r := range rows {
		out[i] = evaluationView{AiEvaluation: r, DepartmentUUID: dept(r.DepartmentID), Config: rawJSON(r.Config), Report: rawJSON(r.Report), Fingerprint: rawJSON(r.Fingerprint)}
	}
	return out, nil
}

func (h *Handler) resultViews(ctx context.Context, org int64, rows []db.AiEvaluationResult) ([]evaluationResultView, error) {
	ids := make([]pgtype.Int8, len(rows))
	for i, r := range rows {
		ids[i] = r.DepartmentID
	}
	dept, err := h.departments(ctx, org, ids)
	if err != nil {
		return nil, err
	}
	out := make([]evaluationResultView, len(rows))
	for i, r := range rows {
		out[i] = evaluationResultView{AiEvaluationResult: r, DepartmentUUID: dept(r.DepartmentID), CaseSnapshot: rawJSON(r.CaseSnapshot), Output: rawJSON(r.Output)}
	}
	return out, nil
}

func (h *Handler) oneCase(w http.ResponseWriter, r *http.Request, org int64, row db.AiEvaluationCase, err error) {
	if err != nil {
		fail(w, r, err)
		return
	}
	views, err := h.caseViews(r.Context(), org, []db.AiEvaluationCase{row})
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, views[0])
}

func (h *Handler) oneEvaluation(w http.ResponseWriter, r *http.Request, org int64, row db.AiEvaluation, err error) {
	if err != nil {
		fail(w, r, err)
		return
	}
	views, err := h.evaluationViews(r.Context(), org, []db.AiEvaluation{row})
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, views[0])
}

// departmentFilter parses ?department_uuid= into an internal id.
func (h *Handler) departmentFilter(w http.ResponseWriter, r *http.Request, org int64) (pgtype.Int8, bool) {
	raw := r.URL.Query().Get("department_uuid")
	if raw == "" {
		return pgtype.Int8{}, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		fail(w, r, model.ErrInvalid)
		return pgtype.Int8{}, false
	}
	dept, err := h.Service.ResolveDepartment(r.Context(), org, &id)
	if err != nil {
		fail(w, r, err)
		return pgtype.Int8{}, false
	}
	return pgtype.Int8{Int64: *dept, Valid: true}, true
}

// UpdateEvaluationCase replaces a case of the set (PUT, bumps revision).
func (h *Handler) UpdateEvaluationCase(w http.ResponseWriter, r *http.Request) {
	org, _ := scope(r)
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	var in usecase.EvaluationCaseInput
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	row, err := h.Service.UpdateEvaluationCase(r.Context(), org, id, in)
	if err == nil {
		h.audit(r, "evaluation_case.updated")
	}
	h.oneCase(w, r, org, row, err)
}

// EvaluationWatch reads (GET) or sets (PUT) automatic runs.
func (h *Handler) EvaluationWatch(w http.ResponseWriter, r *http.Request) {
	org, actor := scope(r)
	if r.Method == http.MethodGet {
		out, err := h.Service.EvaluationWatch(r.Context(), org)
		writeResult(w, r, out, err)
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	out, err := h.Service.SetEvaluationWatch(r.Context(), org, actor, in.Enabled)
	if err == nil {
		h.audit(r, "evaluation_watch.updated")
	}
	writeResult(w, r, out, err)
}

func departmentsUnavailable(w http.ResponseWriter, r *http.Request, err error) bool {
	if errors.Is(err, usecase.ErrDepartmentsUnavailable) {
		response.BadRequest(w, r, "DEPARTMENTS_UNAVAILABLE", "Departments are not available yet")
		return true
	}
	return false
}
