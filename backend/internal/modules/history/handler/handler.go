// Package handler is the HTTP surface of the history pipeline (CF-217).
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/history/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/history/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxBody = 128 << 10

type Handler struct {
	Svc      *usecase.Service
	Activity *activity.Recorder
	// Sample is the documented example file served for download.
	Sample []byte
}

// ImportView is a run as the panel shows it: progress, consent and whether
// the raw file is gone. Internal ids are never exposed.
type ImportView struct {
	UUID                  uuid.UUID          `json:"uuid"`
	Source                string             `json:"source"`
	Connector             string             `json:"connector"`
	FileName              string             `json:"file_name"`
	Status                string             `json:"status"`
	Stage                 string             `json:"stage"`
	TotalRecords          int32              `json:"total_records"`
	ProcessedRecords      int32              `json:"processed_records"`
	SkippedRecords        int32              `json:"skipped_records"`
	ClustersTouched       int32              `json:"clusters_touched"`
	SuggestionsCreated    int32              `json:"suggestions_created"`
	SuggestionsSuppressed int32              `json:"suggestions_suppressed"`
	ConsentAnalysis       bool               `json:"consent_analysis"`
	ConsentTraining       bool               `json:"consent_training"`
	ConsentAt             pgtype.Timestamptz `json:"consent_at"`
	ConsentVersion        string             `json:"consent_version"`
	RawDeleted            bool               `json:"raw_deleted"`
	RawRetainUntil        pgtype.Timestamptz `json:"raw_retain_until"`
	BrandUUID             pgtype.UUID        `json:"brand_uuid"`
	DepartmentID          pgtype.Int8        `json:"department_id"`
	WindowStart           pgtype.Timestamptz `json:"window_start"`
	WindowEnd             pgtype.Timestamptz `json:"window_end"`
	CancelRequested       bool               `json:"cancel_requested"`
	Error                 string             `json:"error"`
	StartedAt             pgtype.Timestamptz `json:"started_at"`
	FinishedAt            pgtype.Timestamptz `json:"finished_at"`
	CreatedAt             pgtype.Timestamptz `json:"created_at"`
}

func importView(r db.HistoryImport) ImportView {
	return ImportView{UUID: r.Uuid, Source: r.Source, Connector: r.Connector, FileName: r.FileName, Status: r.Status, Stage: r.Stage, TotalRecords: r.TotalRecords,
		ProcessedRecords: r.ProcessedRecords, SkippedRecords: r.SkippedRecords, ClustersTouched: r.ClustersTouched, SuggestionsCreated: r.SuggestionsCreated,
		SuggestionsSuppressed: r.SuggestionsSuppressed, ConsentAnalysis: r.ConsentAnalysis, ConsentTraining: r.ConsentTraining, ConsentAt: r.ConsentAt,
		ConsentVersion: r.ConsentVersion, RawDeleted: r.RawKey == "", RawRetainUntil: r.RawRetainUntil, BrandUUID: r.BrandID, DepartmentID: r.DepartmentID,
		WindowStart: r.WindowStart, WindowEnd: r.WindowEnd, CancelRequested: r.CancelRequested, Error: r.Error, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, CreatedAt: r.CreatedAt}
}

type SuggestionView struct {
	UUID                 uuid.UUID          `json:"uuid"`
	Topic                string             `json:"topic"`
	Title                string             `json:"title"`
	Body                 string             `json:"body"`
	Status               string             `json:"status"`
	Source               string             `json:"source"`
	EvidenceCount        int32              `json:"evidence_count"`
	ClusterUUID          pgtype.UUID        `json:"cluster_uuid"`
	ClusterSize          pgtype.Int4        `json:"cluster_size"`
	DepartmentID         pgtype.Int8        `json:"department_id"`
	ConflictArticleUUID  pgtype.UUID        `json:"conflict_article_uuid"`
	ConflictArticleTitle pgtype.Text        `json:"conflict_article_title"`
	ConflictScore        float32            `json:"conflict_score"`
	CreatedAt            pgtype.Timestamptz `json:"created_at"`
	UpdatedAt            pgtype.Timestamptz `json:"updated_at"`
}

