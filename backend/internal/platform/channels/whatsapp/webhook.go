package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

// ErrMalformed: the body is not a WhatsApp Business Account webhook.
var ErrMalformed = errors.New("whatsapp: malformed webhook")

type wireMedia struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	Voice    bool   `json:"voice"`
	Animated bool   `json:"animated"`
}

type wireReply struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type wireMessage struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Context   struct {
		ID string `json:"id"`
	} `json:"context"`
	Text struct {
		Body string `json:"body"`
	} `json:"text"`
	Image    *wireMedia `json:"image"`
	Audio    *wireMedia `json:"audio"`
	Video    *wireMedia `json:"video"`
	Document *wireMedia `json:"document"`
	Sticker  *wireMedia `json:"sticker"`
	Location struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Name      string  `json:"name"`
		Address   string  `json:"address"`
		URL       string  `json:"url"`
	} `json:"location"`
	Contacts []struct {
		Name struct {
			FormattedName string `json:"formatted_name"`
		} `json:"name"`
		Phones []struct {
			Phone string `json:"phone"`
			WaID  string `json:"wa_id"`
		} `json:"phones"`
		Emails []struct {
			Email string `json:"email"`
		} `json:"emails"`
	} `json:"contacts"`
	Interactive struct {
		Type        string    `json:"type"`
		ButtonReply wireReply `json:"button_reply"`
		ListReply   wireReply `json:"list_reply"`
	} `json:"interactive"`
	Button struct {
		Payload string `json:"payload"`
		Text    string `json:"text"`
	} `json:"button"`
	Reaction struct {
		MessageID string `json:"message_id"`
		Emoji     string `json:"emoji"`
	} `json:"reaction"`
	Referral map[string]any `json:"referral"`
	Errors   []wireError    `json:"errors"`
}

