package usecase

import (
	"context"
	"encoding/json"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
)

// AppointmentEvent writes an appointment lifecycle event (CF-138: booked,
// rescheduled, confirmed, completed, no-show, cancelled) into a
// conversation's timeline. attrs carry "event" plus the appointment fields
// the timeline shows; the actor is filled in here.
func (s *Service) AppointmentEvent(ctx context.Context, orgID, convID int64, actor Actor, attrs map[string]any) error {
	actor = s.Named(ctx, actor)
	out := map[string]any{"actor_uuid": actor.UUID, "actor_name": actor.Name}
	for k, v := range attrs {
		out[k] = v
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	var conv db.Conversation
	var msg db.Message
	err = s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		c, err := tx.Q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: orgID, ID: convID})
		if err != nil {
			return notFound(err)
		}
		conv = c
		app, err := tx.AppendMessage(ctx, repository.NewMessage{
			OrganizationID: orgID, ConversationID: convID, Direction: model.DirectionInternal, SenderType: model.SenderSystem,
			SenderUserID: actor.ID, Kind: model.KindEvent, Attributes: raw, At: s.now(),
		})
		msg = app.Message
		return err
	})
	if err != nil {
		return err
	}
	s.publishTimeline(ctx, orgID, conv, []db.Message{msg})
	return nil
}
