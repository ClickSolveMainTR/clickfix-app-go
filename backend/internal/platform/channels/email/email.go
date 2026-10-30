// Package email is the e-mail channel (CF-56).
//
// Inbound: every instance owns a routing address on ClickFix's receiving
// domain (<alias>@<InboundDomain>, MX at Resend). An organization forwards
// its support mailbox there (or publishes it directly). Resend posts
// email.received to the shared webhook /v1/webhooks/channels/email (Svix
// signed); Route maps the recipient alias to the instance and ParseWebhook
// downloads the raw MIME through the receiving API (ported from
// 216detay-mail-system) and normalizes it: threading by References /
// In-Reply-To / Message-ID, the visible reply without quotes and
// signature, sanitized HTML, CC, files and bounce reports.
//
// Outbound: through Resend from the instance's from_address (its domain is
// verified with the DNS wizard, see Resend domains) or through the
// organization's own SMTP server (provider=smtp). Reply-To is always the
// routing address for forwarding and the mailbox for IMAP, so answers come back
// even when the organization never
// set up forwarding. Our Message-ID carries the thread root, so a client
// that drops References still threads by In-Reply-To.
//
// Loop protection: automatic replies (Auto-Submitted, Precedence, list
// mail) are dropped, our own messages carry X-ClickFix-Instance and are
// never read back, and outbound mail asks Exchange not to auto-answer.
// Delivery: Resend email.delivered / email.bounced / email.complained
// events (tagged with the instance) and RFC 3464 bounce mails move the
// message forward or fail it.
//
// Provider imap receives from an existing mailbox using TLS and sends through
// the same instance's SMTP configuration. Polling reuses ParseRaw unchanged.
package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
)

// Credential fields.
const (
	FieldFromAddress  = "from_address"
	FieldFromName     = "from_name"
	FieldProvider     = "provider"
	FieldSMTPHost     = "smtp_host"
	FieldSMTPPort     = "smtp_port"
	FieldSMTPUsername = "smtp_username"
	FieldSMTPPassword = "smtp_password"
	FieldSMTPTLS      = "smtp_tls"
)

// Providers of outbound mail.
const (
	ProviderResend = "resend"
	ProviderSMTP   = "smtp"
	ProviderIMAP   = "imap"
)

// SettingAlias is the instance setting holding the routing alias.
const SettingAlias = "inbound_alias"

// HeaderInstance marks mail ClickFix sent (loop protection).
const HeaderInstance = "X-ClickFix-Instance"

// AttrNewThread marks an outbound message that starts a thread: its
// subject is sent as is, without a reply prefix.
const AttrNewThread = "new_thread"

// TagInstance is the Resend tag that routes delivery events back.
const TagInstance = "cf_instance"

// Config is the platform-wide e-mail setup.
type Config struct {
	// IMAPTLSConfig optionally supplies a private CA trust store. Nil uses system roots.
	IMAPTLSConfig *tls.Config
	// InboundDomain is the receiving domain (MX at Resend), e.g.
	// "in.getclickfix.com".
	InboundDomain string
	// ResendAPIKey must allow sending and reading received mail.
	ResendAPIKey string
	// WebhookSecret is the Svix signing secret (whsec_…) of the Resend
	// webhook.
	WebhookSecret string
	BaseURL       string
	HTTPClient    *http.Client
	// MaxAttachmentBytes caps one outbound SMTP message's files (20 MiB).
	MaxAttachmentBytes int64
}

// Adapter is the e-mail channel.
type Adapter struct {
	cfg    Config
	resend *Resend
	now    func() time.Time
	// smtpSend is swapped in tests.
	smtpSend func(ctx context.Context, c SMTPConfig, from string, rcpt []string, msg []byte) error
	smtpTest func(ctx context.Context, c SMTPConfig) error
	imapTest func(ctx context.Context, c IMAPConfig) error
}

var (
	_ channels.Adapter       = (*Adapter)(nil)
	_ channels.Router        = (*Adapter)(nil)
	_ channels.HealthChecker = (*Adapter)(nil)
)

