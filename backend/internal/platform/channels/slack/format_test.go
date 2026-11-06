package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
)

// CF-177: the text is mrkdwn with Slack's control characters escaped.
func TestSendTranslatesFormat(t *testing.T) {
	t.Parallel()
	var text string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		text, _ = b["text"].(string)
		_, _ = w.Write([]byte(`{"ok":true,"ts":"124.1"}`))
	}))
	defer srv.Close()
	a := New(Config{SigningSecret: "s", BaseURL: srv.URL})
	inst := channels.Instance{Credentials: channels.Credentials{"bot_token": "xoxb-test"}, Settings: map[string]any{"channel_ids": []string{"C1"}}}
	_, err := a.Send(context.Background(), inst, channels.OutboundMessage{MessageUUID: uuid.New(), ThreadID: "C1:123.1",
		Text: "**Çözüldü** <b> & [kayıt](https://ex.com/a_b?x=1&y=2)\n- *bir*"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "*Çözüldü* &lt;b&gt; &amp; <https://ex.com/a_b?x=1&amp;y=2|kayıt>\n• _bir_"; text != want {
		t.Fatalf("text = %q", text)
	}
}
