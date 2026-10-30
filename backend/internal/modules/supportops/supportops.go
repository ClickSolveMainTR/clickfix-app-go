// Package supportops shares boundary validation for the support operations modules.
package supportops

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	aiusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	billingmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrInvalid = errors.New("support operations: invalid input")
var ErrConflict = errors.New("support operations: conflicting state")
var ErrDenied = errors.New("support operations: forbidden")

type AI interface {
	RunAgent(context.Context, int64, int64, aimodel.RunInput, aiusecase.AgentOptions) (aiusecase.RunResult, error)
}

func Decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		Fail(w, r, ErrInvalid)
		return false
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		Fail(w, r, ErrInvalid)
		return false
	}
	return true
}
func ID(r *http.Request) uuid.UUID    { id, _ := uuid.Parse(r.PathValue("uuid")); return id }
func Actor(ctx context.Context) int64 { return authctx.MustPrincipal(ctx).UserInternal }
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	if entitlement.WriteError(w, r, err) {
		return
	}
	var pe *pgconn.PgError
	switch {
	case entitlement.WriteError(w, r, err):
		// Plan gate (403 ENTITLEMENT_REQUIRED / PLAN_LIMIT_EXCEEDED) already written.
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, billingmodel.ErrNotFound):
		response.NotFound(w, r, "Record not found")
	case errors.Is(err, ErrInvalid), errors.Is(err, billingmodel.ErrInvalid), errors.As(err, &pe) && (pe.Code == "23514" || pe.Code == "22001"):
		response.ValidationError(w, r, []response.Detail{{Message: "Invalid input"}})
	case errors.Is(err, ErrDenied):
		response.Forbidden(w, r, "Access denied")
	case errors.Is(err, ErrConflict), errors.Is(err, billingmodel.ErrConflict), errors.As(err, &pe) && (pe.Code == "23505" || pe.Code == "23503"):
		response.Conflict(w, r, response.CodeConflict, "Record has changed or conflicts with existing data")
	case errors.Is(err, aimodel.ErrDisabled):
		response.Error(w, r, http.StatusForbidden, "AI_DISABLED", "AI is turned off for this organization")
	case errors.Is(err, aimodel.ErrBudget), errors.Is(err, aimodel.ErrCredits), errors.Is(err, aimodel.ErrConcurrency), errors.Is(err, aimodel.ErrDaily):
		response.Error(w, r, 429, "AI_LIMIT_REACHED", "AI execution limit reached")
	case errors.Is(err, ai.ErrInjection), errors.Is(err, ai.ErrInvalidOutput), errors.Is(err, ai.ErrRefused):
		response.Error(w, r, 422, "AI_OUTPUT_REJECTED", "AI input or output rejected")
	case errors.Is(err, ai.ErrNotConfigured):
		response.Conflict(w, r, "AI_NOT_CONFIGURED", "AI is not configured on this platform")
	case errors.Is(err, ai.ErrUnavailable):
		response.Error(w, r, 503, "AI_UNAVAILABLE", "AI unavailable")
	default:
		response.InternalErr(w, r, err, "")
	}
}
func Record(rec *activity.Recorder, r *http.Request, action string, id *uuid.UUID) {
	if rec == nil {
		return
	}
	actor := Actor(r.Context())
	rec.Record(r.Context(), &actor, action, "support_operations", id, map[string]any{"organization_id": scopeID(r.Context())}, r)
}

// Conversation checks the same team visibility rule used by the inbox before any transcript is loaded.
func Conversation(ctx context.Context, q *db.Queries, org, actor int64, id uuid.UUID) (db.Conversation, error) {
	c, err := q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return c, err
	}
	scope := orgctx.Must(ctx)
	if !access.CanRead(scope) {
		return c, ErrDenied
	}
	v, err := access.For(ctx, q, scope, actor)
	if err != nil {
		return c, err
	}
	ok, err := v.CanSee(ctx, q, org, c.ID)
	if err != nil {
		return c, err
	}
	if !ok {
		return c, pgx.ErrNoRows
	}
	return c, nil
}
func Transcript(ctx context.Context, q *db.Queries, org, actor int64, id uuid.UUID) (db.Conversation, string, error) {
	c, err := Conversation(ctx, q, org, actor, id)
	if err != nil {
		return c, "", err
	}
	rows, err := q.OpsTranscript(ctx, db.OpsTranscriptParams{OrganizationID: org, ConversationID: c.ID})
	if err != nil {
		return c, "", err
	}
	var b strings.Builder
	for i := len(rows) - 1; i >= 0; i-- {
		line := rows[i].SenderType + ": " + rows[i].Content + "\n"
		if b.Len()+len(line) > 16000 {
			break
		}
		b.WriteString(line)
	}
	if b.Len() == 0 {
		return c, "", ErrInvalid
	}
	return c, b.String(), nil
}

func scopeID(ctx context.Context) int64 { s, _ := orgctx.From(ctx); return s.ID }
