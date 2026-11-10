package usecase

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/repository"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
)

type fakeNotices struct{ conversations []int64 }

func (f *fakeNotices) NotifyHandoff(_ context.Context, _ int64, conv int64) error {
	f.conversations = append(f.conversations, conv)
	return nil
}

// CF-198: when an autonomous agent hands a conversation to a person the
// customer hears about it: always when they asked for a person, otherwise
// only when the agent had answered them before. A draft-mode agent never
// spoke to the customer, so it stays silent.
func TestHandoffNotifiesTheCustomer(t *testing.T) {
	ctx := context.Background()
	raw, _ := json.Marshal(model.Reply{Text: "Supported answer", Summary: "s", Confidence: 0.2, Citations: []uuid.UUID{}})
	run := func(mode, text string, spoke bool) []int64 {
		// Each case gets its own organization (the fixture numbers
		// conversations from 1).
		engine, fake, org, actor := agentCoreFixture(t)
		fake.response = string(raw)
		engine.RAG = fakeSources{citation: kbmodel.Citation{ChunkUUID: uuid.New(), ArticleUUID: uuid.New(), Version: 1, Content: "Supported answer", Locale: "tr"}}
		c, msg := conversationFixture(t, engine, org)
		if _, err := engine.Repo.Billing.Pool.Exec(ctx, "UPDATE messages SET content = $3 WHERE organization_id = $1 AND id = $2", org, msg.ID, text); err != nil {
			t.Fatal(err)
		}
		if spoke {
			if _, err := engine.Repo.Billing.Pool.Exec(ctx, `INSERT INTO messages(organization_id,conversation_id,direction,sender_type,kind,status,content,content_attributes)
				VALUES($1,$2,'out','bot','text','sent','Earlier AI answer','{"ai_trigger_message_uuid":"x"}')`, org, c.ID); err != nil {
				t.Fatal(err)
			}
		}
		notices := &fakeNotices{}
		svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, &fakeBot{})
		svc.Notices = notices
		cfg := model.DefaultConfig()
		cfg.Mode = mode
		cfg.Channels = []string{"whatsapp"}
		if _, err := svc.Repo.SaveConfig(ctx, org, actor, cfg); err != nil {
			t.Fatal(err)
		}
		if err := svc.Inbound(ctx, org, msg.Uuid); err != nil {
			t.Fatal(err)
		}
		rows, err := svc.Repo.Q.AIAgentJobs(ctx, db.AIAgentJobsParams{OrganizationID: org, Uuid: c.Uuid, Limit: 5})
		if err != nil || len(rows) != 1 || rows[0].Status != "handoff" {
			t.Fatalf("jobs=%+v err=%v", rows, err)
		}
		t.Logf("%s/%q spoke=%v: reason=%s notices=%v", mode, text, spoke, rows[0].Reason, notices.conversations)
		return notices.conversations
	}
	if got := run("autonomous", "Bir temsilci ile görüşmek istiyorum", false); len(got) != 1 {
		t.Fatalf("customer asked for a person: notices=%v", got)
	}
	if got := run("autonomous", "Kargom nerede?", true); len(got) != 1 {
		t.Fatalf("agent had answered before, low confidence now: notices=%v", got)
	}
	if got := run("autonomous", "Kargom nerede?", false); len(got) != 0 {
		t.Fatalf("agent never spoke: no notice, got %v", got)
	}
	if got := run("draft", "temsilci lütfen", true); len(got) != 0 {
		t.Fatalf("draft mode never talks to the customer: got %v", got)
	}
}