// New builds the adapter.
func New(cfg Config) *Adapter {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.resend.com"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.MaxAttachmentBytes <= 0 {
		cfg.MaxAttachmentBytes = 20 << 20
	}
	cfg.InboundDomain = strings.ToLower(strings.TrimSpace(cfg.InboundDomain))
	a := &Adapter{cfg: cfg, now: time.Now}
	a.resend = &Resend{BaseURL: cfg.BaseURL, APIKey: cfg.ResendAPIKey, HTTP: cfg.HTTPClient}
	a.smtpSend = func(ctx context.Context, c SMTPConfig, from string, rcpt []string, msg []byte) error {
		return SendSMTP(ctx, c, from, rcpt, msg, nil)
	}
	a.smtpTest = func(ctx context.Context, c SMTPConfig) error { return CheckSMTP(ctx, c, nil) }
	a.imapTest = func(ctx context.Context, c IMAPConfig) error { return CheckIMAP(ctx, c, cfg.IMAPTLSConfig) }
	return a
}

// Enabled reports whether inbound routing is configured.
func (a *Adapter) Enabled() bool {
	return a.cfg.InboundDomain != "" && a.cfg.ResendAPIKey != "" && a.cfg.WebhookSecret != ""
}

// Resend exposes the API client (domain wizard).
func (a *Adapter) Resend() *Resend { return a.resend }

// InboundDomain is the receiving domain.
func (a *Adapter) InboundDomain() string { return a.cfg.InboundDomain }

// Type implements channels.Adapter.
func (a *Adapter) Type() string { return channels.TypeEmail }

// Capabilities implements channels.Adapter.
func (a *Adapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{Media: true, DeliveryReceipts: true, Threading: true}
}

// Fields implements channels.Adapter. Everything is optional: an instance
// without a from_address sends from its routing address through Resend.
func (a *Adapter) Fields() []channels.Field {
	f := func(key string, secret bool) channels.Field {
		return channels.Field{Key: key, LabelKey: "channels.fields." + key, Secret: secret}
	}
	return []channels.Field{
		f(FieldFromAddress, false), f(FieldFromName, false), f(FieldProvider, false),
		f(FieldSMTPHost, false), f(FieldSMTPPort, false), f(FieldSMTPUsername, false),
		f(FieldSMTPPassword, true), f(FieldSMTPTLS, false),
		f(FieldIMAPHost, false), f(FieldIMAPPort, false), f(FieldIMAPUsername, false),
		f(FieldIMAPPassword, true), f(FieldIMAPTLS, false), f(FieldIMAPFolder, false), f(FieldIMAPAuth, false),
	}
}

var aliasRE = regexp.MustCompile(`^[a-z0-9][a-z0-9.\-]{1,62}[a-z0-9]$`)

// Alias is the routing alias of an instance: its inbound_alias setting or
// one derived from its uuid.
func Alias(inst channels.Instance) string {
	if v, ok := inst.Settings[SettingAlias].(string); ok {
		if v = strings.ToLower(strings.TrimSpace(v)); aliasRE.MatchString(v) {
			return v
		}
	}
	return "in-" + strings.ReplaceAll(inst.UUID.String(), "-", "")[:12]
}

// Address is the routing address of an instance.
func (a *Adapter) Address(inst channels.Instance) string {
	return Alias(inst) + "@" + a.cfg.InboundDomain
}

