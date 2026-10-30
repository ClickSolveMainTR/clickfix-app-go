// Package voice handles authenticated Twilio and Netsantral lifecycle events,
// recording attachments and CDR reconciliation. The browser webphone uses direct
// SIP over WSS; server-side voice SMS and streaming AI are separate capabilities.
package voice

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/channelhttp"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/twilioutil"
)

type Config struct {
	PublicBaseURL   string
	TwilioURL       string
	NetgsmURL       string
	Client          channelhttp.Client
	RecordingClient *http.Client
}
type Adapter struct {
	cfg    Config
	lookup CallLookup
}

func New(cfg Config) *Adapter {
	if cfg.Client == nil {
		cfg.Client = channelhttp.DefaultClient()
	}
	if cfg.NetgsmURL == "" {
		cfg.NetgsmURL = "https://api.netgsm.com.tr"
	}
	if cfg.TwilioURL == "" {
		cfg.TwilioURL = "https://api.twilio.com"
	}
	return &Adapter{cfg: cfg}
}
func (*Adapter) Type() string { return channels.TypeVoice }
func (*Adapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{Threading: true, MaxTextLength: 1000}
}
func (*Adapter) Fields() []channels.Field {
	return channelhttp.Fields("provider", "account_sid auth_token from_number webhook_token usercode password", "auth_token webhook_token usercode password")
}
func (a *Adapter) Connect(ctx context.Context, inst channels.Instance) (string, error) {
	switch inst.Credentials["provider"] {
	case "twilio":
		return "", twilioutil.Check(ctx, a.cfg.Client, a.cfg.TwilioURL, inst)
	case "netgsm":
		if len(inst.Credentials["webhook_token"]) < 32 {
			return "", errors.New("netgsm callback token must have at least 32 characters")
		}
		_, err := a.netgsmBalance(ctx, inst)
		return "", err
	default:
		return "", errors.New("voice provider must be twilio or netgsm")
	}
}
func (a *Adapter) CheckHealth(ctx context.Context, inst channels.Instance) (channels.Health, error) {
	if inst.Credentials["provider"] == "netgsm" {
		detail, err := a.netgsmBalance(ctx, inst)
		return channels.Health{OK: err == nil, Detail: detail}, err
	}
	_, err := a.Connect(ctx, inst)
	return channels.Health{OK: err == nil}, err
}
func (a *Adapter) VerifyWebhook(r *http.Request, b []byte, inst channels.Instance) error {
	if inst.Credentials["provider"] == "twilio" {
		return twilioutil.Verify(r, b, inst, a.cfg.PublicBaseURL)
	}
	if inst.Credentials["provider"] != "netgsm" {
		return channels.ErrSignature
	}
	token := inst.Credentials["webhook_token"]
	expected, actual := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(r.URL.Query().Get("token")))
	if len(token) < 32 || subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
		return channels.ErrSignature
	}
	return nil
}
func (*Adapter) AcknowledgeWebhook(_ *http.Request, _ []byte, inst channels.Instance) (string, string) {
	if inst.Credentials["provider"] == "netgsm" {
		return "application/json", `{"received":true}`
	}
	// Twilio expects empty TwiML acknowledgment.
	return "application/xml", "<?xml version=\"1.0\" encoding=\"UTF-8\"?><Response></Response>"
}

var recordingID = regexp.MustCompile(`^RE[0-9a-fA-F]{32}$`)

