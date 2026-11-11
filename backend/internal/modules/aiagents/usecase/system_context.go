package usecase

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	systems "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/screenctx"
)

// maxInputBytes mirrors aicore RunInput's limit.
const maxInputBytes = 24000

// screenOf is the screen the customer wrote from (CF-197): the page and
// tracked events the widget stored on the inbound message.
func screenOf(msg db.Message) *screenctx.Screen {
	var attrs struct {
		PageURL string          `json:"page_url"`
		Events  json.RawMessage `json:"events"`
	}
	if json.Unmarshal(msg.ContentAttributes, &attrs) != nil {
		return nil
	}
	return screenctx.Parse(attrs.PageURL, attrs.Events, msg.CreatedAt.Time)
}

// systemContext is the connected system context of the message's triage
// intent. The connected system is optional: an error is logged and the
// reply goes on without it (the model is told nothing it could guess from).
func (s *Service) systemContext(ctx context.Context, org int64, conv db.Conversation, message int64) systems.AIContext {
	if s.Systems == nil {
		return systems.AIContext{}
	}
	intent, err := s.triageIntent(ctx, org, conv.Uuid, message)
	if err != nil || intent == "" {
		return systems.AIContext{}
	}
	out, err := s.Systems.Context(ctx, org, conv.ID, intent)
	if err != nil {
		slog.WarnContext(ctx, "ai_system_context_unavailable", "error", err)
		return systems.AIContext{}
	}
	return out
}

// systemTools are the matched playbook's AI-allowed actions.
func (s *Service) systemTools(ctx context.Context, org int64, sc systems.AIContext) []ai.Tool {
	if s.Systems == nil || sc.Playbook == nil || len(sc.Playbook.ActionKeys) == 0 {
		return nil
	}
	tools, err := s.Systems.Tools(ctx, org, sc.Playbook.ActionKeys)
	if err != nil {
		slog.WarnContext(ctx, "ai_system_tools_unavailable", "error", err)
		return nil
	}
	return tools
}

func clipInput(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	for len(string(r)) > n {
		r = r[:len(r)-len(r)/10-1]
	}
	return string(r)
}
