package usecase

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func pgInt8(v int64) pgtype.Int8 {
	if v == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

func pgTime(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func pgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func uuidPtr(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	v := uuid.UUID(u.Bytes)
	return &v
}

func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}

func decodeJSON(raw []byte) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil || v == nil {
		return map[string]any{}
	}
	return v
}

// toMessage maps a stored message to its API shape.
func toMessage(m db.Message, conv uuid.UUID, sender model.Sender) model.Message {
	attrs := decodeJSON(m.ContentAttributes)
	// Group channels (Slack, Teams, Telegram groups) make the shared chat the
	// contact; the person who wrote the message travels as author_name.
	if obj, ok := attrs.(map[string]any); ok && sender.Type == model.SenderContact {
		if author, _ := obj["author_name"].(string); strings.TrimSpace(author) != "" {
			sender.Name = strings.TrimSpace(author)
		}
	}
	return model.Message{
		UUID: m.Uuid, ConversationUUID: conv, Direction: m.Direction, Sender: sender, Kind: m.Kind,
		Content: m.Content, Attributes: attrs, Private: m.Private, Status: m.Status,
		Error: m.Error, ClientID: uuidPtr(m.ClientID), Mentions: []model.Ref{}, Attachments: []model.MessageAttachment{},
		SentAt: timePtr(m.SentAt), DeliveredAt: timePtr(m.DeliveredAt), ReadAt: timePtr(m.ReadAt),
		CreatedAt: m.CreatedAt.Time.UTC(),
	}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func fullName(name, surname pgtype.Text) string {
	return strings.TrimSpace(name.String + " " + surname.String)
}
