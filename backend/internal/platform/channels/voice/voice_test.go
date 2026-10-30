package voice

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/twilioutil"
	"github.com/google/uuid"
)

func TestTwilioLifecycleRecordingAndTTS(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rid := "RE" + strings.Repeat("a", 32)
	sid := "AC" + strings.Repeat("1", 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/Calls.json") {
			_ = r.ParseForm()
			if strings.Contains(r.Form.Get("Twiml"), "<Hangup>") || !strings.Contains(r.Form.Get("Twiml"), "&lt;Hangup&gt;") {
				t.Error("TwiML text not escaped")
			}
			_, _ = w.Write([]byte(`{"sid":"CA1"}`))
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/Recordings/"+rid+".wav") {
			t.Errorf("untrusted recording URL fetched: %s", r.URL)
		}
		u, p, ok := r.BasicAuth()
		if !ok || u != sid || p != "token" {
			t.Error("recording auth missing")
		}
		_, _ = w.Write([]byte("wave"))
	}))
	defer server.Close()
	a := New(Config{PublicBaseURL: "https://api.example", TwilioURL: server.URL})
	inst := channels.Instance{UUID: uuid.New(),
		OrganizationID: 12,
		ID:             34,
		Credentials: channels.Credentials{"provider": "twilio",
			"account_sid": sid,
			"auth_token":  "token",
			"from_number": "+905551110000"}}
	f := url.Values{"AccountSid": {sid},
		"CallSid":    {"CA1"},
		"From":       {"+905551112233"},
		"To":         {"+905551110000"},
		"CallStatus": {"ringing"}}
	r := httptest.NewRequest("POST", "https://api.example/hook", nil)
	r.Header.Set("X-Twilio-Signature", twilioutil.Sign("token", "https://api.example/hook", f))
	b := []byte(f.Encode())
	if err := a.VerifyWebhook(r, b, inst); err != nil {
		t.Fatal(err)
	}
	out, err := a.ParseWebhook(ctx, r, b, inst)
	if err != nil || len(out.Calls) != 1 || out.Calls[0].ExternalID != "CA1" || out.Messages[0].ThreadID != "CA1" {
		t.Fatalf("call: %+v %v", out, err)
	}
	// Recording callbacks omit From/To. Lookup must remain inside this instance/org.
	a.WithCallLookup(func(_ context.Context, i channels.Instance, id string) (channels.CallEvent, error) {
		if i.OrganizationID != 12 || i.ID != 34 || id != "CA1" {
			t.Error("recording lookup escaped instance")
		}
		return channels.CallEvent{From: "+905551112233", To: "+905551110000", Direction: "inbound"}, nil
	})
	recording := url.Values{"CallSid": {"CA1"},
		"RecordingSid":    {rid},
		"RecordingStatus": {"completed"},
		"RecordingUrl":    {"https://127.0.0.1/steal"}}
	out, err = a.ParseWebhook(ctx, r, []byte(recording.Encode()), inst)
	if err != nil || len(out.Messages[0].Attachments) != 1 || out.Calls[0].RecordingMessageExternalID != "recording:"+rid {
		t.Fatalf("recording %+v %v", out, err)
	}
	stream, err := out.Messages[0].Attachments[0].Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(stream)
	_ = stream.Close()
	if string(content) != "wave" {
		t.Fatal("recording missing")
	}
	sent, err := a.Send(ctx,
		inst,
		channels.OutboundMessage{To: channels.Recipient{Phone: "+905551112233"},
			Text: "<Hangup>"})
	if err != nil || sent.ExternalID != "CA1" {
		t.Fatalf("call send %+v %v", sent, err)
	}
}
func TestNetgsmCDRToken(t *testing.T) {
	t.Parallel()
	a := New(Config{})
	inst := channels.Instance{Credentials: channels.Credentials{"provider": "netgsm",
		"webhook_token": strings.Repeat("a",
			32)}}
	r := httptest.NewRequest("POST", "https://api.example/hook?token="+strings.Repeat("a", 32), nil)
	b := []byte(`{"uniqueid":"call1","source":"05551112233","destination":"08501110000","direction":1,"duration":15}`)
	if err := a.VerifyWebhook(r, b, inst); err != nil {
		t.Fatal(err)
	}
	out, err := a.ParseWebhook(context.Background(), r, b, inst)
	if err != nil || out.Calls[0].DurationSeconds != 15 {
		t.Fatalf("CDR %+v %v", out, err)
	}
	r.URL.RawQuery = "token=wrong"
	if a.VerifyWebhook(r, b, inst) == nil {
		t.Fatal("unsigned CDR accepted")
	}
	if _, err := a.Send(context.Background(), inst, channels.OutboundMessage{Text: "call"}); !channels.IsPermanent(err) {
		t.Fatal("unimplemented telephony claimed success")
	}
}
