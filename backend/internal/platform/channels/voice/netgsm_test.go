package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

func TestSantralScenariosJSONAndForm(t *testing.T) {
	a := New(Config{})
	inst := channels.Instance{Credentials: channels.Credentials{"provider": "netgsm"}}
	ctx := context.Background()
	for scenario := range santralScenarios {
		t.Run(scenario, func(t *testing.T) {
			p := map[string]any{"scenario": scenario,
				"unique_id":      "fixture-call",
				"customer_num":   "05551112233",
				"internal_num":   "101",
				"yon":            "1",
				"timestamp":      "2026-10-06 12:00:00",
				"callQueuedTime": "2026-10-06 12:00:01",
				"callEndedTime":  "2026-10-06 12:01:00",
				"talktime":       15,
				"seskaydi":       "https://record.netgsm.com.tr/fixture.wav"}
			b, _ := json.Marshal(p)
			form := url.Values{}
			for k, v := range p {
				form.Set(k, fmt.Sprint(v))
			}
			var previous string
			for _, body := range []string{string(b), form.Encode()} {
				r, e := a.ParseWebhook(ctx, nil, []byte(body), inst)
				if e != nil || len(r.Calls) != 1 || len(r.Messages) != 1 {
					t.Fatalf("%+v %v", r, e)
				}
				c := r.Calls[0]
				if c.InternalNumber != "101" || c.DurationSeconds != 15 || c.EndedAt.IsZero() {
					t.Fatalf("%+v", c)
				}
				if previous != "" && previous == r.Messages[0].ExternalID {
					t.Fatal("number/string canonical payloads deliberately differ")
				}
				previous = r.Messages[0].ExternalID
				if !strings.HasPrefix(r.Messages[0].From.Phone, "+90") {
					t.Fatal("phone normalization")
				}
			}
		})
	}
}
func TestSantralDuplicateAndSparseLifecycle(t *testing.T) {
	a := New(Config{}).WithCallLookup(func(context.Context, channels.Instance, string) (channels.CallEvent, error) {
		return channels.CallEvent{From: "05551112233", To: "101", Direction: "inbound"}, nil
	})
	inst := channels.Instance{Credentials: channels.Credentials{"provider": "netgsm"}}
	b := []byte(`{"scenario":"Answer","unique_id":"one","timestamp":"1791280800000"}`)
	r, e := a.netgsmEvent(context.Background(), b, inst)
	if e != nil || len(r.Messages) != 1 || r.Calls[0].Status != "in-progress" || r.Calls[0].AnsweredAt.IsZero() {
		t.Fatalf("%+v %v", r, e)
	}
	same, _ := a.netgsmEvent(context.Background(), b, inst)
	if same.Messages[0].ExternalID != r.Messages[0].ExternalID {
		t.Fatal("duplicate hash changed")
	}
	unknown, e := a.netgsmEvent(context.Background(), []byte(`{"scenario":"unexpected"}`), inst)
	if e != nil || len(unknown.Calls) != 0 {
		t.Fatal("unknown must be ignored")
	}
}
func TestRecordingURLAllowlist(t *testing.T) {
	for _, u := range []string{"http://rec.netgsm.com.tr/a",
		"https://netgsm.com.tr.evil.test/a",
		"https://evilnetgsm.com.tr/a",
		"https://user:pass@netgsm.com.tr/a",
		"https://netgsm.com.tr:444/a",
		"https://127.0.0.1/a",
		"file:///tmp/a"} {
		if assertRecordingURL(u) == nil {
			t.Fatalf("accepted %s", u)
		}
	}
	for _, u := range []string{"https://netgsm.com.tr/a", "https://record.netgsm.com.tr/a"} {
		if e := assertRecordingURL(u); e != nil {
			t.Fatal(e)
		}
	}
}
func TestCDRAndBalanceContracts(t *testing.T) {
	fixture, e := os.ReadFile("testdata/call_report_minimal.json")
	if e != nil {
		t.Fatal(e)
	}
	var contract struct {
		Payload struct {
			StartDate string `json:"start_date"`
		} `json:"payload"`
	}
	if e = json.Unmarshal(fixture, &contract); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]any
		_ = json.NewDecoder(r.Body).Decode(&p)
		switch r.URL.Path {
		case "/netsantral/report":
			if p["startdate"] != "061020260000" || p["stopdate"] != "061020262359" {
				t.Error("wrong Istanbul window")
			}
			_, _ = fmt.Fprint(w,
				`[{"uniqueid":"call-fixture","values":[{"direction":2,"duration":0,"source":"05551112233","destination":"101","date":"2026-10-06 12:00:00"}]}]`)
		case "/balance":
			if p["stip"] != float64(2) && p["stip"] != float64(1) {
				t.Error("wrong stip")
			}
			_, _ = fmt.Fprint(w, `{"code":"00","balance":"1.234,56"}`)
		default:
			t.Error("unexpected URL")
		}
	}))
	defer server.Close()
	a := New(Config{NetgsmURL: server.URL})
	inst := channels.Instance{Credentials: channels.Credentials{"provider": "netgsm",
		"usercode": "fixture",
		"password": "fixture"}}
	day, _ := time.Parse("2006-01-02", contract.Payload.StartDate)
	r, e := a.ReconcileNetgsm(context.Background(), inst, day)
	if e != nil || len(r.Calls) != 1 || r.Calls[0].Status != "no-answer" || !r.Calls[0].ProviderFinal {
		t.Fatalf("%+v %v", r, e)
	}
	health, e := a.CheckHealth(context.Background(), inst)
	if e != nil || !health.OK || health.Detail != "Bakiye: 1.234,56 TL" {
		t.Fatalf("%+v %v", health, e)
	}
	for code := range cdrErrors {
		r, e := a.parseCDR(context.Background(), inst, []byte(code))
		if code == "40" {
			if e != nil || len(r.Calls) != 0 {
				t.Fatal("40 means empty")
			}
		} else if e == nil {
			t.Fatalf("%s accepted", code)
		}
	}
}

