package telegram

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

type sentMessage struct {
	MessageID int64 `json:"message_id"`
	Chat      chat  `json:"chat"`
}

type sendStep struct {
	method string
	field  string // media field name; "" for sendMessage
	att    *channels.OutboundAttachment
	params map[string]any
}

// Send implements channels.Adapter.
func (a *Adapter) Send(ctx context.Context, inst channels.Instance, msg channels.OutboundMessage) (channels.SendResult, error) {
	token, err := tokenOf(inst)
	if err != nil {
		return channels.SendResult{}, channels.Permanent("token_invalid", err)
	}
	chatID := chatParam(msg.To.ExternalID)
	if chatID == nil {
		return channels.SendResult{}, channels.Permanent("invalid_recipient", errors.New("telegram: recipient chat id is empty"))
	}

	text := msg.Text
	caption := ""
	if len(msg.Attachments) > 0 && utf16Len(text) <= maxCaption {
		caption, text = text, ""
	}
	var steps []sendStep
	for _, chunk := range splitText(text, maxText) {
		steps = append(steps, sendStep{method: "sendMessage", params: map[string]any{"chat_id": chatID, "text": chunk}})
	}
	for i := range msg.Attachments {
		att := &msg.Attachments[i]
		method, field := mediaMethod(*att)
		p := map[string]any{"chat_id": chatID}
		if i == 0 && strings.TrimSpace(caption) != "" {
			p["caption"] = caption
		}
		steps = append(steps, sendStep{method: method, field: field, att: att, params: p})
	}
	if len(steps) == 0 {
		return channels.SendResult{}, channels.Permanent("empty_message", errors.New("telegram: nothing to send"))
	}
	if r := replyParam(msg.ReplyToExternalID); r != nil {
		steps[0].params["reply_parameters"] = r
	}
	if kb := inlineKeyboard(msg.Attributes["buttons"]); kb != nil {
		steps[len(steps)-1].params["reply_markup"] = kb
	}

	var last sentMessage
	for _, s := range steps {
		if err := a.sendOne(ctx, token, s, &last); err != nil {
			return channels.SendResult{}, mapSendError(err)
		}
	}
	return channels.SendResult{ExternalID: msgRef(last.Chat.ID, last.MessageID)}, nil
}

func (a *Adapter) sendOne(ctx context.Context, token string, s sendStep, out *sentMessage) error {
	if s.att == nil {
		return a.call(ctx, token, s.method, s.params, out)
	}
	if s.att.URL != "" {
		s.params[s.field] = s.att.URL
		return a.call(ctx, token, s.method, s.params, out)
	}
	if s.att.Open == nil {
		return channels.Permanent("attachment_missing", errors.New("telegram: attachment has neither URL nor content"))
	}
	return a.upload(ctx, token, s.method, s.params, s.field, *s.att, out)
}

// mediaMethod picks the Bot API method and field for an attachment.
func mediaMethod(att channels.OutboundAttachment) (method, field string) {
	ct := strings.ToLower(att.ContentType)
	switch {
	case strings.HasPrefix(ct, "image/") && ct != "image/svg+xml" && ct != "image/gif" && att.Size <= maxPhotoURLSize:
		return "sendPhoto", "photo"
	case strings.HasPrefix(ct, "video/"):
		return "sendVideo", "video"
	case strings.HasPrefix(ct, "audio/"):
		return "sendAudio", "audio"
	}
	return "sendDocument", "document"
}

// chatParam turns a chat id string into the chat_id value (nil if empty).
func chatParam(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if id, err := strconv.ParseInt(s, 10, 64); err == nil {
		return id
	}
	return s // @channelusername
}

