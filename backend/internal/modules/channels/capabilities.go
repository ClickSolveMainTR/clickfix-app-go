package channels

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	platformchannels "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/channelhttp"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Assistant capabilities of the channels module (CF-185, scenario 1 of
// docs/design/asistan-yetenekleri.md): list channels, set up and update
// the REST API channel, test it and produce example requests. Every call
// goes through the panel's own /v1/tenant/channels routes as the user.

const (
	capChannelsPath = "/v1/tenant/channels"
	keySecret       = "signing_secret"
	maxListed       = 30
)

// channelView is the part of InstanceView the assistant may see: no
// credential hints of secret fields.
type channelView struct {
	UUID            uuid.UUID         `json:"uuid"`
	Type            string            `json:"type"`
	Name            string            `json:"name"`
	Status          string            `json:"status"`
	WebhookURL      string            `json:"webhook_url"`
	WebhookPath     string            `json:"webhook_path"`
	CredentialHints map[string]string `json:"credential_hints"`
	Health          struct {
		State  string `json:"state"`
		Detail string `json:"detail"`
	} `json:"health"`
	LastInboundAt *time.Time `json:"last_inbound_at"`
	LastError     string     `json:"last_error"`
}

// inbound is where the organization's system posts messages to ClickFix.
func (v channelView) inbound() string {
	if v.WebhookURL != "" {
		return v.WebhookURL
	}
	return v.WebhookPath
}

// outbound is where ClickFix posts replies (a non-secret field).
func (v channelView) outbound() string { return v.CredentialHints["webhook_url"] }

func (v channelView) summary() map[string]any {
	out := map[string]any{"uuid": v.UUID, "type": v.Type, "name": v.Name, "status": v.Status,
		"health": v.Health.State, "inbound_url": v.inbound()}
	if v.Type == platformchannels.TypeAPI {
		out["outbound_url"] = v.outbound()
	}
	if v.LastError != "" {
		out["last_error"] = clip(v.LastError, 200)
	}
	if v.LastInboundAt != nil {
		out["last_inbound_at"] = v.LastInboundAt
	}
	return out
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func listChannels(ctx context.Context, api capability.API, typ string) ([]channelView, error) {
	path := capChannelsPath
	if typ != "" {
		path += "?type=" + typ
	}
	r, err := api.Call(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []channelView `json:"items"`
	}
	decErr := r.Decode(&out)
	return out.Items, decErr
}

func getChannel(ctx context.Context, api capability.API, id string) (channelView, error) {
	var v channelView
	if _, err := uuid.Parse(id); err != nil {
		return v, capability.Invalid("channel_uuid must be a UUID")
	}
	r, err := api.Call(ctx, http.MethodGet, capChannelsPath+"/"+id, nil)
	if err != nil {
		return v, err
	}
	decErr := r.Decode(&v)
	return v, decErr
}

func getAPIChannel(ctx context.Context, api capability.API, id string) (channelView, error) {
	v, err := getChannel(ctx, api, id)
	if err == nil && v.Type != platformchannels.TypeAPI {
		return v, capability.Invalid("channel %s is a %s channel, not a REST API channel", id, v.Type)
	}
	return v, err
}

func str(p capability.Params, k string) string {
	s, _ := p[k].(string)
	return strings.TrimSpace(s)
}

// validWebhook applies the channel's own SSRF rules (https, public host,
// no credentials or fragment, no private IP literal); it does no DNS.
func validWebhook(raw string) error {
	if err := channelhttp.ValidateURL(raw); err != nil {
		return capability.Invalid("webhook_url must be a public HTTPS address (no localhost, private IP or user:password)")
	}
	return nil
}

var uuidProp = map[string]any{"type": "string", "title": "Channel", "description": "uuid from channels_list",
	"pattern": "^[0-9a-fA-F-]{36}$"}

var nameProp = map[string]any{"type": "string", "title": "Name", "description": "Inbox name shown to agents", "minLength": 1, "maxLength": 80}

var webhookProp = map[string]any{"type": "string", "title": "Webhook URL", "format": "uri", "maxLength": 2048,
	"description": "HTTPS endpoint of the organization's system where ClickFix posts agent replies"}

var secretProp = map[string]any{"type": "string", "title": "Signing secret", "minLength": 32, "maxLength": 256}

var secretLabel = map[string]string{"tr": "İmza anahtarı", "en": "Signing secret"}

// SampleRequests are the example requests of an API channel. The secret
// is never included: it is the $CLICKFIX_SIGNING_SECRET placeholder.
func SampleRequests(inbound, outbound string, en bool) []capability.Detail {
	if inbound == "" {
		inbound = "<inbound URL>"
	}
	in := `BODY='{"id":"msg-1001","sender_id":"customer-42","sender_name":"Ayşe Yılmaz","text":"Merhaba, siparişim nerede?","thread_id":"order-7"}'
TS=$(date +%s)
SIG="sha256=$(printf '%s.%s' "$TS" "$BODY" | openssl dgst -sha256 -hmac "$CLICKFIX_SIGNING_SECRET" -hex | sed 's/^.* //')"
curl -X POST '` + inbound + `' \
  -H 'Content-Type: application/json' \
  -H "X-ClickFix-Timestamp: $TS" \
  -H "X-ClickFix-Signature: $SIG" \
  --data "$BODY"`
	out := `POST ` + firstNonEmpty(outbound, "<webhook_url>") + `
Content-Type: application/json
X-ClickFix-Timestamp: 1767225600
X-ClickFix-Signature: sha256=<HMAC-SHA256(secret, timestamp + "." + body)>
Idempotency-Key: <message uuid>

{"id":"<message uuid>","conversation_id":"<conversation uuid>","recipient_id":"customer-42","text":"Kargonuz yolda.","thread_id":"order-7","reply_to":""}`
	labels := []string{"Gelen mesaj örneği (sisteminizden ClickFix'e)", "Giden yanıt biçimi (ClickFix'ten sisteminize)",
		"İmza: zaman damgası + \".\" + gövde üzerinde HMAC-SHA256; 5 dakikadan eski istekler reddedilir. Test olayı X-ClickFix-Event: test başlığıyla gelir, mesaj oluşturmayın."}
	if en {
		labels = []string{"Inbound message example (your system to ClickFix)", "Outbound reply format (ClickFix to your system)",
			"Signature: HMAC-SHA256 over timestamp + \".\" + body; requests older than 5 minutes are rejected. The test event carries X-ClickFix-Event: test; do not create a message for it."}
	}
	return []capability.Detail{{Label: labels[0], Code: in}, {Label: labels[1], Code: out}, {Label: labels[2]}}
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

func t(e capability.Exec, tr, en string, a ...any) string {
	if e.En() {
		return fmt.Sprintf(en, a...)
	}
	return fmt.Sprintf(tr, a...)
}

// healthCheck runs the channel check and reports it as a verification.
func healthCheck(ctx context.Context, e capability.Exec, id string) capability.Check {
	if e.API == nil {
		return capability.Check{Status: capability.VerifySkipped}
	}
	r, err := e.API.Call(ctx, http.MethodPost, capChannelsPath+"/"+id+"/check", nil)
	if err != nil {
		return capability.Check{Status: capability.VerifyFailed}
	}
	var v channelView
	if err := r.Decode(&v); err != nil {
		return capability.Check{Status: capability.VerifyFailed, Message: clip(r.Message, 200)}
	}
	if v.Health.State == "error" {
		return capability.Check{Status: capability.VerifyFailed, Message: clip(v.Health.Detail, 200)}
	}
	return capability.Check{Status: capability.VerifyOK, Message: t(e, "Kanal durumu: %s", "Channel status: %s", v.Status)}
}

// Capabilities are the channels module's assistant capabilities.
func Capabilities() []*capability.Capability {
	routes := []string{"/channels"}
	return []*capability.Capability{
		{
			Name: "channels.list", Module: "channels", Kind: capability.Read, Permission: rbac.PermTenantChannelsRead, Routes: routes,
			Title:       map[string]string{"tr": "Kanalları listele", "en": "List channels"},
			Description: "List the organization's connected channels (inboxes): uuid, type, name, status, health, inbound URL and, for REST API channels, the outbound webhook URL. Call this before proposing to create a channel.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
				"type": map[string]any{"type": "string", "title": "Type", "description": "Filter by channel type (api, email, whatsapp, ...)", "pattern": "^[a-z_]{1,20}$"},
			}},
			Run: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				items, err := listChannels(ctx, e.API, str(p, "type"))
				if err != nil {
					return capability.Result{}, err
				}
				out := []map[string]any{}
				for i, v := range items {
					if i == maxListed {
						break
					}
					out = append(out, v.summary())
				}
				return capability.Result{Summary: fmt.Sprintf("%d channels", len(items)), Data: map[string]any{"items": out, "total": len(items)}}, nil
			},
		},
		{
			Name: "channels.api.sample_request", Module: "channels", Kind: capability.Read, Permission: rbac.PermTenantChannelsRead, Routes: routes,
			Title:       map[string]string{"tr": "Örnek istek üret", "en": "Example requests"},
			Description: "Example requests of a REST API channel: a signed curl call that sends a customer message into ClickFix, the JSON ClickFix posts back to the organization's webhook, and how to verify the signature. The signing secret is a placeholder; never ask for it. Paste the examples to the user as code blocks.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"channel_uuid"},
				"properties": map[string]any{"channel_uuid": uuidProp}},
			Run: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				v, err := getAPIChannel(ctx, e.API, str(p, "channel_uuid"))
				if err != nil {
					return capability.Result{}, err
				}
				details := SampleRequests(v.inbound(), v.outbound(), e.En())
				data := map[string]any{"channel": v.Name}
				for i, d := range details {
					data[[]string{"inbound_example", "outbound_format", "signature"}[i]] = firstNonEmpty(d.Code, d.Label)
				}
				return capability.Result{Summary: v.Name, Data: data, Details: details}, nil
			},
		},
		{
			Name: "channels.api.create", Module: "channels", Kind: capability.Write, Permission: rbac.PermTenantChannelsManage, Routes: routes,
			Title:       map[string]string{"tr": "REST API kanalı kur", "en": "Set up the REST API channel"},
			Description: "Create a REST API channel so the organization's own software can send customer messages to ClickFix and receive agent replies on its webhook. Needs a name and the HTTPS webhook_url of their system. The signing secret is generated by the server and shown to the user once after confirmation; never ask for it. If an API channel already exists, propose channels_api_update instead.",
			Undo:        "/channels",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "webhook_url"},
				"properties": map[string]any{"name": nameProp, "webhook_url": webhookProp, keySecret: secretProp}},
			Secrets: []capability.Secret{{Key: keySecret, Label: secretLabel, Generate: capability.RandomSecret}},
			Validate: func(p capability.Params) error {
				return validWebhook(str(p, "webhook_url"))
			},
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				existing, err := listChannels(ctx, e.API, platformchannels.TypeAPI)
				if err != nil {
					return capability.Result{}, err
				}
				for _, v := range existing {
					if strings.EqualFold(v.Name, str(p, "name")) || (v.outbound() != "" && v.outbound() == str(p, "webhook_url")) {
						return capability.Result{}, capability.Invalid("%s", t(e,
							"Bu ada ya da adrese sahip bir REST API kanalı zaten var (%s). Yeni kanal yerine onu güncelleyin.",
							"A REST API channel with this name or address already exists (%s). Update it instead of creating a new one.", v.Name))
					}
				}
				return capability.Result{
					Summary: t(e, "“%s” adlı REST API kanalı oluşturulacak. Temsilci yanıtları %s adresine imzalı olarak gönderilecek; imza anahtarı sunucuda üretilip size bir kez gösterilecek.",
						"A REST API channel named “%s” will be created. Agent replies will be posted, signed, to %s; the signing secret is generated by the server and shown to you once.",
						str(p, "name"), str(p, "webhook_url")),
					Details: []capability.Detail{{Label: t(e, "Mevcut REST API kanalı", "Existing REST API channels"), Value: fmt.Sprint(len(existing))}},
				}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				secret := e.Secrets[keySecret]
				if len(secret) < 32 {
					return capability.Result{}, capability.Invalid("the signing secret must have at least 32 characters")
				}
				r, err := e.API.Call(ctx, http.MethodPost, capChannelsPath, map[string]any{
					"type": platformchannels.TypeAPI, "name": str(p, "name"),
					"credentials": map[string]string{keySecret: secret, "webhook_url": str(p, "webhook_url")},
				})
				if err != nil {
					return capability.Result{}, err
				}
				var v channelView
				if err := r.Decode(&v); err != nil {
					return capability.Result{}, err
				}
				details := append([]capability.Detail{
					{Label: t(e, "Gelen mesaj adresi", "Inbound URL"), Value: v.inbound()},
					{Label: t(e, "Kanal kimliği", "Channel ID"), Value: v.UUID.String()},
				}, SampleRequests(v.inbound(), str(p, "webhook_url"), e.En())...)
				route := "/channels/" + v.UUID.String()
				return capability.Result{Summary: v.UUID.String(), Details: details, Route: route, Focus: &capability.Focus{Route: route},
					Secrets: map[string]string{keySecret: secret}}, nil
			},
			Verify: func(ctx context.Context, e capability.Exec, _ capability.Params, r capability.Result) capability.Check {
				return healthCheck(ctx, e, r.Summary)
			},
		},
		{
			Name: "channels.api.update", Module: "channels", Kind: capability.Write, Permission: rbac.PermTenantChannelsManage, Routes: routes,
			Title:       map[string]string{"tr": "REST API kanalını güncelle", "en": "Update the REST API channel"},
			Description: "Change the name or the outbound webhook_url of an existing REST API channel (channel_uuid from channels_list). Leave a field out to keep it. To issue a new signing secret propose channels_api_rotate_secret.",
			Undo:        "/channels",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"channel_uuid"},
				"properties": map[string]any{"channel_uuid": uuidProp, "name": nameProp, "webhook_url": webhookProp}},
			Validate: func(p capability.Params) error {
				if str(p, "name") == "" && str(p, "webhook_url") == "" {
					return capability.Invalid("give a new name or webhook_url: ask the user what to change")
				}
				if u := str(p, "webhook_url"); u != "" {
					return validWebhook(u)
				}
				return nil
			},
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				v, err := getAPIChannel(ctx, e.API, str(p, "channel_uuid"))
				if err != nil {
					return capability.Result{}, err
				}
				details := []capability.Detail{}
				if n := str(p, "name"); n != "" && n != v.Name {
					details = append(details, capability.Detail{Label: t(e, "Ad", "Name"), Value: v.Name + " → " + n})
				}
				if u := str(p, "webhook_url"); u != "" && u != v.outbound() {
					details = append(details, capability.Detail{Label: "Webhook URL", Value: firstNonEmpty(v.outbound(), "—") + " → " + u})
				}
				if len(details) == 0 {
					return capability.Result{}, capability.Invalid("%s", t(e, "Değişiklik yok: değerler zaten aynı.", "Nothing changes: the values are already the same."))
				}
				return capability.Result{Summary: t(e, "“%s” kanalı güncellenecek.", "Channel “%s” will be updated.", v.Name), Details: details}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				id := str(p, "channel_uuid")
				if _, err := getAPIChannel(ctx, e.API, id); err != nil {
					return capability.Result{}, err
				}
				body := map[string]any{}
				if n := str(p, "name"); n != "" {
					body["name"] = n
				}
				if u := str(p, "webhook_url"); u != "" {
					body["credentials"] = map[string]string{"webhook_url": u}
				}
				r, err := e.API.Call(ctx, http.MethodPatch, capChannelsPath+"/"+id, body)
				if err != nil {
					return capability.Result{}, err
				}
				var v channelView
				if err := r.Decode(&v); err != nil {
					return capability.Result{}, err
				}
				route := "/channels/" + id
				return capability.Result{Summary: id, Route: route, Focus: &capability.Focus{Route: route},
					Details: []capability.Detail{{Label: t(e, "Ad", "Name"), Value: v.Name}, {Label: "Webhook URL", Value: v.outbound()}}}, nil
			},
			Verify: func(ctx context.Context, e capability.Exec, _ capability.Params, r capability.Result) capability.Check {
				return healthCheck(ctx, e, r.Summary)
			},
		},
		{
			Name: "channels.api.rotate_secret", Module: "channels", Kind: capability.Destructive, Permission: rbac.PermTenantChannelsManage, Routes: routes,
			Title:       map[string]string{"tr": "İmza anahtarını yenile", "en": "Rotate the signing secret"},
			Description: "Issue a new signing secret for a REST API channel. The old secret stops working at once, so the organization's system must be updated. The new secret is shown to the user once.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"channel_uuid"},
				"properties": map[string]any{"channel_uuid": uuidProp, keySecret: secretProp}},
			Secrets: []capability.Secret{{Key: keySecret, Label: secretLabel, Generate: capability.RandomSecret}},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				id := str(p, "channel_uuid")
				if _, err := getAPIChannel(ctx, e.API, id); err != nil {
					return capability.Result{}, err
				}
				secret := e.Secrets[keySecret]
				if len(secret) < 32 {
					return capability.Result{}, capability.Invalid("the signing secret must have at least 32 characters")
				}
				r, err := e.API.Call(ctx, http.MethodPatch, capChannelsPath+"/"+id, map[string]any{"credentials": map[string]string{keySecret: secret}})
				if err != nil {
					return capability.Result{}, err
				}
				if err := r.Err(); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: id, Route: "/channels/" + id, Secrets: map[string]string{keySecret: secret}}, nil
			},
		},
		{
			Name: "channels.test", Module: "channels", Kind: capability.Write, Permission: rbac.PermTenantChannelsManage, Routes: routes,
			Title:       map[string]string{"tr": "Kanalı test et", "en": "Test the channel"},
			Description: "Test a channel: check its connection; for a REST API channel also post a signed test event (X-ClickFix-Event: test) to the organization's webhook, for an e-mail channel send a real test e-mail to the confirming user's own address. Reports the result. Contacts an external system, so it is confirmed.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"channel_uuid"},
				"properties": map[string]any{"channel_uuid": uuidProp}},
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				v, err := getChannel(ctx, e.API, str(p, "channel_uuid"))
				if err != nil {
					return capability.Result{}, err
				}
				target := t(e, "bağlantı denetimi", "connection check")
				switch v.Type {
				case platformchannels.TypeAPI:
					target = t(e, "%s adresine imzalı test olayı", "signed test event to %s", firstNonEmpty(v.outbound(), "webhook_url"))
				case platformchannels.TypeEmail:
					target = t(e, "bağlantı denetimi ve kendi e-posta adresinize gerçek bir test e-postası", "a connection check and a real test e-mail to your own address")
				}
				return capability.Result{Summary: t(e, "“%s”: %s gönderilecek; veri değişmez.", "“%s”: %s will be sent; no data changes.", v.Name, target)}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				id := str(p, "channel_uuid")
				v, err := getChannel(ctx, e.API, id)
				if err != nil {
					return capability.Result{}, err
				}
				check := healthCheck(ctx, e, id)
				details := []capability.Detail{{Label: t(e, "Bağlantı denetimi", "Connection check"), Value: firstNonEmpty(check.Message, check.Status)}}
				if v.Type == platformchannels.TypeAPI || v.Type == platformchannels.TypeEmail {
					r, err := e.API.Call(ctx, http.MethodPost, capChannelsPath+"/"+id+"/test-delivery", nil)
					if err != nil {
						return capability.Result{}, err
					}
					var out platformchannels.DeliveryTest
					if err := r.Decode(&out); err != nil {
						return capability.Result{}, err
					}
					res := t(e, "Başarılı (HTTP %d)", "Delivered (HTTP %d)", out.StatusCode)
					if out.Recipient != "" {
						res = t(e, "Test e-postası %s adresine gönderildi; gelen kutunuzu kontrol edin", "Test e-mail sent to %s; check your inbox", out.Recipient)
					}
					if !out.OK {
						res = t(e, "Başarısız: %s", "Failed: %s", out.Detail)
						check = capability.Check{Status: capability.VerifyFailed, Message: out.Detail}
					}
					details = append(details, capability.Detail{Label: t(e, "Test teslimatı", "Test delivery"), Value: res})
				}
				// The outcome is reported by Verify, so a failed test still
				// shows its details on the card.
				return capability.Result{Summary: id, Details: details, Route: "/channels/" + id, Data: check}, nil
			},
			Verify: func(_ context.Context, _ capability.Exec, _ capability.Params, r capability.Result) capability.Check {
				if c, ok := r.Data.(capability.Check); ok {
					return c
				}
				return capability.Check{Status: capability.VerifySkipped}
			},
		},
	}
}
