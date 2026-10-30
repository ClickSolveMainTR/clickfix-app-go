package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

// Wire types: the subset of the Bot API objects the adapter reads.

type user struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Username     string `json:"username"`
	LanguageCode string `json:"language_code"`
}

func (u *user) FullName() string {
	if u == nil {
		return ""
	}
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}

type chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type entity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	User   *user  `json:"user"`
}

type photoSize struct {
	FileID   string `json:"file_id"`
	UniqueID string `json:"file_unique_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int64  `json:"file_size"`
}

// file covers document, video, video_note, voice, audio, sticker, animation.
type file struct {
	FileID     string `json:"file_id"`
	UniqueID   string `json:"file_unique_id"`
	FileName   string `json:"file_name"`
	MimeType   string `json:"mime_type"`
	FileSize   int64  `json:"file_size"`
	IsAnimated bool   `json:"is_animated"`
	IsVideo    bool   `json:"is_video"`
	Emoji      string `json:"emoji"`
	Title      string `json:"title"`
	Performer  string `json:"performer"`
}

type contact struct {
	PhoneNumber string `json:"phone_number"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	UserID      int64  `json:"user_id"`
}

type location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type venue struct {
	Location location `json:"location"`
	Title    string   `json:"title"`
	Address  string   `json:"address"`
}

type inlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
	URL          string `json:"url"`
}

type message struct {
	MessageID       int64           `json:"message_id"`
	MessageThreadID int64           `json:"message_thread_id"`
	From            *user           `json:"from"`
	Chat            chat            `json:"chat"`
	Date            int64           `json:"date"`
	Text            string          `json:"text"`
	Caption         string          `json:"caption"`
	Entities        []entity        `json:"entities"`
	CaptionEntities []entity        `json:"caption_entities"`
	ReplyToMessage  *message        `json:"reply_to_message"`
	Photo           []photoSize     `json:"photo"`
	Document        *file           `json:"document"`
	Animation       *file           `json:"animation"`
	Video           *file           `json:"video"`
	VideoNote       *file           `json:"video_note"`
	Voice           *file           `json:"voice"`
	Audio           *file           `json:"audio"`
	Sticker         *file           `json:"sticker"`
	Contact         *contact        `json:"contact"`
	Location        *location       `json:"location"`
	Venue           *venue          `json:"venue"`
	Poll            json.RawMessage `json:"poll"`
	Dice            json.RawMessage `json:"dice"`
	Game            json.RawMessage `json:"game"`
	Story           json.RawMessage `json:"story"`
	ReplyMarkup     *struct {
		InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
	} `json:"reply_markup"`
}

type callbackQuery struct {
	ID      string   `json:"id"`
	From    user     `json:"from"`
	Message *message `json:"message"`
	Data    string   `json:"data"`
}

type update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *message       `json:"message"`
	EditedMessage *message       `json:"edited_message"`
	CallbackQuery *callbackQuery `json:"callback_query"`
}

func msgRef(chatID, messageID int64) string {
	return strconv.FormatInt(chatID, 10) + ":" + strconv.FormatInt(messageID, 10)
}

// ParseWebhook implements channels.Adapter. One Telegram update carries at
// most one message; ignored updates yield an empty result.
func (a *Adapter) ParseWebhook(ctx context.Context, _ *http.Request, body []byte, inst channels.Instance) (channels.WebhookResult, error) {
	var u update
	if err := json.Unmarshal(body, &u); err != nil {
		return channels.WebhookResult{}, fmt.Errorf("telegram: decode update: %w", err)
	}
	token, err := tokenOf(inst)
	if err != nil {
		return channels.WebhookResult{}, err
	}
	var out channels.WebhookResult
	switch {
	case u.Message != nil:
		if m, ok := a.fromMessage(ctx, inst, token, u.Message); ok {
			out.Messages = append(out.Messages, m)
		}
	case u.CallbackQuery != nil:
		if m, ok := a.fromCallback(inst, token, u.CallbackQuery); ok {
			out.Messages = append(out.Messages, m)
		}
	}
	return out, nil
}

func privateSender(c chat, from *user) channels.Sender {
	name := strings.TrimSpace(c.FirstName + " " + c.LastName)
	username := c.Username
	meta := map[string]any{"chat_type": c.Type}
	if from != nil {
		if name == "" {
			name = from.FullName()
		}
		if username == "" {
			username = from.Username
		}
		if from.LanguageCode != "" {
			meta["language_code"] = from.LanguageCode
		}
	}
	return channels.Sender{
		ExternalID: strconv.FormatInt(c.ID, 10), InstanceScoped: true,
		Name: name, Username: username, Meta: meta,
	}
}

func groupSender(c chat) channels.Sender {
	return channels.Sender{
		ExternalID: strconv.FormatInt(c.ID, 10), InstanceScoped: true,
		Name: c.Title, Username: c.Username, Meta: map[string]any{"chat_type": c.Type},
	}
}

