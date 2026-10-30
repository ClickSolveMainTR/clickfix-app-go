package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	attachmentsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/i18n"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

type Handler struct {
	svc         *usecase.Service
	ent         *entitlements.Service
	activity    *activity.Recorder
	attachments *attachmentsusecase.Service
	log         *slog.Logger
	limit       Limiter
}

// Limiter is a fixed-window rate limit (platform/ratelimit Allow).
type Limiter func(r *http.Request, action, subject string, limit int, window time.Duration) (bool, time.Duration)

func New(s *usecase.Service, e *entitlements.Service, a *activity.Recorder) *Handler {
	return &Handler{svc: s, ent: e, activity: a}
}

// WithWebhookGuards sets the structured audit log and rate limit of the
// public payment endpoints (provider notifications and browser returns).
func (h *Handler) WithWebhookGuards(log *slog.Logger, limit Limiter) *Handler {
	h.log, h.limit = log, limit
	return h
}
func (h *Handler) WithAttachments(s *attachmentsusecase.Service) *Handler {
	h.attachments = s
	return h
}
func fail(w http.ResponseWriter, r *http.Request, err error) {
	if entitlement.WriteError(w, r, err) {
		return
	}
	err = repository.Normalize(err)
	var coded *model.CodedError
	var refusal *payments.ProviderError
	switch {
	case errors.As(err, &coded):
		writeCode(w, r, coded.Status, coded.Code)
	case errors.Is(err, payments.ErrProviderDisabled):
		writeCode(w, r, http.StatusConflict, "PAYMENT_PROVIDER_DISABLED")
	case errors.Is(err, payments.ErrNotConfigured):
		writeCode(w, r, http.StatusConflict, "PAYMENT_GATEWAY_NOT_CONFIGURED")
	case errors.Is(err, payments.ErrUnsupported):
		writeCode(w, r, http.StatusBadRequest, "PAYMENT_OPERATION_UNSUPPORTED")
	case errors.Is(err, payments.ErrInvalidSignature), errors.Is(err, payments.ErrMalformed):
		writeCode(w, r, http.StatusBadRequest, "PAYMENT_VERIFICATION_FAILED")
	case errors.As(err, &refusal) && refusal.Temporary:
		writeCode(w, r, http.StatusServiceUnavailable, "PAYMENT_PROVIDER_UNAVAILABLE")
	case errors.As(err, &refusal):
		writeCode(w, r, http.StatusBadGateway, "PAYMENT_PROVIDER_REJECTED")
	case errors.Is(err, model.ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Field: "body", Code: "invalid", Message: err.Error()}})
	case errors.Is(err, model.ErrNotFound):
		response.NotFound(w, r, "")
	case errors.Is(err, model.ErrConflict):
		response.Conflict(w, r, "BILLING_CONFLICT", "Billing state conflicts with this operation")
	default:
		response.Internal(w, r, "")
	}
}

// writeCode writes a billing error code with its message in the caller's
// language (Accept-Language en, else Turkish).
func writeCode(w http.ResponseWriter, r *http.Request, status int, code string) {
	response.Error(w, r, status, code, i18n.Translate(requestLocale(r), "billing.errors."+code))
}

