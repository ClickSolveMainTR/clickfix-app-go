package inbox_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
)

// CF-246: a finished AI agent job reaches the conversation's inbox channel
// as "ai.job_finished", so an open composer shows the draft without a
// reload. Another organization's conversation id publishes nothing.
func TestAIJobFinishedLiveEvent(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if code := e.post(t, `{"messages":[{"id":"ai-1","from":{"id":"v-1","name":"Ada"},"text":"Kargom nerede?"}]}`, ""); code != http.StatusOK {
		t.Fatalf("webhook = %d", code)
	}
	var convID int64
	var convUUID uuid.UUID
	if err := e.pool.QueryRow(ctx, `SELECT id, uuid FROM conversations WHERE organization_id = $1`, e.org).Scan(&convID, &convUUID); err != nil {
		t.Fatal(err)
	}
	before := len(e.live.types())
	e.ib.Conversations.AIJobFinished(ctx, e.org, convID)
	e.live.mu.Lock()
	got := append([]convmodel.LiveEvent(nil), e.live.events[before:]...)
	e.live.mu.Unlock()
	if len(got) != 1 || got[0].Type != convmodel.EventAIJobFinished || got[0].ConversationUUID != convUUID || got[0].Message != nil {
		t.Fatalf("live events = %+v", got)
	}
	e.ib.Conversations.AIJobFinished(ctx, e.org+1_000_000, convID)
	if n := len(e.live.types()); n != before+1 {
		t.Fatalf("foreign organization published: %v", e.live.types()[before:])
	}
}
