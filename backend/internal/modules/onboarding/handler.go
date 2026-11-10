package onboarding

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// Error codes of the onboarding endpoints.
const (
	CodeSelfServeDisabled  = "SELF_SERVE_DISABLED"
	CodeOrganizationLimit  = "ORGANIZATION_LIMIT"
	CodeOnboardingInactive = "ONBOARDING_NOT_ACTIVE"
	CodeUnknownStep        = "ONBOARDING_UNKNOWN_STEP"
	CodeSlugTaken          = "ORGANIZATION_SLUG_TAKEN"
)

// Resource is the activity resource of onboarding events.
const Resource = "onboarding"

const maxBodyBytes = 16 << 10

// Handler serves the onboarding endpoints.
type Handler struct {
	svc      *Service
	activity *activity.Recorder
}

// NewHandler builds the handler.
func NewHandler(svc *Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

type createRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	// PriceUUID optionally picks the trial plan (pricing page → sign-up).
	PriceUUID uuid.UUID `json:"price_uuid"`
}

// CreateOrganization opens an organization owned by the caller.
func (h *Handler) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return
	}
	principal := authctx.MustPrincipal(r.Context())
	org, err := h.svc.CreateOrganization(r.Context(), CreateInput{
		UserID: principal.UserInternal, Name: req.Name, Slug: req.Slug, PriceUUID: req.PriceUUID,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		actor := principal.UserInternal
		h.activity.Record(r.Context(), &actor, "onboarding.organization_created", Resource, &org.UUID, map[string]any{
			"organization_uuid": org.UUID, "name": org.Name, "slug": org.Slug,
		}, r)
	}
	response.JSON(w, r, http.StatusCreated, map[string]any{"organization": org})
}

// Get returns the checklist of the organization in scope.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("onboarding: missing organization scope"), "")
		return
	}
	st, err := h.svc.Get(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// SkipStep marks a step as skipped; it stays as a warning.
func (h *Handler) SkipStep(w http.ResponseWriter, r *http.Request) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("onboarding: missing organization scope"), "")
		return
	}
	st, err := h.svc.SkipStep(r.Context(), scope.ID, r.PathValue("step"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// Finish closes the checklist.
func (h *Handler) Finish(w http.ResponseWriter, r *http.Request) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("onboarding: missing organization scope"), "")
		return
	}
	st, err := h.svc.Finish(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		actor := authctx.MustPrincipal(r.Context()).UserInternal
		h.activity.Record(r.Context(), &actor, "onboarding.finished", Resource, &scope.UUID, map[string]any{
			"organization_uuid": scope.UUID, "completed": st.Completed, "total": st.Total,
		}, r)
	}
	response.JSON(w, r, http.StatusOK, st)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrSelfServeDisabled):
		response.Error(w, r, http.StatusForbidden, CodeSelfServeDisabled, "Opening an organization is not available")
	case errors.Is(err, ErrOrganizationLimit):
		response.Conflict(w, r, CodeOrganizationLimit, "This account already owns the maximum number of organizations")
	case errors.Is(err, ErrNotActive):
		response.Conflict(w, r, CodeOnboardingInactive, "The guided set-up is not active for this organization")
	case errors.Is(err, ErrUnknownStep):
		response.BadRequest(w, r, CodeUnknownStep, "Unknown step")
	case errors.Is(err, ErrInvalidPrice):
		response.ValidationError(w, r, []response.Detail{{Field: "price_uuid", Message: err.Error()}})
	case errors.Is(err, ErrInvalidName):
		response.ValidationError(w, r, []response.Detail{{Field: "name", Message: err.Error()}})
	case errors.Is(err, orgusecase.ErrConflict):
		response.Conflict(w, r, CodeSlugTaken, "This address is already taken")
	case errors.Is(err, orgusecase.ErrInvalidRequest):
		response.ValidationError(w, r, []response.Detail{{Field: "slug", Message: err.Error()}})
	default:
		response.InternalErr(w, r, err, "")
	}
}
