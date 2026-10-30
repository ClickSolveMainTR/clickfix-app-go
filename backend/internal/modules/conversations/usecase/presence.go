package usecase

import (
	"context"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/google/uuid"
)

// PresenceTTL is how long a viewer stays listed without a heartbeat.
const PresenceTTL = 30 * time.Second

// PresenceStore keeps who is looking at (and typing in) a conversation
// (CF-49). Implemented on Redis by the inbox package.
type PresenceStore interface {
	Touch(ctx context.Context, orgID int64, conversation uuid.UUID, v model.Viewer) error
	Leave(ctx context.Context, orgID int64, conversation uuid.UUID, user uuid.UUID) error
	List(ctx context.Context, orgID int64, conversation uuid.UUID) ([]model.Viewer, error)
}

// WithPresence sets the presence store.
func (s *Service) WithPresence(p PresenceStore) *Service { s.presence = p; return s }

// Heartbeat records that actor views the conversation (typing when they
// are writing a reply or note) and returns everyone viewing it. Viewers are
// broadcast on the inbox channel so open screens update without polling.
func (s *Service) Heartbeat(ctx context.Context, orgID int64, conversation uuid.UUID, actor Actor, typing bool) ([]model.Viewer, error) {
	if _, err := s.store.Q().GetConversationByUUID(ctx, dbConvParams(orgID, conversation)); err != nil {
		return nil, notFound(err)
	}
	if s.presence == nil {
		return []model.Viewer{}, nil
	}
	actor = s.Named(ctx, actor)
	if err := s.presence.Touch(ctx, orgID, conversation, model.Viewer{UUID: actor.UUID, Name: actor.Name, Typing: typing, At: s.now()}); err != nil {
		return nil, err
	}
	return s.broadcastViewers(ctx, orgID, conversation)
}

// LeaveConversation removes actor from the viewers.
func (s *Service) LeaveConversation(ctx context.Context, orgID int64, conversation uuid.UUID, actor Actor) error {
	if s.presence == nil {
		return nil
	}
	if err := s.presence.Leave(ctx, orgID, conversation, actor.UUID); err != nil {
		return err
	}
	_, err := s.broadcastViewers(ctx, orgID, conversation)
	return err
}

func (s *Service) broadcastViewers(ctx context.Context, orgID int64, conversation uuid.UUID) ([]model.Viewer, error) {
	viewers, err := s.presence.List(ctx, orgID, conversation)
	if err != nil {
		return nil, err
	}
	s.notify(ctx, orgID, model.LiveEvent{Type: model.EventConversationViewers, ConversationUUID: conversation, Viewers: viewers})
	return viewers, nil
}

func dbConvParams(orgID int64, id uuid.UUID) db.GetConversationByUUIDParams {
	return db.GetConversationByUUIDParams{OrganizationID: orgID, Uuid: id}
}
