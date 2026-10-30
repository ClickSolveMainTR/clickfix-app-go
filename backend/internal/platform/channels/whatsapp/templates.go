package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

// MessageTemplate is one template of a WABA.
type MessageTemplate struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Language       string           `json:"language"`
	Status         string           `json:"status"`
	Category       string           `json:"category"`
	Components     []map[string]any `json:"components,omitempty"`
	RejectedReason string           `json:"rejected_reason,omitempty"`
}

// Template button types.
const (
	ButtonQuickReply = "QUICK_REPLY"
	ButtonURL        = "URL"
	ButtonPhone      = "PHONE_NUMBER"
)

// TemplateButton is one button of a new template.
type TemplateButton struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	URL   string `json:"url,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// CreateTemplateInput is a template submitted for approval. Example holds
// one sample value per body placeholder ({{1}}..{{n}}).
type CreateTemplateInput struct {
	Name     string           `json:"name"`
	Language string           `json:"language"`
	Category string           `json:"category"`
	Body     string           `json:"body"`
	Example  []string         `json:"example"`
	Header   string           `json:"header"`
	Footer   string           `json:"footer"`
	Buttons  []TemplateButton `json:"buttons"`
}

var (
	templateNameRE = regexp.MustCompile(`^[a-z0-9_]{1,512}$`)
	placeholderRE  = regexp.MustCompile(`\{\{\s*([0-9]+)\s*\}\}`)
)

// TemplateVariables counts the {{n}} placeholders of a body; -1 when they
// are not exactly {{1}}..{{n}}.
func TemplateVariables(body string) int {
	seen := map[int]bool{}
	for _, m := range placeholderRE.FindAllStringSubmatch(body, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return -1
		}
		seen[n] = true
	}
	for i := 1; i <= len(seen); i++ {
		if !seen[i] {
			return -1
		}
	}
	return len(seen)
}

func wabaOf(inst channels.Instance) (string, string, error) {
	waba, token := strings.TrimSpace(inst.Credentials[FieldWABAID]), inst.Credentials[FieldAccessToken]
	if !ValidID(waba) || token == "" {
		return "", "", errors.New("whatsapp: waba_id and access_token are required")
	}
	return waba, token, nil
}

// ListTemplates lists the WABA's templates (100 per page, at most 10 pages).
func (a *Adapter) ListTemplates(ctx context.Context, inst channels.Instance) ([]MessageTemplate, error) {
	waba, token, err := wabaOf(inst)
	if err != nil {
		return nil, err
	}
	out := []MessageTemplate{}
	after := ""
	for page := 0; page < 10; page++ {
		q := url.Values{
			"fields": {"id,name,language,status,category,components,rejected_reason"},
			"limit":  {"100"},
		}
		if after != "" {
			q.Set("after", after)
		}
		body, err := a.call(ctx, http.MethodGet, waba+"/message_templates", q, nil, token)
		if err != nil {
			return nil, err
		}
		var res struct {
			Data   []MessageTemplate `json:"data"`
			Paging struct {
				Cursors struct {
					After string `json:"after"`
				} `json:"cursors"`
				Next string `json:"next"`
			} `json:"paging"`
		}
		if err := json.Unmarshal(body, &res); err != nil {
			return nil, &Error{Status: http.StatusOK, Message: "unreadable template list"}
		}
		for _, t := range res.Data {
			if t.RejectedReason == "NONE" {
				t.RejectedReason = ""
			}
			out = append(out, t)
		}
		if res.Paging.Next == "" || res.Paging.Cursors.After == "" || res.Paging.Cursors.After == after {
			break
		}
		after = res.Paging.Cursors.After
	}
	return out, nil
}

// CreateTemplate submits a template for Meta's review.
func (a *Adapter) CreateTemplate(ctx context.Context, inst channels.Instance, in CreateTemplateInput) (MessageTemplate, error) {
	waba, token, err := wabaOf(inst)
	if err != nil {
		return MessageTemplate{}, err
	}
	components, err := templateComponents(&in)
	if err != nil {
		return MessageTemplate{}, err
	}
	body, err := a.call(ctx, http.MethodPost, waba+"/message_templates", nil, map[string]any{
		"name": in.Name, "language": in.Language, "category": in.Category, "components": components,
	}, token)
	if err != nil {
		return MessageTemplate{}, err
	}
	var res struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Category string `json:"category"`
	}
	if err := json.Unmarshal(body, &res); err != nil || res.ID == "" {
		return MessageTemplate{}, &Error{Status: http.StatusOK, Message: "answer without a template id"}
	}
	return MessageTemplate{
		ID: res.ID, Name: in.Name, Language: in.Language, Status: res.Status,
		Category: firstNonEmpty(res.Category, in.Category), Components: components,
	}, nil
}

// templateComponents validates the input (normalizing it in place) and
// builds the Graph components.
func templateComponents(in *CreateTemplateInput) ([]map[string]any, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Language = firstNonEmpty(in.Language, DefaultLanguage)
	in.Category = strings.ToUpper(strings.TrimSpace(in.Category))
	in.Body = strings.TrimSpace(in.Body)
	if !templateNameRE.MatchString(in.Name) {
		return nil, errors.New("whatsapp: template name may only contain a-z, 0-9 and _")
	}
	switch in.Category {
	case "MARKETING", "UTILITY", "AUTHENTICATION":
	default:
		return nil, errors.New("whatsapp: category must be MARKETING, UTILITY or AUTHENTICATION")
	}
	if in.Body == "" || len([]rune(in.Body)) > 1024 {
		return nil, errors.New("whatsapp: body must be 1 to 1024 characters")
	}
	n := TemplateVariables(in.Body)
	if n < 0 {
		return nil, errors.New("whatsapp: body placeholders must be {{1}}..{{n}} without gaps")
	}
	if len(in.Example) != n {
		return nil, fmt.Errorf("whatsapp: body has %d placeholders but %d examples", n, len(in.Example))
	}
	comps := []map[string]any{}
	if h := strings.TrimSpace(in.Header); h != "" {
		if len([]rune(h)) > 60 || strings.Contains(h, "{{") {
			return nil, errors.New("whatsapp: header must be at most 60 characters without placeholders")
		}
		comps = append(comps, map[string]any{"type": "HEADER", "format": "TEXT", "text": h})
	}
	bodyComp := map[string]any{"type": "BODY", "text": in.Body}
	if n > 0 {
		bodyComp["example"] = map[string]any{"body_text": [][]string{in.Example}}
	}
	comps = append(comps, bodyComp)
	if f := strings.TrimSpace(in.Footer); f != "" {
		if len([]rune(f)) > 60 {
			return nil, errors.New("whatsapp: footer must be at most 60 characters")
		}
		comps = append(comps, map[string]any{"type": "FOOTER", "text": f})
	}
	if len(in.Buttons) > 10 {
		return nil, errors.New("whatsapp: at most 10 buttons")
	}
	if len(in.Buttons) > 0 {
		buttons := make([]map[string]any, 0, len(in.Buttons))
		for i, b := range in.Buttons {
			text := strings.TrimSpace(b.Text)
			if text == "" || len([]rune(text)) > 25 {
				return nil, fmt.Errorf("whatsapp: button %d text must be 1 to 25 characters", i+1)
			}
			btn := map[string]any{"type": strings.ToUpper(b.Type), "text": text}
			switch btn["type"] {
			case ButtonQuickReply:
			case ButtonURL:
				if !strings.HasPrefix(b.URL, "https://") && !strings.HasPrefix(b.URL, "http://") {
					return nil, fmt.Errorf("whatsapp: button %d needs an http(s) url", i+1)
				}
				btn["url"] = b.URL
			case ButtonPhone:
				if digits(b.Phone) == "" {
					return nil, fmt.Errorf("whatsapp: button %d needs a phone number", i+1)
				}
				btn["phone_number"] = "+" + digits(b.Phone)
			default:
				return nil, fmt.Errorf("whatsapp: button %d type must be QUICK_REPLY, URL or PHONE_NUMBER", i+1)
			}
			buttons = append(buttons, btn)
		}
		comps = append(comps, map[string]any{"type": "BUTTONS", "buttons": buttons})
	}
	return comps, nil
}

// DeleteTemplate deletes every language of a template by name.
func (a *Adapter) DeleteTemplate(ctx context.Context, inst channels.Instance, name string) error {
	waba, token, err := wabaOf(inst)
	if err != nil {
		return err
	}
	if !templateNameRE.MatchString(name) {
		return errors.New("whatsapp: invalid template name")
	}
	_, err = a.call(ctx, http.MethodDelete, waba+"/message_templates", url.Values{"name": {name}}, nil, token)
	return err
}