func requestLocale(r *http.Request) i18n.Locale {
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Accept-Language")), "en") {
		return i18n.LocaleEN
	}
	return i18n.LocaleTR
}
func decode(w http.ResponseWriter, r *http.Request, in any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(in); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "body", Code: "invalid", Message: "Invalid JSON body"}})
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		response.ValidationError(w, r, []response.Detail{{Field: "body", Code: "invalid", Message: "Expected one JSON object"}})
		return false
	}
	return true
}
func path(w http.ResponseWriter, r *http.Request, key string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(key))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: key, Code: "invalid", Message: "Invalid UUID"}})
		return uuid.Nil, false
	}
	return id, true
}
func actor(r *http.Request) int64 { return authctx.MustPrincipal(r.Context()).UserInternal }
func (h *Handler) audit(r *http.Request, action, resource string, id uuid.UUID) {
	a := actor(r)
	h.activity.Record(r.Context(), &a, action, resource, &id, nil, r)
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request)    { h.list(w, r, false) }
func (h *Handler) Pricing(w http.ResponseWriter, r *http.Request) { h.list(w, r, true) }
func (h *Handler) list(w http.ResponseWriter, r *http.Request, public bool) {
	q := apiquery.Parse(r.URL.Query())
	if len(apiquery.ForbiddenParams(r.URL.Query())) > 0 || len(q.Sort) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: "query", Code: "invalid", Message: "Use limit, offset, q; catalog order is sort_order,code"}})
		return
	}
	items, total, err := h.svc.List(r.Context(), public, q.Q, q.Limit, q.Offset)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, apiquery.NewPage(items, total, q.Limit, q.Offset))
}
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, 200, resourcemeta.ResourceMeta{Resource: "platform.plans", DefaultSort: "sort_order,code", DefaultFields: []string{"uuid", "name", "visibility"}, Capabilities: resourcemeta.Capabilities{Create: true, Read: true, Update: true, Delete: true, Search: true}, SearchableFields: []string{"name", "code"}, Columns: []resourcemeta.Column{{Key: "name", LabelKey: "billing.plan_name", Type: resourcemeta.ColumnTypeString, DefaultVisible: true}}})
}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := path(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in model.PlanInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Create(r.Context(), in)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.plan_created", "billing_plans", out.UUID)
	response.JSON(w, r, 201, out)
}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := path(w, r, "uuid")
	if !ok {
		return
	}
	var in model.PlanInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.plan_updated", "billing_plans", id)
	response.JSON(w, r, 200, out)
}
func (h *Handler) Archive(w http.ResponseWriter, r *http.Request) {
	id, ok := path(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.Archive(r.Context(), id); err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.plan_archived", "billing_plans", id)
	response.JSON(w, r, 200, map[string]string{"status": "archived"})
}
func (h *Handler) Features(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Features(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}
func (h *Handler) SaveVersion(w http.ResponseWriter, r *http.Request) {
	plan, ok := path(w, r, "uuid")
	if !ok {
		return
	}
	version := uuid.Nil
	if r.PathValue("version") != "" {
		version, ok = path(w, r, "version")
		if !ok {
			return
		}
	}
	var in model.VersionInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.SaveVersion(r.Context(), plan, version, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.version_saved", "billing_plans", plan)
	status := 200
	if version == uuid.Nil {
		status = 201
	}
	response.JSON(w, r, status, out)
}
func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	plan, ok := path(w, r, "uuid")
	if !ok {
		return
	}
	v, ok := path(w, r, "version")
	if !ok {
		return
	}
	out, err := h.svc.Publish(r.Context(), plan, v, actor(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.plan_published", "billing_plans", plan)
	response.JSON(w, r, 200, out)
}
func (h *Handler) Migrate(w http.ResponseWriter, r *http.Request) {
	var in model.Migration
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.ScheduleMigration(r.Context(), in, actor(r)); err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.migration_scheduled", "billing_plans", in.VersionUUID)
	response.JSON(w, r, 200, map[string]int{"scheduled": len(in.SubscriptionUUIDs)})
}
func (h *Handler) organization(w http.ResponseWriter, r *http.Request) (int64, bool) {
	if r.PathValue("organization") == "" {
		return orgctx.Must(r.Context()).ID, true
	}
	id, ok := path(w, r, "organization")
	if !ok {
		return 0, false
	}
	org, err := h.svc.OrganizationID(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return 0, false
	}
	return org, true
}
func (h *Handler) Subscription(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Subscription(r.Context(), org)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}
func (h *Handler) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	var in model.CreateSubscription
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.CreateSubscription(r.Context(), in, actor(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.subscription_created", "billing_subscriptions", out.UUID)
	response.JSON(w, r, 201, out)
}
func (h *Handler) Transition(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	id, ok := path(w, r, "subscription")
	if !ok {
		return
	}
	var in model.Transition
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Transition(r.Context(), org, id, in, actor(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.subscription_transitioned", "billing_subscriptions", id)
	response.JSON(w, r, 200, out)
}
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	id, ok := path(w, r, "subscription")
	if !ok {
		return
	}
	out, err := h.svc.History(r.Context(), org, id)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	id, ok := path(w, r, "subscription")
	if !ok {
		return
	}
	var in struct {
		Immediate bool `json:"immediate"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Cancel(r.Context(), org, id, in.Immediate, actor(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	h.audit(r, "billing.subscription_canceled", "billing_subscriptions", id)
	response.JSON(w, r, 200, out)
}
func (h *Handler) Entitlements(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	out, err := h.ent.Resolve(r.Context(), org)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, out)
}
func (h *Handler) Override(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	var in *model.Override
	if r.Method != "DELETE" {
		in = &model.Override{}
		if !decode(w, r, in) {
			return
		}
	}
	if err := h.ent.SetOverride(r.Context(), org, r.PathValue("feature"), in, actor(r)); err != nil {
		fail(w, r, err)
		return
	}
	id, _ := uuid.Parse(r.PathValue("organization"))
	h.audit(r, "billing.override_changed", "organizations", id)
	response.JSON(w, r, 200, map[string]string{"status": "updated"})
}