// replyParam builds reply_parameters from "<chat>:<message_id>".
func replyParam(ref string) map[string]any {
	if ref == "" {
		return nil
	}
	id, err := strconv.ParseInt(ref[strings.LastIndexByte(ref, ':')+1:], 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	return map[string]any{"message_id": id, "allow_sending_without_reply": true}
}

// inlineKeyboard converts Attributes["buttons"] ([{id,title}|{title,url}])
// into an inline keyboard, one button per row.
func inlineKeyboard(v any) map[string]any {
	var items []map[string]any
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			if m, ok := e.(map[string]any); ok {
				items = append(items, m)
			}
		}
	case []map[string]any:
		items = t
	}
	var rows [][]map[string]any
	for _, m := range items {
		title := str(m["title"])
		if title == "" {
			continue
		}
		btn := map[string]any{"text": title}
		if u := str(m["url"]); u != "" {
			btn["url"] = u
		} else {
			data := str(m["id"])
			if data == "" {
				data = title
			}
			btn["callback_data"] = truncateBytes(data, maxCallbackData)
		}
		rows = append(rows, []map[string]any{btn})
	}
	if len(rows) == 0 {
		return nil
	}
	return map[string]any{"inline_keyboard": rows}
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case fmt.Stringer:
		return t.String()
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := 0
	for i := range s { // rune start offsets; cut before the rune that overflows
		if i > n {
			break
		}
		cut = i
	}
	return s[:cut]
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// splitText cuts s into chunks of at most limit UTF-16 units on rune
// boundaries, preferring a newline, then a space, in the chunk's second half.
func splitText(s string, limit int) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	if utf16Len(s) <= limit {
		return []string{s}
	}
	var out []string
	runes := []rune(s)
	for len(runes) > 0 {
		n, units := 0, 0
		for n < len(runes) {
			w := 1
			if runes[n] >= 0x10000 {
				w = 2
			}
			if units+w > limit {
				break
			}
			units += w
			n++
		}
		if n < len(runes) {
			if cut := lastBreak(runes[:n], func(r rune) bool { return r == '\n' }); cut > n/2 {
				n = cut
			} else if cut := lastBreak(runes[:n], unicode.IsSpace); cut > n/2 {
				n = cut
			}
		}
		if chunk := string(runes[:n]); strings.TrimSpace(chunk) != "" {
			out = append(out, chunk)
		}
		runes = runes[n:]
	}
	return out
}

// lastBreak returns the index just after the last rune matching f (0 = none).
func lastBreak(rs []rune, f func(rune) bool) int {
	for i := len(rs) - 1; i >= 0; i-- {
		if f(rs[i]) {
			return i + 1
		}
	}
	return 0
}

// mapSendError classifies Bot API failures for the outbound queue.
func mapSendError(err error) error {
	var se *channels.SendError
	if errors.As(err, &se) {
		return err
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		return &channels.SendError{Code: "network", Err: err}
	}
	d := strings.ToLower(ae.Description)
	switch {
	case ae.Code == http.StatusUnauthorized, ae.Code == http.StatusNotFound:
		return channels.Permanent("token_invalid", err)
	case ae.Code == http.StatusForbidden:
		code := "forbidden"
		switch {
		case strings.Contains(d, "blocked"):
			code = "bot_blocked"
		case strings.Contains(d, "deactivated"):
			code = "user_deactivated"
		case strings.Contains(d, "kicked"), strings.Contains(d, "not a member"):
			code = "bot_kicked"
		}
		return channels.Permanent(code, err)
	case ae.Code == http.StatusTooManyRequests:
		return &channels.SendError{Code: "rate_limited", Err: fmt.Errorf("%w (retry after %ds)", err, ae.RetryAfter)}
	case ae.Code >= 500:
		return &channels.SendError{Code: "unavailable", Err: err}
	case ae.MigrateToChatID != 0:
		return channels.Permanent("chat_migrated", fmt.Errorf("%w (new chat id %d)", err, ae.MigrateToChatID))
	case strings.Contains(d, "chat not found"):
		return channels.Permanent("chat_not_found", err)
	case strings.Contains(d, "failed to get http url content"):
		// Telegram could not fetch our signed attachment URL; may be transient.
		return &channels.SendError{Code: "media_fetch_failed", Err: err}
	case ae.Code == http.StatusBadRequest:
		return channels.Permanent("bad_request", err)
	}
	return &channels.SendError{Code: "unknown", Err: err}
}
