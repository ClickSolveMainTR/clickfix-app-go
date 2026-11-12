package assistant

import (
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

// History budget of one reply (CF-211). The last verbatimMessages
// messages go to the model as they are (each clipped); older ones are
// condensed into one data turn of short lines, so a long chat costs a few
// hundred tokens instead of growing with every reply.
const (
	verbatimMessages = 6
	verbatimRunes    = 1500
	condensedRunes   = 160
)

// historyTurns turns stored messages (oldest first) into model turns.
func historyTurns(history []db.AssistantMessage) []ai.Turn {
	msgs := make([]db.AssistantMessage, 0, len(history))
	for _, m := range history {
		if (m.Role == "user" || m.Role == "assistant") && strings.TrimSpace(m.Content) != "" {
			msgs = append(msgs, m)
		}
	}
	turns := []ai.Turn{}
	cut := max(0, len(msgs)-verbatimMessages)
	// The verbatim part starts with a user message (providers expect the
	// transcript to open with the user).
	for cut < len(msgs) && msgs[cut].Role != "user" {
		cut++
	}
	if cut > 0 {
		var b strings.Builder
		b.WriteString("Earlier in this chat (condensed, oldest first):\n")
		for _, m := range msgs[:cut] {
			b.WriteString(m.Role)
			b.WriteString(": ")
			b.WriteString(strings.Join(strings.Fields(clipRunes(m.Content, condensedRunes)), " "))
			b.WriteString("\n")
		}
		turns = append(turns, ai.Turn{Role: "user", Text: ai.ConversationData(b.String())})
	}
	for _, m := range msgs[cut:] {
		turns = append(turns, ai.Turn{Role: m.Role, Text: clipRunes(m.Content, verbatimRunes)})
	}
	return turns
}

// lastUserText is the user's previous message: with the new one it
// decides which tools to load (a follow-up like "evet, yap" keeps the
// topic of the message before it).
func lastUserText(history []db.AssistantMessage) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			return history[i].Content
		}
	}
	return ""
}
