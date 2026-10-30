package activity

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/netip"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/clientip"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Recorder persists audit events.
type Recorder struct {
	q   *db.Queries
	log *slog.Logger
}

// NewRecorder creates an activity recorder.
func NewRecorder(q *db.Queries, log *slog.Logger) *Recorder {
	if log == nil {
		log = slog.Default()
	}
	return &Recorder{q: q, log: log}
}

// Record writes an activity event (fail-soft). Inside an organization scope
// (orgctx on ctx: tenant requests, org-scoped jobs) the event is stamped with
// the organization so it shows in that organization's audit log (CF-12).
// Personal data is scrubbed from the payload first (see Scrub).
func (r *Recorder) Record(ctx context.Context, actorID *int64, action, resource string, resourceUUID *uuid.UUID, payload map[string]any, rreq *http.Request) {
	if r == nil || r.q == nil {
		return
	}
	clean := Scrub(payload)
	if s, ok := orgctx.From(ctx); ok && s.Support {
		// Support staff actions are always marked (CF-14).
		if clean == nil {
			clean = map[string]any{}
		}
		clean["via_support"] = true
	}
	body, err := json.Marshal(clean)
	if err != nil || payload == nil {
		body = []byte("{}")
	}
	var ru pgtype.UUID
	if resourceUUID != nil {
		ru = pgtype.UUID{Bytes: *resourceUUID, Valid: true}
	}
	var ip *netip.Addr
	var ua pgtype.Text
	if rreq != nil {
		// The peer is the BFF in production; audit rows need the browser's
		// address it forwards (CF-153).
		if parsed, err := netip.ParseAddr(clientip.FromRequest(rreq)); err == nil {
			ip = &parsed
		}
		if rreq.UserAgent() != "" {
			ua = pgtype.Text{String: rreq.UserAgent(), Valid: true}
		}
	}
	_, err = r.q.InsertActivityEvent(ctx, db.InsertActivityEventParams{
		ActorUserID:    pgtypeInt8(actorID),
		Action:         action,
		Resource:       resource,
		ResourceUuid:   ru,
		Payload:        body,
		IpAddress:      ip,
		UserAgent:      ua,
		OrganizationID: orgID(ctx),
	})
	if err != nil {
		r.log.Warn("activity_record_failed", "action", action, "error", err)
	}
}

func orgID(ctx context.Context) pgtype.Int8 {
	if id, ok := orgctx.OrganizationID(ctx); ok {
		return pgtype.Int8{Int64: id, Valid: true}
	}
	return pgtype.Int8{}
}

func pgtypeInt8(id *int64) pgtype.Int8 {
	if id == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *id, Valid: true}
}
