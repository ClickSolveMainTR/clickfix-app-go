package transcription

import (
	"encoding/json"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type View struct {
	UUID          uuid.UUID       `json:"uuid"`
	Status        string          `json:"status"`
	Transcript    string          `json:"transcript"`
	Segments      json.RawMessage `json:"segments"`
	Summary       json.RawMessage `json:"summary"`
	SummaryStatus string          `json:"summary_status"`
	ErrorCode     string          `json:"error_code"`
}

func view(t db.RecordingTranscript) View {
	return View{t.Uuid, t.Status, t.Transcript, t.Segments, t.Summary, t.SummaryStatus, t.ErrorCode}
}
func (s *Service) authorized(r *http.Request) (db.RecordingTranscript, error) {
	org := orgctx.Must(r.Context()).ID
	t, err := s.Repo.TranscriptGet(r.Context(), db.TranscriptGetParams{OrganizationID: org, Uuid: supportops.ID(r)})
	if err != nil {
		return t, err
	}
	if t.ConversationID.Valid {
		id, err := s.Repo.ComplianceConversationPublicID(r.Context(), db.ComplianceConversationPublicIDParams{OrganizationID: org, ID: t.ConversationID.Int64})
		if err != nil {
			return t, err
		}
		_, err = supportops.Conversation(r.Context(), s.Repo.Queries, org, supportops.Actor(r.Context()), id)
		return t, err
	}
	if !orgctx.Must(r.Context()).Has(rbac.PermTenantVideoManage) {
		return t, pgx.ErrNoRows
	}
	return t, nil
}
func RegisterRoutes(rt supportops.Router, s *Service) {
	rt.Tenant("GET /v1/tenant/recording-transcripts/{uuid}", rbac.PermTenantConversationsRead, func(r *http.Request) (any, error) {
		t, err := s.authorized(r)
		return view(t), err
	})
	rt.Tenant("GET /v1/tenant/recording-transcripts/{uuid}/summary", rbac.PermTenantConversationsRead, func(r *http.Request) (any, error) {
		t, err := s.authorized(r)
		return map[string]any{"status": t.SummaryStatus, "summary": json.RawMessage(t.Summary)}, err
	})
	rt.Tenant("GET /v1/tenant/conversations/{uuid}/recording-transcripts", rbac.PermTenantConversationsRead, func(r *http.Request) (any, error) {
		org := orgctx.Must(r.Context()).ID
		c, err := supportops.Conversation(r.Context(), s.Repo.Queries, org, supportops.Actor(r.Context()), supportops.ID(r))
		if err != nil {
			return nil, err
		}
		page, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		rows, err := s.Repo.TranscriptConversation(r.Context(), db.TranscriptConversationParams{OrganizationID: org, ConversationID: pgtype.Int8{Int64: c.ID, Valid: true}, LimitCount: page.Limit, OffsetCount: page.Offset})
		items := make([]View, 0, len(rows))
		for _, t := range rows {
			items = append(items, view(t))
		}
		if err != nil {
			return nil, err
		}
		total, err := s.Repo.TranscriptConversationCount(r.Context(), db.TranscriptConversationCountParams{OrganizationID: org, ConversationID: pgtype.Int8{Int64: c.ID, Valid: true}})
		return supportops.Page(items, total, page), err
	})
	rt.Tenant("GET /v1/tenant/video/recordings/{uuid}/transcript", rbac.PermTenantVideoManage, func(r *http.Request) (any, error) {
		org := orgctx.Must(r.Context()).ID
		id, err := s.Repo.TranscriptByVideo(r.Context(), db.TranscriptByVideoParams{OrganizationID: org, Uuid: supportops.ID(r)})
		if err != nil {
			return nil, err
		}
		r.SetPathValue("uuid", id.String())
		t, err := s.authorized(r)
		return view(t), err
	})
}
