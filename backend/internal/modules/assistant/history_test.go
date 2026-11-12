package assistant

import (
	"fmt"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

func TestHistoryTurnsCondenseOldMessages(t *testing.T) {
	msgs := []db.AssistantMessage{}
	for i := 0; i < 20; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs = append(msgs, db.AssistantMessage{Role: role, Content: fmt.Sprintf("mesaj %d %s", i, strings.Repeat("uzun ", 600))})
	}
	msgs = append(msgs, db.AssistantMessage{Role: "tool", Content: "kart sonucu"})
	turns := historyTurns(msgs)
	if len(turns) != 1+verbatimMessages {
		t.Fatalf("turns %d", len(turns))
	}
	if turns[0].Role != "user" || !strings.Contains(turns[0].Text, "condensed") || !strings.Contains(turns[0].Text, "mesaj 0") || strings.Contains(turns[0].Text, "mesaj 14") {
		t.Fatalf("condensed turn %q", turns[0].Text[:200])
	}
	if turns[1].Role != "user" || !strings.HasPrefix(turns[1].Text, "mesaj 14") {
		t.Fatalf("verbatim part starts with %q", turns[1].Text[:20])
	}
	for _, tr := range turns[1:] {
		if len([]rune(tr.Text)) > verbatimRunes {
			t.Fatal("verbatim message not clipped")
		}
	}
	if short := historyTurns(msgs[:4]); len(short) != 4 || strings.Contains(short[0].Text, "condensed") {
		t.Fatal("short history changed")
	}
	if lastUserText(msgs) != msgs[18].Content {
		t.Fatal("last user text")
	}
}
