package inbox_test

import (
	"context"
	"testing"

	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/testchannel"
	"github.com/google/uuid"
)

type meteredChannel struct{ *testchannel.Adapter }

func (a meteredChannel) Send(ctx context.Context, inst channels.Instance, m channels.OutboundMessage) (channels.SendResult, error) {
	r, err := a.Adapter.Send(ctx, inst, m)
	r.Usage = &channels.Usage{Feature: "sms", Quantity: 1}
	return r, err
}
func TestAcceptedChannelUsageWrittenOnce(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.ib.Registry.Register(meteredChannel{Adapter: e.ib.Test})
	if code := e.post(t, `{"messages":[{"id":"meter-1","from":{"id":"phone-1"},"text":"Yardım"}]}`, ""); code != 200 {
		t.Fatalf("inbound %d", code)
	}
	var conversation uuid.UUID
	if err := e.pool.QueryRow(ctx, "SELECT uuid FROM conversations WHERE organization_id=$1", e.org).Scan(&conversation); err != nil {
		t.Fatal(err)
	}
	result, err := e.ib.Conversations.Post(ctx, convusecase.PostInput{OrganizationID: e.org, ConversationUUID: conversation, AuthorID: e.agent, Content: "Yanıt", ClientID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	var messageID int64
	if err := e.pool.QueryRow(ctx, "SELECT id FROM messages WHERE organization_id=$1 AND uuid=$2", e.org, result.Message.UUID).Scan(&messageID); err != nil {
		t.Fatal(err)
	}
	if err := e.ib.Deliver(ctx, e.org, messageID, 0, 3); err != nil {
		t.Fatal(err)
	}
	var n int
	err = e.pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_name='billing.usage_recorded' AND payload->'data'->>'event_key'=$1 AND payload->'data'->>'feature'='sms'`, "channel-send:"+result.Message.UUID.String()).Scan(&n)
	if err != nil || n != 1 {
		t.Fatalf("usage events %d: %v", n, err)
	}
}
