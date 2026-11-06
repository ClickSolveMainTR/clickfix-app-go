// Package sms implements Netgsm, İleti Merkezi and Twilio on the inbox channel contract.
package sms

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/channelhttp"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/format"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/sms/netgsm"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/twilioutil"
)

// ConsentCheck is the İYS hook. Commercial sends fail closed without this hook.
// Implementations query the organization's MESAJ / ONAY consent, as takwim platform/iys.Status does.
type ConsentCheck func(context.Context, channels.Instance, string) (bool, error)
type Config struct {
	PublicBaseURL string
	TwilioURL     string
	IletiURL      string
	NetgsmURL     string
	Client        *http.Client
	Consent       ConsentCheck
}
type Adapter struct {
	cfg    Config
	netgsm *netgsm.Provider
}

func New(cfg Config) *Adapter {
	if cfg.Client == nil {
		cfg.Client = channelhttp.DefaultClient()
	}
	if cfg.TwilioURL == "" {
		cfg.TwilioURL = "https://api.twilio.com"
	}
	if cfg.IletiURL == "" {
		cfg.IletiURL = "https://api.iletimerkezi.com"
	}
	return &Adapter{cfg: cfg, netgsm: netgsm.New(cfg.NetgsmURL, cfg.Client)}
}
func (*Adapter) Type() string { return channels.TypeSMS }
func (*Adapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{DeliveryReceipts: true, MaxTextLength: 1000}
}
func (*Adapter) Fields() []channels.Field {
	return channelhttp.Fields("provider",
		"usercode password msgheader api_key api_hash account_sid auth_token from_number "+
			"webhook_token iys_username iys_password iys_code iys_brand_code",
		"password api_key api_hash auth_token webhook_token iys_password")
}
func (a *Adapter) Connect(ctx context.Context, inst channels.Instance) (string, error) {
	switch inst.Credentials["provider"] {
	case "netgsm":
		return "", a.netgsm.Check(ctx, inst)
	case "iletimerkezi":
		if err := channelhttp.Required(inst, "api_key", "api_hash", "msgheader"); err != nil {
			return "", err
		}
		_, err := a.ileti(ctx, inst, "get-balance", nil)
		return "", err
	case "twilio":
		return "", twilioutil.Check(ctx, a.cfg.Client, a.cfg.TwilioURL, inst)
	default:
		return "", errors.New("provider must be netgsm, iletimerkezi or twilio")
	}
}
func (a *Adapter) CheckHealth(ctx context.Context, inst channels.Instance) (channels.Health, error) {
	_, err := a.Connect(ctx, inst)
	return channels.Health{OK: err == nil}, err
}
func (a *Adapter) VerifyWebhook(r *http.Request, b []byte, inst channels.Instance) error {
	switch inst.Credentials["provider"] {
	case "twilio":
		return twilioutil.Verify(r, b, inst, a.cfg.PublicBaseURL)
	case "iletimerkezi", "netgsm":
		secret := inst.Credentials["webhook_token"]
		expected, actual := sha256.Sum256([]byte(secret)), sha256.Sum256([]byte(r.URL.Query().Get("token")))
		if len(secret) < 32 || subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
			return channels.ErrSignature
		}
		return nil
	default:
		return channels.ErrSignature // Netgsm reports are polled; no unsigned inbound endpoint.
	}
}
func (*Adapter) ParseWebhook(_ context.Context,
	_ *http.Request,
	b []byte,
	inst channels.Instance) (channels.WebhookResult, error) {
	if inst.Credentials["provider"] == "netgsm" {
		var p struct {
			ID   json.Number `json:"messageId"`
			From string      `json:"sourceNumber"`
			To   string      `json:"subscriberNumber"`
			Date string      `json:"messageDateTime"`
			Text string      `json:"content"`
		}
		if !bytes.HasPrefix(bytes.TrimSpace(b), []byte("{")) {
			f, err := url.ParseQuery(string(b))
			if err != nil {
				return channels.WebhookResult{}, errors.New("invalid Netgsm SMS form")
			}
			fields := map[string]string{}
			for k, v := range f {
				if len(v) > 0 {
					fields[k] = v[0]
				}
			}
			b, _ = json.Marshal(fields)
		}
		if err := json.Unmarshal(b, &p); err != nil {
			return channels.WebhookResult{}, errors.New("invalid Netgsm inbound SMS")
		}
		from := twilioutil.E164(p.From)
		if p.ID == "" || from == "" || p.Text == "" {
			return channels.WebhookResult{}, errors.New("incomplete Netgsm SMS")
		}
		if own := twilioutil.E164(inst.Credentials["from_number"]); own != "" && own != twilioutil.E164(p.To) {
			return channels.WebhookResult{}, errors.New("SMS destination mismatch")
		}
		at, _ := time.ParseInLocation("2006-01-02 15:04:05", p.Date, netgsm.Location)
		if at.IsZero() {
			at = time.Now().UTC()
		}
		return channels.WebhookResult{Messages: []channels.InboundMessage{{ExternalID: p.ID.String(), From: channels.Sender{ExternalID: from, Phone: from}, Kind: "text", Text: p.Text, At: at}}}, nil
	}
	if inst.Credentials["provider"] == "iletimerkezi" {
		var v struct {
			Report struct {
				Packet json.Number `json:"packet_id"`
				Status string      `json:"status"`
			} `json:"report"`
		}
		if err := json.Unmarshal(b, &v); err != nil {
			return channels.WebhookResult{}, err
		}
		status := map[string]string{"accepted": channels.StatusSent,
			"delivered":   channels.StatusDelivered,
			"undelivered": channels.StatusFailed}[v.Report.Status]
		if v.Report.Packet == "" || status == "" {
			return channels.WebhookResult{}, errors.New("invalid delivery report")
		}
		return channels.WebhookResult{Statuses: []channels.StatusUpdate{{ExternalID: v.Report.Packet.String(),
				Status: status,
				At:     time.Now().UTC()}}},
			nil
	}
	f, err := url.ParseQuery(string(b))
	if err != nil {
		return channels.WebhookResult{}, err
	}
	id := f.Get("MessageSid")
	if id == "" {
		return channels.WebhookResult{}, errors.New("missing MessageSid")
	}
	if status := f.Get("MessageStatus"); status != "" {
		mapped := map[string]string{"sent": channels.StatusSent,
			"delivered":   channels.StatusDelivered,
			"failed":      channels.StatusFailed,
			"undelivered": channels.StatusFailed}[status]
		if mapped == "" {
			return channels.WebhookResult{}, nil
		}
		return channels.WebhookResult{Statuses: []channels.StatusUpdate{{ExternalID: id,
				Status: mapped,
				Error:  f.Get("ErrorCode"),
				At:     time.Now().UTC()}}},
			nil
	}
	from := twilioutil.E164(f.Get("From"))
	if from == "" ||
		f.Get("Body") == "" ||
		twilioutil.E164(f.Get("To")) != twilioutil.E164(inst.Credentials["from_number"]) {
		return channels.WebhookResult{}, errors.New("invalid SMS sender, body or destination")
	}
	return channels.WebhookResult{Messages: []channels.InboundMessage{{ExternalID: id,
			From: channels.Sender{ExternalID: from,
				Phone: from},
			Kind: "text",
			Text: f.Get("Body"),
			At:   time.Now().UTC()}}},
		nil
}
func (a *Adapter) Send(ctx context.Context,
	inst channels.Instance,
	m channels.OutboundMessage) (channels.SendResult,
	error) {
	phone := m.To.Phone
	if phone == "" {
		phone = m.To.ExternalID
	}
	phone = twilioutil.E164(phone)
	m.Text = format.Plain(m.Text) // SMS is plain text (CF-177)
	if phone == "" || m.Text == "" || len([]rune(m.Text)) > 1000 || len(m.Attachments) > 0 {
		return channels.SendResult{},
			channels.Permanent("content",
				errors.New("valid phone and up to 1000 text characters required"))
	}
	commercial, _ := inst.Settings["commercial"].(bool)
	if v, _ := m.Attributes["commercial"].(bool); v {
		commercial = true
	}
	if commercial {
		if a.cfg.Consent == nil {
			return channels.SendResult{},
				channels.Permanent("iys_unavailable",
					errors.New("İYS consent checker is not configured"))
		}
		ok, err := a.cfg.Consent(ctx, inst, phone)
		if err != nil {
			return channels.SendResult{}, fmt.Errorf("İYS consent check: %w", err)
		}
		if !ok {
			return channels.SendResult{}, channels.Permanent("iys_rejected", errors.New("commercial SMS consent missing"))
		}
	}
	var id string
	var err error
	switch inst.Credentials["provider"] {
	case "netgsm":
		id, err = a.netgsm.Send(ctx, inst, netgsm.Message{To: phone, Text: m.Text, Commercial: commercial})
		var rejected *netgsm.Error
		if errors.As(err,
			&rejected) &&
			rejected.Code != "80" &&
			rejected.Code != "85" &&
			rejected.Code != "100" &&
			rejected.Code != "101" {
			err = channels.Permanent("netgsm_rejected", err)
		}
	case "iletimerkezi":
		if err := channelhttp.Required(inst, "api_key", "api_hash", "msgheader"); err != nil {
			return channels.SendResult{}, channels.Permanent("configuration", err)
		}
		iys := "0"
		if commercial {
			iys = "1"
		}
		id,
			err = a.ileti(ctx,
			inst,
			"send-sms",
			map[string]any{"sender": inst.Credentials["msgheader"],
				"iys":     iys,
				"iysList": "BIREYSEL",
				"message": map[string]any{"text": m.Text,
					"receipents": map[string]any{"number": []string{phone}}}})
	case "twilio":
		form := url.Values{"To": {phone}, "From": {inst.Credentials["from_number"]}, "Body": {m.Text}}
		if a.cfg.PublicBaseURL != "" {
			form.Set("StatusCallback",
				strings.TrimRight(a.cfg.PublicBaseURL,
					"/")+"/v1/webhooks/channels/sms/"+inst.UUID.String())
		}
		req,
			e := twilioutil.Request(ctx,
			inst,
			http.MethodPost,
			a.cfg.TwilioURL+twilioutil.AccountPath(inst)+"/Messages.json",
			form)
		if e != nil {
			return channels.SendResult{}, e
		}
		var res struct {
			SID string `json:"sid"`
		}
		err = channelhttp.Do(a.cfg.Client, req, &res)
		id = res.SID
	default:
		err = channels.Permanent("configuration", errors.New("unknown SMS provider"))
	}
	if err != nil {
		return channels.SendResult{}, err
	}
	if id == "" {
		return channels.SendResult{}, errors.New("provider accepted no message ID")
	}
	return channels.SendResult{ExternalID: id, Usage: &channels.Usage{Feature: "sms", Quantity: 1}}, nil
}
func (a *Adapter) ileti(ctx context.Context,
	inst channels.Instance,
	method string,
	order map[string]any) (string,
	error) {
	payload := map[string]any{"authentication": map[string]string{"key": inst.Credentials["api_key"],
		"hash": inst.Credentials["api_hash"]}}
	if order != nil {
		payload["order"] = order
	}
	b, err := json.Marshal(map[string]any{"request": payload})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.IletiURL+"/v1/"+method+"/json", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var v struct {
		Response struct {
			Status struct {
				Code int `json:"code"`
			} `json:"status"`
			Order struct {
				ID json.Number `json:"id"`
			} `json:"order"`
		} `json:"response"`
	}
	if err := channelhttp.Do(a.cfg.Client, req, &v); err != nil {
		return "", err
	}
	if v.Response.Status.Code != 200 {
		e := fmt.Errorf("iletimerkezi response code %d", v.Response.Status.Code)
		if v.Response.Status.Code >= 500 {
			return "", e
		}
		return "", channels.Permanent("iletimerkezi_rejected", e)
	}
	return v.Response.Order.ID.String(), nil
}

var _ channels.Adapter = (*Adapter)(nil)

func (*Adapter) AcknowledgeWebhook(_ *http.Request, _ []byte, inst channels.Instance) (string, string) {
	if inst.Credentials["provider"] == "twilio" {
		return "application/xml", `<?xml version="1.0" encoding="UTF-8"?><Response></Response>`
	}
	return "application/json", `{"received":true}`
}

// PollNetgsm combines delivery receipts and inbound messages on the inbox contract.
func (a *Adapter) PollNetgsm(ctx context.Context, inst channels.Instance, ids []string) (channels.WebhookResult, error) {
	reports, reportErr := a.netgsm.Reports(ctx, inst, ids)
	out, inboxErr := a.netgsm.Inbox(ctx, inst, time.Now().Add(-24*time.Hour), time.Now())
	for _, r := range reports {
		if r.Status == netgsm.StatusWaiting {
			continue
		}
		status := channels.StatusDelivered
		detail := ""
		if r.Failed() {
			status = channels.StatusFailed
			detail = r.Error()
		}
		out.Statuses = append(out.Statuses, channels.StatusUpdate{ExternalID: r.JobID, Status: status, Error: detail, At: time.Now().UTC()})
	}
	return out, errors.Join(reportErr, inboxErr)
}