func TestNetgsmBalanceFormats(t *testing.T) {
	fixture, err := os.ReadFile("testdata/account_balance_minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Operation string `json:"operation"`
	}
	if json.Unmarshal(fixture, &envelope) != nil || envelope.Operation != "account.balance" {
		t.Fatal("invalid balance contract")
	}
	for _, body := range []string{`{"code":"00","balance":"1.234,56"}`, `{"code":0,"amount":1234.56}`, `00 1234,56`} {
		p := parseNetgsmBalanceResponse([]byte(body))
		if !validNetgsmAmount(p.Amount) {
			t.Fatalf("invalid balance %q", body)
		}
	}
	p := parseNetgsmBalanceResponse([]byte(`{"code":"00","balance":[{"amount":1500,"balance_name":"SMS"}]}`))
	if len(p.Packages) != 1 || p.Packages[0] != "1500 SMS" {
		t.Fatalf("%+v", p)
	}
	for _, bad := range []string{"", "NaN", "Inf", "-Inf", "broken"} {
		if validNetgsmAmount(bad) {
			t.Fatal("invalid amount accepted")
		}
	}
}

type recordingTransport func(*http.Request) (*http.Response, error)

func (f recordingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestNetgsmRecordingDownloadBounds(t *testing.T) {
	for _, test := range []struct {
		name string
		code int
		mime string
		size int64
		body string
		ok   bool
	}{{"audio",
		200,
		"audio/wav",
		4,
		"RIFF",
		true},
		{"redirect",
			302,
			"audio/wav",
			4,
			"RIFF",
			false},
		{"oversize",
			200,
			"audio/wav",
			101 << 20,
			"RIFF",
			false},
		{"html",
			200,
			"text/html",
			4,
			"html",
			false},
		{"empty",
			200,
			"audio/wav",
			0,
			"",
			false}} {
		t.Run(test.name, func(t *testing.T) {
			hits := 0
			client := &http.Client{Transport: recordingTransport(func(r *http.Request) (*http.Response, error) {
				hits++
				if deadline, ok := r.Context().Deadline(); !ok || time.Until(deadline) > 61*time.Second {
					t.Error("download deadline missing")
				}
				return &http.Response{StatusCode: test.code,
						Header: http.Header{"Content-Type": {test.mime},
							"Location": {"https://evil.test/secret"}},
						ContentLength: test.size,
						Body:          io.NopCloser(strings.NewReader(test.body)),
						Request:       r},
					nil
			})}
			a := New(Config{RecordingClient: client})
			body, err := a.netgsmRecording("https://record.netgsm.com.tr/fixture.wav")(context.Background())
			if (err == nil) != test.ok {
				t.Fatalf("err=%v", err)
			}
			if body != nil {
				_ = body.Close()
			}
			if hits != 1 {
				t.Fatal("redirect followed")
			}
		})
	}
}