type ClusterView struct {
	db.HistoryClusterListRow
	SuggestionUuid   *string `json:"suggestion_uuid"`
	SuggestionStatus *string `json:"suggestion_status"`
}

type SettingsView struct {
	EvidenceThreshold      int32              `json:"evidence_threshold"`
	ClusterSimilarity      float32            `json:"cluster_similarity"`
	ResolutionSimilarity   float32            `json:"resolution_similarity"`
	RejectionSimilarity    float32            `json:"rejection_similarity"`
	RawRetentionHours      int32              `json:"raw_retention_hours"`
	ArchiveRetentionDays   int32              `json:"archive_retention_days"`
	NightlyEnabled         bool               `json:"nightly_enabled"`
	NightlyConsentAnalysis bool               `json:"nightly_consent_analysis"`
	NightlyConsentTraining bool               `json:"nightly_consent_training"`
	NightlyWatermark       pgtype.Timestamptz `json:"nightly_watermark"`
	UpdatedAt              pgtype.Timestamptz `json:"updated_at"`
}

func settingsView(s db.HistorySetting) SettingsView {
	return SettingsView{EvidenceThreshold: s.EvidenceThreshold, ClusterSimilarity: s.ClusterSimilarity, ResolutionSimilarity: s.ResolutionSimilarity,
		RejectionSimilarity: s.RejectionSimilarity, RawRetentionHours: s.RawRetentionHours, ArchiveRetentionDays: s.ArchiveRetentionDays,
		NightlyEnabled: s.NightlyEnabled, NightlyConsentAnalysis: s.NightlyConsentAnalysis, NightlyConsentTraining: s.NightlyConsentTraining,
		NightlyWatermark: s.NightlyWatermark, UpdatedAt: s.UpdatedAt}
}

func (h *Handler) Imports(w http.ResponseWriter, r *http.Request) {
	org, ok := scope(w, r)
	if !ok {
		return
	}
	q, ok := listQuery(w, r)
	if !ok {
		return
	}
	rows, err := h.Svc.Q.HistoryImportList(r.Context(), db.HistoryImportListParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		writeError(w, r, err)
		return
	}
	total, err := h.Svc.Q.HistoryImportCount(r.Context(), org)
	items := make([]ImportView, len(rows))
	for i, row := range rows {
		items[i] = importView(row)
	}
	page(w, r, items, total, q, err)
}

func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	org, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	row, err := h.Svc.Q.HistoryImportGet(r.Context(), db.HistoryImportGetParams{OrganizationID: org, Uuid: id})
	if err != nil {
		writeError(w, r, notFound(err))
		return
	}
	response.JSON(w, r, http.StatusOK, importView(row))
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	org, ok := scope(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, model.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Send the file as multipart/form-data (max 50 MB)")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	file, header, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, model.MaxUploadBytes+1))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file could not be read")
		return
	}
	in := model.UploadInput{ConsentAnalysis: r.FormValue("consent_analysis") == "true", ConsentTraining: r.FormValue("consent_training") == "true", BrandUUID: r.FormValue("brand_uuid")}
	if v := r.FormValue("department_id"); v != "" {
		if in.DepartmentID, err = strconv.ParseInt(v, 10, 64); err != nil || in.DepartmentID < 1 {
			response.BadRequest(w, r, response.CodeValidationError, "department_id is invalid")
			return
		}
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	row, err := h.Svc.Upload(r.Context(), org, actor, in, header.Filename, data)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, row.Uuid, map[string]any{"history": "uploaded", "consent_analysis": true, "consent_training": in.ConsentTraining, "tickets": row.TotalRecords})
	response.JSON(w, r, http.StatusAccepted, importView(row))
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	org, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	row, err := h.Svc.Cancel(r.Context(), org, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, row.Uuid, map[string]any{"history": "cancelled"})
	response.JSON(w, r, http.StatusOK, importView(row))
}

