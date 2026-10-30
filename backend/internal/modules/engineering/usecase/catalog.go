package usecase

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/google/uuid"
	"strings"
)

func (s *Service) Projects(ctx context.Context, org int64) ([]map[string]any, error) {
	rows, err := s.Store.Q.EngineeringListProjects(ctx, org)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, r := range rows {
		out = append(out, map[string]any{"uuid": r.Uuid, "name": r.Name})
	}
	return out, nil
}
func (s *Service) CreateProject(ctx context.Context, org int64, name string) (map[string]any, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 120 {
		return nil, invalid("name must be 1..120 characters")
	}
	r, err := s.Store.Q.EngineeringCreateProject(ctx, db.EngineeringCreateProjectParams{OrganizationID: org, Name: name})
	return map[string]any{"uuid": r.Uuid, "name": r.Name}, err
}
func (s *Service) Releases(ctx context.Context, org int64) ([]map[string]any, error) {
	rows, err := s.Store.Q.EngineeringListReleases(ctx, org)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, r := range rows {
		out = append(out, ReleaseDTO(r))
	}
	return out, nil
}
func ReleaseDTO(r db.EngineeringRelease) map[string]any {
	var released any
	if r.ReleasedAt.Valid {
		released = r.ReleasedAt.Time
	}
	return map[string]any{"uuid": r.Uuid, "version": r.Version, "changelog_url": r.ChangelogUrl, "released_at": released}
}
func CommentDTO(r db.EngineeringComment) map[string]any {
	return map[string]any{"uuid": r.Uuid, "body": r.Body, "source": r.Source, "created_at": r.CreatedAt.Time}
}
func EscalationDTO(r db.EngineeringEscalation) map[string]any {
	return map[string]any{"uuid": r.Uuid, "tier": r.Tier, "reason": r.Reason, "due_at": r.DueAt.Time, "created_at": r.CreatedAt.Time}
}
func (s *Service) Comments(ctx context.Context, org int64, id uuid.UUID) ([]map[string]any, error) {
	r, err := s.Store.Q.EngineeringIssue(ctx, db.EngineeringIssueParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return nil, missing(err)
	}
	rows, err := s.Store.Q.EngineeringComments(ctx, db.EngineeringCommentsParams{OrganizationID: org, IssueID: r.ID})
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, r := range rows {
		out = append(out, CommentDTO(r))
	}
	return out, nil
}

// Impact is restricted by the HTTP route to conversation.read_all. Currency
// totals remain separate and each company is counted only once.
func (s *Service) Impact(ctx context.Context, org int64, id uuid.UUID) (any, error) {
	r, err := s.Store.Q.EngineeringIssue(ctx, db.EngineeringIssueParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return nil, missing(err)
	}
	currencies, err := s.Store.Q.EngineeringImpact(ctx, db.EngineeringImpactParams{OrganizationID: org, IssueID: r.ID})
	if err != nil {
		return nil, err
	}
	count, err := s.Store.Q.EngineeringCompanyCount(ctx, db.EngineeringCompanyCountParams{OrganizationID: org, IssueID: r.ID})
	if err != nil {
		return nil, err
	}
	return map[string]any{"company_count": count, "currencies": currencies}, nil
}
func (s *Service) ExternalLinks(ctx context.Context, org int64, id uuid.UUID) ([]map[string]any, error) {
	r, err := s.Store.Q.EngineeringIssue(ctx, db.EngineeringIssueParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return nil, missing(err)
	}
	rows, err := s.Store.Q.EngineeringIssueExternals(ctx, db.EngineeringIssueExternalsParams{OrganizationID: org, IssueID: r.ID})
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, r := range rows {
		out = append(out, ExternalDTO(r))
	}
	return out, nil
}
func ExternalDTO(r db.EngineeringExternalIssue) map[string]any {
	return map[string]any{"uuid": r.Uuid, "external_id": r.ExternalID, "external_key": r.ExternalKey, "external_url": r.ExternalUrl, "synced_revision": r.SyncedRevision, "last_error": r.LastError}
}
func (s *Service) Escalations(ctx context.Context, org, actor int64, conversation uuid.UUID) ([]map[string]any, error) {
	if _, err := s.Inbox.Get(ctx, org, actor, conversation); err != nil {
		return nil, missing(err)
	}
	c, err := s.Store.Q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: conversation})
	if err != nil {
		return nil, missing(err)
	}
	rows, err := s.Store.Q.EngineeringEscalations(ctx, db.EngineeringEscalationsParams{OrganizationID: org, ConversationID: c.ID})
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, r := range rows {
		out = append(out, EscalationDTO(r))
	}
	return out, nil
}
func (s *Service) Deliveries(ctx context.Context, org int64, id uuid.UUID) ([]map[string]any, error) {
	issue, err := s.Store.Q.EngineeringIssue(ctx, db.EngineeringIssueParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return nil, missing(err)
	}
	rows, err := s.Store.Q.EngineeringDeliveries(ctx, db.EngineeringDeliveriesParams{OrganizationID: org, IssueID: issue.ID})
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, r := range rows {
		out = append(out, map[string]any{"uuid": r.Uuid, "status": r.Status, "message_uuid": r.MessageUuid, "attempts": r.Attempts, "last_error": r.LastError})
	}
	return out, nil
}
