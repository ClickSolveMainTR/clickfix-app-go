package usecase

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/repository"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
)

// liveRecorder records AIJobFinished calls and the job status the database
// holds at that moment (the event must follow the commit).
type liveRecorder struct {
	mu       sync.Mutex
	q        *db.Queries
	convUUID uuid.UUID
	calls    []int64
	statuses []string
}

func (l *liveRecorder) AIJobFinished(ctx context.Context, org, conversationID int64) {
	rows, _ := l.q.AIAgentJobs(ctx, db.AIAgentJobsParams{OrganizationID: org, Uuid: l.convUUID, Limit: 1})
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, conversationID)
	if len(rows) == 1 {
		l.statuses = append(l.statuses, rows[0].Status)
	}
}

// CF-246: when a draft-mode reply job finishes, whether as a draft or as an
// unsourced handoff, the conversation's agents hear about it live, after
// the outcome is committed. The handoff keeps the model's text for the
// agent; nothing is posted to the customer.
func TestJobOutcomeIsPublishedLive(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		reply      model.Reply
		wantStatus string
	}{
		"unsourced handoff keeps the text": {reply: model.Reply{Text: "Belki yarın gelir", Summary: "s", Confidence: 0.3, Citations: []uuid.UUID{}}, wantStatus: "handoff"},
		"grounded draft":                   {wantStatus: "draft"},
	} {
		t.Run(name, func(t *testing.T) {
			engine, fake, org, actor := agentCoreFixture(t)
			chunk := uuid.New()
			engine.RAG = fakeSources{citation: kbmodel.Citation{ChunkUUID: chunk, ArticleUUID: uuid.New(), Version: 1, Content: "Kargo 2 günde teslim edilir", Locale: "tr"}}
			if tc.reply.Text == "" {
				tc.reply = model.Reply{Text: "Kargonuz 2 günde gelir", Summary: "s", Confidence: 0.95, Citations: []uuid.UUID{chunk}}
			}
			raw, _ := json.Marshal(tc.reply)
			fake.response = string(raw)
			c, msg := conversationFixture(t, engine, org)
			bot := &fakeBot{}
			svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, bot)
			live := &liveRecorder{q: engine.Repo.Billing.Q, convUUID: c.Uuid}
			svc.Live = live
			cfg := model.DefaultConfig()
			cfg.Mode = "draft"
			cfg.Channels = []string{"whatsapp"}
			if _, err := svc.Repo.SaveConfig(ctx, org, actor, cfg); err != nil {
				t.Fatal(err)
			}
			if err := svc.Inbound(ctx, org, msg.Uuid); err != nil {
				t.Fatal(err)
			}
			rows, err := svc.Repo.Q.AIAgentJobs(ctx, db.AIAgentJobsParams{OrganizationID: org, Uuid: c.Uuid, Limit: 5})
			if err != nil || len(rows) != 1 || rows[0].Status != tc.wantStatus {
				t.Fatalf("jobs=%+v err=%v", rows, err)
			}
			var stored model.Reply
			if err := json.Unmarshal(rows[0].Result, &stored); err != nil || stored.Text != tc.reply.Text {
				t.Fatalf("result=%s err=%v", rows[0].Result, err)
			}
			live.mu.Lock()
			defer live.mu.Unlock()
			// Triage finishes through the same path; the reply job is last.
			if len(live.calls) == 0 || live.calls[len(live.calls)-1] != c.ID {
				t.Fatalf("live calls=%v want conversation %d", live.calls, c.ID)
			}
			if got := live.statuses[len(live.statuses)-1]; got != tc.wantStatus {
				t.Fatalf("published before commit: status %q", got)
			}
			if bot.calls != 0 {
				t.Fatal("draft mode must not post to the customer")
			}
		})
	}
}

// New picks the live notifier up from a sender that implements it.
func TestNewWiresLiveFromSender(t *testing.T) {
	sender := &liveSender{}
	if svc := New(nil, nil, sender); svc.Live != sender {
		t.Fatal("Live not set from sender")
	}
	if svc := New(nil, nil, &fakeBot{}); svc.Live != nil {
		t.Fatal("Live set from a plain sender")
	}
}

type liveSender struct{ fakeBot }

func (*liveSender) AIJobFinished(context.Context, int64, int64) {}
