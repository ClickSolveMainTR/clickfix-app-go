package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/twilioutil"
	"github.com/google/uuid"
)

func TestProvidersAndConsentGate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/sms/rest/v2/msgheader":
			_, _ = w.Write([]byte(`{"code":"00","msgheader":["CF"]}`))
		case "/sms/rest/v2/send":
			var v map[string]any
			_ = json.NewDecoder(r.Body).Decode(&v)
			if v["iysfilter"] != "11" {
				t.Error("Netgsm commercial filter missing")
			}
			_, _ = w.Write([]byte(`{"code":"00","jobid":"job1"}`))
		case "/v1/get-balance/json":
			_, _ = w.Write([]byte(`{"response":{"status":{"code":200}}}`))
		case "/v1/send-sms/json":
			var v struct {
				Request struct {
					Order struct {
						IYS string `json:"iys"`
					} `json:"order"`
				} `json:"request"`
			}
			_ = json.NewDecoder(r.Body).Decode(&v)
			if v.Request.Order.IYS != "1" {
				t.Error("IYS filter missing")
			}
			_, _ = w.Write([]byte(`{"response":{"status":{"code":200},"order":{"id":12}}}`))
		default:
			if strings.HasSuffix(r.URL.Path, "Messages.json") {
				_ = r.ParseForm()
				if r.Form.Get("To") != "+905551112233" {
					t.Error("phone not normalized")
				}
				_, _ = w.Write([]byte(`{"sid":"SM1"}`))
			} else {
				_, _ = w.Write([]byte(`{}`))
			}
		}
	}))
	defer server.Close()
	for _, provider := range []string{"netgsm", "iletimerkezi", "twilio"} {
		t.Run(provider, func(t *testing.T) {
			inst := channels.Instance{UUID: uuid.New(),
				Credentials: channels.Credentials{"provider": provider,
					"usercode":  "u",
					"password":  "pw",
					"msgheader": "CF",
					"api_key":   "key",
					"api_hash":  "hash",
					"account_sid": "AC" + strings.Repeat("1",
						32),
					"auth_token":  "token",
					"from_number": "+905551110000"},
				Settings: map[string]any{"commercial": true}}
			cfg := Config{NetgsmURL: server.URL,
				IletiURL:      server.URL,
				TwilioURL:     server.URL,
				PublicBaseURL: "https://api.example"}
			a := New(cfg)
			if _, err := a.Connect(ctx, inst); err != nil {
				t.Fatal(err)
			}
			msg := channels.OutboundMessage{MessageUUID: uuid.New(),
				To:   channels.Recipient{Phone: "05551112233"},
				Text: "Destek"}
			before := requests
			if _, err := a.Send(ctx, inst, msg); !channels.IsPermanent(err) {
				t.Fatalf("commercial must fail closed: %v", err)
			}
			if requests != before {
				t.Fatal("provider called without consent")
			}
			cfg.Consent = func(context.Context, channels.Instance, string) (bool, error) { return false, nil }
			a = New(cfg)
			if _, err := a.Send(ctx, inst, msg); !channels.IsPermanent(err) {
				t.Fatal("denied consent sent")
			}
			cfg.Consent = func(context.Context, channels.Instance, string) (bool, error) { return true, nil }
			a = New(cfg)
			sent, err := a.Send(ctx, inst, msg)
			if err != nil || sent.ExternalID == "" {
				t.Fatalf("send: %+v %v", sent, err)
			}
		})
	}
}
func TestTwilioInboundAndDelivery(t *testing.T) {
	t.Parallel()
	a := New(Config{PublicBaseURL: "https://api.example"})
	inst := channels.Instance{Credentials: channels.Credentials{"provider": "twilio",
		"account_sid": "AC1",
		"auth_token":  "token",
		"from_number": "+905551110000"}}
	f := url.Values{"AccountSid": {"AC1"},
		"MessageSid": {"SM1"},
		"From":       {"+905551112233"},
		"To":         {"+905551110000"},
		"Body":       {"Yardım"}}
	r := httptest.NewRequest("POST", "https://api.example/webhook", nil)
	r.Header.Set("X-Twilio-Signature", twilioutil.Sign("token", "https://api.example/webhook", f))
	if err := a.VerifyWebhook(r, []byte(f.Encode()), inst); err != nil {
		t.Fatal(err)
	}
	out, err := a.ParseWebhook(context.Background(), r, []byte(f.Encode()), inst)
	if err != nil || len(out.Messages) != 1 || out.Messages[0].From.Phone != "+905551112233" {
		t.Fatalf("parse: %+v %v", out, err)
	}
	f.Set("Body", "changed")
	if a.VerifyWebhook(r, []byte(f.Encode()), inst) == nil {
		t.Fatal("tampered SMS accepted")
	}
	f.Set("MessageStatus", "delivered")
	out, err = a.ParseWebhook(context.Background(), r, []byte(f.Encode()), inst)
	if err != nil || len(out.Statuses) != 1 || out.Statuses[0].Status != channels.StatusDelivered {
		t.Fatal("delivery not normalized")
	}
}
func TestIYSConsentStatus(t *testing.T) {
	t.Parallel()
	state := "RET"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			_, _ = w.Write([]byte(`{"access_token":"token"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing IYS token")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": state})
	}))
	defer s.Close()
	check := IYSConsentCheck(nil, s.URL)
	inst := channels.Instance{Credentials: channels.Credentials{"iys_username": "u",
		"iys_password":   "p",
		"iys_code":       "1",
		"iys_brand_code": "2"}}
	ok, err := check(context.Background(), inst, "+905551112233")
	if err != nil || ok {
		t.Fatal("RET allowed")
	}
	state = "ONAY"
	ok, err = check(context.Background(), inst, "+905551112233")
	if err != nil || !ok {
		t.Fatalf("ONAY refused: %v", err)
	}
}

func TestNetgsmReportsAndInboundToken(t *testing.T) {
	ctx := context.Background()
	var inboxDenied atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sms/rest/v2/report":
			_, _ = fmt.Fprint(w, `{"code":"00","response":{"jobs":[{"jobid":"fixture-job","number":"5550000000","status":1,"deliveredDate":"2026-10-06 12:00:00"}]}}`)
		case "/sms/rest/v2/inbox":
			if inboxDenied.Load() {
				_, _ = fmt.Fprint(w, `{"code":"30"}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"code":"00","messages":[{"id":121386491,"kayittar":"1770000000000","mesaj":"Fixture inbound","gonderen":"5550000000"}]}`)
		case "/sms/rest/v2/send":
			_, _ = fmt.Fprint(w, `{"code":"00","jobid":"fixture-job"}`)
		default:
			t.Error("unexpected request")
		}
	}))
	defer server.Close()
	a := New(Config{NetgsmURL: server.URL})
	inst := channels.Instance{Credentials: channels.Credentials{"provider": "netgsm", "usercode": "fixture", "password": "fixture", "msgheader": "Fixture", "webhook_token": strings.Repeat("x", 32)}}
	b, err := os.ReadFile("testdata/sms_send_minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Payload struct {
			Phone string `json:"recipient_phone"`
			Text  string `json:"message"`
		} `json:"payload"`
	}
	if err = json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	sent, err := a.Send(ctx, inst, channels.OutboundMessage{To: channels.Recipient{Phone: fixture.Payload.Phone}, Text: fixture.Payload.Text})
	if err != nil || sent.ExternalID != "fixture-job" {
		t.Fatal(err)
	}
	result, err := a.PollNetgsm(ctx, inst, []string{sent.ExternalID})
	if err != nil || len(result.Statuses) != 1 || result.Statuses[0].Status != channels.StatusDelivered || len(result.Messages) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	inboxDenied.Store(true)
	partial, partialErr := a.PollNetgsm(ctx, inst, []string{sent.ExternalID})
	if partialErr == nil || len(partial.Statuses) != 1 {
		t.Fatal("inbox permission failure discarded delivery reports")
	}
	inboxDenied.Store(false)
	req := httptest.NewRequest("POST", "https://example.test/hook?token="+strings.Repeat("x", 32), nil)
	if err = a.VerifyWebhook(req, nil, inst); err != nil {
		t.Fatal(err)
	}
	req.URL.RawQuery = "token=wrong"
	if a.VerifyWebhook(req, nil, inst) == nil {
		t.Fatal("bad token accepted")
	}
	for _, b := range []string{`{"messageId":"121386491","sourceNumber":"5550000000","subscriberNumber":"8500000000","content":"Fixture inbound","messageDateTime":"2026-10-06 12:00:00"}`, "messageId=121386491&sourceNumber=5550000000&subscriberNumber=8500000000&content=Fixture+inbound"} {
		result, err = a.ParseWebhook(ctx, req, []byte(b), inst)
		if err != nil || len(result.Messages) != 1 || result.Messages[0].ExternalID != "121386491" {
			t.Fatalf("%+v %v", result, err)
		}
	}
}
