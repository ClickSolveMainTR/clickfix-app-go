package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
)

// PostAuto posts an automatic customer-facing message (CF-198: the
// acknowledgement, the out-of-hours notice or the AI-to-human handoff
// notice) as a bot message on a conversation. kind is stored in the
// message attributes (auto_reply) so a conversation gets each kind at most
// as often as the auto-reply rules allow. Unlike PostAI it is independent
// of the AI policy; like PostAI it never stamps the conversation's first
// response (only agents do), so SLA clocks keep running.
func (s *Service) PostAuto(ctx context.Context, org, conversationID int64, kind, content string) (uuid.UUID, error) {
	content = strings.TrimSpace(content)
	if kind == "" || content == "" || len(content) > 4000 {
		return uuid.Nil, ErrInvalid
	}
	if s.queue == nil || s.instances == nil {
		return uuid.Nil, fmt.Errorf("%w: delivery unavailable", ErrInvalid)
	}
	var app repository.Appended
	err := s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		conv, err := tx.Q.LockConversation(ctx, db.LockConversationParams{OrganizationID: org, ID: conversationID})
		if err != nil {
			return notFound(err)
		}
		if conv.Status == model.StatusClosed || !conv.ChannelInstanceID.Valid {
			return fmt.Errorf("%w: conversation cannot receive messages", ErrInvalid)
		}
		inst, adapter, err := s.instances.Instance(ctx, org, conv.ChannelInstanceID.Int64)
		if err != nil {
			return err
		}
		if inst.Status != "active" || !channels.WindowOpen(adapter.Capabilities(), conv.LastInboundAt.Time, s.now()) {
			return channels.ErrWindowClosed
		}
		attrs, _ := json.Marshal(map[string]any{"auto_reply": kind})
		app, err = tx.AppendMessage(ctx, repository.NewMessage{
			OrganizationID: org, ConversationID: conv.ID, InstanceID: conv.ChannelInstanceID.Int64,
			Direction: model.DirectionOut, SenderType: model.SenderBot, Kind: model.KindText, Content: content,
			Status: model.MessageQueued, At: s.now(), Attributes: attrs,
		})
		if err != nil || app.Duplicate {
			return err
		}
		return s.emit(ctx, tx, events.MessagesSent, org, 0, app.Conversation, map[string]any{
			"message_uuid": app.Message.Uuid.String(), "auto_reply": kind,
		})
	})
	if err != nil {
		return uuid.Nil, err
	}
	if !app.Duplicate {
		if err = s.queueSendPriority(ctx, org, app.Message.ID); err != nil {
			s.log.Error("auto_reply_enqueue_failed", "message_uuid", app.Message.Uuid, "error", err)
		}
		s.notify(ctx, org, model.LiveEvent{Type: model.EventMessageCreated, ConversationUUID: app.Conversation.Uuid})
		s.indexed(ctx, app.Conversation.Uuid, app.Message.Uuid, uuid.Nil)
	}
	return app.Message.Uuid, nil
}