// Connect implements channels.Adapter: checks the alias, the sender and,
// for SMTP, that the server accepts the login. The alias is the external
// id webhooks route by (unique across organizations).
func (a *Adapter) Connect(ctx context.Context, inst channels.Instance) (string, error) {
	if a.cfg.InboundDomain == "" && provider(inst.Credentials) != ProviderIMAP {
		return "", errors.New("e-mail receiving is not configured")
	}
	if v, ok := inst.Settings[SettingAlias].(string); ok && strings.TrimSpace(v) != "" && !aliasRE.MatchString(strings.ToLower(strings.TrimSpace(v))) {
		return "", errors.New("inbound_alias: use 3-64 lowercase letters, digits, dots or dashes")
	}
	c := inst.Credentials
	if from := strings.TrimSpace(c[FieldFromAddress]); from != "" {
		addr, err := mail.ParseAddress(from)
		if err != nil || addr.Name != "" {
			return "", errors.New("from_address must be a plain e-mail address")
		}
		if strings.EqualFold(domainOf(addr.Address), a.cfg.InboundDomain) {
			return "", errors.New("from_address cannot be on the receiving domain")
		}
	}
	switch provider(c) {
	case ProviderIMAP, ProviderSMTP:
		if provider(c) == ProviderIMAP {
			ic, err := IMAPConfiguration(inst)
			if err != nil {
				return "", err
			}
			if err := a.imapTest(ctx, ic); err != nil {
				return "", err
			}
		}
		sc, err := smtpConfig(c)
		if err != nil {
			return "", err
		}
		if c[FieldFromAddress] == "" {
			return "", errors.New("from_address is required with SMTP")
		}
		if err := a.smtpTest(ctx, sc); err != nil {
			return "", SMTPConnectionError(sc.Port, err)
		}
	case ProviderResend:
		if from := c[FieldFromAddress]; from != "" {
			d, found, err := a.resend.FindDomain(ctx, domainOf(from))
			if err != nil {
				return "", err
			}
			if !found || d.Status != "verified" {
				return "", fmt.Errorf("the domain %s is not verified yet: finish the DNS setup first", domainOf(from))
			}
		}
	default:
		return "", errors.New("provider must be resend, smtp or imap")
	}
	return Alias(inst), nil
}

// CheckHealth implements channels.HealthChecker: the sender domain must
// stay verified (or the SMTP login must work).
func (a *Adapter) CheckHealth(ctx context.Context, inst channels.Instance) (channels.Health, error) {
	c := inst.Credentials
	if provider(c) == ProviderIMAP {
		ic, err := IMAPConfiguration(inst)
		if err == nil {
			err = a.imapTest(ctx, ic)
		}
		if err != nil {
			return channels.Health{Detail: err.Error()}, nil
		}
	}
	if provider(c) == ProviderSMTP || provider(c) == ProviderIMAP {
		sc, err := smtpConfig(c)
		if err == nil {
			err = a.smtpTest(ctx, sc)
		}
		if err != nil {
			return channels.Health{Detail: err.Error()}, nil
		}
		return channels.Health{OK: true}, nil
	}
	if from := c[FieldFromAddress]; from != "" {
		d, found, err := a.resend.FindDomain(ctx, domainOf(from))
		if err != nil {
			return channels.Health{}, err
		}
		if !found || d.Status != "verified" {
			return channels.Health{Detail: "sender domain " + domainOf(from) + " is not verified"}, nil
		}
	}
	return channels.Health{OK: true}, nil
}

// webhookEvent is a Resend webhook body.
type webhookEvent struct {
	Type string `json:"type"`
	Data struct {
		EmailID   string          `json:"email_id"`
		From      string          `json:"from"`
		To        []string        `json:"to"`
		Cc        []string        `json:"cc"`
		Subject   string          `json:"subject"`
		MessageID string          `json:"message_id"`
		CreatedAt string          `json:"created_at"`
		Tags      json.RawMessage `json:"tags"`
		Bounce    *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			SubType string `json:"subType"`
		} `json:"bounce"`
	} `json:"data"`
}

func (e webhookEvent) tag(name string) string {
	var asMap map[string]string
	if json.Unmarshal(e.Data.Tags, &asMap) == nil {
		return asMap[name]
	}
	var asList []ResendTag
	if json.Unmarshal(e.Data.Tags, &asList) == nil {
		for _, t := range asList {
			if t.Name == name {
				return t.Value
			}
		}
	}
	return ""
}

