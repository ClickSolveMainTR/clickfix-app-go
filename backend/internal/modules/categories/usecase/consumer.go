package usecase

import (
	"context"
	"errors"
	"log/slog"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/categories/classify"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/jackc/pgx/v5"
)

// Consumer classifies a conversation when its first inbound customer
// message arrives. A conversation that already has a category (machine or
// human) is left alone; agents re-run classification from the inbox.
func (s *Service) Consumer() eventdispatch.Consumer {
	return eventdispatch.Consumer{Name: "categories.classify", Events: []string{events.MessageReceived}, Handle: func(ctx context.Context, ev events.Event) error {
		if ev.TenantID == nil {
			return nil
		}
		p, err := events.DecodePayload[events.MessagePayload](ev)
		if err != nil || p.Private || p.Direction == "outbound" || p.ConversationID <= 0 {
			return nil
		}
		org := *ev.TenantID
		if existing, err := s.assignment(ctx, org, classify.SubjectConversation, p.ConversationID); err != nil || existing != nil {
			return err
		}
		conv, err := s.q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: org, ID: p.ConversationID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		sub, err := conversationSource{q: s.q}.system(ctx, org, conv)
		if err != nil {
			return err
		}
		_, err = s.classifySubject(ctx, org, 0, classify.SubjectConversation, sub, false)
		switch {
		case errors.Is(err, ErrNoCategories), errors.Is(err, ErrNoText):
			return nil
		case err != nil:
			slog.WarnContext(ctx, "categories: classify failed", "organization_id", org, "conversation_id", p.ConversationID, "error", err)
			return err
		}
		return nil
	}}
}
