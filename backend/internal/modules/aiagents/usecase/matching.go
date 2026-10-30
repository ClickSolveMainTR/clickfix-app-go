package usecase

import (
	"context"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/google/uuid"
)

// IssueGateway is the engineering module boundary. The gateway must validate
// organization ownership on both issue and conversation before linking.
type IssueGateway interface {
	MatchOpen(context.Context, int64, string) ([]IssueMatch, error)
	Create(context.Context, int64, int64, uuid.UUID, string, string) (uuid.UUID, error)
	Link(context.Context, int64, int64, uuid.UUID, uuid.UUID) error
}
type StatusGateway interface {
	Query(context.Context, int64) (string, error)
}
type IssueMatch = model.IssueMatch

func (s *Service) enrichTriage(ctx context.Context, org int64, conv db.Conversation, summary string) ([]model.ConversationMatch, []IssueMatch, error) {
	// Search stored organization-owned text; model-generated UUIDs are never trusted.
	query := strings.TrimSpace(conv.Subject)
	if query == "" {
		query = summary
	}
	if len(query) > 1000 {
		query = string([]rune(query)[:min(250, len([]rune(query)))])
	}
	rows, err := s.Repo.Q.AISimilarConversations(ctx, db.AISimilarConversationsParams{OrganizationID: org, CurrentID: conv.ID, Q: query})
	if err != nil {
		return nil, nil, err
	}
	for i := range rows {
		rows[i].Subject = ai.NewMasker().Mask(rows[i].Subject)
		score := min(float32(1), rows[i].Score)
		if err = s.Repo.Q.AISaveSimilarLink(ctx, db.AISaveSimilarLinkParams{OrganizationID: org, ConversationID: conv.ID, SimilarID: rows[i].ID, Score: score}); err != nil {
			return nil, nil, err
		}
	}
	matches := []model.ConversationMatch{}
	for _, row := range rows {
		matches = append(matches, model.ConversationMatch{UUID: row.Uuid, Subject: row.Subject, Status: row.Status, Score: row.Score})
	}
	issues := []IssueMatch{}
	if s.Tools != nil && s.Tools.Issues != nil {
		issues, err = s.Tools.Issues.MatchOpen(ctx, org, query)
	}
	return matches, issues, err
}
