package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	agentmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// PostAI rechecks policy, human activity and delivery windows under the same
// conversation lock used by human posts. A deterministic client ID ties exactly
// one bot reply to an inbound message. Actual delivery rechecks channel windows.
func (s *Service) PostAI(ctx context.Context, org int64, trigger uuid.UUID, content string, revision int32) (uuid.UUID, error) {
	if s.queue == nil || s.instances == nil || strings.TrimSpace(content) == "" || len(content) > 10000 {
		return uuid.Nil, ErrInvalid
	}
	msg, err := s.store.Q().GetMessageByUUID(ctx, db.GetMessageByUUIDParams{OrganizationID: org, Uuid: trigger})
	if err != nil {
		return uuid.Nil, notFound(err)
	}
	if msg.Direction != "in" || msg.Private || msg.DeletedAt.Valid {
		return uuid.Nil, ErrInvalid
	}
	var app repository.Appended
	err = s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		policy, err := tx.Q.AIAgentLockConfig(ctx, org)
		if err != nil {
			return err
		}
		var cfg agentmodel.Config
		if err = json.Unmarshal(policy.Config, &cfg); err != nil {
			return err
		}
		conv, err := tx.Q.LockConversation(ctx, db.LockConversationParams{OrganizationID: org, ID: msg.ConversationID})
		if err != nil {
			return err
		}
		if reason := cfg.InactiveReason(conv.ChannelType, s.open(ctx, org)); reason != "" {
			return fmt.Errorf("%w: %s", ErrInvalid, reason)
		}
		if policy.Revision != revision || cfg.Mode != "autonomous" {
			return ErrInvalid
		}
		if conv.AssigneeUserID.Valid || conv.Status == model.StatusClosed || conv.Status == model.StatusResolved || !conv.ChannelInstanceID.Valid {
			return ErrInvalid
		}
		human, err := tx.Q.AIAgentHumanSince(ctx, db.AIAgentHumanSinceParams{OrganizationID: org, ConversationID: conv.ID, TriggerID: 0})
		if err != nil {
			return err
		}
		newer, err := tx.Q.AIAgentNewerInbound(ctx, db.AIAgentNewerInboundParams{OrganizationID: org, ConversationID: conv.ID, TriggerID: msg.ID})
		if err != nil {
			return err
		}
		if human || newer {
			return fmt.Errorf("%w: conversation changed during AI generation", ErrInvalid)
		}
		inst, adapter, err := s.instances.Instance(ctx, org, conv.ChannelInstanceID.Int64)
		if err != nil {
			return err
		}
		if inst.Status != "active" || !channels.WindowOpen(adapter.Capabilities(), conv.LastInboundAt.Time, s.now()) {
			return channels.ErrWindowClosed
		}
		client := uuid.NewSHA1(trigger, []byte("ai-reply"))
		attrs, _ := json.Marshal(map[string]any{"ai_trigger_message_uuid": trigger, "ai_config_revision": revision})
		app, err = tx.AppendMessage(ctx, repository.NewMessage{OrganizationID: org, ConversationID: conv.ID,
			InstanceID: conv.ChannelInstanceID.Int64, Direction: model.DirectionOut, SenderType: model.SenderBot,
			Kind: model.KindText, Content: content, Status: model.MessageQueued, ReplyToID: msg.ID,
			ClientID: pgtype.UUID{Bytes: client, Valid: true}, At: s.now(), Attributes: attrs,
		})
		if err != nil || app.Duplicate {
			return err
		}
		return s.emit(ctx, tx, events.MessagesSent, org, 0, app.Conversation, map[string]any{"message_uuid": app.Message.Uuid.String(), "ai": true, "trigger_uuid": trigger.String()})
	})
	if err != nil {
		return uuid.Nil, err
	}
	if !app.Duplicate {
		if err = s.queueSend(ctx, org, app.Message.ID, 0); err != nil {
			s.log.Error("ai_reply_enqueue_failed", "message_uuid", app.Message.Uuid, "error", err)
		}
		s.notify(ctx, org, model.LiveEvent{Type: model.EventMessageCreated, ConversationUUID: app.Conversation.Uuid})
		s.indexed(ctx, app.Conversation.Uuid, app.Message.Uuid, uuid.Nil)
	}
	return app.Message.Uuid, nil
}