func (h *Handler) Rollback(w http.ResponseWriter, r *http.Request) {
	org, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	row, err := h.Svc.Rollback(r.Context(), org, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, row.Uuid, map[string]any{"history": "rolled_back"})
	response.JSON(w, r, http.StatusOK, importView(row))
}

func (h *Handler) Cases(w http.ResponseWriter, r *http.Request) {
	org, ok := scope(w, r)
	if !ok {
		return
	}
	q, ok := listQuery(w, r, "import", "cluster")
	if !ok {
		return
	}
	importID, ok1 := optionalUUID(w, r, "import")
	clusterID, ok2 := optionalUUID(w, r, "cluster")
	if !ok1 || !ok2 {
		return
	}
	rows, err := h.Svc.Q.HistoryCaseList(r.Context(), db.HistoryCaseListParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset, ImportUuid: importID, ClusterUuid: clusterID})
	if err != nil {
		writeError(w, r, err)
		return
	}
	total, err := h.Svc.Q.HistoryCaseCount(r.Context(), db.HistoryCaseCountParams{OrganizationID: org, ImportUuid: importID, ClusterUuid: clusterID})
	page(w, r, rows, total, q, err)
}

func (h *Handler) Clusters(w http.ResponseWriter, r *http.Request) {
	org, ok := scope(w, r)
	if !ok {
		return
	}
	q, ok := listQuery(w, r)
	if !ok {
		return
	}
	rows, err := h.Svc.Q.HistoryClusterList(r.Context(), db.HistoryClusterListParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		writeError(w, r, err)
		return
	}
	total, err := h.Svc.Q.HistoryClusterCount(r.Context(), org)
	items := make([]ClusterView, len(rows))
	for i, row := range rows {
		items[i] = ClusterView{HistoryClusterListRow: row}
		if row.SuggestionUuid != "" {
			id, status := row.SuggestionUuid, row.SuggestionStatus
			items[i].SuggestionUuid, items[i].SuggestionStatus = &id, &status
		}
	}
	page(w, r, items, total, q, err)
}

func (h *Handler) Suggestions(w http.ResponseWriter, r *http.Request) {
	org, ok := scope(w, r)
	if !ok {
		return
	}
	q, ok := listQuery(w, r, "status")
	if !ok {
		return
	}
	status := pgtype.Text{}
	if v := r.URL.Query().Get("status"); v != "" {
		if v != "pending" && v != "approved" && v != "rejected" {
			response.BadRequest(w, r, response.CodeValidationError, "status must be pending, approved or rejected")
			return
		}
		status = pgtype.Text{String: v, Valid: true}
	}
	rows, err := h.Svc.Q.HistorySuggestionList(r.Context(), db.HistorySuggestionListParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset, Status: status})
	if err != nil {
		writeError(w, r, err)
		return
	}
	total, err := h.Svc.Q.HistorySuggestionCount(r.Context(), db.HistorySuggestionCountParams{OrganizationID: org, Status: status})
	items := make([]SuggestionView, len(rows))
	for i, s := range rows {
		items[i] = SuggestionView{UUID: s.Uuid, Topic: s.Topic, Title: s.Title, Body: s.Body, Status: s.Status, Source: s.Source, EvidenceCount: s.EvidenceCount,
			ClusterUUID: s.ClusterUuid, ClusterSize: s.ClusterSize, DepartmentID: s.DepartmentID, ConflictArticleUUID: s.ConflictArticleUuid,
			ConflictArticleTitle: s.ConflictArticleTitle, ConflictScore: s.ConflictScore, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
	}
	page(w, r, items, total, q, err)
}

