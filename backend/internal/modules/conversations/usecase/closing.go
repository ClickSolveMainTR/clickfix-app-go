package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Closing texts (CF-250): an agent picks one in the composer and it is
// appended to the end of the reply before it is stored, so every channel
// (API included) sends the same text the timeline shows. Autonomous AI
// replies are the exception: aiClosing appends the default at delivery only,
// so their stored text has none.

// Closing modes of a post.
const (
	// ClosingOff never adds a closing text (system senders, notes).
	ClosingOff = ""
	// ClosingAuto uses the author's last pick, else the default.
	ClosingAuto = "auto"
	// ClosingNone sends without a closing text and remembers that.
	ClosingNone = "none"
	// ClosingPick uses Closing.UUID and remembers it.
	ClosingPick = "pick"
)

// AttrClosingText marks a message whose text ends with a closing text, so the
// email brand signature is not added a second time.
const AttrClosingText = "closing_text"

// Closing is the closing text choice of a post.
type Closing struct {
	Mode string
	UUID uuid.UUID
}

// ClosingOption is one closing text offered for a conversation, rendered.
type ClosingOption struct {
	UUID      uuid.UUID  `json:"uuid"`
	Name      string     `json:"name"`
	Text      string     `json:"text"`
	IsDefault bool       `json:"is_default"`
	BrandUUID *uuid.UUID `json:"brand_uuid"`
}

// ClosingOptions is the composer's closing text picker for a conversation.
type ClosingOptions struct {
	Items []ClosingOption `json:"items"`
	// Selected is the text the composer starts with (the agent's last pick,
	// else the default); nil means no closing text.
	Selected *uuid.UUID `json:"selected"`
	// Language is the language the texts were rendered in (tr|en).
	Language string `json:"language"`
}

// ClosingOptionsFor lists the closing texts an agent may use on a
// conversation, rendered for it, with the pick the composer starts with.
func (s *Service) ClosingOptionsFor(ctx context.Context, org int64, agent Actor, conversation uuid.UUID) (ClosingOptions, error) {
	q := s.store.Q()
	conv, err := q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: conversation})
	if err != nil {
		return ClosingOptions{}, notFound(err)
	}
	if err := s.mustSee(ctx, org, conv.ID); err != nil {
		return ClosingOptions{}, err
	}
	rows, err := q.ListClosingTextsForBrand(ctx, db.ListClosingTextsForBrandParams{OrganizationID: org, BrandID: conv.BrandID})
	if err != nil {
		return ClosingOptions{}, err
	}
	vars, lang := s.closingVars(ctx, conv, agent.ID)
	out := ClosingOptions{Items: make([]ClosingOption, 0, len(rows)), Language: lang}
	for _, r := range rows {
		o := ClosingOption{UUID: r.Uuid, Name: r.Name, Text: renderClosing(closingBody(r, lang), vars), IsDefault: r.IsDefault}
		if r.BrandID.Valid {
			b := uuid.UUID(r.BrandID.Bytes)
			o.BrandUUID = &b
		}
		out.Items = append(out.Items, o)
	}
	picked, err := s.resolveClosing(ctx, conv, agent.ID, Closing{Mode: ClosingAuto})
	if err != nil {
		return ClosingOptions{}, err
	}
	if picked != nil {
		out.Selected = &picked.Uuid
	}
	return out, nil
}

// applyClosing appends the chosen closing text to an agent reply and records
// it in the message attributes. Returns the new content and attributes.
func (s *Service) applyClosing(ctx context.Context, conv db.Conversation, authorID int64, c Closing, content string, attrs map[string]any) (string, map[string]any, error) {
	if c.Mode == ClosingOff {
		return content, attrs, nil
	}
	t, err := s.resolveClosing(ctx, conv, authorID, c)
	if err != nil {
		return "", nil, err
	}
	if authorID > 0 && (c.Mode == ClosingNone || c.Mode == ClosingPick) {
		var id pgtype.Int8
		if t != nil {
			id = pgtype.Int8{Int64: t.ID, Valid: true}
		}
		if err := s.store.Q().SetClosingTextChoice(ctx, db.SetClosingTextChoiceParams{OrganizationID: conv.OrganizationID, UserID: authorID, ClosingTextID: id}); err != nil {
			return "", nil, err
		}
	}
	if t == nil {
		return content, attrs, nil
	}
	vars, lang := s.closingVars(ctx, conv, authorID)
	text := strings.TrimSpace(renderClosing(closingBody(*t, lang), vars))
	if text == "" {
		return content, attrs, nil
	}
	if content != "" {
		content += "\n\n"
	}
	if attrs == nil {
		attrs = map[string]any{}
	}
	attrs[AttrClosingText] = map[string]any{"uuid": t.Uuid.String(), "name": t.Name}
	return content + text, attrs, nil
}

