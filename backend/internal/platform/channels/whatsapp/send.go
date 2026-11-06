package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/format"
)

// Send implements channels.Adapter. A template wins over attachments,
// attachments over interactive buttons/lists, those over plain text.
func (a *Adapter) Send(ctx context.Context, inst channels.Instance, msg channels.OutboundMessage) (channels.SendResult, error) {
	to := digits(firstNonEmpty(msg.To.ExternalID, msg.To.Phone))
	if to == "" {
		return channels.SendResult{}, channels.Permanent("invalid_recipient", errors.New("whatsapp: recipient has no phone number"))
	}
	if msg.Template == nil {
		// Source format → WhatsApp marks (CF-177); the limit applies to what
		// is actually sent. Approved templates are never rewritten.
		msg.Text = format.WhatsApp(msg.Text)
	}
	if len([]rune(msg.Text)) > 4096 {
		return channels.SendResult{}, channels.Permanent("too_long", errors.New("whatsapp: text longer than 4096 characters"))
	}
	base := func(kind string) map[string]any {
		b := map[string]any{"messaging_product": "whatsapp", "recipient_type": "individual", "to": to, "type": kind}
		if msg.ReplyToExternalID != "" {
			b["context"] = map[string]any{"message_id": msg.ReplyToExternalID}
		}
		return b
	}
	switch {
	case msg.Template != nil:
		b := base("template")
		b["template"] = templatePayload(*msg.Template)
		return a.post(ctx, inst, b)
	case len(msg.Attachments) > 0:
		return a.sendMedia(ctx, inst, msg, base)
	case msg.Attributes["buttons"] != nil || msg.Attributes["list"] != nil:
		inter, err := interactivePayload(msg)
		if err != nil {
			return channels.SendResult{}, channels.Permanent("invalid_interactive", err)
		}
		b := base("interactive")
		b["interactive"] = inter
		return a.post(ctx, inst, b)
	case strings.TrimSpace(msg.Text) == "":
		return channels.SendResult{}, channels.Permanent("empty", errors.New("whatsapp: nothing to send"))
	default:
		b := base("text")
		b["text"] = map[string]any{"body": msg.Text, "preview_url": hasURL(msg.Text)}
		return a.post(ctx, inst, b)
	}
}

func hasURL(s string) bool {
	return strings.Contains(s, "https://") || strings.Contains(s, "http://")
}

func templatePayload(t channels.Template) map[string]any {
	lang := firstNonEmpty(t.Language, DefaultLanguage)
	tpl := map[string]any{"name": t.Name, "language": map[string]any{"code": lang}}
	if len(t.Parameters) > 0 {
		params := make([]map[string]any, 0, len(t.Parameters))
		for _, v := range t.Parameters {
			if strings.TrimSpace(v) == "" {
				v = "-" // the Cloud API refuses an empty parameter
			}
			params = append(params, map[string]any{"type": "text", "text": v})
		}
		tpl["components"] = []map[string]any{{"type": "body", "parameters": params}}
	}
	return tpl
}

// mediaKind maps a content type to the Cloud API message type.
func mediaKind(contentType string) string {
	switch ct := strings.ToLower(contentType); {
	case strings.HasPrefix(ct, "image/"):
		return "image"
	case strings.HasPrefix(ct, "video/"):
		return "video"
	case strings.HasPrefix(ct, "audio/"):
		return "audio"
	default:
		return "document"
	}
}

func (a *Adapter) sendMedia(ctx context.Context, inst channels.Instance, msg channels.OutboundMessage, base func(string) map[string]any) (channels.SendResult, error) {
	caption := strings.TrimSpace(msg.Text)
	var last channels.SendResult
	for i, att := range msg.Attachments {
		if att.URL == "" {
			return last, channels.Permanent("media_url_missing", fmt.Errorf("whatsapp: attachment %q has no url", att.Filename))
		}
		kind := mediaKind(att.ContentType)
		if i == 0 && kind == "audio" && caption != "" {
			b := base("text")
			b["text"] = map[string]any{"body": caption, "preview_url": hasURL(caption)}
			if _, err := a.post(ctx, inst, b); err != nil {
				return last, err
			}
			caption = ""
		}
		media := map[string]any{"link": att.URL}
		if i == 0 && caption != "" && kind != "audio" {
			media["caption"] = caption
		}
		if kind == "document" && att.Filename != "" {
			media["filename"] = att.Filename
		}
		b := base(kind)
		if i > 0 {
			delete(b, "context")
		}
		b[kind] = media
		res, err := a.post(ctx, inst, b)
		if err != nil {
			return last, err
		}
		last = res
	}
	return last, nil
}

