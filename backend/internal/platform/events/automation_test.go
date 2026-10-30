package events

import (
	"context"
	"testing"
)

func TestAutomationTraceIsolation(t *testing.T) {
	trace := []string{"first"}
	ctx := WithAutomationTrace(context.Background(), trace)
	trace[0] = "changed"
	out := AutomationTrace(ctx)
	out[0] = "changed again"
	if AutomationTrace(ctx)[0] != "first" {
		t.Fatal("trace aliasing")
	}
	if DomainAliases()[ConversationsCreated] != ConversationCreated {
		t.Fatal("producer alias missing")
	}
}
func TestInboxAliasExpandsStatusAndPreservesStableIds(t *testing.T) {
	e := New(ConversationsUpdated).WithPayload(map[string]any{"change": "status_changed", "to": "snoozed|2026-10-30T12:00:00Z"})
	one, two := ExpandDomainAlias(e), ExpandDomainAlias(e)
	if len(one) != 3 || one[0].Name != ConversationUpdated || one[1].Name != ConversationStatusChanged || one[2].Name != ConversationSnoozed {
		t.Fatal(one)
	}
	if one[1].EventID == one[0].EventID || one[1].EventID != two[1].EventID {
		t.Fatal("derived event ids are not stable/distinct")
	}
}
