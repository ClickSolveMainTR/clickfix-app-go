package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/autoreply"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
)

// autoPoster stores automatic replies as the inbox core does (a bot message
// with its auto_reply kind), so the real auto-reply rules run on top.
type autoPoster struct {
	t     *testing.T
	pool  *pgxpool.Pool
	posts []int64
}

func (p *autoPoster) PostAuto(ctx context.Context, org, conv int64, kind, content string) (uuid.UUID, error) {
	attrs, _ := json.Marshal(map[string]string{"auto_reply": kind})
	var id uuid.UUID
	if err := p.pool.QueryRow(ctx, `INSERT INTO messages(organization_id,conversation_id,direction,sender_type,kind,status,content,content_attributes)
		VALUES($1,$2,'out','bot','text','queued',$3,$4) RETURNING uuid`, org, conv, content, attrs).Scan(&id); err != nil {
		p.t.Fatal(err)
	}
	p.posts = append(p.posts, conv)
	return id, nil
}

type handoffCase struct {
	mode        string
	text        string
	spoke       bool   // the agent answered earlier in the conversation
	human       bool   // a person replied before the message
	humanMidway bool   // a person replies while the agent generates
	disabled    bool   // the organization turned the handoff notice off
	aiErr       error  // the AI call fails
	followUps   int    // further customer messages after the first
	wantNotices int    // handoff notices posted
	wantReason  string // reason of the first job ("" skips)
}