func addAuthor(attrs map[string]any, u *user, chatType string) {
	attrs["chat_type"] = chatType
	if u == nil {
		return
	}
	attrs["author_id"] = strconv.FormatInt(u.ID, 10)
	attrs["author_name"] = u.FullName()
	if u.Username != "" {
		attrs["author_username"] = u.Username
	}
}

func isGroup(t string) bool { return t == "group" || t == "supergroup" }

func (a *Adapter) fromMessage(ctx context.Context, inst channels.Instance, token string, m *message) (channels.InboundMessage, bool) {
	attrs := map[string]any{}
	var from channels.Sender
	switch {
	case m.Chat.Type == "private":
		from = privateSender(m.Chat, m.From)
	case isGroup(m.Chat.Type):
		botID := botIDOf(token, inst)
		if m.From != nil && m.From.ID == botID {
			return channels.InboundMessage{}, false
		}
		if !a.addressed(ctx, inst, token, botID, m) {
			return channels.InboundMessage{}, false
		}
		from = groupSender(m.Chat)
		addAuthor(attrs, m.From, m.Chat.Type)
	default: // channels (channel_post never arrives here) and unknown types
		return channels.InboundMessage{}, false
	}

	text := m.Text
	if text == "" {
		text = m.Caption
	}
	kind := "text"
	atts := a.attachments(token, m)
	if len(atts) > 0 {
		kind = "media"
	}
	switch {
	case m.Contact != nil:
		c := m.Contact
		name := strings.TrimSpace(c.FirstName + " " + c.LastName)
		text = strings.TrimSpace("Contact: " + strings.TrimSpace(name+" "+c.PhoneNumber))
		attrs["contact_name"] = name
		attrs["contact_phone"] = c.PhoneNumber
		if c.UserID != 0 {
			attrs["contact_user_id"] = strconv.FormatInt(c.UserID, 10)
		}
	case m.Venue != nil:
		v := m.Venue
		text = strings.TrimSpace(fmt.Sprintf("Location: %s, %s (%g, %g)", v.Title, v.Address, v.Location.Latitude, v.Location.Longitude))
		attrs["latitude"], attrs["longitude"] = v.Location.Latitude, v.Location.Longitude
		attrs["venue_title"], attrs["venue_address"] = v.Title, v.Address
	case m.Location != nil:
		text = fmt.Sprintf("Location: %g, %g", m.Location.Latitude, m.Location.Longitude)
		attrs["latitude"], attrs["longitude"] = m.Location.Latitude, m.Location.Longitude
	}
	if text == "" && len(atts) == 0 {
		t := unsupportedType(m)
		if t == "" { // service message (pinned, title change …)
			return channels.InboundMessage{}, false
		}
		kind = "unsupported"
		attrs["telegram_type"] = t
	}
	if m.MessageThreadID != 0 {
		attrs["message_thread_id"] = m.MessageThreadID
	}
	in := channels.InboundMessage{
		ExternalID:  msgRef(m.Chat.ID, m.MessageID),
		From:        from,
		Kind:        kind,
		Text:        text,
		Attachments: atts,
		At:          time.Unix(m.Date, 0).UTC(),
	}
	if m.ReplyToMessage != nil {
		in.ReplyToExternalID = msgRef(m.Chat.ID, m.ReplyToMessage.MessageID)
	}
	if len(attrs) > 0 {
		in.Attributes = attrs
	}
	return in, true
}

func unsupportedType(m *message) string {
	switch {
	case len(m.Poll) > 0:
		return "poll"
	case len(m.Dice) > 0:
		return "dice"
	case len(m.Game) > 0:
		return "game"
	case len(m.Story) > 0:
		return "story"
	}
	return ""
}

// addressed reports whether a group message is meant for the bot.
func (a *Adapter) addressed(ctx context.Context, inst channels.Instance, token string, botID int64, m *message) bool {
	if r := m.ReplyToMessage; r != nil && r.From != nil && r.From.ID == botID {
		return true
	}
	text, ents := m.Text, m.Entities
	if text == "" {
		text, ents = m.Caption, m.CaptionEntities
	}
	needName := false
	for _, e := range ents {
		switch e.Type {
		case "text_mention":
			if e.User != nil && e.User.ID == botID {
				return true
			}
		case "mention", "bot_command":
			needName = true
		}
	}
	if !needName {
		return false
	}
	name := a.botUsername(ctx, inst, token)
	if name == "" {
		return false
	}
	for _, e := range ents {
		s := entityText(text, e)
		switch e.Type {
		case "mention":
			if strings.EqualFold(s, "@"+name) {
				return true
			}
		case "bot_command":
			if i := strings.IndexByte(s, '@'); i >= 0 && strings.EqualFold(s[i+1:], name) {
				return true
			}
		}
	}
	return false
}