// Route implements channels.Router: received mail routes by the aliases
// of its recipients on the receiving domain, delivery events by the
// instance tag of the sent mail.
func (a *Adapter) Route(_ *http.Request, body []byte) ([]string, error) {
	var ev webhookEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return nil, err
	}
	if ev.Type != "email.received" {
		if v := ev.tag(TagInstance); v != "" {
			return []string{v}, nil
		}
		return nil, nil
	}
	seen := map[string]bool{}
	var out []string
	for _, raw := range append(append([]string{}, ev.Data.To...), ev.Data.Cc...) {
		for _, addr := range addressList(raw) {
			local, domain, ok := strings.Cut(addr.Address, "@")
			if !ok || !strings.EqualFold(domain, a.cfg.InboundDomain) {
				continue
			}
			local, _, _ = strings.Cut(local, "+")
			if !seen[local] {
				seen[local] = true
				out = append(out, local)
			}
		}
	}
	return out, nil
}

// VerifyWebhook implements channels.Adapter (Svix signature of Resend).
func (a *Adapter) VerifyWebhook(r *http.Request, body []byte, _ channels.Instance) error {
	if !VerifySvix(a.cfg.WebhookSecret, r.Header, body, a.now()) {
		return channels.ErrSignature
	}
	return nil
}

// ParseWebhook implements channels.Adapter.
func (a *Adapter) ParseWebhook(ctx context.Context, _ *http.Request, body []byte, inst channels.Instance) (channels.WebhookResult, error) {
	var ev webhookEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return channels.WebhookResult{}, err
	}
	at := a.now()
	if t, err := time.Parse(time.RFC3339Nano, strings.Replace(ev.Data.CreatedAt, " ", "T", 1)); err == nil {
		at = t
	}
	switch ev.Type {
	case "email.received":
		raw, err := a.resend.RawReceived(ctx, ev.Data.EmailID)
		if err != nil {
			return channels.WebhookResult{}, err
		}
		return a.ParseRaw(raw, inst, ev.Data.EmailID)
	case "email.delivered":
		return channels.WebhookResult{Statuses: []channels.StatusUpdate{{ExternalID: ev.Data.EmailID, Status: channels.StatusDelivered, At: at}}}, nil
	case "email.bounced", "email.complained", "email.failed":
		reason := strings.TrimPrefix(ev.Type, "email.")
		if ev.Data.Bounce != nil && ev.Data.Bounce.Message != "" {
			reason += ": " + ev.Data.Bounce.Message
		}
		return channels.WebhookResult{Statuses: []channels.StatusUpdate{{
			ExternalID: ev.Data.EmailID, Status: channels.StatusFailed, Error: reason, At: at,
			Attributes: map[string]any{"event": ev.Type},
		}}}, nil
	}
	return channels.WebhookResult{}, nil
}

// Size caps of what an inbound message keeps.
const (
	maxHTMLBytes     = 256 << 10
	maxFullTextBytes = 64 << 10
	maxInboundFiles  = 20
)