// resolveClosing returns the closing text of a choice, nil for none.
func (s *Service) resolveClosing(ctx context.Context, conv db.Conversation, authorID int64, c Closing) (*db.ClosingText, error) {
	q := s.store.Q()
	switch c.Mode {
	case ClosingNone, ClosingOff:
		return nil, nil
	case ClosingPick:
		t, err := q.GetClosingText(ctx, db.GetClosingTextParams{OrganizationID: conv.OrganizationID, Uuid: c.UUID})
		if err != nil || !closingFits(t, conv) {
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			return nil, fmt.Errorf("%w: closing text is not available on this conversation", ErrInvalid)
		}
		return &t, nil
	}
	if authorID > 0 {
		choice, err := q.GetClosingTextChoice(ctx, db.GetClosingTextChoiceParams{OrganizationID: conv.OrganizationID, UserID: authorID})
		switch {
		case err == nil && !choice.ClosingTextID.Valid:
			return nil, nil // the agent chose "no closing text"
		case err == nil:
			t, err := q.GetClosingTextByID(ctx, db.GetClosingTextByIDParams{OrganizationID: conv.OrganizationID, ID: choice.ClosingTextID.Int64})
			if err == nil && closingFits(t, conv) {
				return &t, nil
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, err
		}
	}
	return s.defaultClosing(ctx, conv)
}

func (s *Service) defaultClosing(ctx context.Context, conv db.Conversation) (*db.ClosingText, error) {
	t, err := s.store.Q().DefaultClosingText(ctx, db.DefaultClosingTextParams{OrganizationID: conv.OrganizationID, BrandID: conv.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// aiClosing appends the default closing text to an autonomous AI reply at
// delivery when the organization turned that on (off by default).
func (s *Service) aiClosing(ctx context.Context, conv db.Conversation, m db.Message, attrs map[string]any, text string) (string, bool) {
	if m.SenderType != model.SenderBot || attrs == nil || attrs["ai_trigger_message_uuid"] == nil || attrs[AttrClosingText] != nil {
		return text, false
	}
	set, err := s.store.Q().GetClosingTextSettings(ctx, conv.OrganizationID)
	if err != nil || !set.ApplyToAi {
		return text, false
	}
	t, err := s.defaultClosing(ctx, conv)
	if err != nil || t == nil {
		return text, false
	}
	vars, lang := s.closingVars(ctx, conv, 0)
	body := strings.TrimSpace(renderClosing(closingBody(*t, lang), vars))
	if body == "" {
		return text, false
	}
	if text != "" {
		text += "\n\n"
	}
	return text + body, true
}

func closingFits(t db.ClosingText, conv db.Conversation) bool {
	return !t.BrandID.Valid || (conv.BrandID.Valid && t.BrandID.Bytes == conv.BrandID.Bytes)
}

func closingBody(t db.ClosingText, lang string) string {
	if lang == "en" && strings.TrimSpace(t.BodyEn) != "" {
		return t.BodyEn
	}
	return t.BodyTr
}

// closingVars are the variables a closing text may use, and the language of
// the conversation's contact (en when the contact's locale is English).
func (s *Service) closingVars(ctx context.Context, conv db.Conversation, agentID int64) (map[string]string, string) {
	q := s.store.Q()
	vars := map[string]string{"conversation.number": fmt.Sprint(conv.Number), "conversation.subject": conv.Subject}
	lang := "tr"
	if agentID > 0 {
		if u, err := q.GetUserByID(ctx, agentID); err == nil {
			vars["agent.name"] = strings.TrimSpace(u.Name + " " + u.Surname)
			vars["agent.first_name"] = u.Name
			vars["agent.email"] = u.Email
		}
	}
	if c, err := q.GetContactByID(ctx, db.GetContactByIDParams{OrganizationID: conv.OrganizationID, ID: conv.ContactID}); err == nil {
		vars["contact.name"] = c.Name
		if f := strings.Fields(c.Name); len(f) > 0 {
			vars["contact.first_name"] = f[0]
		}
		if strings.HasPrefix(strings.ToLower(c.Locale.String), "en") {
			lang = "en"
		}
	}
	if o, err := q.GetOrganizationByID(ctx, conv.OrganizationID); err == nil {
		vars["organization.name"] = o.Name
	}
	if conv.BrandID.Valid {
		if b, err := q.GetBrand(ctx, db.GetBrandParams{OrganizationID: conv.OrganizationID, Uuid: uuid.UUID(conv.BrandID.Bytes)}); err == nil {
			vars["brand.name"] = b.Name
		}
	}
	if vars["brand.name"] == "" {
		vars["brand.name"] = vars["organization.name"]
	}
	return vars, lang
}

var closingVarRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)

// renderClosing replaces {{name}} variables; unknown ones become empty.
func renderClosing(body string, vars map[string]string) string {
	return closingVarRe.ReplaceAllStringFunc(body, func(m string) string {
		return vars[closingVarRe.FindStringSubmatch(m)[1]]
	})
}

// ClosingVariables lists the variables a closing text may use.
var ClosingVariables = []string{
	"agent.name", "agent.first_name", "agent.email", "contact.name", "contact.first_name",
	"brand.name", "organization.name", "conversation.number", "conversation.subject",
}