// entityText cuts an entity out of text; offsets are UTF-16 code units.
func entityText(text string, e entity) string {
	u := utf16.Encode([]rune(text))
	if e.Offset < 0 || e.Length <= 0 || e.Offset+e.Length > len(u) {
		return ""
	}
	return string(utf16.Decode(u[e.Offset : e.Offset+e.Length]))
}

func (a *Adapter) attachments(token string, m *message) []channels.InboundAttachment {
	var out []channels.InboundAttachment
	add := func(f *file, name, ctype string) {
		if f == nil || f.FileID == "" {
			return
		}
		if f.FileName != "" {
			name = f.FileName
		}
		if f.MimeType != "" {
			ctype = f.MimeType
		}
		out = append(out, channels.InboundAttachment{
			Filename: name, ContentType: ctype, Size: f.FileSize,
			Open: a.opener(token, f.FileID, f.FileSize),
		})
	}
	if n := len(m.Photo); n > 0 {
		p := m.Photo[0]
		for _, s := range m.Photo[1:] { // sizes are ascending, but pick by area to be safe
			if s.Width*s.Height > p.Width*p.Height {
				p = s
			}
		}
		add(&file{FileID: p.FileID, FileSize: p.FileSize}, "photo_"+p.UniqueID+".jpg", "image/jpeg")
	}
	if m.Animation != nil { // an animation message also carries the same file as document
		add(m.Animation, "animation.mp4", "video/mp4")
	} else {
		add(m.Document, "document", "application/octet-stream")
	}
	add(m.Video, "video.mp4", "video/mp4")
	add(m.VideoNote, "video_note.mp4", "video/mp4")
	add(m.Voice, "voice.ogg", "audio/ogg")
	add(m.Audio, "audio.mp3", "audio/mpeg")
	if s := m.Sticker; s != nil {
		switch {
		case s.IsAnimated:
			add(s, "sticker.tgs", "application/x-tgsticker")
		case s.IsVideo:
			add(s, "sticker.webm", "video/webm")
		default:
			add(s, "sticker.webp", "image/webp")
		}
	}
	return out
}

// opener streams a file via getFile + the file download endpoint.
func (a *Adapter) opener(token, fileID string, size int64) func(ctx context.Context) (io.ReadCloser, error) {
	return func(ctx context.Context) (io.ReadCloser, error) {
		if size > MaxDownloadSize {
			return nil, ErrFileTooLarge
		}
		var f struct {
			FilePath string `json:"file_path"`
			FileSize int64  `json:"file_size"`
		}
		if err := a.call(ctx, token, "getFile", map[string]any{"file_id": fileID}, &f); err != nil {
			return nil, err
		}
		if f.FileSize > MaxDownloadSize {
			return nil, ErrFileTooLarge
		}
		if f.FilePath == "" {
			return nil, fmt.Errorf("telegram: getFile returned no file_path")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/file/bot"+token+"/"+f.FilePath, nil)
		if err != nil {
			return nil, fmt.Errorf("telegram: download: %w", redact(err))
		}
		resp, err := a.hc.Do(req)
		if err != nil {
			return nil, fmt.Errorf("telegram: download: %w", redact(err))
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("telegram: download: status %d", resp.StatusCode)
		}
		return resp.Body, nil
	}
}

func (a *Adapter) fromCallback(inst channels.Instance, token string, cq *callbackQuery) (channels.InboundMessage, bool) {
	if cq.Message == nil || cq.ID == "" { // inline-mode messages have no chat
		return channels.InboundMessage{}, false
	}
	c := cq.Message.Chat
	attrs := map[string]any{"callback_data": cq.Data}
	var from channels.Sender
	switch {
	case c.Type == "private":
		from = privateSender(c, &cq.From)
	case isGroup(c.Type):
		from = groupSender(c)
		addAuthor(attrs, &cq.From, c.Type)
	default:
		return channels.InboundMessage{}, false
	}
	text := cq.Data
	if cq.Message.ReplyMarkup != nil {
	find:
		for _, row := range cq.Message.ReplyMarkup.InlineKeyboard {
			for _, b := range row {
				if b.CallbackData == cq.Data && b.Text != "" {
					text = b.Text
					attrs["button_title"] = b.Text
					break find
				}
			}
		}
	}
	a.answerCallback(token, cq.ID)
	return channels.InboundMessage{
		ExternalID:        "cb:" + cq.ID,
		From:              from,
		Kind:              "interactive",
		Text:              text,
		ReplyToExternalID: msgRef(c.ID, cq.Message.MessageID),
		Attributes:        attrs,
		At:                a.now().UTC(),
	}, true
}

// answerCallback stops the button's spinner; best effort, never blocks.
func (a *Adapter) answerCallback(token, id string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = a.call(ctx, token, "answerCallbackQuery", map[string]any{"callback_query_id": id}, nil)
	}()
}
