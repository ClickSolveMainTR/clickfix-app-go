package usecase

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
)

// AIJobFinished tells the conversation's audience that an AI agent job
// finished (CF-246), so an open composer shows the draft without a page
// reload. Best effort: a missing conversation or notifier publishes nothing.
func (s *Service) AIJobFinished(ctx context.Context, org, conversationID int64) {
	if s.notifier == nil {
		return
	}
	conv, err := s.store.Q().GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: org, ID: conversationID})
	if err != nil {
		s.log.Warn("ai_job_live_lookup_failed", "org", org, "conversation_id", conversationID, "error", err)
		return
	}
	s.notify(ctx, org, model.LiveEvent{Type: model.EventAIJobFinished, ConversationUUID: conv.Uuid})
}