// ParseRaw normalizes one raw inbound message for an instance. fallbackID
// identifies a message without a Message-ID header.
func (a *Adapter) ParseRaw(raw []byte, inst channels.Instance, fallbackID string) (channels.WebhookResult, error) {
	p, err := Parse(raw)
	if err != nil {
		return channels.WebhookResult{}, err
	}
	if p.Report != nil && p.Report.Action != "" {
		if p.Report.Action == "failed" && p.Report.OriginalMessageID != "" {
			return channels.WebhookResult{Statuses: []channels.StatusUpdate{{
				ExternalID: p.Report.OriginalMessageID, Status: channels.StatusFailed,
				Error: strings.TrimSpace("bounced " + p.Report.Status + " " + p.Report.Diagnostic), At: a.now(),
			}}}, nil
		}
		return channels.WebhookResult{}, nil
	}
	if AutoReason(p.Header, inst.UUID.String()) != "" {
		return channels.WebhookResult{}, nil
	}
	from := p.From
	if len(p.ReplyTo) > 0 && !strings.EqualFold(domainOf(p.ReplyTo[0].Address), a.cfg.InboundDomain) {
		from = Address{Name: firstNonEmpty(p.ReplyTo[0].Name, p.From.Name), Address: p.ReplyTo[0].Address}
	}
	if from.Address == "" || strings.EqualFold(domainOf(from.Address), a.cfg.InboundDomain) {
		return channels.WebhookResult{}, nil
	}
	id := p.MessageID
	if id == "" {
		id = "resend:" + fallbackID
	}
	text := VisibleReply(p.Text)
	attrs := map[string]any{"from": from, "to": p.To, "message_id": p.MessageID}
	if len(p.Cc) > 0 {
		attrs["cc"] = p.Cc
	}
	if p.HTML != "" {
		if h := SanitizeHTML(p.HTML); len(h) <= maxHTMLBytes {
			attrs["html"] = h
		} else {
			attrs["html_truncated"] = true
		}
	}
	if text != p.Text {
		attrs["full_text"] = truncateBytes(p.Text, maxFullTextBytes)
	}
	if len(p.InReplyTo) > 0 {
		attrs["in_reply_to"] = p.InReplyTo[0]
	}
	var files []channels.InboundAttachment
	for i, part := range p.Parts {
		if i >= maxInboundFiles {
			break
		}
		data := part.Data
		files = append(files, channels.InboundAttachment{
			Filename: part.Filename, ContentType: part.ContentType, Size: int64(len(data)),
			Open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil },
		})
	}
	if len(files) > 0 {
		var inline []string
		for _, part := range p.Parts {
			if part.Inline && part.ContentID != "" {
				inline = append(inline, part.ContentID)
			}
		}
		if len(inline) > 0 {
			attrs["inline_content_ids"] = inline
		}
	}
	at := p.Date
	if at.IsZero() {
		at = a.now()
	}
	replyTo := ""
	if len(p.InReplyTo) > 0 {
		replyTo = p.InReplyTo[0]
	}
	return channels.WebhookResult{Messages: []channels.InboundMessage{{
		ExternalID: id,
		From:       channels.Sender{ExternalID: from.Address, Name: from.Name, Email: from.Address},
		Kind:       "email", Text: text, Attachments: files,
		ThreadID: ThreadRoot(p), Subject: p.Subject, ReplyToExternalID: replyTo,
		Attributes: attrs, At: at,
	}}}, nil
}

// ThreadRoot is the thread id of a message: the root encoded in one of
// our own Message-IDs it answers, else the first References entry, else
// In-Reply-To, else its own Message-ID.
func ThreadRoot(p *Parsed) string {
	for _, id := range append(append([]string{}, p.InReplyTo...), p.References...) {
		if root := rootFromOwnID(id); root != "" {
			return root
		}
	}
	if len(p.References) > 0 {
		return p.References[0]
	}
	if len(p.InReplyTo) > 0 {
		return p.InReplyTo[0]
	}
	return p.MessageID
}

// OwnMessageID builds the Message-ID of an outbound message:
// <cf.{message uuid}.{base64url(thread root)}@domain>; roots too long for a
// header are left out (References still carries them).
func OwnMessageID(msg uuid.UUID, root, domain string) string {
	enc := ""
	if root != "" && len(root) <= 300 {
		enc = "." + base64.RawURLEncoding.EncodeToString([]byte(root))
	}
	return "<cf." + msg.String() + enc + "@" + domain + ">"
}

var ownIDRE = regexp.MustCompile(`^<cf\.[0-9a-f-]{36}\.([A-Za-z0-9_-]+)@[^>]+>$`)