type wireError struct {
	Code    int    `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

type wireStatus struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Timestamp    string `json:"timestamp"`
	RecipientID  string `json:"recipient_id"`
	Conversation *struct {
		ID     string `json:"id"`
		Origin struct {
			Type string `json:"type"`
		} `json:"origin"`
	} `json:"conversation"`
	Pricing *struct {
		Billable     *bool  `json:"billable"`
		PricingModel string `json:"pricing_model"`
		Category     string `json:"category"`
	} `json:"pricing"`
	Errors []wireError `json:"errors"`
}

type wireWebhook struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				Metadata struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberID      string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					WaID    string `json:"wa_id"`
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
				} `json:"contacts"`
				Messages []wireMessage `json:"messages"`
				Statuses []wireStatus  `json:"statuses"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

func decodeWebhook(body []byte) (wireWebhook, error) {
	var w wireWebhook
	if err := json.Unmarshal(body, &w); err != nil || w.Object != "whatsapp_business_account" {
		return wireWebhook{}, ErrMalformed
	}
	return w, nil
}

func unix(v string) time.Time {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}

func errorText(es []wireError) string {
	if len(es) == 0 {
		return ""
	}
	e := es[0]
	msg := strings.TrimSpace(e.Title)
	if m := strings.TrimSpace(e.Message); m != "" && m != msg {
		if msg != "" {
			msg += ": "
		}
		msg += m
	}
	if e.Code != 0 {
		msg = strings.TrimSpace(strconv.Itoa(e.Code) + " " + msg)
	}
	return msg
}

// ParseWebhook implements channels.Adapter: only the changes addressed to
// the instance's phone number are kept.
func (a *Adapter) ParseWebhook(_ context.Context, _ *http.Request, body []byte, inst channels.Instance) (channels.WebhookResult, error) {
	w, err := decodeWebhook(body)
	if err != nil {
		return channels.WebhookResult{}, err
	}
	phoneID := firstNonEmpty(inst.ExternalID, inst.Credentials[FieldPhoneNumberID])
	if phoneID == "" {
		return channels.WebhookResult{}, errors.New("whatsapp: instance has no phone_number_id")
	}
	token := inst.Credentials[FieldAccessToken]
	var out channels.WebhookResult
	for _, e := range w.Entry {
		for _, c := range e.Changes {
			v := c.Value
			if c.Field != "messages" || v.Metadata.PhoneNumberID != phoneID {
				continue
			}
			names := map[string]string{}
			for _, ct := range v.Contacts {
				names[ct.WaID] = strings.TrimSpace(ct.Profile.Name)
			}
			for _, m := range v.Messages {
				if m.ID == "" || m.From == "" {
					continue
				}
				out.Messages = append(out.Messages, a.inbound(m, names[m.From], token))
			}
			for _, s := range v.Statuses {
				if st, ok := status(s); ok {
					out.Statuses = append(out.Statuses, st)
				}
			}
		}
	}
	return out, nil
}

func (a *Adapter) inbound(m wireMessage, name, token string) channels.InboundMessage {
	wa := digits(m.From)
	msg := channels.InboundMessage{
		ExternalID: m.ID,
		From: channels.Sender{
			ExternalID: wa, InstanceScoped: false, Name: name, Phone: "+" + wa,
		},
		Kind: "text", ReplyToExternalID: m.Context.ID, At: unix(m.Timestamp),
		Attributes: map[string]any{},
	}
	if m.Type != "text" {
		msg.Attributes["wa_type"] = m.Type
	}
	if len(m.Referral) > 0 {
		msg.Attributes["referral"] = m.Referral
	}
	switch m.Type {
	case "text":
		msg.Text = m.Text.Body
	case "image", "audio", "video", "document", "sticker":
		md := map[string]*wireMedia{"image": m.Image, "audio": m.Audio, "video": m.Video, "document": m.Document, "sticker": m.Sticker}[m.Type]
		if md == nil || md.ID == "" {
			msg.Kind = "unsupported"
			break
		}
		msg.Kind, msg.Text = "media", md.Caption
		msg.Attachments = []channels.InboundAttachment{a.attachment(m.Type, *md, token)}
		if md.Voice {
			msg.Attributes["voice"] = true
		}
	case "location":
		l := m.Location
		parts := []string{}
		for _, p := range []string{l.Name, l.Address} {
			if strings.TrimSpace(p) != "" {
				parts = append(parts, strings.TrimSpace(p))
			}
		}
		parts = append(parts, fmt.Sprintf("https://maps.google.com/?q=%s,%s",
			strconv.FormatFloat(l.Latitude, 'f', -1, 64), strconv.FormatFloat(l.Longitude, 'f', -1, 64)))
		msg.Text = strings.Join(parts, "\n")
		msg.Attributes["location"] = map[string]any{
			"latitude": l.Latitude, "longitude": l.Longitude, "name": l.Name, "address": l.Address, "url": l.URL,
		}
	case "contacts":
		lines := []string{}
		cards := []map[string]any{}
		for _, c := range m.Contacts {
			phones, emails := []string{}, []string{}
			for _, p := range c.Phones {
				phones = append(phones, firstNonEmpty(p.Phone, p.WaID))
			}
			for _, e := range c.Emails {
				emails = append(emails, e.Email)
			}
			lines = append(lines, strings.TrimSpace(c.Name.FormattedName+" "+strings.Join(append(phones, emails...), ", ")))
			cards = append(cards, map[string]any{"name": c.Name.FormattedName, "phones": phones, "emails": emails})
		}
		msg.Text = strings.Join(lines, "\n")
		msg.Attributes["contacts"] = cards
	case "interactive":
		r := m.Interactive.ButtonReply
		if m.Interactive.Type == "list_reply" {
			r = m.Interactive.ListReply
		}
		msg.Kind, msg.Text = "interactive", r.Title
		msg.Attributes["reply_id"] = r.ID
		if r.Description != "" {
			msg.Attributes["reply_description"] = r.Description
		}
	case "button":
		msg.Kind, msg.Text = "interactive", m.Button.Text
		msg.Attributes["reply_id"] = m.Button.Payload
	case "reaction":
		msg.Text = m.Reaction.Emoji
		msg.Attributes["reaction"] = m.Reaction.Emoji
		msg.Attributes["reacted_message_id"] = m.Reaction.MessageID
	default:
		msg.Kind = "unsupported"
		if e := errorText(m.Errors); e != "" {
			msg.Attributes["error"] = e
		}
	}
	return msg
}

func status(s wireStatus) (channels.StatusUpdate, bool) {
	switch s.Status {
	case channels.StatusSent, channels.StatusDelivered, channels.StatusRead, channels.StatusFailed:
	default:
		return channels.StatusUpdate{}, false
	}
	st := channels.StatusUpdate{ExternalID: s.ID, Status: s.Status, At: unix(s.Timestamp), Error: errorText(s.Errors)}
	attrs := map[string]any{}
	if s.RecipientID != "" {
		attrs["recipient_id"] = s.RecipientID
	}
	if p := s.Pricing; p != nil {
		if p.Billable != nil {
			attrs["billable"] = *p.Billable
		}
		if p.Category != "" {
			attrs["pricing_category"] = p.Category
		}
		if p.PricingModel != "" {
			attrs["pricing_model"] = p.PricingModel
		}
	}
	if c := s.Conversation; c != nil {
		if c.ID != "" {
			attrs["conversation_id"] = c.ID
		}
		if c.Origin.Type != "" {
			attrs["conversation_origin"] = c.Origin.Type
		}
	}
	if len(attrs) > 0 {
		st.Attributes = attrs
	}
	return st, true
}

func (a *Adapter) attachment(kind string, md wireMedia, token string) channels.InboundAttachment {
	ct := md.MimeType
	if base, _, err := mime.ParseMediaType(ct); err == nil {
		ct = base
	}
	name := strings.TrimSpace(md.Filename)
	if name == "" {
		name = kind + "-" + md.ID
		if exts, _ := mime.ExtensionsByType(ct); len(exts) > 0 {
			name += exts[0]
		}
	}
	id := md.ID
	return channels.InboundAttachment{
		Filename: name, ContentType: ct,
		Open: func(ctx context.Context) (io.ReadCloser, error) { return a.OpenMedia(ctx, token, id) },
	}
}

// OpenMedia streams an inbound media file: GET /{media_id} answers a
// short-lived URL that is downloaded with the same token (max 25MB).
func (a *Adapter) OpenMedia(ctx context.Context, token, mediaID string) (io.ReadCloser, error) {
	if !ValidID(mediaID) {
		return nil, errors.New("whatsapp: invalid media id")
	}
	body, err := a.call(ctx, http.MethodGet, mediaID, nil, nil, token)
	if err != nil {
		return nil, err
	}
	var meta struct {
		URL      string `json:"url"`
		FileSize int64  `json:"file_size"`
	}
	if err := json.Unmarshal(body, &meta); err != nil || meta.URL == "" {
		return nil, &Error{Status: http.StatusOK, Message: "media answer without a url"}
	}
	if meta.FileSize > MaxMediaSize {
		return nil, fmt.Errorf("whatsapp: media is %d bytes, more than %d", meta.FileSize, MaxMediaSize)
	}
	if u, err := url.Parse(meta.URL); err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("whatsapp: invalid media url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, meta.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := a.media.Do(req)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: download media: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_ = res.Body.Close()
		return nil, &Error{Status: res.StatusCode, Message: "media download failed"}
	}
	if res.ContentLength > MaxMediaSize {
		_ = res.Body.Close()
		return nil, fmt.Errorf("whatsapp: media is %d bytes, more than %d", res.ContentLength, MaxMediaSize)
	}
	return &capReader{rc: res.Body, left: MaxMediaSize}, nil
}

// capReader fails a stream that grows past its cap instead of truncating it.
type capReader struct {
	rc   io.ReadCloser
	left int64
}

func (c *capReader) Read(p []byte) (int, error) {
	if int64(len(p)) > c.left+1 {
		p = p[:c.left+1]
	}
	n, err := c.rc.Read(p)
	c.left -= int64(n)
	if c.left < 0 {
		return 0, fmt.Errorf("whatsapp: media larger than %d bytes", MaxMediaSize)
	}
	return n, err
}

func (c *capReader) Close() error { return c.rc.Close() }
