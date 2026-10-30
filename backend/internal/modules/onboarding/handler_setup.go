package onboarding

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// Error codes of the set-up endpoints (CF-140).
const (
	CodeSetupNotFound       = "SETUP_RUN_NOT_FOUND"
	CodeSetupAlreadyApplied = "SETUP_RUN_NOT_DRAFT"
	CodeSetupActive         = "SETUP_RUN_ACTIVE"
	CodeSetupUndoExpired    = "SETUP_UNDO_EXPIRED"
	CodeSetupDiscoverLimit  = "SETUP_DISCOVER_LIMIT"
	CodeSetupUnknownTpl     = "SETUP_UNKNOWN_TEMPLATE"
	CodeSetupWebsite        = "SETUP_WEBSITE_INVALID"
)

// SetupHandler serves /v1/tenant/onboarding/setup/*.
type SetupHandler struct {
	svc      *SetupService
	activity *activity.Recorder
}

// NewSetupHandler builds the handler.
func NewSetupHandler(svc *SetupService, rec *activity.Recorder) *SetupHandler {
	return &SetupHandler{svc: svc, activity: rec}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

func scopeOf(w http.ResponseWriter, r *http.Request) (orgctx.Scope, int64, bool) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("onboarding: missing organization scope"), "")
		return orgctx.Scope{}, 0, false
	}
	return scope, authctx.MustPrincipal(r.Context()).UserInternal, true
}

func runID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Setup run not found")
		return uuid.Nil, false
	}
	return id, true
}

func (h *SetupHandler) record(r *http.Request, actor int64, action string, scope orgctx.Scope, run RunView, extra map[string]any) {
	if h.activity == nil {
		return
	}
	meta := map[string]any{"organization_uuid": scope.UUID, "run_uuid": run.UUID, "source": run.Source, "template_key": run.TemplateKey}
	for k, v := range extra {
		meta[k] = v
	}
	h.activity.Record(r.Context(), &actor, action, Resource, &run.UUID, meta, r)
}

// Templates lists the gallery.
func (h *SetupHandler) Templates(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]any{"items": h.svc.TemplateList()})
}

type discoverRequest struct {
	Website string  `json:"website"`
	Answers Answers `json:"answers"`
	Locale  string  `json:"locale"`
}

// Discover reads the website + answers and drafts a plan.
func (h *SetupHandler) Discover(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var req discoverRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Website != "" {
		norm, err := NormalizeWebsite(req.Website)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "website", Code: CodeSetupWebsite, Message: "website must be a public HTTPS address"}})
			return
		}
		req.Website = norm
	}
	if req.Answers.CompanyName == "" {
		req.Answers.CompanyName = scope.Name
	}
	run, err := h.svc.Discover(r.Context(), scope.ID, actor, DiscoverInput(req))
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	h.record(r, actor, "onboarding.setup_discovered", scope, run, map[string]any{"mode": run.Discovery.Mode})
	response.JSON(w, r, http.StatusCreated, run)
}

// UseTemplate drafts a run from a gallery template.
func (h *SetupHandler) UseTemplate(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := scopeOf(w, r)
	if !ok {
		return
	}
	run, err := h.svc.FromTemplate(r.Context(), scope.ID, actor, r.PathValue("key"), scope.Name)
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	h.record(r, actor, "onboarding.setup_template_selected", scope, run, nil)
	response.JSON(w, r, http.StatusCreated, run)
}

// Latest returns the newest run (or null).
func (h *SetupHandler) Latest(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := scopeOf(w, r)
	if !ok {
		return
	}
	run, err := h.svc.Latest(r.Context(), scope.ID)
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"run": run})
}

// Get returns one run.
func (h *SetupHandler) Get(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := scopeOf(w, r)
	if !ok {
		return
	}
	id, ok := runID(w, r)
	if !ok {
		return
	}
	run, err := h.svc.Run(r.Context(), scope.ID, id)
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, run)
}

type applyRequest struct {
	Selection []string `json:"selection"`
}

// Apply queues the run with the ticked items.
func (h *SetupHandler) Apply(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := scopeOf(w, r)
	if !ok {
		return
	}
	id, ok := runID(w, r)
	if !ok {
		return
	}
	var req applyRequest
	if !decode(w, r, &req) {
		return
	}
	run, err := h.svc.Apply(r.Context(), scope.ID, id, req.Selection)
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	h.record(r, actor, "onboarding.setup_applied", scope, run, map[string]any{"items": run.Total})
	response.JSON(w, r, http.StatusAccepted, run)
}

// Undo reverses an applied run within 24 hours.
func (h *SetupHandler) Undo(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := scopeOf(w, r)
	if !ok {
		return
	}
	id, ok := runID(w, r)
	if !ok {
		return
	}
	run, err := h.svc.Undo(r.Context(), scope.ID, actor, id)
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	h.record(r, actor, "onboarding.setup_undone", scope, run, nil)
	response.JSON(w, r, http.StatusOK, run)
}

func writeSetupError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrRunNotFound):
		response.Error(w, r, http.StatusNotFound, CodeSetupNotFound, "Setup run not found")
	case errors.Is(err, ErrRunNotDraft):
		response.Conflict(w, r, CodeSetupAlreadyApplied, "This setup run was already applied")
	case errors.Is(err, ErrRunActive):
		response.Conflict(w, r, CodeSetupActive, "Another setup run is being applied")
	case errors.Is(err, ErrUndoExpired):
		response.Conflict(w, r, CodeSetupUndoExpired, "This setup run can no longer be undone")
	case errors.Is(err, ErrDiscoverLimit):
		response.Error(w, r, http.StatusTooManyRequests, CodeSetupDiscoverLimit, "Too many AI set-up attempts today; pick a template instead")
	case errors.Is(err, ErrUnknownTemplate):
		response.Error(w, r, http.StatusNotFound, CodeSetupUnknownTpl, "Unknown setup template")
	case errors.Is(err, ErrEmptySelect):
		response.ValidationError(w, r, []response.Detail{{Field: "selection", Message: err.Error()}})
	case errors.Is(err, ErrWebsite):
		response.ValidationError(w, r, []response.Detail{{Field: "website", Code: CodeSetupWebsite, Message: err.Error()}})
	default:
		response.InternalErr(w, r, err, "")
	}
}