// interactivePayload builds a reply-button message from
// Attributes["buttons"] ([{id,title}], max 3) or a list message from
// Attributes["list"] ({button, sections:[{title, rows:[{id,title,description}]}]}
// or a bare rows array).
func interactivePayload(msg channels.OutboundMessage) (map[string]any, error) {
	body := strings.TrimSpace(msg.Text)
	if body == "" {
		return nil, errors.New("interactive messages need a body text")
	}
	if raw := msg.Attributes["buttons"]; raw != nil {
		items := objects(raw)
		if len(items) == 0 || len(items) > 3 {
			return nil, errors.New("1 to 3 buttons are allowed")
		}
		buttons := make([]map[string]any, 0, len(items))
		for i, it := range items {
			title := truncate(str(it["title"]), 20)
			if title == "" {
				return nil, fmt.Errorf("button %d has no title", i+1)
			}
			id := firstNonEmpty(str(it["id"]), "btn_"+strconv.Itoa(i+1))
			buttons = append(buttons, map[string]any{"type": "reply", "reply": map[string]any{"id": truncate(id, 256), "title": title}})
		}
		return map[string]any{"type": "button", "body": map[string]any{"text": body}, "action": map[string]any{"buttons": buttons}}, nil
	}
	button, sections := "Options", []map[string]any{}
	switch l := msg.Attributes["list"].(type) {
	case map[string]any:
		button = firstNonEmpty(str(l["button"]), button)
		for _, s := range objects(l["sections"]) {
			rows, err := listRows(s["rows"])
			if err != nil {
				return nil, err
			}
			sec := map[string]any{"rows": rows}
			if t := str(s["title"]); t != "" {
				sec["title"] = truncate(t, 24)
			}
			sections = append(sections, sec)
		}
	default:
		rows, err := listRows(l)
		if err != nil {
			return nil, err
		}
		sections = append(sections, map[string]any{"rows": rows})
	}
	total := 0
	for _, s := range sections {
		total += len(s["rows"].([]map[string]any))
	}
	if total == 0 || total > 10 {
		return nil, errors.New("1 to 10 list rows are allowed")
	}
	return map[string]any{
		"type": "list", "body": map[string]any{"text": body},
		"action": map[string]any{"button": truncate(button, 20), "sections": sections},
	}, nil
}

func listRows(raw any) ([]map[string]any, error) {
	rows := []map[string]any{}
	for i, r := range objects(raw) {
		title := truncate(str(r["title"]), 24)
		if title == "" {
			return nil, fmt.Errorf("list row %d has no title", i+1)
		}
		row := map[string]any{"id": truncate(firstNonEmpty(str(r["id"]), "row_"+strconv.Itoa(i+1)), 200), "title": title}
		if d := str(r["description"]); d != "" {
			row["description"] = truncate(d, 72)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// objects reads a JSON-ish array of objects ([]any of map[string]any, or
// the typed forms Go callers build).
func objects(raw any) []map[string]any {
	switch v := raw.(type) {
	case []map[string]any:
		return v
	case []map[string]string:
		out := make([]map[string]any, 0, len(v))
		for _, m := range v {
			o := map[string]any{}
			for k, s := range m {
				o[k] = s
			}
			out = append(out, o)
		}
		return out
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, it := range v {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

func str(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// post sends one message and maps Graph errors onto channels send errors.
func (a *Adapter) post(ctx context.Context, inst channels.Instance, body map[string]any) (channels.SendResult, error) {
	id := strings.TrimSpace(inst.Credentials[FieldPhoneNumberID])
	if !ValidID(id) {
		return channels.SendResult{}, channels.Permanent("not_configured", errors.New("whatsapp: phone_number_id is not set"))
	}
	raw, err := a.call(ctx, http.MethodPost, id+"/messages", nil, body, inst.Credentials[FieldAccessToken])
	if err != nil {
		return channels.SendResult{}, mapSendError(err)
	}
	var out struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Messages) == 0 || out.Messages[0].ID == "" {
		return channels.SendResult{}, &Error{Status: http.StatusOK, Message: "answer without a message id"}
	}
	return channels.SendResult{ExternalID: out.Messages[0].ID}, nil
}

// mapSendError classifies a Graph error: retryable errors stay plain.
func mapSendError(err error) error {
	var e *Error
	if !errors.As(err, &e) {
		return err // network: temporary
	}
	switch {
	case e.Code == 131047:
		return channels.Permanent("window_closed", fmt.Errorf("%w: %w", channels.ErrWindowClosed, e))
	case e.Code == 190 || e.Status == http.StatusUnauthorized:
		return channels.Permanent("token_invalid", e)
	case e.Status == http.StatusTooManyRequests, e.Status >= 500,
		e.Code == 130429, e.Code == 131056, e.Code == 80007, e.Code == 4, e.Code == 131000:
		return err
	case e.Status >= 400:
		code := "rejected"
		if e.Code != 0 {
			code = strconv.Itoa(e.Code)
		}
		return channels.Permanent(code, e)
	}
	return err
}

// MarkRead marks an inbound message read; with typing it also shows the
// typing indicator to the contact (up to 25s or until the next reply).
func (a *Adapter) MarkRead(ctx context.Context, inst channels.Instance, messageID string, typing bool) error {
	id := strings.TrimSpace(inst.Credentials[FieldPhoneNumberID])
	if !ValidID(id) || messageID == "" {
		return errors.New("whatsapp: phone_number_id and message id are required")
	}
	body := map[string]any{"messaging_product": "whatsapp", "status": "read", "message_id": messageID}
	if typing {
		body["typing_indicator"] = map[string]any{"type": "text"}
	}
	_, err := a.call(ctx, http.MethodPost, id+"/messages", nil, body, inst.Credentials[FieldAccessToken])
	return err
}
