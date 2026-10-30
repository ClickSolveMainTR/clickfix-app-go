package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/apierror"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	kbhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	Service  *usecase.Service
	Activity *activity.Recorder
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		response.NotFound(w, r, "AI run or organization not found")
	case errors.Is(err, model.ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Field: "input", Code: "invalid", Message: "Invalid AI request"}})
	case apierror.Write(w, r, err):
	default:
		response.InternalErr(w, r, err, "")
	}
}

func (h *Handler) org(w http.ResponseWriter, r *http.Request) (int64, bool) {
	if raw := r.PathValue("organization"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			fail(w, r, model.ErrInvalid)
			return 0, false
		}
		org, err := h.Service.Repo.Organization(r.Context(), id)
		if err != nil {
			fail(w, r, err)
			return 0, false
		}
		return org, true
	}
	return orgctx.Must(r.Context()).ID, true
}

func (h *Handler) Policy(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context()).ID
	if r.Method == http.MethodGet {
		policy, err := h.Service.Repo.Policy(r.Context(), org)
		if err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, policy)
		return
	}
	var in struct {
		Enabled           bool  `json:"enabled"`
		MaskPii           bool  `json:"mask_pii"`
		AllowUnmask       bool  `json:"allow_unmask"`
		DailyCalls        int32 `json:"daily_calls"`
		Concurrency       int32 `json:"concurrency"`
		DailyBudgetMicros int64 `json:"daily_budget_micros"`
	}
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	policy, err := h.Service.Repo.SavePolicy(r.Context(), db.AISavePolicyParams{OrganizationID: org, Enabled: in.Enabled, MaskPii: in.MaskPii, AllowUnmask: in.AllowUnmask, DailyCalls: in.DailyCalls, Concurrency: in.Concurrency, DailyBudgetMicros: in.DailyBudgetMicros})
	if err != nil {
		fail(w, r, err)
		return
	}
	if h.Activity != nil {
		p := authctx.MustPrincipal(r.Context())
		scope := orgctx.Must(r.Context())
		h.Activity.Record(r.Context(), &p.UserInternal, "organizations.updated", "organizations", &scope.UUID, map[string]any{"area": "ai_policy"}, r)
	}
	response.JSON(w, r, 200, policy)
}

func (h *Handler) Run(w http.ResponseWriter, r *http.Request) {
	var in model.RunInput
	if !kbhandler.Decode(w, r, &in) {
		return
	}
	out, err := h.Service.Run(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, in)
	if err != nil {
		w.Header().Set("X-AI-Run-ID", out.UUID.String())
		fail(w, r, err)
		return
	}
	if in.Stream {
		// Provider SSE is buffered until output guards complete; raw deltas could leak
		// split PII or disallowed content. The final guarded result is an SSE event.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		raw, err := json.Marshal(out)
		if err != nil {
			fail(w, r, err)
			return
		}
		_, _ = fmt.Fprintf(w, "event: result\ndata: %s\n\n", raw)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}
	response.JSON(w, r, 200, out)
}

func (h *Handler) Runs(w http.ResponseWriter, r *http.Request) {
	org, ok := h.org(w, r)
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	if len(apiquery.ForbiddenParams(r.URL.Query())) > 0 || len(q.Sort) > 0 || q.Q != "" {
		fail(w, r, model.ErrInvalid)
		return
	}
	rows, total, err := h.Service.Repo.List(r.Context(), org, q.Limit, q.Offset)
	if err != nil {
		fail(w, r, err)
		return
	}
	items := make([]runView, len(rows))
	for i, row := range rows {
		items[i] = view(row)
	}
	response.JSON(w, r, 200, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := h.org(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		fail(w, r, model.ErrInvalid)
		return
	}
	row, err := h.Service.Repo.Get(r.Context(), org, id)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, view(row))
}

// Availability: GET /v1/tenant/ai/availability. Lets the UI hide or disable
// AI actions (copilot, AI runs) and say why before the agent clicks.
func (h *Handler) Availability(w http.ResponseWriter, r *http.Request) {
	a, err := h.Service.Availability(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, a)
}

func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, 200, resourcemeta.ResourceMeta{Resource: "tenant.ai.runs", DefaultSort: "-created_at", DefaultFields: []string{"uuid", "task", "model", "status", "created_at"}, Capabilities: resourcemeta.Capabilities{Read: true}, Columns: []resourcemeta.Column{{Key: "uuid", LabelKey: "users.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: true}}})
}

// json.RawMessage preserves JSON trace arrays instead of []byte's base64 encoding.
type runView struct {
	db.AiRun
	RetrievedChunks json.RawMessage `json:"retrieved_chunks"`
	ToolCalls       json.RawMessage `json:"tool_calls"`
}

func view(row db.AiRun) runView {
	return runView{AiRun: row, RetrievedChunks: json.RawMessage(row.RetrievedChunks), ToolCalls: json.RawMessage(row.ToolCalls)}
}