func (a *Adapter) ParseWebhook(ctx context.Context,
	r *http.Request,
	b []byte,
	inst channels.Instance) (channels.WebhookResult, error) {
	if inst.Credentials["provider"] == "netgsm" {
		if r != nil && strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			p, err := multipartPayload(r.Header.Get("Content-Type"), b)
			if err != nil {
				return channels.WebhookResult{}, err
			}
			return a.normalizeNetgsm(ctx, p, inst, false)
		}
		return a.netgsmEvent(ctx, b, inst)
	}
	f, err := url.ParseQuery(string(b))
	if err != nil {
		return channels.WebhookResult{}, err
	}
	call := f.Get("CallSid")
	if call == "" {
		return channels.WebhookResult{}, errors.New("missing CallSid")
	}
	if f.Get("RecordingSid") != "" && (f.Get("From") == "" || f.Get("To") == "") {
		if a.lookup == nil {
			return channels.WebhookResult{}, errors.New("recording call lookup unavailable")
		}
		previous, err := a.lookup(ctx, inst, call)
		if err != nil {
			return channels.WebhookResult{}, err
		}
		f.Set("From", previous.From)
		f.Set("To", previous.To)
		f.Set("Direction", previous.Direction)
	}
	direction := "inbound"
	if strings.HasPrefix(f.Get("Direction"), "outbound") {
		direction = "outbound"
	}
	from, to := twilioutil.E164(f.Get("From")), twilioutil.E164(f.Get("To"))
	own := twilioutil.E164(inst.Credentials["from_number"])
	if own == "" || direction == "inbound" && to != own || direction == "outbound" && from != own {
		return channels.WebhookResult{}, errors.New("voice destination does not belong to instance")
	}
	status := f.Get("CallStatus")
	if status == "" {
		status = "recording"
	}
	duration, _ := strconv.ParseInt(f.Get("CallDuration"), 10, 32)
	if duration < 0 {
		duration = 0
	}
	ev := channels.CallEvent{ExternalID: call,
		Provider:        "twilio",
		Direction:       direction,
		Status:          status,
		From:            from,
		To:              to,
		DurationSeconds: int32(duration),
		At:              time.Now().UTC()}
	out := channels.WebhookResult{Calls: []channels.CallEvent{ev}}
	phone := from
	if direction == "outbound" {
		phone = to
	}
	if phone == "" {
		return channels.WebhookResult{}, errors.New("missing caller number")
	}
	id := "call:" + call
	m := channels.InboundMessage{ExternalID: id,
		From: channels.Sender{ExternalID: phone,
			Phone: phone},
		Kind:     "text",
		Text:     "Arama: " + status,
		ThreadID: call,
		At:       ev.At,
		Attributes: map[string]any{"call_id": call,
			"call_status": status,
			"direction":   direction}}
	if rid := f.Get("RecordingSid"); rid != "" && f.Get("RecordingStatus") == "completed" {
		if !recordingID.MatchString(rid) {
			return channels.WebhookResult{}, errors.New("invalid RecordingSid")
		}
		m.ExternalID = "recording:" + rid
		m.Kind = "media"
		m.Text = "Görüşme kaydı"
		recordingSeconds, _ := strconv.ParseInt(f.Get("RecordingDuration"), 10, 32)
		if recordingSeconds > 0 {
			m.Attributes["recording_duration_seconds"] = recordingSeconds
		}
		out.Calls[0].RecordingMessageExternalID = m.ExternalID
		m.Attachments = []channels.InboundAttachment{{Filename: rid + ".wav",
			ContentType: "audio/wav",
			Open: a.recording(inst,
				rid)}}
	} else {
		m.ExternalID = id + ":" + status
	}
	out.Messages = []channels.InboundMessage{m}
	return out, nil
}
func (a *Adapter) recording(inst channels.Instance, rid string) func(context.Context) (io.ReadCloser, error) {
	return func(ctx context.Context) (io.ReadCloser, error) {
		target := a.cfg.TwilioURL + twilioutil.AccountPath(inst) + "/Recordings/" + rid + ".wav"
		req, err := twilioutil.Request(ctx, inst, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
		res, err := a.cfg.Client.Do(req)
		if err != nil {
			return nil, errors.New("recording download failed")
		}
		if res.StatusCode != http.StatusOK || res.ContentLength > 25<<20 {
			_ = res.Body.Close()
			return nil, errors.New("recording unavailable or too large")
		}
		return &boundedRecording{ReadCloser: res.Body, left: (25 << 20) + 1}, nil
	}
}

type boundedRecording struct {
	io.ReadCloser
	left int64
}

func (r *boundedRecording) Read(p []byte) (int, error) {
	if r.left <= 0 {
		return 0, errors.New("voice recording exceeds 25 MB")
	}
	if int64(len(p)) > r.left {
		p = p[:r.left]
	}
	n, err := r.ReadCloser.Read(p)
	r.left -= int64(n)
	return n, err
}

func (a *Adapter) Send(ctx context.Context,
	inst channels.Instance,
	m channels.OutboundMessage) (channels.SendResult,
	error) {
	if inst.Credentials["provider"] != "twilio" {
		return channels.SendResult{},
			channels.Permanent("unsupported",
				errors.New("netgsm live call initiation is not implemented"))
	}
	to := m.To.Phone
	if to == "" {
		to = m.To.ExternalID
	}
	to = twilioutil.E164(to)
	if to == "" || m.Text == "" || len([]rune(m.Text)) > 1000 || len(m.Attachments) > 0 || a.cfg.PublicBaseURL == "" {
		return channels.SendResult{},
			channels.Permanent("content",
				errors.New("voice requires a phone, up to 1000 text characters and public callback URL"))
	}
	// Ported from takwim voice/twilio. TwiML is escaped, so message text cannot inject call instructions.
	var escaped strings.Builder
	_ = xml.EscapeText(&escaped, []byte(m.Text))
	twiml := `<Response><Say language="tr-TR" voice="Polly.Filiz">` + escaped.String() + `</Say></Response>`
	callback := strings.TrimRight(a.cfg.PublicBaseURL, "/") + "/v1/webhooks/channels/voice/" + inst.UUID.String()
	f := url.Values{"To": {to},
		"From":                 {inst.Credentials["from_number"]},
		"Twiml":                {twiml},
		"StatusCallback":       {callback},
		"StatusCallbackMethod": {"POST"},
		"StatusCallbackEvent": {"initiated",
			"ringing",
			"answered",
			"completed"}}
	req, err := twilioutil.Request(ctx,
		inst,
		http.MethodPost,
		a.cfg.TwilioURL+twilioutil.AccountPath(inst)+"/Calls.json",
		f)
	if err != nil {
		return channels.SendResult{}, err
	}
	var out struct {
		SID string `json:"sid"`
	}
	if err := channelhttp.Do(a.cfg.Client, req, &out); err != nil {
		return channels.SendResult{}, err
	}
	if out.SID == "" {
		return channels.SendResult{}, errors.New("twilio response missing call ID")
	}
	return channels.SendResult{ExternalID: out.SID}, nil
}

var _ channels.Adapter = (*Adapter)(nil)

// CallLookup resolves recording-only callbacks, which contain no caller/destination.
type CallLookup func(context.Context, channels.Instance, string) (channels.CallEvent, error)

func (a *Adapter) WithCallLookup(lookup CallLookup) *Adapter { a.lookup = lookup; return a }