func runHandoffCase(t *testing.T, tc handoffCase) ([]int64, []db.AiAgentJob) {
	t.Helper()
	ctx := context.Background()
	raw, _ := json.Marshal(model.Reply{Text: "Unsupported draft answer", Summary: "s", Confidence: 0.3, Citations: []uuid.UUID{}})
	// Each case gets its own organization (the fixture numbers
	// conversations from 1).
	engine, fake, org, actor := agentCoreFixture(t)
	fake.response = string(raw)
	fake.err = tc.aiErr
	engine.RAG = fakeSources{citation: kbmodel.Citation{ChunkUUID: uuid.New(), ArticleUUID: uuid.New(), Version: 1, Content: "Other topic", Locale: "tr"}}
	pool := engine.Repo.Billing.Pool
	c, msg := conversationFixture(t, engine, org)
	if _, err := pool.Exec(ctx, "UPDATE messages SET content = $3 WHERE organization_id = $1 AND id = $2", org, msg.ID, tc.text); err != nil {
		t.Fatal(err)
	}
	insert := func(sender, content, attrs string) {
		if _, err := pool.Exec(ctx, `INSERT INTO messages(organization_id,conversation_id,direction,sender_type,kind,status,content,content_attributes)
			VALUES($1,$2,'out',$3,'text','sent',$4,$5)`, org, c.ID, sender, content, attrs); err != nil {
			t.Fatal(err)
		}
	}
	if tc.spoke {
		insert("bot", "Earlier AI answer", `{"ai_trigger_message_uuid":"x"}`)
	}
	if tc.human {
		insert("agent", "Merhaba, ben bakıyorum", `{}`)
	}
	if tc.humanMidway {
		fake.hook = func() { insert("agent", "Ben ilgileniyorum", `{}`) }
	}
	poster := &autoPoster{t: t, pool: pool}
	notices := autoreply.New(engine.Repo.Billing.Q, poster, nil, nil, nil)
	if tc.disabled {
		settings := autoreply.Defaults()
		settings.Handoff.Enabled = false
		if _, err := notices.Put(ctx, org, settings); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, &fakeBot{})
	svc.Notices = notices
	cfg := model.DefaultConfig()
	cfg.Mode = tc.mode
	cfg.Channels = []string{"whatsapp"}
	if _, err := svc.Repo.SaveConfig(ctx, org, actor, cfg); err != nil {
		t.Fatal(err)
	}
	if err := svc.Inbound(ctx, org, msg.Uuid); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < tc.followUps; i++ {
		var next uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO messages(organization_id,conversation_id,direction,sender_type,sender_contact_id,kind,status,content)
			VALUES($1,$2,'in','contact',$3,'text','received','Hâlâ bekliyorum') RETURNING uuid`, org, c.ID, c.ContactID).Scan(&next); err != nil {
			t.Fatal(err)
		}
		if err := svc.Inbound(ctx, org, next); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := svc.Repo.Q.AIAgentJobs(ctx, db.AIAgentJobsParams{OrganizationID: org, Uuid: c.Uuid, Limit: 10})
	if err != nil || len(rows) != 1+tc.followUps {
		t.Fatalf("jobs=%+v err=%v", rows, err)
	}
	for _, r := range rows {
		if r.Status != "handoff" {
			t.Fatalf("every job hands off: %+v", r)
		}
	}
	return poster.posts, rows
}

// CF-198, CF-244: when an autonomous agent hands a conversation to a person
// the customer hears about it once: when they asked for a person, when the
// agent had answered them before, and when the agent could not answer
// safely (no sources, low confidence, AI failure), which used to leave the
// customer in silence after the acknowledgement. The model's unchecked text
// is never sent. A draft-mode agent never speaks to the customer; a person
// who already replied, or the organization's own setting, keeps it quiet.
func TestHandoffNotifiesTheCustomer(t *testing.T) {
	lowConfidence := "Low confidence or insufficient public sources"
	for name, tc := range map[string]handoffCase{
		"customer asked for a person":    {mode: "autonomous", text: "Bir temsilci ile görüşmek istiyorum", wantNotices: 1, wantReason: "Customer requested a human" + model.HandoffNoticeSent},
		"answered before, unsure now":    {mode: "autonomous", text: "Kargom nerede?", spoke: true, wantNotices: 1, wantReason: lowConfidence + model.HandoffNoticeSent},
		"never answered, no sources":     {mode: "autonomous", text: "Kargom nerede?", wantNotices: 1, wantReason: lowConfidence + model.HandoffNoticeSent},
		"notice once per conversation":   {mode: "autonomous", text: "Kargom nerede?", followUps: 2, wantNotices: 1},
		"AI unavailable":                 {mode: "autonomous", text: "Kargom nerede?", aiErr: errors.New("provider down"), wantNotices: 1, wantReason: "AI unavailable, guard rejected, or credits exhausted" + model.HandoffNoticeSent},
		"draft mode stays quiet":         {mode: "draft", text: "Kargom nerede?", wantNotices: 0, wantReason: lowConfidence},
		"draft mode, asked for person":   {mode: "draft", text: "temsilci lütfen", spoke: true, wantNotices: 0},
		"a person already replied":       {mode: "autonomous", text: "Kargom nerede?", human: true, wantNotices: 0, wantReason: "Human public reply exists in this conversation"},
		"a person replies meanwhile":     {mode: "autonomous", text: "Kargom nerede?", humanMidway: true, wantNotices: 0, wantReason: lowConfidence},
		"organization turned notice off": {mode: "autonomous", text: "Kargom nerede?", disabled: true, wantNotices: 0, wantReason: lowConfidence},
	} {
		t.Run(name, func(t *testing.T) {
			posts, rows := runHandoffCase(t, tc)
			if len(posts) != tc.wantNotices {
				t.Fatalf("notices=%v want %d", posts, tc.wantNotices)
			}
			// Jobs come newest first; the first message's job is last.
			first := rows[len(rows)-1]
			if tc.wantReason != "" && first.Reason != tc.wantReason {
				t.Fatalf("reason %q want %q", first.Reason, tc.wantReason)
			}
			for _, r := range rows[:len(rows)-1] {
				if strings.HasSuffix(r.Reason, model.HandoffNoticeSent) {
					t.Fatalf("a follow-up must not send a second notice: %q", r.Reason)
				}
			}
		})
	}
}

// closedHours: the organization is outside its working hours.
type closedHours struct{}

func (closedHours) HoursState(context.Context, int64, time.Time) (businesshours.State, error) {
	return businesshours.State{Open: false}, nil
}

// CF-243: a channel outside the agent's scope and a message outside working
// hours hand off with different reasons.
func TestInactiveHandoffReasons(t *testing.T) {
	ctx := context.Background()
	run := func(channels []string, hoursOnly bool) string {
		engine, _, org, actor := agentCoreFixture(t)
		c, msg := conversationFixture(t, engine, org)
		svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, &fakeBot{})
		svc.Hours = closedHours{}
		cfg := model.DefaultConfig()
		cfg.Mode = "autonomous"
		cfg.Channels = channels
		cfg.BusinessHoursOnly = hoursOnly
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
		return rows[0].Reason
	}
	if got := run([]string{"email"}, false); got != model.ReasonChannelNotEnabled {
		t.Fatalf("channel not in scope: reason %q", got)
	}
	if got := run([]string{"whatsapp"}, true); got != model.ReasonOutsideBusinessHours {
		t.Fatalf("outside hours: reason %q", got)
	}
}

// CF-243: connecting a channel adds its type to an active agent's scope
// once; a missing or off config stays untouched.
func TestChannelConnectedEnablesType(t *testing.T) {
	ctx := context.Background()
	engine, _, org, actor := agentCoreFixture(t)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, &fakeBot{})
	svc.ChannelConnected(ctx, org, "email")
	if _, revision, err := svc.Repo.Config(ctx, org); err != nil || revision != 0 {
		t.Fatalf("missing config must stay missing: revision=%d err=%v", revision, err)
	}
	cfg := model.DefaultConfig()
	if _, err := svc.Repo.SaveConfig(ctx, org, actor, cfg); err != nil {
		t.Fatal(err)
	}
	svc.ChannelConnected(ctx, org, "email")
	if got, _, _ := svc.Repo.Config(ctx, org); len(got.Channels) != 0 {
		t.Fatalf("off config must stay untouched: %v", got.Channels)
	}
	cfg.Mode = "draft"
	cfg.Channels = []string{"whatsapp"}
	if _, err := svc.Repo.SaveConfig(ctx, org, actor, cfg); err != nil {
		t.Fatal(err)
	}
	svc.ChannelConnected(ctx, org, "email")
	svc.ChannelConnected(ctx, org, "email")
	got, _, err := svc.Repo.Config(ctx, org)
	if err != nil || len(got.Channels) != 2 || got.Channels[0] != "whatsapp" || got.Channels[1] != "email" {
		t.Fatalf("channels=%v err=%v", got.Channels, err)
	}
}
