package teams

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
)

// CF-177: the activity is CommonMark with textFormat "extendedmarkdown";
// when Teams refuses it the same message goes out as plain text.
func TestSendTranslatesFormatAndFallsBack(t *testing.T) {
	t.Parallel()
	var activities []map[string]any
	reject, rejectStatus := false, 400
	client := clientFunc(func(r *http.Request) (*http.Response, error) {
		status, body := 200, `{"access_token":"access","token_type":"Bearer"}`
		if !strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token") {
			var a map[string]any
			_ = json.NewDecoder(r.Body).Decode(&a)
			activities = append(activities, a)
			body = `{"id":"reply"}`
			if reject && a["textFormat"] != "plain" {
				status, body = rejectStatus, `{"error":{"code":"BadSyntax"}}`
			}
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	a := New(Config{Client: client, KeysURL: "https://keys.example/keys"})
	inst := channels.Instance{Credentials: channels.Credentials{"app_id": uuid.NewString(), "app_password": "secret", "tenant_id": uuid.NewString()}}
	ref, _ := json.Marshal(thread{Service: "https://smba.trafficmanager.net/emea/", Conversation: "conversation", Root: "root"})
	msg := channels.OutboundMessage{ThreadID: base64.RawURLEncoding.EncodeToString(ref), Text: "**Çözüldü**\n2*3*4 ve [kayıt](https://ex.com/a_b)"}

	if out, err := a.Send(context.Background(), inst, msg); err != nil || out.ExternalID != "conversation:reply" {
		t.Fatalf("send: %+v %v", out, err)
	}
	if got := activities[0]; got["textFormat"] != "extendedmarkdown" || got["text"] != "**Çözüldü**  \n2\\*3\\*4 ve [kayıt](https://ex.com/a_b)" {
		t.Fatalf("activity = %+v", got)
	}

	reject, activities = true, nil
	if out, err := a.Send(context.Background(), inst, msg); err != nil || out.ExternalID != "conversation:reply" {
		t.Fatalf("fallback: %+v %v", out, err)
	}
	if len(activities) != 2 || activities[1]["textFormat"] != "plain" || activities[1]["text"] != "Çözüldü\n2*3*4 ve kayıt (https://ex.com/a_b)" {
		t.Fatalf("activities = %+v", activities)
	}

	// 403 is not a format problem: no second request.
	rejectStatus, activities = 403, nil
	if _, err := a.Send(context.Background(), inst, msg); !channels.IsPermanent(err) || len(activities) != 1 {
		t.Fatalf("403: %v, %d requests", err, len(activities))
	}
}