func rootFromOwnID(id string) string {
	m := ownIDRE.FindStringSubmatch(id)
	if m == nil {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(m[1])
	if err != nil || !strings.HasPrefix(string(raw), "<") {
		return ""
	}
	return string(raw)
}

// Send implements channels.Adapter.
func (a *Adapter) Send(ctx context.Context, inst channels.Instance, msg channels.OutboundMessage) (channels.SendResult, error) {
	to := strings.TrimSpace(firstNonEmpty(msg.To.Email, msg.To.ExternalID))
	if _, err := mail.ParseAddress(to); err != nil {
		return channels.SendResult{}, channels.Permanent("invalid_recipient", fmt.Errorf("recipient %q is not an e-mail address", to))
	}
	c := inst.Credentials
	fromAddr := strings.TrimSpace(c[FieldFromAddress])
	if fromAddr == "" {
		fromAddr = a.Address(inst)
	}
	from := Address{Name: firstNonEmpty(c[FieldFromName], inst.Name), Address: fromAddr}
	subject := replySubject(msg.Subject, inst.Name)
	if fresh, _ := msg.Attributes[AttrNewThread].(bool); fresh && strings.TrimSpace(msg.Subject) != "" {
		// The first mail of a thread we start (a side conversation, CF-98).
		subject = strings.TrimSpace(msg.Subject)
	}
	root := msg.ThreadID
	messageID := OwnMessageID(msg.MessageUUID, root, domainOf(fromAddr))
	headers := map[string]string{
		"Message-ID":               messageID,
		HeaderInstance:             inst.UUID.String(),
		"X-Auto-Response-Suppress": "OOF, AutoReply",
	}
	refs := []string{}
	if root != "" {
		refs = append(refs, root)
	}
	if r := msg.ReplyToExternalID; strings.HasPrefix(r, "<") && r != root {
		refs = append(refs, r)
	}
	if len(refs) > 0 {
		headers["References"] = strings.Join(refs, " ")
		headers["In-Reply-To"] = refs[len(refs)-1]
	}
	cc, bcc := addrAttr(msg.Attributes, "cc"), addrAttr(msg.Attributes, "bcc")
	replyTo := a.Address(inst)
	if provider(c) == ProviderIMAP {
		replyTo = fromAddr
	}
	if v, ok := inst.Settings["brand_reply_to"].(string); ok && v != "" {
		replyTo = v
	}
	if provider(c) == ProviderSMTP || provider(c) == ProviderIMAP {
		sc, err := smtpConfig(c)
		if err != nil {
			return channels.SendResult{}, channels.Permanent("smtp_config", err)
		}
		files, err := a.loadFiles(ctx, msg.Attachments)
		if err != nil {
			return channels.SendResult{}, err
		}
		if replyTo != "" && (a.cfg.InboundDomain != "" || provider(c) == ProviderIMAP || inst.Settings["brand_reply_to"] != nil) {
			headers["Reply-To"] = replyTo
		}
		raw, err := Build(Outgoing{From: from, To: []string{to}, Cc: cc, Subject: subject, Text: msg.Text, HTML: TextToHTML(msg.Text), Headers: headers, Files: files})
		if err != nil {
			return channels.SendResult{}, channels.Permanent("invalid_message", err)
		}
		rcpt := append(append([]string{to}, cc...), bcc...)
		if err := a.smtpSend(ctx, sc, fromAddr, rcpt, raw); err != nil {
			var se *SMTPError
			if errors.As(err, &se) && se.Code >= 500 {
				return channels.SendResult{}, channels.Permanent("smtp_"+strconv.Itoa(se.Code), err)
			}
			return channels.SendResult{}, err
		}
		return channels.SendResult{ExternalID: messageID}, nil
	}
	req := SendRequest{
		From: from.String(), To: []string{to}, Cc: cc, Bcc: bcc, Subject: subject,
		Text: msg.Text, HTML: TextToHTML(msg.Text), Headers: headers,
		Tags: []ResendTag{{Name: TagInstance, Value: tagValue(Alias(inst))}},
	}
	if a.cfg.InboundDomain != "" || inst.Settings["brand_reply_to"] != nil {
		req.ReplyTo = []string{replyTo}
	}
	for _, f := range msg.Attachments {
		if f.URL != "" {
			req.Attachments = append(req.Attachments, ResendAttachment{Filename: sanitizeFilename(f.Filename), Path: f.URL})
		}
	}
	id, err := a.resend.Send(ctx, req, "cf-message/"+msg.MessageUUID.String())
	if err != nil {
		var ae *APIError
		if errors.As(err, &ae) && !ae.Temporary() {
			return channels.SendResult{}, channels.Permanent("resend_"+strconv.Itoa(ae.Status), err)
		}
		return channels.SendResult{}, err
	}
	return channels.SendResult{ExternalID: id}, nil
}

// tagValue keeps Resend's tag charset (ASCII letters, digits, _ and -).
func tagValue(v string) string {
	return strings.Map(func(r rune) rune {
		if r == '.' {
			return '_'
		}
		return r
	}, v)
}

func (a *Adapter) loadFiles(ctx context.Context, in []channels.OutboundAttachment) ([]File, error) {
	var out []File
	var total int64
	for _, f := range in {
		if f.Open == nil {
			continue
		}
		rc, err := f.Open(ctx)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, a.cfg.MaxAttachmentBytes+1))
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		total += int64(len(data))
		if total > a.cfg.MaxAttachmentBytes {
			return nil, channels.Permanent("attachments_too_large", errors.New("attachments exceed the e-mail size limit"))
		}
		out = append(out, File{Filename: f.Filename, ContentType: f.ContentType, Data: data})
	}
	return out, nil
}