func (h *Handler) Decide(w http.ResponseWriter, r *http.Request) {
	org, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	var in model.DecisionInput
	if !decode(w, r, &in) {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	out, err := h.Svc.Decide(r.Context(), org, actor, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, out.Suggestion.Uuid, map[string]any{"history": "decided", "decision": out.Decision.Decision})
	response.JSON(w, r, http.StatusOK, map[string]any{"suggestion_uuid": out.Suggestion.Uuid, "status": out.Suggestion.Status, "decision": out.Decision.Decision, "decision_uuid": out.Decision.Uuid})
}

func (h *Handler) Decisions(w http.ResponseWriter, r *http.Request) {
	org, ok := scope(w, r)
	if !ok {
		return
	}
	q, ok := listQuery(w, r)
	if !ok {
		return
	}
	rows, err := h.Svc.Q.HistoryDecisionList(r.Context(), db.HistoryDecisionListParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		writeError(w, r, err)
		return
	}
	total, err := h.Svc.Q.HistoryDecisionCount(r.Context(), org)
	page(w, r, rows, total, q, err)
}

func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	org, ok := scope(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		s, err := h.Svc.Settings(r.Context(), org)
		if err != nil {
			writeError(w, r, err)
			return
		}
		response.JSON(w, r, http.StatusOK, settingsView(s))
		return
	}
	var in model.SettingsInput
	if !decode(w, r, &in) {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	s, err := h.Svc.SaveSettings(r.Context(), org, actor, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, uuid.Nil, map[string]any{"history": "settings", "nightly_enabled": in.NightlyEnabled, "evidence_threshold": in.EvidenceThreshold})
	response.JSON(w, r, http.StatusOK, settingsView(s))
}

// SampleFile serves the documented example export (invented data only).
func (h *Handler) SampleFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="gecmis-ornek.json"`)
	_, _ = w.Write(h.Sample)
}

func (h *Handler) record(r *http.Request, id uuid.UUID, payload map[string]any) {
	if h.Activity == nil {
		return
	}
	actor := authctx.MustPrincipal(r.Context())
	var ref *uuid.UUID
	if id != uuid.Nil {
		ref = &id
	}
	h.Activity.Record(r.Context(), &actor.UserInternal, "updated", "settings", ref, payload, r)
}

func page(w http.ResponseWriter, r *http.Request, items any, total int64, q apiquery.Query, err error) {
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "total": total, "limit": q.Limit, "offset": q.Offset})
}

func listQuery(w http.ResponseWriter, r *http.Request, filters ...string) (apiquery.Query, bool) {
	values := r.URL.Query()
	for _, f := range filters {
		values.Del(f)
	}
	q := apiquery.Parse(values)
	if len(apiquery.ForbiddenParams(values)) > 0 || len(q.Sort) > 0 || q.Q != "" {
		response.ValidationError(w, r, []response.Detail{{Field: "query", Code: "unsupported", Message: "Only limit, offset and the documented filters are supported"}})
		return q, false
	}
	return q, true
}

func optionalUUID(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return pgtype.UUID{}, true
	}
	id, err := uuid.Parse(v)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, name+" is invalid")
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: id, Valid: true}, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func scope(w http.ResponseWriter, r *http.Request) (int64, bool) {
	s, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("history: missing organization scope"), "")
		return 0, false
	}
	return s.ID, true
}

func scopeAndID(w http.ResponseWriter, r *http.Request) (int64, uuid.UUID, bool) {
	org, ok := scope(w, r)
	if !ok {
		return 0, uuid.Nil, false
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return 0, uuid.Nil, false
	}
	return org, id, true
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ErrNotFound
	}
	return err
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, model.ErrNotFound):
		response.NotFound(w, r, "Resource was not found")
	case errors.Is(err, model.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, strings.TrimPrefix(err.Error(), model.ErrInvalid.Error()+": "))
	case errors.Is(err, model.ErrConflict):
		response.Conflict(w, r, "HISTORY_CONFLICT", strings.TrimPrefix(err.Error(), model.ErrConflict.Error()+": "))
	case errors.Is(err, model.ErrForbidden):
		response.Forbidden(w, r, "Only the department lead can review this suggestion")
	case errors.Is(err, model.ErrUnavailable), errors.Is(err, ai.ErrNotConfigured):
		response.ServiceUnavailable(w, r, "HISTORY_UNAVAILABLE", "Storage or AI embedding is not configured")
	case errors.Is(err, ai.ErrInjection):
		response.BadRequest(w, r, response.CodeValidationError, "The text looks like an instruction to the AI and was refused")
	default:
		response.InternalErr(w, r, err, "")
	}
}
