package usecase

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	agentmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/testchannel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type aiWindowAdapter struct{ *testchannel.Adapter }

func (a aiWindowAdapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{ReplyWindow: 24 * time.Hour}
}

type aiTestInstances struct{ instance channels.Instance }

func (f aiTestInstances) Instance(context.Context, int64, int64) (channels.Instance, channels.Adapter, error) {
	return f.instance, aiWindowAdapter{Adapter: testchannel.New()}, nil
}

type aiSendQueue struct{ count int }

func (q *aiSendQueue) EnqueueSend(context.Context, int64, int64, time.Duration) error {
	q.count++
	return nil
}

func TestBotPostingLocksHumanAndWindowChecks(t *testing.T) {
	for _, tc := range []struct {
		name          string
		human         bool
		stale         bool
		expired       bool
		policyChanged bool
		allowed       bool
	}{
		{name: "one bot reply", allowed: true}, {name: "human public reply", human: true}, {name: "new inbound", stale: true},
		{name: "24h window closed", expired: true}, {name: "policy changed", policyChanged: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := os.Getenv("TEST_DATABASE_URL")
			if database == "" {
				t.Skip("TEST_DATABASE_URL required")
			}
			ctx := context.Background()
			pool, err := pgxpool.New(ctx, database)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			var org, contact, instance, conversation int64
			if err = pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Bot gate') RETURNING id", uuid.NewString()).Scan(&org); err != nil {
				t.Fatal(err)
			}
			if err = pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Customer') RETURNING id", org).Scan(&contact); err != nil {
				t.Fatal(err)
			}
			if err = pool.QueryRow(ctx, "INSERT INTO channel_instances(organization_id,channel_type,name) VALUES($1,'test','Bot test') RETURNING id", org).Scan(&instance); err != nil {
				t.Fatal(err)
			}
			inbound := time.Now()
			if tc.expired {
				inbound = inbound.Add(-25 * time.Hour)
			}
			if err = pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,contact_id,channel_instance_id,channel_type,source,last_inbound_at) VALUES($1,1,$2,$3,'test','inbound',$4) RETURNING id", org, contact, instance, inbound).Scan(&conversation); err != nil {
				t.Fatal(err)
			}
			var trigger uuid.UUID
			if err = pool.QueryRow(ctx, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,status,content) VALUES($1,$2,'in','contact','received','Question') RETURNING uuid", org, conversation).Scan(&trigger); err != nil {
				t.Fatal(err)
			}
			if tc.human || tc.stale {
				direction, sender, status := "out", "agent", "queued"
				if tc.stale {
					direction, sender, status = "in", "contact", "received"
				}
				if _, err = pool.Exec(ctx, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,status,content) VALUES($1,$2,$3,$4,$5,'New message')", org, conversation, direction, sender, status); err != nil {
					t.Fatal(err)
				}
			}
			cfg := agentmodel.DefaultConfig()
			cfg.Mode = "autonomous"
			cfg.Channels = []string{"test"}
			raw, _ := json.Marshal(cfg)
			if _, err = pool.Exec(ctx, "INSERT INTO ai_agent_configs(organization_id,config) VALUES($1,$2)", org, raw); err != nil {
				t.Fatal(err)
			}
			queue := &aiSendQueue{}
			svc := New(repository.New(pool, db.New(pool)), aiTestInstances{instance: channels.Instance{ID: instance, OrganizationID: org, Status: "active", Type: "test"}}, nil).WithQueue(queue)
			revision := int32(1)
			if tc.policyChanged {
				revision = 2
			}
			id, err := svc.PostAI(ctx, org, trigger, "Grounded answer", revision)
			if tc.allowed {
				if err != nil || id == uuid.Nil {
					t.Fatalf("id=%s err=%v", id, err)
				}
				replay, err := svc.PostAI(ctx, org, trigger, "Grounded answer", revision)
				if err != nil || replay != id || queue.count != 1 {
					t.Fatalf("replay=%s count=%d err=%v", replay, queue.count, err)
				}
			} else if err == nil || queue.count != 0 {
				t.Fatalf("unsafe bot message queued: count=%d err=%v", queue.count, err)
			}
		})
	}
}