var replyPrefixRE = regexp.MustCompile(`(?i)^\s*(re|ynt|yanıt|cevap|aw|sv)\s*:`)

func replySubject(subject, fallback string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return fallback
	}
	if replyPrefixRE.MatchString(subject) {
		return subject
	}
	return "Re: " + subject
}

func addrAttr(attrs map[string]any, key string) []string {
	var out []string
	add := func(v string) {
		if addr, err := mail.ParseAddress(strings.TrimSpace(v)); err == nil {
			out = append(out, addr.Address)
		}
	}
	switch v := attrs[key].(type) {
	case []any:
		for _, x := range v {
			switch x := x.(type) {
			case string:
				add(x)
			case map[string]any:
				if s, ok := x["address"].(string); ok {
					add(s)
				}
			}
		}
	case []string:
		for _, x := range v {
			add(x)
		}
	case string:
		for _, x := range strings.Split(v, ",") {
			add(x)
		}
	}
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

func provider(c channels.Credentials) string {
	p := strings.ToLower(strings.TrimSpace(c[FieldProvider]))
	if p == "" {
		return ProviderResend
	}
	return p
}

func smtpConfig(c channels.Credentials) (SMTPConfig, error) {
	sc := SMTPConfig{
		Host: strings.TrimSpace(c[FieldSMTPHost]), Port: strings.TrimSpace(c[FieldSMTPPort]),
		Username: c[FieldSMTPUsername], Password: c[FieldSMTPPassword], TLS: strings.ToLower(strings.TrimSpace(c[FieldSMTPTLS])),
	}
	if sc.Host == "" {
		return sc, errors.New("smtp_host is required")
	}
	if sc.Port == "" {
		sc.Port = "587"
	}
	if _, err := strconv.Atoi(sc.Port); err != nil {
		return sc, errors.New("smtp_port must be a number")
	}
	if sc.TLS == "" {
		sc.TLS = "starttls"
		if sc.Port == "465" {
			sc.TLS = "implicit"
		}
	}
	if sc.TLS != "starttls" && sc.TLS != "implicit" {
		return sc, errors.New("smtp_tls must be starttls or implicit")
	}
	return sc, nil
}

func domainOf(addr string) string {
	if a, err := mail.ParseAddress(addr); err == nil {
		addr = a.Address
	}
	_, d, _ := strings.Cut(addr, "@")
	return strings.ToLower(d)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// SMTPPortHint explains this deployment's SMTP egress restriction.
const SMTPPortHint = "Sunucu sağlayıcısı giden 25/465/587 SMTP portlarını kısıtlıyor. Kendi SMTP sunucunuza bağlantı zaman aşımına uğradı. Resend / yönlendirme modunu kullanın."

// SMTPConnectionError only classifies timeouts on the restricted submission ports.
func SMTPConnectionError(port string, err error) error {
	var ne net.Error
	if (port == "465" || port == "587") && (errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne) && ne.Timeout()) {
		return &SMTPPortRestrictionError{Err: err}
	}
	return err
}

type SMTPPortRestrictionError struct{ Err error }

func (e *SMTPPortRestrictionError) Error() string { return SMTPPortHint }
func (e *SMTPPortRestrictionError) Unwrap() error { return e.Err }
